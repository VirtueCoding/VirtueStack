# VirtueStack Billing Module — Implementation Plan

## 1. Codebase Findings

### WHMCS Integration (current state)

VirtueStack treats billing as an **external concern**, delegated entirely to WHMCS. The integration is mature (~4,400 lines of PHP) and covers the full VM lifecycle.

**PHP Module** — `modules/servers/virtuestack/`

| File | Lines | Role |
|------|-------|------|
| `virtuestack.php` | 1,185 | Main provisioning module. Implements 16 WHMCS hook functions: `CreateAccount`, `SuspendAccount`, `UnsuspendAccount`, `TerminateAccount`, `ChangePackage`, `ChangePassword`, `ClientArea`, `AdminServicesTabFields`, `TestConnection`, `SingleSignOn`, `UsageUpdate`, plus custom power-operation buttons. |
| `hooks.php` | 1,057 | Registers 11 WHMCS hooks. Key: `Cron` (polls pending provisioning tasks every 5 min as webhook fallback), `AfterModuleCreate`, `ProductConfigurationPage` (plan/template dropdowns), `IntelligentSearchUpdate` (bulk VM status sync). |
| `webhook.php` | 617 | Receives async notifications from the Controller. Handles 14 event types (`vm.created`, `vm.creation_failed`, `vm.deleted`, `vm.suspended`, `vm.unsuspended`, `vm.resized`, `vm.started`, `vm.stopped`, `vm.reinstalled`, `vm.migrated`, `backup.completed`, `backup.failed`, `task.completed`, `task.failed`). Security: HMAC-SHA256 signature verification, 64 KB body limit, event whitelist. |
| `lib/ApiClient.php` | 730 | HTTP client for the Provisioning API. Calls 20 endpoints over HTTPS with `X-API-Key` auth, 30 s timeout, idempotency-key support, and async task polling (3 s interval, 60 max polls). |
| `lib/VirtueStackHelper.php` | 439 | Crypto-safe password generation (Fisher-Yates via `random_int`), AES-256-CBC encryption for stored credentials, log sanitization, SSO URL builder, hostname/UUID validation. |
| `lib/shared_functions.php` | 401 | Custom-field CRUD, webhook signature verification (`hash_equals` timing-safe), field value validation (UUID, IP, status enums). |

**Go Provisioning API** — `internal/controller/api/provisioning/`

The Controller exposes 19 REST endpoints under `/api/v1/provisioning/*` authenticated via `X-API-Key` (SHA-256 hashed, looked up in `provisioning_keys` table). Middleware stack: `APIKeyAuth` → rate limiter → audit logger.

Key billing-relevant endpoints:

| Endpoint | Handler file | Purpose |
|----------|-------------|---------|
| `POST /vms` | `vms.go` | Async VM creation (returns `task_id`, `vm_id`). Accepts `whmcs_service_id` for cross-reference. |
| `DELETE /vms/:id` | `vms.go` | Async VM deletion. |
| `POST /vms/:id/suspend` | `suspend.go` | Suspend VM for non-payment. Stops VM, blocks console. |
| `POST /vms/:id/unsuspend` | `suspend.go` | Re-activate VM after payment. |
| `POST /vms/:id/resize` | `resize.go` | Change plan resources (vCPU, RAM, disk). |
| `GET /vms/:id/usage` | `usage.go` | Returns `bandwidth_used_gb`, `bandwidth_limit_gb`, `disk_used_gb`, `disk_limit_gb` for WHMCS overage billing. |
| `POST /customers` | `customers.go` | Idempotent customer creation (by email). Links `whmcs_client_id`. |
| `POST /sso-tokens` | `sso.go` | Issues 5-minute single-use opaque token. WHMCS redirects browser to `/customer/auth/sso-exchange?token={token}`. |
| `GET /plans`, `GET /plans/:id` | `plans.go` | Plan listing for WHMCS product config dropdowns. |

**SSO flow**: WHMCS calls `POST /provisioning/sso-tokens` → gets opaque token → redirects customer to Controller → Controller hashes token, looks up in `sso_tokens` table, consumes atomically → sets HttpOnly session cookies → redirects to `/vms/{vm_id}`.

**Models**: `SSOToken` (`internal/controller/models/sso_token.go`), `ProvisioningKey` (`internal/controller/models/provisioning_key.go`).
**Repositories**: `sso_token_repo.go`, `provisioning_key_repo.go` in `internal/controller/repository/`.

### User/Auth Model (current state)

**Customer model** — `internal/controller/models/customer.go`

```go
type Customer struct {
    ID                   string    // UUID PK
    Email                string    // unique, max 254
    PasswordHash         string    // argon2id (64 MB, 3 iter, 4 parallelism)
    Name                 string
    Phone                *string
    WHMCSClientID        *int      // nullable FK to WHMCS client — billing link
    TOTPSecretEncrypted  *string   // AES-256-GCM encrypted
    TOTPEnabled          bool
    TOTPBackupCodesHash  []string
    TOTPBackupCodesShown bool
    Status               string    // active | pending_verification | suspended | deleted
    CreatedAt, UpdatedAt time.Time
}
```

Notable: `WHMCSClientID` is the only billing-system field on the customer. No balance, credit, or payment-method fields exist.

**Admin model** — same file, lines 30-44. Roles: `admin`, `super_admin`. 27 granular permissions across 12 resources. Mandatory TOTP.

**Session model** — same file, lines 56-67. Refresh token stored as SHA-256 hash only. Tracks IP, user-agent, `last_reauth_at`. Limits: admin 3 concurrent, customer 10.

**Auth middleware** — `internal/controller/api/middleware/auth.go`

