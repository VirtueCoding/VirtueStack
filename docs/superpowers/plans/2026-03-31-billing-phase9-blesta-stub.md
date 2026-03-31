# Billing Phase 9: Blesta Provider Stub — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a minimal Blesta billing provider stub that registers with the billing registry, enabling future Blesta integration without blocking the current release.

**Architecture:** Stub adapter implementing BillingProvider interface with ErrNotSupported returns for all operational methods. Registered conditionally via BILLING_BLESTA_ENABLED config flag. Documentation explains what needs implementation for full Blesta support.

**Tech Stack:** Go 1.26

**Depends on:** Phase 0 (billing registry), Phase 1 (config flags)
**Depended on by:** None (future work)

---

## Task 1: Blesta Adapter — Stub Implementation

- [ ] Create the Blesta billing provider stub that implements BillingProvider

**File to create:** `internal/controller/billing/blesta/adapter.go`

```go
// Package blesta provides a stub BillingProvider implementation for future
// Blesta billing integration. All operational methods return ErrNotSupported.
// This stub exists so the billing registry can reference Blesta as a valid
// provider name without crashing, and so customers with billing_provider='blesta'
// in the database are handled gracefully.
package blesta

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// BlestaConfig holds configuration for the Blesta billing provider.
type BlestaConfig struct {
	APIURL string
	APIKey string
	Logger *slog.Logger
}

// Adapter implements billing.BillingProvider as a stub for Blesta.
// All operational methods return ErrNotSupported until a full Blesta
// integration is implemented.
type Adapter struct {
	apiURL string
	apiKey string
	logger *slog.Logger
}

// NewAdapter creates a new Blesta billing provider stub.
func NewAdapter(cfg BlestaConfig) *Adapter {
	return &Adapter{
		apiURL: cfg.APIURL,
		apiKey: cfg.APIKey,
		logger: cfg.Logger.With("component", "billing-blesta"),
	}
}

// compile-time interface check
var _ billing.BillingProvider = (*Adapter)(nil)

func (a *Adapter) Name() string { return "blesta" }

// ValidateConfig checks whether the Blesta configuration is sufficient.
// When enabled, BLESTA_API_URL and BLESTA_API_KEY are required.
func (a *Adapter) ValidateConfig() error {
	if a.apiURL == "" {
		return fmt.Errorf("blesta: BLESTA_API_URL is required when BILLING_BLESTA_ENABLED=true")
	}
	if a.apiKey == "" {
		return fmt.Errorf("blesta: BLESTA_API_KEY is required when BILLING_BLESTA_ENABLED=true")
	}
	return nil
}

func (a *Adapter) CreateUser(
	_ context.Context, _ billing.CreateUserRequest,
) (*billing.UserResult, error) {
	return nil, sharederrors.ErrNotSupported
}

func (a *Adapter) GetUserBillingStatus(
	_ context.Context, _ string,
) (*billing.BillingStatus, error) {
	return &billing.BillingStatus{
		Status:  "active",
		Message: "Blesta billing status is managed externally",
	}, nil
}

func (a *Adapter) OnVMCreated(_ context.Context, _ billing.VMRef) error {
	return nil
}

func (a *Adapter) OnVMDeleted(_ context.Context, _ billing.VMRef) error {
	return nil
}

func (a *Adapter) OnVMResized(
	_ context.Context, _ billing.VMRef, _, _ string,
) error {
	return nil
}

func (a *Adapter) SuspendForNonPayment(_ context.Context, _ string) error {
	return sharederrors.ErrNotSupported
}

func (a *Adapter) UnsuspendAfterPayment(_ context.Context, _ string) error {
	return sharederrors.ErrNotSupported
}

func (a *Adapter) GetBalance(
	_ context.Context, _ string,
) (*billing.Balance, error) {
	return nil, sharederrors.ErrNotSupported
}

func (a *Adapter) ProcessTopUp(
	_ context.Context, _ billing.TopUpRequest,
) (*billing.TopUpResult, error) {
	return nil, sharederrors.ErrNotSupported
}

func (a *Adapter) GetUsageHistory(
	_ context.Context, _ string, _ billing.PaginationOpts,
) (*billing.UsageHistory, error) {
	return nil, sharederrors.ErrNotSupported
}
```

