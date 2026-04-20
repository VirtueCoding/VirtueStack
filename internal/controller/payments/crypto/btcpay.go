package crypto

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// BTCPayConfig holds configuration for the BTCPay Server provider.
type BTCPayConfig struct {
	ServerURL     string // BTCPay Server base URL (e.g., https://btcpay.example.com)
	APIKey        string // Greenfield API key
	StoreID       string // BTCPay store ID
	WebhookSecret string // HMAC-SHA256 webhook secret
	RedirectURL   string // Redirect URL after payment
	HTTPClient    *http.Client
	Logger        *slog.Logger
}

// BTCPayProvider implements CryptoProvider using BTCPay Server Greenfield API v1.
type BTCPayProvider struct {
	serverURL     string
	apiKey        string
	storeID       string
	webhookSecret string
	redirectURL   string
	httpClient    *http.Client
	logger        *slog.Logger
}

// NewBTCPayProvider creates a BTCPay Server payment provider.
func NewBTCPayProvider(cfg BTCPayConfig) (*BTCPayProvider, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("btcpay server URL is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("btcpay API key is required")
	}
	if cfg.StoreID == "" {
		return nil, fmt.Errorf("btcpay store ID is required")
	}
	return &BTCPayProvider{
		serverURL:     strings.TrimRight(cfg.ServerURL, "/"),
		apiKey:        cfg.APIKey,
		storeID:       cfg.StoreID,
		webhookSecret: cfg.WebhookSecret,
		redirectURL:   cfg.RedirectURL,
		httpClient:    cfg.HTTPClient,
		logger:        cfg.Logger.With("component", "btcpay-provider"),
	}, nil
}

// ProviderName returns the provider identifier.
func (p *BTCPayProvider) ProviderName() string {
	return "btcpay"
}

// --- BTCPay Greenfield API types ---