Three auth strategies:
1. **JWT** (`JWTAuth`) — 15-min access token from HttpOnly cookie or `Authorization` header. Claims: `sub` (user UUID), `user_type`, `role`, `purpose`.
2. **Provisioning API Key** (`APIKeyAuth`) — `X-API-Key` header, SHA-256 hashed lookup, optional IP allowlist.
3. **Customer API Key** (`CustomerAPIKeyAuth`) — `X-API-Key` with HMAC-SHA256 hashing, scoped permissions (`vm:read`, `vm:write`, `vm:power`, `backup:read`, `backup:write`, `snapshot:read`, `snapshot:write`), optional VM-ID restrictions.
4. **Combined** (`JWTOrCustomerAPIKeyAuth`) — tries JWT first, falls back to API key.

**Database tables** (from `migrations/000001_initial_schema.up.sql`): `customers`, `admins`, `sessions`, `provisioning_keys`, `customer_api_keys`, `password_resets`.

### Existing Billing Code

There is **no standalone billing module** — no `internal/billing/`, no Stripe/PayPal/crypto integration, no invoice table, no credit/balance system.

What does exist that is billing-adjacent:

| What | Where | Details |
|------|-------|---------|
| **Plan pricing** | `internal/controller/models/plan.go` | `PriceMonthly int64`, `PriceHourly int64` — stored in cents (minor units) to avoid floating-point issues. Non-negative CHECK constraint in DB (`migrations/000027`). |
| **Bandwidth metering** | `internal/controller/models/bandwidth.go`, `repository/bandwidth_repo.go` | Monthly per-VM bandwidth tracking with `LimitBytes`, `ResetMonthCounters()`, `GetMonthlyUsage()`. Used by WHMCS `UsageUpdate` for overage billing. |
| **Suspend/unsuspend** | `internal/controller/api/provisioning/suspend.go` | VM lifecycle hooks triggered by WHMCS on non-payment. |
| **Usage reporting** | `internal/controller/api/provisioning/usage.go` | `VMUsageResponse` with bandwidth and disk metrics consumed by WHMCS for billing calculations. |
| **Plan admin UI** | `webui/admin/components/plans/PlanCreateDialog.tsx`, `PlanEditDialog.tsx`, `PlanList.tsx` | Price display uses `Intl.NumberFormat` with USD currency, cents-to-dollars conversion. |
| **Bandwidth notification** | `internal/controller/notifications/templates/bandwidth-exceeded.html` | Warns customer of throttling "until the next billing cycle". |

No customer-facing billing portal exists in the WebUI. All payment, invoicing, and billing display is handled externally by WHMCS.

### Config Patterns

**Loading** — `internal/shared/config/config.go` (743 lines)

1. Hardcoded defaults (e.g., `ListenAddr: ":8080"`, `LogLevel: "info"`).
2. Optional YAML file (path from `VS_CONFIG_FILE` env var).
3. **Environment variables override YAML** — every config field has an `env:` tag.
4. Validation: required fields (`DATABASE_URL`, `NATS_URL`, `JWT_SECRET`, `ENCRYPTION_KEY`), weak-password rejection.

**Feature flags** — boolean env vars consumed directly:
- `ALLOW_SELF_REGISTRATION` (default `false`) — gates `/customer/auth/register` and `/customer/auth/verify-email` routes.
- `REGISTRATION_EMAIL_VERIFICATION` (default `true`) — requires email verification on self-registration.

There is no feature-flag framework (no LaunchDarkly, no DB-backed toggles). New features are gated by env-var booleans checked at route registration time or in service logic.

**Secrets** — `config.Secret` type wraps sensitive strings. `.String()` returns `"[REDACTED]"`, `.Value()` returns plaintext. Prevents accidental logging.

**No ADR directory** — `docs/decisions/` does not exist (referenced in AGENTS.md but not created yet).

---

## 2. Architecture Decision

### Options Evaluated

**Option A: Separate microservice (own repo/DB)**

- Pros: Full isolation, independent deploy cycle, clean boundary.
- Cons: Doubles operational complexity for self-hosted operators (second binary, second DB, service discovery, network config). VirtueStack is a self-hosted product, not SaaS — every additional service is a support burden. Requires duplicating customer data or building a sync layer.
- Verdict: Over-engineered for a self-hosted product where the operator runs everything on one or two machines.

**Option B: Internal Go module within this repo (`internal/controller/billing/`)**

- Pros: Ships in the same binary — zero additional deployment for operators. Uses the same PostgreSQL database with new tables (leverages existing migration tooling). Shares the existing customer model and auth middleware. Can be feature-flagged off entirely (WHMCS remains default). Clear code boundary via Go package.
- Cons: Couples billing code to controller release cycle (acceptable — they need to stay in sync anyway). Requires discipline to keep package boundaries clean.
- Verdict: Right fit for a self-hosted product.

**Option C: Direct integration into existing controllers/services**

- Pros: Minimal boilerplate.
- Cons: Scatters billing logic across handlers, services, and repositories. Makes it impossible to cleanly disable native billing when WHMCS is in use. Increases cognitive load on every file. Billing is a distinct domain — mixing it into VM lifecycle code violates separation of concerns.
- Verdict: Too messy. Billing deserves its own package.

### Decision: Option B — Internal Go module (`internal/controller/billing/`)

**Reasoning:**

