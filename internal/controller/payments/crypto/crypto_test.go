package crypto

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_Factory(t *testing.T) {
	tests := []struct {
		name         string
		cfg          FactoryConfig
		wantNil      bool
		wantProvider string
		wantErr      bool
	}{
		{
			"disabled returns nil",
			FactoryConfig{Provider: "disabled"},
			true, "", false,
		},
		{
			"empty returns nil",
			FactoryConfig{Provider: ""},
			true, "", false,
		},
		{
			"btcpay valid",
			FactoryConfig{
				Provider:        "btcpay",
				BTCPayServerURL: "https://btcpay.example.com",
				BTCPayAPIKey:    "key",
				BTCPayStoreID:   "store",
				HTTPClient:      http.DefaultClient,
				Logger:          testLogger(),
			},
			false, "btcpay", false,
		},
		{
			"btcpay missing server URL",
			FactoryConfig{
				Provider:      "btcpay",
				BTCPayAPIKey:  "key",
				BTCPayStoreID: "store",
				Logger:        testLogger(),
			},
			false, "", true,
		},
		{
			"nowpayments valid",
			FactoryConfig{
				Provider:             "nowpayments",
				NOWPaymentsAPIKey:    "key",
				NOWPaymentsIPNSecret: "secret",
				HTTPClient:           http.DefaultClient,
				Logger:               testLogger(),
			},
			false, "nowpayments", false,
		},
		{
			"nowpayments missing API key",
			FactoryConfig{
				Provider:             "nowpayments",
				NOWPaymentsIPNSecret: "secret",
				Logger:               testLogger(),
			},
			false, "", true,
		},
		{
			"unknown provider",
			FactoryConfig{Provider: "coinbase"},
			false, "", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, provider)
				return
			}
			require.NotNil(t, provider)
			assert.Equal(t, tt.wantProvider, provider.ProviderName())
		})
	}
}

type namedCryptoProvider struct {
	name string
}

func (p namedCryptoProvider) CreatePaymentSession(
	_ context.Context, _ *CreatePaymentRequest,
) (*PaymentSession, error) {
	return &PaymentSession{}, nil
}

func (p namedCryptoProvider) HandleWebhook(
	_ context.Context, _ http.Header, _ []byte,
) (*WebhookResult, error) {
	return nil, nil
}

func (p namedCryptoProvider) GetPaymentStatus(
	_ context.Context, _ string,
) (*PaymentStatus, error) {
	return nil, nil
}

func (p namedCryptoProvider) ProviderName() string {
	return p.name
}

func TestAdapter_NameUsesConcreteProviderName(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
	}{
		{"btcpay", "btcpay"},
		{"nowpayments", "nowpayments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewAdapter(namedCryptoProvider{name: tt.providerName})

			assert.Equal(t, tt.providerName, adapter.Name())
		})
	}
}