type btcpayInvoiceRequest struct {
	Amount   string            `json:"amount"`
	Currency string            `json:"currency"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Checkout *btcpayCheckout   `json:"checkout,omitempty"`
}

type btcpayCheckout struct {
	RedirectURL           string `json:"redirectURL,omitempty"`
	RedirectAutomatically bool   `json:"redirectAutomatically"`
}

type btcpayInvoiceResponse struct {
	ID             string            `json:"id"`
	Status         string            `json:"status"`
	Amount         string            `json:"amount"`
	Currency       string            `json:"currency"`
	CheckoutLink   string            `json:"checkoutLink"`
	CreatedTime    int64             `json:"createdTime"`
	ExpirationTime int64             `json:"expirationTime"`
	Metadata       map[string]string `json:"metadata"`
}

type btcpayWebhookPayload struct {
	DeliveryID         string `json:"deliveryId"`
	WebhookID          string `json:"webhookId"`
	OriginalDeliveryID string `json:"originalDeliveryId"`
	IsRedelivery       bool   `json:"isRedelivery"`
	Type               string `json:"type"`
	Timestamp          int64  `json:"timestamp"`
	StoreID            string `json:"storeId"`
	InvoiceID          string `json:"invoiceId"`
}

// CreatePaymentSession creates a BTCPay invoice and returns the checkout URL.
func (p *BTCPayProvider) CreatePaymentSession(
	ctx context.Context,
	req *CreatePaymentRequest,
) (*PaymentSession, error) {
	invoiceReq := btcpayInvoiceRequest{
		Amount:   centsToDecimal(req.AmountCents),
		Currency: strings.ToUpper(req.Currency),
		Metadata: map[string]string{
			"account_id": req.AccountID,
			"payment_id": req.PaymentID,
		},
		Checkout: &btcpayCheckout{
			RedirectURL:           p.redirectURL,
			RedirectAutomatically: true,
		},
	}

	body, err := json.Marshal(invoiceReq)
	if err != nil {
		return nil, fmt.Errorf("marshal btcpay invoice request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/api/v1/stores/%s/invoices",
		p.serverURL, p.storeID,
	)
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint,
		strings.NewReader(string(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("build btcpay request: %w", err)
	}
	httpReq.Header.Set("Authorization", "token "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("btcpay create invoice: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read btcpay response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"btcpay create invoice failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var invoice btcpayInvoiceResponse
	if err := json.Unmarshal(respBody, &invoice); err != nil {
		return nil, fmt.Errorf("decode btcpay response: %w", err)
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	if invoice.ExpirationTime > 0 {
		expiresAt = time.Unix(invoice.ExpirationTime, 0)
	}

	p.logger.Info("btcpay invoice created",
		"invoice_id", invoice.ID,
		"payment_id", req.PaymentID,
	)

	return &PaymentSession{
		GatewayPaymentID: invoice.ID,
		CheckoutURL:      invoice.CheckoutLink,
		ExpiresAt:        expiresAt,
	}, nil
}

// GetPaymentStatus retrieves the current status of a BTCPay invoice.
func (p *BTCPayProvider) GetPaymentStatus(
	ctx context.Context,
	invoiceID string,
) (*PaymentStatus, error) {
	invoice, err := p.fetchInvoice(ctx, invoiceID)
	if err != nil {
		return nil, err
	}

	cents, err := decimalToCents(invoice.Amount)
	if err != nil {
		return nil, fmt.Errorf("parse btcpay amount: %w", err)
	}

	return &PaymentStatus{
		Status:      invoice.Status,
		AmountCents: cents,
		Currency:    invoice.Currency,
	}, nil
}

// HandleWebhook verifies the HMAC-SHA256 signature and processes
// BTCPay webhook events. Only processes InvoiceSettled events.
func (p *BTCPayProvider) HandleWebhook(
	ctx context.Context,
	headers http.Header,
	body []byte,
) (*WebhookResult, error) {
	sig := headers.Get("BTCPay-Sig")
	if err := verifyBTCPaySignature(p.webhookSecret, sig, body); err != nil {
		return nil, fmt.Errorf("btcpay webhook signature invalid: %w", err)
	}

	var payload btcpayWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse btcpay webhook: %w", err)
	}

	p.logger.Info("btcpay webhook received",
		"type", payload.Type,
		"invoice_id", payload.InvoiceID,
		"delivery_id", payload.DeliveryID,
	)

	if payload.Type != "InvoiceSettled" {
		p.logger.Debug("ignoring non-settled event",
			"type", payload.Type)
		return nil, nil
	}

	// Fetch the full invoice to get amount and metadata.
	invoice, err := p.fetchInvoice(ctx, payload.InvoiceID)
	if err != nil {
		return nil, fmt.Errorf("fetch btcpay invoice: %w", err)
	}

	cents, err := decimalToCents(invoice.Amount)
	if err != nil {
		return nil, fmt.Errorf("parse invoice amount: %w", err)
	}

	accountID := invoice.Metadata["account_id"]
	if accountID == "" {
		return nil, fmt.Errorf(
			"btcpay invoice %s missing account_id metadata",
			payload.InvoiceID,
		)
	}

	return &WebhookResult{
		EventType:      payload.Type,
		InvoiceID:      payload.InvoiceID,
		AccountID:      accountID,
		AmountCents:    cents,
		Currency:       invoice.Currency,
		Status:         invoice.Status,
		IdempotencyKey: "btcpay:invoice:" + payload.InvoiceID,
	}, nil
}

// fetchInvoice retrieves a BTCPay invoice by ID.
func (p *BTCPayProvider) fetchInvoice(
	ctx context.Context,
	invoiceID string,
) (*btcpayInvoiceResponse, error) {
	endpoint := fmt.Sprintf(
		"%s/api/v1/stores/%s/invoices/%s",
		p.serverURL, p.storeID, invoiceID,
	)
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, endpoint, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build btcpay request: %w", err)
	}
	httpReq.Header.Set("Authorization", "token "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("btcpay fetch invoice: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read btcpay response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"btcpay fetch invoice failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var invoice btcpayInvoiceResponse
	if err := json.Unmarshal(respBody, &invoice); err != nil {
		return nil, fmt.Errorf("decode btcpay invoice: %w", err)
	}
	return &invoice, nil
}

// centsToDecimal converts an integer cents amount to a decimal string.
func centsToDecimal(cents int64) string {
	whole := cents / 100
	frac := cents % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}

// decimalToCents converts a decimal string to integer cents.
func decimalToCents(s string) (int64, error) {
	parts := strings.SplitN(s, ".", 2)
	whole := int64(0)
	if _, err := fmt.Sscanf(parts[0], "%d", &whole); err != nil {
		return 0, fmt.Errorf("parse whole part %q: %w", parts[0], err)
	}
	frac := int64(0)
	if len(parts) == 2 {
		fracStr := parts[1]
		switch len(fracStr) {
		case 0:
			frac = 0
		case 1:
			if _, err := fmt.Sscanf(fracStr, "%d", &frac); err != nil {
				return 0, fmt.Errorf("parse frac part: %w", err)
			}
			frac *= 10
		default:
			if _, err := fmt.Sscanf(fracStr[:2], "%d", &frac); err != nil {
				return 0, fmt.Errorf("parse frac part: %w", err)
			}
		}
	}
	return whole*100 + frac, nil
}
