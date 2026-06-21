package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/yuliusw/RPA-market/services/wallet/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WalletRepository struct {
	db *gorm.DB
}

type walletModel struct {
	WalletID     uuid.UUID       `gorm:"column:wallet_id;type:uuid;primaryKey"`
	OwnerID      uuid.UUID       `gorm:"column:owner_id;type:uuid;not null"`
	OwnerType    string          `gorm:"column:owner_type;type:varchar(20);not null"`
	Balance      decimal.Decimal `gorm:"column:balance;type:decimal(18,4)"`
	CurrencyCode string          `gorm:"column:currency_code;type:varchar(10);not null"`
	Status       string          `gorm:"column:status;type:varchar(20);not null"`
	UpdatedAt    time.Time       `gorm:"column:updated_at"`
}

type transactionModel struct {
	TxID           uuid.UUID       `gorm:"column:tx_id;type:uuid;primaryKey"`
	WalletID       uuid.UUID       `gorm:"column:wallet_id;type:uuid;not null"`
	TxType         string          `gorm:"column:tx_type;type:varchar(50);not null"`
	Amount         decimal.Decimal `gorm:"column:amount;type:decimal(18,4);not null"`
	BalanceAfter   decimal.Decimal `gorm:"column:balance_after;type:decimal(18,4);not null"`
	ReferenceID    *uuid.UUID      `gorm:"column:reference_id;type:uuid"`
	IdempotencyKey *string         `gorm:"column:idempotency_key"`
	Description    string          `gorm:"column:description"`
	CreatedAt      time.Time       `gorm:"column:created_at"`
}

func (walletModel) TableName() string {
	return "wallets"
}

func (transactionModel) TableName() string {
	return "wallet_transactions"
}

