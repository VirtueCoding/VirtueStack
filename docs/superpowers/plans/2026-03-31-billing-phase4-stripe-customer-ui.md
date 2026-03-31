# Billing Phase 4: Stripe Integration + Customer Billing UI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Stripe as the first payment gateway, build the customer billing UI page with balance display and top-up flow, and add admin billing management pages. After this phase, customers can add funds to their accounts via Stripe Checkout and view their billing history in the portal.

**Architecture:** New `internal/controller/payments/` package defines a `PaymentProvider` interface and `PaymentRegistry` for multi-gateway extensibility. The `stripe` sub-package implements the Stripe provider using the official Go SDK. `PaymentService` orchestrates the top-up flow: create Stripe Checkout Session → customer pays on Stripe-hosted page → Stripe webhook callback → verify signature → credit ledger. The customer portal gets a new `/billing` page with balance display, top-up form (preset + custom amounts), transaction history, and payment history. The admin portal gets a `/billing` page with payment list, refund capability, and billing config overview.

**Tech Stack:** Go 1.26, Stripe Go SDK v82 (`github.com/stripe/stripe-go/v82`), React 19, Next.js, TanStack Query, shadcn/ui, Zod, react-hook-form

**Depends on:** Phase 0 (billing registry), Phase 1 (config flags — `StripeConfig`, `BillingConfig`, billing permissions), Phase 3 (credit ledger — `BillingLedgerService`, `BillingTransactionRepository`, `BillingPaymentRepository`, customer/admin billing handlers with balance and transaction endpoints)
**Depended on by:** Phase 5 (Invoicing reads ledger + payment history), Phase 6 (PayPal reuses `PaymentProvider` interface + `PaymentRegistry`)

---

## Task 1: PaymentProvider interface + types

- [ ] Create `internal/controller/payments/provider.go` with the provider interface and shared types

**File:** `internal/controller/payments/provider.go`

### 1a. Create the payments package and provider interface

Create the new `internal/controller/payments/` directory and the provider interface file:

```go
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
	// Amount is in cents (minor currency units).
	RefundPayment(
		ctx context.Context, gatewayPaymentID string, amountCents int64,
	) (*RefundResult, error)

	// ValidateConfig checks that all required configuration is present.
	ValidateConfig() error
}
```

### 1b. Add shared types

Add below the interface in the same file:

```go
// PaymentRequest holds the parameters for creating a payment session.
type PaymentRequest struct {
	CustomerID   string            `json:"customer_id"`
	CustomerEmail string           `json:"customer_email"`
	AmountCents  int64             `json:"amount_cents"`
	Currency     string            `json:"currency"`
	Description  string            `json:"description"`
	ReturnURL    string            `json:"return_url"`
	CancelURL    string            `json:"cancel_url"`
	Metadata     map[string]string `json:"metadata,omitempty"`
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
	Type           WebhookEventType `json:"type"`
	GatewayEventID string           `json:"gateway_event_id"`
	PaymentID      string           `json:"payment_id"`
	AmountCents    int64            `json:"amount_cents"`
	Currency       string           `json:"currency"`
	Status         string           `json:"status"`
	IdempotencyKey string           `json:"idempotency_key"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// PaymentStatus represents the current state of a payment at the gateway.
type PaymentStatus struct {
	GatewayPaymentID string    `json:"gateway_payment_id"`
	Status           string    `json:"status"`
	AmountCents      int64     `json:"amount_cents"`
	Currency         string    `json:"currency"`
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
```

**Test:**

```bash
go build ./internal/controller/payments/...
```

**Commit:**

```
feat(payments): add PaymentProvider interface and shared types

Define the PaymentProvider interface with five methods (Name,
CreatePaymentSession, HandleWebhook, GetPaymentStatus, RefundPayment,
ValidateConfig) and shared types (PaymentRequest, PaymentSession,
WebhookEvent, PaymentStatus, RefundResult). All amounts in cents.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 2: PaymentProvider interface compilation test

- [ ] Add a compilation-time interface check for future providers

**File:** `internal/controller/payments/provider_test.go`

### 2a. Create the test file

```go
package payments_test

import (
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
)

// Compile-time check that the interface types are well-formed.
func TestPaymentProviderInterfaceCompiles(t *testing.T) {
	// Verify the interface is usable as a type constraint.
	var _ payments.PaymentProvider
	_ = payments.PaymentRequest{}
	_ = payments.PaymentSession{}
	_ = payments.WebhookEvent{}
	_ = payments.PaymentStatus{}
	_ = payments.RefundResult{}
}

func TestWebhookEventTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		val  payments.WebhookEventType
		want string
	}{
		{"payment completed", payments.WebhookEventPaymentCompleted, "payment.completed"},
		{"payment failed", payments.WebhookEventPaymentFailed, "payment.failed"},
		{"refund completed", payments.WebhookEventRefundCompleted, "refund.completed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.val) != tt.want {
				t.Errorf("got %q, want %q", tt.val, tt.want)
			}
		})
	}
}
```

**Test:**

```bash
go test -race ./internal/controller/payments/...
```

**Commit:**

```
test(payments): add compilation check for PaymentProvider interface

Verify interface types compile correctly and webhook event type
constants have expected string values.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 3: Payment registry

- [ ] Create `internal/controller/payments/registry.go` with provider registration and lookup

**File:** `internal/controller/payments/registry.go`

### 3a. Create the registry

```go
package payments

import (
	"fmt"
	"sync"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// PaymentRegistry manages registered payment providers.
type PaymentRegistry struct {
	mu        sync.RWMutex
	providers map[string]PaymentProvider
}

// NewPaymentRegistry creates an empty PaymentRegistry.
func NewPaymentRegistry() *PaymentRegistry {
	return &PaymentRegistry{
		providers: make(map[string]PaymentProvider),
	}
}

// Register adds a payment provider to the registry.
// Panics if a provider with the same name is already registered.
func (r *PaymentRegistry) Register(name string, p PaymentProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		panic(fmt.Sprintf("payment provider %q already registered", name))
	}
	r.providers[name] = p
}

// Get retrieves a payment provider by name.
// Returns ErrNotFound if the provider is not registered.
func (r *PaymentRegistry) Get(name string) (PaymentProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("payment provider %q: %w", name, sharederrors.ErrNotFound)
	}
	return p, nil
}

// Available returns the names of all registered providers.
func (r *PaymentRegistry) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
```

**Test:**

```bash
go build ./internal/controller/payments/...
```

**Commit:**

```
feat(payments): add PaymentRegistry for provider management

Registry stores named PaymentProvider instances. Get returns
ErrNotFound for unknown providers. Available lists all registered
provider names. Thread-safe via sync.RWMutex.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 4: Payment registry tests

- [ ] Add table-driven tests for PaymentRegistry

**File:** `internal/controller/payments/registry_test.go`

### 4a. Create tests

```go
package payments_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// mockProvider is a minimal PaymentProvider for testing the registry.
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) CreatePaymentSession(_ context.Context, _ payments.PaymentRequest) (*payments.PaymentSession, error) {
	return nil, nil
}
func (m *mockProvider) HandleWebhook(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
	return nil, nil
}
func (m *mockProvider) GetPaymentStatus(_ context.Context, _ string) (*payments.PaymentStatus, error) {
	return nil, nil
}
func (m *mockProvider) RefundPayment(_ context.Context, _ string, _ int64) (*payments.RefundResult, error) {
	return nil, nil
}
func (m *mockProvider) ValidateConfig() error { return nil }

func TestPaymentRegistry_RegisterAndGet(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		wantErr   bool
		errTarget error
	}{
		{"registered provider", "stripe", false, nil},
		{"unknown provider", "unknown", true, sharederrors.ErrNotFound},
	}

	reg := payments.NewPaymentRegistry()
	reg.Register("stripe", &mockProvider{name: "stripe"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := reg.Get(tt.provider)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errTarget))
				assert.Nil(t, p)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.provider, p.Name())
			}
		})
	}
}

func TestPaymentRegistry_Available(t *testing.T) {
	reg := payments.NewPaymentRegistry()
	reg.Register("stripe", &mockProvider{name: "stripe"})
	reg.Register("paypal", &mockProvider{name: "paypal"})

	available := reg.Available()
	sort.Strings(available)
	assert.Equal(t, []string{"paypal", "stripe"}, available)
}

func TestPaymentRegistry_DuplicateRegistrationPanics(t *testing.T) {
	reg := payments.NewPaymentRegistry()
	reg.Register("stripe", &mockProvider{name: "stripe"})

	assert.Panics(t, func() {
		reg.Register("stripe", &mockProvider{name: "stripe"})
	})
}
```

