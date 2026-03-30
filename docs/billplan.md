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
