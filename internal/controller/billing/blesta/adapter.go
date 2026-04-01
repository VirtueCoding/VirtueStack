package blesta

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// BlestaConfig holds configuration for the Blesta billing provider adapter.
type BlestaConfig struct {
	APIURL string
	APIKey string
	Logger *slog.Logger
}

// Adapter implements billing.BillingProvider as a no-op for Blesta.
// Blesta manages all billing externally via the Provisioning REST API
// (PHP module → POST /provisioning/vms etc). The Go adapter exists only
// so customers with billing_provider='blesta' are handled gracefully.
type Adapter struct {
	logger *slog.Logger
}

// NewAdapter creates a new Blesta billing adapter.
func NewAdapter(cfg BlestaConfig) *Adapter {
	return &Adapter{
		logger: cfg.Logger.With("component", "billing-blesta"),
	}
}

var _ billing.BillingProvider = (*Adapter)(nil)

func (a *Adapter) Name() string { return "blesta" }

func (a *Adapter) ValidateConfig() error { return nil }

// CreateUser echoes back the customer ID. Blesta creates customers
// via POST /provisioning/customers — the Go side just acknowledges.
func (a *Adapter) CreateUser(_ context.Context, req billing.CreateUserRequest) (*billing.UserResult, error) {
	return &billing.UserResult{
		CustomerID: req.CustomerID,
		ProviderID: req.CustomerID,
	}, nil
}

// GetUserBillingStatus always returns active. Blesta manages suspension
// via POST /provisioning/vms/:id/suspend.
func (a *Adapter) GetUserBillingStatus(_ context.Context, customerID string) (*billing.BillingStatus, error) {
	return &billing.BillingStatus{
		CustomerID: customerID,
		Provider:   "blesta",
		IsActive:   true,
		Status:     "active",
		Message:    "Billing managed by Blesta",
	}, nil
}

// VM lifecycle hooks are no-ops — Blesta manages VM billing externally.
func (a *Adapter) OnVMCreated(_ context.Context, _ billing.VMRef) error  { return nil }
func (a *Adapter) OnVMDeleted(_ context.Context, _ billing.VMRef) error  { return nil }
func (a *Adapter) OnVMResized(_ context.Context, _ billing.VMRef, _, _ string) error {
	return nil
}

// Suspend/Unsuspend are no-ops — Blesta drives these via Provisioning API.
func (a *Adapter) SuspendForNonPayment(_ context.Context, _ string) error  { return nil }
func (a *Adapter) UnsuspendAfterPayment(_ context.Context, _ string) error { return nil }

// Balance, top-up, and usage are managed by Blesta — not available from Go side.
func (a *Adapter) GetBalance(_ context.Context, _ string) (*billing.Balance, error) {
	return nil, fmt.Errorf("get balance: %w", sharederrors.ErrNotSupported)
}

func (a *Adapter) ProcessTopUp(_ context.Context, _ billing.TopUpRequest) (*billing.TopUpResult, error) {
	return nil, fmt.Errorf("process top-up: %w", sharederrors.ErrNotSupported)
}

func (a *Adapter) GetUsageHistory(_ context.Context, _ string, _ billing.PaginationOpts) (*billing.UsageHistory, error) {
	return nil, fmt.Errorf("get usage history: %w", sharederrors.ErrNotSupported)
}
