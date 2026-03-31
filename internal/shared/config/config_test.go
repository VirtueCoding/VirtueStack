package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// validBillingConfig returns a minimal ControllerConfig that passes billing validation.
func validBillingConfig() *ControllerConfig {
	return &ControllerConfig{
		Billing: BillingConfig{
			Providers: BillingProvidersConfig{
				WHMCS: BillingProviderConfig{Enabled: true, Primary: true},
			},
			GracePeriodHours:     12,
			WarningIntervalHours: 24,
		},
		PayPal: PayPalConfig{Mode: "sandbox"},
		Crypto: CryptoConfig{Provider: "disabled"},
	}
}

func TestValidateBillingConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(cfg *ControllerConfig)
		wantErr string
	}{
		{
			name:   "valid default config",
			modify: func(_ *ControllerConfig) {},
		},
		{
			name: "no providers enabled is valid",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{}
			},
		},
		{
			name: "primary but not enabled",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: false, Primary: true}
			},
			wantErr: "marked primary but not enabled",
		},
		{
			name: "two primaries",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: true}
			},
			wantErr: "exactly one enabled billing provider must be primary",
		},
		{
			name: "enabled but no primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: true, Primary: false}
			},
			wantErr: "exactly one enabled billing provider must be primary",
		},
		{
			name: "self registration without primary",
			modify: func(cfg *ControllerConfig) {
				cfg.AllowSelfRegistration = true
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{}
			},
		},
		{
			name: "stripe secret without webhook",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe.SecretKey = Secret("sk_test_123")
			},
			wantErr: "stripe: secret_key and webhook_secret must both be set",
		},
		{
			name: "stripe webhook without secret",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe.WebhookSecret = Secret("whsec_123")
			},
			wantErr: "stripe: secret_key and webhook_secret must both be set",
		},
		{
			name: "stripe both set is valid",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe.SecretKey = Secret("sk_test_123")
				cfg.Stripe.WebhookSecret = Secret("whsec_123")
			},
		},
		{
			name: "stripe both empty is valid",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe = StripeConfig{}
			},
		},
		{
			name: "paypal client_id without secret",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.ClientID = Secret("client123")
				cfg.PayPal.Mode = "sandbox"
			},
			wantErr: "paypal: client_id and client_secret must both be set",
		},
		{
			name: "paypal secret without client_id",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.ClientSecret = Secret("secret123")
				cfg.PayPal.Mode = "sandbox"
			},
			wantErr: "paypal: client_id and client_secret must both be set",
		},
		{
			name: "paypal both set is valid",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.ClientID = Secret("client123")
				cfg.PayPal.ClientSecret = Secret("secret123")
				cfg.PayPal.Mode = "production"
			},
		},
		{
			name: "paypal invalid mode",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.Mode = "staging"
			},
			wantErr: "paypal: mode must be",
		},
		{
			name: "crypto invalid provider",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "bitcoin-core"
			},
			wantErr: "crypto: provider must be",
		},
		{
			name: "crypto btcpay missing url",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
				cfg.Crypto.BTCPayAPIKey = Secret("key")
				cfg.Crypto.BTCPayStoreID = "store"
			},
			wantErr: "btcpay requires server_url",
		},
		{
			name: "crypto btcpay missing api_key",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
				cfg.Crypto.BTCPayServerURL = "https://btcpay.example.com"
				cfg.Crypto.BTCPayStoreID = "store"
			},
			wantErr: "btcpay requires server_url",
		},
		{
			name: "crypto btcpay missing store_id",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
				cfg.Crypto.BTCPayServerURL = "https://btcpay.example.com"
				cfg.Crypto.BTCPayAPIKey = Secret("key")
			},
			wantErr: "btcpay requires server_url",
		},
		{
			name: "crypto btcpay fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
				cfg.Crypto.BTCPayServerURL = "https://btcpay.example.com"
				cfg.Crypto.BTCPayAPIKey = Secret("key")
				cfg.Crypto.BTCPayStoreID = "store"
			},
		},
		{
			name: "crypto nowpayments missing api_key",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "nowpayments"
				cfg.Crypto.NOWPaymentsIPNSecret = Secret("secret")
			},
			wantErr: "nowpayments requires api_key",
		},
		{
			name: "crypto nowpayments missing ipn_secret",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "nowpayments"
				cfg.Crypto.NOWPaymentsAPIKey = Secret("key")
			},
			wantErr: "nowpayments requires api_key",
		},
		{
			name: "crypto nowpayments fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "nowpayments"
				cfg.Crypto.NOWPaymentsAPIKey = Secret("key")
				cfg.Crypto.NOWPaymentsIPNSecret = Secret("secret")
			},
		},
		{
			name: "oauth google enabled without client_id",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.Google.Enabled = true
				cfg.OAuth.Google.ClientSecret = Secret("secret")
			},
			wantErr: "oauth: google enabled but client_id or client_secret missing",
		},
		{
			name: "oauth google enabled without client_secret",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.Google.Enabled = true
				cfg.OAuth.Google.ClientID = "id"
			},
			wantErr: "oauth: google enabled but client_id or client_secret missing",
		},
		{
			name: "oauth google fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.Google.Enabled = true
				cfg.OAuth.Google.ClientID = "id"
				cfg.OAuth.Google.ClientSecret = Secret("secret")
			},
		},
		{
			name: "oauth github enabled without credentials",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.GitHub.Enabled = true
			},
			wantErr: "oauth: github enabled but client_id or client_secret missing",
		},
		{
			name: "oauth github fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.GitHub.Enabled = true
				cfg.OAuth.GitHub.ClientID = "id"
				cfg.OAuth.GitHub.ClientSecret = Secret("secret")
			},
		},
		{
			name: "negative grace_period_hours",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.GracePeriodHours = -1
			},
			wantErr: "grace_period_hours must be non-negative",
		},
		{
			name: "negative warning_interval_hours",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.WarningIntervalHours = -5
			},
			wantErr: "warning_interval_hours must be non-negative",
		},
		{
			name: "negative auto_delete_days",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.AutoDeleteDays = -1
			},
			wantErr: "auto_delete_days must be non-negative",
		},
		{
			name: "zero tuning params are valid",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.GracePeriodHours = 0
				cfg.Billing.WarningIntervalHours = 0
				cfg.Billing.AutoDeleteDays = 0
			},
		},
		{
			name: "blesta primary but not enabled",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{}
				cfg.Billing.Providers.Blesta = BillingProviderConfig{Enabled: false, Primary: true}
			},
			wantErr: "marked primary but not enabled",
		},
		{
			name: "native primary but not enabled",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{}
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: false, Primary: true}
			},
			wantErr: "marked primary but not enabled",
		},
		{
			name: "all three enabled but only one primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true}
				cfg.Billing.Providers.Blesta = BillingProviderConfig{Enabled: true}
				cfg.Blesta.APIURL = "https://blesta.example.com/api"
				cfg.Blesta.APIKey = Secret("test-key")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			err := validateBillingConfig(cfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateBillingConfig_NativeWithoutGatewaysWarns(t *testing.T) {
	cfg := validBillingConfig()
	cfg.Billing.Providers.WHMCS = BillingProviderConfig{}
	cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: true}
	// No payment gateways configured — should warn but not error
	err := validateBillingConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for native without gateways, got: %v", err)
	}
}

