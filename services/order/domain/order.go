package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidOrderAmount  = errors.New("order: amount must be positive")
	ErrOrderAlreadyPaid    = errors.New("order: already paid")
	ErrOrderNotPayable     = errors.New("order: not payable")
	ErrOrderNotCancellable = errors.New("order: not cancellable")
	ErrOrderForbidden      = errors.New("order: forbidden")
	ErrOrderIdempotency    = errors.New("order: idempotency key already used for another order")
	ErrAppNotFound         = errors.New("order: app not found")
)

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type Order struct {
	OrderID        uuid.UUID
	UserID         uuid.UUID
	AppID          uuid.UUID
	WalletID       uuid.UUID
	Amount         decimal.Decimal
	CurrencyCode   string
	Status         OrderStatus
	TxID           *uuid.UUID
	SubscriptionID *uuid.UUID
	IdempotencyKey string
	Description    string
	CreatedAt      time.Time
	PaidAt         *time.Time
	UpdatedAt      time.Time
}

func NewOrder(userID, appID, walletID uuid.UUID, amount decimal.Decimal, currencyCode, idempotencyKey, description string) (*Order, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidOrderAmount
	}
	now := time.Now()
	return &Order{
		OrderID:        uuid.New(),
		UserID:         userID,
		AppID:          appID,
		WalletID:       walletID,
		Amount:         amount,
		CurrencyCode:   currencyCode,
		Status:         OrderStatusPending,
		IdempotencyKey: idempotencyKey,
		Description:    description,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (o *Order) MarkPaid(txID, subscriptionID uuid.UUID) error {
	if o.Status == OrderStatusPaid {
		return ErrOrderAlreadyPaid
	}
	if o.Status != OrderStatusPending {
		return ErrOrderNotPayable
	}
	now := time.Now()
	o.Status = OrderStatusPaid
	o.TxID = &txID
	o.SubscriptionID = &subscriptionID
	o.PaidAt = &now
	o.UpdatedAt = now
	return nil
}

func (o *Order) MarkSubscription(subscriptionID uuid.UUID) {
	o.SubscriptionID = &subscriptionID
	o.UpdatedAt = time.Now()
}

func (o *Order) Cancel() error {
	if o.Status != OrderStatusPending {
		return ErrOrderNotCancellable
	}
	o.Status = OrderStatusCancelled
	o.UpdatedAt = time.Now()
	return nil
}
