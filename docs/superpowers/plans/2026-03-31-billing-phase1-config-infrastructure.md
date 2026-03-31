# Billing Phase 1: Feature Flags + Config Infrastructure — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add all billing/OAuth/payment configuration to the Config struct, implement startup validation, and wire conditional route registration so billing features can be toggled without code changes.

**Architecture:** Extends `internal/shared/config/config.go` with new fields, adds validation logic, updates `server.go` for conditional billing route registration, and adds billing RBAC permissions.

**Tech Stack:** Go 1.26

**Depends on:** Phase 0 (billing registry must exist)
**Depended on by:** Phase 2 (Notifications), Phase 3 (Credit Ledger), Phase 8 (OAuth)

---

## Task 1: Add Billing Provider Config Structs

- [ ] Add `BillingProvidersConfig` sub-struct and billing provider fields to `ControllerConfig`

**File:** `internal/shared/config/config.go`

### 1a. Add the `BillingProviderConfig` and `BillingProvidersConfig` structs

Add after the `RedisConfig` struct (after line 118):

```go
// BillingProviderConfig holds the enabled/primary flags for a single billing provider.
type BillingProviderConfig struct {
	Enabled bool `yaml:"enabled"`
	Primary bool `yaml:"primary"`
}

// BillingProvidersConfig holds the billing provider toggles.
type BillingProvidersConfig struct {
	WHMCS  BillingProviderConfig `yaml:"whmcs"`
	Native BillingProviderConfig `yaml:"native"`
	Blesta BillingProviderConfig `yaml:"blesta"`
}

// BillingConfig holds all billing-related configuration.
type BillingConfig struct {
	Providers BillingProvidersConfig `yaml:"providers"`

	// Tuning parameters for native billing
	GracePeriodHours    int `yaml:"grace_period_hours"`
	WarningIntervalHours int `yaml:"warning_interval_hours"`
	AutoDeleteDays      int `yaml:"auto_delete_days"`
}
```

### 1b. Add `Billing` field to `ControllerConfig`

Add to the `ControllerConfig` struct after the `FileStorage` field (after line 147):

```go
	Billing BillingConfig `yaml:"billing"`
```

### 1c. Set defaults in `LoadControllerConfig`

Add to the defaults block in `LoadControllerConfig` (around line 221, after `RegistrationEmailVerification: true`):

```go
		Billing: BillingConfig{
			Providers: BillingProvidersConfig{
				WHMCS: BillingProviderConfig{Enabled: true, Primary: false},
			},
			GracePeriodHours:     12,
			WarningIntervalHours: 24,
			AutoDeleteDays:       0,
		},
```

### 1d. Add `applyEnvOverridesBilling` function and register it

Add a new function:

```go
// applyEnvOverridesBilling applies billing-related environment variables.
func applyEnvOverridesBilling(cfg *ControllerConfig) {
	if v := os.Getenv("BILLING_WHMCS_ENABLED"); v != "" {
		cfg.Billing.Providers.WHMCS.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_WHMCS_PRIMARY"); v != "" {
		cfg.Billing.Providers.WHMCS.Primary = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_NATIVE_ENABLED"); v != "" {
		cfg.Billing.Providers.Native.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_NATIVE_PRIMARY"); v != "" {
		cfg.Billing.Providers.Native.Primary = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_BLESTA_ENABLED"); v != "" {
		cfg.Billing.Providers.Blesta.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_BLESTA_PRIMARY"); v != "" {
		cfg.Billing.Providers.Blesta.Primary = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_GRACE_PERIOD_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Billing.GracePeriodHours = n
		} else {
			slog.Warn("invalid BILLING_GRACE_PERIOD_HOURS value, ignoring", "value", v, "error", err)
		}
	}
	if v := os.Getenv("BILLING_WARNING_INTERVAL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Billing.WarningIntervalHours = n
		} else {
			slog.Warn("invalid BILLING_WARNING_INTERVAL_HOURS value, ignoring", "value", v, "error", err)
		}
	}
	if v := os.Getenv("BILLING_NATIVE_AUTO_DELETE_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Billing.AutoDeleteDays = n
		} else {
			slog.Warn("invalid BILLING_NATIVE_AUTO_DELETE_DAYS value, ignoring", "value", v, "error", err)
		}
	}
}
```

Register it in `applyEnvOverrides` (line 318):

```go
func applyEnvOverrides(cfg *ControllerConfig) {
	applyEnvOverridesCore(cfg)
	applyEnvOverridesNATS(cfg)
	applyEnvOverridesSMTP(cfg)
	applyEnvOverridesTelegram(cfg)
	applyEnvOverridesStorage(cfg)
	applyEnvOverridesBilling(cfg)
}
```

**Test:**

```bash
go test -race -run TestSecret ./internal/shared/config/...
# Expected: PASS (existing tests still pass; no new tests in this task)
```

**Commit:**

```
feat(config): add billing provider config structs and env overrides

Add BillingProviderConfig, BillingProvidersConfig, and BillingConfig
structs to ControllerConfig. WHMCS defaults to enabled. Native and
Blesta default to disabled. Billing tuning params (grace period,
warning interval, auto-delete days) included with sensible defaults.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 2: Add Payment Gateway Config Structs

- [ ] Add Stripe, PayPal, and crypto payment gateway config to `ControllerConfig`

**File:** `internal/shared/config/config.go`

### 2a. Add payment config structs

Add after the `BillingConfig` struct:

```go
// StripeConfig holds Stripe payment gateway configuration.
type StripeConfig struct {
	SecretKey      Secret `yaml:"secret_key" env:"STRIPE_SECRET_KEY"`
	WebhookSecret  Secret `yaml:"webhook_secret" env:"STRIPE_WEBHOOK_SECRET"`
	PublishableKey string `yaml:"publishable_key" env:"STRIPE_PUBLISHABLE_KEY"`
}

// PayPalConfig holds PayPal payment gateway configuration.
type PayPalConfig struct {
	ClientID     Secret `yaml:"client_id" env:"PAYPAL_CLIENT_ID"`
	ClientSecret Secret `yaml:"client_secret" env:"PAYPAL_CLIENT_SECRET"`
	Mode         string `yaml:"mode" env:"PAYPAL_MODE"` // "sandbox" or "production"
}

// CryptoConfig holds cryptocurrency payment configuration.
type CryptoConfig struct {
	Provider          string `yaml:"provider" env:"CRYPTO_PROVIDER"` // "btcpay", "nowpayments", or "disabled"
	BTCPayServerURL   string `yaml:"btcpay_server_url" env:"BTCPAY_SERVER_URL"`
	BTCPayAPIKey      Secret `yaml:"btcpay_api_key" env:"BTCPAY_API_KEY"`
	BTCPayStoreID     string `yaml:"btcpay_store_id" env:"BTCPAY_STORE_ID"`
	NOWPaymentsAPIKey Secret `yaml:"nowpayments_api_key" env:"NOWPAYMENTS_API_KEY"`
	NOWPaymentsIPNSecret Secret `yaml:"nowpayments_ipn_secret" env:"NOWPAYMENTS_IPN_SECRET"`
}
```

### 2b. Add payment fields to `ControllerConfig`

Add after the `Billing` field:

```go
	// Payment gateway configuration
	Stripe StripeConfig `yaml:"stripe"`
	PayPal PayPalConfig `yaml:"paypal"`
	Crypto CryptoConfig `yaml:"crypto"`