**Verify:**
```bash
go build ./internal/controller/billing/blesta/...
```

**Commit:**
```
feat(billing): add Blesta provider stub adapter

Implements BillingProvider interface with ErrNotSupported for all
operational methods (balance, top-up, suspend, unsuspend, usage).
Lifecycle hooks (OnVMCreated/Deleted/Resized) return nil (no-op).
GetUserBillingStatus returns "active" (Blesta manages externally).
ValidateConfig requires BLESTA_API_URL and BLESTA_API_KEY.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 2: Blesta Config Environment Variables

- [ ] Add BLESTA_API_URL and BLESTA_API_KEY to the config struct and env overrides

**File:** `internal/shared/config/config.go`

### 2a. Add BlestaConfig struct

Add after the existing billing/payment config structs (if not already present from Phase 1):

```go
// BlestaConfig holds Blesta billing provider configuration.
type BlestaConfig struct {
	APIURL string `yaml:"api_url"`
	APIKey Secret `yaml:"api_key"`
}
```

### 2b. Add to ControllerConfig

Add a `Blesta` field to `ControllerConfig`:

```go
	Blesta BlestaConfig `yaml:"blesta"`
```

### 2c. Add env overrides

In the `applyEnvOverridesBilling` function (or create a new `applyEnvOverridesBlesta`), add:

```go
	if v := os.Getenv("BLESTA_API_URL"); v != "" {
		cfg.Blesta.APIURL = v
	}
	if v := os.Getenv("BLESTA_API_KEY"); v != "" {
		cfg.Blesta.APIKey = Secret(v)
	}
```

### 2d. Add validation in `validateBillingConfig`

Add Blesta-specific validation:

```go
	// Blesta: if enabled, API URL and API key are required.
	if cfg.Billing.Providers.Blesta.Enabled {
		if cfg.Blesta.APIURL == "" {
			return fmt.Errorf("BILLING_BLESTA_ENABLED=true requires BLESTA_API_URL")
		}
		if cfg.Blesta.APIKey.Value() == "" {
			return fmt.Errorf("BILLING_BLESTA_ENABLED=true requires BLESTA_API_KEY")
		}
	}
```

**Verify:**
```bash
go build ./internal/shared/config/...
go test -race ./internal/shared/config/...
```

**Commit:**
```
feat(config): add BLESTA_API_URL and BLESTA_API_KEY config fields

Adds BlestaConfig struct with APIURL and APIKey (Secret type).
Startup validation enforces both are set when BILLING_BLESTA_ENABLED=true.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 3: Register Blesta Adapter in Billing Registry

- [ ] Wire the Blesta adapter into the billing registry initialization

**File:** `internal/controller/dependencies.go` (or wherever the billing registry is initialized)

In the billing registry initialization section, add conditional Blesta registration:

```go
if s.config.Billing.Providers.Blesta.Enabled {
	blestaAdapter := blesta.NewAdapter(blesta.BlestaConfig{
		APIURL: s.config.Blesta.APIURL,
		APIKey: string(s.config.Blesta.APIKey.Value()),
		Logger: s.logger,
	})
	if err := billingRegistry.Register(blestaAdapter); err != nil {
		return fmt.Errorf("register blesta billing provider: %w", err)
	}
	s.logger.Info("blesta billing provider registered (stub)")
}
```

Add the import:

```go
import "github.com/AbuGosok/VirtueStack/internal/controller/billing/blesta"
```

**Verify:**
```bash
go build ./internal/controller/...
```

**Commit:**
```
feat(deps): register Blesta adapter in billing registry when enabled

Conditionally creates and registers the Blesta stub adapter based on
BILLING_BLESTA_ENABLED config flag. Logs registration at Info level.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 4: Blesta Adapter Tests

- [ ] Write table-driven tests for the Blesta adapter

**File to create:** `internal/controller/billing/blesta/adapter_test.go`

```go
package blesta_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/billing/blesta"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAdapter(apiURL, apiKey string) *blesta.Adapter {
	return blesta.NewAdapter(blesta.BlestaConfig{
		APIURL: apiURL,
		APIKey: apiKey,
		Logger: slog.Default(),
	})
}

