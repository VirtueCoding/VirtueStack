package paypal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider(t *testing.T, handler http.Handler) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Provider{
		tokenClient: &TokenClient{
			clientID:     "test-id",
			clientSecret: "test-secret",
			baseURL:      srv.URL,
			httpClient:   srv.Client(),
			logger:       testLogger(),
			accessToken:  "pre-cached-token",
			expiresAt:    time.Now().Add(1 * time.Hour),
		},
		webhookID:  "webhook-123",
		returnURL:  "https://example.com/return",
		cancelURL:  "https://example.com/cancel",
		httpClient: srv.Client(),
		logger:     slog.Default(),
	}
}

func newTestProviderWithWebhookID(
	t *testing.T, handler http.Handler, webhookID string,
) *Provider {
	t.Helper()
	p := newTestProvider(t, handler)
	p.webhookID = webhookID
	return p
}

func TestCreatePaymentSession_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/checkout/orders" {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "Bearer pre-cached-token", r.Header.Get("Authorization"))

			var req orderRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "CAPTURE", req.Intent)
			assert.Equal(t, "10.50", req.PurchaseUnits[0].Amount.Value)
			assert.Equal(t, "USD", req.PurchaseUnits[0].Amount.CurrencyCode)

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(orderResponse{
				ID:     "ORDER-123",
				Status: "CREATED",
				Links: []orderLink{
					{Href: "https://paypal.com/approve/ORDER-123", Rel: "payer-action"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	p := newTestProvider(t, handler)

	sess, err := p.CreatePaymentSession(context.Background(), paymentRequestFixture())
	require.NoError(t, err)
	assert.Equal(t, "ORDER-123", sess.GatewaySessionID)
	assert.Equal(t, "https://paypal.com/approve/ORDER-123", sess.PaymentURL)
}

func TestCreatePaymentSession_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"name":"INVALID_REQUEST"}`))
	})

	p := newTestProvider(t, handler)
	_, err := p.CreatePaymentSession(context.Background(), paymentRequestFixture())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestCreatePaymentSession_MissingApprovalLink(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(orderResponse{
			ID:     "ORDER-NOLINK",
			Status: "CREATED",
			Links:  []orderLink{},
		})
	})

	p := newTestProvider(t, handler)
	_, err := p.CreatePaymentSession(context.Background(), paymentRequestFixture())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing approval link")
}

func TestCaptureOrder_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/capture")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(captureResponse{
			ID:     "ORDER-456",
			Status: "COMPLETED",
			PurchaseUnits: []capturedUnit{{
				Payments: &capturedPayments{
					Captures: []captureDetail{{
						ID:     "CAP-789",
						Status: "COMPLETED",
						Amount: &amount{CurrencyCode: "USD", Value: "25.00"},
					}},
				},
			}},
		})
	})

	p := newTestProvider(t, handler)
	capID, status, currency, cents, err := p.CaptureOrder(context.Background(), "ORDER-456")

	require.NoError(t, err)
	assert.Equal(t, "CAP-789", capID)
	assert.Equal(t, "COMPLETED", status)
	assert.Equal(t, int64(2500), cents)
	assert.Equal(t, "USD", currency)
}

func TestCaptureOrder_EmptyCaptures(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(captureResponse{
			ID:            "ORDER-EMPTY",
			Status:        "COMPLETED",
			PurchaseUnits: []capturedUnit{},
		})
	})

	p := newTestProvider(t, handler)
	_, _, _, _, err := p.CaptureOrder(context.Background(), "ORDER-EMPTY")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing capture detail")
}

func TestGetPaymentStatus_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(orderResponse{
			ID:     "ORDER-STATUS",
			Status: "COMPLETED",
		})
	})

	p := newTestProvider(t, handler)
	status, err := p.GetPaymentStatus(context.Background(), "ORDER-STATUS")

	require.NoError(t, err)
	assert.Equal(t, "completed", status.Status)
	assert.NotNil(t, status.PaidAt)
}

func TestGetPaymentStatus_Pending(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(orderResponse{
			ID:     "ORDER-PEND",
			Status: "CREATED",
		})
	})

	p := newTestProvider(t, handler)
	status, err := p.GetPaymentStatus(context.Background(), "ORDER-PEND")

	require.NoError(t, err)
	assert.Equal(t, "pending", status.Status)
	assert.Nil(t, status.PaidAt)
}

func TestRefundPayment_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/refund")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(refundResponse{
			ID:     "REF-001",
			Status: "COMPLETED",
		})
	})

	p := newTestProvider(t, handler)
	result, err := p.RefundPayment(context.Background(), "CAP-001", 500)

	require.NoError(t, err)
	assert.Equal(t, "REF-001", result.GatewayRefundID)
	assert.Equal(t, "CAP-001", result.GatewayPaymentID)
	assert.Equal(t, int64(500), result.AmountCents)
	assert.Equal(t, "completed", result.Status)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		secret  string
		wantErr bool
	}{
		{"valid", "id", "secret", false},
		{"missing id", "", "secret", true},
		{"missing secret", "id", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{
				tokenClient: &TokenClient{
					clientID:     tt.id,
					clientSecret: tt.secret,
				},
			}
			err := p.ValidateConfig()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCentsToDecimal(t *testing.T) {
	tests := []struct {
		name  string
		cents int64
		want  string
	}{
		{"zero", 0, "0.00"},
		{"whole dollars", 1000, "10.00"},
		{"with cents", 1050, "10.50"},
		{"single cent", 1, "0.01"},
		{"large amount", 999999, "9999.99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, centsToDecimal(tt.cents))
		})
	}
}

func TestDecimalToCents(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"whole number", "10", 1000, false},
		{"with cents", "10.50", 1050, false},
		{"single decimal", "10.5", 1050, false},
		{"zero", "0.00", 0, false},
		{"one cent", "0.01", 1, false},
		{"large amount", "9999.99", 999999, false},
		{"invalid", "abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decimalToCents(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestName(t *testing.T) {
	p := &Provider{}
	assert.Equal(t, "paypal", p.Name())
}

// --- Webhook tests ---

func TestVerifyWebhookSignature_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/notifications/verify-webhook-signature" {
			var req verifyWebhookRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "SHA256withRSA", req.AuthAlgo)
			assert.Equal(t, "webhook-123", req.WebhookID)
			assert.NotEmpty(t, req.TransmissionID)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(verifyWebhookResponse{
				VerificationStatus: "SUCCESS",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	p := newTestProviderWithWebhookID(t, handler, "webhook-123")

	headers := http.Header{}
	headers.Set("Paypal-Auth-Algo", "SHA256withRSA")
	headers.Set("Paypal-Cert-Url", "https://paypal.com/cert")
	headers.Set("Paypal-Transmission-Id", "txn-001")
	headers.Set("Paypal-Transmission-Sig", "sig-abc")
	headers.Set("Paypal-Transmission-Time", "2026-01-01T00:00:00Z")

	err := p.VerifyWebhookSignature(
		context.Background(),
		headers,
		[]byte(`{"event_type":"CHECKOUT.ORDER.APPROVED"}`),
	)
	require.NoError(t, err)
}

func TestVerifyWebhookSignature_Failure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(verifyWebhookResponse{
			VerificationStatus: "FAILURE",
		})
	})

	p := newTestProviderWithWebhookID(t, handler, "webhook-123")
	err := p.VerifyWebhookSignature(
		context.Background(), http.Header{}, []byte(`{}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestVerifyWebhookSignature_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})

	p := newTestProviderWithWebhookID(t, handler, "webhook-123")
	err := p.VerifyWebhookSignature(
		context.Background(), http.Header{}, []byte(`{}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestParseWebhookEvent(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantType string
		wantErr  bool
	}{
		{
			"valid event",
			`{"id":"EVT-1","event_type":"CHECKOUT.ORDER.APPROVED","resource_type":"checkout-order","resource":{}}`,
			"CHECKOUT.ORDER.APPROVED",
			false,
		},
		{
			"capture completed",
			`{"id":"EVT-2","event_type":"PAYMENT.CAPTURE.COMPLETED","resource_type":"capture","resource":{"id":"CAP-1"}}`,
			"PAYMENT.CAPTURE.COMPLETED",
			false,
		},
		{"invalid json", `{bad`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseWebhookEvent([]byte(tt.body))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, event.EventType)
		})
	}
}

func TestParseWebhookResource(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantID  string
		wantErr bool
	}{
		{
			"valid resource",
			`{"id":"CAP-123","status":"COMPLETED","custom_id":"acct-1","amount":{"currency_code":"USD","value":"10.00"}}`,
			"CAP-123",
			false,
		},
		{"invalid json", `{bad`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseWebhookResource(json.RawMessage(tt.raw))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, res.ID)
		})
	}
}

func TestHandleWebhook_CaptureCompleted(t *testing.T) {
	body := `{
		"id": "EVT-001",
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource_type": "capture",
		"resource": {
			"id": "CAP-ABC",
			"status": "COMPLETED",
			"custom_id": "acct-42",
			"amount": {"currency_code": "USD", "value": "50.00"},
			"supplementary_data": {"related_ids": {"order_id": "ORDER-XYZ"}}
		}
	}`

	p := &Provider{logger: slog.Default()}
	result, err := p.HandleWebhook(context.Background(), []byte(body), "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "paypal:capture:CAP-ABC", result.IdempotencyKey)
	assert.Equal(t, int64(5000), result.AmountCents)
	assert.Equal(t, "USD", result.Currency)
	assert.Equal(t, "acct-42", result.Metadata["customer_id"])
	assert.Equal(t, "CAP-ABC", result.PaymentID)
}

func TestHandleWebhook_NonCaptureEvent(t *testing.T) {
	body := `{
		"id": "EVT-002",
		"event_type": "CHECKOUT.ORDER.APPROVED",
		"resource_type": "checkout-order",
		"resource": {"id": "ORDER-123"}
	}`

	p := &Provider{logger: slog.Default()}
	result, err := p.HandleWebhook(context.Background(), []byte(body), "")
	require.NoError(t, err)
	assert.Nil(t, result, "non-capture events should return nil result")
}

func TestHandleWebhook_CapturePending(t *testing.T) {
	body := `{
		"id": "EVT-003",
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource_type": "capture",
		"resource": {
			"id": "CAP-PEND",
			"status": "PENDING",
			"amount": {"currency_code": "USD", "value": "10.00"}
		}
	}`

	p := &Provider{logger: slog.Default()}
	result, err := p.HandleWebhook(context.Background(), []byte(body), "")
	require.NoError(t, err)
	assert.Nil(t, result, "pending captures should return nil")
}

func TestHandleWebhook_InvalidJSON(t *testing.T) {
	p := &Provider{logger: slog.Default()}
	_, err := p.HandleWebhook(context.Background(), []byte(`{bad`), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse paypal webhook event")
}

func TestMapPayPalStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"COMPLETED", "completed"},
		{"VOIDED", "failed"},
		{"CREATED", "pending"},
		{"APPROVED", "pending"},
		{"UNKNOWN_VALUE", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, mapPayPalStatus(tt.input))
		})
	}
}
