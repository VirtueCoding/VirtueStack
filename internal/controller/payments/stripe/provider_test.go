package stripe

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	gostripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
)

func testProvider(webhookSecret string) *Provider {
	return &Provider{
		secretKey:      "sk_test_fake",
		webhookSecret:  webhookSecret,
		publishableKey: "pk_test_fake",
		logger:         slog.Default(),
	}
}

func signPayload(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	sp := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  secret,
	})
	return sp.Header
}

func makeCheckoutEvent(t *testing.T, secret string) ([]byte, string) {
	t.Helper()
	sess := gostripe.CheckoutSession{
		PaymentStatus: gostripe.CheckoutSessionPaymentStatusPaid,
		PaymentIntent: &gostripe.PaymentIntent{ID: "pi_test_123"},
		AmountTotal:   1000,
		Currency:      "usd",
		Metadata:      map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
	}
	sessBytes, err := json.Marshal(sess)
	require.NoError(t, err)

	event := gostripe.Event{
		ID:   "evt_test_checkout",
		Type: "checkout.session.completed",
		Data: &gostripe.EventData{Raw: sessBytes},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	return payload, signPayload(t, payload, secret)
}

func TestHandleWebhook_CheckoutCompleted(t *testing.T) {
	secret := "whsec_test_secret"
	p := testProvider(secret)
	payload, sig := makeCheckoutEvent(t, secret)

	result, err := p.HandleWebhook(context.Background(), payload, sig)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, payments.WebhookEventPaymentCompleted, result.Type)
	assert.Equal(t, "pi_test_123", result.PaymentID)
	assert.Equal(t, int64(1000), result.AmountCents)
	assert.Equal(t, "usd", result.Currency)
	assert.Equal(t, "completed", result.Status)
	assert.Equal(t, "cust_1", result.Metadata["customer_id"])
	assert.Contains(t, result.IdempotencyKey, "stripe:event:")
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	p := testProvider("whsec_real_secret")
	payload := []byte(`{"id":"evt_bad","type":"checkout.session.completed"}`)
	badSig := "t=123,v1=badsignature"

	result, err := p.HandleWebhook(context.Background(), payload, badSig)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "verify stripe webhook signature")
}

func TestHandleWebhook_UnhandledEventType(t *testing.T) {
	secret := "whsec_test_unhandled"
	p := testProvider(secret)

	event := gostripe.Event{
		ID:   "evt_test_customer",
		Type: "customer.created",
		Data: &gostripe.EventData{Raw: json.RawMessage(`{}`)},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	sig := signPayload(t, payload, secret)
	result, webhookErr := p.HandleWebhook(context.Background(), payload, sig)
	require.NoError(t, webhookErr)
	assert.Nil(t, result)
}

func TestHandleWebhook_UnpaidCheckout(t *testing.T) {
	secret := "whsec_test_unpaid"
	p := testProvider(secret)

	sess := gostripe.CheckoutSession{
		PaymentStatus: gostripe.CheckoutSessionPaymentStatusUnpaid,
	}
	sessBytes, err := json.Marshal(sess)
	require.NoError(t, err)

	event := gostripe.Event{
		ID:   "evt_test_unpaid",
		Type: "checkout.session.completed",
		Data: &gostripe.EventData{Raw: sessBytes},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	sig := signPayload(t, payload, secret)
	result, webhookErr := p.HandleWebhook(context.Background(), payload, sig)
	require.NoError(t, webhookErr)
	assert.Nil(t, result)
}

func TestHandleWebhook_PaymentIntentSucceeded(t *testing.T) {
	secret := "whsec_test_pi"
	p := testProvider(secret)

	pi := gostripe.PaymentIntent{
		ID:       "pi_test_456",
		Amount:   2500,
		Currency: "usd",
		Metadata: map[string]string{"customer_id": "cust_2"},
	}
	piBytes, err := json.Marshal(pi)
	require.NoError(t, err)

	event := gostripe.Event{
		ID:   "evt_test_pi",
		Type: "payment_intent.succeeded",
		Data: &gostripe.EventData{Raw: piBytes},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	sig := signPayload(t, payload, secret)
	result, webhookErr := p.HandleWebhook(context.Background(), payload, sig)
	require.NoError(t, webhookErr)
	require.NotNil(t, result)
	assert.Equal(t, payments.WebhookEventPaymentCompleted, result.Type)
	assert.Equal(t, "pi_test_456", result.PaymentID)
	assert.Equal(t, int64(2500), result.AmountCents)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		webhook string
		wantErr bool
	}{
		{"valid config", "sk_test", "whsec_test", false},
		{"missing secret key", "", "whsec_test", true},
		{"missing webhook secret", "sk_test", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{
				secretKey:     tt.secret,
				webhookSecret: tt.webhook,
				logger:        slog.Default(),
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

func TestName(t *testing.T) {
	p := testProvider("")
	assert.Equal(t, "stripe", p.Name())
}

func TestMapStripeStatus(t *testing.T) {
	tests := []struct {
		name   string
		status gostripe.PaymentIntentStatus
		want   string
	}{
		{"succeeded", gostripe.PaymentIntentStatusSucceeded, "completed"},
		{"canceled", gostripe.PaymentIntentStatusCanceled, "failed"},
		{"processing", gostripe.PaymentIntentStatusProcessing, "pending"},
		{"requires_payment", gostripe.PaymentIntentStatusRequiresPaymentMethod, "pending"},
		{"unknown status", gostripe.PaymentIntentStatus("weird"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mapStripeStatus(tt.status))
		})
	}
}
