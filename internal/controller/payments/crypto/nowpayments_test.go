package crypto

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNOWPaymentsProvider(t *testing.T, handler http.Handler) *NOWPaymentsProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	p, err := NewNOWPaymentsProvider(NOWPaymentsConfig{
		APIKey:      "test-api-key",
		IPNSecret:   "test-ipn-secret",
		CallbackURL: "https://example.com/webhooks/nowpayments",
		RedirectURL: "https://example.com/billing",
		HTTPClient:  srv.Client(),
		Logger:      testLogger(),
	})
	require.NoError(t, err)

	// Override the base URL to point at our test server.
	p.baseURL = srv.URL
	return p
}

func TestNewNOWPaymentsProvider_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     NOWPaymentsConfig
		wantErr bool
	}{
		{
			"valid config",
			NOWPaymentsConfig{
				APIKey:    "key",
				IPNSecret: "secret",
				Logger:    testLogger(),
			},
			false,
		},
		{"missing API key", NOWPaymentsConfig{IPNSecret: "s", Logger: testLogger()}, true},
		{"missing IPN secret", NOWPaymentsConfig{APIKey: "k", Logger: testLogger()}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewNOWPaymentsProvider(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNOWPayments_ProviderName(t *testing.T) {
	p := &NOWPaymentsProvider{}
	assert.Equal(t, "nowpayments", p.ProviderName())
}

func TestNOWPayments_CreatePaymentSession_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))

		var req nowInvoiceRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.InDelta(t, 25.00, req.PriceAmount, 0.001)
		assert.Equal(t, "usd", req.PriceCurrency)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(nowInvoiceResponse{
			ID:         "INV-NOW-001",
			InvoiceURL: "https://nowpayments.io/payment/INV-NOW-001",
			OrderID:    req.OrderID,
		})
	})

	p := newTestNOWPaymentsProvider(t, handler)
	session, err := p.CreatePaymentSession(context.Background(), &CreatePaymentRequest{
		AccountID:   "acct-1",
		PaymentID:   "pay-1",
		AmountCents: 2500,
		Currency:    "USD",
		Description: "Credit Top-Up",
	})

	require.NoError(t, err)
	assert.Equal(t, "INV-NOW-001", session.GatewayPaymentID)
	assert.Equal(t, "https://nowpayments.io/payment/INV-NOW-001", session.CheckoutURL)
}

func TestNOWPayments_GetPaymentStatus_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(nowPaymentResponse{
			PaymentStatus: "finished",
			PriceAmount:   50.00,
			PriceCurrency: "usd",
		})
	})

	p := newTestNOWPaymentsProvider(t, handler)
	status, err := p.GetPaymentStatus(context.Background(), "PAY-123")

	require.NoError(t, err)
	assert.Equal(t, "finished", status.Status)
	assert.Equal(t, int64(5000), status.AmountCents)
	assert.Equal(t, "USD", status.Currency)
}

func computeNOWPaymentsSignature(secret string, body []byte) string {
	var decoded map[string]any
	json.Unmarshal(body, &decoded)
	sorted, _ := json.Marshal(decoded)
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(sorted)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestNOWPayments_HandleWebhook_Finished(t *testing.T) {
	p, err := NewNOWPaymentsProvider(NOWPaymentsConfig{
		APIKey:    "test-api-key",
		IPNSecret: "test-ipn-secret",
		Logger:    testLogger(),
	})
	require.NoError(t, err)

	payload := `{
		"payment_id": 12345,
		"payment_status": "finished",
		"pay_address": "bc1q...",
		"price_amount": 25.00,
		"price_currency": "usd",
		"pay_amount": 0.001,
		"pay_currency": "btc",
		"order_id": "pay-abc-123"
	}`

	sig := computeNOWPaymentsSignature("test-ipn-secret", []byte(payload))
	headers := http.Header{}
	headers.Set("X-Nowpayments-Sig", sig)

	result, err := p.HandleWebhook(context.Background(), headers, []byte(payload))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "payment.finished", result.EventType)
	assert.Equal(t, "12345", result.InvoiceID)
	assert.Equal(t, "pay-abc-123", result.AccountID)
	assert.Equal(t, int64(2500), result.AmountCents)
	assert.Equal(t, "USD", result.Currency)
	assert.Equal(t, "nowpayments:payment:12345", result.IdempotencyKey)
}

func TestNOWPayments_HandleWebhook_NonFinished(t *testing.T) {
	p, err := NewNOWPaymentsProvider(NOWPaymentsConfig{
		APIKey:    "test-api-key",
		IPNSecret: "test-ipn-secret",
		Logger:    testLogger(),
	})
	require.NoError(t, err)

	payload := `{
		"payment_id": 67890,
		"payment_status": "waiting",
		"price_amount": 10.00,
		"price_currency": "usd",
		"order_id": "pay-xyz"
	}`

	sig := computeNOWPaymentsSignature("test-ipn-secret", []byte(payload))
	headers := http.Header{}
	headers.Set("X-Nowpayments-Sig", sig)

	result, err := p.HandleWebhook(context.Background(), headers, []byte(payload))
	require.NoError(t, err)
	assert.Nil(t, result, "non-finished payments should return nil")
}

func TestNOWPayments_HandleWebhook_InvalidSignature(t *testing.T) {
	p, err := NewNOWPaymentsProvider(NOWPaymentsConfig{
		APIKey:    "test-api-key",
		IPNSecret: "test-ipn-secret",
		Logger:    testLogger(),
	})
	require.NoError(t, err)

	headers := http.Header{}
	headers.Set("X-Nowpayments-Sig", "invalid-signature")

	_, err = p.HandleWebhook(context.Background(), headers, []byte(`{"payment_status":"finished"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestNOWPayments_HandleWebhook_MissingSignature(t *testing.T) {
	p, err := NewNOWPaymentsProvider(NOWPaymentsConfig{
		APIKey:    "test-api-key",
		IPNSecret: "test-ipn-secret",
		Logger:    testLogger(),
	})
	require.NoError(t, err)

	_, err = p.HandleWebhook(context.Background(), http.Header{}, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "X-Nowpayments-Sig")
}

func TestVerifyNOWPaymentsSignature(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		sig     string
		body    string
		wantErr bool
	}{
		{
			"valid signature",
			"secret",
			computeNOWPaymentsSignature("secret", []byte(`{"key":"value"}`)),
			`{"key":"value"}`,
			false,
		},
		{"empty secret", "", "sig", `{}`, true},
		{"empty signature", "secret", "", `{}`, true},
		{
			"mismatched signature",
			"secret",
			"wrong-signature",
			`{"key":"value"}`,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyNOWPaymentsSignature(tt.secret, tt.sig, []byte(tt.body))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
