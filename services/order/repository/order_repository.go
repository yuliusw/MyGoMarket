package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	redislock "github.com/yuliusw/RPA-market/common/utils/lock"
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

type entitlementOutboxModel struct {
	EventID    uuid.UUID  `gorm:"column:event_id;type:uuid;primaryKey"`
	OrderID    uuid.UUID  `gorm:"column:order_id;type:uuid"`
	Status     string     `gorm:"column:status"`
	RetryCount int        `gorm:"column:retry_count"`
	NextRunAt  time.Time  `gorm:"column:next_run_at"`
	LastError  string     `gorm:"column:last_error"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
	LockedAt   *time.Time `gorm:"column:locked_at"`
}

var (
	outboxWorkerCancel context.CancelFunc
	outboxWorkerOnce   sync.Once
	pendingCancelStop  context.CancelFunc
	pendingCancelOnce  sync.Once
)

func (orderModel) TableName() string {
	return "orders"
}

func (appModel) TableName() string {
	return "apps"
}

func (subscriptionModel) TableName() string {
	return "subscriptions"
}

func (entitlementOutboxModel) TableName() string {
	return "entitlement_outbox"
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
				if err := r.enqueueEntitlement(ctx, tx, locked.OrderID); err != nil {
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
		if err := locked.MarkPaidAwaitingEntitlement(walletTx.TxID); err != nil {
			return err
		}
		if err := r.enqueueEntitlement(ctx, tx, locked.OrderID); err != nil {
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

func (r *OrderRepository) enqueueEntitlement(ctx context.Context, tx *gorm.DB, orderID uuid.UUID) error {
	now := time.Now()
	return tx.WithContext(ctx).Exec(`
		INSERT INTO entitlement_outbox (event_id, order_id, status, retry_count, next_run_at, created_at, updated_at)
		VALUES (?, ?, 'pending', 0, ?, ?, ?)
		ON CONFLICT (order_id) DO NOTHING
	`, uuid.New(), orderID, now, now, now).Error
}

func StartEntitlementOutboxWorker(db *gorm.DB) {
	if db == nil {
		return
	}
	outboxWorkerOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		outboxWorkerCancel = cancel
		worker := NewOrderRepository(db, walletrepo.NewWalletRepository(db))
		go worker.runEntitlementOutboxWorker(ctx)
	})
}

func StopEntitlementOutboxWorker() {
	if outboxWorkerCancel != nil {
		outboxWorkerCancel()
	}
}

func StartPendingOrderCancelWorker(db *gorm.DB, timeout, scanInterval time.Duration) {
	if db == nil || timeout <= 0 {
		return
	}
	if scanInterval <= 0 {
		scanInterval = time.Minute
	}
	pendingCancelOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		pendingCancelStop = cancel
		worker := NewOrderRepository(db, walletrepo.NewWalletRepository(db))
		go worker.runPendingOrderCancelWorker(ctx, timeout, scanInterval)
	})
}

func StopPendingOrderCancelWorker() {
	if pendingCancelStop != nil {
		pendingCancelStop()
	}
}

func (r *OrderRepository) runPendingOrderCancelWorker(ctx context.Context, timeout, scanInterval time.Duration) {
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.cancelExpiredPendingOrders(ctx, timeout, 100)
		}
	}
}

func (r *OrderRepository) cancelExpiredPendingOrders(ctx context.Context, timeout time.Duration, limit int) error {
	cutoff := time.Now().Add(-timeout)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []orderModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND created_at < ?", string(orderdomain.OrderStatusPending), cutoff).
			Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			order := row.toDomain()
			if err := order.Cancel(); err != nil {
				continue
			}
			if err := updateOrder(ctx, tx, order); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *OrderRepository) runEntitlementOutboxWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.processEntitlementOutboxBatch(ctx, 20)
		}
	}
}

func (r *OrderRepository) processEntitlementOutboxBatch(ctx context.Context, limit int) error {
	var events []entitlementOutboxModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_run_at <= ?", []string{"pending", "failed"}, time.Now()).
			Order("next_run_at ASC").Limit(limit).Find(&events).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, event := range events {
			if err := tx.Model(&entitlementOutboxModel{}).Where("event_id = ?", event.EventID).Updates(map[string]any{"status": "processing", "locked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := r.processEntitlementEvent(ctx, event); err != nil {
			_ = r.markEntitlementEventFailed(context.Background(), event, err)
		}
	}
	return nil
}

func (r *OrderRepository) processEntitlementEvent(ctx context.Context, event entitlementOutboxModel) error {
	lock := redislock.NewRedisLock("entitlement:order:"+event.OrderID.String(), 30*time.Second)
	if lock != nil {
		locked, err := lock.Lock(ctx)
		if err != nil {
			return err
		}
		if !locked {
			return fmt.Errorf("entitlement event already locked: %s", event.OrderID)
		}
		defer lock.Unlock(context.Background())
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := r.lockOrder(ctx, tx, event.OrderID)
		if err != nil {
			return err
		}
		if order.Status != orderdomain.OrderStatusPaid {
			return fmt.Errorf("order is not paid: %s", event.OrderID)
		}
		subID, err := r.grantSubscription(ctx, tx, order)
		if err != nil {
			return err
		}
		order.MarkSubscription(subID)
		if err := updateOrder(ctx, tx, order); err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&entitlementOutboxModel{}).Where("event_id = ?", event.EventID).Updates(map[string]any{"status": "done", "last_error": "", "updated_at": now}).Error
	})
}

func (r *OrderRepository) markEntitlementEventFailed(ctx context.Context, event entitlementOutboxModel, err error) error {
	retryCount := event.RetryCount + 1
	nextRunAt := time.Now().Add(time.Duration(retryCount*retryCount) * time.Second)
	return r.db.WithContext(ctx).Model(&entitlementOutboxModel{}).Where("event_id = ?", event.EventID).Updates(map[string]any{
		"status":      "failed",
		"retry_count": retryCount,
		"next_run_at": nextRunAt,
		"last_error":  err.Error(),
		"updated_at":  time.Now(),
		"locked_at":   nil,
	}).Error
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