1. **Single-binary deployment.** Self-hosted operators `apt install virtuestack` or `docker compose up`. No second service to configure, monitor, or upgrade.
2. **Shared database.** New `billing_*` tables in the same PostgreSQL instance. Leverages existing migration tooling (`make migrate-create`), connection pooling, and RLS infrastructure. Foreign keys to `customers`, `plans`, and `vms` tables enforce referential integrity without cross-service calls.
3. **WHMCS coexistence.** A boolean env var (e.g., `BILLING_ENABLED=false`) gates route registration — identical pattern to `ALLOW_SELF_REGISTRATION`. When disabled, the Provisioning API continues to serve WHMCS exactly as today. When enabled, native billing routes are registered under `/api/v1/customer/billing/*` and the customer WebUI gains billing pages.
4. **Clean boundary.** `internal/controller/billing/` contains its own models, repository, service, and handlers. It imports from `internal/controller/models` (for `Customer`, `Plan`, `VM`) but nothing imports from it except the server wiring in `server.go`. This is a leaf package.
5. **Database safety.** New tables only — no ALTER on existing tables in the initial phase. Existing WHMCS deployments are unaffected. The `plans` table already has `price_monthly` and `price_hourly` in cents, which native billing can consume directly.

**What the billing module would own:**

| Concern | Table(s) | Notes |
|---------|----------|-------|
| Customer balance | `billing_accounts` | Per-customer prepaid credit balance (cents). |
| Transactions | `billing_transactions` | Deposits, charges, refunds. Immutable ledger. |
| Payment methods | `billing_payment_methods` | Stripe customer ID, PayPal email, crypto address. Encrypted at rest. |
| Invoices | `billing_invoices` + `billing_invoice_items` | Generated monthly or on-demand. Links to `plans` and `vms`. |
| Webhooks (inbound) | — | Stripe/PayPal webhook handlers for payment confirmation. |

**What the billing module would NOT own:**

- VM lifecycle (stays in `services/vm_service.go`).
- Plan definitions (stays in `models/plan.go`, `repository/plan_repo.go`).
- Bandwidth metering (stays in `repository/bandwidth_repo.go`).
- Suspend/unsuspend triggers — billing service calls the existing `VMService.Suspend/Unsuspend` methods, same as the Provisioning API does today.

---

## 3. Research Findings — Billing

### 3.1 Industry Billing Models

**Hourly / credit-based billing** is the dominant model for cloud and VPS providers. Customers pre-load credit (also called "balance" or "wallet"), and the system deducts charges hourly based on resource consumption. This is the model used by DigitalOcean, Vultr, Linode, and Hetzner Cloud. Key characteristics:

- Credit is loaded via one-time payments (not subscriptions).
- An hourly rate is derived from the monthly price (e.g., $5/mo plan → $0.00744/hr, capped at the monthly amount).
- A background job runs hourly (or more frequently) to deduct usage from the balance.
- When balance reaches a configurable threshold, warning emails are sent.
- When balance reaches zero (or a grace period expires), VMs are suspended → then deleted after N days.

**Sources:**
- Stripe Usage-Based Billing overview: https://docs.stripe.com/billing/subscriptions/usage-based
- Stripe Billing Credits (prepaid/promotional): https://docs.stripe.com/billing/subscriptions/usage-based/billing-credits
- Stripe Meter Events for recording usage: https://docs.stripe.com/billing/subscriptions/usage-based/recording-usage

### 3.2 How Competing Panels Handle Billing

| Panel | Billing Approach | Registration |
|-------|-----------------|--------------|
| **Virtualizor** | Integrates with WHMCS, Blesta, HostBill, and custom billing via API. No built-in billing — purely a VM management panel. Relies on external billing for invoicing, payments, and customer lifecycle. | Customers created via external billing (WHMCS/Blesta). No native self-registration. Admin API for user creation. |
| **VirtFusion** | Similar to Virtualizor — designed as a companion to external billing. Ships WHMCS and Blesta modules. No internal payment processing. | External billing creates customers via API. Supports optional self-registration with email verification. |
| **Blesta** | Full billing platform with 30+ payment gateways (Stripe, PayPal, Authorize.net, crypto via Coinbase Commerce/BTCPay). Recurring invoices, automatic suspension/unsuspension, credit management, multi-currency, proration. RESTful API with JSON/XML. Over 40 provisioning modules including VirtFusion, SolusVM, Proxmox. | Native registration with custom fields, client groups, and contact management. No OAuth — email/password only. |
| **HostBill** | Complete billing and automation platform. 500+ integrations. Combines billing, client management, and support. Supports recurring billing, proration, coupons, multi-currency, tax automation. | Native client registration. WHMCS migration tools available. Staff and client portals with role-based access. |

**Sources:**
- Blesta features overview: https://www.blesta.com/features/
- HostBill billing features: https://www.hostbill.com/features/billing.html

### 3.3 Payment Gateway Landscape

**Stripe** is the most widely adopted gateway for SaaS/cloud billing:
- **Payment Intents API** handles complex flows including 3D Secure / SCA compliance.
- **Checkout Sessions** provide a hosted payment page — minimal frontend work, PCI-DSS burden on Stripe.
- **Customer objects** map 1:1 to VirtueStack customers (store `stripe_customer_id` on the billing account).
- **Webhooks** deliver async payment confirmations (`payment_intent.succeeded`, `checkout.session.completed`, `charge.refunded`). Must verify signatures with endpoint secret.
- **Metered billing** via Meters API can record hourly VM usage for pay-as-you-go, but for a credit-based system, one-time Payment Intents (credit top-up) are simpler.

**Sources:**
- Stripe Payment Intents: https://docs.stripe.com/payments/payment-intents
- Stripe Webhooks: https://docs.stripe.com/webhooks
- Stripe Customer API: https://docs.stripe.com/api/customers

