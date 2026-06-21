package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/yuliusw/RPA-market/common/response"
	"gorm.io/gorm"
)

type AdminHTTPService struct {
	db *gorm.DB
}

type walletModel struct {
	WalletID     uuid.UUID       `gorm:"column:wallet_id;type:uuid;primaryKey"`
	OwnerID      uuid.UUID       `gorm:"column:owner_id;type:uuid"`
	OwnerType    string          `gorm:"column:owner_type"`
	Balance      decimal.Decimal `gorm:"column:balance"`
	CurrencyCode string          `gorm:"column:currency_code"`
	Status       string          `gorm:"column:status"`
	UpdatedAt    time.Time       `gorm:"column:updated_at"`
}

type transactionModel struct {
	TxID           uuid.UUID       `gorm:"column:tx_id;type:uuid;primaryKey"`
	WalletID       uuid.UUID       `gorm:"column:wallet_id;type:uuid"`
	TxType         string          `gorm:"column:tx_type"`
	Amount         decimal.Decimal `gorm:"column:amount"`
	BalanceAfter   decimal.Decimal `gorm:"column:balance_after"`
	ReferenceID    *uuid.UUID      `gorm:"column:reference_id;type:uuid"`
	IdempotencyKey *string         `gorm:"column:idempotency_key"`
	Description    string          `gorm:"column:description"`
	CreatedAt      time.Time       `gorm:"column:created_at"`
}

type orderModel struct {
	OrderID        uuid.UUID       `gorm:"column:order_id;type:uuid;primaryKey"`
	UserID         uuid.UUID       `gorm:"column:user_id;type:uuid"`
	AppID          uuid.UUID       `gorm:"column:app_id;type:uuid"`
	WalletID       uuid.UUID       `gorm:"column:wallet_id;type:uuid"`
	Amount         decimal.Decimal `gorm:"column:amount"`
	CurrencyCode   string          `gorm:"column:currency_code"`
	Status         string          `gorm:"column:status"`
	TxID           *uuid.UUID      `gorm:"column:tx_id;type:uuid"`
	SubscriptionID *uuid.UUID      `gorm:"column:subscription_id;type:uuid"`
	IdempotencyKey *string         `gorm:"column:idempotency_key"`
	Description    string          `gorm:"column:description"`
	CreatedAt      time.Time       `gorm:"column:created_at"`
	PaidAt         *time.Time      `gorm:"column:paid_at"`
	UpdatedAt      time.Time       `gorm:"column:updated_at"`
}

