package webhooks

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	paypalprovider "github.com/AbuGosok/VirtueStack/internal/controller/payments/paypal"
	stripeprovider "github.com/AbuGosok/VirtueStack/internal/controller/payments/stripe"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type webhookPaymentProvider struct {
	name              string
	handleWebhookFunc func(ctx context.Context, payload []byte, sig string) (*payments.WebhookEvent, error)
	verifyWebhookFunc func(ctx context.Context, headers http.Header, body []byte) error
}

func (p *webhookPaymentProvider) Name() string { return p.name }

func (p *webhookPaymentProvider) CreatePaymentSession(
	_ context.Context, _ payments.PaymentRequest,
) (*payments.PaymentSession, error) {
	return nil, nil
}

func (p *webhookPaymentProvider) HandleWebhook(
	ctx context.Context, payload []byte, sig string,
) (*payments.WebhookEvent, error) {
	return p.handleWebhookFunc(ctx, payload, sig)
}

func (p *webhookPaymentProvider) GetPaymentStatus(
	_ context.Context, _ string,
) (*payments.PaymentStatus, error) {
	return nil, nil
}

func (p *webhookPaymentProvider) RefundPayment(
	_ context.Context, _ string, _ int64, _ string,
) (*payments.RefundResult, error) {
	return nil, nil
}

func (p *webhookPaymentProvider) ValidateConfig() error { return nil }

func (p *webhookPaymentProvider) VerifyWebhookSignature(
	ctx context.Context, headers http.Header, body []byte,
) error {
	return p.verifyWebhookFunc(ctx, headers, body)
}

type webhookPaymentRepo struct {
	getByIDFunc               func(ctx context.Context, id string) (*models.BillingPayment, error)
	getByGatewayFunc          func(ctx context.Context, gw, id string) (*models.BillingPayment, error)
	completeFunc              func(ctx context.Context, id, gateway, gatewayPaymentID string) (bool, error)
	completePayPalCaptureFunc func(ctx context.Context, id, orderID, captureID string) (bool, error)
}

func (r *webhookPaymentRepo) Create(
	_ context.Context, _ *models.BillingPayment,
) error {
	return nil
}

func (r *webhookPaymentRepo) GetByID(
	ctx context.Context, id string,
) (*models.BillingPayment, error) {
	return r.getByIDFunc(ctx, id)
}

func (r *webhookPaymentRepo) GetByGatewayPaymentID(
	ctx context.Context, gw, id string,
) (*models.BillingPayment, error) {
	return r.getByGatewayFunc(ctx, gw, id)
}

func (r *webhookPaymentRepo) UpdateStatus(
	_ context.Context, _, _ string, _ *string,
) error {
	return nil
}

func (r *webhookPaymentRepo) CompleteWithGatewayPaymentID(
	ctx context.Context, id, gateway, gatewayPaymentID string,
) (bool, error) {
	return r.completeFunc(ctx, id, gateway, gatewayPaymentID)
}

func (r *webhookPaymentRepo) CompleteWithGatewayPaymentIDAndCredit(
	ctx context.Context, req repository.PaymentCompletionCredit,
) (bool, error) {
	return r.completeFunc(ctx, req.PaymentID, req.Gateway, req.GatewayPaymentID)
}

func (r *webhookPaymentRepo) CompletePayPalCapture(
	ctx context.Context, id, orderID, captureID string,
) (bool, error) {
	return r.completePayPalCaptureFunc(ctx, id, orderID, captureID)
}

func (r *webhookPaymentRepo) CompletePayPalCaptureAndCredit(
	ctx context.Context, req repository.PayPalCaptureCredit,
) (bool, error) {
	return r.completePayPalCaptureFunc(ctx, req.PaymentID, req.OrderID, req.CaptureID)
}

func (r *webhookPaymentRepo) MarkRefundedAndDebit(
	_ context.Context, _ repository.PaymentRefundDebit,
) error {
	return nil
}

func (r *webhookPaymentRepo) ListByCustomer(
	_ context.Context, _ string, _ models.PaginationParams,
) ([]models.BillingPayment, bool, string, error) {
	return nil, false, "", nil
}

func (r *webhookPaymentRepo) ListAll(
	_ context.Context, _ repository.BillingPaymentListFilter,
) ([]models.BillingPayment, bool, string, error) {
	return nil, false, "", nil
}

func newWebhookPaymentService(
	t *testing.T,
	provider payments.PaymentProvider,
	repo services.BillingPaymentRepo,
) *services.PaymentService {
	t.Helper()
	reg := payments.NewPaymentRegistry()
	require.NoError(t, reg.Register(provider.Name(), provider))
	logger := logging.NewLogger("error")
	return services.NewPaymentService(services.PaymentServiceConfig{
		PaymentRegistry: reg,
		PaymentRepo:     repo,
		Logger:          logger,
	})
}

