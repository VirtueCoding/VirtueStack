# VirtueStack Billing Module — Implementation Plan

## 1. Codebase Findings

### WHMCS Integration (current state)

VirtueStack treats billing as an **external concern**, delegated entirely to WHMCS. The integration is mature (~4,400 lines of PHP) and covers the full VM lifecycle.

**PHP Module** — `modules/servers/virtuestack/`

| File | Lines | Role |
|------|-------|------|
| `virtuestack.php` | 1,185 | Main provisioning module. Exposes the WHMCS module entrypoints (`CreateAccount`, `SuspendAccount`, `UnsuspendAccount`, `TerminateAccount`, `ChangePackage`, `ChangePassword`, `ClientArea`, `AdminServicesTabFields`, `TestConnection`, `SingleSignOn`, `UsageUpdate`, custom power-operation buttons) plus helper/validation functions. |
| `hooks.php` | 1,057 | Registers 12 WHMCS hooks. Key: `Cron` (polls pending provisioning tasks every 5 min as webhook fallback), `AfterModuleCreate`, `ProductConfigurationPage` (plan/template/location dropdowns), `IntelligentSearchUpdate` (bulk VM status sync). |
| `webhook.php` | 617 | Receives async notifications from the Controller. Handles 14 event types (`vm.created`, `vm.creation_failed`, `vm.deleted`, `vm.suspended`, `vm.unsuspended`, `vm.resized`, `vm.started`, `vm.stopped`, `vm.reinstalled`, `vm.migrated`, `backup.completed`, `backup.failed`, `task.completed`, `task.failed`). Security: HMAC-SHA256 signature verification, 64 KB body limit, event whitelist. |
| `lib/ApiClient.php` | 730 | HTTP client for the Provisioning/Admin APIs. Calls provisioning endpoints over HTTPS with `X-API-Key` auth, 30 s timeout, idempotency-key support, async task polling (3 s interval, 60 max polls), and helper lookups for templates/locations. |
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

**SSO flow**: WHMCS calls `POST /provisioning/sso-tokens` → gets opaque token → redirects customer to Controller → Controller hashes the presented token for lookup against the stored hash in `sso_tokens`, consumes the token atomically → sets HttpOnly session cookies → redirects to `/vms/{vm_id}`.

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

### Decision: Option B — Internal Go modules (`internal/controller/billing/` + `internal/controller/payments/`)

**Reasoning:**

1. **Single-binary deployment.** Self-hosted operators `apt install virtuestack` or `docker compose up`. No second service to configure, monitor, or upgrade.
2. **Shared database.** New `billing_*` tables in the same PostgreSQL instance. Leverages existing migration tooling (`make migrate-create`), connection pooling, and RLS infrastructure. Foreign keys to `customers`, `plans`, and `vms` tables enforce referential integrity without cross-service calls.
3. **WHMCS coexistence.** Per-provider billing config gates route registration, identical in spirit to `ALLOW_SELF_REGISTRATION` but modeled as enabled providers plus a single primary owner for new native users. WHMCS-owned customers continue using the Provisioning API exactly as today, while native billing routes are only registered when the native provider is enabled.
4. **Two-package architecture.** `internal/controller/billing/` contains the provider abstraction (interface, registry, WHMCS/native/Blesta adapters) and credit ledger logic. `internal/controller/payments/` contains payment gateway integrations (Stripe, PayPal, crypto). Billing consumes payment confirmations; payments know nothing about billing state. Both are leaf packages imported only by `server.go` wiring.
5. **Database safety.** New tables only — no ALTER on existing columns in the initial phase. One additive column (`billing_provider` on `customers`). Existing WHMCS deployments are unaffected. The `plans` table already has `price_monthly` and `price_hourly` in cents, which native billing can consume directly.

**What the billing module would own:**

| Concern | Table(s) | Package | Notes |
|---------|----------|---------|-------|
| Provider abstraction | — | `billing/` | `BillingProvider` interface + registry. See section 9. |
| WHMCS adapter | — | `billing/whmcs/` | No-op adapter (WHMCS drives via Provisioning API). |
| Customer balance | `billing_accounts` | `billing/native/` | Per-customer prepaid credit balance (cents). |
| Transactions | `billing_transactions` | `billing/native/` | Deposits, charges, refunds. Immutable ledger. |
| Payments | `billing_payments` | `billing/native/` | Tracks payment gateway interactions. |
| Stripe gateway | — | `payments/stripe/` | Stripe Checkout + webhooks. See section 7a. |
| PayPal gateway | — | `payments/paypal/` | PayPal Orders API v2. See section 7b. |
| Crypto gateway | — | `payments/crypto/` | BTCPay / NOWPayments. See section 7c. |

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
  providers:
    whmcs:
      enabled: true
      primary: false
    native:
      enabled: false   # enables native billing routes/ledger/payment processing
      primary: false   # default owner for newly created non-WHMCS users
    blesta:
      enabled: false
      primary: false

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
BILLING_WHMCS_ENABLED=true
BILLING_WHMCS_PRIMARY=false
BILLING_NATIVE_ENABLED=false
BILLING_NATIVE_PRIMARY=false
BILLING_BLESTA_ENABLED=false
BILLING_BLESTA_PRIMARY=false

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
| `BILLING_WHMCS_ENABLED` | `true` | Prevents WHMCS from being selected as an active billing owner for new or re-evaluated customers inside the controller-side billing registry. It does **not** remove the existing provisioning API surface in the same release. | WHMCS ownership is available for customers whose `billing_provider='whmcs'`. |
| `BILLING_NATIVE_ENABLED` | `false` | Native billing routes, ledger, and payment processing stay off. | Native billing routes registered at `/api/v1/customer/billing/*`. Credit ledger active. Hourly deduction cron active. Payment webhooks active. |
| `BILLING_BLESTA_ENABLED` | `false` | Blesta support unavailable. | Blesta ownership available once an adapter exists. |
| `*_PRIMARY` | all `false` by default | Provider cannot be selected as default for newly created native users. | Exactly one enabled provider must be marked primary. Config validation at startup rejects zero or multiple primary providers. |
| `ALLOW_SELF_REGISTRATION` | `false` | Registration routes not registered. WHMCS creates customers via provisioning API. | `/customer/auth/register` and `/customer/auth/verify-email` routes active. |
| `OAUTH_GOOGLE_ENABLED` | `false` | No Google sign-in routes. | `/customer/auth/oauth/google` and `/customer/auth/oauth/google/callback` routes registered. |
| `OAUTH_GITHUB_ENABLED` | `false` | No GitHub sign-in routes. | `/customer/auth/oauth/github` and `/customer/auth/oauth/github/callback` routes registered. |

### 5.5 Config Location

- **Primary:** YAML file at path specified by `VS_CONFIG_FILE` env var (existing pattern).
- **Override:** Environment variables (existing pattern — env always wins over YAML).
- **No database-backed toggles.** Feature flags are set at deploy time, not runtime. This matches VirtueStack's existing approach and avoids complexity for self-hosted operators.
- **Operational note:** changing billing-provider flags requires a process restart (and, in HA deployments, a coordinated rollout) because config is loaded at startup.

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
    idempotency_key VARCHAR(255),                      -- unique key for webhook retries / reconciliation
    metadata        JSONB,                             -- extra context (payment gateway response, etc.)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_tx_account   ON billing_transactions(account_id);
CREATE INDEX idx_billing_tx_created   ON billing_transactions(created_at);
CREATE INDEX idx_billing_tx_reference ON billing_transactions(reference_type, reference_id);
CREATE UNIQUE INDEX idx_billing_tx_idempotency ON billing_transactions(idempotency_key) WHERE idempotency_key IS NOT NULL;

