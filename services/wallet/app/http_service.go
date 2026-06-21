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
	"github.com/yuliusw/RPA-market/services/wallet/domain"
	"github.com/yuliusw/RPA-market/services/wallet/repository"
)

type WalletHTTPService struct {
	repo *repository.WalletRepository
}

type walletChangeRequest struct {
	Amount         string `json:"amount" binding:"required"`
	ReferenceID    string `json:"reference_id"`
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotency_key"`
}

type walletTransferRequest struct {
	FromWalletID   string `json:"from_wallet_id" binding:"required"`
	ToWalletID     string `json:"to_wallet_id" binding:"required"`
	Amount         string `json:"amount" binding:"required"`
	ReferenceID    string `json:"reference_id"`
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotency_key"`
}

func NewWalletHTTPService(repo *repository.WalletRepository) *WalletHTTPService {
	return &WalletHTTPService{repo: repo}
}

func (s *WalletHTTPService) GetMyWallet(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	currencyCode := c.DefaultQuery("currency_code", "COIN")
	wallet, err := s.repo.GetOrCreateByOwner(c.Request.Context(), userID, domain.OwnerTypeUser, currencyCode)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "WALLET_GET_FAILED", "failed to get wallet")
		return
	}
	c.JSON(http.StatusOK, toHTTPWallet(wallet))
}

func (s *WalletHTTPService) ListTransactions(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	walletID, ok := parseWalletIDParam(c)
	if !ok {
		return
	}
	if !s.ensureOwnedWallet(c, walletID, userID) {
		return
	}

	page, pageSize := pagination(c)
	txs, err := s.repo.ListTransactions(c.Request.Context(), walletID, pageSize, (page-1)*pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "WALLET_TX_LIST_FAILED", "failed to list wallet transactions")
		return
	}

	items := make([]gin.H, 0, len(txs))
	for _, tx := range txs {
		items = append(items, toHTTPTransaction(tx))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "page_size": pageSize})
}

func (s *WalletHTTPService) Credit(c *gin.Context) {
	s.changeBalance(c, true)
}

func (s *WalletHTTPService) Debit(c *gin.Context) {
	s.changeBalance(c, false)
}

func (s *WalletHTTPService) Transfer(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req walletTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	fromWalletID, err := uuid.Parse(strings.TrimSpace(req.FromWalletID))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_WALLET_ID", "invalid from_wallet_id")
		return
	}
	toWalletID, err := uuid.Parse(strings.TrimSpace(req.ToWalletID))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_WALLET_ID", "invalid to_wallet_id")
		return
	}
	amount, ok := parsePositiveAmount(c, req.Amount)
	if !ok {
		return
	}
	referenceID, ok := parseOptionalUUID(c, req.ReferenceID, "reference_id")
	if !ok {
		return
	}
	if !s.ensureOwnedWallet(c, fromWalletID, userID) {
		return
	}

	outTx, inTx, err := s.repo.Transfer(c.Request.Context(), fromWalletID, toWalletID, amount, referenceID, req.Description, strings.TrimSpace(req.IdempotencyKey))
	if err != nil {
		s.writeWalletError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"out": toHTTPTransaction(outTx), "in": toHTTPTransaction(inTx)})
}

func (s *WalletHTTPService) changeBalance(c *gin.Context, credit bool) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	walletID, ok := parseWalletIDParam(c)
	if !ok {
		return
	}
	if !s.ensureOwnedWallet(c, walletID, userID) {
		return
	}

	var req walletChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	amount, ok := parsePositiveAmount(c, req.Amount)
	if !ok {
		return
	}
	referenceID, ok := parseOptionalUUID(c, req.ReferenceID, "reference_id")
	if !ok {
		return
	}

	var wallet *domain.Wallet
	var tx *domain.Transaction
	var err error
	if credit {
		wallet, tx, err = s.repo.Credit(c.Request.Context(), walletID, amount, referenceID, req.Description, strings.TrimSpace(req.IdempotencyKey))
	} else {
		wallet, tx, err = s.repo.Debit(c.Request.Context(), walletID, amount, referenceID, req.Description, strings.TrimSpace(req.IdempotencyKey))
	}
	if err != nil {
		s.writeWalletError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"wallet": toHTTPWallet(wallet), "transaction": toHTTPTransaction(tx)})
}

func (s *WalletHTTPService) ensureOwnedWallet(c *gin.Context, walletID, userID uuid.UUID) bool {
	wallet, err := s.repo.FindByID(c.Request.Context(), walletID)
	if err != nil {
		if repository.IsNotFound(err) {
			response.Error(c, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
			return false
		}
		response.Error(c, http.StatusInternalServerError, "WALLET_GET_FAILED", "failed to get wallet")
		return false
	}
	if wallet.OwnerType != domain.OwnerTypeUser || wallet.OwnerID != userID {
		response.Error(c, http.StatusForbidden, "WALLET_FORBIDDEN", "wallet is not owned by current user")
		return false
	}
	return true
}

func (s *WalletHTTPService) writeWalletError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidAmount), errors.Is(err, domain.ErrInvalidTransfer):
		response.Error(c, http.StatusBadRequest, "INVALID_WALLET_OPERATION", err.Error())
	case errors.Is(err, domain.ErrInsufficientBalance):
		response.Error(c, http.StatusConflict, "INSUFFICIENT_BALANCE", "insufficient balance")
	case errors.Is(err, domain.ErrWalletInactive):
		response.Error(c, http.StatusConflict, "WALLET_INACTIVE", "wallet is inactive")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		response.Error(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key already used for another operation")
	case repository.IsNotFound(err):
		response.Error(c, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
	default:
		response.Error(c, http.StatusInternalServerError, "WALLET_OPERATION_FAILED", "wallet operation failed")
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

func parseWalletIDParam(c *gin.Context) (uuid.UUID, bool) {
	walletID, err := uuid.Parse(strings.TrimSpace(c.Param("wallet_id")))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_WALLET_ID", "invalid wallet_id")
		return uuid.Nil, false
	}
	return walletID, true
}

func parsePositiveAmount(c *gin.Context, value string) (decimal.Decimal, bool) {
	amount, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		response.Error(c, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be positive")
		return decimal.Zero, false
	}
	return amount, true
}

func parseOptionalUUID(c *gin.Context, value, field string) (*uuid.UUID, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true
	}
	id, err := uuid.Parse(value)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_"+strings.ToUpper(field), "invalid "+field)
		return nil, false
	}
	return &id, true
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

func toHTTPWallet(wallet *domain.Wallet) gin.H {
	return gin.H{
		"wallet_id":     wallet.ID.String(),
		"owner_id":      wallet.OwnerID.String(),
		"owner_type":    string(wallet.OwnerType),
		"balance":       wallet.Balance.StringFixed(4),
		"currency_code": wallet.CurrencyCode,
		"status":        string(wallet.Status),
		"updated_at":    wallet.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toHTTPTransaction(tx *domain.Transaction) gin.H {
	if tx == nil {
		return nil
	}
	referenceID := ""
	if tx.ReferenceID != nil {
		referenceID = tx.ReferenceID.String()
	}
	return gin.H{
		"tx_id":           tx.TxID.String(),
		"wallet_id":       tx.WalletID.String(),
		"tx_type":         string(tx.TxType),
		"amount":          tx.Amount.StringFixed(4),
		"balance_after":   tx.BalanceAfter.StringFixed(4),
		"reference_id":    referenceID,
		"idempotency_key": tx.IdempotencyKey,
		"description":     tx.Description,
		"created_at":      tx.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}