**PayPal** is required for markets where Stripe is unavailable:
- **Orders API v2** for one-time payments (credit top-ups).
- **JavaScript SDK** renders payment buttons (PayPal, Venmo, Pay Later, cards).
- **Expanded Checkout** for card processing with PayPal handling PCI compliance.
- Webhooks and IPN (legacy) for async payment confirmation.
- Sandbox environment for testing.

**Sources:**
- PayPal Checkout integration: https://developer.paypal.com/docs/checkout/
- PayPal Orders API v2: https://developer.paypal.com/docs/api/orders/v2/

**Cryptocurrency** — three main approaches:

| Solution | Type | Fees | Chains | Notes |
|----------|------|------|--------|-------|
| **BTCPay Server** | Self-hosted, open-source | 0% | BTC, BTC Lightning, ETH (via plugin), Monero (plugin), Zcash (plugin) | Full control. Greenfield REST API for invoices, webhooks, refunds. Requires self-hosting infrastructure (Docker). No custodial risk. |
| **NOWPayments** | Hosted SaaS | 0.5% | 350+ coins, 75+ fiat settlements | Lowest fees. Auto-conversion to fiat/stablecoins. Mass payouts API. Simple REST API. |
| **CoinGate** | Hosted SaaS | 1% | BTC, ETH, LTC, USDC, USDT, SOL, DOGE, many more | Fiat settlements (EUR/GBP/USD). WalletConnect support. AML/KYC screening built in. 10+ years operating. |
| **Direct RPC** | Self-hosted | 0% | Any chain you run a node for | Maximum control but massive operational burden: run full nodes, handle reorgs, manage hot wallets, implement confirmation tracking. Not recommended as primary path. |

**Sources:**
- BTCPay Server docs: https://docs.btcpayserver.org/
- BTCPay Greenfield API: https://docs.btcpayserver.org/Development/GreenFieldExample/
- NOWPayments: https://nowpayments.io/
- CoinGate accept page: https://coingate.com/accept

### 3.4 Billing Architecture Recommendation

For VirtueStack's credit-based model:

1. **Credit top-up via one-time payments** (not subscriptions). Customer clicks "Add Credit" → selects amount → redirected to Stripe Checkout / PayPal / crypto invoice → webhook confirms payment → credit added to ledger.
2. **Hourly deduction cron** runs every hour, deducts from balance based on active VM plans.
3. **Payment gateway abstraction** — `internal/controller/payments/` package with a `PaymentProvider` interface. Each gateway (Stripe, PayPal, crypto) implements it. The active providers are controlled by feature flags.

---

## 4. Research Findings — Registration & Auth

### 4.1 Self-Registration Best Practices

Modern SaaS registration flows follow this pattern:

1. **Email + password form** with inline validation (password strength meter, email format check).
2. **Email verification** via signed token (JWT or opaque random token) with 24-hour expiry. Rate limit token generation (max 5 per email per hour). Double opt-in: account is `pending_verification` until email link is clicked.
3. **Argon2id password hashing** (VirtueStack already uses this — 64 MB memory, 3 iterations, parallelism 4).
4. **Account activation** on email verification → status transitions from `pending_verification` to `active`.

VirtueStack already has this flow partially implemented:
- `ALLOW_SELF_REGISTRATION=false` gates `/customer/auth/register` and `/customer/auth/verify-email` routes.
- `email_verification_tokens` table exists (migration 000069).
- Customer status includes `pending_verification` state.

### 4.2 OAuth Sign-In

**Google OAuth 2.0** (Web Server flow):
- Register app in Google Cloud Console → get Client ID + Client Secret.
- Authorization Code flow with PKCE: redirect to Google → user consents → callback with code → exchange for access token → fetch user profile (`email`, `name`, `sub` (Google ID)).
- Go implementation: `golang.org/x/oauth2` package with `google.Endpoint` from `golang.org/x/oauth2/google`.
- Scopes needed: `openid`, `email`, `profile`.

**Sources:**
- `golang.org/x/oauth2` package: https://pkg.go.dev/golang.org/x/oauth2
- Google OAuth 2.0 Web Server: https://developers.google.com/identity/protocols/oauth2/web-server

**GitHub OAuth**:
- GitHub recommends **OAuth Apps** (not GitHub Apps) for user sign-in scenarios. GitHub Apps are designed for repository/organization access automation, not user authentication.
- OAuth Apps use the standard Authorization Code flow: redirect to `github.com/login/oauth/authorize` → callback with code → exchange at `github.com/login/oauth/access_token` → fetch user from `api.github.com/user`.
- PKCE is supported and recommended (`code_challenge` + `code_challenge_method=S256`).
- Relevant scopes: `user:email` (to get verified email addresses).

**Sources:**
- GitHub OAuth Apps authorization: https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps
- GitHub Apps vs OAuth Apps: https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/differences-between-github-apps-and-oauth-apps

### 4.3 Account Linking Strategies

The core problem: a user registers with email, then later tries to sign in with Google (or vice versa) using the same email. Common strategies:

| Strategy | How it works | Pros | Cons |
|----------|-------------|------|------|
| **Auto-link by verified email** | If OAuth email matches an existing verified account email, link the OAuth identity to that account automatically. | Seamless UX. User doesn't notice. | Requires the OAuth provider to guarantee the email is verified (Google and GitHub both do). |
| **Prompt to link** | Show "An account with this email exists. Sign in with your password to link your Google account." | Most secure — proves ownership of both identities. | Worse UX — extra step. User may not remember password. |
| **Separate accounts** | Treat OAuth and email accounts as completely separate even if email matches. | Simplest implementation. | Confusing — user has two accounts with same email. |

**Recommended approach for VirtueStack:** **Auto-link by verified email** for Google and GitHub (both providers verify email ownership). The flow:

