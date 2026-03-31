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