```

### 2c. Set PayPal and Crypto defaults in `LoadControllerConfig`

Add to the defaults block:

```go
		PayPal: PayPalConfig{
			Mode: "sandbox",
		},
		Crypto: CryptoConfig{
			Provider: "disabled",
		},
```

### 2d. Add `applyEnvOverridesPayments` function and register it

```go
// applyEnvOverridesPayments applies payment-gateway-related environment variables.
func applyEnvOverridesPayments(cfg *ControllerConfig) {
	// Stripe
	if v := os.Getenv("STRIPE_SECRET_KEY"); v != "" {
		cfg.Stripe.SecretKey = Secret(v)
	}
	if v := os.Getenv("STRIPE_WEBHOOK_SECRET"); v != "" {
		cfg.Stripe.WebhookSecret = Secret(v)
	}
	if v := os.Getenv("STRIPE_PUBLISHABLE_KEY"); v != "" {
		cfg.Stripe.PublishableKey = v
	}

	// PayPal
	if v := os.Getenv("PAYPAL_CLIENT_ID"); v != "" {
		cfg.PayPal.ClientID = Secret(v)
	}
	if v := os.Getenv("PAYPAL_CLIENT_SECRET"); v != "" {
		cfg.PayPal.ClientSecret = Secret(v)
	}
	if v := os.Getenv("PAYPAL_MODE"); v != "" {
		cfg.PayPal.Mode = v
	}

	// Crypto
	if v := os.Getenv("CRYPTO_PROVIDER"); v != "" {
		cfg.Crypto.Provider = v
	}
	if v := os.Getenv("BTCPAY_SERVER_URL"); v != "" {
		cfg.Crypto.BTCPayServerURL = v
	}
	if v := os.Getenv("BTCPAY_API_KEY"); v != "" {
		cfg.Crypto.BTCPayAPIKey = Secret(v)
	}
	if v := os.Getenv("BTCPAY_STORE_ID"); v != "" {
		cfg.Crypto.BTCPayStoreID = v
	}
	if v := os.Getenv("NOWPAYMENTS_API_KEY"); v != "" {
		cfg.Crypto.NOWPaymentsAPIKey = Secret(v)
	}
	if v := os.Getenv("NOWPAYMENTS_IPN_SECRET"); v != "" {
		cfg.Crypto.NOWPaymentsIPNSecret = Secret(v)
	}
}
```

Register in `applyEnvOverrides`:

```go
func applyEnvOverrides(cfg *ControllerConfig) {
	applyEnvOverridesCore(cfg)
	applyEnvOverridesNATS(cfg)
	applyEnvOverridesSMTP(cfg)
	applyEnvOverridesTelegram(cfg)
	applyEnvOverridesStorage(cfg)
	applyEnvOverridesBilling(cfg)
	applyEnvOverridesPayments(cfg)
}
```

**Test:**

```bash
go test -race -run TestSecret ./internal/shared/config/...
# Expected: PASS
```

**Commit:**

```
feat(config): add Stripe, PayPal, and crypto payment gateway config

Add StripeConfig, PayPalConfig, and CryptoConfig structs with Secret
types for sensitive fields. PayPal defaults to sandbox mode. Crypto
defaults to disabled. All fields overridable via environment variables.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 3: Add OAuth Config Structs

- [ ] Add Google and GitHub OAuth config to `ControllerConfig`

**File:** `internal/shared/config/config.go`

### 3a. Add OAuth config structs

Add after the `CryptoConfig` struct:

```go
// OAuthProviderConfig holds configuration for a single OAuth provider.
type OAuthProviderConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret Secret `yaml:"client_secret"`
}

// OAuthConfig holds all OAuth provider configurations.
type OAuthConfig struct {
	Google OAuthProviderConfig `yaml:"google"`
	GitHub OAuthProviderConfig `yaml:"github"`
}
```

### 3b. Add `OAuth` field to `ControllerConfig`

Add after the `Crypto` field:

```go
	// OAuth configuration
	OAuth OAuthConfig `yaml:"oauth"`
```

### 3c. Add `applyEnvOverridesOAuth` function and register it

```go
// applyEnvOverridesOAuth applies OAuth-related environment variables.
func applyEnvOverridesOAuth(cfg *ControllerConfig) {
	if v := os.Getenv("OAUTH_GOOGLE_ENABLED"); v != "" {
		cfg.OAuth.Google.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("OAUTH_GOOGLE_CLIENT_ID"); v != "" {
		cfg.OAuth.Google.ClientID = v
	}
	if v := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET"); v != "" {
		cfg.OAuth.Google.ClientSecret = Secret(v)
	}
	if v := os.Getenv("OAUTH_GITHUB_ENABLED"); v != "" {
		cfg.OAuth.GitHub.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("OAUTH_GITHUB_CLIENT_ID"); v != "" {
		cfg.OAuth.GitHub.ClientID = v
	}
	if v := os.Getenv("OAUTH_GITHUB_CLIENT_SECRET"); v != "" {
		cfg.OAuth.GitHub.ClientSecret = Secret(v)
	}
}
```

Register in `applyEnvOverrides`:

```go
func applyEnvOverrides(cfg *ControllerConfig) {
	applyEnvOverridesCore(cfg)
	applyEnvOverridesNATS(cfg)
	applyEnvOverridesSMTP(cfg)
	applyEnvOverridesTelegram(cfg)
	applyEnvOverridesStorage(cfg)
	applyEnvOverridesBilling(cfg)
	applyEnvOverridesPayments(cfg)
	applyEnvOverridesOAuth(cfg)
}
```

**Test:**

```bash
go test -race -run TestSecret ./internal/shared/config/...
# Expected: PASS
```

**Commit:**