1. User clicks "Sign in with Google" → OAuth flow completes → VirtueStack receives `email` + `google_id`.
2. Look up `customer_oauth_links` for `provider=google, provider_id=google_id`:
   - Found → sign in as that customer.
   - Not found → look up `customers` by `email`:
     - Found & email verified → create `customer_oauth_links` row, sign in.
     - Found & email NOT verified → reject (must verify email first).
     - Not found → create new customer + `customer_oauth_links` row (if native registration is enabled).

### 4.4 WHMCS Coexistence

VPS panels universally treat WHMCS-created users and native users as the same entity:

- **Virtualizor / VirtFusion:** Customer records are created via the provisioning API when WHMCS provisions a service. If the panel also supports self-registration, the same `customers` table is used. The `whmcs_client_id` field (nullable) distinguishes WHMCS-linked users.
- **Blesta:** Acts as the billing system itself, so all users are "native." But when migrating from WHMCS, users are imported into the same table with metadata preserved.
- **VirtueStack (current):** Already follows this pattern — `customers.whmcs_client_id` is nullable. WHMCS users have it set; native users don't.

**Key rule:** A customer with `whmcs_client_id IS NOT NULL` should NOT be billed by the native billing system — WHMCS owns their billing. The native billing system only manages customers where `whmcs_client_id IS NULL`.

### 4.5 Email Verification Best Practices

Based on industry standards:

- **Token format:** Cryptographically random opaque token (32+ bytes, base64url encoded). Not JWT — opaque tokens can be revoked/consumed atomically in the database.
- **Expiry:** 24 hours (configurable). VirtueStack's `email_verification_tokens` table already supports this.
- **Rate limiting:** Max 5 verification emails per email address per hour. Max 3 per IP per 10 minutes.
- **Double opt-in:** Account stays `pending_verification` until token is consumed. Prevents spam registrations.
- **Token consumption:** Single-use. Delete or mark as consumed atomically in the same transaction that activates the account.

---

## 5. Feature Flag Design

### 5.1 Configuration Format

Feature flags follow VirtueStack's existing pattern: YAML config file with environment variable overrides. Added to `internal/shared/config/config.go`.

### 5.2 Master Toggles

```yaml
# internal/shared/config — Config struct additions
billing:
  provider: "whmcs"                   # "whmcs" | "native" | "disabled"

auth:
  native_registration: false          # Enable email+password self-registration
  oauth:
    google:
      enabled: false
      client_id: ""
      client_secret: ""               # Secret type (redacted in logs)
    github:
      enabled: false
      client_id: ""
      client_secret: ""               # Secret type (redacted in logs)
```

### 5.3 Environment Variable Overrides

```bash
# Billing
BILLING_PROVIDER=whmcs                # "whmcs" | "native" | "disabled"

# Registration (existing)
ALLOW_SELF_REGISTRATION=false         # Already exists — gates /register and /verify-email

# OAuth
OAUTH_GOOGLE_ENABLED=false
OAUTH_GOOGLE_CLIENT_ID=
OAUTH_GOOGLE_CLIENT_SECRET=
OAUTH_GITHUB_ENABLED=false
OAUTH_GITHUB_CLIENT_ID=
OAUTH_GITHUB_CLIENT_SECRET=
```

### 5.4 Flag Behavior Matrix

| Flag | Default | Effect when disabled | Effect when enabled |
|------|---------|---------------------|---------------------|
| `BILLING_PROVIDER=whmcs` | Default | N/A (this IS the default) | Provisioning API serves WHMCS as today. No native billing routes registered. No credit ledger. |
| `BILLING_PROVIDER=native` | — | — | Native billing routes registered at `/api/v1/customer/billing/*`. Credit ledger active. Hourly deduction cron active. Payment webhooks active. WHMCS provisioning API still works (coexistence). |
| `BILLING_PROVIDER=disabled` | — | — | No billing at all. Admin creates VMs manually. For dev/testing. |
| `ALLOW_SELF_REGISTRATION` | `false` | Registration routes not registered. WHMCS creates customers via provisioning API. | `/customer/auth/register` and `/customer/auth/verify-email` routes active. |
| `OAUTH_GOOGLE_ENABLED` | `false` | No Google sign-in routes. | `/customer/auth/oauth/google` and `/customer/auth/oauth/google/callback` routes registered. |
| `OAUTH_GITHUB_ENABLED` | `false` | No GitHub sign-in routes. | `/customer/auth/oauth/github` and `/customer/auth/oauth/github/callback` routes registered. |

### 5.5 Config Location

- **Primary:** YAML file at path specified by `VS_CONFIG_FILE` env var (existing pattern).
- **Override:** Environment variables (existing pattern — env always wins over YAML).
- **No database-backed toggles.** Feature flags are set at deploy time, not runtime. This matches VirtueStack's existing approach and avoids complexity for self-hosted operators.

---

## 6. Native Billing — Credit/Hourly System

### 6.1 Credit Ledger Schema

All amounts stored in **cents** (minor currency units) as `BIGINT` to avoid floating-point issues. Matches existing `plans.price_monthly` and `plans.price_hourly` convention.

