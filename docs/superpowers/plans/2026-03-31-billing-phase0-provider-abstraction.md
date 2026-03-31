# Billing Phase 0: Provider Abstraction + WHMCS Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the BillingProvider abstraction layer and wrap the existing WHMCS integration behind it, enabling future billing providers (native, Blesta) to plug in without modifying core controller logic.

**Architecture:** New `internal/controller/billing/` package with interface, registry, and WHMCS adapter. Single migration adds `billing_provider` column to customers. VMService gets lifecycle hooks that delegate to the appropriate billing provider.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgx/v5

**Depends on:** Nothing (first phase)
**Depended on by:** All subsequent billing phases

---

## Step 1: Migration 000072 — Add `billing_provider` column

- [ ] Create migration files

**Files to create:**
- `migrations/000072_add_billing_provider.up.sql`
- `migrations/000072_add_billing_provider.down.sql`

### 1a. Up migration

**File:** `migrations/000072_add_billing_provider.up.sql`

```sql
SET lock_timeout = '5s';

ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS billing_provider VARCHAR(20)
    CHECK (billing_provider IN ('whmcs', 'native', 'blesta', 'unmanaged'));

-- Backfill from existing ownership data.
-- Customers with a WHMCS client ID are assigned to 'whmcs'.
-- All others are assigned to 'unmanaged' (requires explicit operator assignment).
UPDATE customers
SET billing_provider = CASE
    WHEN whmcs_client_id IS NOT NULL THEN 'whmcs'
    ELSE 'unmanaged'
END
WHERE billing_provider IS NULL;

ALTER TABLE customers
    ALTER COLUMN billing_provider SET NOT NULL;

COMMENT ON COLUMN customers.billing_provider IS
    'Which billing system manages this customer: whmcs, native, blesta, or unmanaged';
```

### 1b. Down migration

**File:** `migrations/000072_add_billing_provider.down.sql`

```sql
SET lock_timeout = '5s';

ALTER TABLE customers DROP COLUMN IF EXISTS billing_provider;
```

**Verify:**
```bash
# Check migration files exist and are valid SQL
head -5 migrations/000072_add_billing_provider.up.sql
head -5 migrations/000072_add_billing_provider.down.sql
```

**Commit:**
```
feat(billing): add billing_provider column to customers (migration 000072)

Adds VARCHAR(20) billing_provider column with CHECK constraint.
Backfills from whmcs_client_id: non-null → 'whmcs', null → 'unmanaged'.
Sets NOT NULL after backfill. Down migration drops the column.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 2: Add `ErrNotSupported` sentinel error

- [ ] Add sentinel error to shared errors package

**File:** `internal/shared/errors/errors.go`

Add after the existing `ErrLimitExceeded` sentinel:

```go
// ErrNotSupported indicates that an operation is not supported by the provider.
ErrNotSupported = stderrors.New("operation not supported")
```

**Verify:**
```bash
go build ./internal/shared/errors/...
```

**Commit:**
```
feat(errors): add ErrNotSupported sentinel error

Used by billing providers that do not support specific operations
(e.g., WHMCS adapter does not support balance queries).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 3: BillingProvider interface + types

- [ ] Create billing provider interface and supporting types

**File to create:** `internal/controller/billing/provider.go`

```go
// Package billing provides the billing provider abstraction layer.
// Each billing source (WHMCS, native, Blesta) implements the BillingProvider
// interface, and the Registry routes lifecycle events to the correct provider
// based on each customer's billing_provider column.
package billing

import (
	"context"
	"time"
)

// BillingProvider is the interface that all billing sources must implement.
// WHMCS is a no-op adapter (WHMCS drives via the Provisioning API).
// Native billing performs ledger operations. Blesta is a future stub.
type BillingProvider interface {
	// Name returns the provider identifier ("whmcs", "native", "blesta").
	Name() string

	// ValidateConfig checks whether the provider's configuration is valid.
	ValidateConfig() error

	// CreateUser is called when a new customer is created for this provider.
	CreateUser(ctx context.Context, req CreateUserRequest) (*UserResult, error)

	// GetUserBillingStatus returns the billing status for a customer.
	GetUserBillingStatus(ctx context.Context, customerID string) (*BillingStatus, error)

	// OnVMCreated is called after a VM is successfully provisioned.
	OnVMCreated(ctx context.Context, vm VMRef) error

	// OnVMDeleted is called when a VM is terminated.
	OnVMDeleted(ctx context.Context, vm VMRef) error

	// OnVMResized is called when a VM's plan changes.
	OnVMResized(ctx context.Context, vm VMRef, oldPlanID, newPlanID string) error

	// SuspendForNonPayment suspends all VMs for a customer due to non-payment.
	SuspendForNonPayment(ctx context.Context, customerID string) error

	// UnsuspendAfterPayment reactivates a customer after payment is received.
	UnsuspendAfterPayment(ctx context.Context, customerID string) error

	// GetBalance returns the current billing balance for a customer.
	GetBalance(ctx context.Context, customerID string) (*Balance, error)

	// ProcessTopUp processes a credit top-up for a customer.
	ProcessTopUp(ctx context.Context, req TopUpRequest) (*TopUpResult, error)

	// GetUsageHistory returns paginated billing usage history.
	GetUsageHistory(ctx context.Context, customerID string, opts PaginationOpts) (*UsageHistory, error)
}

// VMRef is a lightweight reference to a VM for billing lifecycle hooks.
type VMRef struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	PlanID     string `json:"plan_id"`
	Hostname   string `json:"hostname"`
}

// CreateUserRequest holds the data needed to create a billing user.
type CreateUserRequest struct {
	CustomerID string `json:"customer_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}

// UserResult is the response from a successful billing user creation.
type UserResult struct {
	CustomerID string `json:"customer_id"`
	ProviderID string `json:"provider_id"`
}

// BillingStatus represents the billing state of a customer.
type BillingStatus struct {
	CustomerID string `json:"customer_id"`
	Provider   string `json:"provider"`
	IsActive   bool   `json:"is_active"`
	IsSuspended bool  `json:"is_suspended"`
}

// Balance represents a customer's current billing balance.
type Balance struct {
	CustomerID   string `json:"customer_id"`
	BalanceCents int64  `json:"balance_cents"`
	Currency     string `json:"currency"`
}

