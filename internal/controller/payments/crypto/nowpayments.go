package crypto

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

const nowPaymentsBaseURL = "https://api.nowpayments.io/v1"

// NOWPaymentsConfig holds configuration for the NOWPayments provider.
type NOWPaymentsConfig struct {
	APIKey      string // NOWPayments API key
	IPNSecret   string // IPN callback HMAC secret
	CallbackURL string // IPN callback URL
	RedirectURL string // Redirect URL after payment
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

// NOWPaymentsProvider implements CryptoProvider using NOWPayments API v1.
type NOWPaymentsProvider struct {
	apiKey      string
	ipnSecret   string
	callbackURL string
	redirectURL string
	baseURL     string // overridable for testing
	httpClient  *http.Client
	logger      *slog.Logger
}

// NewNOWPaymentsProvider creates a NOWPayments payment provider.
func NewNOWPaymentsProvider(cfg NOWPaymentsConfig) (*NOWPaymentsProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("nowpayments API key is required")
	}
	if cfg.IPNSecret == "" {
		return nil, fmt.Errorf("nowpayments IPN secret is required")
	}
	return &NOWPaymentsProvider{
		apiKey:      cfg.APIKey,
		ipnSecret:   cfg.IPNSecret,
		callbackURL: cfg.CallbackURL,
		redirectURL: cfg.RedirectURL,
		baseURL:     nowPaymentsBaseURL,
		httpClient:  cfg.HTTPClient,
		logger:      cfg.Logger.With("component", "nowpayments-provider"),
	}, nil
}

// ProviderName returns the provider identifier.
func (p *NOWPaymentsProvider) ProviderName() string {
	return "nowpayments"
}

// --- NOWPayments API types ---

type nowPaymentResponse struct {
	ID            string  `json:"id"`
	PaymentID     string  `json:"payment_id"`
	PaymentStatus string  `json:"payment_status"`
	PayAddress    string  `json:"pay_address"`
	PriceAmount   float64 `json:"price_amount"`
	PriceCurrency string  `json:"price_currency"`
	PayAmount     float64 `json:"pay_amount"`
	PayCurrency   string  `json:"pay_currency"`
	OrderID       string  `json:"order_id"`
	InvoiceURL    string  `json:"invoice_url"`
	CreatedAt     string  `json:"created_at"`
}

type nowInvoiceRequest struct {
	PriceAmount   float64 `json:"price_amount"`
	PriceCurrency string  `json:"price_currency"`
	OrderID       string  `json:"order_id,omitempty"`
	IPNURL        string  `json:"ipn_callback_url,omitempty"`
	SuccessURL    string  `json:"success_url,omitempty"`
	CancelURL     string  `json:"cancel_url,omitempty"`
}

type nowInvoiceResponse struct {
	ID         string `json:"id"`
	InvoiceURL string `json:"invoice_url"`
	OrderID    string `json:"order_id"`
	CreatedAt  string `json:"created_at"`
}

type nowIPNPayload struct {
	PaymentID        json.Number `json:"payment_id"`
	PaymentStatus    string      `json:"payment_status"`
	PayAddress       string      `json:"pay_address"`
	PriceAmount      float64     `json:"price_amount"`
	PriceCurrency    string      `json:"price_currency"`
	PayAmount        float64     `json:"pay_amount"`
	PayCurrency      string      `json:"pay_currency"`
	OrderID          string      `json:"order_id"`
	OrderDescription string      `json:"order_description"`
	OutcomeAmount    float64     `json:"outcome_amount"`
	OutcomeCurrency  string      `json:"outcome_currency"`
}

