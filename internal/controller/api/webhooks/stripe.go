package webhooks

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// StripeWebhookHandler handles Stripe webhook callbacks.
type StripeWebhookHandler struct {
	paymentService *services.PaymentService
	logger         *slog.Logger
}

// NewStripeWebhookHandler creates a new StripeWebhookHandler.
func NewStripeWebhookHandler(
	paymentService *services.PaymentService,
	logger *slog.Logger,
) *StripeWebhookHandler {
	return &StripeWebhookHandler{
		paymentService: paymentService,
		logger:         logger.With("component", "stripe-webhook"),
	}
}

// Handle processes POST /api/v1/webhooks/stripe.
// This endpoint is unauthenticated — Stripe signature verification
// happens inside the payment provider.
func (h *StripeWebhookHandler) Handle(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	if signature == "" {
		h.logger.Warn("stripe webhook missing signature header")
		c.Status(http.StatusBadRequest)
		return
	}

	if err := h.paymentService.HandleWebhook(
		c.Request.Context(), "stripe", payload, signature,
	); err != nil {
		h.logger.Error("failed to process stripe webhook", "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}