```sql
-- billing_accounts: one per customer, tracks current balance
CREATE TABLE billing_accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL UNIQUE REFERENCES customers(id) ON DELETE CASCADE,
    balance_cents   BIGINT NOT NULL DEFAULT 0,        -- current prepaid balance
    currency        VARCHAR(3) NOT NULL DEFAULT 'USD', -- ISO 4217
    stripe_customer_id VARCHAR(255),                   -- Stripe Customer ID (created lazily on first payment)
    auto_suspend    BOOLEAN NOT NULL DEFAULT true,     -- suspend VMs on zero balance
    warning_sent_at TIMESTAMPTZ,                       -- last low-balance warning
    suspended_at    TIMESTAMPTZ,                       -- when auto-suspended
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_accounts_customer ON billing_accounts(customer_id);

-- billing_transactions: immutable ledger of all balance changes
CREATE TABLE billing_transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES billing_accounts(id),
    type            VARCHAR(20) NOT NULL,              -- 'deposit', 'charge', 'refund', 'adjustment'
    amount_cents    BIGINT NOT NULL,                   -- positive = credit added, negative = deducted
    balance_after   BIGINT NOT NULL,                   -- balance snapshot after this transaction
    description     TEXT NOT NULL,                     -- human-readable ("Hourly charge: plan-basic × 1hr")
    reference_type  VARCHAR(30),                       -- 'payment', 'vm_usage', 'admin_adjustment'
    reference_id    UUID,                              -- FK to payment, VM, etc.
    metadata        JSONB,                             -- extra context (payment gateway response, etc.)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_tx_account   ON billing_transactions(account_id);
CREATE INDEX idx_billing_tx_created   ON billing_transactions(created_at);
CREATE INDEX idx_billing_tx_reference ON billing_transactions(reference_type, reference_id);

-- billing_payments: tracks payment gateway interactions
CREATE TABLE billing_payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          UUID NOT NULL REFERENCES billing_accounts(id),
    gateway             VARCHAR(20) NOT NULL,           -- 'stripe', 'paypal', 'crypto'
    gateway_payment_id  VARCHAR(255),                   -- Stripe PaymentIntent ID, PayPal order ID, etc.
    amount_cents        BIGINT NOT NULL,
    currency            VARCHAR(3) NOT NULL DEFAULT 'USD',
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',  -- 'pending', 'completed', 'failed', 'refunded'
    metadata            JSONB,                          -- full gateway response stored encrypted
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_payments_account ON billing_payments(account_id);
CREATE INDEX idx_billing_payments_gateway ON billing_payments(gateway, gateway_payment_id);
```

RLS policies would be added for `billing_accounts`, `billing_transactions`, and `billing_payments` using `current_setting('app.current_customer_id')::UUID`, matching the existing pattern.

### 6.2 Hourly Usage Tracking & Deduction

A background scheduler (registered in `internal/controller/schedulers.go`) runs **every hour**:

1. **Enumerate billable VMs** — only VMs in `running` or `stopped` state where the customer has a `billing_accounts` row AND `customers.whmcs_client_id IS NULL` (skip WHMCS-managed customers). VMs in `provisioning`, `suspended`, `migrating`, `reinstalling`, `error`, or `deleted` states are NOT charged.
2. For each billable VM, calculate the hourly charge from `plans.price_hourly`.
3. **Deduct atomically** in a transaction:
   ```sql
   BEGIN;
   UPDATE billing_accounts SET balance_cents = balance_cents - $1
       WHERE id = $2 AND balance_cents >= $1;
   -- If no rows updated, balance is insufficient → trigger suspension flow
   INSERT INTO billing_transactions (...) VALUES (...);
   COMMIT;
   ```
4. If the deduction fails (insufficient balance), the VM enters the suspension flow.

**Monthly cap:** Hourly charges for a VM in a calendar month never exceed `plans.price_monthly`. The deduction cron tracks monthly accumulation per VM and stops charging once the cap is reached.

### 6.3 Low-Balance Warnings & Auto-Suspension

| Trigger | Action |
|---------|--------|
| Balance drops below 48 hours of projected usage | Send email warning. Set `warning_sent_at`. Rate limit: one warning per 24 hours. |
| Balance reaches zero | Grace period: 12 hours (configurable). Send final warning. |
| Grace period expires with zero balance | Auto-suspend all VMs. Set `suspended_at`. Send suspension notification. |
| 14 days after suspension with no payment | (Future) Auto-delete VMs. Send data-deletion warning 7 days before. |

### 6.4 Credit Top-Up Flow

1. Customer navigates to billing page → clicks "Add Credit."
2. Frontend presents amount selector (preset amounts: $5, $10, $25, $50, $100, or custom).
3. Customer selects payment method (Stripe / PayPal / Crypto).
4. Backend creates payment session:
   - Stripe: `POST /v1/checkout/sessions` with `mode: "payment"`, `metadata: { account_id, amount_cents }`.
   - PayPal: `POST /v2/checkout/orders` with `intent: "CAPTURE"`.
   - Crypto: Create invoice via BTCPay Greenfield API or NOWPayments API.
5. Customer completes payment on gateway.
6. Webhook received → verify signature → update `billing_payments.status` → credit `billing_accounts.balance_cents` → insert `billing_transactions` row.

---

## 7. Payment Gateway Integration

Payment gateway logic lives in `internal/controller/payments/`. Each gateway implements a common interface:

```go
// PaymentProvider abstracts payment gateway operations
type PaymentProvider interface {
    // CreatePaymentSession initiates a payment for a credit top-up
    CreatePaymentSession(ctx context.Context, req *CreatePaymentRequest) (*PaymentSession, error)
    // HandleWebhook processes an inbound webhook from the gateway
    HandleWebhook(ctx context.Context, headers http.Header, body []byte) (*WebhookResult, error)
    // GetPaymentStatus checks the current status of a payment
    GetPaymentStatus(ctx context.Context, gatewayPaymentID string) (*PaymentStatus, error)
    // RefundPayment issues a full or partial refund
    RefundPayment(ctx context.Context, gatewayPaymentID string, amountCents int64) error
}
```