```
feat(config): add Google and GitHub OAuth provider config

Add OAuthProviderConfig and OAuthConfig structs. Both providers default
to disabled. ClientSecret uses Secret type for redacted logging.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 4: Add Config Validation Function

- [ ] Add `validateBillingConfig` function with all startup validation rules

**File:** `internal/shared/config/config.go`

### 4a. Add the `validateBillingConfig` function

Add after the `validateProductionConfig` function (after line 620):

```go
// validateBillingConfig validates billing-related configuration at startup.
// It enforces the following rules:
//   - If any billing provider is enabled, exactly one must be primary.
//   - If ALLOW_SELF_REGISTRATION is true, exactly one provider must be primary.
//   - If BILLING_NATIVE_ENABLED is true but no payment gateways are configured, log WARN.
//   - Stripe requires both secret key and webhook secret when configured.
//   - PayPal requires both client ID and client secret when configured.
//   - BTCPay requires server URL, API key, and store ID.
//   - NOWPayments requires API key and IPN secret.
//   - OAuth providers require both client ID and client secret when enabled.
//   - PayPal mode must be "sandbox" or "production".
//   - Crypto provider must be "btcpay", "nowpayments", or "disabled".
func validateBillingConfig(cfg *ControllerConfig) error {
	providers := []struct {
		name    string
		enabled bool
		primary bool
	}{
		{"whmcs", cfg.Billing.Providers.WHMCS.Enabled, cfg.Billing.Providers.WHMCS.Primary},
		{"native", cfg.Billing.Providers.Native.Enabled, cfg.Billing.Providers.Native.Primary},
		{"blesta", cfg.Billing.Providers.Blesta.Enabled, cfg.Billing.Providers.Blesta.Primary},
	}

	// A provider marked primary must also be enabled.
	for _, p := range providers {
		if p.primary && !p.enabled {
			return fmt.Errorf("billing provider %q is marked primary but not enabled", p.name)
		}
	}

	// Count enabled and primary providers.
	var enabledCount, primaryCount int
	for _, p := range providers {
		if p.enabled {
			enabledCount++
		}
		if p.primary {
			primaryCount++
		}
	}

	// If any provider is enabled, exactly one must be primary.
	if enabledCount > 0 && primaryCount != 1 {
		return fmt.Errorf("exactly one billing provider must be primary when providers are enabled (found %d primary out of %d enabled)", primaryCount, enabledCount)
	}

	// If self-registration is on, exactly one provider must be primary (already validated above
	// when enabledCount > 0, but catch the case where no providers are enabled at all).
	if cfg.AllowSelfRegistration && primaryCount == 0 {
		return fmt.Errorf("ALLOW_SELF_REGISTRATION=true requires exactly one billing provider to be primary")
	}

	// Warn (not fatal) if native billing is enabled but no payment gateways are configured.
	if cfg.Billing.Providers.Native.Enabled {
		hasStripe := cfg.Stripe.SecretKey.Value() != ""
		hasPayPal := cfg.PayPal.ClientID.Value() != ""
		hasCrypto := cfg.Crypto.Provider != "" && cfg.Crypto.Provider != "disabled"
		if !hasStripe && !hasPayPal && !hasCrypto {
			slog.Warn("BILLING_NATIVE_ENABLED=true but no payment gateways are configured; customers can only receive manual credit adjustments")
		}
	}

	// Stripe: if either key is set, both must be set.
	stripeHasSecret := cfg.Stripe.SecretKey.Value() != ""
	stripeHasWebhook := cfg.Stripe.WebhookSecret.Value() != ""
	if stripeHasSecret != stripeHasWebhook {
		return fmt.Errorf("Stripe requires both STRIPE_SECRET_KEY and STRIPE_WEBHOOK_SECRET (got secret_key=%t, webhook_secret=%t)", stripeHasSecret, stripeHasWebhook)
	}

	// PayPal: if either key is set, both must be set.
	paypalHasID := cfg.PayPal.ClientID.Value() != ""
	paypalHasSecret := cfg.PayPal.ClientSecret.Value() != ""
	if paypalHasID != paypalHasSecret {
		return fmt.Errorf("PayPal requires both PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET (got client_id=%t, client_secret=%t)", paypalHasID, paypalHasSecret)
	}

	// PayPal mode validation.
	if cfg.PayPal.Mode != "sandbox" && cfg.PayPal.Mode != "production" {
		return fmt.Errorf("PAYPAL_MODE must be \"sandbox\" or \"production\" (got %q)", cfg.PayPal.Mode)
	}

	// Crypto provider validation.
	switch cfg.Crypto.Provider {
	case "disabled", "":
		// OK — no further validation needed.
	case "btcpay":
		var missing []string
		if cfg.Crypto.BTCPayServerURL == "" {
			missing = append(missing, "BTCPAY_SERVER_URL")
		}
		if cfg.Crypto.BTCPayAPIKey.Value() == "" {
			missing = append(missing, "BTCPAY_API_KEY")
		}
		if cfg.Crypto.BTCPayStoreID == "" {
			missing = append(missing, "BTCPAY_STORE_ID")
		}
		if len(missing) > 0 {
			return fmt.Errorf("CRYPTO_PROVIDER=btcpay requires: %s", strings.Join(missing, ", "))
		}
	case "nowpayments":
		var missing []string
		if cfg.Crypto.NOWPaymentsAPIKey.Value() == "" {
			missing = append(missing, "NOWPAYMENTS_API_KEY")
		}
		if cfg.Crypto.NOWPaymentsIPNSecret.Value() == "" {
			missing = append(missing, "NOWPAYMENTS_IPN_SECRET")
		}
		if len(missing) > 0 {
			return fmt.Errorf("CRYPTO_PROVIDER=nowpayments requires: %s", strings.Join(missing, ", "))
		}
	default:
		return fmt.Errorf("CRYPTO_PROVIDER must be \"btcpay\", \"nowpayments\", or \"disabled\" (got %q)", cfg.Crypto.Provider)
	}

	// OAuth: if enabled, both client ID and secret are required.
	if cfg.OAuth.Google.Enabled {
		if cfg.OAuth.Google.ClientID == "" || cfg.OAuth.Google.ClientSecret.Value() == "" {
			return fmt.Errorf("OAUTH_GOOGLE_ENABLED=true requires both OAUTH_GOOGLE_CLIENT_ID and OAUTH_GOOGLE_CLIENT_SECRET")
		}
	}
	if cfg.OAuth.GitHub.Enabled {
		if cfg.OAuth.GitHub.ClientID == "" || cfg.OAuth.GitHub.ClientSecret.Value() == "" {
			return fmt.Errorf("OAUTH_GITHUB_ENABLED=true requires both OAUTH_GITHUB_CLIENT_ID and OAUTH_GITHUB_CLIENT_SECRET")
		}
	}

	// Billing tuning parameter validation.
	if cfg.Billing.GracePeriodHours < 0 {
		return fmt.Errorf("BILLING_GRACE_PERIOD_HOURS must be non-negative (got %d)", cfg.Billing.GracePeriodHours)
	}
	if cfg.Billing.WarningIntervalHours < 0 {
		return fmt.Errorf("BILLING_WARNING_INTERVAL_HOURS must be non-negative (got %d)", cfg.Billing.WarningIntervalHours)
	}
	if cfg.Billing.AutoDeleteDays < 0 {
		return fmt.Errorf("BILLING_NATIVE_AUTO_DELETE_DAYS must be non-negative (got %d)", cfg.Billing.AutoDeleteDays)
	}

	return nil
}
```

### 4b. Call `validateBillingConfig` from `LoadControllerConfig`

Insert after the `validateDefaultPasswords` call (after line 248) and before the `return cfg, nil`:

```go
	// Validate billing configuration
	if err := validateBillingConfig(cfg); err != nil {
		return nil, fmt.Errorf("billing config validation: %w", err)
	}
