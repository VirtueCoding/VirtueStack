package customer

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// GetBillingBalance handles GET /customer/billing/balance.
func (h *CustomerHandler) GetBillingBalance(c *gin.Context) {
	customerID := middleware.GetUserID(c)

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
		"balance":  balance,
		"currency": "USD",
	}})
}

// ListBillingTransactions handles GET /customer/billing/transactions.
func (h *CustomerHandler) ListBillingTransactions(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	pagination := models.ParsePagination(c)

	txs, hasMore, lastID, err := h.billingLedgerService.GetTransactionHistory(
		c.Request.Context(), customerID, pagination,
	)
	if err != nil {
		h.logger.Error("failed to list billing transactions",
			"customer_id", customerID,
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

// GetBillingUsage handles GET /customer/billing/usage.
func (h *CustomerHandler) GetBillingUsage(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	balance, err := h.billingLedgerService.GetBalance(
		c.Request.Context(), customerID,
	)
	if err != nil {
		h.logger.Error("failed to get billing usage",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"BILLING_USAGE_FAILED", "Failed to retrieve billing usage")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: map[string]any{
		"balance":  balance,
		"currency": "USD",
	}})
}

// InitiateTopUp handles POST /customer/billing/top-up.
func (h *CustomerHandler) InitiateTopUp(c *gin.Context) {
	var req models.TopUpRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		return
	}

	customerID := middleware.GetUserID(c)
	email := h.getCustomerEmail(c, customerID)

	config, err := h.paymentService.GetTopUpConfig(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get top-up config",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TOPUP_CONFIG_FAILED", "Failed to retrieve top-up configuration")
		return
	}

	if err := validateTopUpAmount(req.Amount, config); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_AMOUNT", err.Error())
		return
	}

	sess, paymentID, topUpErr := h.paymentService.InitiateTopUp(
		c.Request.Context(),
		customerID, email, req.Amount, req.Currency,
		req.Gateway, req.ReturnURL, req.CancelURL,
	)
	if topUpErr != nil {
		h.logger.Error("failed to initiate top-up",
			"customer_id", customerID,
			"error", topUpErr,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TOPUP_FAILED", "Failed to initiate payment")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: models.TopUpResponse{
		PaymentID:  paymentID,
		PaymentURL: sess.PaymentURL,
	}})
}

func validateTopUpAmount(amount int64, config *services.TopUpConfig) error {
	if amount < config.MinAmountCents || amount > config.MaxAmountCents {
		return fmt.Errorf(
			"amount must be between %d and %d cents",
			config.MinAmountCents, config.MaxAmountCents,
		)
	}
	return nil
}

func (h *CustomerHandler) getCustomerEmail(c *gin.Context, customerID string) string {
	cust, err := h.customerRepo.GetByID(c.Request.Context(), customerID)
	if err != nil {
		return ""
	}
	return cust.Email
}

// GetPaymentHistory handles GET /customer/billing/payments.
func (h *CustomerHandler) GetPaymentHistory(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	pagination := models.ParsePagination(c)

	pymnts, hasMore, lastID, err := h.paymentService.GetPaymentHistory(
		c.Request.Context(), customerID, pagination,
	)
	if err != nil {
		h.logger.Error("failed to list payments",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PAYMENT_LIST_FAILED", "Failed to list payments")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: pymnts,
		Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
	})
}

// GetTopUpConfig handles GET /customer/billing/top-up/config.
func (h *CustomerHandler) GetTopUpConfig(c *gin.Context) {
	config, err := h.paymentService.GetTopUpConfig(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get top-up config",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TOPUP_CONFIG_FAILED", "Failed to retrieve top-up configuration")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: config})
}
