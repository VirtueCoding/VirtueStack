// Package whmcs implements the BillingProvider interface for WHMCS.
// The WHMCS adapter is intentionally a no-op for lifecycle hooks because
// WHMCS drives billing operations externally via the Provisioning REST API.
package whmcs

import (
	"context"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// Adapter implements billing.BillingProvider for WHMCS.
type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string { return "whmcs" }

func (a *Adapter) ValidateConfig() error { return nil }

func (a *Adapter) CreateUser(_ context.Context, req billing.CreateUserRequest) (*billing.UserResult, error) {
	return &billing.UserResult{
		CustomerID: req.CustomerID,
		ProviderID: req.CustomerID,
	}, nil
}

func (a *Adapter) GetUserBillingStatus(_ context.Context, customerID string) (*billing.BillingStatus, error) {
	return &billing.BillingStatus{
		CustomerID:  customerID,
		Provider:    "whmcs",
		IsActive:    true,
		IsSuspended: false,
	}, nil
}

func (a *Adapter) OnVMCreated(_ context.Context, _ billing.VMRef) error  { return nil }
func (a *Adapter) OnVMDeleted(_ context.Context, _ billing.VMRef) error  { return nil }
func (a *Adapter) OnVMResized(_ context.Context, _ billing.VMRef, _, _ string) error {
	return nil
}
func (a *Adapter) SuspendForNonPayment(_ context.Context, _ string) error  { return nil }
func (a *Adapter) UnsuspendAfterPayment(_ context.Context, _ string) error { return nil }

func (a *Adapter) GetBalance(_ context.Context, _ string) (*billing.Balance, error) {
	return nil, fmt.Errorf("get balance: %w", sharederrors.ErrNotSupported)
}

func (a *Adapter) ProcessTopUp(_ context.Context, _ billing.TopUpRequest) (*billing.TopUpResult, error) {
	return nil, fmt.Errorf("process top-up: %w", sharederrors.ErrNotSupported)
}

func (a *Adapter) GetUsageHistory(_ context.Context, _ string, _ billing.PaginationOpts) (*billing.UsageHistory, error) {
	return nil, fmt.Errorf("get usage history: %w", sharederrors.ErrNotSupported)
}

var _ billing.BillingProvider = (*Adapter)(nil)
