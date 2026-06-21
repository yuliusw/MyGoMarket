package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrInsufficientBalance = errors.New("wallet: insufficient balance")
	ErrWalletInactive      = errors.New("wallet: status is not active")
	ErrInvalidAmount       = errors.New("wallet: amount must be positive")
	ErrIdempotencyConflict = errors.New("wallet: idempotency key already used for another operation")
	ErrInvalidTransfer     = errors.New("wallet: invalid transfer")
)

type OwnerType string

const (
	OwnerTypeUser  OwnerType = "user"
	OwnerTypeGroup OwnerType = "group"
)

type WalletStatus string

const (
	WalletStatusActive WalletStatus = "active"
	WalletStatusFrozen WalletStatus = "frozen"
)

// Wallet 钱包领域实体
type Wallet struct {
	ID           uuid.UUID
	OwnerID      uuid.UUID
	OwnerType    OwnerType
	Balance      decimal.Decimal
	CurrencyCode string
	Status       WalletStatus
	UpdatedAt    time.Time
}

// NewWallet 工厂方法，用于创建新钱包
func NewWallet(ownerID uuid.UUID, ownerType OwnerType, currencyCode string) *Wallet {
	return &Wallet{
		ID:           uuid.New(),
		OwnerID:      ownerID,
		OwnerType:    ownerType,
		Balance:      decimal.Zero,
		CurrencyCode: currencyCode,
		Status:       WalletStatusActive,
		UpdatedAt:    time.Now(),
	}
}

// Deduct 扣减余额（核心防超卖逻辑）
func (w *Wallet) Deduct(amount decimal.Decimal) error {
	if w.Status != WalletStatusActive {
		return ErrWalletInactive
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}
	if w.Balance.LessThan(amount) {
		return ErrInsufficientBalance
	}

	w.Balance = w.Balance.Sub(amount)
	w.UpdatedAt = time.Now()
	return nil
}

// Deposit 增加余额
func (w *Wallet) Deposit(amount decimal.Decimal) error {
	if w.Status != WalletStatusActive {
		return ErrWalletInactive
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	w.Balance = w.Balance.Add(amount)
	w.UpdatedAt = time.Now()
	return nil
}
