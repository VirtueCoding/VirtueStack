package webhooks

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments/crypto"
	"github.com/gin-gonic/gin"
)

// BillingCreditor credits a billing account from a payment.
type BillingCreditor interface {
	CreditFromPayment(
		ctx context.Context,
		accountID string,
		amountCents int64,
		currency string,
		gateway string,
		gatewayPaymentID string,
		idempotencyKey string,
	) error
}

// CryptoWebhookHandler handles webhook events from crypto payment providers.
type CryptoWebhookHandler struct {
	provider crypto.CryptoProvider
	creditor BillingCreditor
	logger   *slog.Logger
}

// NewCryptoWebhookHandler creates a new crypto webhook handler.
func NewCryptoWebhookHandler(
	provider crypto.CryptoProvider,
	creditor BillingCreditor,
	logger *slog.Logger,
) *CryptoWebhookHandler {
	return &CryptoWebhookHandler{
		provider: provider,
		creditor: creditor,
		logger: logger.With(
			"component", "crypto-webhook-handler",
			"provider", provider.ProviderName(),
		),
	}
}

// HandleWebhook processes inbound crypto payment webhook POST requests.
// This endpoint is unauthenticated — HMAC signature verification is
// performed by the provider implementation.
func (h *CryptoWebhookHandler) HandleWebhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	result, err := h.provider.HandleWebhook(
		c.Request.Context(),
		c.Request.Header,
		body,
	)
	if err != nil {
		h.logger.Error("crypto webhook processing failed", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	if result == nil {
		c.Status(http.StatusOK)
		return
	}

	if err := h.creditor.CreditFromPayment(
		c.Request.Context(),
		result.AccountID,
		result.AmountCents,
		result.Currency,
		"crypto",
		result.InvoiceID,
		result.IdempotencyKey,
	); err != nil {
		h.logger.Error("failed to credit account from crypto payment",
			"error", err,
			"invoice_id", result.InvoiceID,
			"account_id", result.AccountID,
		)
		c.Status(http.StatusInternalServerError)
		return
	}

	h.logger.Info("crypto payment credited",
		"invoice_id", result.InvoiceID,
		"account_id", result.AccountID,
		"amount_cents", result.AmountCents,
	)
	c.Status(http.StatusOK)
}