func TestSecret_String(t *testing.T) {
	t.Run("non-empty secret returns REDACTED", func(t *testing.T) {
		s := Secret("super-secret-value")
		got := s.String()
		if got != "[REDACTED]" {
			t.Errorf("String() = %q, want %q", got, "[REDACTED]")
		}
	})

	t.Run("empty secret returns REDACTED", func(t *testing.T) {
		s := Secret("")
		got := s.String()
		if got != "[REDACTED]" {
			t.Errorf("String() = %q, want %q", got, "[REDACTED]")
		}
	})
}

func TestSecret_Value(t *testing.T) {
	t.Run("returns underlying string", func(t *testing.T) {
		s := Secret("my-actual-secret")
		got := s.Value()
		if got != "my-actual-secret" {
			t.Errorf("Value() = %q, want %q", got, "my-actual-secret")
		}
	})

	t.Run("empty secret returns empty string", func(t *testing.T) {
		s := Secret("")
		got := s.Value()
		if got != "" {
			t.Errorf("Value() = %q, want %q", got, "")
		}
	})
}

func TestSecret_MarshalJSON(t *testing.T) {
	t.Run("returns JSON-encoded REDACTED", func(t *testing.T) {
		s := Secret("do-not-expose")
		b, err := s.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		// Raw JSON bytes should be: "\"[REDACTED]\""
		want := `"[REDACTED]"`
		if string(b) != want {
			t.Errorf("MarshalJSON() = %s, want %s", string(b), want)
		}
	})

	t.Run("empty secret also returns REDACTED JSON", func(t *testing.T) {
		s := Secret("")
		b, err := s.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		want := `"[REDACTED]"`
		if string(b) != want {
			t.Errorf("MarshalJSON() = %s, want %s", string(b), want)
		}
	})
}