// TopUpRequest holds the data for a credit top-up operation.
type TopUpRequest struct {
	CustomerID  string `json:"customer_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Reference   string `json:"reference"`
}

// TopUpResult is the response from a successful top-up.
type TopUpResult struct {
	TransactionID string `json:"transaction_id"`
	BalanceCents  int64  `json:"balance_cents"`
	Currency      string `json:"currency"`
}

// PaginationOpts holds pagination parameters for list operations.
type PaginationOpts struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// UsageHistory holds paginated billing usage records.
type UsageHistory struct {
	Records    []UsageRecord `json:"records"`
	TotalCount int           `json:"total_count"`
	Page       int           `json:"page"`
	PerPage    int           `json:"per_page"`
}

// UsageRecord represents a single billing usage entry.
type UsageRecord struct {
	ID          string    `json:"id"`
	VMID        string    `json:"vm_id"`
	Description string    `json:"description"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"created_at"`
}
```

**Verify:**
```bash
go build ./internal/controller/billing/...
```

**Commit:**
```
feat(billing): add BillingProvider interface and types

Defines the billing abstraction layer with lifecycle hooks (OnVMCreated,
OnVMDeleted, OnVMResized), balance operations, and usage history.
Types: VMRef, CreateUserRequest, UserResult, BillingStatus, Balance,
TopUpRequest, TopUpResult, PaginationOpts, UsageHistory, UsageRecord.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 4: Tests for billing types

- [ ] Add type validation tests

**File to create:** `internal/controller/billing/provider_test.go`

```go
package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVMRef_Fields(t *testing.T) {
	ref := VMRef{
		ID:         "vm-123",
		CustomerID: "cust-456",
		PlanID:     "plan-789",
		Hostname:   "test-vm",
	}
	assert.Equal(t, "vm-123", ref.ID)
	assert.Equal(t, "cust-456", ref.CustomerID)
	assert.Equal(t, "plan-789", ref.PlanID)
	assert.Equal(t, "test-vm", ref.Hostname)
}

func TestCreateUserRequest_Fields(t *testing.T) {
	req := CreateUserRequest{
		CustomerID: "cust-123",
		Email:      "user@example.com",
		Name:       "Test User",
	}
	assert.Equal(t, "cust-123", req.CustomerID)
	assert.Equal(t, "user@example.com", req.Email)
	assert.Equal(t, "Test User", req.Name)
}

func TestPaginationOpts_Defaults(t *testing.T) {
	opts := PaginationOpts{}
	assert.Equal(t, 0, opts.Page)
	assert.Equal(t, 0, opts.PerPage)
}

func TestBalance_Fields(t *testing.T) {
	b := Balance{
		CustomerID:   "cust-123",
		BalanceCents: 5000,
		Currency:     "USD",
	}
	assert.Equal(t, "cust-123", b.CustomerID)
	assert.Equal(t, int64(5000), b.BalanceCents)
	assert.Equal(t, "USD", b.Currency)
}

func TestTopUpRequest_Fields(t *testing.T) {
	req := TopUpRequest{
		CustomerID:  "cust-123",
		AmountCents: 1000,
		Currency:    "USD",
		Reference:   "stripe_pi_abc",
	}
	assert.Equal(t, "cust-123", req.CustomerID)
	assert.Equal(t, int64(1000), req.AmountCents)
	assert.Equal(t, "USD", req.Currency)
	assert.Equal(t, "stripe_pi_abc", req.Reference)
}

func TestUsageHistory_Empty(t *testing.T) {
	h := UsageHistory{
		Records:    []UsageRecord{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
	}
	assert.Empty(t, h.Records)
	assert.Equal(t, 0, h.TotalCount)
	assert.Equal(t, 1, h.Page)
	assert.Equal(t, 20, h.PerPage)
}
```

**Verify:**
```bash
go test -race ./internal/controller/billing/...
# Expected: PASS
```

**Commit:**
```
test(billing): add billing type validation tests

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 5: Registry implementation

- [ ] Create provider registry

**File to create:** `internal/controller/billing/registry.go`

```go
package billing

import (
	"fmt"
	"log/slog"
	"sync"
)

// Registry manages all enabled billing providers and routes lifecycle
// events to the correct provider based on each customer's billing_provider
// column value.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]BillingProvider
	primary   string
	logger    *slog.Logger
}

// NewRegistry creates an empty billing provider registry.
// Providers are registered via Register(). The primary provider is used
// for new self-registered customers.
func NewRegistry(primary string, logger *slog.Logger) *Registry {
	return &Registry{
		providers: make(map[string]BillingProvider),
		primary:   primary,
		logger:    logger.With("component", "billing-registry"),
	}
}

// Register adds a billing provider to the registry.
// Returns an error if a provider with the same name is already registered.
func (r *Registry) Register(p BillingProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("billing provider %q already registered", name)
	}
	r.providers[name] = p
	r.logger.Info("billing provider registered", "provider", name)
	return nil
}

// ForCustomer returns the billing provider that manages a specific customer.
// The providerName is the value of the customer's billing_provider column.
func (r *Registry) ForCustomer(providerName string) (BillingProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("billing provider %q not registered", providerName)
	}
	return p, nil
}

// Primary returns the default provider for new self-registered customers.
// Returns nil if no primary provider is configured or registered.
func (r *Registry) Primary() BillingProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.primary == "" {
		return nil
	}
	return r.providers[r.primary]
}

// All returns all registered billing providers.
func (r *Registry) All() []BillingProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]BillingProvider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// HasProvider checks whether a provider with the given name is registered.
func (r *Registry) HasProvider(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.providers[name]
	return ok
}
```

**Verify:**
```bash
go build ./internal/controller/billing/...
```

**Commit:**
```
feat(billing): add provider registry

Thread-safe registry maps provider names to BillingProvider implementations.
ForCustomer(name) returns the provider for a customer's billing_provider
column value. Primary() returns the default for self-registered users.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 6: Registry tests

- [ ] Add registry unit tests

**File to create:** `internal/controller/billing/registry_test.go`

```go
package billing

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubProvider implements BillingProvider for testing.
type stubProvider struct {
	name string
}

func (s *stubProvider) Name() string                { return s.name }
func (s *stubProvider) ValidateConfig() error        { return nil }
func (s *stubProvider) CreateUser(_ context.Context, _ CreateUserRequest) (*UserResult, error) {
	return nil, nil
}
func (s *stubProvider) GetUserBillingStatus(_ context.Context, _ string) (*BillingStatus, error) {
	return nil, nil
}
func (s *stubProvider) OnVMCreated(_ context.Context, _ VMRef) error  { return nil }
func (s *stubProvider) OnVMDeleted(_ context.Context, _ VMRef) error  { return nil }
func (s *stubProvider) OnVMResized(_ context.Context, _ VMRef, _, _ string) error {
	return nil
}
func (s *stubProvider) SuspendForNonPayment(_ context.Context, _ string) error  { return nil }
func (s *stubProvider) UnsuspendAfterPayment(_ context.Context, _ string) error { return nil }
func (s *stubProvider) GetBalance(_ context.Context, _ string) (*Balance, error) {
	return nil, nil
}
func (s *stubProvider) ProcessTopUp(_ context.Context, _ TopUpRequest) (*TopUpResult, error) {
	return nil, nil
}
func (s *stubProvider) GetUsageHistory(_ context.Context, _ string, _ PaginationOpts) (*UsageHistory, error) {
	return nil, nil
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name      string
		providers []string
		wantErr   bool
	}{
		{"single provider", []string{"whmcs"}, false},
		{"multiple providers", []string{"whmcs", "native"}, false},
		{"duplicate provider", []string{"whmcs", "whmcs"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry("", testLogger())
			var lastErr error
			for _, name := range tt.providers {
				lastErr = reg.Register(&stubProvider{name: name})
			}
			if tt.wantErr {
				require.Error(t, lastErr)
			} else {
				require.NoError(t, lastErr)
			}
		})
	}
}

func TestRegistry_ForCustomer(t *testing.T) {
	tests := []struct {
		name         string
		registered   []string
		lookup       string
		wantErr      bool
		wantProvider string
	}{
		{"existing provider", []string{"whmcs"}, "whmcs", false, "whmcs"},
		{"missing provider", []string{"whmcs"}, "native", true, ""},
		{"empty lookup", []string{"whmcs"}, "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry("", testLogger())
			for _, name := range tt.registered {
				require.NoError(t, reg.Register(&stubProvider{name: name}))
			}
			p, err := reg.ForCustomer(tt.lookup)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, p)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantProvider, p.Name())
			}
		})
	}
}