**Test:**

```bash
go test -race -run TestPaymentRegistry ./internal/controller/payments/...
```

**Commit:**

```
test(payments): add PaymentRegistry tests

Table-driven tests for Get (found/not found), Available (lists all),
and duplicate registration panic. Uses a mock PaymentProvider.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 5: Add Stripe Go SDK dependency

- [ ] Add `github.com/stripe/stripe-go/v82` to `go.mod`

### 5a. Add the dependency

```bash
go get github.com/stripe/stripe-go/v82@latest
go mod tidy
```

### 5b. Verify

```bash
grep "stripe-go" go.mod
# Expected: github.com/stripe/stripe-go/v82 v82.x.x
```

**Commit:**

```
build(deps): add Stripe Go SDK v82

Add github.com/stripe/stripe-go/v82 for Stripe Checkout Session
creation, webhook signature verification, and refund processing.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 6: Stripe provider implementation

- [ ] Create `internal/controller/payments/stripe/provider.go` implementing `PaymentProvider`

**File:** `internal/controller/payments/stripe/provider.go`

### 6a. Create the Stripe provider struct

```go
package stripe

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/refund"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
)

// compile-time interface check
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
```

### 6b. CreatePaymentSession — Stripe Checkout Session

```go
// CreatePaymentSession creates a Stripe Checkout Session in "payment" mode
// (one-time, not subscription). Returns a redirect URL for the customer.
func (p *Provider) CreatePaymentSession(
	ctx context.Context, req payments.PaymentRequest,
) (*payments.PaymentSession, error) {
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

	// Attach metadata for webhook correlation
	params.Metadata = make(map[string]string)
	params.Metadata["customer_id"] = req.CustomerID
	params.Metadata["amount_cents"] = fmt.Sprintf("%d", req.AmountCents)
	for k, v := range req.Metadata {
		params.Metadata[k] = v
	}

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
```

### 6c. HandleWebhook — verify signature and parse event

```go
// HandleWebhook verifies the Stripe webhook signature and parses the event.
// Only processes checkout.session.completed and payment_intent.succeeded events.
func (p *Provider) HandleWebhook(
	_ context.Context, payload []byte, signature string,
) (*payments.WebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, signature, p.webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("verify stripe webhook signature: %w", err)
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
```

### 6d. GetPaymentStatus and RefundPayment

```go
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
	_ context.Context, gatewayPaymentID string, amountCents int64,
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
```

### 6e. ValidateConfig and helpers

```go
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
```

### 6f. JSON parsing helper

**File:** `internal/controller/payments/stripe/helpers.go`

```go
package stripe

import "encoding/json"

// parseStripeObject unmarshals the raw JSON from a Stripe event data object.
func parseStripeObject(raw json.RawMessage, target any) error {
	return json.Unmarshal(raw, target)
}
```

Add required imports to provider.go:

```go
import (
	"time"

	"github.com/stripe/stripe-go/v82/paymentintent"
)
```

**Test:**

```bash
go build ./internal/controller/payments/stripe/...
```

**Commit:**

```
feat(payments): implement Stripe PaymentProvider

Stripe provider creates Checkout Sessions in one-time payment mode,
verifies webhook signatures with stripe.ConstructEvent, processes
checkout.session.completed and payment_intent.succeeded events,
supports refunds via the Refunds API. Idempotency key format:
stripe:event:{event_id}.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 7: Stripe provider tests

- [ ] Add table-driven tests for the Stripe provider with mocked HTTP

**File:** `internal/controller/payments/stripe/provider_test.go`

### 7a. Test webhook signature verification and event parsing

Table-driven tests using `stripe/webhook.GenerateTestSignedPayload` for signature generation:

- HandleWebhook — valid `checkout.session.completed` event, returns WebhookEvent with correct fields
- HandleWebhook — valid `payment_intent.succeeded` event, returns WebhookEvent
- HandleWebhook — invalid signature, returns error
- HandleWebhook — unhandled event type (e.g., `customer.created`), returns nil event and nil error
- HandleWebhook — checkout session with `payment_status != "paid"`, returns nil
- ValidateConfig — valid config, returns nil
- ValidateConfig — missing secret key, returns error
- ValidateConfig — missing webhook secret, returns error
- Name — returns "stripe"

### 7b. Test helpers

- mapStripeStatus — succeeded returns "completed"
- mapStripeStatus — canceled returns "failed"
- mapStripeStatus — processing returns "pending"
- mapStripeStatus — unknown status returns "unknown"

**Test:**

```bash
go test -race -run TestStripe ./internal/controller/payments/stripe/...
```

**Commit:**

```
test(payments): add Stripe provider tests

Table-driven tests for webhook handling (valid/invalid signatures,
event types, payment statuses), config validation, and status
mapping. Uses stripe test signature generation utilities.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 8: Payment service

- [ ] Create `internal/controller/services/payment_service.go` orchestrating top-up flow

**File:** `internal/controller/services/payment_service.go`

### 8a. Define the service struct and constructor

```go
package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// BillingPaymentRepo defines the interface for payment persistence.
type BillingPaymentRepo interface {
	Create(ctx context.Context, payment *models.BillingPayment) error
	GetByID(ctx context.Context, id string) (*models.BillingPayment, error)
	GetByGatewayPaymentID(ctx context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error)
	UpdateStatus(ctx context.Context, id, status string, gatewayPaymentID *string) error
	ListByCustomer(ctx context.Context, customerID string, filter models.PaginationParams) ([]models.BillingPayment, bool, string, error)
	ListAll(ctx context.Context, filter PaymentListFilter) ([]models.BillingPayment, bool, string, error)
}

// PaymentListFilter holds optional filters for listing payments.
type PaymentListFilter struct {
	CustomerID *string
	Gateway    *string
	Status     *string
	models.PaginationParams
}

// PaymentServiceConfig holds dependencies for the PaymentService.
type PaymentServiceConfig struct {
	PaymentRegistry *payments.PaymentRegistry
	LedgerService   *BillingLedgerService
	PaymentRepo     BillingPaymentRepo
	SettingsRepo    SettingsRepo
	Logger          *slog.Logger
}

// PaymentService orchestrates payment operations (top-up, webhook, refund).
type PaymentService struct {
	registry    *payments.PaymentRegistry
	ledger      *BillingLedgerService
	paymentRepo BillingPaymentRepo
	settingsRepo SettingsRepo
	logger      *slog.Logger
}

// NewPaymentService creates a new PaymentService.
func NewPaymentService(cfg PaymentServiceConfig) *PaymentService {
	return &PaymentService{
		registry:     cfg.PaymentRegistry,
		ledger:       cfg.LedgerService,
		paymentRepo:  cfg.PaymentRepo,
		settingsRepo: cfg.SettingsRepo,
		logger:       cfg.Logger.With("component", "payment-service"),
	}
}
```

### 8b. InitiateTopUp method

```go
// InitiateTopUp creates a pending payment record and a gateway checkout session.
func (s *PaymentService) InitiateTopUp(
	ctx context.Context,
	customerID string,
	email string,
	amountCents int64,
	currency string,
	gateway string,
	returnURL string,
	cancelURL string,
) (*payments.PaymentSession, string, error) {
	provider, err := s.registry.Get(gateway)
	if err != nil {
		return nil, "", fmt.Errorf("get payment provider: %w", err)
	}

	// Create a pending payment record
	payment := &models.BillingPayment{
		CustomerID: customerID,
		Gateway:    gateway,
		Amount:     amountCents,
		Currency:   currency,
		Status:     models.PaymentStatusPending,
	}
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, "", fmt.Errorf("create payment record: %w", err)
	}

	// Create the gateway session
	sess, err := provider.CreatePaymentSession(ctx, payments.PaymentRequest{
		CustomerID:    customerID,
		CustomerEmail: email,
		AmountCents:   amountCents,
		Currency:      currency,
		Description:   "Credit Top-Up",
		ReturnURL:     returnURL,
		CancelURL:     cancelURL,
		Metadata: map[string]string{
			"payment_id":  payment.ID,
			"customer_id": customerID,
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("create payment session: %w", err)
	}

	s.logger.Info("top-up payment initiated",
		"payment_id", payment.ID,
		"customer_id", customerID,
		"gateway", gateway,
		"amount_cents", amountCents,
	)

	return sess, payment.ID, nil
}
```

