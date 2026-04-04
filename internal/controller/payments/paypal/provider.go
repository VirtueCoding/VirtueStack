package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
)

// compile-time check that Provider implements PaymentProvider.
var _ payments.PaymentProvider = (*Provider)(nil)

// ProviderConfig holds the configuration for the PayPal payment provider.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	Mode         string // "sandbox" or "production"
	WebhookID    string
	ReturnURL    string
	CancelURL    string
	HTTPClient   *http.Client
	Logger       *slog.Logger
}

// Provider implements PaymentProvider using the PayPal Orders API v2.
type Provider struct {
	tokenClient *TokenClient
	webhookID   string
	returnURL   string
	cancelURL   string
	httpClient  *http.Client
	logger      *slog.Logger
}

// NewProvider creates a PayPal PaymentProvider.
func NewProvider(cfg ProviderConfig) *Provider {
	tc := NewTokenClient(
		cfg.ClientID, cfg.ClientSecret, cfg.Mode,
		cfg.HTTPClient, cfg.Logger,
	)
	return &Provider{
		tokenClient: tc,
		webhookID:   cfg.WebhookID,
		returnURL:   cfg.ReturnURL,
		cancelURL:   cfg.CancelURL,
		httpClient:  cfg.HTTPClient,
		logger:      cfg.Logger.With("component", "paypal-provider"),
	}
}

// Name returns "paypal".
func (p *Provider) Name() string { return "paypal" }

// CreatePaymentSession creates a PayPal order and returns the approval URL.
func (p *Provider) CreatePaymentSession(
	ctx context.Context, req payments.PaymentRequest,
) (*payments.PaymentSession, error) {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get paypal token: %w", err)
	}

	orderReq := p.buildOrderRequest(req)
	return p.executeCreateOrder(ctx, token, orderReq, req.Metadata)
}

func (p *Provider) buildOrderRequest(req payments.PaymentRequest) orderRequest {
	return orderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []purchaseUnit{{
			ReferenceID: req.Metadata["payment_id"],
			Description: req.Description,
			CustomID:    req.CustomerID,
			Amount: &amount{
				CurrencyCode: strings.ToUpper(req.Currency),
				Value:        centsToDecimal(req.AmountCents),
			},
		}},
		PaymentSource: &paymentSource{
			PayPal: &paypalSource{
				ExperienceContext: &experienceContext{
					ReturnURL: p.returnURL,
					CancelURL: p.cancelURL,
				},
			},
		},
	}
}

func (p *Provider) executeCreateOrder(
	ctx context.Context, token string,
	orderReq orderRequest, metadata map[string]string,
) (*payments.PaymentSession, error) {
	body, err := json.Marshal(orderReq)
	if err != nil {
		return nil, fmt.Errorf("marshal order request: %w", err)
	}

	endpoint := p.tokenClient.BaseURL() + "/v2/checkout/orders"
	resp, err := p.doAuthedRequest(ctx, http.MethodPost, endpoint, token, body)
	if err != nil {
		return nil, fmt.Errorf("paypal create order: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			p.logger.Debug("failed to close PayPal create-order response body", "error", closeErr)
		}
	}()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"paypal create order failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var order orderResponse
	if err := json.Unmarshal(respBody, &order); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}

	approvalURL := findApprovalURL(order.Links)
	if approvalURL == "" {
		return nil, fmt.Errorf("paypal order missing approval link")
	}

	p.logger.Info("paypal order created",
		"order_id", order.ID,
		"customer_id", orderReq.PurchaseUnits[0].CustomID,
	)

	return &payments.PaymentSession{
		ID:               order.ID,
		GatewaySessionID: order.ID,
		PaymentURL:       approvalURL,
	}, nil
}

// HandleWebhook verifies and processes an inbound PayPal webhook.
func (p *Provider) HandleWebhook(
	ctx context.Context, payload []byte, _ string,
) (*payments.WebhookEvent, error) {
	event, err := ParseWebhookEvent(payload)
	if err != nil {
		return nil, fmt.Errorf("parse paypal webhook event: %w", err)
	}

	p.logger.Info("paypal webhook received",
		"event_type", event.EventType,
		"event_id", event.ID,
	)

	if event.EventType != "PAYMENT.CAPTURE.COMPLETED" {
		p.logger.Debug("ignoring non-capture event",
			"event_type", event.EventType)
		return nil, nil
	}

	return p.processCaptureEvent(event)
}

func (p *Provider) processCaptureEvent(
	event *WebhookEvent,
) (*payments.WebhookEvent, error) {
	resource, err := ParseWebhookResource(event.Resource)
	if err != nil {
		return nil, fmt.Errorf("parse capture resource: %w", err)
	}

	if resource.Status != "COMPLETED" {
		p.logger.Warn("capture not completed",
			"capture_id", resource.ID, "status", resource.Status)
		return nil, nil
	}

	cents, currency, err := extractAmount(resource)
	if err != nil {
		return nil, err
	}

	orderID := extractOrderID(resource)
	metadata := map[string]string{"customer_id": resource.CustomID}
	if orderID != "" {
		metadata["payment_id"] = orderID
	}

	return &payments.WebhookEvent{
		Type:           payments.WebhookEventPaymentCompleted,
		GatewayEventID: event.ID,
		PaymentID:      resource.ID,
		AmountCents:    cents,
		Currency:       currency,
		Status:         "completed",
		IdempotencyKey: "paypal:capture:" + resource.ID,
		Metadata:       metadata,
	}, nil
}

