package webhooks

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments/paypal"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

const maxPayPalWebhookBodySize int64 = 1 << 20 // 1MB

// PayPalWebhookHandler handles PayPal webhook callbacks.
type PayPalWebhookHandler struct {
	paymentService *services.PaymentService
	logger         *slog.Logger
}

// NewPayPalWebhookHandler creates a new PayPalWebhookHandler.
func NewPayPalWebhookHandler(
	paymentService *services.PaymentService,
	logger *slog.Logger,
) *PayPalWebhookHandler {
	return &PayPalWebhookHandler{
		paymentService: paymentService,
		logger:         logger.With("component", "paypal-webhook"),
	}
}

// Handle processes POST /api/v1/webhooks/paypal.
// This endpoint is unauthenticated — PayPal signature verification
// happens inside the PayPal payment provider via the PayPal API.
func (h *PayPalWebhookHandler) Handle(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(
		c.Writer, c.Request.Body, maxPayPalWebhookBodySize,
	)
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	if !h.hasRequiredHeaders(c) {
		c.Status(http.StatusBadRequest)
		return
	}

	if err := h.paymentService.HandlePayPalWebhook(
		c.Request.Context(), c.Request.Header, payload,
	); err != nil {
		h.logger.Error("paypal webhook processing failed", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	c.Status(http.StatusOK)
}

func (h *PayPalWebhookHandler) hasRequiredHeaders(c *gin.Context) bool {
	required := []string{
		paypal.HeaderTransmissionID,
		paypal.HeaderTransmissionSig,
	}
	for _, header := range required {
		if c.GetHeader(header) == "" {
			h.logger.Warn("paypal webhook missing header",
				"header", header)
			return false
		}
	}
	return true
}