### 8c. HandleWebhook method

```go
// HandleWebhook processes an incoming gateway webhook. Verifies the
// signature, credits the ledger on success, and updates the payment record.
func (s *PaymentService) HandleWebhook(
	ctx context.Context,
	gateway string,
	payload []byte,
	signature string,
) error {
	provider, err := s.registry.Get(gateway)
	if err != nil {
		return fmt.Errorf("get payment provider: %w", err)
	}

	event, err := provider.HandleWebhook(ctx, payload, signature)
	if err != nil {
		return fmt.Errorf("handle webhook: %w", err)
	}

	// Nil event means unhandled event type — acknowledge without processing
	if event == nil {
		return nil
	}

	switch event.Type {
	case payments.WebhookEventPaymentCompleted:
		return s.handlePaymentCompleted(ctx, gateway, event)
	case payments.WebhookEventPaymentFailed:
		return s.handlePaymentFailed(ctx, gateway, event)
	case payments.WebhookEventRefundCompleted:
		return s.handleRefundCompleted(ctx, gateway, event)
	default:
		return nil
	}
}

func (s *PaymentService) handlePaymentCompleted(
	ctx context.Context, gateway string, event *payments.WebhookEvent,
) error {
	// Look up the payment record by metadata or gateway payment ID
	customerID := event.Metadata["customer_id"]
	paymentID := event.Metadata["payment_id"]

	if paymentID != "" {
		if err := s.paymentRepo.UpdateStatus(
			ctx, paymentID, models.PaymentStatusCompleted, &event.PaymentID,
		); err != nil {
			s.logger.Error("failed to update payment status",
				"payment_id", paymentID, "error", err)
		}
	}

	// Credit the customer's ledger
	if customerID == "" {
		return fmt.Errorf("webhook event missing customer_id metadata")
	}

	_, err := s.ledger.CreditAccount(
		ctx, customerID, event.AmountCents,
		fmt.Sprintf("Top-up via %s", gateway),
		&event.IdempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("credit account: %w", err)
	}

	s.logger.Info("payment completed and ledger credited",
		"customer_id", customerID,
		"amount_cents", event.AmountCents,
		"gateway", gateway,
		"idempotency_key", event.IdempotencyKey,
	)
	return nil
}

func (s *PaymentService) handlePaymentFailed(
	ctx context.Context, _ string, event *payments.WebhookEvent,
) error {
	paymentID := event.Metadata["payment_id"]
	if paymentID != "" {
		return s.paymentRepo.UpdateStatus(
			ctx, paymentID, models.PaymentStatusFailed, &event.PaymentID,
		)
	}
	return nil
}

func (s *PaymentService) handleRefundCompleted(
	ctx context.Context, gateway string, event *payments.WebhookEvent,
) error {
	existing, err := s.paymentRepo.GetByGatewayPaymentID(ctx, gateway, event.PaymentID)
	if err != nil {
		return fmt.Errorf("get payment for refund: %w", err)
	}

	if err := s.paymentRepo.UpdateStatus(
		ctx, existing.ID, models.PaymentStatusRefunded, nil,
	); err != nil {
		return fmt.Errorf("update payment status for refund: %w", err)
	}

	refType := models.BillingRefTypeRefund
	_, err = s.ledger.DebitAccount(
		ctx, existing.CustomerID, event.AmountCents,
		fmt.Sprintf("Refund via %s", gateway),
		&refType, &existing.ID, &event.IdempotencyKey,
	)
	return err
}
```

### 8d. GetPaymentHistory and GetTopUpConfig methods

```go
// GetPaymentHistory returns paginated payment history for a customer.
func (s *PaymentService) GetPaymentHistory(
	ctx context.Context, customerID string, filter models.PaginationParams,
) ([]models.BillingPayment, bool, string, error) {
	return s.paymentRepo.ListByCustomer(ctx, customerID, filter)
}

// ListAllPayments returns paginated payments across all customers (admin).
func (s *PaymentService) ListAllPayments(
	ctx context.Context, filter PaymentListFilter,
) ([]models.BillingPayment, bool, string, error) {
	return s.paymentRepo.ListAll(ctx, filter)
}

// TopUpConfig holds the top-up configuration returned to clients.
type TopUpConfig struct {
	MinAmountCents int64    `json:"min_amount_cents"`
	MaxAmountCents int64    `json:"max_amount_cents"`
	Presets        []int64  `json:"presets"`
	Gateways       []string `json:"gateways"`
	Currency       string   `json:"currency"`
}

// GetTopUpConfig returns the top-up configuration including available
// gateways, min/max amounts, and preset amounts.
func (s *PaymentService) GetTopUpConfig(ctx context.Context) (*TopUpConfig, error) {
	gateways := s.registry.Available()

	minAmount := int64(500)   // $5.00 default
	maxAmount := int64(50000) // $500.00 default
	presets := []int64{500, 1000, 2500, 5000, 10000}

	// Override from system_settings if available
	if s.settingsRepo != nil {
		if v, err := s.settingsRepo.GetInt64(ctx, "billing.topup.min_amount"); err == nil {
			minAmount = v
		}
		if v, err := s.settingsRepo.GetInt64(ctx, "billing.topup.max_amount"); err == nil {
			maxAmount = v
		}
		if v, err := s.settingsRepo.GetInt64Slice(ctx, "billing.topup.presets"); err == nil && len(v) > 0 {
			presets = v
		}
	}

	return &TopUpConfig{
		MinAmountCents: minAmount,
		MaxAmountCents: maxAmount,
		Presets:        presets,
		Gateways:       gateways,
		Currency:       "USD",
	}, nil
}
```

### 8e. RefundPayment method (admin)

```go
// RefundPayment initiates a refund for a completed payment.
func (s *PaymentService) RefundPayment(
	ctx context.Context, paymentID string, amountCents int64,
) (*payments.RefundResult, error) {
	payment, err := s.paymentRepo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("get payment for refund: %w", err)
	}

	if payment.Status != models.PaymentStatusCompleted {
		return nil, sharederrors.NewValidationError("status",
			"can only refund completed payments")
	}

	if payment.GatewayPaymentID == nil || *payment.GatewayPaymentID == "" {
		return nil, sharederrors.NewValidationError("gateway_payment_id",
			"payment has no gateway reference for refund")
	}

	if amountCents <= 0 || amountCents > payment.Amount {
		return nil, sharederrors.NewValidationError("amount",
			"refund amount must be between 1 and the original payment amount")
	}

	provider, err := s.registry.Get(payment.Gateway)
	if err != nil {
		return nil, fmt.Errorf("get payment provider: %w", err)
	}

	result, err := provider.RefundPayment(ctx, *payment.GatewayPaymentID, amountCents)
	if err != nil {
		return nil, fmt.Errorf("process refund via %s: %w", payment.Gateway, err)
	}

	s.logger.Info("refund processed",
		"payment_id", paymentID,
		"refund_id", result.GatewayRefundID,
		"amount_cents", amountCents,
	)

	return result, nil
}
```

**Test:**

```bash
go build ./internal/controller/services/...
```

**Commit:**

