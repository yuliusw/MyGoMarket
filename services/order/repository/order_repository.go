package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	orderdomain "github.com/yuliusw/RPA-market/services/order/domain"
	walletdomain "github.com/yuliusw/RPA-market/services/wallet/domain"
	walletrepo "github.com/yuliusw/RPA-market/services/wallet/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrderRepository struct {
	db     *gorm.DB
	wallet *walletrepo.WalletRepository
}

type orderModel struct {
	OrderID        uuid.UUID       `gorm:"column:order_id;type:uuid;primaryKey"`
	UserID         uuid.UUID       `gorm:"column:user_id;type:uuid;not null"`
	AppID          uuid.UUID       `gorm:"column:app_id;type:uuid;not null"`
	WalletID       uuid.UUID       `gorm:"column:wallet_id;type:uuid;not null"`
	Amount         decimal.Decimal `gorm:"column:amount;type:decimal(18,4);not null"`
	CurrencyCode   string          `gorm:"column:currency_code;type:varchar(10);not null"`
	Status         string          `gorm:"column:status;type:varchar(20);not null"`
	TxID           *uuid.UUID      `gorm:"column:tx_id;type:uuid"`
	SubscriptionID *uuid.UUID      `gorm:"column:subscription_id;type:uuid"`
	IdempotencyKey *string         `gorm:"column:idempotency_key"`
	Description    string          `gorm:"column:description"`
	CreatedAt      time.Time       `gorm:"column:created_at"`
	PaidAt         *time.Time      `gorm:"column:paid_at"`
	UpdatedAt      time.Time       `gorm:"column:updated_at"`
}

type appModel struct {
	AppID  uuid.UUID `gorm:"column:app_id;type:uuid;primaryKey"`
	Status string    `gorm:"column:status"`
}

type subscriptionModel struct {
	SubID         uuid.UUID  `gorm:"column:sub_id;type:uuid"`
	AppID         uuid.UUID  `gorm:"column:app_id;type:uuid"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid"`
	PlanType      string     `gorm:"column:plan_type"`
	ExpiredAt     time.Time  `gorm:"column:expired_at"`
	Status        string     `gorm:"column:status"`
	SourceOrderID *uuid.UUID `gorm:"column:source_order_id;type:uuid"`
}

func (orderModel) TableName() string {
	return "orders"
}

func (appModel) TableName() string {
	return "apps"
}

func (subscriptionModel) TableName() string {
	return "subscriptions"
}

func NewOrderRepository(db *gorm.DB, wallet *walletrepo.WalletRepository) *OrderRepository {
	return &OrderRepository{db: db, wallet: wallet}
}

func (r *OrderRepository) Create(ctx context.Context, userID, appID uuid.UUID, amount decimal.Decimal, currencyCode, idempotencyKey, description string) (*orderdomain.Order, error) {
	currencyCode = normalizeCurrency(currencyCode)
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	if idempotencyKey != "" {
		existing, err := r.findByIdempotencyKey(ctx, idempotencyKey)
		if err == nil {
			if existing.UserID != userID || existing.AppID != appID || !existing.Amount.Equal(amount) || existing.CurrencyCode != currencyCode {
				return nil, orderdomain.ErrOrderIdempotency
			}
			return existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	var app appModel
	if err := r.db.WithContext(ctx).Where("app_id = ? AND status = ?", appID, "published").First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, orderdomain.ErrAppNotFound
		}
		return nil, err
	}
	wallet, err := r.wallet.GetOrCreateByOwner(ctx, userID, walletdomain.OwnerTypeUser, currencyCode)
	if err != nil {
		return nil, err
	}

	order, err := orderdomain.NewOrder(userID, appID, wallet.ID, amount, currencyCode, idempotencyKey, description)
	if err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Create(newOrderModel(order)).Error; err != nil {
		return nil, err
	}
	return order, nil
}

func (r *OrderRepository) FindByID(ctx context.Context, orderID uuid.UUID) (*orderdomain.Order, error) {
	var model orderModel
	if err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&model).Error; err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func (r *OrderRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*orderdomain.Order, error) {
	var models []orderModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&models).Error; err != nil {
		return nil, err
	}

	orders := make([]*orderdomain.Order, 0, len(models))
	for _, model := range models {
		orders = append(orders, model.toDomain())
	}
	return orders, nil
}

func (r *OrderRepository) Pay(ctx context.Context, orderID, userID uuid.UUID, idempotencyKey string) (*orderdomain.Order, error) {
	var order *orderdomain.Order
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := r.lockOrder(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if locked.UserID != userID {
			return orderdomain.ErrOrderForbidden
		}
		if locked.Status == orderdomain.OrderStatusPaid {
			if locked.SubscriptionID == nil {
				subID, err := r.grantSubscription(ctx, tx, locked)
				if err != nil {
					return err
				}
				locked.MarkSubscription(subID)
				if err := updateOrder(ctx, tx, locked); err != nil {
					return err
				}
			}
			order = locked
			return nil
		}
		if locked.Status != orderdomain.OrderStatusPending {
			return orderdomain.ErrOrderNotPayable
		}

		refID := locked.OrderID
		paymentKey := paymentIdempotencyKey(locked.OrderID, idempotencyKey)
		_, walletTx, err := r.wallet.DebitInTx(ctx, tx, locked.WalletID, locked.Amount, &refID, "order payment", paymentKey)
		if err != nil {
			return err
		}
		subID, err := r.grantSubscription(ctx, tx, locked)
		if err != nil {
			return err
		}
		if err := locked.MarkPaid(walletTx.TxID, subID); err != nil {
			return err
		}
		if err := updateOrder(ctx, tx, locked); err != nil {
			return err
		}
		order = locked
		return nil
	})
	return order, err
}

