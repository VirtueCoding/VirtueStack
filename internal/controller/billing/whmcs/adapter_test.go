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
		ID: "vm-123", CustomerID: "cust-456",
		PlanID: "plan-789", Hostname: "test-vm",
	}
	tests := []struct {
		name string
		fn   func() error
	}{
		{"OnVMCreated", func() error { return adapter.OnVMCreated(ctx, ref) }},
		{"OnVMDeleted", func() error { return adapter.OnVMDeleted(ctx, ref) }},
		{"OnVMResized", func() error { return adapter.OnVMResized(ctx, ref, "old", "new") }},
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
		{"GetBalance", func() error { _, err := adapter.GetBalance(ctx, "cust-123"); return err }},
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