```

**Test:**

```bash
go test -race -run TestSecret ./internal/shared/config/...
# Expected: PASS
```

**Commit:**

```
feat(config): add billing config validation at startup

Validate billing provider primary/enabled consistency, payment gateway
credential completeness, OAuth provider requirements, and billing
tuning parameters. Warns (non-fatal) when native billing has no
payment gateways configured.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 5: Add Config Validation Tests

- [ ] Add comprehensive tests for `validateBillingConfig`

**File:** `internal/shared/config/config_test.go`

### 5a. Add test helper function

```go
// validBillingConfig returns a ControllerConfig with billing set to the minimum
// valid state (one provider enabled and primary, PayPal mode set).
func validBillingConfig() *ControllerConfig {
	return &ControllerConfig{
		Billing: BillingConfig{
			Providers: BillingProvidersConfig{
				WHMCS: BillingProviderConfig{Enabled: true, Primary: true},
			},
		},
		PayPal: PayPalConfig{Mode: "sandbox"},
		Crypto: CryptoConfig{Provider: "disabled"},
	}
}
```

### 5b. Add `TestValidateBillingConfig` table-driven test

```go
func TestValidateBillingConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(cfg *ControllerConfig)
		wantErr string
	}{
		{
			name:   "valid defaults — WHMCS enabled and primary",
			modify: func(cfg *ControllerConfig) {},
		},
		{
			name: "valid — native enabled and primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: false}
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: true}
			},
		},
		{
			name: "valid — multiple enabled but one primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: true, Primary: true}
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: false}
			},
		},
		{
			name: "error — primary but not enabled",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: false, Primary: true}
			},
			wantErr: "marked primary but not enabled",
		},
		{
			name: "error — multiple primaries",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: true, Primary: true}
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: true}
			},
			wantErr: "exactly one billing provider must be primary",
		},
		{
			name: "error — enabled but no primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: true, Primary: false}
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: false}
			},
			wantErr: "exactly one billing provider must be primary",
		},
		{
			name: "error — self-registration with no primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{}
				cfg.AllowSelfRegistration = true
			},
			wantErr: "ALLOW_SELF_REGISTRATION=true requires exactly one billing provider to be primary",
		},
		{
			name: "valid — self-registration with primary",
			modify: func(cfg *ControllerConfig) {
				cfg.AllowSelfRegistration = true
			},
		},
		{
			name: "error — stripe secret without webhook secret",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe.SecretKey = Secret("sk_test_123")
			},
			wantErr: "Stripe requires both",
		},
		{
			name: "error — stripe webhook secret without secret key",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe.WebhookSecret = Secret("whsec_123")
			},
			wantErr: "Stripe requires both",
		},
		{
			name: "valid — stripe both keys set",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe.SecretKey = Secret("sk_test_123")
				cfg.Stripe.WebhookSecret = Secret("whsec_123")
			},
		},
		{
			name: "error — paypal client ID without secret",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.ClientID = Secret("client_123")
			},
			wantErr: "PayPal requires both",
		},
		{
			name: "error — paypal client secret without ID",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.ClientSecret = Secret("secret_123")
			},
			wantErr: "PayPal requires both",
		},
		{
			name: "valid — paypal both keys set",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.ClientID = Secret("client_123")
				cfg.PayPal.ClientSecret = Secret("secret_123")
			},
		},
		{
			name: "error — paypal invalid mode",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.Mode = "invalid"
			},
			wantErr: "PAYPAL_MODE must be",
		},
		{
			name: "error — crypto btcpay missing server URL",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
				cfg.Crypto.BTCPayAPIKey = Secret("key")
				cfg.Crypto.BTCPayStoreID = "store"
			},
			wantErr: "BTCPAY_SERVER_URL",
		},
		{
			name: "error — crypto btcpay missing all fields",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
			},
			wantErr: "BTCPAY_SERVER_URL",
		},
		{
			name: "valid — crypto btcpay fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
				cfg.Crypto.BTCPayServerURL = "https://btcpay.example.com"
				cfg.Crypto.BTCPayAPIKey = Secret("api-key")
				cfg.Crypto.BTCPayStoreID = "store-id"
			},
		},
		{
			name: "error — crypto nowpayments missing API key",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "nowpayments"
				cfg.Crypto.NOWPaymentsIPNSecret = Secret("ipn-secret")
			},
			wantErr: "NOWPAYMENTS_API_KEY",
		},
		{
			name: "error — crypto nowpayments missing IPN secret",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "nowpayments"
				cfg.Crypto.NOWPaymentsAPIKey = Secret("api-key")
			},
			wantErr: "NOWPAYMENTS_IPN_SECRET",
		},
		{
			name: "valid — crypto nowpayments fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "nowpayments"
				cfg.Crypto.NOWPaymentsAPIKey = Secret("api-key")
				cfg.Crypto.NOWPaymentsIPNSecret = Secret("ipn-secret")
			},
		},
		{
			name: "error — unknown crypto provider",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "coinbase"
			},
			wantErr: "CRYPTO_PROVIDER must be",
		},
		{
			name: "error — google OAuth enabled without client ID",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.Google.Enabled = true
				cfg.OAuth.Google.ClientSecret = Secret("secret")
			},
			wantErr: "OAUTH_GOOGLE_ENABLED=true requires both",
		},
		{
			name: "error — google OAuth enabled without client secret",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.Google.Enabled = true
				cfg.OAuth.Google.ClientID = "client-id"
			},
			wantErr: "OAUTH_GOOGLE_ENABLED=true requires both",
		},
		{
			name: "valid — google OAuth fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.Google.Enabled = true
				cfg.OAuth.Google.ClientID = "client-id"
				cfg.OAuth.Google.ClientSecret = Secret("secret")
			},
		},
		{
			name: "error — github OAuth enabled without credentials",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.GitHub.Enabled = true
			},
			wantErr: "OAUTH_GITHUB_ENABLED=true requires both",
		},
		{
			name: "valid — github OAuth fully configured",
			modify: func(cfg *ControllerConfig) {
				cfg.OAuth.GitHub.Enabled = true
				cfg.OAuth.GitHub.ClientID = "client-id"
				cfg.OAuth.GitHub.ClientSecret = Secret("secret")
			},
		},
		{
			name: "error — negative grace period",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.GracePeriodHours = -1
			},
			wantErr: "BILLING_GRACE_PERIOD_HOURS must be non-negative",
		},
		{
			name: "error — negative warning interval",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.WarningIntervalHours = -1
			},
			wantErr: "BILLING_WARNING_INTERVAL_HOURS must be non-negative",
		},
		{
			name: "error — negative auto-delete days",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.AutoDeleteDays = -1
			},
			wantErr: "BILLING_NATIVE_AUTO_DELETE_DAYS must be non-negative",
		},
		{
			name: "valid — no providers enabled (all disabled)",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS = BillingProviderConfig{}
				cfg.Billing.Providers.Native = BillingProviderConfig{}
				cfg.Billing.Providers.Blesta = BillingProviderConfig{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)

			err := validateBillingConfig(cfg)

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}
```