func TestAdapter_Name(t *testing.T) {
	adapter := newTestAdapter("https://blesta.example.com/api", "test-key")
	assert.Equal(t, "blesta", adapter.Name())
}

func TestAdapter_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		apiURL  string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "valid config",
			apiURL:  "https://blesta.example.com/api",
			apiKey:  "test-api-key-123",
			wantErr: false,
		},
		{
			name:    "missing API URL",
			apiURL:  "",
			apiKey:  "test-key",
			wantErr: true,
		},
		{
			name:    "missing API key",
			apiURL:  "https://blesta.example.com/api",
			apiKey:  "",
			wantErr: true,
		},
		{
			name:    "both missing",
			apiURL:  "",
			apiKey:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newTestAdapter(tt.apiURL, tt.apiKey)
			err := adapter.ValidateConfig()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAdapter_OperationalMethods_ReturnNotSupported(t *testing.T) {
	adapter := newTestAdapter("https://blesta.example.com/api", "key")
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "CreateUser",
			fn: func() error {
				_, err := adapter.CreateUser(ctx, billing.CreateUserRequest{
					CustomerID: "cust-1", Email: "a@b.com", Name: "Test",
				})
				return err
			},
		},
		{
			name: "SuspendForNonPayment",
			fn:   func() error { return adapter.SuspendForNonPayment(ctx, "cust-1") },
		},
		{
			name: "UnsuspendAfterPayment",
			fn:   func() error { return adapter.UnsuspendAfterPayment(ctx, "cust-1") },
		},
		{
			name: "GetBalance",
			fn: func() error {
				_, err := adapter.GetBalance(ctx, "cust-1")
				return err
			},
		},
		{
			name: "ProcessTopUp",
			fn: func() error {
				_, err := adapter.ProcessTopUp(ctx, billing.TopUpRequest{
					CustomerID: "cust-1", AmountCents: 1000,
				})
				return err
			},
		},
		{
			name: "GetUsageHistory",
			fn: func() error {
				_, err := adapter.GetUsageHistory(ctx, "cust-1", billing.PaginationOpts{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.Error(t, err)
			assert.True(t, sharederrors.Is(err, sharederrors.ErrNotSupported),
				"expected ErrNotSupported, got: %v", err)
		})
	}
}

func TestAdapter_LifecycleHooks_ReturnNil(t *testing.T) {
	adapter := newTestAdapter("https://blesta.example.com/api", "key")
	ctx := context.Background()
	vmRef := billing.VMRef{
		ID: "vm-1", CustomerID: "cust-1", PlanID: "plan-1", Hostname: "test-vm",
	}

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "OnVMCreated",
			fn:   func() error { return adapter.OnVMCreated(ctx, vmRef) },
		},
		{
			name: "OnVMDeleted",
			fn:   func() error { return adapter.OnVMDeleted(ctx, vmRef) },
		},
		{
			name: "OnVMResized",
			fn:   func() error { return adapter.OnVMResized(ctx, vmRef, "old-plan", "new-plan") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.NoError(t, err)
		})
	}
}

func TestAdapter_GetUserBillingStatus_ReturnsActive(t *testing.T) {
	adapter := newTestAdapter("https://blesta.example.com/api", "key")
	ctx := context.Background()

	status, err := adapter.GetUserBillingStatus(ctx, "cust-1")
	require.NoError(t, err)
	assert.Equal(t, "active", status.Status)
	assert.Contains(t, status.Message, "externally")
}

func TestAdapter_ImplementsBillingProvider(t *testing.T) {
	adapter := newTestAdapter("https://blesta.example.com/api", "key")
	var _ billing.BillingProvider = adapter
}
```

**Verify:**
```bash
go test -race -run TestAdapter ./internal/controller/billing/blesta/...
```

**Commit:**
```
test(billing): add Blesta adapter tests

Table-driven tests verifying: Name returns "blesta", ValidateConfig
enforces API URL + key, operational methods return ErrNotSupported,
lifecycle hooks return nil, GetUserBillingStatus returns "active",
and adapter satisfies BillingProvider interface.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 5: Config Validation Tests for Blesta