func TestRegistry_Primary(t *testing.T) {
	tests := []struct {
		name       string
		primary    string
		registered []string
		wantNil    bool
	}{
		{"no primary set", "", []string{"whmcs"}, true},
		{"primary registered", "whmcs", []string{"whmcs"}, false},
		{"primary not registered", "native", []string{"whmcs"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(tt.primary, testLogger())
			for _, name := range tt.registered {
				require.NoError(t, reg.Register(&stubProvider{name: name}))
			}
			p := reg.Primary()
			if tt.wantNil {
				assert.Nil(t, p)
			} else {
				require.NotNil(t, p)
				assert.Equal(t, tt.primary, p.Name())
			}
		})
	}
}

func TestRegistry_All(t *testing.T) {
	reg := NewRegistry("", testLogger())
	require.NoError(t, reg.Register(&stubProvider{name: "whmcs"}))
	require.NoError(t, reg.Register(&stubProvider{name: "native"}))

	all := reg.All()
	assert.Len(t, all, 2)

	names := make(map[string]bool)
	for _, p := range all {
		names[p.Name()] = true
	}
	assert.True(t, names["whmcs"])
	assert.True(t, names["native"])
}

func TestRegistry_HasProvider(t *testing.T) {
	reg := NewRegistry("", testLogger())
	require.NoError(t, reg.Register(&stubProvider{name: "whmcs"}))

	assert.True(t, reg.HasProvider("whmcs"))
	assert.False(t, reg.HasProvider("native"))
}
```

**Verify:**
```bash
go test -race ./internal/controller/billing/...
# Expected: PASS
```

**Commit:**
```
test(billing): add registry unit tests

Covers Register (single, multiple, duplicate), ForCustomer (found,
missing, empty), Primary (set, unset, not registered), All, HasProvider.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 7: WHMCS adapter implementation

- [ ] Create WHMCS billing adapter

**File to create:** `internal/controller/billing/whmcs/adapter.go`

```go
// Package whmcs implements the BillingProvider interface for WHMCS.
// The WHMCS adapter is intentionally a no-op for lifecycle hooks because
// WHMCS drives billing operations externally via the Provisioning REST API.
// Balance, top-up, and usage operations return ErrNotSupported since WHMCS
// manages those concerns internally.
package whmcs

import (
	"context"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// Adapter implements billing.BillingProvider for WHMCS.
// All lifecycle hooks are no-ops: WHMCS initiates actions via the Provisioning
// API and is notified of results via system webhooks.
type Adapter struct{}

// NewAdapter creates a new WHMCS billing adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns the provider identifier.
func (a *Adapter) Name() string {
	return "whmcs"
}

// ValidateConfig checks WHMCS configuration.
// WHMCS configuration is managed externally (PHP module), so validation
// always succeeds on the Go side.
func (a *Adapter) ValidateConfig() error {
	return nil
}

// CreateUser is a no-op for WHMCS.
// WHMCS creates users via the Provisioning API's CreateOrGetCustomer endpoint.
func (a *Adapter) CreateUser(_ context.Context, req billing.CreateUserRequest) (*billing.UserResult, error) {
	return &billing.UserResult{
		CustomerID: req.CustomerID,
		ProviderID: req.CustomerID,
	}, nil
}

// GetUserBillingStatus returns a default active status for WHMCS customers.
// WHMCS manages billing status internally; the Go side assumes active.
func (a *Adapter) GetUserBillingStatus(_ context.Context, customerID string) (*billing.BillingStatus, error) {
	return &billing.BillingStatus{
		CustomerID:  customerID,
		Provider:    "whmcs",
		IsActive:    true,
		IsSuspended: false,
	}, nil
}

// OnVMCreated is a no-op. WHMCS tracks VM creation via its own database
// after receiving a success response from the Provisioning API.
func (a *Adapter) OnVMCreated(_ context.Context, _ billing.VMRef) error {
	return nil
}

// OnVMDeleted is a no-op. WHMCS tracks VM deletion via system webhooks.
func (a *Adapter) OnVMDeleted(_ context.Context, _ billing.VMRef) error {
	return nil
}

// OnVMResized is a no-op. WHMCS handles plan changes via ChangePackage
// in the PHP module, which calls the Provisioning API's resize endpoint.
func (a *Adapter) OnVMResized(_ context.Context, _ billing.VMRef, _, _ string) error {
	return nil
}

// SuspendForNonPayment is a no-op. WHMCS triggers suspension via the
// Provisioning API's suspend endpoint.
func (a *Adapter) SuspendForNonPayment(_ context.Context, _ string) error {
	return nil
}

// UnsuspendAfterPayment is a no-op. WHMCS triggers unsuspension via the
// Provisioning API's unsuspend endpoint.
func (a *Adapter) UnsuspendAfterPayment(_ context.Context, _ string) error {
	return nil
}

// GetBalance is not supported by WHMCS. Balance is managed in the WHMCS
// billing system, not in VirtueStack.
func (a *Adapter) GetBalance(_ context.Context, _ string) (*billing.Balance, error) {
	return nil, fmt.Errorf("get balance: %w", sharederrors.ErrNotSupported)
}

// ProcessTopUp is not supported by WHMCS. Top-ups are handled within the
// WHMCS client area.
func (a *Adapter) ProcessTopUp(_ context.Context, _ billing.TopUpRequest) (*billing.TopUpResult, error) {
	return nil, fmt.Errorf("process top-up: %w", sharederrors.ErrNotSupported)
}

// GetUsageHistory is not supported by WHMCS. Usage history is available
// within the WHMCS client area.
func (a *Adapter) GetUsageHistory(_ context.Context, _ string, _ billing.PaginationOpts) (*billing.UsageHistory, error) {
	return nil, fmt.Errorf("get usage history: %w", sharederrors.ErrNotSupported)
}

// Compile-time interface compliance check.
var _ billing.BillingProvider = (*Adapter)(nil)
```

