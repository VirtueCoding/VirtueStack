package crypto

import (
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