-- billing_payments: tracks payment gateway interactions
CREATE TABLE billing_payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          UUID NOT NULL REFERENCES billing_accounts(id),
    gateway             VARCHAR(20) NOT NULL,           -- 'stripe', 'paypal', 'crypto'
    gateway_payment_id  VARCHAR(255),                   -- Stripe PaymentIntent ID, PayPal order ID, etc.
    reuse_key           VARCHAR(255),                   -- deduplicates concurrent pending session creation
    amount_cents        BIGINT NOT NULL,
    currency            VARCHAR(3) NOT NULL DEFAULT 'USD',
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',  -- 'pending', 'completed', 'failed', 'refunded'
    metadata            JSONB,                          -- full gateway response stored encrypted
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_payments_account ON billing_payments(account_id);
CREATE UNIQUE INDEX idx_billing_payments_gateway ON billing_payments(gateway, gateway_payment_id) WHERE gateway_payment_id IS NOT NULL;
CREATE UNIQUE INDEX idx_billing_payments_reuse_key ON billing_payments(reuse_key) WHERE status = 'pending' AND reuse_key IS NOT NULL;
```

RLS policies would be added for `billing_accounts`, `billing_transactions`, and `billing_payments` using `current_setting('app.current_customer_id')::UUID`, matching the existing pattern.

**Concurrency / double-spend controls:**
- Balance mutation must lock the account row first (`SELECT ... FOR UPDATE`) before updating `balance_cents` and inserting the ledger row.
- Every inbound payment webhook must map to a deterministic `idempotency_key` (for example `stripe:evt_x`, `paypal:transmission_id`, `btcpay:delivery_id`, `nowpayments:payment_id`) so retries cannot credit twice.
- Idempotency keys are namespaced by source to avoid collisions:
  - Stripe webhook: `stripe:event:{event_id}`
  - PayPal webhook: `paypal:transmission:{transmission_id}`
  - BTCPay webhook: `btcpay:delivery:{delivery_id}`
  - NOWPayments callback: `nowpayments:payment:{payment_id}`
  - Admin/manual adjustment: `admin-adjustment:{request_uuid}`
- `balance_after` is derived inside the same database transaction that inserts the ledger row; it must never be computed in application memory before locking.
- Admin adjustments and refunds use the same ledger mutation path as automated charges and deposits.
- Pending-payment reuse uses a deterministic `reuse_key`, for example `{account_id}:{gateway}:{amount_cents}:{currency}:{15-minute-window-bucket}`. The repository must create/reuse pending payments inside a transaction so concurrent requests converge on the same row.

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

**Downtime / reconciliation requirements:**
- The scheduler cannot simply “skip” missed periods during controller downtime. Each run must reconcile from the last successfully billed hour to the current hour and create one ledger entry per missing hour bucket (or per contiguous reconciliation batch with explicit bucket metadata).
- The implementation must persist a durable per-VM billing checkpoint (for example a `billing_vm_checkpoints` table or an equivalent unique `(vm_id, charge_hour)` record) so the controller can deterministically resume after restart.
- In HA mode, only one controller instance may execute the billing scheduler for a given hour bucket. Use a durable leader/lease mechanism or a unique `(vm_id, charge_hour)` constraint to make duplicate execution harmless.
- If the system cannot determine whether a past hour was already billed, it must fail closed (no charge) and emit an operator alert rather than risk double-billing.

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
   - Stripe: `POST /v1/checkout/sessions` with `mode: "payment"`, `metadata: { account_id, payment_id }`.
   - PayPal: `POST /v2/checkout/orders` with `intent: "CAPTURE"` and a server-generated internal payment record.
   - Crypto: Create invoice via BTCPay Greenfield API or NOWPayments API and persist the provider invoice/payment ID before redirecting the user.
5. Customer completes payment on gateway.
6. Webhook received → verify signature → look up the internal payment record → apply idempotent status transition → credit `billing_accounts.balance_cents` → insert `billing_transactions` row with an `idempotency_key`.

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

The payment layer also owns a small registry/dispatcher:

```go
type PaymentRegistry interface {
    ForProvider(name string) (PaymentProvider, error)
}
```

### 7a. Stripe

**Integration approach:** Stripe Checkout (hosted payment page) for credit top-ups.

**Flow:**
1. Backend calls `stripe.CheckoutSessions.New()` with:
   - `mode: "payment"` (one-time, not subscription).
   - `line_items`: single item with description "Credit Top-Up" and the requested amount.
   - `customer`: Stripe Customer ID (created lazily and stored in `billing_accounts.stripe_customer_id`).
   - `success_url`: Redirect back to VirtueStack billing page.
   - `cancel_url`: Redirect back to VirtueStack billing page.
   - `metadata`: `{ "account_id": "...", "payment_id": "..." }` for webhook reconciliation.
2. Frontend redirects to Stripe Checkout URL.
3. On completion, Stripe fires `checkout.session.completed` webhook.
4. Webhook handler (registered at `/api/v1/payments/webhooks/stripe`):
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
7. **Additionally**, use PayPal webhooks (`PAYMENT.CAPTURE.COMPLETED`) as the source of truth for idempotent async confirmation; browser return alone is not sufficient for credit posting.

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
- Final credit posting must wait for the provider's “confirmed/final” state rather than the first seen invoice payment event.

**Exchange rate handling:**
- Both providers lock the exchange rate at invoice creation time (typically 15-20 minute window).
- The amount credited to the VirtueStack balance is the USD-equivalent at the locked rate, not the fluctuating market rate.
- Underpayments do not create partial credit automatically. They move the payment into a provider-specific `underpaid` / `manual_review` state until the operator resolves it.
- Chain reorg handling is delegated to the provider, but VirtueStack must treat any later reorg/reversal callback as a compensating ledger event rather than silently ignoring it.

---

## 8. Customer Registration (Native — Disabled by Default)

### 8.1 Registration Form (Email + Password)

When `ALLOW_SELF_REGISTRATION=true`, the following endpoint is active (already exists):

- `POST /api/v1/customer/auth/register` — accepts `email`, `password`, `name`.
- Password requirements: minimum 12 characters, validated via existing `go-playground/validator` tags.
- Response: `201 Created` with `{ id, email, name, requires_verification }`.
- Email verification token sent immediately.
- Account status: `pending_verification` until email is verified.

### 8.2 Email Verification

Already implemented (migration 000069 `email_verification_tokens`):

- `POST /api/v1/customer/auth/verify-email` — accepts `token` in the request body.
- Token: 32 bytes, cryptographically random, base64url-encoded. 24-hour expiry.
- Existing route protection currently uses the registration rate-limit middleware; any stronger verify-email-specific throttling is future hardening work, not current behavior.
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
      - **Email exists, status = active, `whmcs_client_id IS NULL`** → auto-link: insert `customer_oauth_links` row. Sign in.
      - **Email exists, status = active, `whmcs_client_id IS NOT NULL`** → DO NOT auto-link. Require the customer to authenticate into the existing portal account first (password or SSO) and perform an explicit “Link OAuth Provider” action from account settings via a JWT-protected link flow.
      - **Email exists, status = pending_verification** → reject. "Please verify your email first."
      - **Email exists, status = suspended/deleted** → reject. "Account suspended."
      - **Email not found, `ALLOW_SELF_REGISTRATION=true`** → create new customer (status = `active`, no password set) + `customer_oauth_links` row. Sign in.
      - **Email not found, `ALLOW_SELF_REGISTRATION=false`** → reject. "Self-registration is disabled. Contact your provider."

**Edge cases:**
- A customer can have multiple OAuth links (Google AND GitHub) pointing to the same account.
- A customer created via WHMCS (has `whmcs_client_id`) can only link OAuth from an already-authenticated session; email match alone is not enough.
- OAuth-only customers (no password) can set a password later via the "Set Password" flow in account settings.

**Explicit JWT-protected link flow (required for WHMCS-linked accounts):**
1. Authenticated customer visits account settings and clicks “Link Google” or “Link GitHub.”
2. Backend issues a short-lived signed link-state tied to the current authenticated customer ID and provider, then redirects to the OAuth provider.
3. Callback validates both the OAuth provider state and the signed link-state, and also verifies the user still has a valid authenticated session.
4. If the OAuth provider email does not match the authenticated customer email, linking is rejected unless the operator has explicitly enabled a manual exception workflow.
5. On success, create `customer_oauth_links` and return the customer to settings with a success message; do not disturb the existing session.

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
- `whmcs_client_id IS NULL` AND `billing_provider = 'native'` → billing managed by native credit system.
- `whmcs_client_id IS NULL` AND `billing_provider = 'unmanaged'` → customer is a legacy/manual account that must be explicitly assigned before native billing is enabled.

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

---

## 9. Billing Provider Abstraction Layer

VirtueStack must support multiple billing sources running simultaneously or individually. The current WHMCS integration is tightly coupled to the Provisioning API handlers — it needs to be refactored behind a common interface so that adding new providers (native billing, Blesta, or any future provider) does not require touching core controller logic.

### 9a. WHMCS Integration Code Audit

**Current architecture:** WHMCS communicates with VirtueStack via two channels:
1. **Outbound (WHMCS → VirtueStack):** The PHP module (`modules/servers/virtuestack/`) calls the Provisioning REST API at `/api/v1/provisioning/*` using `ApiClient.php`. All calls use `X-API-Key` auth. The PHP side is entirely external — it runs inside the WHMCS installation, not inside VirtueStack.
2. **Inbound (VirtueStack → WHMCS):** The Controller fires system webhooks (via `internal/controller/services/system_event_service.go` and `tasks/webhook_deliver.go`) to `webhook.php` on the WHMCS side, delivering VM lifecycle events (`vm.created`, `vm.suspended`, etc.).

**Files with WHMCS-specific logic on the Go side:**

| File | WHMCS-Specific Code | What It Does |
|------|---------------------|-------------|
| `internal/controller/api/provisioning/handler.go` | `ProvisioningHandler` struct, `ProvisioningCreateVMRequest.WHMCSServiceID` | Handler struct with multiple service/repo dependencies. Request types include WHMCS-specific fields. |
| `internal/controller/api/provisioning/routes.go` | `RegisterProvisioningRoutes()`, `APIKeyValidatorFunc()` | Registers all 19 provisioning endpoints. API key auth middleware. |
| `internal/controller/api/provisioning/customers.go` | `CreateOrGetCustomer()`, `updateWHMCSClientID()` | Idempotent customer creation with `whmcs_client_id` linking. |
| `internal/controller/api/provisioning/vms.go` | `CreateVM()`, `DeleteVM()`, `ProvisioningCreateVMRequest.WHMCSServiceID` | VM lifecycle operations. `WHMCSServiceID` attached to create request. |
| `internal/controller/api/provisioning/suspend.go` | `SuspendVM()`, `UnsuspendVM()` | Billing suspension/unsuspension. Called by WHMCS on non-payment. |
| `internal/controller/api/provisioning/resize.go` | `ResizeVM()` — requires `plan_id` for billing integrity | Plan-validated resize. Comment: "WHMCS is responsible for price-to-plan matching." |
| `internal/controller/api/provisioning/usage.go` | `GetVMUsage()`, `VMUsageResponse` | Bandwidth/disk usage for WHMCS `UsageUpdate()`. |
| `internal/controller/api/provisioning/sso.go` | `CreateSSOToken()` | Issues opaque tokens for WHMCS browser SSO. |
| `internal/controller/api/provisioning/password.go` | `SetPassword()`, `ResetPassword()` | Password operations triggered by WHMCS `ChangePassword`. |
| `internal/controller/api/provisioning/status.go` | `GetStatus()`, `GetVMInfo()`, `GetVMByWHMCSServiceID()` | VM status lookup. `GetByWHMCSServiceID` is WHMCS-specific. |
| `internal/controller/api/provisioning/plans.go` | `ListPlans()`, `GetPlan()` | Plan listing for WHMCS product config dropdowns. |
| `internal/controller/api/provisioning/tasks.go` | `GetTask()` | Task status polling (WHMCS cron fallback). |
| `internal/controller/api/customer/sso.go` | `ExchangeSSOToken()` | SSO token consumption on the customer portal side. |
| `internal/controller/models/customer.go` | `WHMCSClientID *int` field | Nullable FK — links VirtueStack customer to WHMCS client. |
| `internal/controller/models/vm.go` | `WHMCSServiceID *int` field | Nullable FK — links VirtueStack VM to WHMCS service. |
| `internal/controller/models/sso_token.go` | `SSOToken` model, `SSOTokenTTL` | SSO token model with 5-min TTL. |
| `internal/controller/repository/customer_repo.go` | `UpdateWHMCSClientID()` | Updates `whmcs_client_id` on existing customer. |
| `internal/controller/repository/vm_repo.go` | `GetByWHMCSServiceID()` | Looks up VM by WHMCS service ID. |
| `internal/controller/server.go` | Wires `provisioningHandler`, calls `RegisterProvisioningRoutes()` | Route registration in server startup. |

**Key finding:** The Go side has **no outbound calls to WHMCS**. Communication is strictly:
- WHMCS PHP module → VirtueStack Provisioning API (WHMCS initiates)
- VirtueStack → WHMCS `webhook.php` (generic system webhooks, not WHMCS-specific on the Go side)

This means the Go-side "WHMCS adapter" is essentially the **Provisioning API itself** — it doesn't need to make API calls back to WHMCS. The WHMCS PHP module is the client, not the server.

**Operations that are WHMCS-specific:**
1. **User provisioning:** `POST /provisioning/customers` — creates customer with `whmcs_client_id`.
2. **VM create:** `POST /provisioning/vms` — includes `whmcs_service_id`.
3. **VM suspend/unsuspend:** `POST /provisioning/vms/:id/suspend|unsuspend` — triggered by billing.
4. **VM terminate:** `DELETE /provisioning/vms/:id` — triggered by WHMCS cancellation.
5. **VM resize:** `POST /provisioning/vms/:id/resize` — triggered by WHMCS upgrade.
6. **Usage reporting:** `GET /provisioning/vms/:id/usage` — consumed by WHMCS for billing calcs.
7. **SSO:** `POST /provisioning/sso-tokens` + `GET /customer/auth/sso-exchange` — WHMCS → portal login.
8. **Password operations:** `POST /provisioning/vms/:id/password` and `/password/reset`.
9. **Status/info queries:** `GET /provisioning/vms/:id`, `/vms/:id/status`, `/vms/by-service/:service_id`.
10. **Plan listing:** `GET /provisioning/plans`, `/plans/:id` — for WHMCS config dropdowns.

**Data flows:** API-only. No shared database. WHMCS is an external HTTP client.

### 9b. BillingProvider Interface

Based on the actual WHMCS integration code and the native billing requirements from sections 6-7, the `BillingProvider` interface abstracts these operations:

```go
package billing

import "context"

// BillingProvider abstracts billing system operations.
// Implementations: WHMCS adapter (wraps existing Provisioning API behavior),
// native billing (credit ledger + payment gateways), Blesta (future stub).
type BillingProvider interface {
    // Name returns the provider identifier ("whmcs", "native", "blesta").
    Name() string

    // --- User Lifecycle ---

    // CreateUser provisions a user in the billing system.
    // WHMCS: no-op (WHMCS calls us, not the other way).
    // Native: creates billing_accounts row.
    CreateUser(ctx context.Context, req CreateUserRequest) (*UserResult, error)

    // GetUserBillingStatus returns whether the user is in good standing.
    // WHMCS: always returns "active" (WHMCS handles billing state).
    // Native: checks credit balance, returns "active", "warning", or "suspended".
    GetUserBillingStatus(ctx context.Context, customerID string) (*BillingStatus, error)

    // --- VM Lifecycle Hooks ---
    // Called by core controller when VM events occur.

    // OnVMCreated notifies the billing system that a VM has been provisioned.
    // WHMCS: no-op (WHMCS already knows — it triggered the create via provisioning API).
    // Native: starts hourly charge tracking for this VM.
    OnVMCreated(ctx context.Context, vm VMRef) error

    // OnVMDeleted notifies the billing system that a VM has been terminated.
    // WHMCS: no-op.
    // Native: stops hourly charge tracking, applies final pro-rata charge.
    OnVMDeleted(ctx context.Context, vm VMRef) error

    // OnVMResized notifies the billing system that a VM's plan changed.
    // WHMCS: no-op.
    // Native: updates hourly rate to match new plan.
    OnVMResized(ctx context.Context, vm VMRef, oldPlanID, newPlanID string) error

    // SuspendForNonPayment suspends a user's VMs due to billing.
    // WHMCS: called via provisioning API (suspend.go) — already implemented.
    // Native: called by the hourly deduction cron when balance hits zero.
    SuspendForNonPayment(ctx context.Context, customerID string) error

    // UnsuspendAfterPayment lifts billing suspension.
    // WHMCS: called via provisioning API (suspend.go) — already implemented.
    // Native: called after successful credit top-up.
    UnsuspendAfterPayment(ctx context.Context, customerID string) error

    // --- Balance & Payments (native billing only) ---
    // WHMCS adapter returns ErrNotSupported for these.

    // GetBalance returns the customer's current credit balance.
    GetBalance(ctx context.Context, customerID string) (*Balance, error)

    // ProcessTopUp credits the customer's account after a payment is confirmed.
    ProcessTopUp(ctx context.Context, req TopUpRequest) (*TopUpResult, error)

    // GetUsageHistory returns the hourly charge history for a customer.
    GetUsageHistory(ctx context.Context, customerID string, opts PaginationOpts) (*UsageHistory, error)

    // --- Configuration ---

    // ValidateConfig checks that the provider's configuration is complete and valid.
    ValidateConfig() error
}
```

**Supporting types:**

```go
// VMRef is a lightweight reference to a VM, used in billing hooks.
// Avoids importing the full models.VM to keep the billing package independent.
type VMRef struct {
    ID         string
    CustomerID string
    PlanID     string
    Hostname   string
}

// CreateUserRequest carries the data needed to provision a user in the billing system.
type CreateUserRequest struct {
    CustomerID string
    Email      string
    Name       string
}

// UserResult is returned after user provisioning in the billing system.
type UserResult struct {
    ExternalID string // billing-system-specific user ID (empty for native)
}

// BillingStatus represents a user's billing health.
type BillingStatus struct {
    Status       string // "active", "warning", "suspended"
    BalanceCents *int64 // nil for WHMCS (balance not tracked locally)
    Message      string // human-readable status message
}

// Balance represents a customer's credit balance.
type Balance struct {
    BalanceCents int64
    Currency     string
}

// TopUpRequest carries payment-confirmed data for crediting an account.
type TopUpRequest struct {
    CustomerID   string
    AmountCents  int64
    Currency     string
    Gateway      string // "stripe", "paypal", "crypto"
    GatewayTxID  string // gateway-specific transaction ID
    Description  string
}

// TopUpResult is returned after a successful top-up.
type TopUpResult struct {
    TransactionID string
    NewBalance    int64
}
```

**Note:** The `BillingProvider` interface does NOT include payment gateway webhook dispatch or payment-session orchestration. Those belong to the separate `PaymentProvider` interface in `internal/controller/payments/` (section 7). The billing provider consumes confirmed top-ups via `ProcessTopUp()` after the payment layer has already validated and normalized the event.

### 9c. Provider Registry & Multi-Provider Support

The billing system supports multiple providers active simultaneously. This is critical for migration scenarios where existing WHMCS users coexist with new native-billing users.

**Registry design:**

```go
package billing

import "fmt"

// Registry holds all enabled billing providers and routes operations
// to the correct provider based on the customer's billing_provider field.
type Registry struct {
    providers map[string]BillingProvider // keyed by provider name
    primary   string                     // default provider for new users
}

// NewRegistry creates a Registry from config. Validates all enabled providers.
func NewRegistry(cfg BillingConfig) (*Registry, error) {
    r := &Registry{providers: make(map[string]BillingProvider)}
    // Load enabled providers from config
    // Register each, call ValidateConfig()
    return r, nil
}

// ForCustomer returns the billing provider that manages a specific customer.
// Looks up the customer's billing_provider field in the database.
func (r *Registry) ForCustomer(providerName string) (BillingProvider, error) {
    p, ok := r.providers[providerName]
    if !ok {
        return nil, fmt.Errorf("billing provider %q not registered", providerName)
    }
    return p, nil
}

// Primary returns the default provider for new user registration.
func (r *Registry) Primary() BillingProvider {
    return r.providers[r.primary]
}

// All returns all enabled providers (for startup validation, health checks).
func (r *Registry) All() []BillingProvider { /* ... */ }
```

**Multi-provider rules:**
- Each customer record has a `billing_provider` column, but existing rows must be backfilled from current state rather than globally defaulted to `"whmcs"`.
- When a new customer is created via the Provisioning API (WHMCS), `billing_provider` is set to `"whmcs"`.
- When a new customer self-registers (native registration), `billing_provider` is set to the config's `primary` provider (typically `"native"`).
- Controller/handler code calls `registry.ForCustomer(customer.BillingProvider)`, never a specific provider directly.
- Multiple providers CAN be active simultaneously. Example: WHMCS for legacy users + native for new users.
- `unmanaged` is a temporary migration state for legacy/manual customers with no verified external billing owner yet. The registry must reject native billing actions for `unmanaged` rows until the operator explicitly assigns ownership.

**Config:**

```yaml
billing:
  providers:
    whmcs:
      enabled: true
      primary: false   # WHMCS-created users are assigned explicitly by the provisioning path
    native:
      enabled: false
      primary: false   # set to true when migrating away from WHMCS
    blesta:
      enabled: false   # future — stub only
      primary: false
