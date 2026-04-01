package blesta

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAdapter() *Adapter {
	return NewAdapter(BlestaConfig{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

func TestAdapter_Name(t *testing.T) {
	a := newTestAdapter()
	assert.Equal(t, "blesta", a.Name())
}

func TestAdapter_ValidateConfig(t *testing.T) {
	a := newTestAdapter()
	assert.NoError(t, a.ValidateConfig())
}

func TestAdapter_CreateUser(t *testing.T) {
	a := newTestAdapter()
	result, err := a.CreateUser(context.Background(), billing.CreateUserRequest{
		CustomerID: "cust-123",
		Email:      "test@example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "cust-123", result.CustomerID)
	assert.Equal(t, "cust-123", result.ProviderID)
}

func TestAdapter_GetUserBillingStatus(t *testing.T) {
	a := newTestAdapter()
	status, err := a.GetUserBillingStatus(context.Background(), "cust-123")
	require.NoError(t, err)
	assert.True(t, status.IsActive)
	assert.Equal(t, "blesta", status.Provider)
	assert.Equal(t, "cust-123", status.CustomerID)
	assert.Equal(t, "active", status.Status)
}

func TestAdapter_LifecycleHooks(t *testing.T) {
	a := newTestAdapter()
	ctx := context.Background()
	ref := billing.VMRef{ID: "vm-1", CustomerID: "cust-1"}

	assert.NoError(t, a.OnVMCreated(ctx, ref))
	assert.NoError(t, a.OnVMDeleted(ctx, ref))
	assert.NoError(t, a.OnVMResized(ctx, ref, "plan-old", "plan-new"))
	assert.NoError(t, a.SuspendForNonPayment(ctx, "cust-1"))
	assert.NoError(t, a.UnsuspendAfterPayment(ctx, "cust-1"))
}

func TestAdapter_UnsupportedOperations(t *testing.T) {
	a := newTestAdapter()
	ctx := context.Background()

	_, err := a.GetBalance(ctx, "cust-1")
	assert.True(t, errors.Is(err, sharederrors.ErrNotSupported))

	_, err = a.ProcessTopUp(ctx, billing.TopUpRequest{})
	assert.True(t, errors.Is(err, sharederrors.ErrNotSupported))

	_, err = a.GetUsageHistory(ctx, "cust-1", billing.PaginationOpts{})
	assert.True(t, errors.Is(err, sharederrors.ErrNotSupported))
}

func TestAdapter_ImplementsInterface(t *testing.T) {
	var _ billing.BillingProvider = (*Adapter)(nil)
}
