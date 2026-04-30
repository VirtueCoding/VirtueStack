package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	creditFunc func(ctx context.Context, accountID string, amountCents int64, currency, gateway, gatewayPaymentID, externalPaymentID, idempotencyKey string) error
}

func (m *mockBillingCreditor) CreditFromPayment(ctx context.Context, accountID string, amountCents int64, currency, gateway, gatewayPaymentID, externalPaymentID, idempotencyKey string) error {
	return m.creditFunc(ctx, accountID, amountCents, currency, gateway, gatewayPaymentID, externalPaymentID, idempotencyKey)
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
		creditFunc: func(_ context.Context, accountID string, amountCents int64, currency, gateway, gatewayPaymentID, externalPaymentID, idempotencyKey string) error {
			creditCalled = true
			assert.Equal(t, "acct-1", accountID)
			assert.Equal(t, int64(5000), amountCents)
			assert.Equal(t, "mock-crypto", gateway)
			assert.Equal(t, "INV-1", gatewayPaymentID)
			assert.Equal(t, "", externalPaymentID)
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
			return nil, fmt.Errorf("signature mismatch: %w", crypto.ErrWebhookVerification)
		},
	}

	handler := NewCryptoWebhookHandler(provider, nil, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/btcpay", bytes.NewReader([]byte(`{}`)))

	handler.HandleWebhook(c)

	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}

func TestCryptoWebhookHandler_VerifiedBTCPayFetchFailureReturnsServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/api/v1/stores/test-store/invoices/INV-1")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"temporary outage"}`))
	}))
	t.Cleanup(server.Close)

	provider, err := crypto.NewBTCPayProvider(crypto.BTCPayConfig{
		ServerURL:     server.URL,
		APIKey:        "test-api-key",
		StoreID:       "test-store",
		WebhookSecret: "test-webhook-secret",
		HTTPClient:    server.Client(),
		Logger:        slog.Default(),
	})
	assert.NoError(t, err)
	handler := NewCryptoWebhookHandler(provider, nil, slog.Default())

	payload := map[string]any{
		"deliveryId": "delivery-1",
		"type":       "InvoiceSettled",
		"invoiceId":  "INV-1",
		"storeId":    "test-store",
	}
	body, err := json.Marshal(payload)
	assert.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/btcpay", bytes.NewReader(body))
	c.Request.Header.Set("BTCPay-Sig", "sha256="+hmacSHA256Hex("test-webhook-secret", body))

	handler.HandleWebhook(c)

	assert.Equal(t, http.StatusInternalServerError, c.Writer.Status())
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
		creditFunc: func(_ context.Context, _ string, _ int64, _, _, _, _, _ string) error {
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

func hmacSHA256Hex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