### 5c. Add test for native billing without gateways warning (non-fatal)

```go
func TestValidateBillingConfig_NativeWithoutGatewaysWarns(t *testing.T) {
	cfg := validBillingConfig()
	cfg.Billing.Providers.WHMCS = BillingProviderConfig{Enabled: false}
	cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: true}
	// No payment gateways configured

	// Should NOT return an error — only logs a warning.
	err := validateBillingConfig(cfg)
	assert.NoError(t, err)
}
```

**Test:**

```bash
go test -race -run TestValidateBillingConfig ./internal/shared/config/...
# Expected: PASS — all 30+ subtests pass
```

**Commit:**

```
test(config): add comprehensive billing config validation tests

Cover billing provider primary/enabled rules, payment gateway credential
pairing, crypto provider field requirements, OAuth credential checks,
tuning parameter bounds, and the native-without-gateways warning path.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 6: Add Billing RBAC Permissions

- [ ] Add `billing:read` and `billing:write` permission constants

**File:** `internal/controller/models/permission.go`

### 6a. Add billing permission constants

Add after the storage backends permission block (after line 76):

```go
// Permission constants for billing resource.
const (
	PermissionBillingRead  Permission = "billing:read"
	PermissionBillingWrite Permission = "billing:write"
)
```

### 6b. Add to `allPermissions` slice

Update the `allPermissions` var (line 86) to include billing permissions at the end:

```go
var allPermissions = []Permission{
	PermissionPlansRead, PermissionPlansWrite, PermissionPlansDelete,
	PermissionNodesRead, PermissionNodesWrite, PermissionNodesDelete,
	PermissionCustomersRead, PermissionCustomersWrite, PermissionCustomersDelete,
	PermissionVMsRead, PermissionVMsWrite, PermissionVMsDelete,
	PermissionSettingsRead, PermissionSettingsWrite,
	PermissionBackupsRead, PermissionBackupsWrite,
	PermissionIPSetsRead, PermissionIPSetsWrite, PermissionIPSetsDelete,
	PermissionTemplatesRead, PermissionTemplatesWrite,
	PermissionRDNSRead, PermissionRDNSWrite,
	PermissionAuditLogsRead,
	PermissionStorageBackendsRead, PermissionStorageBackendsWrite, PermissionStorageBackendsDelete,
	PermissionBillingRead, PermissionBillingWrite,
}
```

### 6c. Add billing permissions to default role sets

Update `defaultPermissions` to include billing permissions:

- **super_admin**: gets all permissions (automatic via `allPermissions`).
- **admin**: add `PermissionBillingRead, PermissionBillingWrite` to the admin slice.
- **viewer**: add `PermissionBillingRead` to the viewer slice.

In the `admin` role default list (around line 113), add before the closing brace:

```go
		PermissionBillingRead, PermissionBillingWrite,
```

In the `viewer` role default list (around line 128), add before the closing brace:

```go
		PermissionBillingRead,
```

**Test:**

```bash
go test -race -run TestGetAllPermissions ./internal/controller/models/...
# Expected: PASS — allPermissions now has 29 entries (was 27)
```

**Commit:**

```
feat(rbac): add billing:read and billing:write permissions

Add billing permissions to the RBAC system. Super admins and admins get
both read and write. Viewers get read-only. Enables future billing
admin routes to gate access via RequireAdminPermission middleware.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 7: Add Billing Permission Tests

- [ ] Add tests verifying billing permissions in role defaults

**File:** `internal/controller/models/permission_test.go`

### 7a. Add test for billing permission inclusion in roles

```go
func TestBillingPermissions_InRoleDefaults(t *testing.T) {
	tests := []struct {
		name         string
		role         string
		wantRead     bool
		wantWrite    bool
	}{
		{"super_admin has billing:read", RoleSuperAdmin, true, true},
		{"admin has billing:read and write", RoleAdmin, true, true},
		{"viewer has billing:read only", RoleViewer, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms := GetDefaultPermissions(tt.role)
			require.NotNil(t, perms)
			assert.Equal(t, tt.wantRead, HasPermission(perms, PermissionBillingRead), "billing:read")
			assert.Equal(t, tt.wantWrite, HasPermission(perms, PermissionBillingWrite), "billing:write")
		})
	}
}

func TestGetAllPermissions_IncludesBilling(t *testing.T) {
	all := GetAllPermissions()
	assert.Contains(t, all, PermissionBillingRead)
	assert.Contains(t, all, PermissionBillingWrite)
}
```

**Test:**

```bash
go test -race -run TestBillingPermissions ./internal/controller/models/...
go test -race -run TestGetAllPermissions_IncludesBilling ./internal/controller/models/...
# Expected: PASS
```

**Commit:**

```
test(rbac): add billing permission tests

Verify billing:read and billing:write are present in all role defaults
and in the allPermissions slice.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 8: Update `.env.example`

- [ ] Add all new billing, payment, OAuth, and tuning env vars to `.env.example`

**File:** `.env.example`

### 8a. Add billing section

Add before the `# --- Environment ---` section (before line 54):

```bash
# --- Billing Provider Configuration ---
# Enable/disable billing providers and set exactly one as primary.
# Default: WHMCS enabled (for backward compatibility), none primary.
# BILLING_WHMCS_ENABLED=true
# BILLING_WHMCS_PRIMARY=false
# BILLING_NATIVE_ENABLED=false
# BILLING_NATIVE_PRIMARY=false
# BILLING_BLESTA_ENABLED=false
# BILLING_BLESTA_PRIMARY=false

# --- Native Billing Tuning ---
# BILLING_GRACE_PERIOD_HOURS=12
# BILLING_WARNING_INTERVAL_HOURS=24
# BILLING_NATIVE_AUTO_DELETE_DAYS=0

# --- Stripe (payment gateway) ---
# STRIPE_SECRET_KEY=sk_test_...
# STRIPE_WEBHOOK_SECRET=whsec_...
# STRIPE_PUBLISHABLE_KEY=pk_test_...

# --- PayPal (payment gateway) ---
# PAYPAL_CLIENT_ID=...
# PAYPAL_CLIENT_SECRET=...
# PAYPAL_MODE=sandbox

# --- Cryptocurrency (payment gateway) ---
# CRYPTO_PROVIDER=disabled
# BTCPay Server
# BTCPAY_SERVER_URL=https://btcpay.example.com
# BTCPAY_API_KEY=...
# BTCPAY_STORE_ID=...
# NOWPayments
# NOWPAYMENTS_API_KEY=...
# NOWPAYMENTS_IPN_SECRET=...

# --- OAuth Providers ---
# OAUTH_GOOGLE_ENABLED=false
# OAUTH_GOOGLE_CLIENT_ID=
# OAUTH_GOOGLE_CLIENT_SECRET=
# OAUTH_GITHUB_ENABLED=false
# OAUTH_GITHUB_CLIENT_ID=
# OAUTH_GITHUB_CLIENT_SECRET=
```