func NewWalletRepository(db *gorm.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) FindByOwner(ctx context.Context, ownerID uuid.UUID, ownerType domain.OwnerType, currencyCode string) (*domain.Wallet, error) {
	var model walletModel
	err := r.db.WithContext(ctx).
		Where("owner_id = ? AND owner_type = ? AND currency_code = ?", ownerID, ownerType, normalizeCurrency(currencyCode)).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func (r *WalletRepository) FindByID(ctx context.Context, walletID uuid.UUID) (*domain.Wallet, error) {
	var model walletModel
	err := r.db.WithContext(ctx).Where("wallet_id = ?", walletID).First(&model).Error
	if err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func (r *WalletRepository) GetOrCreateByOwner(ctx context.Context, ownerID uuid.UUID, ownerType domain.OwnerType, currencyCode string) (*domain.Wallet, error) {
	currencyCode = normalizeCurrency(currencyCode)
	wallet := domain.NewWallet(ownerID, ownerType, currencyCode)
	model := walletModel{
		WalletID:     wallet.ID,
		OwnerID:      wallet.OwnerID,
		OwnerType:    string(wallet.OwnerType),
		Balance:      wallet.Balance,
		CurrencyCode: wallet.CurrencyCode,
		Status:       string(wallet.Status),
		UpdatedAt:    wallet.UpdatedAt,
	}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model).Error
	if err != nil {
		return nil, err
	}

	wallet, err = r.FindByOwner(ctx, ownerID, ownerType, currencyCode)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

func (r *WalletRepository) Credit(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, refID *uuid.UUID, desc, idempotencyKey string) (*domain.Wallet, *domain.Transaction, error) {
	return r.applyWalletChange(ctx, walletID, domain.TxTypeRecharge, amount, refID, desc, idempotencyKey)
}

func (r *WalletRepository) Debit(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, refID *uuid.UUID, desc, idempotencyKey string) (*domain.Wallet, *domain.Transaction, error) {
	return r.applyWalletChange(ctx, walletID, domain.TxTypeConsume, amount.Neg(), refID, desc, idempotencyKey)
}

func (r *WalletRepository) DebitInTx(ctx context.Context, tx *gorm.DB, walletID uuid.UUID, amount decimal.Decimal, refID *uuid.UUID, desc, idempotencyKey string) (*domain.Wallet, *domain.Transaction, error) {
	return r.applyWalletChangeInTx(ctx, tx, walletID, domain.TxTypeConsume, amount.Neg(), refID, desc, idempotencyKey)
}

func (r *WalletRepository) Transfer(ctx context.Context, fromWalletID, toWalletID uuid.UUID, amount decimal.Decimal, refID *uuid.UUID, desc, idempotencyKey string) (*domain.Transaction, *domain.Transaction, error) {
	if fromWalletID == toWalletID || amount.LessThanOrEqual(decimal.Zero) {
		return nil, nil, domain.ErrInvalidTransfer
	}

	var outTx, inTx *domain.Transaction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if idempotencyKey != "" {
			found, err := r.findExistingTransfer(ctx, tx, idempotencyKey, fromWalletID, toWalletID, amount)
			if err == nil && found != nil {
				outTx, inTx = found.out, found.in
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		firstID, secondID := fromWalletID, toWalletID
		if strings.Compare(firstID.String(), secondID.String()) > 0 {
			firstID, secondID = secondID, firstID
		}

		firstWallet, err := r.lockWallet(ctx, tx, firstID)
		if err != nil {
			return err
		}
		secondWallet, err := r.lockWallet(ctx, tx, secondID)
		if err != nil {
			return err
		}

		fromWallet, toWallet := firstWallet, secondWallet
		if firstID != fromWalletID {
			fromWallet, toWallet = secondWallet, firstWallet
		}
		if idempotencyKey != "" {
			found, err := r.findExistingTransfer(ctx, tx, idempotencyKey, fromWalletID, toWalletID, amount)
			if err == nil && found != nil {
				outTx, inTx = found.out, found.in
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		if err := fromWallet.Deduct(amount); err != nil {
			return err
		}
		if err := toWallet.Deposit(amount); err != nil {
			return err
		}

		if err := updateWallet(ctx, tx, fromWallet); err != nil {
			return err
		}
		if err := updateWallet(ctx, tx, toWallet); err != nil {
			return err
		}

		outKey, inKey := "", ""
		if idempotencyKey != "" {
			outKey = transferKey(idempotencyKey, "out")
			inKey = transferKey(idempotencyKey, "in")
		}
		outTx = domain.NewTransaction(fromWallet.ID, domain.TxTypeTransferOut, amount.Neg(), fromWallet.Balance, refID, outKey, desc)
		inTx = domain.NewTransaction(toWallet.ID, domain.TxTypeTransferIn, amount, toWallet.Balance, refID, inKey, desc)

		if err := tx.WithContext(ctx).Create(newTransactionModel(outTx)).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Create(newTransactionModel(inTx)).Error; err != nil {
			return err
		}
		return nil
	})
	return outTx, inTx, err
}

func (r *WalletRepository) ListTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]*domain.Transaction, error) {
	var models []transactionModel
	err := r.db.WithContext(ctx).
		Where("wallet_id = ?", walletID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	txs := make([]*domain.Transaction, 0, len(models))
	for _, model := range models {
		txs = append(txs, model.toDomain())
	}
	return txs, nil
}

func (r *WalletRepository) applyWalletChange(ctx context.Context, walletID uuid.UUID, txType domain.TxType, signedAmount decimal.Decimal, refID *uuid.UUID, desc, idempotencyKey string) (*domain.Wallet, *domain.Transaction, error) {
	if signedAmount.Equal(decimal.Zero) {
		return nil, nil, domain.ErrInvalidAmount
	}

	var wallet *domain.Wallet
	var transaction *domain.Transaction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		wallet, transaction, err = r.applyWalletChangeInTx(ctx, tx, walletID, txType, signedAmount, refID, desc, idempotencyKey)
		return err
	})
	return wallet, transaction, err
}

func (r *WalletRepository) applyWalletChangeInTx(ctx context.Context, tx *gorm.DB, walletID uuid.UUID, txType domain.TxType, signedAmount decimal.Decimal, refID *uuid.UUID, desc, idempotencyKey string) (*domain.Wallet, *domain.Transaction, error) {
	if idempotencyKey != "" {
		existing, err := r.findTransactionByKey(ctx, tx, idempotencyKey)
		if err == nil {
			if existing.WalletID != walletID || !existing.Amount.Equal(signedAmount) || existing.TxType != txType {
				return nil, nil, domain.ErrIdempotencyConflict
			}
			wallet, err := r.findWalletByID(ctx, tx, existing.WalletID)
			return wallet, existing, err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
	}

	wallet, err := r.lockWallet(ctx, tx, walletID)
	if err != nil {
		return nil, nil, err
	}
	if idempotencyKey != "" {
		existing, err := r.findTransactionByKey(ctx, tx, idempotencyKey)
		if err == nil {
			if !sameTransaction(existing, walletID, txType, signedAmount) {
				return nil, nil, domain.ErrIdempotencyConflict
			}
			return wallet, existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
	}

	if signedAmount.IsPositive() {
		err = wallet.Deposit(signedAmount)
	} else {
		err = wallet.Deduct(signedAmount.Abs())
	}
	if err != nil {
		return nil, nil, err
	}

	if err := updateWallet(ctx, tx, wallet); err != nil {
		return nil, nil, err
	}

	transaction := domain.NewTransaction(wallet.ID, txType, signedAmount, wallet.Balance, refID, idempotencyKey, desc)
	if err := tx.WithContext(ctx).Create(newTransactionModel(transaction)).Error; err != nil {
		return nil, nil, err
	}
	return wallet, transaction, nil
}

func (r *WalletRepository) lockWallet(ctx context.Context, tx *gorm.DB, walletID uuid.UUID) (*domain.Wallet, error) {
	var model walletModel
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("wallet_id = ?", walletID).First(&model).Error
	if err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func (r *WalletRepository) findWalletByID(ctx context.Context, tx *gorm.DB, walletID uuid.UUID) (*domain.Wallet, error) {
	var model walletModel
	err := tx.WithContext(ctx).Where("wallet_id = ?", walletID).First(&model).Error
	if err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func (r *WalletRepository) findTransactionByKey(ctx context.Context, tx *gorm.DB, idempotencyKey string) (*domain.Transaction, error) {
	var model transactionModel
	err := tx.WithContext(ctx).Where("idempotency_key = ?", idempotencyKey).First(&model).Error
	if err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func updateWallet(ctx context.Context, tx *gorm.DB, wallet *domain.Wallet) error {
	return tx.WithContext(ctx).Model(&walletModel{}).
		Where("wallet_id = ?", wallet.ID).
		Updates(map[string]any{
			"balance":    wallet.Balance,
			"updated_at": wallet.UpdatedAt,
		}).Error
}

func transferKey(idempotencyKey, side string) string {
	return fmt.Sprintf("%s:%s", idempotencyKey, side)
}

type existingTransfer struct {
	out *domain.Transaction
	in  *domain.Transaction
}

func (r *WalletRepository) findExistingTransfer(ctx context.Context, tx *gorm.DB, idempotencyKey string, fromWalletID, toWalletID uuid.UUID, amount decimal.Decimal) (*existingTransfer, error) {
	outTx, err := r.findTransactionByKey(ctx, tx, transferKey(idempotencyKey, "out"))
	if err != nil {
		return nil, err
	}
	inTx, err := r.findTransactionByKey(ctx, tx, transferKey(idempotencyKey, "in"))
	if err != nil {
		return nil, err
	}
	if !sameTransaction(outTx, fromWalletID, domain.TxTypeTransferOut, amount.Neg()) || !sameTransaction(inTx, toWalletID, domain.TxTypeTransferIn, amount) {
		return nil, domain.ErrIdempotencyConflict
	}
	return &existingTransfer{out: outTx, in: inTx}, nil
}

func sameTransaction(tx *domain.Transaction, walletID uuid.UUID, txType domain.TxType, amount decimal.Decimal) bool {
	return tx.WalletID == walletID && tx.TxType == txType && tx.Amount.Equal(amount)
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func normalizeCurrency(currencyCode string) string {
	currencyCode = strings.TrimSpace(strings.ToUpper(currencyCode))
	if currencyCode == "" {
		return "COIN"
	}
	return currencyCode
}

func (m walletModel) toDomain() *domain.Wallet {
	return &domain.Wallet{
		ID:           m.WalletID,
		OwnerID:      m.OwnerID,
		OwnerType:    domain.OwnerType(m.OwnerType),
		Balance:      m.Balance,
		CurrencyCode: m.CurrencyCode,
		Status:       domain.WalletStatus(m.Status),
		UpdatedAt:    m.UpdatedAt,
	}
}

func newTransactionModel(tx *domain.Transaction) *transactionModel {
	idempotencyKey := strings.TrimSpace(tx.IdempotencyKey)
	var idempotencyKeyPtr *string
	if idempotencyKey != "" {
		idempotencyKeyPtr = &idempotencyKey
	}

	return &transactionModel{
		TxID:           tx.TxID,
		WalletID:       tx.WalletID,
		TxType:         string(tx.TxType),
		Amount:         tx.Amount,
		BalanceAfter:   tx.BalanceAfter,
		ReferenceID:    tx.ReferenceID,
		IdempotencyKey: idempotencyKeyPtr,
		Description:    tx.Description,
		CreatedAt:      tx.CreatedAt,
	}
}

func (m transactionModel) toDomain() *domain.Transaction {
	idempotencyKey := ""
	if m.IdempotencyKey != nil {
		idempotencyKey = *m.IdempotencyKey
	}

	return &domain.Transaction{
		TxID:           m.TxID,
		WalletID:       m.WalletID,
		TxType:         domain.TxType(m.TxType),
		Amount:         m.Amount,
		BalanceAfter:   m.BalanceAfter,
		ReferenceID:    m.ReferenceID,
		IdempotencyKey: idempotencyKey,
		Description:    m.Description,
		CreatedAt:      m.CreatedAt,
	}
}