### 7a. Stripe

**Integration approach:** Stripe Checkout (hosted payment page) for credit top-ups.

**Flow:**
1. Backend calls `stripe.CheckoutSessions.New()` with:
   - `mode: "payment"` (one-time, not subscription).
   - `line_items`: single item with description "Credit Top-Up" and the requested amount.
   - `customer`: Stripe Customer ID (created lazily and stored in `billing_payments.gateway_customer_id`).
   - `success_url`: Redirect back to VirtueStack billing page.
   - `cancel_url`: Redirect back to VirtueStack billing page.
   - `metadata`: `{ "account_id": "...", "payment_id": "..." }` for webhook reconciliation.
2. Frontend redirects to Stripe Checkout URL.
3. On completion, Stripe fires `checkout.session.completed` webhook.
4. Webhook handler (registered at `/api/v1/webhooks/stripe`):
   - Verify signature using `stripe.ConstructEvent()` with endpoint secret.
   - Extract `metadata.payment_id`, look up `billing_payments`.
   - Update payment status → credit account → log transaction.

**Stripe Customer mapping:** One Stripe Customer per VirtueStack customer. Created lazily on first payment. `stripe_customer_id` stored on `billing_accounts`.

**Library:** `github.com/stripe/stripe-go/v82` (official Go SDK).

**Sources:**
- Stripe Checkout quickstart: https://docs.stripe.com/checkout/quickstart
- Stripe Payment Intents: https://docs.stripe.com/payments/payment-intents
- Stripe Webhooks: https://docs.stripe.com/webhooks

### 7b. PayPal

**Integration approach:** PayPal Orders API v2 for one-time credit top-ups.

**Flow:**
1. Backend calls `POST /v2/checkout/orders` with `intent: "CAPTURE"`, amount, and return URLs.
2. Frontend redirects to PayPal approval URL (from `links[rel=approve]` in response).
3. Customer approves payment on PayPal.
4. PayPal redirects back to VirtueStack with `token` query parameter.
5. Backend calls `POST /v2/checkout/orders/{id}/capture` to finalize.
6. On success, credit account and log transaction.
7. **Alternatively**, use PayPal webhooks (`PAYMENT.CAPTURE.COMPLETED`) for async confirmation.

**PayPal Sandbox:** Use sandbox credentials for testing. PayPal provides sandbox buyer accounts.

**Library:** No official Go SDK — use direct HTTP client with PayPal REST API v2. Existing SSRF-safe HTTP client pattern (`tasks.DefaultHTTPClient()`) applies.

**Sources:**
- PayPal Checkout: https://developer.paypal.com/docs/checkout/
- PayPal Orders API v2: https://developer.paypal.com/docs/api/orders/v2/

### 7c. Cryptocurrency

**Supported chains (initial scope):**

| Chain | Assets | Rationale |
|-------|--------|-----------|
| Bitcoin mainnet | BTC | Most widely held crypto. Required for hosting providers. |
| Ethereum mainnet | ETH, USDT (ERC-20), USDC (ERC-20) | Largest smart contract platform. Stablecoins for predictable value. |
| BNB Smart Chain | BNB, USDT (BEP-20), USDC (BEP-20) | Low fees. Popular in Asia-Pacific market. |
| Base (Coinbase L2) | USDC (native) | Very low fees. Native USDC support. Growing adoption. |

**Provider comparison:**

| Criteria | BTCPay Server | NOWPayments | CoinGate |
|----------|--------------|-------------|----------|
| **Hosting** | Self-hosted (Docker) | Hosted SaaS | Hosted SaaS |
| **Fees** | 0% | 0.5% | 1% |
| **Chains** | BTC, BTC Lightning, + plugins for ETH, Monero | 350+ coins | 70+ coins |
| **API** | Greenfield REST API (invoices, webhooks, refunds) | REST API (payments, callbacks) | REST API (orders, callbacks) |
| **Fiat settlement** | No (crypto only) | Yes (auto-convert) | Yes (EUR/GBP/USD) |
| **Privacy** | Full (self-hosted, no KYC for operator) | KYC required for operator | KYC required for operator |
| **Stablecoins** | Limited (via plugins) | Full support (USDT, USDC on many chains) | Full support |
| **Operational cost** | High (run Docker stack, manage updates) | Zero (SaaS) | Zero (SaaS) |