```

Corresponding env vars:

```bash
BILLING_WHMCS_ENABLED=true
BILLING_WHMCS_PRIMARY=true
BILLING_NATIVE_ENABLED=false
BILLING_NATIVE_PRIMARY=false
BILLING_BLESTA_ENABLED=false
BILLING_BLESTA_PRIMARY=false
```

### 9d. Refactoring Plan for Existing WHMCS Code

**Key insight:** The Go-side WHMCS integration is already cleanly isolated in the `internal/controller/api/provisioning/` package. The WHMCS PHP module calls these REST endpoints — there is no WHMCS-specific logic scattered across core services. The refactoring is therefore **minimal** — mostly wrapping existing handlers behind the abstraction.

**What moves vs. what stays:**

| Current Location | After Refactor | Rationale |
|-----------------|---------------|-----------|
| `api/provisioning/handler.go` | **Stays** (minor changes) | The Provisioning API remains as-is — it's the WHMCS adapter's external interface. |
| `api/provisioning/routes.go` | **Stays** | Route registration unchanged. |
| `api/provisioning/customers.go` | **Stays** | `CreateOrGetCustomer` explicitly assigns `billing_provider="whmcs"` for WHMCS-created users. It must NOT call the registry's primary provider for this path. |
| `api/provisioning/suspend.go` | **Stays** | Suspend/unsuspend logic is provider-agnostic (operates on VM state). The native billing cron calls the same `VMService.Suspend/Unsuspend` methods. |
| `api/provisioning/vms.go` | **Stays** | VM create/delete unchanged. After create, fires `provider.OnVMCreated()` hook. |
| `api/provisioning/usage.go` | **Stays** | Usage reporting is consumed by WHMCS but is provider-agnostic data. |
| `api/provisioning/sso.go` | **Stays** | SSO is an auth concern, not billing. Works with any provider. |
| `models/customer.go` | **Add** `BillingProvider string` field | New column, defaults to `"whmcs"` for existing records. |
| `repository/customer_repo.go` | **Add** `UpdateBillingProvider()` | Minor addition. |

**New packages:**

```
internal/controller/billing/
  provider.go              # BillingProvider interface + types (section 9b)
  registry.go              # Provider registry (section 9c)
  whmcs/
    adapter.go             # Implements BillingProvider — mostly no-ops since
                           # WHMCS initiates via Provisioning API
  native/
    adapter.go             # Implements BillingProvider for native billing
    credits.go             # Credit ledger logic (section 6)
    scheduler.go           # Hourly deduction cron (section 6.2)
  blesta/
    adapter.go             # Stub — returns ErrNotImplemented for all ops