// GetPaymentStatus retrieves the current status of a PayPal order.
func (p *Provider) GetPaymentStatus(
	ctx context.Context, orderID string,
) (*payments.PaymentStatus, error) {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get paypal token: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/v2/checkout/orders/%s",
		p.tokenClient.BaseURL(), orderID,
	)
	resp, err := p.doAuthedRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return nil, fmt.Errorf("paypal get order: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			p.logger.Debug("failed to close PayPal get-order response body", "error", closeErr)
		}
	}()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"paypal get order failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var order orderResponse
	if err := json.Unmarshal(respBody, &order); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}

	var paidAt *time.Time
	if order.Status == "COMPLETED" {
		now := time.Now()
		paidAt = &now
	}

	return &payments.PaymentStatus{
		GatewayPaymentID: order.ID,
		Status:           mapPayPalStatus(order.Status),
		PaidAt:           paidAt,
	}, nil
}

// RefundPayment issues a refund for a captured PayPal payment.
func (p *Provider) RefundPayment(
	ctx context.Context, captureID string, amountCents int64, currency string,
) (*payments.RefundResult, error) {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get paypal token: %w", err)
	}

	return p.executeRefund(ctx, token, captureID, amountCents, currency)
}

func (p *Provider) executeRefund(
	ctx context.Context, token, captureID string, amountCents int64, currency string,
) (*payments.RefundResult, error) {
	endpoint := fmt.Sprintf(
		"%s/v2/payments/captures/%s/refund",
		p.tokenClient.BaseURL(), captureID,
	)

	refundReq := refundRequest{
		Amount: &amount{
			CurrencyCode: strings.ToUpper(currency),
			Value:        centsToDecimal(amountCents),
		},
	}
	body, err := json.Marshal(refundReq)
	if err != nil {
		return nil, fmt.Errorf("marshal refund request: %w", err)
	}

	resp, err := p.doAuthedRequest(ctx, http.MethodPost, endpoint, token, body)
	if err != nil {
		return nil, fmt.Errorf("paypal refund: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			p.logger.Debug("failed to close PayPal refund response body", "error", closeErr)
		}
	}()

	return p.parseRefundResponse(resp, captureID, amountCents, currency)
}

func (p *Provider) parseRefundResponse(
	resp *http.Response, captureID string, amountCents int64, currency string,
) (*payments.RefundResult, error) {
	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"paypal refund failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var refResp refundResponse
	if err := json.Unmarshal(respBody, &refResp); err != nil {
		return nil, fmt.Errorf("decode refund response: %w", err)
	}

	p.logger.Info("paypal refund created",
		"refund_id", refResp.ID,
		"capture_id", captureID,
		"amount_cents", amountCents,
	)

	return &payments.RefundResult{
		GatewayRefundID:  refResp.ID,
		GatewayPaymentID: captureID,
		AmountCents:      amountCents,
		Currency:         strings.ToUpper(currency),
		Status:           strings.ToLower(refResp.Status),
	}, nil
}

// ValidateConfig checks that required PayPal configuration is present.
func (p *Provider) ValidateConfig() error {
	if p.tokenClient.clientID == "" {
		return fmt.Errorf("paypal client_id is required")
	}
	if p.tokenClient.clientSecret == "" {
		return fmt.Errorf("paypal client_secret is required")
	}
	if p.returnURL == "" {
		return fmt.Errorf("paypal return_url is required")
	}
	if p.cancelURL == "" {
		return fmt.Errorf("paypal cancel_url is required")
	}
	return nil
}

// CaptureOrder finalizes payment for an approved PayPal order.
// Returns capture ID, status, currency, and amount in cents.
func (p *Provider) CaptureOrder(
	ctx context.Context, orderID string,
) (captureID, status, currency string, amountCents int64, err error) {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("get paypal token: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/v2/checkout/orders/%s/capture",
		p.tokenClient.BaseURL(), orderID,
	)
	resp, err := p.doAuthedRequest(ctx, http.MethodPost, endpoint, token, nil)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("paypal capture order: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			p.logger.Debug("failed to close PayPal capture response body", "error", closeErr)
		}
	}()

	result, parseErr := p.parseCaptureResponse(resp, orderID)
	if parseErr != nil {
		return "", "", "", 0, parseErr
	}
	return result.CaptureID, result.Status, result.Currency, result.AmountCents, nil
}

func (p *Provider) parseCaptureResponse(
	resp *http.Response, orderID string,
) (*CaptureResult, error) {
	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"paypal capture failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var capture captureResponse
	if err := json.Unmarshal(respBody, &capture); err != nil {
		return nil, fmt.Errorf("decode capture response: %w", err)
	}

	if len(capture.PurchaseUnits) == 0 ||
		capture.PurchaseUnits[0].Payments == nil ||
		len(capture.PurchaseUnits[0].Payments.Captures) == 0 {
		return nil, fmt.Errorf("paypal capture response missing capture detail")
	}

	cap := capture.PurchaseUnits[0].Payments.Captures[0]
	cents, err := decimalToCents(cap.Amount.Value)
	if err != nil {
		return nil, fmt.Errorf("parse capture amount: %w", err)
	}

	p.logger.Info("paypal order captured",
		"order_id", orderID,
		"capture_id", cap.ID,
		"status", cap.Status,
	)

	return &CaptureResult{
		CaptureID:   cap.ID,
		Status:      cap.Status,
		AmountCents: cents,
		Currency:    cap.Amount.CurrencyCode,
	}, nil
}

// doAuthedRequest builds and executes an authenticated PayPal API request.
func (p *Provider) doAuthedRequest(
	ctx context.Context, method, endpoint, token string, body []byte,
) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Prefer", "return=representation")
	}

	return p.httpClient.Do(req)
}

func readLimitedBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return data, nil
}