```
feat(services): add PaymentService for top-up and webhook orchestration

PaymentService.InitiateTopUp creates a pending payment record and
delegates to the PaymentProvider for checkout session creation.
HandleWebhook verifies signatures, credits the ledger on success,
and updates payment status. Supports refunds and admin-configurable
top-up presets via system_settings.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 9: Payment service tests

- [ ] Add table-driven tests for PaymentService

**File:** `internal/controller/services/payment_service_test.go`

### 9a. Mock interfaces

Create mock implementations for `BillingPaymentRepo`, `BillingLedgerService`, and `PaymentProvider`. Use function-field mocks following the existing VirtueStack pattern.

### 9b. Test cases

Table-driven tests:
- **InitiateTopUp** — valid request, payment record created, session returned
- **InitiateTopUp** — unknown gateway, returns ErrNotFound
- **InitiateTopUp** — provider error propagated
- **HandleWebhook** — payment completed, ledger credited
- **HandleWebhook** — payment completed, idempotency key prevents double-credit (ledger returns existing)
- **HandleWebhook** — unknown gateway, returns error
- **HandleWebhook** — invalid signature (provider returns error), returns error
- **HandleWebhook** — unhandled event type, returns nil
- **HandleWebhook** — payment failed, status updated
- **GetPaymentHistory** — delegates to repo with pagination
- **GetTopUpConfig** — returns defaults when no settings
- **GetTopUpConfig** — returns overrides from settings repo
- **RefundPayment** — valid refund on completed payment
- **RefundPayment** — refund on pending payment returns validation error
- **RefundPayment** — amount exceeds original returns validation error

**Test:**

```bash
go test -race -run TestPaymentService ./internal/controller/services/...
```

**Commit:**

```
test(services): add PaymentService tests

Table-driven tests covering InitiateTopUp, HandleWebhook with
idempotency, refund validation, top-up config with defaults and
overrides. Mock PaymentProvider and BillingPaymentRepo.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 10: Stripe webhook handler (unauthenticated route)

- [ ] Create webhook handler and register unauthenticated route

**File:** `internal/controller/api/webhooks/stripe.go`

### 10a. Create the webhook handler

```go
package webhooks

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// StripeWebhookHandler handles Stripe webhook callbacks.
type StripeWebhookHandler struct {
	paymentService *services.PaymentService
	logger         *slog.Logger
}

// NewStripeWebhookHandler creates a new StripeWebhookHandler.
func NewStripeWebhookHandler(
	paymentService *services.PaymentService,
	logger *slog.Logger,
) *StripeWebhookHandler {
	return &StripeWebhookHandler{
		paymentService: paymentService,
		logger:         logger.With("component", "stripe-webhook"),
	}
}

// Handle processes POST /api/v1/webhooks/stripe.
// This endpoint is unauthenticated — Stripe signature verification
// happens inside the payment provider.
func (h *StripeWebhookHandler) Handle(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	if signature == "" {
		h.logger.Warn("stripe webhook missing signature header")
		c.Status(http.StatusBadRequest)
		return
	}

	if err := h.paymentService.HandleWebhook(
		c.Request.Context(), "stripe", payload, signature,
	); err != nil {
		h.logger.Error("failed to process stripe webhook",
			"error", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}
```

### 10b. Register the route

**File:** `internal/controller/server.go`

Add webhook route registration in `SetupRoutes()` (outside the authenticated route groups):

```go
	// Payment webhooks — unauthenticated, signature-verified internally
	webhooks := router.Group("/api/v1/webhooks")
	{
		if s.stripeWebhookHandler != nil {
			webhooks.POST("/stripe", s.stripeWebhookHandler.Handle)
		}
	}
```

Add `stripeWebhookHandler *webhooks.StripeWebhookHandler` field to the `Server` struct.

**Test:**

```bash
go build ./cmd/controller/...
```

**Commit:**

```
feat(api): add Stripe webhook handler at POST /api/v1/webhooks/stripe

Unauthenticated endpoint reads raw body, extracts Stripe-Signature
header, and delegates to PaymentService.HandleWebhook for signature
verification and ledger crediting. Registered outside auth middleware.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 11: Webhook handler tests

- [ ] Add table-driven tests for the Stripe webhook handler

**File:** `internal/controller/api/webhooks/stripe_test.go`

### 11a. Tests

Table-driven tests using `httptest.NewRecorder` and `gin.CreateTestContext`:

- Valid webhook with correct signature → HTTP 200
- Missing Stripe-Signature header → HTTP 400
- Empty body → HTTP 400
- Invalid signature (PaymentService returns error) → HTTP 500
- Valid unhandled event type → HTTP 200 (no processing)

**Test:**

```bash
go test -race -run TestStripeWebhook ./internal/controller/api/webhooks/...
```

**Commit:**

```
test(api): add Stripe webhook handler tests

Table-driven tests for valid/invalid webhook signatures, missing
headers, and unhandled event types. Uses mock PaymentService.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 12: Customer top-up API endpoint

- [ ] Add `POST /customer/billing/top-up` handler to initiate payment

**File:** `internal/controller/api/customer/billing.go`

### 12a. Add top-up request model

**File:** `internal/controller/models/billing_payment.go`

Add to the existing billing payment models file:

```go
// TopUpRequest holds fields for initiating a credit top-up.
type TopUpRequest struct {
	Gateway   string `json:"gateway" validate:"required,oneof=stripe paypal btcpay nowpayments"`
	Amount    int64  `json:"amount" validate:"required,gt=0"`
	Currency  string `json:"currency" validate:"required,len=3"`
	ReturnURL string `json:"return_url" validate:"required,url"`
	CancelURL string `json:"cancel_url" validate:"required,url"`
}

// TopUpResponse is returned after initiating a top-up.
type TopUpResponse struct {
	PaymentID  string `json:"payment_id"`
	PaymentURL string `json:"payment_url"`
}
```

### 12b. Add handler method

Add to `internal/controller/api/customer/billing.go` (extends Phase 3 file):

```go
// InitiateTopUp handles POST /customer/billing/top-up.
func (h *CustomerHandler) InitiateTopUp(c *gin.Context) {
	var req models.TopUpRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		return
	}

	customerID := middleware.GetUserID(c)
	email := middleware.GetUserEmail(c)

	// Validate amount against config
	config, err := h.paymentService.GetTopUpConfig(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get top-up config",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TOPUP_CONFIG_FAILED", "Failed to retrieve top-up configuration")
		return
	}

	if req.Amount < config.MinAmountCents || req.Amount > config.MaxAmountCents {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_AMOUNT",
			fmt.Sprintf("Amount must be between %d and %d cents",
				config.MinAmountCents, config.MaxAmountCents))
		return
	}

	sess, paymentID, err := h.paymentService.InitiateTopUp(
		c.Request.Context(),
		customerID, email, req.Amount, req.Currency,
		req.Gateway, req.ReturnURL, req.CancelURL,
	)
	if err != nil {
		h.logger.Error("failed to initiate top-up",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TOPUP_FAILED", "Failed to initiate payment")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: models.TopUpResponse{
		PaymentID:  paymentID,
		PaymentURL: sess.PaymentURL,
	}})
}
```

**Commit:**

```
feat(api): add customer top-up endpoint POST /customer/billing/top-up

Validates amount against admin-configurable min/max, creates pending
payment record, creates Stripe Checkout Session, returns redirect URL.
Amount validated in cents. CSRF required.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 13: Customer payment history endpoint

- [ ] Add `GET /customer/billing/payments` handler

**File:** `internal/controller/api/customer/billing.go`

### 13a. Add handler method

```go
// GetPaymentHistory handles GET /customer/billing/payments.
func (h *CustomerHandler) GetPaymentHistory(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	pagination := models.ParsePagination(c)

	payments, hasMore, lastID, err := h.paymentService.GetPaymentHistory(
		c.Request.Context(), customerID, pagination,
	)
	if err != nil {
		h.logger.Error("failed to list payments",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PAYMENT_LIST_FAILED", "Failed to list payments")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: payments,
		Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
	})
}
```

**Commit:**

```
feat(api): add customer payment history endpoint

GET /customer/billing/payments returns cursor-paginated payment
history for the authenticated customer. JWT-only access.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 14: Customer top-up config endpoint

- [ ] Add `GET /customer/billing/top-up/config` handler

**File:** `internal/controller/api/customer/billing.go`

### 14a. Add handler method