type auditEventModel struct {
	EventID   uuid.UUID `gorm:"column:event_id;type:uuid;primaryKey"`
	EventType string    `gorm:"column:event_type"`
	TraceID   string    `gorm:"column:trace_id"`
	ActorID   string    `gorm:"column:actor_id"`
	Resource  string    `gorm:"column:resource"`
	Metadata  []byte    `gorm:"column:metadata;type:jsonb"`
	Error     string    `gorm:"column:error"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (walletModel) TableName() string      { return "wallets" }
func (transactionModel) TableName() string { return "wallet_transactions" }
func (orderModel) TableName() string       { return "orders" }
func (auditEventModel) TableName() string  { return "audit_events" }

func NewAdminHTTPService(db *gorm.DB) *AdminHTTPService {
	return &AdminHTTPService{db: db}
}

func (s *AdminHTTPService) ListVirtualOrders(c *gin.Context) {
	page, pageSize, offset := pagination(c)
	query := s.db.WithContext(c.Request.Context()).Model(&walletModel{})
	query = whereUUID(c, query, "owner_id", "owner_id")
	query = whereEqual(c, query, "owner_type", "owner_type")
	query = whereEqual(c, query, "currency_code", "currency_code")
	query = whereEqual(c, query, "status", "status")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_WALLET_COUNT_FAILED", "failed to count wallets")
		return
	}

	var rows []walletModel
	if err := query.Order("updated_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_WALLET_LIST_FAILED", "failed to list wallets")
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, walletToJSON(row))
	}
	c.JSON(http.StatusOK, listResponse(items, total, page, pageSize))
}

func (s *AdminHTTPService) ListWalletTransactions(c *gin.Context) {
	page, pageSize, offset := pagination(c)
	query := s.db.WithContext(c.Request.Context()).Model(&transactionModel{})
	query = whereUUID(c, query, "wallet_id", "wallet_id")
	query = whereUUID(c, query, "reference_id", "reference_id")
	query = whereEqual(c, query, "tx_type", "tx_type")
	query = whereTimeRange(c, query, "created_at")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_TX_COUNT_FAILED", "failed to count wallet transactions")
		return
	}

	var rows []transactionModel
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_TX_LIST_FAILED", "failed to list wallet transactions")
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, transactionToJSON(row))
	}
	c.JSON(http.StatusOK, listResponse(items, total, page, pageSize))
}

func (s *AdminHTTPService) ListOrders(c *gin.Context) {
	page, pageSize, offset := pagination(c)
	query := s.db.WithContext(c.Request.Context()).Model(&orderModel{})
	query = whereUUID(c, query, "user_id", "user_id")
	query = whereUUID(c, query, "app_id", "app_id")
	query = whereUUID(c, query, "wallet_id", "wallet_id")
	query = whereEqual(c, query, "status", "status")
	query = whereEqual(c, query, "currency_code", "currency_code")
	query = whereTimeRange(c, query, "created_at")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_ORDER_COUNT_FAILED", "failed to count orders")
		return
	}

	var rows []orderModel
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_ORDER_LIST_FAILED", "failed to list orders")
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, orderToJSON(row))
	}
	c.JSON(http.StatusOK, listResponse(items, total, page, pageSize))
}

func (s *AdminHTTPService) ListChangeLogs(c *gin.Context) {
	page, pageSize, offset := pagination(c)
	query := s.db.WithContext(c.Request.Context()).Model(&auditEventModel{})
	query = whereEqual(c, query, "event_type", "event_type")
	query = whereEqual(c, query, "trace_id", "trace_id")
	query = whereEqual(c, query, "actor_id", "actor_id")
	query = whereEqual(c, query, "resource", "resource")
	query = whereTimeRange(c, query, "created_at")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_CHANGE_LOG_COUNT_FAILED", "failed to count change logs")
		return
	}

	var rows []auditEventModel
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.Error(c, http.StatusInternalServerError, "ADMIN_CHANGE_LOG_LIST_FAILED", "failed to list change logs")
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, auditEventToJSON(row))
	}
	c.JSON(http.StatusOK, listResponse(items, total, page, pageSize))
}

func whereEqual(c *gin.Context, query *gorm.DB, queryKey, column string) *gorm.DB {
	value := strings.TrimSpace(c.Query(queryKey))
	if value == "" {
		return query
	}
	return query.Where(column+" = ?", value)
}

func whereUUID(c *gin.Context, query *gorm.DB, queryKey, column string) *gorm.DB {
	value := strings.TrimSpace(c.Query(queryKey))
	if value == "" {
		return query
	}
	if _, err := uuid.Parse(value); err != nil {
		return query.Where("1 = 0")
	}
	return query.Where(column+" = ?", value)
}

func whereTimeRange(c *gin.Context, query *gorm.DB, column string) *gorm.DB {
	if from, ok := parseTimeQuery(c, "from"); ok {
		query = query.Where(column+" >= ?", from)
	}
	if to, ok := parseTimeQuery(c, "to"); ok {
		query = query.Where(column+" <= ?", to)
	}
	return query
}

func parseTimeQuery(c *gin.Context, key string) (time.Time, bool) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func pagination(c *gin.Context) (int, int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

func listResponse(items []gin.H, total int64, page, pageSize int) gin.H {
	return gin.H{"items": items, "total": total, "page": page, "page_size": pageSize}
}

func walletToJSON(row walletModel) gin.H {
	return gin.H{
		"wallet_id":     row.WalletID.String(),
		"owner_id":      row.OwnerID.String(),
		"owner_type":    row.OwnerType,
		"balance":       row.Balance.StringFixed(4),
		"currency_code": row.CurrencyCode,
		"status":        row.Status,
		"updated_at":    formatTime(row.UpdatedAt),
	}
}

func transactionToJSON(row transactionModel) gin.H {
	return gin.H{
		"tx_id":           row.TxID.String(),
		"wallet_id":       row.WalletID.String(),
		"tx_type":         row.TxType,
		"amount":          row.Amount.StringFixed(4),
		"balance_after":   row.BalanceAfter.StringFixed(4),
		"reference_id":    uuidPtrString(row.ReferenceID),
		"idempotency_key": stringPtrValue(row.IdempotencyKey),
		"description":     row.Description,
		"created_at":      formatTime(row.CreatedAt),
	}
}

func orderToJSON(row orderModel) gin.H {
	return gin.H{
		"order_id":        row.OrderID.String(),
		"user_id":         row.UserID.String(),
		"app_id":          row.AppID.String(),
		"wallet_id":       row.WalletID.String(),
		"amount":          row.Amount.StringFixed(4),
		"currency_code":   row.CurrencyCode,
		"status":          row.Status,
		"tx_id":           uuidPtrString(row.TxID),
		"subscription_id": uuidPtrString(row.SubscriptionID),
		"idempotency_key": stringPtrValue(row.IdempotencyKey),
		"description":     row.Description,
		"created_at":      formatTime(row.CreatedAt),
		"paid_at":         timePtrString(row.PaidAt),
		"updated_at":      formatTime(row.UpdatedAt),
	}
}

func auditEventToJSON(row auditEventModel) gin.H {
	return gin.H{
		"event_id":   row.EventID.String(),
		"event_type": row.EventType,
		"trace_id":   row.TraceID,
		"actor_id":   row.ActorID,
		"resource":   row.Resource,
		"metadata":   string(row.Metadata),
		"error":      row.Error,
		"created_at": formatTime(row.CreatedAt),
	}
}

func uuidPtrString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timePtrString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
