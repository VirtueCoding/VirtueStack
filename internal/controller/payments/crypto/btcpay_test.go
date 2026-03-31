package crypto

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.Default()
}

func newTestBTCPayProvider(t *testing.T, handler http.Handler) *BTCPayProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	p, err := NewBTCPayProvider(BTCPayConfig{
		ServerURL:     srv.URL,
		APIKey:        "test-api-key",
		StoreID:       "test-store",
		WebhookSecret: "test-webhook-secret",
		RedirectURL:   "https://example.com/billing",
		HTTPClient:    srv.Client(),
		Logger:        testLogger(),
	})
	require.NoError(t, err)
	return p
}

func TestNewBTCPayProvider_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BTCPayConfig
		wantErr bool
	}{
		{
			"valid config",
			BTCPayConfig{
				ServerURL: "https://btcpay.example.com",
				APIKey:    "key",
				StoreID:   "store",
				Logger:    testLogger(),
			},
			false,
		},
		{"missing server URL", BTCPayConfig{APIKey: "k", StoreID: "s", Logger: testLogger()}, true},
		{"missing API key", BTCPayConfig{ServerURL: "u", StoreID: "s", Logger: testLogger()}, true},
		{"missing store ID", BTCPayConfig{ServerURL: "u", APIKey: "k", Logger: testLogger()}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBTCPayProvider(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBTCPay_CreatePaymentSession_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/invoices")
		assert.Equal(t, "token test-api-key", r.Header.Get("Authorization"))

		var req btcpayInvoiceRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "25.00", req.Amount)
		assert.Equal(t, "USD", req.Currency)
		assert.Equal(t, "acct-1", req.Metadata["account_id"])

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(btcpayInvoiceResponse{
			ID:             "INV-BTC-001",
			Status:         "New",
			Amount:         "25.00",
			Currency:       "USD",
			CheckoutLink:   "https://btcpay.example.com/i/INV-BTC-001",
			ExpirationTime: 1735689600,
		})
	})

	p := newTestBTCPayProvider(t, handler)
	session, err := p.CreatePaymentSession(context.Background(), &CreatePaymentRequest{
		AccountID:   "acct-1",
		PaymentID:   "pay-1",
		AmountCents: 2500,
		Currency:    "USD",
		Description: "Credit Top-Up",
	})

	require.NoError(t, err)
	assert.Equal(t, "INV-BTC-001", session.GatewayPaymentID)
	assert.Equal(t, "https://btcpay.example.com/i/INV-BTC-001", session.CheckoutURL)
}

func TestBTCPay_CreatePaymentSession_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request"}`))
	})

	p := newTestBTCPayProvider(t, handler)
	_, err := p.CreatePaymentSession(context.Background(), &CreatePaymentRequest{
		AmountCents: 100,
		Currency:    "USD",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestBTCPay_GetPaymentStatus_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(btcpayInvoiceResponse{
			ID:       "INV-STATUS",
			Status:   "Settled",
			Amount:   "50.00",
			Currency: "USD",
		})
	})

	p := newTestBTCPayProvider(t, handler)
	status, err := p.GetPaymentStatus(context.Background(), "INV-STATUS")

	require.NoError(t, err)
	assert.Equal(t, "Settled", status.Status)
	assert.Equal(t, int64(5000), status.AmountCents)
	assert.Equal(t, "USD", status.Currency)
}

func TestBTCPay_HandleWebhook_InvoiceSettled(t *testing.T) {
	invoiceHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(btcpayInvoiceResponse{
			ID:       "INV-SETTLED",
			Status:   "Settled",
			Amount:   "100.00",
			Currency: "USD",
			Metadata: map[string]string{
				"account_id": "acct-42",
				"payment_id": "pay-99",
			},
		})
	})

	p := newTestBTCPayProvider(t, invoiceHandler)

	payload := `{"deliveryId":"d1","type":"InvoiceSettled","invoiceId":"INV-SETTLED","storeId":"test-store"}`
	sig := "sha256=" + computeHMACSHA256("test-webhook-secret", []byte(payload))

	headers := http.Header{}
	headers.Set("BTCPay-Sig", sig)

	result, err := p.HandleWebhook(context.Background(), headers, []byte(payload))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "InvoiceSettled", result.EventType)
	assert.Equal(t, "INV-SETTLED", result.InvoiceID)
	assert.Equal(t, "acct-42", result.AccountID)
	assert.Equal(t, int64(10000), result.AmountCents)
	assert.Equal(t, "btcpay:invoice:INV-SETTLED", result.IdempotencyKey)
}

func TestBTCPay_HandleWebhook_NonSettledEvent(t *testing.T) {
	p := newTestBTCPayProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	payload := `{"deliveryId":"d2","type":"InvoiceCreated","invoiceId":"INV-NEW"}`
	sig := "sha256=" + computeHMACSHA256("test-webhook-secret", []byte(payload))

	headers := http.Header{}
	headers.Set("BTCPay-Sig", sig)

	result, err := p.HandleWebhook(context.Background(), headers, []byte(payload))
	require.NoError(t, err)
	assert.Nil(t, result, "non-settled events should return nil")
}

func TestBTCPay_HandleWebhook_InvalidSignature(t *testing.T) {
	p := newTestBTCPayProvider(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	headers := http.Header{}
	headers.Set("BTCPay-Sig", "sha256=0000000000000000000000000000000000000000000000000000000000000000")

	_, err := p.HandleWebhook(context.Background(), headers, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature")
}

func TestBTCPay_HandleWebhook_MissingSignature(t *testing.T) {
	p := newTestBTCPayProvider(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	_, err := p.HandleWebhook(context.Background(), http.Header{}, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BTCPay-Sig")
}

func TestBTCPay_ProviderName(t *testing.T) {
	p := &BTCPayProvider{}
	assert.Equal(t, "btcpay", p.ProviderName())
}
