package webhooks

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments/paypal"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePayPalWebhookPaymentService struct {
	handleFunc func(ctx context.Context, headers http.Header, payload []byte) error
}

func (f *fakePayPalWebhookPaymentService) HandlePayPalWebhook(
	ctx context.Context, headers http.Header, payload []byte,
) error {
	if f.handleFunc != nil {
		return f.handleFunc(ctx, headers, payload)
	}
	return nil
}

func TestNewPayPalWebhookHandler(t *testing.T) {
	logger := logging.NewLogger("error")
	h := NewPayPalWebhookHandler(&fakePayPalWebhookPaymentService{}, logger)
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

func TestPayPalWebhookHandler_StatusMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		handleErr  error
		wantStatus int
	}{
		{
			name:       "validation failure returns bad request",
			handleErr:  sharederrors.NewValidationError("signature", "invalid"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "internal failure returns internal server error",
			handleErr:  errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewPayPalWebhookHandler(&fakePayPalWebhookPaymentService{
				handleFunc: func(ctx context.Context, headers http.Header, payload []byte) error {
					require.NotNil(t, ctx)
					require.Equal(t, "sig-123", headers.Get(paypal.HeaderTransmissionSig))
					require.Equal(t, `{"event_type":"PAYMENT.SALE.COMPLETED"}`, string(payload))
					return tt.handleErr
				},
			}, slog.Default())

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/webhooks/paypal",
				strings.NewReader(`{"event_type":"PAYMENT.SALE.COMPLETED"}`),
			)
			req.Header.Set(paypal.HeaderTransmissionID, "tx-123")
			req.Header.Set(paypal.HeaderTransmissionSig, "sig-123")
			c.Request = req

			handler.Handle(c)

			assert.Equal(t, tt.wantStatus, c.Writer.Status())
		})
	}
}