- [ ] Add test cases for Blesta config validation

**File:** `internal/shared/config/config_test.go`

Add test cases to the existing `TestValidateBillingConfig` (or create a new test function):

```go
func TestValidateBillingConfig_Blesta(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(cfg *ControllerConfig)
		wantErr bool
		errMsg  string
	}{
		{
			name: "blesta enabled with valid config",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = true
				cfg.Billing.Providers.Blesta.Primary = true
				cfg.Billing.Providers.WHMCS.Primary = false
				cfg.Blesta.APIURL = "https://blesta.example.com/api"
				cfg.Blesta.APIKey = Secret("test-api-key")
			},
			wantErr: false,
		},
		{
			name: "blesta enabled missing API URL",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = true
				cfg.Billing.Providers.Blesta.Primary = true
				cfg.Billing.Providers.WHMCS.Primary = false
				cfg.Blesta.APIKey = Secret("test-api-key")
			},
			wantErr: true,
			errMsg:  "BLESTA_API_URL",
		},
		{
			name: "blesta enabled missing API key",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = true
				cfg.Billing.Providers.Blesta.Primary = true
				cfg.Billing.Providers.WHMCS.Primary = false
				cfg.Blesta.APIURL = "https://blesta.example.com/api"
			},
			wantErr: true,
			errMsg:  "BLESTA_API_KEY",
		},
		{
			name: "blesta disabled — no validation needed",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = false
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			tt.modify(cfg)
			err := validateBillingConfig(cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
```

**Verify:**
```bash
go test -race -run TestValidateBillingConfig_Blesta ./internal/shared/config/...
```

**Commit:**
```
test(config): add Blesta config validation tests

Tests that BILLING_BLESTA_ENABLED=true requires both BLESTA_API_URL
and BLESTA_API_KEY. Verifies disabled Blesta skips validation.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 6: Blesta Integration Documentation

- [ ] Create documentation explaining the Blesta stub and what needs implementation

**File to create:** `docs/billing-blesta.md`

```markdown
# Blesta Billing Integration

## Current Status: Stub

The Blesta billing provider is registered as a **stub adapter**. It satisfies the
`BillingProvider` interface so the billing registry can reference `blesta` as a
valid provider name, but all operational methods return `ErrNotSupported`.

## What Works

| Method | Behavior |
|--------|----------|
| `Name()` | Returns `"blesta"` |
| `ValidateConfig()` | Checks `BLESTA_API_URL` + `BLESTA_API_KEY` are set |
| `OnVMCreated()` | No-op (returns nil) |
| `OnVMDeleted()` | No-op (returns nil) |
| `OnVMResized()` | No-op (returns nil) |
| `GetUserBillingStatus()` | Returns `"active"` (Blesta manages externally) |

## What Returns `ErrNotSupported`

These methods need full implementation for production Blesta support:

| Method | What It Should Do |
|--------|-------------------|
| `CreateUser()` | Call Blesta API to create a client |
| `SuspendForNonPayment()` | Call Blesta API to suspend services |
| `UnsuspendAfterPayment()` | Call Blesta API to unsuspend services |
| `GetBalance()` | Query Blesta API for client credit balance |
| `ProcessTopUp()` | Post credit to Blesta client account |
| `GetUsageHistory()` | Fetch Blesta invoice/transaction history |

## Configuration

| Environment Variable | Required | Description |
|---------------------|----------|-------------|
| `BILLING_BLESTA_ENABLED` | No | Enable Blesta provider (default: `false`) |
| `BILLING_BLESTA_PRIMARY` | No | Set as primary for new registrations (default: `false`) |
| `BLESTA_API_URL` | When enabled | Blesta API base URL (e.g., `https://blesta.example.com/api/`) |
| `BLESTA_API_KEY` | When enabled | Blesta API authentication key |

## Architecture

```
internal/controller/billing/blesta/
  adapter.go        # BillingProvider stub implementation
  adapter_test.go   # Tests
```

The adapter is registered conditionally in `internal/controller/dependencies.go`
when `BILLING_BLESTA_ENABLED=true`.

