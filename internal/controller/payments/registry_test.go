package payments_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) CreatePaymentSession(
	_ context.Context, _ payments.PaymentRequest,
) (*payments.PaymentSession, error) {
	return nil, nil
}

func (m *mockProvider) HandleWebhook(
	_ context.Context, _ []byte, _ string,
) (*payments.WebhookEvent, error) {
	return nil, nil
}

func (m *mockProvider) GetPaymentStatus(
	_ context.Context, _ string,
) (*payments.PaymentStatus, error) {
	return nil, nil
}

func (m *mockProvider) RefundPayment(
	_ context.Context, _ string, _ int64,
) (*payments.RefundResult, error) {
	return nil, nil
}

func (m *mockProvider) ValidateConfig() error { return nil }

func TestPaymentRegistry_RegisterAndGet(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		wantErr   bool
		errTarget error
	}{
		{"registered provider", "stripe", false, nil},
		{"unknown provider", "unknown", true, sharederrors.ErrNotFound},
	}

	reg := payments.NewPaymentRegistry()
	reg.Register("stripe", &mockProvider{name: "stripe"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := reg.Get(tt.provider)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errTarget))
				assert.Nil(t, p)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.provider, p.Name())
			}
		})
	}
}

func TestPaymentRegistry_Available(t *testing.T) {
	reg := payments.NewPaymentRegistry()
	reg.Register("stripe", &mockProvider{name: "stripe"})
	reg.Register("paypal", &mockProvider{name: "paypal"})

	available := reg.Available()
	sort.Strings(available)
	assert.Equal(t, []string{"paypal", "stripe"}, available)
}

func TestPaymentRegistry_DuplicateRegistrationPanics(t *testing.T) {
	reg := payments.NewPaymentRegistry()
	reg.Register("stripe", &mockProvider{name: "stripe"})

	assert.Panics(t, func() {
		reg.Register("stripe", &mockProvider{name: "stripe"})
	})
}
