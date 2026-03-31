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
type BillingProvider interface {
	Name() string
	ValidateConfig() error
	CreateUser(ctx context.Context, req CreateUserRequest) (*UserResult, error)
	GetUserBillingStatus(ctx context.Context, customerID string) (*BillingStatus, error)
	OnVMCreated(ctx context.Context, vm VMRef) error
	OnVMDeleted(ctx context.Context, vm VMRef) error
	OnVMResized(ctx context.Context, vm VMRef, oldPlanID, newPlanID string) error
	SuspendForNonPayment(ctx context.Context, customerID string) error
	UnsuspendAfterPayment(ctx context.Context, customerID string) error
	GetBalance(ctx context.Context, customerID string) (*Balance, error)
	ProcessTopUp(ctx context.Context, req TopUpRequest) (*TopUpResult, error)
	GetUsageHistory(ctx context.Context, customerID string, opts PaginationOpts) (*UsageHistory, error)
}

type VMRef struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	PlanID     string `json:"plan_id"`
	Hostname   string `json:"hostname"`
}

type CreateUserRequest struct {
	CustomerID string `json:"customer_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}

type UserResult struct {
	CustomerID string `json:"customer_id"`
	ProviderID string `json:"provider_id"`
}

type BillingStatus struct {
	CustomerID  string `json:"customer_id"`
	Provider    string `json:"provider"`
	IsActive    bool   `json:"is_active"`
	IsSuspended bool   `json:"is_suspended"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

type Balance struct {
	CustomerID   string `json:"customer_id"`
	BalanceCents int64  `json:"balance_cents"`
	Currency     string `json:"currency"`
}

type TopUpRequest struct {
	CustomerID  string `json:"customer_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Reference   string `json:"reference"`
}

type TopUpResult struct {
	TransactionID string `json:"transaction_id"`
	BalanceCents  int64  `json:"balance_cents"`
	Currency      string `json:"currency"`
}

type PaginationOpts struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

type UsageHistory struct {
	Records    []UsageRecord `json:"records"`
	TotalCount int           `json:"total_count"`
	Page       int           `json:"page"`
	PerPage    int           `json:"per_page"`
}

type UsageRecord struct {
	ID          string    `json:"id"`
	VMID        string    `json:"vm_id"`
	Description string    `json:"description"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"created_at"`
}