## Implementing Full Blesta Support

To implement full Blesta support:

1. **Add an HTTP client** to `adapter.go` that calls the Blesta API
   (use the SSRF-safe transport from `internal/shared/util/ssrf.go`)
2. **Implement `CreateUser`** — `POST /api/clients/add.json`
3. **Implement `SuspendForNonPayment`** — `POST /api/services/suspend.json`
4. **Implement `UnsuspendAfterPayment`** — `POST /api/services/unsuspend.json`
5. **Implement `GetBalance`** — `GET /api/clients/get_credits.json`
6. **Implement `ProcessTopUp`** — `POST /api/clients/add_credit.json`
7. **Implement `GetUsageHistory`** — `GET /api/invoices/get_list.json`
8. **Add webhook receiver** for Blesta → VirtueStack event notifications
9. **Write integration tests** against a Blesta sandbox instance

Each method should map to the corresponding Blesta API endpoint documented at
https://docs.blesta.com/display/dev/API

## Customer Assignment

Customers are assigned to Blesta billing via the `billing_provider` column:

```sql
UPDATE customers SET billing_provider = 'blesta' WHERE id = :customer_id;
```

Only admin endpoints can change `billing_provider`. When
`BILLING_BLESTA_PRIMARY=true`, new self-registered customers are automatically
assigned to Blesta.
```

**Commit:**
```
docs: add Blesta billing integration guide

Documents the stub adapter's current capabilities, what needs
implementation for full support, configuration variables, architecture,
and step-by-step guide for implementing each BillingProvider method.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 7: Update .env.example

- [ ] Add Blesta config to `.env.example`

**File:** `.env.example`

Add in the billing section:

```bash
# --- Blesta Billing (Stub) ---
# BILLING_BLESTA_ENABLED=false
# BILLING_BLESTA_PRIMARY=false
# BLESTA_API_URL=https://blesta.example.com/api/
# BLESTA_API_KEY=your-blesta-api-key
```

**Verify:**
```bash
grep -q BLESTA .env.example && echo "OK"
```

**Commit:**
```
chore: add Blesta config to .env.example

Documents BILLING_BLESTA_ENABLED, BILLING_BLESTA_PRIMARY,
BLESTA_API_URL, and BLESTA_API_KEY environment variables.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 8: Final Verification

- [ ] Run full build and test suite to verify Blesta stub integration

**Steps:**

```bash
# 1. Build controller
make build-controller

# 2. Run unit tests with race detector
make test-race

# 3. Run Blesta-specific tests
go test -race -run TestAdapter ./internal/controller/billing/blesta/...
go test -race -run TestValidateBillingConfig_Blesta ./internal/shared/config/...

# 4. Verify files exist
ls -la internal/controller/billing/blesta/adapter.go
ls -la internal/controller/billing/blesta/adapter_test.go
ls -la docs/billing-blesta.md
```

**Expected results:**
- Controller builds without errors
- All tests pass (existing + new Blesta adapter tests)
- Blesta adapter implements BillingProvider at compile time
- Documentation file exists with implementation guide

**Commit:**
```
chore: verify Phase 9 (Blesta stub) — all tests pass, builds clean

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Summary of Deliverables

| # | Deliverable | Files Changed/Created |
|---|------------|----------------------|
| 1 | Blesta adapter stub | `internal/controller/billing/blesta/adapter.go` |
| 2 | Blesta config fields | `internal/shared/config/config.go` |
| 3 | Registry registration | `internal/controller/dependencies.go` |
| 4 | Adapter tests | `internal/controller/billing/blesta/adapter_test.go` |
| 5 | Config validation tests | `internal/shared/config/config_test.go` |
| 6 | Integration documentation | `docs/billing-blesta.md` |
| 7 | .env.example update | `.env.example` |
| 8 | Final verification | (no files — build/test confirmation) |

## Environment Variables Introduced

| Variable | Type | Default | Required When |
|----------|------|---------|---------------|
| `BLESTA_API_URL` | string | — | `BILLING_BLESTA_ENABLED=true` |
| `BLESTA_API_KEY` | Secret | — | `BILLING_BLESTA_ENABLED=true` |