**Test:**

```bash
# Verify the file has no syntax errors (it's just comments + assignments)
grep -c "BILLING_WHMCS_ENABLED" .env.example
# Expected: 1
grep -c "STRIPE_SECRET_KEY" .env.example
# Expected: 1
grep -c "OAUTH_GOOGLE_ENABLED" .env.example
# Expected: 1
```

**Commit:**

```
docs: add billing, payment, and OAuth env vars to .env.example

Document all new configuration variables with comments explaining
defaults and requirements.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 9: Update `RegisterCustomerRoutes` Signature for Billing Config

- [ ] Pass billing config to `RegisterCustomerRoutes` for conditional route registration

**File:** `internal/controller/api/customer/routes.go`

### 9a. Add `BillingRoutesConfig` struct

Add at the top of the file, after the permission constants:

```go
// BillingRoutesConfig controls which billing-related route groups are registered.
type BillingRoutesConfig struct {
	NativeBillingEnabled bool
	OAuthGoogleEnabled   bool
	OAuthGitHubEnabled   bool
}
```

### 9b. Update `RegisterCustomerRoutes` signature

Add `billingCfg BillingRoutesConfig` as a new parameter:

```go
func RegisterCustomerRoutes(
	router *gin.RouterGroup,
	handler *CustomerHandler,
	notifyHandler *NotificationsHandler,
	apiKeyRepo *repository.CustomerAPIKeyRepository,
	allowSelfRegistration bool,
	billingCfg BillingRoutesConfig,
) {
```

### 9c. Add conditional billing route stub

At the end of `RegisterCustomerRoutes`, before the closing brace, add:

```go
	// Native billing routes — only registered when native billing is enabled.
	// Actual billing handlers will be added in Phase 3 (Credit Ledger).
	if billingCfg.NativeBillingEnabled {
		// billing := protected.Group("/billing")
		// Routes will be registered here by Phase 3.
		_ = billingCfg // prevent unused warning until Phase 3 adds routes
	}

	// OAuth routes — only registered when respective OAuth provider is enabled.
	// Actual OAuth handlers will be added in Phase 8 (OAuth).
	if billingCfg.OAuthGoogleEnabled {
		// auth.GET("/oauth/google", handler.OAuthGoogleRedirect)
		// auth.GET("/oauth/google/callback", handler.OAuthGoogleCallback)
	}
	if billingCfg.OAuthGitHubEnabled {
		// auth.GET("/oauth/github", handler.OAuthGitHubRedirect)
		// auth.GET("/oauth/github/callback", handler.OAuthGitHubCallback)
	}
```

**Note:** The commented-out route registrations are intentional — they document what Phase 3 and Phase 8 will wire up. The `if` guards are the deliverable of this task: they ensure route groups are conditionally gated by config.

**File:** `internal/controller/server.go`

### 9d. Update the `RegisterAPIRoutes` call to pass billing config

Update the `customer.RegisterCustomerRoutes` call (around line 211) to pass the billing config:

```go
	customer.RegisterCustomerRoutes(
		v1,
		s.customerHandler,
		s.notifyHandler,
		s.customerAPIKeyRepo,
		s.config.AllowSelfRegistration,
		customer.BillingRoutesConfig{
			NativeBillingEnabled: s.config.Billing.Providers.Native.Enabled,
			OAuthGoogleEnabled:   s.config.OAuth.Google.Enabled,
			OAuthGitHubEnabled:   s.config.OAuth.GitHub.Enabled,
		},
	)
```

**Test:**

```bash
make build-controller
# Expected: build succeeds with no errors
```

**Commit:**

```
feat(routes): add conditional billing/OAuth route registration gates

Pass BillingRoutesConfig to RegisterCustomerRoutes. Native billing
routes will only be registered when BILLING_NATIVE_ENABLED=true. OAuth
routes will only be registered when their respective provider is
enabled. Route handlers are stubs for now — Phase 3 and Phase 8 will
add the actual implementations.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 10: Add `billing_provider` to Admin Customer Update

- [ ] Allow admins to update a customer's `billing_provider` via `PUT /admin/customers/:id`

**File:** `internal/controller/api/admin/customers.go`

### 10a. Add `BillingProvider` to `CustomerUpdateRequest`

Update the existing `CustomerUpdateRequest` struct:

```go
type CustomerUpdateRequest struct {
	Name            *string `json:"name,omitempty" validate:"omitempty,max=255"`
	Status          *string `json:"status,omitempty" validate:"omitempty,oneof=active pending_verification suspended"`
	BillingProvider *string `json:"billing_provider,omitempty" validate:"omitempty,oneof=whmcs native blesta unmanaged"`
}
```

### 10b. Add `BillingProvider` field to the Customer model

**File:** `internal/controller/models/customer.go`

Add `BillingProvider` field to the `Customer` struct:

```go
type Customer struct {
	ID                   string   `json:"id" db:"id"`
	Email                string   `json:"email" db:"email"`
	PasswordHash         string   `json:"-" db:"password_hash"`
	Name                 string   `json:"name" db:"name"`
	Phone                *string  `json:"phone,omitempty" db:"phone"`
	WHMCSClientID        *int     `json:"whmcs_client_id,omitempty" db:"whmcs_client_id"`
	BillingProvider      *string  `json:"billing_provider,omitempty" db:"billing_provider"`
	TOTPSecretEncrypted  *string  `json:"-" db:"totp_secret_encrypted"`
	TOTPEnabled          bool     `json:"totp_enabled" db:"totp_enabled"`
	TOTPBackupCodesHash  []string `json:"-" db:"totp_backup_codes_hash"`
	TOTPBackupCodesShown bool     `json:"-" db:"totp_backup_codes_shown"`
	Status               string   `json:"status" db:"status"`
	Timestamps
}
```

### 10c. Add billing_provider handling to `UpdateCustomer` handler

In `internal/controller/api/admin/customers.go`, inside the `UpdateCustomer` handler, add after the name-update block but before the audit log:

```go
	// Apply billing_provider update if specified
	if req.BillingProvider != nil {
		customer.BillingProvider = req.BillingProvider
		actorIP := c.ClientIP()
		actorID := middleware.GetUserID(c)
		if err := h.customerService.Update(c.Request.Context(), actorID, actorIP, customer); err != nil {
			h.logger.Error("failed to update customer billing provider",
				"customer_id", customerID,
				"billing_provider", *req.BillingProvider,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_UPDATE_FAILED", "Internal server error")
			return
		}
	}
```

### 10d. Add database migration for `billing_provider` column

**Run:** `make migrate-create NAME=add_customer_billing_provider`

This creates two files. Edit them:

**File:** `migrations/000072_add_customer_billing_provider.up.sql`

```sql
SET lock_timeout = '5s';

ALTER TABLE customers
    ADD COLUMN billing_provider VARCHAR(20) DEFAULT 'unmanaged'
    CHECK (billing_provider IN ('whmcs', 'native', 'blesta', 'unmanaged'));

-- Backfill: customers with whmcs_client_id set are WHMCS-managed
UPDATE customers SET billing_provider = 'whmcs' WHERE whmcs_client_id IS NOT NULL;

CREATE INDEX idx_customers_billing_provider ON customers(billing_provider);
```

**File:** `migrations/000072_add_customer_billing_provider.down.sql`

```sql
SET lock_timeout = '5s';

DROP INDEX IF EXISTS idx_customers_billing_provider;
ALTER TABLE customers DROP COLUMN IF EXISTS billing_provider;
```

**Note:** The migration number (000072) may differ if other migrations have been added. Use whatever number `make migrate-create` generates.

### 10e. Update customer repository scan functions

The existing repository scan functions for customers (in `internal/controller/repository/customer_repo.go`) need to include `billing_provider` in the column list and scan target. Find the `customerColumns` constant or the scan function and add `billing_provider`. The exact change depends on the repository implementation — the scan function should map `billing_provider` to `customer.BillingProvider`.

**Test:**

```bash
make build-controller
# Expected: build succeeds
go test -race -run TestUpdateCustomer ./internal/controller/api/admin/...
# Expected: PASS (if test exists; otherwise build confirmation is sufficient)
```

**Commit:**

```
feat(admin): allow updating customer billing_provider via admin API

Add billing_provider field to Customer model and CustomerUpdateRequest.
Admins can assign customers to whmcs, native, blesta, or unmanaged
providers. Includes database migration with WHMCS backfill for existing
customers with whmcs_client_id set.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 11: Update Customer Repository for `billing_provider`

- [ ] Update customer repository column list and scan functions to include `billing_provider`

**File:** `internal/controller/repository/customer_repo.go`

### 11a. Add `billing_provider` to customer column list

Find the `customerColumns` constant (or equivalent column list string) and add `billing_provider` to it. For example:

```go
// Before
const customerColumns = "id, email, password_hash, name, phone, whmcs_client_id, totp_secret_encrypted, totp_enabled, totp_backup_codes_hash, totp_backup_codes_shown, status, created_at, updated_at"

// After
const customerColumns = "id, email, password_hash, name, phone, whmcs_client_id, billing_provider, totp_secret_encrypted, totp_enabled, totp_backup_codes_hash, totp_backup_codes_shown, status, created_at, updated_at"
```

### 11b. Update scan function

Find the `scanCustomer` function and add `&c.BillingProvider` to the scan target list in the same position as the column:

```go
// Add &c.BillingProvider after &c.WHMCSClientID in the scan call
```

### 11c. Update the `Update` method

Ensure the `Update` SQL statement includes `billing_provider` in its SET clause:

```go
// Add billing_provider = $N to the UPDATE statement
```

### 11d. Update the `Create` method

Ensure `billing_provider` is included in INSERT statements for creating customers.

**Test:**

```bash
make build-controller && make test
# Expected: build and all unit tests pass
```

**Commit:**

```
feat(repo): include billing_provider in customer CRUD operations

Update customer column list, scan function, and CRUD methods to
handle the new billing_provider column.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 12: Add `BillingProviderEnabled` Helper Method

- [ ] Add convenience methods on `ControllerConfig` for checking billing state

**File:** `internal/shared/config/config.go`

### 12a. Add helper methods

```go
// AnyBillingProviderEnabled returns true if at least one billing provider is enabled.
func (c *ControllerConfig) AnyBillingProviderEnabled() bool {
	return c.Billing.Providers.WHMCS.Enabled ||
		c.Billing.Providers.Native.Enabled ||
		c.Billing.Providers.Blesta.Enabled
}

// PrimaryBillingProvider returns the name of the primary billing provider,
// or an empty string if none is primary.
func (c *ControllerConfig) PrimaryBillingProvider() string {
	if c.Billing.Providers.WHMCS.Primary {
		return "whmcs"
	}
	if c.Billing.Providers.Native.Primary {
		return "native"
	}
	if c.Billing.Providers.Blesta.Primary {
		return "blesta"
	}
	return ""
}

// HasPaymentGateway returns true if at least one payment gateway is configured.
func (c *ControllerConfig) HasPaymentGateway() bool {
	return c.Stripe.SecretKey.Value() != "" ||
		c.PayPal.ClientID.Value() != "" ||
		(c.Crypto.Provider != "" && c.Crypto.Provider != "disabled")
}
```

### 12b. Add tests for helper methods

**File:** `internal/shared/config/config_test.go`

```go
func TestAnyBillingProviderEnabled(t *testing.T) {
	tests := []struct {
		name   string
		modify func(cfg *ControllerConfig)
		want   bool
	}{
		{
			name:   "default — WHMCS enabled",
			modify: func(cfg *ControllerConfig) {},
			want:   true,
		},
		{
			name: "all disabled",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS.Enabled = false
			},
			want: false,
		},
		{
			name: "native only",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS.Enabled = false
				cfg.Billing.Providers.Native.Enabled = true
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			assert.Equal(t, tt.want, cfg.AnyBillingProviderEnabled())
		})
	}
}

func TestPrimaryBillingProvider(t *testing.T) {
	tests := []struct {
		name   string
		modify func(cfg *ControllerConfig)
		want   string
	}{
		{
			name:   "WHMCS primary",
			modify: func(cfg *ControllerConfig) {},
			want:   "whmcs",
		},
		{
			name: "native primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS.Primary = false
				cfg.Billing.Providers.Native = BillingProviderConfig{Enabled: true, Primary: true}
			},
			want: "native",
		},
		{
			name: "none primary",
			modify: func(cfg *ControllerConfig) {
				cfg.Billing.Providers.WHMCS.Primary = false
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			assert.Equal(t, tt.want, cfg.PrimaryBillingProvider())
		})
	}
}

func TestHasPaymentGateway(t *testing.T) {
	tests := []struct {
		name   string
		modify func(cfg *ControllerConfig)
		want   bool
	}{
		{
			name:   "no gateways",
			modify: func(cfg *ControllerConfig) {},
			want:   false,
		},
		{
			name: "stripe configured",
			modify: func(cfg *ControllerConfig) {
				cfg.Stripe.SecretKey = Secret("sk_test_123")
			},
			want: true,
		},
		{
			name: "paypal configured",
			modify: func(cfg *ControllerConfig) {
				cfg.PayPal.ClientID = Secret("client_123")
			},
			want: true,
		},
		{
			name: "crypto btcpay configured",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "btcpay"
			},
			want: true,
		},
		{
			name: "crypto disabled",
			modify: func(cfg *ControllerConfig) {
				cfg.Crypto.Provider = "disabled"
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBillingConfig()
			tt.modify(cfg)
			assert.Equal(t, tt.want, cfg.HasPaymentGateway())
		})
	}
}
```

**Test:**

```bash
go test -race -run "TestAnyBillingProvider|TestPrimaryBilling|TestHasPaymentGateway" ./internal/shared/config/...
# Expected: PASS
```

**Commit:**

```
feat(config): add billing helper methods on ControllerConfig

Add AnyBillingProviderEnabled, PrimaryBillingProvider, and
HasPaymentGateway convenience methods for use in service wiring
and conditional route registration.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 13: Log Billing Config Summary at Startup

- [ ] Add startup log summarizing active billing configuration

**File:** `internal/controller/server.go`

### 13a. Add billing config logging in `RegisterAPIRoutes`

Add after the existing `s.logger.Info("API routes registered")` line (line 227):

```go
	// Log billing configuration summary
	s.logBillingConfig()
```

### 13b. Add `logBillingConfig` method

```go
// logBillingConfig logs a summary of the active billing configuration at startup.
func (s *Server) logBillingConfig() {
	cfg := s.config

	s.logger.Info("billing configuration",
		"whmcs_enabled", cfg.Billing.Providers.WHMCS.Enabled,
		"whmcs_primary", cfg.Billing.Providers.WHMCS.Primary,
		"native_enabled", cfg.Billing.Providers.Native.Enabled,
		"native_primary", cfg.Billing.Providers.Native.Primary,
		"blesta_enabled", cfg.Billing.Providers.Blesta.Enabled,
		"blesta_primary", cfg.Billing.Providers.Blesta.Primary,
		"primary_provider", cfg.PrimaryBillingProvider(),
		"has_payment_gateway", cfg.HasPaymentGateway(),
		"oauth_google", cfg.OAuth.Google.Enabled,
		"oauth_github", cfg.OAuth.GitHub.Enabled,
	)
}
```

**Test:**

```bash
make build-controller
# Expected: build succeeds
```

**Commit:**

```
feat(server): log billing config summary at startup

Log which billing providers are enabled/primary, payment gateway
availability, and OAuth status for operational visibility.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 14: Final Build and Test Verification

- [ ] Verify the full build, tests, and lint pass

### 14a. Build controller

```bash
make build-controller
# Expected: binary produced at bin/controller with exit code 0
```

### 14b. Run unit tests

```bash
make test
# Expected: all tests pass including new billing config and permission tests
```

### 14c. Run unit tests with race detector

```bash
make test-race
# Expected: no race conditions detected
```

### 14d. Run linter (if golangci-lint is installed)

```bash
make lint 2>/dev/null || echo "golangci-lint not installed — skip"
# Expected: no new lint errors introduced
```

### 14e. Verify env var documentation

```bash
grep -c "BILLING_" .env.example && grep -c "STRIPE_" .env.example && grep -c "PAYPAL_" .env.example && grep -c "OAUTH_" .env.example
# Expected: non-zero counts for all four patterns
```

**Commit:**

```
chore: verify Phase 1 billing config infrastructure

All unit tests pass. Build succeeds. No regressions.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Summary of Deliverables

| # | Deliverable | Files Changed |
|---|------------|---------------|
| 1 | Billing provider config structs | `internal/shared/config/config.go` |
| 2 | Payment gateway config structs | `internal/shared/config/config.go` |
| 3 | OAuth config structs | `internal/shared/config/config.go` |
| 4 | Config validation function | `internal/shared/config/config.go` |
| 5 | Config validation tests | `internal/shared/config/config_test.go` |
| 6 | Billing RBAC permissions | `internal/controller/models/permission.go` |
| 7 | Permission tests | `internal/controller/models/permission_test.go` |
| 8 | `.env.example` update | `.env.example` |
| 9 | Conditional route registration | `internal/controller/api/customer/routes.go`, `internal/controller/server.go` |
| 10 | Customer billing_provider update | `internal/controller/api/admin/customers.go`, `internal/controller/models/customer.go`, `migrations/000072_*.sql` |
| 11 | Customer repository update | `internal/controller/repository/customer_repo.go` |
| 12 | Config helper methods + tests | `internal/shared/config/config.go`, `internal/shared/config/config_test.go` |
| 13 | Startup billing logging | `internal/controller/server.go` |
| 14 | Final verification | (no files — build/test confirmation) |

## Environment Variables Introduced

| Variable | Type | Default | Required When |
|----------|------|---------|---------------|
| `BILLING_WHMCS_ENABLED` | bool | `true` | — |
| `BILLING_WHMCS_PRIMARY` | bool | `false` | — |
| `BILLING_NATIVE_ENABLED` | bool | `false` | — |
| `BILLING_NATIVE_PRIMARY` | bool | `false` | — |
| `BILLING_BLESTA_ENABLED` | bool | `false` | — |
| `BILLING_BLESTA_PRIMARY` | bool | `false` | — |
| `BILLING_GRACE_PERIOD_HOURS` | int | `12` | — |
| `BILLING_WARNING_INTERVAL_HOURS` | int | `24` | — |
| `BILLING_NATIVE_AUTO_DELETE_DAYS` | int | `0` | — |
| `STRIPE_SECRET_KEY` | Secret | — | Stripe in use |
| `STRIPE_WEBHOOK_SECRET` | Secret | — | Stripe in use |
| `STRIPE_PUBLISHABLE_KEY` | string | — | Stripe in use |
| `PAYPAL_CLIENT_ID` | Secret | — | PayPal in use |
| `PAYPAL_CLIENT_SECRET` | Secret | — | PayPal in use |
| `PAYPAL_MODE` | string | `sandbox` | — |
| `CRYPTO_PROVIDER` | string | `disabled` | — |
| `BTCPAY_SERVER_URL` | string | — | `CRYPTO_PROVIDER=btcpay` |
| `BTCPAY_API_KEY` | Secret | — | `CRYPTO_PROVIDER=btcpay` |
| `BTCPAY_STORE_ID` | string | — | `CRYPTO_PROVIDER=btcpay` |
| `NOWPAYMENTS_API_KEY` | Secret | — | `CRYPTO_PROVIDER=nowpayments` |
| `NOWPAYMENTS_IPN_SECRET` | Secret | — | `CRYPTO_PROVIDER=nowpayments` |
| `OAUTH_GOOGLE_ENABLED` | bool | `false` | — |
| `OAUTH_GOOGLE_CLIENT_ID` | string | — | `OAUTH_GOOGLE_ENABLED=true` |
| `OAUTH_GOOGLE_CLIENT_SECRET` | Secret | — | `OAUTH_GOOGLE_ENABLED=true` |
| `OAUTH_GITHUB_ENABLED` | bool | `false` | — |
| `OAUTH_GITHUB_CLIENT_ID` | string | — | `OAUTH_GITHUB_ENABLED=true` |
| `OAUTH_GITHUB_CLIENT_SECRET` | Secret | — | `OAUTH_GITHUB_ENABLED=true` |
