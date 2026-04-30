package stripe

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/refund"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
)

var _ payments.PaymentProvider = (*Provider)(nil)

// ProviderConfig holds the Stripe provider configuration.
type ProviderConfig struct {
	SecretKey      string
	WebhookSecret  string
	PublishableKey string
	Logger         *slog.Logger
}

// Provider implements PaymentProvider for Stripe.
type Provider struct {
	secretKey      string
	webhookSecret  string
	publishableKey string
	logger         *slog.Logger
}

// NewProvider creates a new Stripe PaymentProvider.
func NewProvider(cfg ProviderConfig) *Provider {
	stripe.Key = cfg.SecretKey
	return &Provider{
		secretKey:      cfg.SecretKey,
		webhookSecret:  cfg.WebhookSecret,
		publishableKey: cfg.PublishableKey,
		logger:         cfg.Logger.With("component", "stripe-provider"),
	}
}

// Name returns "stripe".
func (p *Provider) Name() string { return "stripe" }

// CreatePaymentSession creates a Stripe Checkout Session in one-time payment mode.
func (p *Provider) CreatePaymentSession(
	_ context.Context, req payments.PaymentRequest,
) (*payments.PaymentSession, error) {
	params := p.buildCheckoutParams(req)

	sess, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("create stripe checkout session: %w", err)
	}

	p.logger.Info("stripe checkout session created",
		"session_id", sess.ID,
		"customer_id", req.CustomerID,
		"amount_cents", req.AmountCents,
	)

	return &payments.PaymentSession{
		ID:               sess.ID,
		GatewaySessionID: sess.ID,
		PaymentURL:       sess.URL,
	}, nil
}

func (p *Provider) buildCheckoutParams(req payments.PaymentRequest) *stripe.CheckoutSessionParams {
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(req.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String("Credit Top-Up"),
						Description: stripe.String(req.Description),
					},
					UnitAmount: stripe.Int64(req.AmountCents),
				},
				Quantity: stripe.Int64(1),
			},
		},
		CustomerEmail: stripe.String(req.CustomerEmail),
		SuccessURL:    stripe.String(req.ReturnURL + "?status=success"),
		CancelURL:     stripe.String(req.CancelURL),
	}

	params.Metadata = make(map[string]string)
	params.Metadata["customer_id"] = req.CustomerID
	params.Metadata["amount_cents"] = fmt.Sprintf("%d", req.AmountCents)
	for k, v := range req.Metadata {
		params.Metadata[k] = v
	}

	params.PaymentIntentData = &stripe.CheckoutSessionPaymentIntentDataParams{
		Metadata: make(map[string]string, len(params.Metadata)),
	}
	for k, v := range params.Metadata {
		params.PaymentIntentData.Metadata[k] = v
	}

	return params
}

// HandleWebhook verifies the Stripe webhook signature and parses the event.
func (p *Provider) HandleWebhook(
	_ context.Context, payload []byte, signature string,
) (*payments.WebhookEvent, error) {
	event, err := webhook.ConstructEventWithOptions(
		payload, signature, p.webhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"verify stripe webhook signature: %w: %w",
			payments.ErrWebhookVerification,
			err,
		)
	}

	idempotencyKey := fmt.Sprintf("stripe:event:%s", event.ID)

	switch event.Type {
	case "checkout.session.completed":
		return p.handleCheckoutCompleted(&event, idempotencyKey)
	case "payment_intent.succeeded":
		return p.handlePaymentIntentSucceeded(&event, idempotencyKey)
	default:
		p.logger.Debug("ignoring unhandled stripe event type",
			"event_type", event.Type,
			"event_id", event.ID,
		)
		return nil, nil
	}
}