**Verify:**
```bash
go build ./internal/controller/billing/whmcs/...
```

**Commit:**
```
feat(billing): add WHMCS adapter (no-op lifecycle hooks)

Implements BillingProvider for WHMCS. Lifecycle hooks (OnVMCreated,
OnVMDeleted, OnVMResized) return nil since WHMCS drives via the
Provisioning API. Balance/TopUp/UsageHistory return ErrNotSupported.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 8: WHMCS adapter tests

- [ ] Add WHMCS adapter unit tests

**File to create:** `internal/controller/billing/whmcs/adapter_test.go`

```go
package whmcs

import (
	"context"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_Name(t *testing.T) {
	adapter := NewAdapter()
	assert.Equal(t, "whmcs", adapter.Name())
}

func TestAdapter_ValidateConfig(t *testing.T) {
	adapter := NewAdapter()
	assert.NoError(t, adapter.ValidateConfig())
}

func TestAdapter_CreateUser(t *testing.T) {
	adapter := NewAdapter()
	ctx := context.Background()

	result, err := adapter.CreateUser(ctx, billing.CreateUserRequest{
		CustomerID: "cust-123",
		Email:      "test@example.com",
		Name:       "Test User",
	})
	require.NoError(t, err)
	assert.Equal(t, "cust-123", result.CustomerID)
	assert.Equal(t, "cust-123", result.ProviderID)
}

func TestAdapter_GetUserBillingStatus(t *testing.T) {
	adapter := NewAdapter()
	ctx := context.Background()

	status, err := adapter.GetUserBillingStatus(ctx, "cust-123")
	require.NoError(t, err)
	assert.Equal(t, "cust-123", status.CustomerID)
	assert.Equal(t, "whmcs", status.Provider)
	assert.True(t, status.IsActive)
	assert.False(t, status.IsSuspended)
}

func TestAdapter_LifecycleHooksAreNoOps(t *testing.T) {
	adapter := NewAdapter()
	ctx := context.Background()
	ref := billing.VMRef{
		ID:         "vm-123",
		CustomerID: "cust-456",
		PlanID:     "plan-789",
		Hostname:   "test-vm",
	}

	tests := []struct {
		name string
		fn   func() error
	}{
		{"OnVMCreated", func() error { return adapter.OnVMCreated(ctx, ref) }},
		{"OnVMDeleted", func() error { return adapter.OnVMDeleted(ctx, ref) }},
		{"OnVMResized", func() error { return adapter.OnVMResized(ctx, ref, "old-plan", "new-plan") }},
		{"SuspendForNonPayment", func() error { return adapter.SuspendForNonPayment(ctx, "cust-456") }},
		{"UnsuspendAfterPayment", func() error { return adapter.UnsuspendAfterPayment(ctx, "cust-456") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, tt.fn())
		})
	}
}

func TestAdapter_UnsupportedOperations(t *testing.T) {
	adapter := NewAdapter()
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"GetBalance", func() error {
			_, err := adapter.GetBalance(ctx, "cust-123")
			return err
		}},
		{"ProcessTopUp", func() error {
			_, err := adapter.ProcessTopUp(ctx, billing.TopUpRequest{CustomerID: "cust-123"})
			return err
		}},
		{"GetUsageHistory", func() error {
			_, err := adapter.GetUsageHistory(ctx, "cust-123", billing.PaginationOpts{})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.Error(t, err)
			assert.ErrorIs(t, err, sharederrors.ErrNotSupported)
		})
	}
}

func TestAdapter_ImplementsInterface(t *testing.T) {
	var _ billing.BillingProvider = (*Adapter)(nil)
}
```

**Verify:**
```bash
go test -race ./internal/controller/billing/whmcs/...
# Expected: PASS
```

**Commit:**
```
test(billing): add WHMCS adapter unit tests

Covers Name, ValidateConfig, CreateUser, GetUserBillingStatus,
lifecycle hook no-ops, and ErrNotSupported for unsupported operations.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 9: Customer model update

- [ ] Add `BillingProvider` field to Customer struct

**File:** `internal/controller/models/customer.go`

Add the `BillingProvider` field after `WHMCSClientID` in the `Customer` struct and add billing provider constants:

```go
// Billing provider constants define which billing system manages a customer.
const (
	BillingProviderWHMCS     = "whmcs"
	BillingProviderNative    = "native"
	BillingProviderBlesta    = "blesta"
	BillingProviderUnmanaged = "unmanaged"
)
```

Add field to the `Customer` struct, after `WHMCSClientID`:

```go
BillingProvider string `json:"billing_provider" db:"billing_provider"`
```

**Exact edit — add constants before `CustomerStatusActive`:**

In `internal/controller/models/customer.go`, insert the billing provider constants block before the existing customer status constants, and add the field to the struct.

The Customer struct should become:

```go
type Customer struct {
	ID                   string   `json:"id" db:"id"`
	Email                string   `json:"email" db:"email"`
	PasswordHash         string   `json:"-" db:"password_hash"`
	Name                 string   `json:"name" db:"name"`
	Phone                *string  `json:"phone,omitempty" db:"phone"`
	WHMCSClientID        *int     `json:"whmcs_client_id,omitempty" db:"whmcs_client_id"`
	BillingProvider      string   `json:"billing_provider" db:"billing_provider"`
	TOTPSecretEncrypted  *string  `json:"-" db:"totp_secret_encrypted"`
	TOTPEnabled          bool     `json:"totp_enabled" db:"totp_enabled"`
	TOTPBackupCodesHash  []string `json:"-" db:"totp_backup_codes_hash"`
	TOTPBackupCodesShown bool     `json:"-" db:"totp_backup_codes_shown"`
	Status               string   `json:"status" db:"status"`
	Timestamps
}
```

