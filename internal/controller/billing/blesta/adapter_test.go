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
