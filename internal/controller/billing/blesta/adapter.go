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

// Name returns the provider identifier.
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

// CreateUser is not yet implemented for Blesta.
func (a *Adapter) CreateUser(
	_ context.Context, _ billing.CreateUserRequest,
) (*billing.UserResult, error) {
	return nil, sharederrors.ErrNotSupported
}

// GetUserBillingStatus returns "active" because Blesta manages status externally.
func (a *Adapter) GetUserBillingStatus(
	_ context.Context, customerID string,
) (*billing.BillingStatus, error) {
	return &billing.BillingStatus{
		CustomerID: customerID,
		Provider:   "blesta",
		IsActive:   true,
		Status:     "active",
		Message:    "Blesta billing status is managed externally",
	}, nil
}

// OnVMCreated is a no-op; Blesta manages VM billing externally.
func (a *Adapter) OnVMCreated(_ context.Context, _ billing.VMRef) error {
	return nil
}

// OnVMDeleted is a no-op; Blesta manages VM billing externally.
func (a *Adapter) OnVMDeleted(_ context.Context, _ billing.VMRef) error {
	return nil
}

// OnVMResized is a no-op; Blesta manages VM billing externally.
func (a *Adapter) OnVMResized(
	_ context.Context, _ billing.VMRef, _, _ string,
) error {
	return nil
}

// SuspendForNonPayment is not yet implemented for Blesta.
func (a *Adapter) SuspendForNonPayment(_ context.Context, _ string) error {
	return sharederrors.ErrNotSupported
}

// UnsuspendAfterPayment is not yet implemented for Blesta.
func (a *Adapter) UnsuspendAfterPayment(_ context.Context, _ string) error {
	return sharederrors.ErrNotSupported
}

// GetBalance is not yet implemented for Blesta.
func (a *Adapter) GetBalance(
	_ context.Context, _ string,
) (*billing.Balance, error) {
	return nil, sharederrors.ErrNotSupported
}

// ProcessTopUp is not yet implemented for Blesta.
func (a *Adapter) ProcessTopUp(
	_ context.Context, _ billing.TopUpRequest,
) (*billing.TopUpResult, error) {
	return nil, sharederrors.ErrNotSupported
}

// GetUsageHistory is not yet implemented for Blesta.
func (a *Adapter) GetUsageHistory(
	_ context.Context, _ string, _ billing.PaginationOpts,
) (*billing.UsageHistory, error) {
	return nil, sharederrors.ErrNotSupported
}
