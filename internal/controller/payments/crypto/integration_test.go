package crypto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBTCPay_FullCreateToWebhookFlow(t *testing.T) {
	var createdInvoiceID string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/invoices"):
			var req btcpayInvoiceRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			createdInvoiceID = "INV-FLOW-001"
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(btcpayInvoiceResponse{
				ID:           createdInvoiceID,
				Status:       "New",
				Amount:       req.Amount,
				Currency:     req.Currency,
				CheckoutLink: "https://btcpay.example.com/i/" + createdInvoiceID,
				Metadata:     req.Metadata,
			})

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, createdInvoiceID):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(btcpayInvoiceResponse{
				ID:       createdInvoiceID,
				Status:   "Settled",
				Amount:   "75.00",
				Currency: "USD",
				Metadata: map[string]string{
					"account_id": "acct-flow-test",
					"payment_id": "pay-flow-test",
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

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

	// Step 1: Create payment session
	session, err := p.CreatePaymentSession(context.Background(), &CreatePaymentRequest{
		AccountID:   "acct-flow-test",
		PaymentID:   "pay-flow-test",
		AmountCents: 7500,
		Currency:    "USD",
		Description: "Flow Test",
	})
	require.NoError(t, err)
	assert.Equal(t, "INV-FLOW-001", session.GatewayPaymentID)

	// Step 2: Simulate webhook for InvoiceSettled
	webhookPayload := `{"deliveryId":"d-flow","type":"InvoiceSettled","invoiceId":"INV-FLOW-001","storeId":"test-store"}`
	sig := "sha256=" + computeHMACSHA256("test-webhook-secret", []byte(webhookPayload))

	headers := http.Header{}
	headers.Set("BTCPay-Sig", sig)

	result, err := p.HandleWebhook(context.Background(), headers, []byte(webhookPayload))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "acct-flow-test", result.AccountID)
	assert.Equal(t, int64(7500), result.AmountCents)
	assert.Equal(t, "btcpay:invoice:INV-FLOW-001", result.IdempotencyKey)
}