func (r *OrderRepository) Purchase(ctx context.Context, userID, appID uuid.UUID, amount decimal.Decimal, currencyCode, idempotencyKey, description string) (*orderdomain.Order, error) {
	order, err := r.Create(ctx, userID, appID, amount, currencyCode, idempotencyKey, description)
	if err != nil {
		return nil, err
	}
	return r.Pay(ctx, order.OrderID, userID, idempotencyKey)
}

func (r *OrderRepository) Cancel(ctx context.Context, orderID, userID uuid.UUID) (*orderdomain.Order, error) {
	var order *orderdomain.Order
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := r.lockOrder(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if locked.UserID != userID {
			return orderdomain.ErrOrderForbidden
		}
		if err := locked.Cancel(); err != nil {
			return err
		}
		if err := updateOrder(ctx, tx, locked); err != nil {
			return err
		}
		order = locked
		return nil
	})
	return order, err
}

func (r *OrderRepository) lockOrder(ctx context.Context, tx *gorm.DB, orderID uuid.UUID) (*orderdomain.Order, error) {
	var model orderModel
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", orderID).First(&model).Error; err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func (r *OrderRepository) findByIdempotencyKey(ctx context.Context, idempotencyKey string) (*orderdomain.Order, error) {
	var model orderModel
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", idempotencyKey).First(&model).Error; err != nil {
		return nil, err
	}
	return model.toDomain(), nil
}

func updateOrder(ctx context.Context, tx *gorm.DB, order *orderdomain.Order) error {
	return tx.WithContext(ctx).Model(&orderModel{}).
		Where("order_id = ?", order.OrderID).
		Updates(map[string]any{
			"status":          string(order.Status),
			"tx_id":           order.TxID,
			"subscription_id": order.SubscriptionID,
			"paid_at":         order.PaidAt,
			"updated_at":      order.UpdatedAt,
		}).Error
}

func (r *OrderRepository) grantSubscription(ctx context.Context, tx *gorm.DB, order *orderdomain.Order) (uuid.UUID, error) {
	if order.SubscriptionID != nil {
		return *order.SubscriptionID, nil
	}
	if existing, err := r.findSubscriptionByOrder(ctx, tx, order.OrderID); err == nil {
		return existing.SubID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, err
	}

	sub := subscriptionModel{
		SubID:         uuid.New(),
		AppID:         order.AppID,
		UserID:        order.UserID,
		PlanType:      "purchase",
		ExpiredAt:     time.Now().AddDate(1, 0, 0),
		Status:        "active",
		SourceOrderID: &order.OrderID,
	}
	if err := tx.WithContext(ctx).Create(&sub).Error; err != nil {
		return uuid.Nil, err
	}
	return sub.SubID, nil
}

func (r *OrderRepository) findSubscriptionByOrder(ctx context.Context, tx *gorm.DB, orderID uuid.UUID) (*subscriptionModel, error) {
	var model subscriptionModel
	if err := tx.WithContext(ctx).Where("source_order_id = ?", orderID).Order("expired_at DESC").First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

func paymentIdempotencyKey(orderID uuid.UUID, idempotencyKey string) string {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return fmt.Sprintf("order:%s:pay", orderID.String())
	}
	return fmt.Sprintf("order:%s:pay:%s", orderID.String(), idempotencyKey)
}

func normalizeCurrency(currencyCode string) string {
	currencyCode = strings.TrimSpace(strings.ToUpper(currencyCode))
	if currencyCode == "" {
		return "COIN"
	}
	return currencyCode
}

func newOrderModel(order *orderdomain.Order) *orderModel {
	idempotencyKey := strings.TrimSpace(order.IdempotencyKey)
	var idempotencyKeyPtr *string
	if idempotencyKey != "" {
		idempotencyKeyPtr = &idempotencyKey
	}
	return &orderModel{
		OrderID:        order.OrderID,
		UserID:         order.UserID,
		AppID:          order.AppID,
		WalletID:       order.WalletID,
		Amount:         order.Amount,
		CurrencyCode:   order.CurrencyCode,
		Status:         string(order.Status),
		TxID:           order.TxID,
		SubscriptionID: order.SubscriptionID,
		IdempotencyKey: idempotencyKeyPtr,
		Description:    order.Description,
		CreatedAt:      order.CreatedAt,
		PaidAt:         order.PaidAt,
		UpdatedAt:      order.UpdatedAt,
	}
}

func (m orderModel) toDomain() *orderdomain.Order {
	idempotencyKey := ""
	if m.IdempotencyKey != nil {
		idempotencyKey = *m.IdempotencyKey
	}
	return &orderdomain.Order{
		OrderID:        m.OrderID,
		UserID:         m.UserID,
		AppID:          m.AppID,
		WalletID:       m.WalletID,
		Amount:         m.Amount,
		CurrencyCode:   m.CurrencyCode,
		Status:         orderdomain.OrderStatus(m.Status),
		TxID:           m.TxID,
		SubscriptionID: m.SubscriptionID,
		IdempotencyKey: idempotencyKey,
		Description:    m.Description,
		CreatedAt:      m.CreatedAt,
		PaidAt:         m.PaidAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
