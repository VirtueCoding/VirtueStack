package webhooks

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments/crypto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type mockCryptoProvider struct {
	handleWebhookFunc func(ctx context.Context, headers http.Header, body []byte) (*crypto.WebhookResult, error)
}

func (m *mockCryptoProvider) CreatePaymentSession(_ context.Context, _ *crypto.CreatePaymentRequest) (*crypto.PaymentSession, error) {
	return nil, nil
}

func (m *mockCryptoProvider) HandleWebhook(ctx context.Context, headers http.Header, body []byte) (*crypto.WebhookResult, error) {
	return m.handleWebhookFunc(ctx, headers, body)
}

func (m *mockCryptoProvider) GetPaymentStatus(_ context.Context, _ string) (*crypto.PaymentStatus, error) {
	return nil, nil
}

func (m *mockCryptoProvider) ProviderName() string {
	return "mock-crypto"
}

type mockBillingCreditor struct {
	creditFunc func(ctx context.Context, accountID string, amountCents int64, currency, gateway, gatewayPaymentID, idempotencyKey string) error
}

func (m *mockBillingCreditor) CreditFromPayment(ctx context.Context, accountID string, amountCents int64, currency, gateway, gatewayPaymentID, idempotencyKey string) error {
	return m.creditFunc(ctx, accountID, amountCents, currency, gateway, gatewayPaymentID, idempotencyKey)
}

func TestCryptoWebhookHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCryptoProvider{
		handleWebhookFunc: func(_ context.Context, _ http.Header, _ []byte) (*crypto.WebhookResult, error) {
			return &crypto.WebhookResult{
				InvoiceID:      "INV-1",
				AccountID:      "acct-1",
				AmountCents:    5000,
				Currency:       "USD",
				IdempotencyKey: "btcpay:invoice:INV-1",
			}, nil
		},
	}

	var creditCalled bool
	creditor := &mockBillingCreditor{
		creditFunc: func(_ context.Context, accountID string, amountCents int64, currency, gateway, gatewayPaymentID, idempotencyKey string) error {
			creditCalled = true
			assert.Equal(t, "acct-1", accountID)
			assert.Equal(t, int64(5000), amountCents)
			assert.Equal(t, "crypto", gateway)
			assert.Equal(t, "btcpay:invoice:INV-1", idempotencyKey)
			return nil
		},
	}

	handler := NewCryptoWebhookHandler(provider, creditor, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/btcpay", bytes.NewReader([]byte(`{}`)))

	handler.HandleWebhook(c)

	assert.Equal(t, http.StatusOK, c.Writer.Status())
	assert.True(t, creditCalled, "creditor should have been called")
}

func TestCryptoWebhookHandler_NonActionableEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCryptoProvider{
		handleWebhookFunc: func(_ context.Context, _ http.Header, _ []byte) (*crypto.WebhookResult, error) {
			return nil, nil
		},
	}

	handler := NewCryptoWebhookHandler(provider, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/btcpay", bytes.NewReader([]byte(`{}`)))

	handler.HandleWebhook(c)

	assert.Equal(t, http.StatusOK, c.Writer.Status())
}

func TestCryptoWebhookHandler_VerificationFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCryptoProvider{
		handleWebhookFunc: func(_ context.Context, _ http.Header, _ []byte) (*crypto.WebhookResult, error) {
			return nil, fmt.Errorf("signature mismatch")
		},
	}

	handler := NewCryptoWebhookHandler(provider, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/btcpay", bytes.NewReader([]byte(`{}`)))

	handler.HandleWebhook(c)

	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}

func TestCryptoWebhookHandler_CreditFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &mockCryptoProvider{
		handleWebhookFunc: func(_ context.Context, _ http.Header, _ []byte) (*crypto.WebhookResult, error) {
			return &crypto.WebhookResult{
				InvoiceID:      "INV-2",
				AccountID:      "acct-2",
				AmountCents:    1000,
				Currency:       "USD",
				IdempotencyKey: "btcpay:invoice:INV-2",
			}, nil
		},
	}

	creditor := &mockBillingCreditor{
		creditFunc: func(_ context.Context, _ string, _ int64, _, _, _, _ string) error {
			return fmt.Errorf("database error")
		},
	}

	handler := NewCryptoWebhookHandler(provider, creditor, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/btcpay", bytes.NewReader([]byte(`{}`)))

	handler.HandleWebhook(c)

	assert.Equal(t, http.StatusInternalServerError, c.Writer.Status())
}
