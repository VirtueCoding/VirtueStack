package payments

import (
	"context"
	"time"
)

// PaymentProvider defines the interface that each payment gateway must implement.
// Each gateway (Stripe, PayPal, crypto) provides its own implementation.
type PaymentProvider interface {
	// Name returns the provider identifier (e.g., "stripe", "paypal").
	Name() string

	// CreatePaymentSession initiates a payment session with the gateway.
	// Returns a session with a redirect URL for the customer.
	CreatePaymentSession(
		ctx context.Context, req PaymentRequest,
	) (*PaymentSession, error)

	// HandleWebhook processes an incoming webhook from the gateway.
	// Verifies the signature and returns a parsed event.
	HandleWebhook(
		ctx context.Context, payload []byte, signature string,
	) (*WebhookEvent, error)

	// GetPaymentStatus queries the gateway for the current payment status.
	GetPaymentStatus(
		ctx context.Context, gatewayPaymentID string,
	) (*PaymentStatus, error)

	// RefundPayment issues a full or partial refund for a completed payment.
	// Amount is in cents (minor currency units). Currency is the original payment currency.
	RefundPayment(
		ctx context.Context, gatewayPaymentID string, amountCents int64, currency string,
	) (*RefundResult, error)

	// ValidateConfig checks that all required configuration is present.
	ValidateConfig() error
}

// PaymentRequest holds the parameters for creating a payment session.
type PaymentRequest struct {
	CustomerID    string            `json:"customer_id"`
	CustomerEmail string            `json:"customer_email"`
	AmountCents   int64             `json:"amount_cents"`
	Currency      string            `json:"currency"`
	Description   string            `json:"description"`
	ReturnURL     string            `json:"return_url"`
	CancelURL     string            `json:"cancel_url"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// PaymentSession is returned by CreatePaymentSession with the redirect URL.
type PaymentSession struct {
	ID               string `json:"id"`
	GatewaySessionID string `json:"gateway_session_id"`
	PaymentURL       string `json:"payment_url"`
}

// WebhookEventType enumerates the webhook event types we handle.
type WebhookEventType string

const (
	WebhookEventPaymentCompleted WebhookEventType = "payment.completed"
	WebhookEventPaymentFailed    WebhookEventType = "payment.failed"
	WebhookEventRefundCompleted  WebhookEventType = "refund.completed"
)

// WebhookEvent is the normalized representation of a gateway webhook.
type WebhookEvent struct {
	Type           WebhookEventType  `json:"type"`
	GatewayEventID string            `json:"gateway_event_id"`
	PaymentID      string            `json:"payment_id"`
	AmountCents    int64             `json:"amount_cents"`
	Currency       string            `json:"currency"`
	Status         string            `json:"status"`
	IdempotencyKey string            `json:"idempotency_key"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// PaymentStatus represents the current state of a payment at the gateway.
type PaymentStatus struct {
	GatewayPaymentID string     `json:"gateway_payment_id"`
	Status           string     `json:"status"`
	AmountCents      int64      `json:"amount_cents"`
	Currency         string     `json:"currency"`
	PaidAt           *time.Time `json:"paid_at,omitempty"`
}

// RefundResult contains the result of a refund operation.
type RefundResult struct {
	GatewayRefundID  string `json:"gateway_refund_id"`
	GatewayPaymentID string `json:"gateway_payment_id"`
	AmountCents      int64  `json:"amount_cents"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
}
