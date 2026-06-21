package repository

import (
	"context"
	"errors"
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

func (walletModel) TableName() string {
	return "wallets"
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