**Recommendation:** Support **BTCPay Server** as primary (self-hosted, zero fees, privacy-focused — aligns with VirtueStack's self-hosted ethos) AND **NOWPayments** as alternative (easier setup, broader chain support, minimal fees). Let the operator choose via config:

```yaml
billing:
  crypto:
    provider: "btcpay"           # "btcpay" | "nowpayments" | "disabled"
    btcpay:
      server_url: ""             # BTCPay Server URL
      api_key: ""                # Secret type
      store_id: ""
    nowpayments:
      api_key: ""                # Secret type
      ipn_secret: ""             # Secret type
```

**Wallet generation strategy:**
- **BTCPay Server:** Handles wallet generation internally. Generates new addresses per invoice from the store's HD wallet (xpub). No hot wallet needed on VirtueStack side.
- **NOWPayments:** Handles wallet generation per payment via their API. VirtueStack provides a callback URL.

**Confirmation tracking:**
- Both providers handle confirmation tracking and notify VirtueStack via webhooks/callbacks when payment is confirmed.
- VirtueStack stores the crypto payment as `billing_payments` with `gateway='crypto'` and the provider-specific payment ID.

**Exchange rate handling:**
- Both providers lock the exchange rate at invoice creation time (typically 15-20 minute window).
- The amount credited to the VirtueStack balance is the USD-equivalent at the locked rate, not the fluctuating market rate.

---

## 8. Customer Registration (Native — Disabled by Default)

### 8.1 Registration Form (Email + Password)

When `ALLOW_SELF_REGISTRATION=true`, the following endpoint is active (already exists):

- `POST /api/v1/customer/auth/register` — accepts `email`, `password`, `name`.
- Password requirements: minimum 8 characters, validated via existing `go-playground/validator` tags.
- Response: `201 Created` with `{ status: "pending_verification" }`.
- Email verification token sent immediately.
- Account status: `pending_verification` until email is verified.

### 8.2 Email Verification

Already implemented (migration 000069 `email_verification_tokens`):

- `POST /api/v1/customer/auth/verify-email` — accepts `token`.
- Token: 32 bytes, cryptographically random, base64url-encoded. 24-hour expiry.
- Rate limiting: max 5 verification emails per email per hour (configurable).
- On verification: customer status → `active`, token consumed atomically.

### 8.3 OAuth Sign-In (Google + GitHub)

Both disabled by default (`OAUTH_GOOGLE_ENABLED=false`, `OAUTH_GITHUB_ENABLED=false`).

**New routes (registered conditionally):**

```
GET  /api/v1/customer/auth/oauth/google           → redirect to Google consent screen
GET  /api/v1/customer/auth/oauth/google/callback   → handle OAuth callback
GET  /api/v1/customer/auth/oauth/github            → redirect to GitHub authorization
GET  /api/v1/customer/auth/oauth/github/callback   → handle OAuth callback
```

**Implementation:**
- Use `golang.org/x/oauth2` package with provider-specific endpoints (`google.Endpoint`, custom GitHub endpoint).
- PKCE (S256) for all OAuth flows — `oauth2.GenerateVerifier()` + `oauth2.S256ChallengeOption()`.
- State parameter: cryptographically random, stored in short-lived server-side session (5 minutes TTL), validated on callback.
- On callback: exchange code for access token → fetch user profile → apply account linking logic (section 4.3).

### 8.4 Default Behavior (All Disabled)

When all registration flags are off (the default):

- No `/register`, `/verify-email`, or `/oauth/*` routes are registered.
- WHMCS creates customers via `POST /api/v1/provisioning/customers`.
- SSO tokens provide customer portal access via `POST /api/v1/provisioning/sso-tokens`.
- The customer portal login page shows only email + password fields.
- This is the current behavior — no changes required.

### 8.5 Account Linking Strategy

**Goal:** A customer should have exactly one account regardless of how they sign in.

**New table:**

```sql
CREATE TABLE customer_oauth_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    provider        VARCHAR(20) NOT NULL,       -- 'google', 'github'
    provider_id     VARCHAR(255) NOT NULL,       -- Google 'sub' claim, GitHub user ID
    provider_email  VARCHAR(254) NOT NULL,       -- email from provider (for diagnostics)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_id)
);
CREATE INDEX idx_oauth_links_customer ON customer_oauth_links(customer_id);
```

**Linking flow (from section 4.3):**

1. OAuth callback returns `provider`, `provider_id`, `email`.
2. Look up `customer_oauth_links` by `(provider, provider_id)`:
   - **Found** → sign in as that customer (existing session flow).
   - **Not found** → look up `customers` by `email`:
     - **Email exists, status = active** → auto-link: insert `customer_oauth_links` row. Sign in.
     - **Email exists, status = pending_verification** → reject. "Please verify your email first."
     - **Email exists, status = suspended/deleted** → reject. "Account suspended."
     - **Email not found, `ALLOW_SELF_REGISTRATION=true`** → create new customer (status = `active`, no password set) + `customer_oauth_links` row. Sign in.
     - **Email not found, `ALLOW_SELF_REGISTRATION=false`** → reject. "Self-registration is disabled. Contact your provider."

**Edge cases:**
- A customer can have multiple OAuth links (Google AND GitHub) pointing to the same account.
- A customer created via WHMCS (has `whmcs_client_id`) can still link OAuth if they use the same email.
- OAuth-only customers (no password) can set a password later via the "Set Password" flow in account settings.

### 8.6 WHMCS User Coexistence

WHMCS-created and native-registered customers share the same `customers` table:

| Field | WHMCS customer | Native customer | OAuth-only customer |
|-------|---------------|-----------------|---------------------|
| `email` | Set by WHMCS | Set by user at registration | Set by OAuth provider |
| `password_hash` | Set by WHMCS (via `ChangePassword`) | Set by user at registration | NULL (can set later) |
| `whmcs_client_id` | Set (non-null) | NULL | NULL |
| `status` | Set by WHMCS lifecycle | `pending_verification` → `active` | `active` (email verified by provider) |
| `customer_oauth_links` | None (unless they manually link) | None (unless they link OAuth) | One or more rows |

**Billing ownership rule:**
- `whmcs_client_id IS NOT NULL` → billing managed by WHMCS. Native billing system ignores this customer.
- `whmcs_client_id IS NULL` AND `BILLING_PROVIDER=native` → billing managed by native credit system.
- `whmcs_client_id IS NULL` AND `BILLING_PROVIDER=whmcs` → customer has no billing (admin-managed).

### 8.7 User Model Additions

Additions to `internal/controller/models/customer.go`:

```go
// New fields on Customer (future migration)
type Customer struct {
    // ... existing fields ...
    AuthProvider    *string   // "local", "google", "github" — how the account was created
    PasswordHash    *string   // nullable for OAuth-only accounts (pointer to allow NULL)
}
```

The `customer_oauth_links` table gets its own model and repository in `internal/controller/models/` and `internal/controller/repository/`.