```go
// GetTopUpConfig handles GET /customer/billing/top-up/config.
func (h *CustomerHandler) GetTopUpConfig(c *gin.Context) {
	config, err := h.paymentService.GetTopUpConfig(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get top-up config",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TOPUP_CONFIG_FAILED", "Failed to retrieve top-up configuration")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: config})
}
```

**Commit:**

```
feat(api): add customer top-up config endpoint

GET /customer/billing/top-up/config returns available gateways,
min/max amounts, preset amounts, and currency. Values configurable
via system_settings by admin.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 15: Customer billing handler tests

- [ ] Add table-driven tests for the new customer billing handlers

**File:** `internal/controller/api/customer/billing_test.go`

### 15a. Extend existing billing tests

Add tests alongside the Phase 3 billing handler tests:

- **InitiateTopUp** — valid request with Stripe, returns payment URL
- **InitiateTopUp** — amount below minimum, returns 400
- **InitiateTopUp** — amount above maximum, returns 400
- **InitiateTopUp** — unknown gateway, returns error
- **InitiateTopUp** — missing return_url, validation error
- **GetPaymentHistory** — returns paginated list
- **GetPaymentHistory** — empty list returns empty array
- **GetTopUpConfig** — returns config with defaults
- **GetTopUpConfig** — returns config with custom settings

**Test:**

```bash
go test -race -run TestCustomerBilling ./internal/controller/api/customer/...
```

**Commit:**

```
test(api): add customer billing handler tests for top-up and payments

Table-driven tests for InitiateTopUp (valid/invalid amounts, unknown
gateway), GetPaymentHistory (paginated/empty), and GetTopUpConfig
(defaults/overrides).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 16: Admin payment list endpoint

- [ ] Add `GET /admin/billing/payments` handler

**File:** `internal/controller/api/admin/billing.go`

### 16a. Add handler method

Extend the Phase 3 admin billing handler file:

```go
// ListPayments handles GET /admin/billing/payments.
func (h *AdminHandler) ListPayments(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := services.PaymentListFilter{
		PaginationParams: pagination,
	}

	if customerID := c.Query("customer_id"); customerID != "" {
		if _, err := uuid.Parse(customerID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"INVALID_CUSTOMER_ID", "customer_id must be a valid UUID")
			return
		}
		filter.CustomerID = &customerID
	}

	if gateway := c.Query("gateway"); gateway != "" {
		filter.Gateway = &gateway
	}

	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	payments, hasMore, lastID, err := h.paymentService.ListAllPayments(
		c.Request.Context(), filter,
	)
	if err != nil {
		h.logger.Error("failed to list payments",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PAYMENT_LIST_FAILED", "Failed to list payments")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: payments,
		Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
	})
}
```

**Commit:**

```
feat(api): add admin payment list endpoint

GET /admin/billing/payments returns cursor-paginated payments across
all customers. Supports customer_id, gateway, and status filters.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 17: Admin refund endpoint

- [ ] Add `POST /admin/billing/refund/:paymentId` handler

**File:** `internal/controller/api/admin/billing.go`

### 17a. Add refund request model

**File:** `internal/controller/models/billing_payment.go`

```go
// RefundRequest holds fields for an admin-initiated refund.
type RefundRequest struct {
	Amount int64  `json:"amount" validate:"required,gt=0"`
	Reason string `json:"reason" validate:"required,max=500"`
}
```

### 17b. Add handler method

```go
// RefundPayment handles POST /admin/billing/refund/:paymentId.
func (h *AdminHandler) RefundPayment(c *gin.Context) {
	paymentID := c.Param("paymentId")
	if _, err := uuid.Parse(paymentID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PAYMENT_ID", "paymentId must be a valid UUID")
		return
	}

	var req models.RefundRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		return
	}

	result, err := h.paymentService.RefundPayment(
		c.Request.Context(), paymentID, req.Amount,
	)
	if err != nil {
		h.logger.Error("failed to process refund",
			"payment_id", paymentID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"PAYMENT_NOT_FOUND", "Payment not found")
			return
		}

		var valErr sharederrors.ValidationError
		if errors.As(err, &valErr) {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"VALIDATION_ERROR", valErr.Error())
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError,
			"REFUND_FAILED", "Failed to process refund")
		return
	}

	actorID := middleware.GetUserID(c)
	h.logAuditEvent(c, "billing.refund", "payment", paymentID,
		map[string]any{
			"amount":    req.Amount,
			"reason":    req.Reason,
			"refund_id": result.GatewayRefundID,
			"actor_id":  actorID,
		}, true)

	c.JSON(http.StatusOK, models.Response{Data: result})
}
```

**Commit:**

```
feat(api): add admin refund endpoint POST /admin/billing/refund/:paymentId

Validates payment exists and is completed, delegates to PaymentService
for gateway refund processing. Audit-logged with actor ID and reason.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 18: Admin billing config endpoint

- [ ] Add `GET /admin/billing/config` handler

**File:** `internal/controller/api/admin/billing.go`

### 18a. Add handler method

```go
// GetBillingConfig handles GET /admin/billing/config.
func (h *AdminHandler) GetBillingConfig(c *gin.Context) {
	topUpConfig, err := h.paymentService.GetTopUpConfig(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get billing config",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"BILLING_CONFIG_FAILED", "Failed to retrieve billing configuration")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: map[string]any{
		"top_up":    topUpConfig,
		"gateways":  topUpConfig.Gateways,
	}})
}
```

**Commit:**

```
feat(api): add admin billing config endpoint

GET /admin/billing/config returns current top-up configuration
including min/max amounts, preset amounts, and available gateways.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 19: Admin billing handler tests

- [ ] Add table-driven tests for the new admin billing handlers

**File:** `internal/controller/api/admin/billing_test.go`

### 19a. Extend existing billing tests

Add tests alongside the Phase 3 admin billing handler tests:

- **ListPayments** — returns paginated list
- **ListPayments** — with customer_id filter
- **ListPayments** — with gateway filter
- **ListPayments** — with status filter
- **ListPayments** — invalid customer_id returns 400
- **RefundPayment** — valid refund on completed payment, returns refund result
- **RefundPayment** — invalid payment UUID, returns 400
- **RefundPayment** — payment not found, returns 404
- **RefundPayment** — pending payment, returns 400
- **RefundPayment** — amount exceeds original, returns 400
- **RefundPayment** — audit event logged
- **GetBillingConfig** — returns top-up config

**Test:**

```bash
go test -race -run TestAdminBilling ./internal/controller/api/admin/...
```

**Commit:**

```
test(api): add admin billing tests for payments, refunds, and config

Table-driven tests covering ListPayments with multiple filters,
RefundPayment validation (status, amount, not found), audit logging,
and GetBillingConfig response.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 20: Customer billing routes registration

- [ ] Register the new billing routes in customer routes.go

**File:** `internal/controller/api/customer/routes.go`

### 20a. Extend the billing route group

In the existing `registerBillingRoutes` function (added by Phase 3), add the new routes:

```go
func registerBillingRoutes(group *gin.RouterGroup, handler *CustomerHandler) {
	billing := group.Group("/billing")
	{
		billing.GET("/balance", handler.GetBillingBalance)           // Phase 3
		billing.GET("/transactions", handler.ListBillingTransactions) // Phase 3
		billing.GET("/usage", handler.GetBillingUsage)               // Phase 3
		billing.POST("/top-up", handler.InitiateTopUp)               // Phase 4
		billing.GET("/payments", handler.GetPaymentHistory)          // Phase 4
		billing.GET("/top-up/config", handler.GetTopUpConfig)        // Phase 4
	}
}
```

**Commit:**

```
feat(api): register customer top-up, payments, and config routes

Add POST /customer/billing/top-up, GET /customer/billing/payments,
GET /customer/billing/top-up/config to the JWT-only billing route
group. Top-up requires CSRF protection.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 21: Admin billing routes registration

- [ ] Register the new billing routes in admin routes.go

**File:** `internal/controller/api/admin/routes.go`

### 21a. Extend the billing route group

In the existing billing route group (added by Phase 3), add the new routes:

```go
		// Billing management
		billing := protected.Group("/billing")
		{
			billing.GET("/transactions",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.ListBillingTransactions)                         // Phase 3
			billing.POST("/credit",
				middleware.RequireAdminPermission(models.PermissionBillingWrite),
				handler.AdminCreditAdjustment)                          // Phase 3
			billing.GET("/payments",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.ListPayments)                                   // Phase 4
			billing.POST("/refund/:paymentId",
				middleware.RequireAdminPermission(models.PermissionBillingWrite),
				handler.RefundPayment)                                  // Phase 4
			billing.GET("/config",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.GetBillingConfig)                               // Phase 4
		}
