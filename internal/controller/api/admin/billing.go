package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// ListBillingTransactions handles GET /admin/billing/transactions.
func (h *AdminHandler) ListBillingTransactions(c *gin.Context) {
	pagination := models.ParsePagination(c)

	customerID := c.Query("customer_id")
	if customerID != "" {
		if _, err := uuid.Parse(customerID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"INVALID_CUSTOMER_ID", "customer_id must be a valid UUID")
			return
		}
	}

	txs, hasMore, lastID, err := h.billingLedgerService.GetTransactionHistory(
		c.Request.Context(), customerID, pagination,
	)
	if err != nil {
		h.logger.Error("failed to list billing transactions",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"BILLING_TX_LIST_FAILED", "Failed to list billing transactions")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: txs,
		Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
	})
}

// AdminCreditAdjustment handles POST /admin/billing/credit.
func (h *AdminHandler) AdminCreditAdjustment(c *gin.Context) {
	var req models.AdminCreditAdjustmentRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		return
	}

	customerID := c.Query("customer_id")
	if _, err := uuid.Parse(customerID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_CUSTOMER_ID",
			"customer_id query parameter is required and must be a valid UUID")
		return
	}

	idempotencyKey := fmt.Sprintf("admin-adjustment:%s", uuid.New().String())

	var tx *models.BillingTransaction
	var txErr error
	if req.Amount > 0 {
		tx, txErr = h.billingLedgerService.CreditAccount(
			c.Request.Context(), customerID, req.Amount,
			req.Description, &idempotencyKey,
		)
	} else {
		absAmount := -req.Amount
		refType := models.BillingRefTypeAdminAdjust
		tx, txErr = h.billingLedgerService.DebitAccount(
			c.Request.Context(), customerID, absAmount,
			req.Description, &refType, nil, &idempotencyKey,
		)
	}
	if txErr != nil {
		h.logger.Error("failed to process credit adjustment",
			"customer_id", customerID,
			"amount", req.Amount,
			"error", txErr,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"BILLING_ADJUST_FAILED", "Failed to process credit adjustment")
		return
	}

	h.logAuditEvent(c, "billing.credit_adjustment", "customer", customerID,
		map[string]any{
			"amount":      req.Amount,
			"description": req.Description,
			"actor_id":    middleware.GetUserID(c),
		}, true)

	c.JSON(http.StatusOK, models.Response{Data: tx})
}

// GetCustomerBalance handles GET /admin/billing/balance.
func (h *AdminHandler) GetCustomerBalance(c *gin.Context) {
	customerID := c.Query("customer_id")
	if _, err := uuid.Parse(customerID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_CUSTOMER_ID",
			"customer_id query parameter is required and must be a valid UUID")
		return
	}

	balance, err := h.billingLedgerService.GetBalance(
		c.Request.Context(), customerID,
	)
	if err != nil {
		h.logger.Error("failed to get billing balance",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"BILLING_BALANCE_FAILED", "Failed to retrieve billing balance")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: map[string]any{
		"customer_id": customerID,
		"balance":     balance,
		"currency":    "USD",
	}})
}

// ListExchangeRates handles GET /admin/exchange-rates.
func (h *AdminHandler) ListExchangeRates(c *gin.Context) {
	rates, err := h.exchangeRateService.ListRates(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list exchange rates",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"EXCHANGE_RATE_LIST_FAILED", "Failed to list exchange rates")
		return
	}
	c.JSON(http.StatusOK, models.Response{Data: rates})
}

// UpdateExchangeRate handles PUT /admin/exchange-rates/:currency.
func (h *AdminHandler) UpdateExchangeRate(c *gin.Context) {
	currency := strings.ToUpper(c.Param("currency"))
	if len(currency) != 3 {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_CURRENCY", "Currency must be a 3-letter ISO 4217 code")
		return
	}

	var req models.UpdateExchangeRateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		return
	}

	if err := h.exchangeRateService.UpdateRate(
		c.Request.Context(), currency, req.RateToUSD,
	); err != nil {
		h.logger.Error("failed to update exchange rate",
			"currency", currency,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"EXCHANGE_RATE_UPDATE_FAILED", "Failed to update exchange rate")
		return
	}

	h.logAuditEvent(c, "exchange_rate.update", "exchange_rate", currency,
		map[string]any{"rate_to_usd": req.RateToUSD}, true)

	c.JSON(http.StatusOK, models.Response{Data: map[string]any{
		"currency":    currency,
		"rate_to_usd": req.RateToUSD,
	}})
}