// CreatePaymentSession creates a NOWPayments invoice and returns the payment URL.
func (p *NOWPaymentsProvider) CreatePaymentSession(
	ctx context.Context,
	req *CreatePaymentRequest,
) (*PaymentSession, error) {
	priceAmount := float64(req.AmountCents) / 100.0

	invoiceReq := nowInvoiceRequest{
		PriceAmount:   priceAmount,
		PriceCurrency: strings.ToLower(req.Currency),
		OrderID:       req.AccountID + ":" + req.PaymentID,
		IPNURL:        p.callbackURL,
		SuccessURL:    p.redirectURL,
		CancelURL:     p.redirectURL,
	}

	body, err := json.Marshal(invoiceReq)
	if err != nil {
		return nil, fmt.Errorf("marshal nowpayments request: %w", err)
	}

	endpoint := p.baseURL + "/invoice"
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint,
		strings.NewReader(string(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("build nowpayments request: %w", err)
	}
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("nowpayments create invoice: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read nowpayments response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"nowpayments create invoice failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var invoice nowInvoiceResponse
	if err := json.Unmarshal(respBody, &invoice); err != nil {
		return nil, fmt.Errorf("decode nowpayments response: %w", err)
	}

	p.logger.Info("nowpayments invoice created",
		"invoice_id", invoice.ID,
		"payment_id", req.PaymentID,
	)

	return &PaymentSession{
		GatewayPaymentID: invoice.ID,
		CheckoutURL:      invoice.InvoiceURL,
		ExpiresAt:        time.Now().Add(20 * time.Minute),
	}, nil
}

// GetPaymentStatus retrieves the current status of a NOWPayments payment.
func (p *NOWPaymentsProvider) GetPaymentStatus(
	ctx context.Context,
	paymentID string,
) (*PaymentStatus, error) {
	endpoint := p.baseURL + "/payment/" + paymentID
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, endpoint, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build nowpayments status request: %w", err)
	}
	httpReq.Header.Set("x-api-key", p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("nowpayments get payment: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read nowpayments response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"nowpayments get payment failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var payment nowPaymentResponse
	if err := json.Unmarshal(respBody, &payment); err != nil {
		return nil, fmt.Errorf("decode nowpayments response: %w", err)
	}

	amountCents := int64(math.Round(payment.PriceAmount * 100))

	return &PaymentStatus{
		Status:      payment.PaymentStatus,
		AmountCents: amountCents,
		Currency:    strings.ToUpper(payment.PriceCurrency),
	}, nil
}

// HandleWebhook verifies the IPN HMAC signature and processes
// NOWPayments IPN callbacks. Only processes "finished" payments.
func (p *NOWPaymentsProvider) HandleWebhook(
	_ context.Context,
	headers http.Header,
	body []byte,
) (*WebhookResult, error) {
	sig := headers.Get("X-Nowpayments-Sig")
	if err := verifyNOWPaymentsSignature(p.ipnSecret, sig, body); err != nil {
		return nil, fmt.Errorf(
			"nowpayments IPN signature invalid: %w: %w",
			ErrWebhookVerification, err,
		)
	}

	var payload nowIPNPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf(
			"parse nowpayments IPN: %w: %w",
			ErrWebhookValidation, err,
		)
	}

	p.logger.Info("nowpayments IPN received",
		"payment_id", payload.PaymentID,
		"status", payload.PaymentStatus,
		"order_id", payload.OrderID,
	)

	if payload.PaymentStatus != "finished" {
		p.logger.Debug("ignoring non-finished payment",
			"status", payload.PaymentStatus)
		return nil, nil
	}

	paymentIDStr := payload.PaymentID.String()
	if paymentIDStr == "" {
		return nil, fmt.Errorf(
			"nowpayments finished IPN missing payment_id: %w",
			ErrWebhookValidation,
		)
	}
	accountID, localPaymentID := parseNOWPaymentsOrderID(payload.OrderID)
	if accountID == "" || localPaymentID == "" {
		return nil, fmt.Errorf(
			"nowpayments IPN missing order reference: %w",
			ErrWebhookValidation,
		)
	}
	amountCents := int64(math.Round(payload.PriceAmount * 100))

	return &WebhookResult{
		EventType:         "payment.finished",
		InvoiceID:         localPaymentID,
		ExternalPaymentID: paymentIDStr,
		AccountID:         accountID,
		AmountCents:       amountCents,
		Currency:          strings.ToUpper(payload.PriceCurrency),
		Status:            payload.PaymentStatus,
		IdempotencyKey:    "nowpayments:payment:" + paymentIDStr,
	}, nil
}

func parseNOWPaymentsOrderID(orderID string) (accountID, paymentID string) {
	parts := strings.SplitN(orderID, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