```

**Commit:**

```
feat(api): register admin payment, refund, and config routes

Add GET /admin/billing/payments, POST /admin/billing/refund/:paymentId,
GET /admin/billing/config. Payments and config require billing:read,
refund requires billing:write.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 22: Dependencies.go wiring

- [ ] Wire PaymentRegistry, Stripe provider, PaymentService, and webhook handler

**File:** `internal/controller/dependencies.go`

### 22a. Create PaymentRegistry and register Stripe provider

Add after the Phase 3 billing service initialization:

```go
	// Payment gateway registry
	paymentRegistry := payments.NewPaymentRegistry()

	// Register Stripe provider if configured
	if s.config.Stripe.SecretKey.Value() != "" {
		stripeProvider := stripePayments.NewProvider(stripePayments.ProviderConfig{
			SecretKey:      s.config.Stripe.SecretKey.Value(),
			WebhookSecret:  s.config.Stripe.WebhookSecret.Value(),
			PublishableKey: s.config.Stripe.PublishableKey,
			Logger:         s.logger,
		})
		if err := stripeProvider.ValidateConfig(); err != nil {
			return fmt.Errorf("stripe config validation: %w", err)
		}
		paymentRegistry.Register("stripe", stripeProvider)
		s.logger.Info("stripe payment provider registered")
	}
```

### 22b. Create PaymentService

```go
	// Payment service
	paymentService := services.NewPaymentService(services.PaymentServiceConfig{
		PaymentRegistry: paymentRegistry,
		LedgerService:   billingLedgerService,
		PaymentRepo:     billingPaymentRepo,
		SettingsRepo:    settingsRepo,
		Logger:          s.logger,
	})
```

### 22c. Add PaymentService to handler configs

Add to `CustomerHandlerConfig`:

```go
		PaymentService: paymentService,
```

Add to `AdminHandlerConfig`:

```go
		PaymentService: paymentService,
```

### 22d. Create and store webhook handler

```go
	// Stripe webhook handler
	if s.config.Stripe.SecretKey.Value() != "" {
		s.stripeWebhookHandler = webhooks.NewStripeWebhookHandler(
			paymentService, s.logger,
		)
	}
```

### 22e. Add required imports

```go
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	stripePayments "github.com/AbuGosok/VirtueStack/internal/controller/payments/stripe"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/webhooks"
```

### 22f. Update handler structs

**File:** `internal/controller/api/customer/handler.go`

Add `paymentService *services.PaymentService` to `CustomerHandlerConfig` and `CustomerHandler` struct. Assign in `NewCustomerHandler`.

**File:** `internal/controller/api/admin/handler.go`

Add `paymentService *services.PaymentService` to `AdminHandlerConfig` and `AdminHandler` struct. Assign in `NewAdminHandler`.

**File:** `internal/controller/server.go`

Add `stripeWebhookHandler *webhooks.StripeWebhookHandler` field to `Server`.

**Test:**

```bash
make build-controller
```

**Commit:**

```
feat(wiring): wire PaymentRegistry, Stripe provider, and PaymentService

Create PaymentRegistry, register Stripe provider when configured,
create PaymentService with ledger integration. Wire PaymentService
into customer and admin handlers. Add Stripe webhook handler to
Server struct.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 23: BillingPayment repository — add ListAll method

- [ ] Add `ListAll` method to `BillingPaymentRepository` for admin queries

**File:** `internal/controller/repository/billing_payment_repo.go`

### 23a. Add the ListAll method

Extend the Phase 3 `BillingPaymentRepository` with a multi-filter list method:

```go
// ListAll returns paginated payments with optional filters (admin use).
func (r *BillingPaymentRepository) ListAll(
	ctx context.Context, filter services.PaymentListFilter,
) ([]models.BillingPayment, bool, string, error) {
	var args []any
	argPos := 1
	conditions := []string{}

	if filter.CustomerID != nil {
		conditions = append(conditions, fmt.Sprintf("customer_id = $%d", argPos))
		args = append(args, *filter.CustomerID)
		argPos++
	}
	if filter.Gateway != nil {
		conditions = append(conditions, fmt.Sprintf("gateway = $%d", argPos))
		args = append(args, *filter.Gateway)
		argPos++
	}
	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *filter.Status)
		argPos++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	if filter.Cursor != "" {
		cursorCond := fmt.Sprintf("id < $%d", argPos)
		args = append(args, filter.Cursor)
		argPos++
		if where == "" {
			where = "WHERE " + cursorCond
		} else {
			where += " AND " + cursorCond
		}
	}

	q := fmt.Sprintf(
		"SELECT %s FROM billing_payments %s ORDER BY created_at DESC, id DESC LIMIT $%d",
		billingPaymentSelectCols, where, argPos,
	)
	args = append(args, filter.PerPage+1)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, false, "", fmt.Errorf("list all payments: %w", err)
	}
	defer rows.Close()

	payments, err := scanBillingPayments(rows)
	if err != nil {
		return nil, false, "", err
	}

	hasMore := len(payments) > filter.PerPage
	if hasMore {
		payments = payments[:filter.PerPage]
	}

	var lastID string
	if len(payments) > 0 {
		lastID = payments[len(payments)-1].ID
	}

	return payments, hasMore, lastID, nil
}
```

**Note:** Import `"strings"` if not already imported.

**Test:**

```bash
go build ./internal/controller/repository/...
```

**Commit:**

```
feat(repository): add ListAll method to BillingPaymentRepository

Add multi-filter list method for admin payment queries. Supports
filtering by customer_id, gateway, and status with cursor pagination.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 24: Customer API client updates

- [ ] Add billing methods to the customer frontend API client

**File:** `webui/customer/lib/api-client.ts`

### 24a. Add billing types

```typescript
// Billing types
export interface BillingBalance {
  balance: number;
  currency: string;
}

export interface BillingTransaction {
  id: string;
  customer_id: string;
  type: "credit" | "debit" | "adjustment" | "refund";
  amount: number;
  balance_after: number;
  description: string;
  reference_type?: string;
  reference_id?: string;
  created_at: string;
}

export interface BillingPayment {
  id: string;
  customer_id: string;
  gateway: string;
  gateway_payment_id?: string;
  amount: number;
  currency: string;
  status: "pending" | "completed" | "failed" | "refunded";
  created_at: string;
  updated_at: string;
}

export interface TopUpConfig {
  min_amount_cents: number;
  max_amount_cents: number;
  presets: number[];
  gateways: string[];
  currency: string;
}

export interface TopUpRequest {
  gateway: string;
  amount: number;
  currency: string;
  return_url: string;
  cancel_url: string;
}

export interface TopUpResponse {
  payment_id: string;
  payment_url: string;
}
```

### 24b. Add billingApi object

```typescript
export const billingApi = {
  async getBalance(): Promise<BillingBalance> {
    const resp = await apiClient.get<{ data: BillingBalance }>(
      "/customer/billing/balance"
    );
    return resp.data;
  },

  async getTransactions(
    params: { perPage?: number; cursor?: string } = {}
  ): Promise<CursorPaginatedResponse<BillingTransaction>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<BillingTransaction>>(
      `/customer/billing/transactions${query}`
    );
  },

  async getPayments(
    params: { perPage?: number; cursor?: string } = {}
  ): Promise<CursorPaginatedResponse<BillingPayment>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<BillingPayment>>(
      `/customer/billing/payments${query}`
    );
  },

  async getTopUpConfig(): Promise<TopUpConfig> {
    const resp = await apiClient.get<{ data: TopUpConfig }>(
      "/customer/billing/top-up/config"
    );
    return resp.data;
  },

  async initiateTopUp(req: TopUpRequest): Promise<TopUpResponse> {
    await fetchCsrfToken();
    const resp = await apiClient.post<{ data: TopUpResponse }>(
      "/customer/billing/top-up",
      req
    );
    return resp.data;
  },
};
```