internal/controller/payments/
  provider.go              # PaymentProvider interface (section 7, already defined)
  registry.go              # Payment gateway registry
  stripe/
    client.go              # Stripe Checkout + webhooks
  paypal/
    client.go              # PayPal Orders API v2 + webhooks
  crypto/
    btcpay.go              # BTCPay Server integration
    nowpayments.go         # NOWPayments integration
```

**Migration path (zero-downtime):**

1. **Phase 0a:** Add `billing_provider` column to `customers` table via migration 000072, backfilled from current state (`whmcs_client_id != NULL -> 'whmcs'`, otherwise `'unmanaged'`). No native billing behavior is enabled yet.
2. **Phase 0b:** Create `internal/controller/billing/` package with interface + registry + WHMCS adapter (all no-ops since WHMCS drives via API). Wire registry into `server.go`.
3. **Phase 0c:** Add lifecycle hooks in `VMService` — after `CreateVM()` completes, call `registry.ForCustomer(customer.BillingProvider).OnVMCreated()`. Same for delete/resize. WHMCS adapter returns nil (no-op), so existing behavior is unchanged.
4. **Phase 0d:** Run existing integration tests. All WHMCS functionality MUST pass identically.

**Verification checklist (Phase 0):**

- [ ] `POST /provisioning/customers` still creates customers with `whmcs_client_id` and `billing_provider="whmcs"`.
- [ ] `POST /provisioning/vms` still creates VMs with `whmcs_service_id`. WHMCS adapter's `OnVMCreated()` returns nil.
- [ ] `POST /provisioning/vms/:id/suspend` still suspends VMs. No billing provider involvement.
- [ ] `POST /provisioning/vms/:id/unsuspend` still unsuspends VMs.
- [ ] `DELETE /provisioning/vms/:id` still terminates VMs. WHMCS adapter's `OnVMDeleted()` returns nil.
- [ ] `POST /provisioning/vms/:id/resize` still resizes with plan validation.
- [ ] `GET /provisioning/vms/:id/usage` still returns bandwidth/disk metrics.
- [ ] `POST /provisioning/sso-tokens` + `GET /customer/auth/sso-exchange` SSO flow works.
- [ ] `GET /provisioning/plans` still returns plan list.
- [ ] All existing unit tests pass (`make test`).
- [ ] WHMCS PHP module's `TestConnection` function succeeds against refactored controller.

---

## 10. Database Migrations

### 10.1 New Tables

All new tables follow VirtueStack conventions: UUID primary keys (`gen_random_uuid()`), `created_at`/`updated_at` timestamps, `SET lock_timeout = '5s'` in migrations.

**Migration 000072: Add `billing_provider` column to `customers`**

```sql
SET lock_timeout = '5s';

ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS billing_provider VARCHAR(20)
    CHECK (billing_provider IN ('whmcs', 'native', 'blesta', 'unmanaged'));