**Verify:**
```bash
go build ./internal/controller/models/...
```

**Commit:**
```
feat(models): add BillingProvider field to Customer struct

Adds billing_provider string field and constants (whmcs, native,
blesta, unmanaged) matching the CHECK constraint in migration 000072.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 10: Customer repository update

- [ ] Update customer repository for billing_provider column

**File:** `internal/controller/repository/customer_repo.go`

### 10a. Update `scanCustomer` to include `billing_provider`

The scan order must match `customerSelectCols`. Add `&c.BillingProvider` after `&c.WHMCSClientID`:

```go
func scanCustomer(row pgx.Row) (models.Customer, error) {
	var c models.Customer
	err := row.Scan(
		&c.ID, &c.Email, &c.PasswordHash, &c.Name, &c.Phone,
		&c.WHMCSClientID, &c.BillingProvider, &c.TOTPSecretEncrypted, &c.TOTPEnabled,
		&c.TOTPBackupCodesHash, &c.TOTPBackupCodesShown, &c.Status,
		&c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}
```

### 10b. Update `customerSelectCols` to include `billing_provider`

```go
const customerSelectCols = `
	id, email, password_hash, name, phone,
	whmcs_client_id, billing_provider, totp_secret_encrypted, totp_enabled,
	totp_backup_codes_hash, totp_backup_codes_shown, status,
	created_at, updated_at`
```

### 10c. Update `Create` method to include `billing_provider`

The INSERT statement must include `billing_provider`:

```go
func (r *CustomerRepository) Create(ctx context.Context, customer *models.Customer) error {
	const q = `
		INSERT INTO customers (
			email, password_hash, name, whmcs_client_id, billing_provider,
			totp_secret_encrypted, totp_enabled, totp_backup_codes_hash, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING ` + customerSelectCols

	row := r.db.QueryRow(ctx, q,
		customer.Email, customer.PasswordHash, customer.Name, customer.WHMCSClientID,
		customer.BillingProvider,
		customer.TOTPSecretEncrypted, customer.TOTPEnabled, customer.TOTPBackupCodesHash, customer.Status,
	)
	created, err := scanCustomer(row)
	if err != nil {
		return fmt.Errorf("creating customer: %w", err)
	}
	*customer = created
	return nil
}
```

### 10d. Add `UpdateBillingProvider` method

Add after the existing `UpdateWHMCSClientID` method:

```go
// UpdateBillingProvider updates the billing provider assignment for a customer.
func (r *CustomerRepository) UpdateBillingProvider(ctx context.Context, id, provider string) error {
	const q = `UPDATE customers SET billing_provider = $1, updated_at = NOW() WHERE id = $2 AND status != 'deleted'`
	tag, err := r.db.Exec(ctx, q, provider, id)
	if err != nil {
		return fmt.Errorf("updating customer %s billing provider: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating customer %s billing provider: %w", id, ErrNoRowsAffected)
	}
	return nil
}
```

### 10e. Update `CustomerRepo` interface

Add `UpdateBillingProvider` to the interface:

```go
UpdateBillingProvider(ctx context.Context, id, provider string) error
```

**Verify:**
```bash
go build ./internal/controller/repository/...
```

**Commit:**
```
feat(repo): add billing_provider to customer repository

Updates scanCustomer, customerSelectCols, Create, and the CustomerRepo
interface. Adds UpdateBillingProvider method for changing a customer's
billing provider assignment.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 11: Customer repository tests

- [ ] Add tests for billing_provider repository changes

**File to create:** `internal/controller/repository/customer_billing_test.go`

```go
package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBillingDB struct {
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (f *fakeBillingDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFunc != nil {
		return f.queryRowFunc(ctx, sql, args...)
	}
	return &fakeBillingRow{err: pgx.ErrNoRows}
}

func (f *fakeBillingDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeBillingDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (f *fakeBillingDB) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, fmt.Errorf("not implemented")
}

type fakeBillingRow struct {
	err error
}

func (r *fakeBillingRow) Scan(_ ...any) error {
	return r.err
}

func TestCustomerRepository_UpdateBillingProvider(t *testing.T) {
	tests := []struct {
		name     string
		affected int64
		execErr  error
		wantErr  bool
	}{
		{"success", 1, nil, false},
		{"no rows affected", 0, nil, true},
		{"db error", 0, fmt.Errorf("connection refused"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeBillingDB{
				execFunc: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.affected)), nil
				},
			}
			repo := NewCustomerRepository(db)
			err := repo.UpdateBillingProvider(context.Background(), "cust-123", models.BillingProviderWHMCS)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCustomerBillingProviderConstants(t *testing.T) {
	assert.Equal(t, "whmcs", models.BillingProviderWHMCS)
	assert.Equal(t, "native", models.BillingProviderNative)
	assert.Equal(t, "blesta", models.BillingProviderBlesta)
	assert.Equal(t, "unmanaged", models.BillingProviderUnmanaged)
}
```

**Verify:**
```bash
go test -race ./internal/controller/repository/... -run TestCustomerRepository_UpdateBillingProvider
go test -race ./internal/controller/repository/... -run TestCustomerBillingProviderConstants
# Expected: PASS
```

**Commit:**
```
test(repo): add customer billing_provider repository tests

Covers UpdateBillingProvider success, no-rows-affected, and db error
cases. Validates billing provider constants.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 12: BillingHook interface for VMService

- [ ] Define the billing hook interface consumed by VMService

**File to create:** `internal/controller/billing/hook.go`

```go
package billing

import "context"

// VMLifecycleHook is the subset of BillingProvider consumed by VMService.
// VMService calls these hooks after VM lifecycle events. Using an interface
// avoids a direct dependency on the full BillingProvider or Registry.
type VMLifecycleHook interface {
	// OnVMCreated is called after a VM is successfully provisioned.
	OnVMCreated(ctx context.Context, vm VMRef) error

	// OnVMDeleted is called when a VM is terminated.
	OnVMDeleted(ctx context.Context, vm VMRef) error

	// OnVMResized is called when a VM's plan changes.
	OnVMResized(ctx context.Context, vm VMRef, oldPlanID, newPlanID string) error
}
```

**Verify:**
```bash
go build ./internal/controller/billing/...
```

**Commit:**
```
feat(billing): add VMLifecycleHook interface

Narrow interface consumed by VMService for billing lifecycle callbacks.
Avoids coupling VMService to the full BillingProvider or Registry.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 13: VMService lifecycle hook integration

- [ ] Add billing hook support to VMService

**File:** `internal/controller/services/vm_service.go`

### 13a. Add `BillingHookResolver` interface and field to VMService

Add a new interface near the existing `PreActionChecker` interface:

```go
// BillingHookResolver resolves a billing lifecycle hook for a customer.
// The VMService calls this after VM create/delete/resize to notify the
// customer's billing provider. If nil, billing hooks are skipped.
type BillingHookResolver interface {
	ForCustomer(providerName string) (VMLifecycleHook, error)
}
```

Note: `VMLifecycleHook` needs to be defined in the `services` package (or import from `billing`). Since we want to avoid a direct import cycle, define a local interface.

**Better approach — define a minimal interface in VMService:**

Add after the `PreActionChecker` interface definition:

```go
// BillingHookResolver resolves the billing lifecycle hook for a customer.
type BillingHookResolver interface {
	ForCustomer(providerName string) (BillingVMHook, error)
}

// BillingVMHook is the billing lifecycle callback interface.
type BillingVMHook interface {
	OnVMCreated(ctx context.Context, vm billing.VMRef) error
	OnVMDeleted(ctx context.Context, vm billing.VMRef) error
	OnVMResized(ctx context.Context, vm billing.VMRef, oldPlanID, newPlanID string) error
}
```

Actually, since `billing.VMRef` is a simple struct from an internal package, it's fine to import `billing` directly. The import is:

```go
"github.com/AbuGosok/VirtueStack/internal/controller/billing"
```

### 13a. Add billing imports and interfaces

Add to the imports in `vm_service.go`:

```go
"github.com/AbuGosok/VirtueStack/internal/controller/billing"
```

Add after the existing `PreActionChecker` interface:

```go
// BillingHookResolver resolves the billing lifecycle hook for a customer's
// billing provider. Returns the hook implementation for the given provider name.
type BillingHookResolver interface {
	ForCustomer(providerName string) (billing.VMLifecycleHook, error)
}
```

### 13b. Add fields to VMService struct and VMServiceConfig

Add to `VMService` struct (after `preActionWebhookSvc`):

```go
billingHooks BillingHookResolver
customerRepo *repository.CustomerRepository
```

Add to `VMServiceConfig` (after `PreActionWebhookSvc`):

```go
BillingHooks BillingHookResolver
CustomerRepo *repository.CustomerRepository
```

Update `NewVMService` to set the new fields:

```go
billingHooks:        cfg.BillingHooks,
customerRepo:        cfg.CustomerRepo,
```

### 13c. Add `notifyBillingHook` helper method

Add this helper method to VMService:

```go
// notifyBillingHook resolves the customer's billing provider and calls the
// given hook function. Errors are logged but do not fail the parent operation
// — billing hooks are best-effort side effects.
func (s *VMService) notifyBillingHook(ctx context.Context, customerID string, hookName string, fn func(billing.VMLifecycleHook) error) {
	if s.billingHooks == nil || s.customerRepo == nil {
		return
	}
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		s.logger.Warn("billing hook: failed to get customer",
			"hook", hookName, "customer_id", customerID, "error", err)
		return
	}
	hook, err := s.billingHooks.ForCustomer(customer.BillingProvider)
	if err != nil {
		s.logger.Warn("billing hook: provider not found",
			"hook", hookName, "provider", customer.BillingProvider, "error", err)
		return
	}
	if err := fn(hook); err != nil {
		s.logger.Warn("billing hook failed",
			"hook", hookName, "provider", customer.BillingProvider,
			"customer_id", customerID, "error", err)
	}
}
```

### 13d. Add hooks to CreateVM

In `CreateVM()`, after the success log line (`s.logger.Info("VM creation initiated", ...)`), add:

```go
s.notifyBillingHook(ctx, customerID, "OnVMCreated", func(h billing.VMLifecycleHook) error {
	return h.OnVMCreated(ctx, billing.VMRef{
		ID: vm.ID, CustomerID: customerID,
		PlanID: vm.PlanID, Hostname: vm.Hostname,
	})
})
```

### 13e. Add hooks to DeleteVM

In `DeleteVM()`, after the success log line (`s.logger.Info("VM deletion initiated", ...)`), add:

```go
s.notifyBillingHook(ctx, customerID, "OnVMDeleted", func(h billing.VMLifecycleHook) error {
	return h.OnVMDeleted(ctx, billing.VMRef{
		ID: vm.ID, CustomerID: customerID,
		PlanID: vm.PlanID, Hostname: vm.Hostname,
	})
})
```

### 13f. Add hooks to ResizeVMWithPlan

In `ResizeVMWithPlan()`, when a new plan is provided and updated (inside the `if newPlanID != "" && newPlanID != vm.PlanID` block), after the existing log line, add:

```go
s.notifyBillingHook(ctx, customerID, "OnVMResized", func(h billing.VMLifecycleHook) error {
	return h.OnVMResized(ctx, billing.VMRef{
		ID: vm.ID, CustomerID: customerID,
		PlanID: newPlanID, Hostname: vm.Hostname,
	}, vm.PlanID, newPlanID)
})
```

**Verify:**
```bash
go build ./internal/controller/services/...
```

**Commit:**
```
feat(vm-service): add billing lifecycle hooks

VMService now calls billing provider hooks after CreateVM, DeleteVM,
and ResizeVMWithPlan. Hooks are best-effort (errors logged, not returned).
Uses BillingHookResolver interface to resolve provider per customer.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 14: VMService billing hook tests

- [ ] Add tests for billing hook integration in VMService

**File to create:** `internal/controller/services/vm_billing_hooks_test.go`

```go
package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

type mockBillingHook struct {
	onVMCreatedCalled bool
	onVMDeletedCalled bool
	onVMResizedCalled bool
	returnErr         error
}

func (m *mockBillingHook) OnVMCreated(_ context.Context, _ billing.VMRef) error {
	m.onVMCreatedCalled = true
	return m.returnErr
}

func (m *mockBillingHook) OnVMDeleted(_ context.Context, _ billing.VMRef) error {
	m.onVMDeletedCalled = true
	return m.returnErr
}

func (m *mockBillingHook) OnVMResized(_ context.Context, _ billing.VMRef, _, _ string) error {
	m.onVMResizedCalled = true
	return m.returnErr
}

type mockBillingResolver struct {
	hook    billing.VMLifecycleHook
	hookErr error
}

func (m *mockBillingResolver) ForCustomer(_ string) (billing.VMLifecycleHook, error) {
	return m.hook, m.hookErr
}

type billingTestDB struct {
	customer *models.Customer
	err      error
}

func (b *billingTestDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &billingTestRow{customer: b.customer, err: b.err}
}

func (b *billingTestDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("not implemented")
}

func (b *billingTestDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (b *billingTestDB) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, fmt.Errorf("not implemented")
}

type billingTestRow struct {
	customer *models.Customer
	err      error
}

func (r *billingTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.customer == nil {
		return pgx.ErrNoRows
	}
	// Scan into the 14 customerSelectCols fields
	ptrs := []any{
		&r.customer.ID, &r.customer.Email, &r.customer.PasswordHash,
		&r.customer.Name, &r.customer.Phone,
		&r.customer.WHMCSClientID, &r.customer.BillingProvider,
		&r.customer.TOTPSecretEncrypted, &r.customer.TOTPEnabled,
		&r.customer.TOTPBackupCodesHash, &r.customer.TOTPBackupCodesShown,
		&r.customer.Status, &r.customer.CreatedAt, &r.customer.UpdatedAt,
	}
	for i, p := range ptrs {
		if i < len(dest) {
			// Copy value from source to dest pointer
			switch d := dest[i].(type) {
			case *string:
				*d = *(p.(*string))
			default:
				_ = d
			}
		}
	}
	return nil
}

func testBillingLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNotifyBillingHook_NilResolver(t *testing.T) {
	svc := &VMService{
		billingHooks: nil,
		logger:       testBillingLogger(),
	}
	// Should not panic
	svc.notifyBillingHook(context.Background(), "cust-123", "OnVMCreated",
		func(h billing.VMLifecycleHook) error {
			return h.OnVMCreated(context.Background(), billing.VMRef{})
		})
}

func TestNotifyBillingHook_NilCustomerRepo(t *testing.T) {
	hook := &mockBillingHook{}
	svc := &VMService{
		billingHooks: &mockBillingResolver{hook: hook},
		customerRepo: nil,
		logger:        testBillingLogger(),
	}
	svc.notifyBillingHook(context.Background(), "cust-123", "OnVMCreated",
		func(h billing.VMLifecycleHook) error {
			return h.OnVMCreated(context.Background(), billing.VMRef{})
		})
	assert.False(t, hook.onVMCreatedCalled)
}

func TestNotifyBillingHook_ProviderNotFound(t *testing.T) {
	db := &billingTestDB{
		customer: &models.Customer{
			ID:              "cust-123",
			BillingProvider: "nonexistent",
			Status:          "active",
		},
	}
	hook := &mockBillingHook{}
	svc := &VMService{
		billingHooks: &mockBillingResolver{
			hook:    hook,
			hookErr: fmt.Errorf("provider not found"),
		},
		customerRepo: repository.NewCustomerRepository(db),
		logger:       testBillingLogger(),
	}
	svc.notifyBillingHook(context.Background(), "cust-123", "OnVMCreated",
		func(h billing.VMLifecycleHook) error {
			return h.OnVMCreated(context.Background(), billing.VMRef{})
		})
	assert.False(t, hook.onVMCreatedCalled)
}

func TestNotifyBillingHook_HookError(t *testing.T) {
	db := &billingTestDB{
		customer: &models.Customer{
			ID:              "cust-123",
			BillingProvider: "whmcs",
			Status:          "active",
		},
	}
	hook := &mockBillingHook{returnErr: fmt.Errorf("hook failed")}
	svc := &VMService{
		billingHooks: &mockBillingResolver{hook: hook},
		customerRepo: repository.NewCustomerRepository(db),
		logger:       testBillingLogger(),
	}
	svc.notifyBillingHook(context.Background(), "cust-123", "OnVMCreated",
		func(h billing.VMLifecycleHook) error {
			return h.OnVMCreated(context.Background(), billing.VMRef{})
		})
	// Hook was called despite returning error
	assert.True(t, hook.onVMCreatedCalled)
}
```

**Verify:**
```bash
go test -race ./internal/controller/services/... -run TestNotifyBillingHook
# Expected: PASS
```

**Commit:**
```
test(vm-service): add billing hook integration tests

Covers nil resolver, nil customer repo, provider not found, and
hook error scenarios. Verifies hooks are best-effort (non-fatal).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 15: Provisioning handler update

- [ ] Set `billing_provider = "whmcs"` on customer creation

**File:** `internal/controller/api/provisioning/customers.go`

In the `CreateOrGetCustomer` method, when building the `customer` struct for creation, add the `BillingProvider` field:

```go
customer := &models.Customer{
	Email:           req.Email,
	Name:            req.Name,
	PasswordHash:    passwordHash,
	WHMCSClientID:   req.WHMCSClientID,
	BillingProvider: models.BillingProviderWHMCS,
	Status:          models.CustomerStatusActive,
}
```

This ensures WHMCS-created customers are always explicitly tagged with `billing_provider = "whmcs"`, regardless of whether `whmcs_client_id` is provided in the same request.

**Also update `updateWHMCSClientID`** to set billing_provider when updating a legacy customer's WHMCS ID. After the existing `UpdateWHMCSClientID` call, add:

```go
if bpErr := h.customerRepo.UpdateBillingProvider(c.Request.Context(), customer.ID, models.BillingProviderWHMCS); bpErr != nil {
	h.logger.Error("failed to update billing_provider on existing customer",
		"customer_id", customer.ID,
		"error", bpErr,
		"correlation_id", middleware.GetCorrelationID(c))
}
```

**Verify:**
```bash
go build ./internal/controller/api/provisioning/...
```

**Commit:**
```
feat(provisioning): set billing_provider=whmcs on customer creation

CreateOrGetCustomer now explicitly sets BillingProvider to "whmcs" for
all WHMCS-created customers. Also updates billing_provider when linking
an existing customer to a WHMCS client ID.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 16: Server.go + dependencies.go wiring

- [ ] Create billing registry and wire into VMService

**File:** `internal/controller/dependencies.go`

### 16a. Add imports

Add to the import block:

```go
"github.com/AbuGosok/VirtueStack/internal/controller/billing"
"github.com/AbuGosok/VirtueStack/internal/controller/billing/whmcs"
```

### 16b. Create registry and wire to VMService

In `InitializeServices()`, after the `preActionWebhookService` initialization (line ~92) and before the `VMService` creation (line ~94), add:

```go
// Initialize billing provider registry
billingRegistry := billing.NewRegistry("", s.logger)
if err := billingRegistry.Register(whmcs.NewAdapter()); err != nil {
	return fmt.Errorf("registering WHMCS billing adapter: %w", err)
}
```

### 16c. Update VMServiceConfig

Update the `VMServiceConfig` in `InitializeServices()` to include the new fields:

```go
s.vmService = services.NewVMService(services.VMServiceConfig{
	VMRepo:              vmRepo,
	NodeRepo:            nodeRepo,
	IPRepo:              ipRepo,
	PlanRepo:            planRepo,
	TemplateRepo:        templateRepo,
	TaskRepo:            taskRepo,
	TaskPublisher:       taskPublisher,
	NodeAgent:           nodeAgentClient,
	IPAMService:         s.ipamService,
	StorageBackendSvc:   storageBackendService,
	PreActionWebhookSvc: preActionWebhookService,
	BillingHooks:        billingRegistry,
	CustomerRepo:        customerRepo,
	EncryptionKey:       s.config.EncryptionKey.Value(),
	Logger:              s.logger,
})
```

### 16d. Ensure Registry satisfies BillingHookResolver

The `billing.Registry` already has `ForCustomer(providerName string) (BillingProvider, error)`. Since `BillingProvider` embeds/satisfies `VMLifecycleHook`, we need a thin adapter.

Add a `RegistryAdapter` to bridge Registry → BillingHookResolver:

**File to create:** `internal/controller/billing/registry_adapter.go`

```go
package billing

// RegistryHookAdapter wraps a Registry to satisfy the BillingHookResolver
// interface expected by VMService. It narrows ForCustomer's return type
// from BillingProvider to VMLifecycleHook.
type RegistryHookAdapter struct {
	registry *Registry
}

// NewRegistryHookAdapter creates a new adapter wrapping the given registry.
func NewRegistryHookAdapter(r *Registry) *RegistryHookAdapter {
	return &RegistryHookAdapter{registry: r}
}

// ForCustomer returns the VMLifecycleHook for the given provider name.
func (a *RegistryHookAdapter) ForCustomer(providerName string) (VMLifecycleHook, error) {
	return a.registry.ForCustomer(providerName)
}
```

Since `BillingProvider` includes all methods of `VMLifecycleHook`, the returned `BillingProvider` satisfies `VMLifecycleHook` automatically.

### 16e. Update dependencies.go wiring to use adapter

Change the `BillingHooks` field from `billingRegistry` to:

```go
BillingHooks: billing.NewRegistryHookAdapter(billingRegistry),
```

**Verify:**
```bash
go build ./internal/controller/...
```

**Commit:**
```
feat(server): wire billing registry into VMService

Creates billing.Registry with WHMCS adapter in InitializeServices().
Uses RegistryHookAdapter to bridge Registry to BillingHookResolver
interface consumed by VMService.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Step 17: Full build + test verification

- [ ] Verify all changes compile and pass tests

**Commands:**
```bash
# Build controller (always works without native libs)
make build-controller

# Run unit tests (controller/shared packages only)
make test

# Run tests with race detector
make test-race
```

**Expected:**
- `make build-controller` succeeds with no errors
- `make test` passes all existing tests plus new billing tests
- `make test-race` passes with no data races

**If lint is available:**
```bash
make lint 2>/dev/null || echo "golangci-lint not installed, skipping"
```

**Final commit (if changes were batched):**
```
feat(billing): complete Phase 0 — provider abstraction + WHMCS refactor

Phase 0 delivers:
- Migration 000072: billing_provider column on customers
- BillingProvider interface + VMLifecycleHook
- Provider Registry with thread-safe registration
- WHMCS adapter (no-op lifecycle, ErrNotSupported for balance/topup)
- Customer model + repository updates
- VMService lifecycle hooks (CreateVM, DeleteVM, ResizeVMWithPlan)
- Provisioning handler sets billing_provider=whmcs explicitly
- Server.go wiring of registry + adapter

All existing tests pass. WHMCS workflow unchanged.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Verification Checklist

After all steps are complete, verify:

- [ ] `make build-controller` succeeds
- [ ] `make test` passes (all existing + new tests)
- [ ] `make test-race` passes with no data races
- [ ] Migration 000072 `.up.sql` and `.down.sql` exist and are valid SQL
- [ ] `internal/controller/billing/provider.go` defines `BillingProvider` interface with all 11 methods
- [ ] `internal/controller/billing/registry.go` implements `Register`, `ForCustomer`, `Primary`, `All`
- [ ] `internal/controller/billing/whmcs/adapter.go` implements all 11 methods (compile-time check via `var _ billing.BillingProvider`)
- [ ] WHMCS adapter lifecycle hooks return nil
- [ ] WHMCS adapter balance/topup/usage return `ErrNotSupported`
- [ ] `Customer.BillingProvider` field exists in model
- [ ] `scanCustomer` and `customerSelectCols` include `billing_provider`
- [ ] `CustomerRepository.Create` inserts `billing_provider`
- [ ] `CustomerRepository.UpdateBillingProvider` method exists
- [ ] `CreateOrGetCustomer` sets `BillingProvider: models.BillingProviderWHMCS`
- [ ] `VMService.CreateVM` calls `notifyBillingHook` with `OnVMCreated`
- [ ] `VMService.DeleteVM` calls `notifyBillingHook` with `OnVMDeleted`
- [ ] `VMService.ResizeVMWithPlan` calls `notifyBillingHook` with `OnVMResized`
- [ ] Billing hooks are best-effort (errors logged, not returned to caller)
- [ ] `ErrNotSupported` sentinel exists in `internal/shared/errors/errors.go`

## File Summary

| Action | File |
|--------|------|
| Create | `migrations/000072_add_billing_provider.up.sql` |
| Create | `migrations/000072_add_billing_provider.down.sql` |
| Edit   | `internal/shared/errors/errors.go` |
| Create | `internal/controller/billing/provider.go` |
| Create | `internal/controller/billing/provider_test.go` |
| Create | `internal/controller/billing/registry.go` |
| Create | `internal/controller/billing/registry_test.go` |
| Create | `internal/controller/billing/registry_adapter.go` |
| Create | `internal/controller/billing/hook.go` |
| Create | `internal/controller/billing/whmcs/adapter.go` |
| Create | `internal/controller/billing/whmcs/adapter_test.go` |
| Edit   | `internal/controller/models/customer.go` |
| Edit   | `internal/controller/repository/customer_repo.go` |
| Create | `internal/controller/repository/customer_billing_test.go` |
| Edit   | `internal/controller/services/vm_service.go` |
| Create | `internal/controller/services/vm_billing_hooks_test.go` |
| Edit   | `internal/controller/api/provisioning/customers.go` |
| Edit   | `internal/controller/dependencies.go` |