**Commit:**

```
feat(webui): add customer billing API client methods

Add billingApi with getBalance, getTransactions, getPayments,
getTopUpConfig, and initiateTopUp. All amounts in cents. Uses
cursor-based pagination for list endpoints.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 25: Admin API client updates

- [ ] Add billing methods to the admin frontend API client

**File:** `webui/admin/lib/api-client.ts`

### 25a. Add billing types

```typescript
// Billing types (admin)
export interface BillingPayment {
  id: string;
  customer_id: string;
  gateway: string;
  gateway_payment_id?: string;
  amount: number;
  currency: string;
  status: "pending" | "completed" | "failed" | "refunded";
  created_at: string;
  updated_at: string;
}

export interface BillingTransaction {
  id: string;
  customer_id: string;
  type: "credit" | "debit" | "adjustment" | "refund";
  amount: number;
  balance_after: number;
  description: string;
  reference_type?: string;
  reference_id?: string;
  created_at: string;
}

export interface BillingConfig {
  top_up: {
    min_amount_cents: number;
    max_amount_cents: number;
    presets: number[];
    gateways: string[];
    currency: string;
  };
  gateways: string[];
}

export interface RefundRequest {
  amount: number;
  reason: string;
}

export interface RefundResult {
  gateway_refund_id: string;
  gateway_payment_id: string;
  amount_cents: number;
  currency: string;
  status: string;
}
```

### 25b. Add adminBillingApi object

```typescript
export const adminBillingApi = {
  async getTransactions(
    params: { perPage?: number; cursor?: string; customerID?: string } = {}
  ): Promise<CursorPaginatedResponse<BillingTransaction>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    if (params.customerID) queryParams.set("customer_id", params.customerID);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<BillingTransaction>>(
      `/admin/billing/transactions${query}`
    );
  },

  async creditAdjustment(
    customerID: string,
    amount: number,
    description: string
  ): Promise<BillingTransaction> {
    await fetchCsrfToken();
    const resp = await apiClient.post<{ data: BillingTransaction }>(
      `/admin/billing/credit?customer_id=${customerID}`,
      { amount, description }
    );
    return resp.data;
  },

  async getPayments(
    params: {
      perPage?: number;
      cursor?: string;
      customerID?: string;
      gateway?: string;
      status?: string;
    } = {}
  ): Promise<CursorPaginatedResponse<BillingPayment>> {
    const queryParams = new URLSearchParams();
    if (params.perPage) queryParams.set("per_page", String(params.perPage));
    if (params.cursor) queryParams.set("cursor", params.cursor);
    if (params.customerID) queryParams.set("customer_id", params.customerID);
    if (params.gateway) queryParams.set("gateway", params.gateway);
    if (params.status) queryParams.set("status", params.status);
    const query = queryParams.toString() ? `?${queryParams.toString()}` : "";
    return apiClient.get<CursorPaginatedResponse<BillingPayment>>(
      `/admin/billing/payments${query}`
    );
  },

  async refundPayment(
    paymentID: string,
    req: RefundRequest
  ): Promise<RefundResult> {
    await fetchCsrfToken();
    const resp = await apiClient.post<{ data: RefundResult }>(
      `/admin/billing/refund/${paymentID}`,
      req
    );
    return resp.data;
  },

  async getConfig(): Promise<BillingConfig> {
    const resp = await apiClient.get<{ data: BillingConfig }>(
      "/admin/billing/config"
    );
    return resp.data;
  },
};
```

**Commit:**

```
feat(webui): add admin billing API client methods

Add adminBillingApi with getTransactions, creditAdjustment,
getPayments, refundPayment, and getConfig. Supports filtering
by customer, gateway, and status. CSRF token fetched for mutations.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 26: Customer billing page

- [ ] Create `webui/customer/app/billing/page.tsx` with balance, top-up form, and history tables

**File:** `webui/customer/app/billing/page.tsx`

### 26a. Page structure

Create a `"use client"` page component with four sections:

1. **Balance Card** — prominent display of current balance formatted as currency
2. **Top-Up Section** — preset buttons + custom amount input + gateway selector + "Add Funds" button
3. **Transaction History** — table with cursor pagination
4. **Payment History** — table with cursor pagination

### 26b. State management

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
```

Use three independent data-fetching hooks:
- `fetchBalance()` — calls `billingApi.getBalance()`, auto-refresh every 60s
- `fetchTransactions(cursor)` — calls `billingApi.getTransactions()`, cursor pagination with `cursorStack`
- `fetchPayments(cursor)` — calls `billingApi.getPayments()`, cursor pagination with `cursorStack`
- `fetchTopUpConfig()` — calls `billingApi.getTopUpConfig()`, fetched once on mount

### 26c. Top-Up form with Zod validation

```typescript
const topUpSchema = z.object({
  amount: z.number().min(1, "Amount is required"),
  gateway: z.string().min(1, "Payment method is required"),
});

type TopUpFormData = z.infer<typeof topUpSchema>;
```

Form validates `amount` against `config.min_amount_cents` and `config.max_amount_cents` dynamically. Preset buttons set the amount field. Custom input allows free-form entry.

### 26d. Amount formatting helper

```typescript
function formatCents(cents: number, currency = "USD"): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
  }).format(cents / 100);
}
```

### 26e. Top-Up submission handler

```typescript
const handleTopUp = async (data: TopUpFormData) => {
  setIsSubmitting(true);
  try {
    const result = await billingApi.initiateTopUp({
      gateway: data.gateway,
      amount: data.amount,
      currency: config.currency,
      return_url: `${window.location.origin}/billing`,
      cancel_url: `${window.location.origin}/billing`,
    });
    // Redirect to payment gateway
    window.location.href = result.payment_url;
  } catch (error) {
    toast({
      title: "Payment Error",
      description: "Failed to initiate payment. Please try again.",
      variant: "destructive",
    });
  } finally {
    setIsSubmitting(false);
  }
};
```

### 26f. Component layout

Use shadcn/ui components:
- `<Card>` for balance display with large text
- `<Card>` for top-up section with `<Button>` preset chips and `<Input>` for custom amount
- `<Select>` for gateway selection (only show available gateways from config)
- `<Table>` for transaction history (columns: Date, Type, Description, Amount, Balance After)
- `<Table>` for payment history (columns: Date, Gateway, Amount, Status)
- `<Badge>` for transaction type and payment status (variant mapped by type/status)
- `<Button>` for pagination (Previous/Next) with cursor-stack pattern

### 26g. Transaction type badge variants

```typescript
function getTransactionBadgeVariant(type: string) {
  switch (type) {
    case "credit": return "default";    // green
    case "debit": return "destructive"; // red
    case "adjustment": return "outline";
    case "refund": return "secondary";
    default: return "outline";
  }
}

function getPaymentStatusBadgeVariant(status: string) {
  switch (status) {
    case "completed": return "default";
    case "pending": return "outline";
    case "failed": return "destructive";
    case "refunded": return "secondary";
    default: return "outline";
  }
}
```

### 26h. Loading and empty states

Follow the pattern from `vms/page.tsx`:
- `<Loader2>` spinner during loading
- Empty state card with `<CreditCard>` icon and message when no data
- Error toast on fetch failures

**Commit:**

```
feat(webui): add customer billing page with balance, top-up, and history

Customer billing page displays current balance, top-up form with
preset amounts ($5-$100) and custom input, Stripe gateway selector,
transaction history table, and payment history table. All with cursor
pagination and auto-refresh. Amounts in cents, formatted as currency.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 27: Admin billing page

- [ ] Create `webui/admin/app/billing/page.tsx` with overview, payment list, and quick actions

**File:** `webui/admin/app/billing/page.tsx`

### 27a. Page structure

Create a `"use client"` page component with three sections:

1. **Overview Cards** — total payment count, recent payment stats, available gateways
2. **Payments Table** — filterable by gateway, status, customer; cursor pagination
3. **Quick Actions** — refund dialog