-- Backfill from existing ownership data instead of forcing every legacy row to WHMCS.
-- This update is intentionally scoped to rows that have never been assigned.
UPDATE customers
SET billing_provider = CASE
    WHEN whmcs_client_id IS NOT NULL THEN 'whmcs'
    ELSE 'unmanaged'
END
WHERE billing_provider IS NULL;

ALTER TABLE customers
    ALTER COLUMN billing_provider SET NOT NULL;

COMMENT ON COLUMN customers.billing_provider IS 'Which billing system manages this customer: whmcs, native, blesta, or unmanaged legacy/manual ownership';
```

Rollback:

```sql
ALTER TABLE customers DROP COLUMN IF EXISTS billing_provider;
```

**Migration 000073: `billing_accounts`** (from section 6.1)

```sql
SET lock_timeout = '5s';

CREATE TABLE billing_accounts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id        UUID NOT NULL UNIQUE REFERENCES customers(id) ON DELETE CASCADE,
    balance_cents      BIGINT NOT NULL DEFAULT 0,
    currency           VARCHAR(3) NOT NULL DEFAULT 'USD',
    stripe_customer_id VARCHAR(255),
    auto_suspend       BOOLEAN NOT NULL DEFAULT true,
    warning_sent_at    TIMESTAMPTZ,
    suspended_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_accounts_customer ON billing_accounts(customer_id);

-- RLS: customers can only see their own billing account
ALTER TABLE billing_accounts ENABLE ROW LEVEL SECURITY;
CREATE POLICY billing_accounts_customer_isolation ON billing_accounts
    FOR ALL USING (customer_id = current_setting('app.current_customer_id')::UUID);
```

Rollback:

```sql
DROP POLICY IF EXISTS billing_accounts_customer_isolation ON billing_accounts;
DROP TABLE IF EXISTS billing_accounts;
```

**Migration 000074: `billing_transactions`** (from section 6.1)

```sql
SET lock_timeout = '5s';

CREATE TABLE billing_transactions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id     UUID NOT NULL REFERENCES billing_accounts(id),
    type           VARCHAR(20) NOT NULL CHECK (type IN ('deposit', 'charge', 'refund', 'adjustment')),
    amount_cents   BIGINT NOT NULL,
    balance_after  BIGINT NOT NULL,
    description    TEXT NOT NULL,
    reference_type VARCHAR(30),
    reference_id   UUID,
    metadata       JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_tx_account   ON billing_transactions(account_id);
CREATE INDEX idx_billing_tx_created   ON billing_transactions(created_at);
CREATE INDEX idx_billing_tx_reference ON billing_transactions(reference_type, reference_id);

ALTER TABLE billing_transactions ENABLE ROW LEVEL SECURITY;
CREATE POLICY billing_tx_customer_isolation ON billing_transactions
    FOR ALL USING (account_id IN (
        SELECT id FROM billing_accounts WHERE customer_id = current_setting('app.current_customer_id')::UUID
    ));
```

Rollback:

```sql
DROP POLICY IF EXISTS billing_tx_customer_isolation ON billing_transactions;
DROP TABLE IF EXISTS billing_transactions;
```

**Migration 000075: `billing_payments`** (from section 6.1)

```sql
SET lock_timeout = '5s';

CREATE TABLE billing_payments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID NOT NULL REFERENCES billing_accounts(id),
    gateway            VARCHAR(20) NOT NULL CHECK (gateway IN ('stripe', 'paypal', 'btcpay', 'nowpayments')),
    gateway_payment_id VARCHAR(255),
    amount_cents       BIGINT NOT NULL CHECK (amount_cents > 0),
    currency           VARCHAR(3) NOT NULL DEFAULT 'USD',
    status             VARCHAR(20) NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'completed', 'failed', 'refunded')),
    metadata           JSONB,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_payments_account ON billing_payments(account_id);
CREATE INDEX idx_billing_payments_gateway ON billing_payments(gateway, gateway_payment_id);

ALTER TABLE billing_payments ENABLE ROW LEVEL SECURITY;
CREATE POLICY billing_payments_customer_isolation ON billing_payments
    FOR ALL USING (account_id IN (
        SELECT id FROM billing_accounts WHERE customer_id = current_setting('app.current_customer_id')::UUID
    ));
```

Rollback:

```sql
DROP POLICY IF EXISTS billing_payments_customer_isolation ON billing_payments;
DROP TABLE IF EXISTS billing_payments;
```

**Migration 000076: `customer_oauth_links`** (from section 8.5)

```sql
SET lock_timeout = '5s';

CREATE TABLE customer_oauth_links (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id    UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    provider       VARCHAR(20) NOT NULL,
    provider_id    VARCHAR(255) NOT NULL,
    provider_email VARCHAR(254) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_id)
);
CREATE INDEX idx_oauth_links_customer ON customer_oauth_links(customer_id);

ALTER TABLE customer_oauth_links ENABLE ROW LEVEL SECURITY;
CREATE POLICY oauth_links_customer_isolation ON customer_oauth_links
    FOR ALL USING (customer_id = current_setting('app.current_customer_id')::UUID);
