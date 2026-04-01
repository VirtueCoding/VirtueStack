package webhooks

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
)

func TestNewPayPalWebhookHandler(t *testing.T) {
	logger := logging.NewLogger("error")
	svc := &services.PaymentService{}
	h := NewPayPalWebhookHandler(svc, logger)
	assert.NotNil(t, h)
	assert.NotNil(t, h.paymentService)
	assert.NotNil(t, h.logger)
}

func TestPayPalWebhookHandler_MissingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.Default()

	h := &PayPalWebhookHandler{logger: logger}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost, "/api/v1/webhooks/paypal",
		strings.NewReader(`{"event_type":"test"}`),
	)

	h.Handle(c)
	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}
