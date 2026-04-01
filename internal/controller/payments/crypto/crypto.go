// Package crypto provides cryptocurrency payment provider implementations.
// Only one crypto provider can be active at a time, selected via config.
package crypto

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
)

// CryptoProvider extends the base PaymentProvider with crypto-specific methods.
// Both BTCPay and NOWPayments implement this interface.
type CryptoProvider interface {
	// CreatePaymentSession creates a crypto payment invoice/session.
	CreatePaymentSession(
		ctx context.Context,
		req *CreatePaymentRequest,
	) (*PaymentSession, error)

	// HandleWebhook verifies and processes a crypto payment webhook.
	HandleWebhook(
		ctx context.Context,
		headers http.Header,
		body []byte,
	) (*WebhookResult, error)

	// GetPaymentStatus checks the current status of a crypto payment.
	GetPaymentStatus(
		ctx context.Context,
		gatewayPaymentID string,
	) (*PaymentStatus, error)

	// ProviderName returns the crypto provider identifier.
	ProviderName() string
}

// CreatePaymentRequest holds the parameters for creating a crypto payment.
type CreatePaymentRequest struct {
	AccountID   string
	PaymentID   string
	AmountCents int64
	Currency    string
	Description string
	RedirectURL string
}

// PaymentSession is returned after creating a crypto payment session.
type PaymentSession struct {
	GatewayPaymentID string
	CheckoutURL      string
	ExpiresAt        time.Time
}

// PaymentStatus holds the current status of a crypto payment.
type PaymentStatus struct {
	Status      string
	AmountCents int64
	Currency    string
}

// WebhookResult holds the outcome of processing a crypto webhook.
type WebhookResult struct {
	EventType      string
	InvoiceID      string
	AccountID      string
	AmountCents    int64
	Currency       string
	Status         string
	IdempotencyKey string
}

// FactoryConfig holds configuration for the crypto provider factory.
type FactoryConfig struct {
	Provider string // "btcpay", "nowpayments", or "disabled"

	BTCPayServerURL     string
	BTCPayAPIKey        string
	BTCPayStoreID       string
	BTCPayWebhookSecret string

	NOWPaymentsAPIKey      string
	NOWPaymentsIPNSecret   string
	NOWPaymentsCallbackURL string

	RedirectURL string
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

// NewProvider creates the configured crypto provider based on FactoryConfig.
// Returns nil if the provider is "disabled" or empty.
func NewProvider(cfg FactoryConfig) (CryptoProvider, error) {
	switch cfg.Provider {
	case "disabled", "":
		return nil, nil
	case "btcpay":
		return NewBTCPayProvider(BTCPayConfig{
			ServerURL:     cfg.BTCPayServerURL,
			APIKey:        cfg.BTCPayAPIKey,
			StoreID:       cfg.BTCPayStoreID,
			WebhookSecret: cfg.BTCPayWebhookSecret,
			RedirectURL:   cfg.RedirectURL,
			HTTPClient:    cfg.HTTPClient,
			Logger:        cfg.Logger,
		})
	case "nowpayments":
		return NewNOWPaymentsProvider(NOWPaymentsConfig{
			APIKey:      cfg.NOWPaymentsAPIKey,
			IPNSecret:   cfg.NOWPaymentsIPNSecret,
			CallbackURL: cfg.NOWPaymentsCallbackURL,
			RedirectURL: cfg.RedirectURL,
			HTTPClient:  cfg.HTTPClient,
			Logger:      cfg.Logger,
		})
	default:
		return nil, fmt.Errorf(
			"unknown crypto provider %q (valid: btcpay, nowpayments, disabled)",
			cfg.Provider,
		)
	}
}

// Adapter wraps a CryptoProvider to satisfy the payments.PaymentProvider interface,
// enabling registration in the PaymentRegistry for payment session creation.
type Adapter struct {
	provider CryptoProvider
}

// NewAdapter wraps a CryptoProvider as a payments.PaymentProvider.
func NewAdapter(provider CryptoProvider) *Adapter {
	return &Adapter{provider: provider}
}

// Name returns the provider identifier for the registry.
func (a *Adapter) Name() string {
	return "crypto"
}

// CreatePaymentSession adapts a PaymentRequest to the crypto provider.
func (a *Adapter) CreatePaymentSession(
	ctx context.Context, req payments.PaymentRequest,
) (*payments.PaymentSession, error) {
	session, err := a.provider.CreatePaymentSession(ctx, &CreatePaymentRequest{
		AccountID:   req.CustomerID,
		PaymentID:   req.Metadata["payment_id"],
		AmountCents: req.AmountCents,
		Currency:    req.Currency,
		Description: req.Description,
		RedirectURL: req.ReturnURL,
	})
	if err != nil {
		return nil, err
	}
	return &payments.PaymentSession{
		GatewaySessionID: session.GatewayPaymentID,
		PaymentURL:       session.CheckoutURL,
	}, nil
}

// HandleWebhook is not used for crypto — webhooks go through CryptoWebhookHandler.
func (a *Adapter) HandleWebhook(
	_ context.Context, _ []byte, _ string,
) (*payments.WebhookEvent, error) {
	return nil, fmt.Errorf("crypto webhooks use dedicated handler, not PaymentProvider.HandleWebhook")
}

// GetPaymentStatus adapts to the crypto provider's status check.
func (a *Adapter) GetPaymentStatus(
	ctx context.Context, gatewayPaymentID string,
) (*payments.PaymentStatus, error) {
	status, err := a.provider.GetPaymentStatus(ctx, gatewayPaymentID)
	if err != nil {
		return nil, err
	}
	return &payments.PaymentStatus{
		GatewayPaymentID: gatewayPaymentID,
		Status:           status.Status,
		AmountCents:      status.AmountCents,
		Currency:         status.Currency,
	}, nil
}

// RefundPayment is not supported for crypto payments.
func (a *Adapter) RefundPayment(
	_ context.Context, _ string, _ int64, _ string,
) (*payments.RefundResult, error) {
	return nil, fmt.Errorf("refunds are not supported for cryptocurrency payments")
}

// ValidateConfig is already validated during factory construction.
func (a *Adapter) ValidateConfig() error {
	return nil
}
