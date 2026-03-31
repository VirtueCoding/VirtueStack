package customer

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
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