func TestStripeWebhookHandler_InvalidSignatureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := stripeprovider.NewProvider(stripeprovider.ProviderConfig{
		SecretKey:      "sk_test_fake",
		WebhookSecret:  "whsec_route_test",
		PublishableKey: "pk_test_fake",
		Logger:         logging.NewLogger("error"),
	})
	svc := newWebhookPaymentService(t, provider, &webhookPaymentRepo{})
	handler := NewStripeWebhookHandler(svc, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader([]byte(`{}`)),
	)
	c.Request.Header.Set("Stripe-Signature", "bad")

	handler.Handle(c)

	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}

func TestStripeWebhookHandler_ProcessingFailureReturnsServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &webhookPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_1",
				AmountCents: 1000, Currency: "usd",
				Metadata: map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
			}, nil
		},
	}
	repo := &webhookPaymentRepo{
		getByIDFunc: func(_ context.Context, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "stripe",
				Amount: 1000, Currency: "usd",
			}, nil
		},
		completeFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			return false, errors.New("database unavailable")
		},
	}
	svc := newWebhookPaymentService(t, provider, repo)
	handler := NewStripeWebhookHandler(svc, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader([]byte(`{}`)),
	)
	c.Request.Header.Set("Stripe-Signature", "valid")

	handler.Handle(c)

	assert.Equal(t, http.StatusInternalServerError, c.Writer.Status())
}

func TestPayPalWebhookHandler_VerificationFailureReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &webhookPaymentProvider{
		name: "paypal",
		verifyWebhookFunc: func(_ context.Context, _ http.Header, _ []byte) error {
			return payments.ErrWebhookVerification
		},
	}
	svc := newWebhookPaymentService(t, provider, &webhookPaymentRepo{})
	handler := NewPayPalWebhookHandler(svc, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost, "/api/v1/webhooks/paypal", bytes.NewReader([]byte(`{}`)),
	)
	setRequiredPayPalHeaders(c.Request.Header)

	handler.Handle(c)

	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}

func TestPayPalWebhookHandler_VerifiedProcessingFailureReturnsServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orderID := "ORDER-1"
	provider := &webhookPaymentProvider{
		name: "paypal",
		verifyWebhookFunc: func(_ context.Context, _ http.Header, _ []byte) error {
			return nil
		},
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type: payments.WebhookEventPaymentCompleted, PaymentID: "CAP-1",
				AmountCents: 1000, Currency: "USD",
				Metadata: map[string]string{"customer_id": "cust_1", "payment_id": orderID},
			}, nil
		},
	}
	repo := &webhookPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
			}, nil
		},
		completePayPalCaptureFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			return false, errors.New("database unavailable")
		},
	}
	svc := newWebhookPaymentService(t, provider, repo)
	handler := NewPayPalWebhookHandler(svc, slog.Default())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost, "/api/v1/webhooks/paypal", bytes.NewReader([]byte(`{}`)),
	)
	setRequiredPayPalHeaders(c.Request.Header)

	handler.Handle(c)

	assert.Equal(t, http.StatusInternalServerError, c.Writer.Status())
}

func TestPayPalWebhookHandler_VerifiedMalformedCaptureResourceReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	realProvider := paypalprovider.NewProvider(paypalprovider.ProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		WebhookID:    "webhook-1",
		HTTPClient:   http.DefaultClient,
		Logger:       logging.NewLogger("error"),
	})
	provider := &webhookPaymentProvider{
		name: "paypal",
		verifyWebhookFunc: func(_ context.Context, _ http.Header, _ []byte) error {
			return nil
		},
		handleWebhookFunc: realProvider.HandleWebhook,
	}
	svc := newWebhookPaymentService(t, provider, &webhookPaymentRepo{})
	handler := NewPayPalWebhookHandler(svc, slog.Default())

	payload := []byte(`{
		"id":"EVT-1",
		"event_type":"PAYMENT.CAPTURE.COMPLETED",
		"resource":"not-an-object"
	}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost, "/api/v1/webhooks/paypal", bytes.NewReader(payload),
	)
	setRequiredPayPalHeaders(c.Request.Header)

	handler.Handle(c)

	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}

func setRequiredPayPalHeaders(headers http.Header) {
	headers.Set("Paypal-Auth-Algo", "SHA256withRSA")
	headers.Set("Paypal-Cert-Url", "https://paypal.test/cert.pem")
	headers.Set("Paypal-Transmission-Id", "txn-1")
	headers.Set("Paypal-Transmission-Sig", "sig-1")
	headers.Set("Paypal-Transmission-Time", "2026-01-01T00:00:00Z")
}