### 27b. State management

```typescript
"use client";

import { useState, useEffect, useCallback } from "react";
```

Data-fetching hooks:
- `fetchPayments(filter, cursor)` — calls `adminBillingApi.getPayments()`
- `fetchConfig()` — calls `adminBillingApi.getConfig()`, fetched once on mount

### 27c. Payment table with filters

Use `<Select>` components for gateway and status filters. Include a search input for customer ID.

Table columns: Date, Customer ID, Gateway, Amount, Status, Actions (refund button for completed payments).

### 27d. Refund dialog

Use shadcn/ui `<Dialog>` for refund confirmation:
- Shows payment details (ID, customer, amount, gateway)
- Input for refund amount (pre-filled with full amount)
- Input for reason (required)
- Confirm/Cancel buttons
- On confirm: calls `adminBillingApi.refundPayment()`
- Success toast + refresh payments list
- Error toast on failure

### 27e. Billing config display

Show the current billing configuration in a summary card:
- Available gateways list
- Min/max top-up amounts
- Preset amounts

### 27f. Component layout

Use shadcn/ui components matching the existing admin pages:
- `<Card>`, `<CardHeader>`, `<CardTitle>`, `<CardContent>` for sections
- `<Table>` with column headers for payments
- `<Badge>` for payment status
- `<Button>` for actions and pagination
- `<Dialog>` for refund confirmation
- `<Input>` and `<Select>` for filters
- `<Tabs>` if needed to separate payments from transactions view

**Commit:**

```
feat(webui): add admin billing page with payments and refunds

Admin billing page displays payment list with gateway/status/customer
filters, refund dialog for completed payments, and billing config
overview. Cursor pagination. Refund requires reason and audit logs.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 28: Customer navigation update

- [ ] Add "Billing" nav item to customer portal navigation

**File:** `webui/customer/lib/navigation.ts` (or `nav-items.ts` — use whichever exists)

### 28a. Add billing nav item

Add between "My VMs" and "Settings":

```typescript
import { CreditCard } from "lucide-react";

// In navItems array:
{ href: "/billing", label: "Billing", icon: CreditCard },
```

Full array should be:
```typescript
export const navItems: NavItem[] = [
  { href: "/vms", label: "My VMs", icon: Monitor },
  { href: "/billing", label: "Billing", icon: CreditCard },
  { href: "/settings", label: "Settings", icon: Settings },
];
```

**Commit:**

```
feat(webui): add Billing to customer portal navigation

Add CreditCard icon nav item for /billing between My VMs and Settings.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 29: Admin navigation update

- [ ] Add "Billing" nav item to admin portal navigation

**File:** `webui/admin/lib/navigation.ts`

### 29a. Add billing nav item

Add after "Customers" and before "Audit Logs":

```typescript
import { CreditCard } from "lucide-react";

// In adminNavItems array, add:
{ href: "/billing", label: "Billing", icon: CreditCard },
```

**Commit:**

```
feat(webui): add Billing to admin portal navigation

Add CreditCard icon nav item for /billing after Customers entry.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 30: Frontend type-check + build verification

- [ ] Verify both frontends type-check and build successfully

### 30a. Customer portal

```bash
cd webui/customer && npm ci && npm run type-check && npm run build
```

### 30b. Admin portal

```bash
cd webui/admin && npm ci && npm run type-check && npm run build
```

### 30c. Fix any issues

Address any TypeScript compilation errors or build failures.

**Commit (if fixes needed):**

```
fix(webui): resolve TypeScript errors in billing pages

Fix type errors found during build verification for customer and
admin billing pages.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 31: Backend build + test verification

- [ ] Verify the backend builds and all tests pass

### 31a. Build

```bash
make build-controller
```

### 31b. Run tests

```bash
make test-race
```

### 31c. Lint (if golangci-lint available)

```bash
make lint
```

### 31d. Verify no forbidden patterns

```bash
grep -rn "TODO\|FIXME\|HACK" \
    internal/controller/payments/ \
    internal/controller/api/webhooks/ \
    internal/controller/services/payment_service.go \
    internal/controller/services/payment_service_test.go \
    internal/controller/api/customer/billing.go \
    internal/controller/api/admin/billing.go 2>/dev/null
# Expected: no output
```

### 31e. Verify function length compliance

Spot-check that no function exceeds 40 lines and nesting doesn't exceed 3 levels.

**Commit:**

```
chore(billing): Phase 4 final verification — all tests pass

Verified build, unit tests with race detector, lint, and coding
standard compliance for the complete billing Phase 4 implementation.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## File Summary

### New Files Created

| File | Purpose |
|------|---------|
| `internal/controller/payments/provider.go` | PaymentProvider interface + shared types |
| `internal/controller/payments/provider_test.go` | Interface compilation check |
| `internal/controller/payments/registry.go` | PaymentRegistry for multi-gateway management |
| `internal/controller/payments/registry_test.go` | Registry tests |
| `internal/controller/payments/stripe/provider.go` | Stripe PaymentProvider implementation |
| `internal/controller/payments/stripe/helpers.go` | Stripe JSON parsing helper |
| `internal/controller/payments/stripe/provider_test.go` | Stripe provider tests |
| `internal/controller/services/payment_service.go` | PaymentService (top-up, webhook, refund) |
| `internal/controller/services/payment_service_test.go` | PaymentService tests |
| `internal/controller/api/webhooks/stripe.go` | Stripe webhook HTTP handler |
| `internal/controller/api/webhooks/stripe_test.go` | Webhook handler tests |
| `webui/customer/app/billing/page.tsx` | Customer billing page |
| `webui/admin/app/billing/page.tsx` | Admin billing page |

### Existing Files Modified

| File | Changes |
|------|---------|
| `go.mod`, `go.sum` | Add `github.com/stripe/stripe-go/v82` |
| `internal/controller/models/billing_payment.go` | Add `TopUpRequest`, `TopUpResponse`, `RefundRequest` |
| `internal/controller/repository/billing_payment_repo.go` | Add `ListAll` method with multi-filter support |
| `internal/controller/api/customer/billing.go` | Add `InitiateTopUp`, `GetPaymentHistory`, `GetTopUpConfig` handlers |
| `internal/controller/api/customer/billing_test.go` | Add tests for new handlers |
| `internal/controller/api/customer/handler.go` | Add `paymentService` field |
| `internal/controller/api/customer/routes.go` | Register top-up, payments, config routes |
| `internal/controller/api/admin/billing.go` | Add `ListPayments`, `RefundPayment`, `GetBillingConfig` handlers |
| `internal/controller/api/admin/billing_test.go` | Add tests for new handlers |
| `internal/controller/api/admin/handler.go` | Add `paymentService` field |
| `internal/controller/api/admin/routes.go` | Register payments, refund, config routes |
| `internal/controller/dependencies.go` | Wire PaymentRegistry, Stripe provider, PaymentService, webhook handler |
| `internal/controller/server.go` | Add `stripeWebhookHandler` field, register webhook route |
| `webui/customer/lib/api-client.ts` | Add billing types and `billingApi` object |
| `webui/customer/lib/navigation.ts` | Add "Billing" nav item |
| `webui/admin/lib/api-client.ts` | Add billing types and `adminBillingApi` object |
| `webui/admin/lib/navigation.ts` | Add "Billing" nav item |

### Environment Variables (from Phase 1, used here)

| Variable | Type | Default | Required When |
|----------|------|---------|---------------|
| `STRIPE_SECRET_KEY` | Secret | — | Stripe enabled |
| `STRIPE_WEBHOOK_SECRET` | Secret | — | Stripe enabled |
| `STRIPE_PUBLISHABLE_KEY` | string | — | Stripe enabled (frontend) |

### System Settings Keys (admin-configurable)

| Key | Type | Default | Purpose |
|-----|------|---------|---------|
| `billing.topup.min_amount` | int64 | 500 (=$5) | Minimum top-up in cents |
| `billing.topup.max_amount` | int64 | 50000 (=$500) | Maximum top-up in cents |
| `billing.topup.presets` | int64[] | [500,1000,2500,5000,10000] | Preset amount buttons |
