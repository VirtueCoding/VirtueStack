// Package native implements the BillingProvider interface for VirtueStack's
// native prepaid billing system with credit ledger and hourly deductions.
package native

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// AdapterConfig holds dependencies for the native billing adapter.
type AdapterConfig struct {
	LedgerService *services.BillingLedgerService
	Logger        *slog.Logger
}

// Adapter implements BillingProvider for native prepaid billing.
type Adapter struct {
	ledger *services.BillingLedgerService
	logger *slog.Logger
}

// NewAdapter creates a new native billing adapter.
func NewAdapter(cfg AdapterConfig) *Adapter {
	return &Adapter{
		ledger: cfg.LedgerService,
		logger: cfg.Logger.With("component", "native-billing-adapter"),
	}
}

// Name returns the provider identifier.
func (a *Adapter) Name() string { return "native" }

// ValidateConfig checks native billing configuration.
func (a *Adapter) ValidateConfig() error { return nil }

// CreateUser is a no-op for native billing.
func (a *Adapter) CreateUser(_ context.Context, _ billing.CreateUserRequest) (*billing.UserResult, error) {
	return &billing.UserResult{}, nil
}

// GetUserBillingStatus returns the billing status for a customer.
func (a *Adapter) GetUserBillingStatus(ctx context.Context, customerID string) (*billing.BillingStatus, error) {
	balance, err := a.ledger.GetBalance(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("get native billing status: %w", err)
	}
	return &billing.BillingStatus{
		CustomerID:  customerID,
		Provider:    "native",
		IsActive:    true,
		IsSuspended: balance <= 0,
		Status:      "active",
	}, nil
}

// OnVMCreated is a no-op; billing starts on the next hourly scheduler tick.
func (a *Adapter) OnVMCreated(_ context.Context, vm billing.VMRef) error {
	a.logger.Info("VM created, billing starts at next hourly tick",
		"vm_id", vm.ID, "customer_id", vm.CustomerID)
	return nil
}

// OnVMDeleted logs the deletion.
func (a *Adapter) OnVMDeleted(_ context.Context, vm billing.VMRef) error {
	a.logger.Info("VM deleted", "vm_id", vm.ID, "customer_id", vm.CustomerID)
	return nil
}

// OnVMResized is a no-op; the scheduler reads the current plan at charge time.
func (a *Adapter) OnVMResized(_ context.Context, vm billing.VMRef, _, _ string) error {
	a.logger.Info("VM resized, new rate applies at next tick", "vm_id", vm.ID)
	return nil
}

// SuspendForNonPayment logs the suspension event.
func (a *Adapter) SuspendForNonPayment(_ context.Context, customerID string) error {
	a.logger.Warn("suspending customer for non-payment", "customer_id", customerID)
	return nil
}

// UnsuspendAfterPayment logs the unsuspension event.
func (a *Adapter) UnsuspendAfterPayment(_ context.Context, customerID string) error {
	a.logger.Info("unsuspending customer after payment", "customer_id", customerID)
	return nil
}

// GetBalance returns the customer's current credit balance.
func (a *Adapter) GetBalance(ctx context.Context, customerID string) (*billing.Balance, error) {
	balance, err := a.ledger.GetBalance(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("get native balance: %w", err)
	}
	return &billing.Balance{
		CustomerID:   customerID,
		BalanceCents: balance,
		Currency:     "USD",
	}, nil
}

// ProcessTopUp credits the customer's account after a confirmed payment.
func (a *Adapter) ProcessTopUp(ctx context.Context, req billing.TopUpRequest) (*billing.TopUpResult, error) {
	idempotencyKey := fmt.Sprintf("topup:%s:%s", req.CustomerID, req.Reference)
	bt, err := a.ledger.CreditAccount(
		ctx, req.CustomerID, req.AmountCents,
		fmt.Sprintf("Top-up: %s", req.Reference), &idempotencyKey,
	)
	if err != nil {
		return nil, fmt.Errorf("process native top-up: %w", err)
	}
	return &billing.TopUpResult{
		TransactionID: bt.ID,
		BalanceCents:  bt.BalanceAfter,
		Currency:      "USD",
	}, nil
}

// GetUsageHistory returns the customer's usage history.
func (a *Adapter) GetUsageHistory(
	_ context.Context, _ string, _ billing.PaginationOpts,
) (*billing.UsageHistory, error) {
	return &billing.UsageHistory{Records: []billing.UsageRecord{}}, nil
}