```

Rollback:

```sql
DROP POLICY IF EXISTS oauth_links_customer_isolation ON customer_oauth_links;
DROP TABLE IF EXISTS customer_oauth_links;
```

### 10.2 Migration Strategy

1. **Expand-only:** All migrations add new tables or nullable columns. No ALTER on existing columns. No renames.
2. **Backfill strategy:** existing rows are backfilled from `whmcs_client_id`; rows without a WHMCS link become `unmanaged` until the operator intentionally migrates them.
   - Recommended operator workflow: review `billing_provider='unmanaged'` customers in the admin UI or via a one-time migration script, bulk-assign the intended owner, then enable native billing for that cohort.
3. **Billing tables only populated for native users:** `billing_accounts`, `billing_transactions`, and `billing_payments` rows are only created for customers with `billing_provider = 'native'`. WHMCS customers never get billing_accounts rows.
4. **Idempotent:** All `CREATE TABLE` and `CREATE INDEX` use `IF NOT EXISTS` guards.
5. **Lock timeout:** Every migration starts with `SET lock_timeout = '5s'` to prevent long locks.

### 10.3 Rollback Plan

Each migration has a corresponding `.down.sql` that drops the table or column in reverse order. Rollback order: 000076 → 000075 → 000074 → 000073 → 000072. The `billing_provider` column (000072) rollback removes the column; the system then reverts to pre-billing-plan behavior where WHMCS-linked customers are identified only by `whmcs_client_id` and native billing features must remain disabled.

---

## 11. API Endpoints

### 11.1 Billing Endpoints (Native — Gated by `BILLING_NATIVE_ENABLED=true`)

Base path: `/api/v1/customer/billing/`
Auth: JWT only (no customer API key support) in the initial implementation. Rationale: the current codebase does not define `billing:*` customer API key scopes, and money-moving endpoints should stay aligned with the existing JWT-only account-management route group until a separate scope expansion is designed. Adding billing scopes later would require a dedicated follow-up phase covering customer API key validation, route authorization, middleware, and API key management UX.

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/balance` | Get current credit balance and billing status | JWT only |
| `GET` | `/transactions` | List transaction history (paginated, cursor-based) | JWT only |
| `GET` | `/transactions/:id` | Get single transaction details | JWT only |
| `POST` | `/top-up` | Initiate credit top-up (returns payment session URL) | JWT only (CSRF protection) |
| `GET` | `/payments` | List payment history | JWT only |
| `GET` | `/usage` | Get hourly usage breakdown for current billing period | JWT only |

Admin billing endpoints (base path: `/api/v1/admin/billing/`):

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| `GET` | `/accounts` | List all billing accounts (paginated) | Admin JWT + new billing-read permission (or temporary mapping to settings/backups permissions until RBAC is extended) |
| `GET` | `/accounts/:customer_id` | Get specific customer's billing account (`:customer_id` is the customer UUID) | Admin JWT + billing-read permission (to be added as part of the billing RBAC expansion) |
| `POST` | `/accounts/:customer_id/adjust` | Manual credit adjustment (add/deduct) (`:customer_id` is the customer UUID) | Admin JWT + billing-write permission (to be added as part of the billing RBAC expansion) |
| `GET` | `/payments` | List all payments across customers | Admin JWT + billing-read permission (to be added as part of the billing RBAC expansion) |
| `POST` | `/payments/:id/refund` | Issue refund for a payment | Admin JWT + billing-write permission (to be added as part of the billing RBAC expansion) |

### 11.2 Payment Webhook Endpoints

Base path: `/api/v1/payments/webhooks/`
Auth: Per-gateway signature verification (no JWT — webhooks come from external services).

| Method | Path | Description | Signature Verification |
|--------|------|-------------|----------------------|
| `POST` | `/stripe` | Stripe webhook receiver | `Stripe-Signature` header, verified with endpoint secret via `stripe.ConstructEvent()` |
| `POST` | `/paypal` | PayPal webhook receiver | PayPal signature headers, verified via PayPal API |
| `POST` | `/crypto/btcpay` | BTCPay Server webhook | `BTCPay-Sig` header, HMAC-SHA256 with webhook secret |
| `POST` | `/crypto/nowpayments` | NOWPayments IPN callback | `x-nowpayments-sig` header, HMAC-SHA512 with IPN secret |

**Provider-agnostic routing:** The webhook router at `/api/v1/payments/webhooks/{provider}` is a thin dispatcher. Each handler:
1. Reads raw body (preserves for signature verification).
2. Verifies signature using provider-specific logic.
3. Calls `paymentRegistry.ForProvider(provider).HandleWebhook()`.
4. Returns `200 OK` on success (webhooks must return 2xx quickly).

### 11.3 Registration & OAuth Endpoints

These are already defined in section 8.3. Registration and verify-email already exist; OAuth routes remain future work:

| Method | Path | Description | Gate |
|--------|------|-------------|------|
| `POST` | `/api/v1/customer/auth/register` | Email+password registration | `ALLOW_SELF_REGISTRATION=true` |
| `POST` | `/api/v1/customer/auth/verify-email` | Email verification token consumption | `ALLOW_SELF_REGISTRATION=true` |
| `GET` | `/api/v1/customer/auth/oauth/google` | Redirect to Google consent screen | `OAUTH_GOOGLE_ENABLED=true` |
| `GET` | `/api/v1/customer/auth/oauth/google/callback` | Google OAuth callback | `OAUTH_GOOGLE_ENABLED=true` |
| `GET` | `/api/v1/customer/auth/oauth/github` | Redirect to GitHub authorization | `OAUTH_GITHUB_ENABLED=true` |
| `GET` | `/api/v1/customer/auth/oauth/github/callback` | GitHub OAuth callback | `OAUTH_GITHUB_ENABLED=true` |
| `GET` | `/api/v1/customer/auth/oauth/google/link` | Start JWT-authenticated Google account-link flow from settings | `OAUTH_GOOGLE_ENABLED=true` + JWT |
| `GET` | `/api/v1/customer/auth/oauth/google/link/callback` | Complete Google account-link flow for an already-authenticated customer | `OAUTH_GOOGLE_ENABLED=true` + signed link state |
| `GET` | `/api/v1/customer/auth/oauth/github/link` | Start JWT-authenticated GitHub account-link flow from settings | `OAUTH_GITHUB_ENABLED=true` + JWT |
| `GET` | `/api/v1/customer/auth/oauth/github/link/callback` | Complete GitHub account-link flow for an already-authenticated customer | `OAUTH_GITHUB_ENABLED=true` + signed link state |

### 11.4 Auth Middleware Requirements

| Endpoint Group | Middleware Stack |
|---------------|-----------------|
| `/customer/billing/*` | `JWTAuth` → `RequireUserType("customer")` → `CSRF(DefaultCSRFConfig())` → `CustomerRateLimits` → `Audit` (JWT-only route group; no `SkipCSRFForAPIKey(...)`) |
| `/payments/webhooks/*` | Raw body preservation → Provider-specific signature verification → `WebhookRateLimit` |
| `/customer/auth/register` | `RegistrationRateLimit` (IP-based, strict: 5 per hour per IP) |
| `/customer/auth/verify-email` | `VerificationRateLimit` (token-based, 10 per minute) |
| `/customer/auth/oauth/*` | `OAuthRateLimit` (IP-based, 20 per minute) → CSRF state validation |
| `/admin/billing/*` | `JWTAuth` → `Require2FA` → `RequirePermission(...)` → `AdminRateLimit` → `Audit` |

---

## 12. Security Considerations

### 12.1 PCI Compliance

VirtueStack MUST NOT handle, store, or transmit raw card numbers or CVVs.

- **Stripe:** Use Stripe Checkout (hosted payment page) or Stripe.js + Elements for client-side tokenization. Card data never touches VirtueStack servers. This qualifies for SAQ A (simplest PCI compliance level).
- **PayPal:** Use PayPal's JavaScript SDK with hosted fields. PayPal handles all card data. VirtueStack only receives order IDs and payment confirmations.
- **Crypto:** No PCI implications — crypto payments don't involve card data.

**Sensitive data storage:**
- `billing_payments.metadata` may contain gateway response data. Encrypt at rest using `internal/shared/crypto/` AES-256-GCM (same as `root_password_encrypted` on VMs). Mark with `json:"-"` to prevent accidental API serialization.
- `billing_transactions.metadata` must contain only non-sensitive reconciliation data by default (payment IDs, hour buckets, admin adjustment references). If a transaction needs provider payload fragments or error details that contain sensitive payment context, encrypt that metadata at rest as well.
- `billing_accounts.stripe_customer_id` is a Stripe-side reference, not sensitive — but still should not appear in logs (use `config.Secret` type pattern).

### 12.2 Crypto Payment Verification

Confirmation count thresholds per chain (minimum confirmations before crediting account):

