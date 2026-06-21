package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/common/response"
	orderdomain "github.com/yuliusw/RPA-market/services/order/domain"
	"github.com/yuliusw/RPA-market/services/order/repository"
	walletdomain "github.com/yuliusw/RPA-market/services/wallet/domain"
	walletrepo "github.com/yuliusw/RPA-market/services/wallet/repository"
)

type OrderHTTPService struct {
	repo *repository.OrderRepository
}

type createOrderRequest struct {
	AppID          string `json:"app_id" binding:"required"`
	Amount         string `json:"amount" binding:"required"`
	CurrencyCode   string `json:"currency_code"`
	IdempotencyKey string `json:"idempotency_key"`
	Description    string `json:"description"`
}

type payOrderRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

func NewOrderHTTPService(repo *repository.OrderRepository) *OrderHTTPService {
	return &OrderHTTPService{repo: repo}
}

func (s *OrderHTTPService) CreateOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	appID, err := uuid.Parse(strings.TrimSpace(req.AppID))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_APP_ID", "invalid app_id")
		return
	}
	amount, ok := parsePositiveAmount(c, req.Amount)
	if !ok {
		return
	}

	order, err := s.repo.Create(c.Request.Context(), userID, appID, amount, req.CurrencyCode, req.IdempotencyKey, req.Description)
	if err != nil {
		s.writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toHTTPOrder(order))
}

func (s *OrderHTTPService) PurchaseApp(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	appID, err := uuid.Parse(strings.TrimSpace(req.AppID))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_APP_ID", "invalid app_id")
		return
	}
	amount, ok := parsePositiveAmount(c, req.Amount)
	if !ok {
		return
	}

	order, err := s.repo.Purchase(c.Request.Context(), userID, appID, amount, req.CurrencyCode, req.IdempotencyKey, req.Description)
	if err != nil {
		s.writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusOK, toHTTPOrder(order))
}

func (s *OrderHTTPService) ListOrders(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	page, pageSize := pagination(c)
	orders, err := s.repo.ListByUser(c.Request.Context(), userID, pageSize, (page-1)*pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "ORDER_LIST_FAILED", "failed to list orders")
		return
	}

	items := make([]gin.H, 0, len(orders))
	for _, order := range orders {
		items = append(items, toHTTPOrder(order))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "page_size": pageSize})
}

func (s *OrderHTTPService) GetOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orderID, ok := parseOrderID(c)
	if !ok {
		return
	}
	order, err := s.repo.FindByID(c.Request.Context(), orderID)
	if err != nil {
		s.writeOrderError(c, err)
		return
	}
	if order.UserID != userID {
		response.Error(c, http.StatusForbidden, "ORDER_FORBIDDEN", "order is not owned by current user")
		return
	}
	c.JSON(http.StatusOK, toHTTPOrder(order))
}

func (s *OrderHTTPService) PayOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orderID, ok := parseOrderID(c)
	if !ok {
		return
	}
	var req payOrderRequest
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}

	order, err := s.repo.Pay(c.Request.Context(), orderID, userID, req.IdempotencyKey)
	if err != nil {
		s.writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusOK, toHTTPOrder(order))
}

func (s *OrderHTTPService) CancelOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orderID, ok := parseOrderID(c)
	if !ok {
		return
	}

	order, err := s.repo.Cancel(c.Request.Context(), orderID, userID)
	if err != nil {
		s.writeOrderError(c, err)
		return
	}
	c.JSON(http.StatusOK, toHTTPOrder(order))
}

func (s *OrderHTTPService) writeOrderError(c *gin.Context, err error) {
	switch {
	case repository.IsNotFound(err):
		response.Error(c, http.StatusNotFound, "ORDER_NOT_FOUND", "order not found")
	case errors.Is(err, orderdomain.ErrAppNotFound):
		response.Error(c, http.StatusNotFound, "APP_NOT_FOUND", "app not found")
	case errors.Is(err, orderdomain.ErrInvalidOrderAmount):
		response.Error(c, http.StatusBadRequest, "INVALID_ORDER_AMOUNT", "amount must be positive")
	case errors.Is(err, orderdomain.ErrOrderForbidden):
		response.Error(c, http.StatusForbidden, "ORDER_FORBIDDEN", "order is not owned by current user")
	case errors.Is(err, orderdomain.ErrOrderAlreadyPaid):
		response.Error(c, http.StatusConflict, "ORDER_ALREADY_PAID", "order already paid")
	case errors.Is(err, orderdomain.ErrOrderNotPayable):
		response.Error(c, http.StatusConflict, "ORDER_NOT_PAYABLE", "order is not payable")
	case errors.Is(err, orderdomain.ErrOrderNotCancellable):
		response.Error(c, http.StatusConflict, "ORDER_NOT_CANCELLABLE", "order is not cancellable")
	case errors.Is(err, orderdomain.ErrOrderIdempotency), errors.Is(err, walletdomain.ErrIdempotencyConflict):
		response.Error(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key already used for another operation")
	case errors.Is(err, walletdomain.ErrInsufficientBalance):
		response.Error(c, http.StatusConflict, "INSUFFICIENT_BALANCE", "insufficient balance")
	case errors.Is(err, walletdomain.ErrWalletInactive):
		response.Error(c, http.StatusConflict, "WALLET_INACTIVE", "wallet is inactive")
	case errors.Is(err, walletdomain.ErrInvalidAmount):
		response.Error(c, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be positive")
	case walletrepo.IsNotFound(err):
		response.Error(c, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
	default:
		response.Error(c, http.StatusInternalServerError, "ORDER_OPERATION_FAILED", "order operation failed")
	}
}

func currentUserID(c *gin.Context) (uuid.UUID, bool) {
	userID, err := uuid.Parse(strings.TrimSpace(middleware.GetUserID(c)))
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid current user")
		return uuid.Nil, false
	}
	return userID, true
}

func parseOrderID(c *gin.Context) (uuid.UUID, bool) {
	orderID, err := uuid.Parse(strings.TrimSpace(c.Param("order_id")))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ORDER_ID", "invalid order_id")
		return uuid.Nil, false
	}
	return orderID, true
}

func parsePositiveAmount(c *gin.Context, value string) (decimal.Decimal, bool) {
	amount, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		response.Error(c, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be positive")
		return decimal.Zero, false
	}
	return amount, true
}

func pagination(c *gin.Context) (int, int) {
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
	return page, pageSize
}

func toHTTPOrder(order *orderdomain.Order) gin.H {
	paidAt := ""
	if order.PaidAt != nil {
		paidAt = order.PaidAt.UTC().Format(time.RFC3339Nano)
	}
	txID := ""
	if order.TxID != nil {
		txID = order.TxID.String()
	}
	subscriptionID := ""
	if order.SubscriptionID != nil {
		subscriptionID = order.SubscriptionID.String()
	}
	return gin.H{
		"order_id":        order.OrderID.String(),
		"user_id":         order.UserID.String(),
		"app_id":          order.AppID.String(),
		"wallet_id":       order.WalletID.String(),
		"amount":          order.Amount.StringFixed(4),
		"currency_code":   order.CurrencyCode,
		"status":          string(order.Status),
		"tx_id":           txID,
		"subscription_id": subscriptionID,
		"idempotency_key": order.IdempotencyKey,
		"description":     order.Description,
		"created_at":      order.CreatedAt.UTC().Format(time.RFC3339Nano),
		"paid_at":         paidAt,
		"updated_at":      order.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}