func (p *Provider) handleCheckoutCompleted(
	event *stripe.Event, idempotencyKey string,
) (*payments.WebhookEvent, error) {
	var sess stripe.CheckoutSession
	if err := parseStripeObject(event.Data.Raw, &sess); err != nil {
		return nil, fmt.Errorf("parse checkout session: %w", err)
	}

	if sess.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		return nil, nil
	}

	return &payments.WebhookEvent{
		Type:           payments.WebhookEventPaymentCompleted,
		GatewayEventID: event.ID,
		PaymentID:      sess.PaymentIntent.ID,
		AmountCents:    sess.AmountTotal,
		Currency:       string(sess.Currency),
		Status:         "completed",
		IdempotencyKey: idempotencyKey,
		Metadata:       sess.Metadata,
	}, nil
}

func (p *Provider) handlePaymentIntentSucceeded(
	event *stripe.Event, idempotencyKey string,
) (*payments.WebhookEvent, error) {
	var pi stripe.PaymentIntent
	if err := parseStripeObject(event.Data.Raw, &pi); err != nil {
		return nil, fmt.Errorf("parse payment intent: %w", err)
	}

	return &payments.WebhookEvent{
		Type:           payments.WebhookEventPaymentCompleted,
		GatewayEventID: event.ID,
		PaymentID:      pi.ID,
		AmountCents:    pi.Amount,
		Currency:       string(pi.Currency),
		Status:         "completed",
		IdempotencyKey: idempotencyKey,
		Metadata:       pi.Metadata,
	}, nil
}

// GetPaymentStatus retrieves the current status of a payment intent.
func (p *Provider) GetPaymentStatus(
	_ context.Context, gatewayPaymentID string,
) (*payments.PaymentStatus, error) {
	pi, err := paymentintent.Get(gatewayPaymentID, nil)
	if err != nil {
		return nil, fmt.Errorf("get stripe payment status: %w", err)
	}

	status := mapStripeStatus(pi.Status)
	var paidAt *time.Time
	if pi.Status == stripe.PaymentIntentStatusSucceeded {
		t := time.Unix(pi.Created, 0)
		paidAt = &t
	}

	return &payments.PaymentStatus{
		GatewayPaymentID: pi.ID,
		Status:           status,
		AmountCents:      pi.Amount,
		Currency:         string(pi.Currency),
		PaidAt:           paidAt,
	}, nil
}

// RefundPayment issues a refund via the Stripe Refunds API.
func (p *Provider) RefundPayment(
	_ context.Context, gatewayPaymentID string, amountCents int64, _ string,
) (*payments.RefundResult, error) {
	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(gatewayPaymentID),
		Amount:        stripe.Int64(amountCents),
	}

	r, err := refund.New(params)
	if err != nil {
		return nil, fmt.Errorf("create stripe refund: %w", err)
	}

	p.logger.Info("stripe refund created",
		"refund_id", r.ID,
		"payment_id", gatewayPaymentID,
		"amount_cents", amountCents,
	)

	return &payments.RefundResult{
		GatewayRefundID:  r.ID,
		GatewayPaymentID: gatewayPaymentID,
		AmountCents:      r.Amount,
		Currency:         string(r.Currency),
		Status:           string(r.Status),
	}, nil
}

// ValidateConfig checks that required Stripe configuration is present.
func (p *Provider) ValidateConfig() error {
	if p.secretKey == "" {
		return fmt.Errorf("stripe secret key is required")
	}
	if p.webhookSecret == "" {
		return fmt.Errorf("stripe webhook secret is required")
	}
	return nil
}

func mapStripeStatus(s stripe.PaymentIntentStatus) string {
	switch s {
	case stripe.PaymentIntentStatusSucceeded:
		return "completed"
	case stripe.PaymentIntentStatusCanceled:
		return "failed"
	case stripe.PaymentIntentStatusRequiresPaymentMethod,
		stripe.PaymentIntentStatusRequiresConfirmation,
		stripe.PaymentIntentStatusRequiresAction,
		stripe.PaymentIntentStatusProcessing:
		return "pending"
	default:
		return "unknown"
	}
}