func TestSecret_StructMarshalJSON(t *testing.T) {
	type Config struct {
		Name   string `json:"name"`
		Token  Secret `json:"token"`
		APIKey Secret `json:"api_key"`
	}

	cfg := Config{
		Name:   "test-config",
		Token:  Secret("real-token-value"),
		APIKey: Secret("real-api-key"),
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Ensure the actual secret values do not appear in JSON output.
	jsonStr := string(data)
	if strings.Contains(jsonStr, "real-token-value") {
		t.Errorf("JSON output contains secret token value: %s", jsonStr)
	}
	if strings.Contains(jsonStr, "real-api-key") {
		t.Errorf("JSON output contains secret api_key value: %s", jsonStr)
	}

	// Unmarshal to verify the field values are "[REDACTED]".
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if raw["token"] != "[REDACTED]" {
		t.Errorf("token field = %q, want %q", raw["token"], "[REDACTED]")
	}
	if raw["api_key"] != "[REDACTED]" {
		t.Errorf("api_key field = %q, want %q", raw["api_key"], "[REDACTED]")
	}
	if raw["name"] != "test-config" {
		t.Errorf("name field = %q, want %q", raw["name"], "test-config")
	}
}

func TestAnyBillingProviderEnabled(t *testing.T) {
	tests := []struct {
		name   string
		modify func(cfg *ControllerConfig)
		want   bool
	}{
		{"none enabled", func(cfg *ControllerConfig) {
			cfg.Billing.Providers = BillingProvidersConfig{}
		}, false},
		{"whmcs enabled", func(cfg *ControllerConfig) {
			cfg.Billing.Providers = BillingProvidersConfig{WHMCS: BillingProviderConfig{Enabled: true}}
		}, true},
		{"native enabled", func(cfg *ControllerConfig) {
			cfg.Billing.Providers = BillingProvidersConfig{Native: BillingProviderConfig{Enabled: true}}
		}, true},
		{"blesta enabled", func(cfg *ControllerConfig) {
			cfg.Billing.Providers = BillingProvidersConfig{Blesta: BillingProviderConfig{Enabled: true}}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			if got := cfg.AnyBillingProviderEnabled(); got != tt.want {
				t.Errorf("AnyBillingProviderEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrimaryBillingProvider(t *testing.T) {
	tests := []struct {
		name   string
		modify func(cfg *ControllerConfig)
		want   string
	}{
		{"whmcs primary", func(cfg *ControllerConfig) {}, "whmcs"},
		{"native primary", func(cfg *ControllerConfig) {
			cfg.Billing.Providers.WHMCS.Primary = false
			cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: true}
		}, "native"},
		{"blesta primary", func(cfg *ControllerConfig) {
			cfg.Billing.Providers.WHMCS.Primary = false
			cfg.Billing.Providers.Blesta = BillingProviderConfig{Enabled: true, Primary: true}
		}, "blesta"},
		{"none primary", func(cfg *ControllerConfig) {
			cfg.Billing.Providers.WHMCS.Primary = false
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			if got := cfg.PrimaryBillingProvider(); got != tt.want {
				t.Errorf("PrimaryBillingProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasPaymentGateway(t *testing.T) {
	tests := []struct {
		name   string
		modify func(cfg *ControllerConfig)
		want   bool
	}{
		{"no gateway", func(_ *ControllerConfig) {}, false},
		{"stripe configured", func(cfg *ControllerConfig) {
			cfg.Stripe.SecretKey = Secret("sk_test")
		}, true},
		{"paypal configured", func(cfg *ControllerConfig) {
			cfg.PayPal.ClientID = Secret("client")
		}, true},
		{"crypto btcpay", func(cfg *ControllerConfig) {
			cfg.Crypto.Provider = "btcpay"
		}, true},
		{"crypto disabled", func(cfg *ControllerConfig) {
			cfg.Crypto.Provider = "disabled"
		}, false},
		{"crypto empty", func(cfg *ControllerConfig) {
			cfg.Crypto.Provider = ""
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			if got := cfg.HasPaymentGateway(); got != tt.want {
				t.Errorf("HasPaymentGateway() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateBillingConfig_Blesta(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(cfg *ControllerConfig)
		wantErr bool
		errMsg  string
	}{
		{
			name: "blesta enabled with valid config",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = true
				cfg.Billing.Providers.Blesta.Primary = true
				cfg.Billing.Providers.WHMCS.Primary = false
				cfg.Blesta.APIURL = "https://blesta.example.com/api"
				cfg.Blesta.APIKey = Secret("test-api-key")
			},
			wantErr: false,
		},
		{
			name: "blesta enabled missing API URL",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = true
				cfg.Billing.Providers.Blesta.Primary = true
				cfg.Billing.Providers.WHMCS.Primary = false
				cfg.Blesta.APIKey = Secret("test-api-key")
			},
			wantErr: true,
			errMsg:  "BLESTA_API_URL",
		},
		{
			name: "blesta enabled missing API key",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = true
				cfg.Billing.Providers.Blesta.Primary = true
				cfg.Billing.Providers.WHMCS.Primary = false
				cfg.Blesta.APIURL = "https://blesta.example.com/api"
			},
			wantErr: true,
			errMsg:  "BLESTA_API_KEY",
		},
		{
			name: "blesta disabled — no validation needed",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.Blesta.Enabled = false
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			err := validateBillingConfig(cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