| Chain | Confirmations | Approx. Time | Rationale |
|-------|--------------|---------------|-----------|
| Bitcoin | 2 | ~20 min | Standard for moderate-value transactions. BTCPay default is 1, but 2 is safer. |
| Ethereum | 12 | ~3 min | Post-merge finality is ~12 blocks. |
| BNB Smart Chain | 15 | ~45 sec | Faster block time, slightly more confirmations needed. |
| Base (L2) | 10 | ~20 sec | L2 with Ethereum settlement. Conservative for an L2. |

Both BTCPay Server and NOWPayments handle confirmation tracking — VirtueStack does not run its own blockchain nodes. The confirmation thresholds are configured in the gateway provider, not in VirtueStack.

**Exchange rate handling:**
- Rate is locked at invoice creation time (standard for both BTCPay and NOWPayments).
- If the customer underpays or the rate window expires, the payment is rejected (customer must retry).
- Overpayments beyond a configurable threshold (e.g., >5% above invoice amount) trigger a manual review flag — auto-credit the invoice amount, hold the excess for admin disposition.

### 12.3 Webhook Signature Verification

Every inbound webhook MUST be signature-verified before any processing:

| Provider | Header | Algorithm | Implementation |
|----------|--------|-----------|---------------|
| Stripe | `Stripe-Signature` | HMAC-SHA256 with timestamp tolerance (5 min) | `stripe.ConstructEvent(body, sig, endpointSecret)` from official SDK |
| PayPal | `PAYPAL-TRANSMISSION-SIG` + `PAYPAL-CERT-URL` | SHA256withRSA using PayPal's public cert | Verify via PayPal `POST /v1/notifications/verify-webhook-signature` API |
| BTCPay | `BTCPay-Sig` | HMAC-SHA256 with webhook secret | `hmac.Equal(computedSig, receivedSig)` — timing-safe comparison |
| NOWPayments | `x-nowpayments-sig` | HMAC-SHA512 with IPN secret | `hmac.Equal(computedSig, receivedSig)` — timing-safe comparison |
| WHMCS (existing) | Custom `X-Signature` | HMAC-SHA256 with shared secret | Already implemented in `webhook.php` via `verifyWebhookSignature()` |

All HMAC comparisons MUST use `crypto/hmac` with `hmac.Equal()` (timing-safe) or `crypto/subtle.ConstantTimeCompare()`. Never use `==` for signature comparison.

**Replay prevention:**
- Stripe: built-in via timestamp tolerance in `ConstructEvent()`.
- PayPal: `PAYPAL-TRANSMISSION-ID` header — store and reject duplicates.
- BTCPay/NOWPayments: Store `gateway_payment_id` in `billing_payments` — reject if already processed.

### 12.4 Rate Limiting

| Endpoint | Limit | Window | Key |
|----------|-------|--------|-----|
| `POST /customer/auth/register` | 5 | per hour | IP address |
| `POST /customer/auth/verify-email` | 10 | per minute | IP address |
| `GET /customer/auth/oauth/*` | 20 | per minute | IP address |
| `POST /customer/billing/top-up` | 10 | per hour | Customer ID |
| `POST /customer/billing/top-up` | 30 | per hour | Source IP |
| `POST /payments/webhooks/stripe` | 100 | per minute | IP address (Stripe IP range) |
| `POST /payments/webhooks/paypal` | 100 | per minute | IP address |
| `POST /payments/webhooks/crypto/*` | 50 | per minute | IP address |

Uses VirtueStack's existing rate limiter middleware (`internal/controller/api/middleware/ratelimit.go`).

**Denial-of-wallet controls:**
- A customer may have only a bounded number of `pending` top-up sessions / open crypto invoices at once (for example 3).
- New payment sessions are reused only when all of the following are true: same customer, same gateway, exact same `amount_cents` and `currency`, existing payment status is `pending`, and the prior payment session/invoice is less than 15 minutes old and has not expired. Otherwise a new pending payment record is created.

### 12.5 CSRF Protection

Payment initiation (`POST /customer/billing/top-up`) requires:
- JWT-only auth (no API key — prevents automated billing attacks).
- CSRF token validation via the existing middleware pattern (`middleware.CSRF(middleware.DefaultCSRFConfig())`).
- Same-origin check on `Referer` / `Origin` headers.

OAuth flows use the `state` parameter as CSRF protection (cryptographically random, short-lived, server-validated).

### 12.6 OAuth Token & Link Security

- OAuth access tokens and refresh tokens are never stored unless a provider explicitly requires refresh-token-based re-consent workflows; if persisted, they must be encrypted at rest using the existing `internal/shared/crypto/` package and excluded from JSON serialization.
- OAuth callback state must be single-use, short-lived, and stored server-side. Reuse of the same state value is rejected and logged.
- Linked OAuth providers can be revoked from the customer settings page, which deletes the associated `customer_oauth_links` row and invalidates any stored provider tokens.
- WHMCS-linked customers cannot be auto-linked by email alone; explicit in-session linking is required to prevent unintended account takeover via email collision.

### 12.7 WHMCS API Key Handling

- The WHMCS → VirtueStack provisioning key remains stored the same way it is today: hashed server-side in the `provisioning_keys` table and only shown in plaintext at creation time.
- Any future controller config for a WHMCS provider must use `config.Secret` fields and must never be exposed from debug/config endpoints in plaintext.
- The billing plan does **not** introduce a controller-side outbound WHMCS API key by default; adding one would be a separate design change.

### 12.8 Provider Isolation

- `billing_provider` is server-managed only. It is never writable from customer-facing APIs, never accepted from self-registration/profile update flows, and may only change through admin actions or controlled migration logic.
- Each `BillingProvider` implementation runs independently. An error in the native billing cron MUST NOT affect WHMCS customer operations.
- Provider operations are wrapped in per-provider error boundaries. Panics in one provider are recovered and logged without crashing the controller.
- Circuit breaker pattern: if a payment gateway consistently fails (e.g., Stripe is down), the circuit opens and returns a user-friendly error instead of queuing retries indefinitely.
- Provider health is exposed via the existing `/health` endpoint (readiness check includes billing provider status).

---

## 13. Implementation Milestones

### Phase 0: Billing Provider Abstraction + WHMCS Refactor (**DO THIS FIRST**)

**Scope:** Medium (M) — ~2-3 weeks

**What ships:**
- `internal/controller/billing/` package: `BillingProvider` interface, `Registry`, types.
- `internal/controller/billing/whmcs/adapter.go` — implements `BillingProvider` with no-ops (WHMCS drives via Provisioning API).
- Migration 000072: Add `billing_provider` column to `customers` with safe backfill to `whmcs` / `unmanaged`.
- `server.go` wiring: create `Registry`, pass to services.
- Lifecycle hooks in `VMService`: call `OnVMCreated/OnVMDeleted/OnVMResized` after operations.
- Provider config wiring in `config.go` (`BILLING_*_ENABLED`, `BILLING_*_PRIMARY`).

**Dependencies:** None (builds on existing code).

**Verification:** ALL existing WHMCS functionality MUST pass identical integration tests before and after refactor. Run `make test` + WHMCS PHP module `TestConnection`.

**Rollback:** revert the controller to the previous release, disable any new billing-provider flags, leave the additive column in place but unused, and verify `CreateAccount`, `SuspendAccount`, `SingleSignOn`, cron polling, webhook delivery, and WHMCS module `TestConnection` against the pre-refactor behavior before reattempting rollout. Do **not** immediately run the down migration for 000072 during an emergency application rollback unless you have confirmed no later code or operator workflow depends on the column.

**Can ship independently:** Yes — this is a pure refactoring. No user-facing changes.

### Phase 1: Feature Flags + Config Infrastructure

**Scope:** Small (S) — ~1 week

**What ships:**
- Config struct additions for `billing.providers.*`, `auth.oauth.*` in `config.go`.
- Env var overrides for all new flags.
- Conditional route registration for billing, OAuth, and registration endpoints.
- Operator tooling to assign `billing_provider` for `unmanaged` legacy customers (admin endpoint, admin UI action, or a documented one-time migration script shipped with the release).
- Documentation update for `.env.example`.

**Dependencies:** Phase 0 (registry must exist to load provider config).

**Can ship independently:** Yes — all new features default to disabled.

### Phase 2: Credit Ledger + Hourly Billing Engine (Native Provider)

