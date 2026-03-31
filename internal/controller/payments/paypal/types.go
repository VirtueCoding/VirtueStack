package paypal

import (
	"encoding/json"
	"fmt"
	"strings"
)

// --- PayPal API types ---

type orderRequest struct {
	Intent        string         `json:"intent"`
	PurchaseUnits []purchaseUnit `json:"purchase_units"`
	PaymentSource *paymentSource `json:"payment_source,omitempty"`
}

type purchaseUnit struct {
	ReferenceID string  `json:"reference_id,omitempty"`
	Description string  `json:"description,omitempty"`
	CustomID    string  `json:"custom_id,omitempty"`
	Amount      *amount `json:"amount"`
}

type amount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type paymentSource struct {
	PayPal *paypalSource `json:"paypal"`
}

type paypalSource struct {
	ExperienceContext *experienceContext `json:"experience_context"`
}

type experienceContext struct {
	ReturnURL string `json:"return_url"`
	CancelURL string `json:"cancel_url"`
}

type orderResponse struct {
	ID     string      `json:"id"`
	Status string      `json:"status"`
	Links  []orderLink `json:"links"`
}

type orderLink struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

type captureResponse struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	PurchaseUnits []capturedUnit `json:"purchase_units"`
}

type capturedUnit struct {
	Payments *capturedPayments `json:"payments"`
}

type capturedPayments struct {
	Captures []captureDetail `json:"captures"`
}

type captureDetail struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Amount *amount `json:"amount"`
}

// CaptureResult holds the result of capturing a PayPal order.
type CaptureResult struct {
	CaptureID   string
	Status      string
	AmountCents int64
	Currency    string
}

type refundRequest struct {
	Amount *amount `json:"amount,omitempty"`
}

type refundResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// --- Helper functions ---

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
		frac = parseFractionalCents(parts[1])
	}
	return whole*100 + frac, nil
}

func parseFractionalCents(fracStr string) int64 {
	switch len(fracStr) {
	case 0:
		return 0
	case 1:
		var frac int64
		fmt.Sscanf(fracStr, "%d", &frac)
		return frac * 10
	default:
		var frac int64
		fmt.Sscanf(fracStr[:2], "%d", &frac)
		return frac
	}
}

func findApprovalURL(links []orderLink) string {
	for _, link := range links {
		if link.Rel == "payer-action" || link.Rel == "approve" {
			return link.Href
		}
	}
	return ""
}

func mapPayPalStatus(status string) string {
	switch status {
	case "COMPLETED":
		return "completed"
	case "VOIDED":
		return "failed"
	case "CREATED", "SAVED", "APPROVED", "PAYER_ACTION_REQUIRED":
		return "pending"
	default:
		return "unknown"
	}
}

func extractAmount(resource *WebhookResource) (int64, string, error) {
	if resource.Amount == nil {
		return 0, "", nil
	}
	cents, err := decimalToCents(resource.Amount.Value)
	if err != nil {
		return 0, "", fmt.Errorf("parse capture amount: %w", err)
	}
	return cents, resource.Amount.CurrencyCode, nil
}

func extractOrderID(resource *WebhookResource) string {
	if resource.SupplementaryData != nil &&
		resource.SupplementaryData.RelatedIDs != nil {
		return resource.SupplementaryData.RelatedIDs.OrderID
	}
	return ""
}

// --- Webhook types ---

// WebhookEvent represents a parsed PayPal webhook event.
type WebhookEvent struct {
	ID           string          `json:"id"`
	EventType    string          `json:"event_type"`
	ResourceType string          `json:"resource_type"`
	Resource     json.RawMessage `json:"resource"`
	Summary      string          `json:"summary"`
}

// WebhookResource represents the resource in a webhook event payload.
type WebhookResource struct {
	ID                string             `json:"id"`
	Status            string             `json:"status"`
	CustomID          string             `json:"custom_id"`
	Amount            *amount            `json:"amount"`
	SupplementaryData *supplementaryData `json:"supplementary_data,omitempty"`
}

type supplementaryData struct {
	RelatedIDs *relatedIDs `json:"related_ids,omitempty"`
}

type relatedIDs struct {
	OrderID string `json:"order_id"`
}

// ParseWebhookEvent parses a raw webhook body into a WebhookEvent.
func ParseWebhookEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("parse webhook event: %w", err)
	}
	return &event, nil
}

// ParseWebhookResource extracts the resource from a webhook event.
func ParseWebhookResource(raw json.RawMessage) (*WebhookResource, error) {
	var res WebhookResource
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("parse webhook resource: %w", err)
	}
	return &res, nil
}