**Scope:** Large (L) — ~3-4 weeks

**What ships:**
- Migrations 000073-000075: `billing_accounts`, `billing_transactions`, `billing_payments`.
- `internal/controller/billing/native/adapter.go` — full `BillingProvider` implementation.
- `native/credits.go` — credit ledger repository and service.
- `native/scheduler.go` — hourly deduction cron, low-balance warnings, auto-suspension.
- Customer billing API endpoints: balance, transactions, usage history.
- Admin billing API endpoints: account management, manual adjustments.
- RLS policies on all billing tables.

**Dependencies:** Phase 0 + Phase 1.

**Can ship independently:** Yes — but useless without at least one payment gateway (Phase 3).

### Phase 3: Stripe Integration

**Scope:** Medium (M) — ~2 weeks

**What ships:**
- `internal/controller/payments/stripe/client.go` — Stripe Checkout session creation.
- `POST /customer/billing/top-up` endpoint.
- `POST /payments/webhooks/stripe` webhook handler with signature verification.
- Stripe Customer mapping (lazy creation, stored in `billing_accounts`).
- Config: `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PUBLISHABLE_KEY`.
- Customer WebUI: billing page with balance display + "Add Credit" button.

**Dependencies:** Phase 2 (credit ledger must exist to receive deposits).

**Can ship independently:** Yes — Stripe alone is sufficient for a minimum viable billing flow.

### Phase 4: PayPal Integration

**Scope:** Medium (M) — ~2 weeks

**What ships:**
- `internal/controller/payments/paypal/client.go` — PayPal Orders API v2 integration.
- `POST /payments/webhooks/paypal` webhook handler.
- PayPal sandbox testing support.
- Customer WebUI: PayPal as a payment option on top-up page.

**Dependencies:** Phase 2.

**Can ship independently:** Yes — PayPal is an additional payment method.

### Phase 5: Crypto Integration

**Scope:** Large (L) — ~3-4 weeks

**What ships:**
- `internal/controller/payments/crypto/btcpay.go` — BTCPay Server Greenfield API client.
- `internal/controller/payments/crypto/nowpayments.go` — NOWPayments API client.
- `POST /payments/webhooks/crypto/btcpay` and `/payments/webhooks/crypto/nowpayments` handlers.
- Supported chains: BTC, ETH + ERC-20 stablecoins, BNB + BEP-20 stablecoins, Base USDC.
- Config: `CRYPTO_PROVIDER` (`btcpay` | `nowpayments`), provider-specific keys.
- Customer WebUI: crypto as a payment option.

**Dependencies:** Phase 2.

**Can ship independently:** Yes — crypto is an additional payment method.

Suggested order: BTC (simplest via BTCPay), then EVM chains (ETH, BNB, Base).

### Phase 6: Native Registration + Google/GitHub OAuth

**Scope:** Medium (M) — ~2-3 weeks

**What ships:**
- Migration 000076: `customer_oauth_links` table.
- OAuth handlers: Google and GitHub sign-in using `golang.org/x/oauth2`.
- Account linking logic (section 8.5).
- JWT-protected explicit OAuth link flows for already-authenticated customers, required for WHMCS-linked accounts.
- PKCE support for all OAuth flows.
- `ALLOW_SELF_REGISTRATION`, `OAUTH_GOOGLE_ENABLED`, `OAUTH_GITHUB_ENABLED` flags.
- Customer WebUI: "Sign in with Google" / "Sign in with GitHub" buttons.
- Email verification improvements (rate limiting per section 4.5).

**Dependencies:** Phase 1 (feature flags must exist). Independent of Phases 2-5 (billing).

**Can ship independently:** Yes — registration is independent of billing.

### Phase 7: Blesta Provider Stub

**Scope:** Small (S) — ~1 week

**What ships:**
- `internal/controller/billing/blesta/adapter.go` — stub implementing `BillingProvider` with `ErrNotImplemented` for all operations.
- Config: `BILLING_BLESTA_ENABLED=false` (cannot be enabled yet).
- Documentation for future Blesta integration.

**Dependencies:** Phase 0 (interface must exist).

**Can ship independently:** Yes — it's a stub.

### Dependency Graph

```
Phase 0 (Abstraction) ─┬─► Phase 1 (Config) ─┬─► Phase 2 (Credit Ledger) ─┬─► Phase 3 (Stripe)
                        │                      │                            ├─► Phase 4 (PayPal)
                        │                      │                            └─► Phase 5 (Crypto)
                        │                      └─► Phase 6 (Registration/OAuth)
                        └─► Phase 7 (Blesta stub)
```

### Total Estimated Timeline

| Phase | Scope | Weeks | Cumulative |
|-------|-------|-------|------------|
| 0 | M | 2-3 | 2-3 |
| 1 | S | 1 | 3-4 |
| 2 | L | 3-4 | 6-8 |
| 3 | M | 2 | 8-10 |
| 4 | M | 2 | 10-12 |
| 5 | L | 3-4 | 13-16 |
| 6 | M | 2-3 | 15-19 |
| 7 | S | 1 | 16-20 |

Phases 3, 4, 5, and 6 can be parallelized if multiple developers are available. The critical path is: Phase 0 → Phase 1 → Phase 2 → Phase 3 (minimum viable native billing in ~8-10 weeks).

---

## 14. Testing, Monitoring, and Dependency Requirements

### 14.1 Testing Strategy

- **Phase 0 (WHMCS refactor):**
  - existing Go unit tests (`make test`)
  - WHMCS module `TestConnection`
  - manual/automated smoke tests for `CreateAccount`, `SuspendAccount`, `UnsuspendAccount`, `TerminateAccount`, `UsageUpdate`, and `SingleSignOn`
- **Phase 2 (native ledger):**
  - unit tests for ledger mutation, locking, insufficient-balance handling, and reconciliation logic
  - repository tests for unique/idempotency constraints
  - integration tests for scheduler catch-up after simulated downtime
- **Phase 3-5 (payments):**
  - provider sandbox/integration tests (Stripe test mode, PayPal sandbox, BTCPay test instance, NOWPayments sandbox if available)
  - webhook replay tests to prove duplicate delivery does not double-credit
  - refund and reversal tests that emit compensating ledger entries
- **Phase 6 (OAuth):**
  - unit tests for collision policy, especially `whmcs_client_id != NULL`
  - callback/state replay tests
  - explicit-linking tests for WHMCS-owned accounts

### 14.2 Monitoring & Alerting

- Export billing-specific Prometheus metrics:
  - successful / failed payment session creation
  - webhook verification failures
  - duplicate webhook suppression counts
  - scheduler lag / missed-hour reconciliation counts
  - auto-suspension counts
- Add alerting for:
  - repeated gateway failures
  - scheduler reconciliation backlog
  - unusually high manual-review / underpayment volume
  - inability to post ledger entries
- Notify operators through the existing notification channels when billing enters a degraded state.

### 14.3 Dependency Notes

Planned Go dependencies implied by this design:

| Dependency | Purpose | Notes |
|------------|---------|-------|
| `github.com/stripe/stripe-go/v82` | Stripe Checkout + webhook verification | Official SDK; pin exact major/minor version and review advisories before adding |
| `golang.org/x/oauth2` | OAuth 2.0 client flows | Actively maintained Go subrepo; used for Google/GitHub OAuth |
| `golang.org/x/oauth2/google` | Google endpoint helpers | Part of `x/oauth2`; low integration risk |
| `github.com/google/uuid` | already present | Reuse existing dependency; no new addition needed |

Dependency policy for the remaining providers:
- **PayPal:** prefer direct REST integration over unofficial/stale Go SDKs unless a well-maintained official SDK becomes available.
- **BTCPay / NOWPayments:** prefer direct HTTPS clients over thin third-party wrappers unless the wrapper is actively maintained and passes security review.
- Run dependency/advisory review before implementation; do not add libraries with unresolved critical CVEs or unclear maintenance status.

### 14.4 Migration Compatibility Notes

- Proposed migrations 000072-000076 do not conflict numerically with the current migration chain (latest existing migration is 000071).
- The billing plan must continue following the repository's migration rules: additive changes first, `SET lock_timeout = '5s';`, and no `CREATE INDEX CONCURRENTLY`.
