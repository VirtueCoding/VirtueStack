# Billing Plan Review — Design Amendments

**Date:** 2026-03-31
**Base document:** `docs/billplan.md`
**Purpose:** Fixes, additions, and design decisions gathered during brainstorming review. All changes here amend the billing plan spec before writing the implementation plan.

---

## 1. Schema/Migration Discrepancies (Bugs in Spec)

These are inconsistencies between the schema definitions in section 6.1 and the migrations in section 10.1 of billplan.md.

### 1a. Missing `idempotency_key` in billing_transactions migration (000074)

**Section 6.1 defines:**
```sql
idempotency_key VARCHAR(255),
```
Plus a unique partial index:
```sql
CREATE UNIQUE INDEX idx_billing_tx_idempotency ON billing_transactions(idempotency_key) WHERE idempotency_key IS NOT NULL;
```

**Migration 000074 omits both.** This column is critical for double-spend prevention — every inbound payment webhook maps to a deterministic idempotency key (section 6.1, lines 474-483). Without it, duplicate webhook deliveries can credit twice.

**Fix:** Add `idempotency_key VARCHAR(255)` and its unique partial index to migration 000074.

### 1b. Missing `reuse_key` in billing_payments migration (000075)

**Section 6.1 defines:**
```sql
reuse_key VARCHAR(255),
```
Plus a unique partial index:
```sql
CREATE UNIQUE INDEX idx_billing_payments_reuse_key ON billing_payments(reuse_key) WHERE status = 'pending' AND reuse_key IS NOT NULL;
```

**Migration 000075 omits both.** This column enables denial-of-wallet controls (section 12.4, lines 1451-1453) — preventing unlimited pending payment sessions.

**Fix:** Add `reuse_key VARCHAR(255)` and its unique partial index to migration 000075.

### 1c. Missing `billing_vm_checkpoints` table

Section 6.2 requires a durable per-VM billing checkpoint for hourly deduction reconciliation but no schema or migration is defined.

**Fix:** Add new migration for `billing_vm_checkpoints`:
```sql
CREATE TABLE billing_vm_checkpoints (
    vm_id          UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    charge_hour    TIMESTAMPTZ NOT NULL,  -- truncated to the hour
    account_id     UUID NOT NULL REFERENCES billing_accounts(id),
    amount_cents   BIGINT NOT NULL,
    currency       VARCHAR(3) NOT NULL,
    transaction_id UUID NOT NULL REFERENCES billing_transactions(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (vm_id, charge_hour)
);
CREATE INDEX idx_billing_vm_ckpt_account ON billing_vm_checkpoints(account_id);
```

The `PRIMARY KEY (vm_id, charge_hour)` makes double-billing physically impossible at the database level.

### 1d. Missing customer model migrations for OAuth

Section 8.7 adds `AuthProvider *string` and changes `PasswordHash` to `*string` (nullable for OAuth-only accounts), but no migration makes `password_hash` nullable or adds `auth_provider` to the `customers` table.

**Fix:** Add migration in the OAuth phase (Phase 7):
```sql
-- Make password_hash nullable for OAuth-only accounts
ALTER TABLE customers ALTER COLUMN password_hash DROP NOT NULL;

-- Add auth_provider column
ALTER TABLE customers ADD COLUMN auth_provider VARCHAR(20) DEFAULT 'local'
    CHECK (auth_provider IN ('local', 'google', 'github'));
```

---

## 2. New Design Decisions

### 2a. Configurable VM billing by state (running vs stopped)

**Decision:** Per-plan configurable pricing for running vs stopped VMs.

**Rationale:** WHMCS flat monthly billing charges the same whether a VM is running or stopped (resources reserved). Native hourly billing should allow reduced pricing for stopped VMs since they consume fewer resources (no CPU/RAM active, only disk).

**Schema change — add to `plans` table:**
```sql
ALTER TABLE plans ADD COLUMN price_hourly_stopped BIGINT;
-- NULL = same as price_hourly (default behavior)
-- 0 = free when stopped
-- >0 = custom stopped rate
```

**Billing cron behavior:**
- Check VM state: if `running`, use `price_hourly`; if `stopped`, use `price_hourly_stopped` (falls back to `price_hourly` if NULL).
- VMs in `provisioning`, `suspended`, `migrating`, `reinstalling`, `error`, or `deleted` states are NOT charged.

### 2b. Multi-currency support

**Decision:** Multi-currency from the start. Plans have a `currency` field.

**Schema changes:**
- `plans` table: Add `currency VARCHAR(3) NOT NULL DEFAULT 'USD'`
- Make `price_monthly` and `price_hourly` **nullable** (NULL = plan has no native billing price, managed externally by WHMCS/Blesta). This is safer than using 0 (which could be a legitimate free plan).
- CHECK constraint: `(price_monthly IS NULL) = (price_hourly IS NULL)` — both null or both set.

**Exchange rate system:**
- New table `exchange_rates` for storing rates (source_currency, target_currency, rate, admin_override, discount_percent).
- Default to API-fetched rates. Recommended provider: Open Exchange Rates (free tier: 1000 req/month, covers hourly updates). Fallback: exchangerate-api.com. The specific API is admin-configurable via `EXCHANGE_RATE_API_URL` and `EXCHANGE_RATE_API_KEY`.
- Admin can override per currency pair with ±% discount/markup.
- Admin UI page for managing exchange rates.
- Background scheduler fetches rates periodically (configurable, default every hour).
- Hourly billing deduction converts plan currency → account currency using the current rate.
- If exchange rate is unavailable (API down, no admin override), billing deduction is **skipped** for cross-currency VMs and an operator alert is emitted. Same-currency deductions proceed normally.

**New table:**
```sql
CREATE TABLE exchange_rates (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_currency   VARCHAR(3) NOT NULL,
    target_currency   VARCHAR(3) NOT NULL,
    api_rate          NUMERIC(16, 8),           -- latest API-fetched rate
    admin_rate        NUMERIC(16, 8),           -- admin override (NULL = use api_rate)
    discount_percent  NUMERIC(5, 2) DEFAULT 0,  -- ±% adjustment applied to effective rate
    api_fetched_at    TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_currency, target_currency)
);
```

**Effective rate calculation:** `effective_rate = (admin_rate ?? api_rate) * (1 + discount_percent/100)`

### 2c. Auto-delete after suspension (native billing)

**Decision:** Configurable auto-delete for native-billing VMs after suspension.

- WHMCS-managed VMs: deletion controlled by WHMCS module (`TerminateAccount`) — no change.
- Native billing VMs: configurable `auto_delete_days` (default 0 = disabled).
- Scheduler iterates native-billed customers' VMs: if `billing_accounts.suspended_at IS NOT NULL` and `NOW() > suspended_at + auto_delete_days`, triggers VM deletion.
- Data-deletion warning email sent 7 days before auto-delete.
- Config: `BILLING_NATIVE_AUTO_DELETE_DAYS=14` (0 = disabled).

### 2d. Startup behavior without payment gateways

**Decision:** Warn loudly at startup but allow the system to run.

- If `BILLING_NATIVE_ENABLED=true` but no payment gateways are configured, log a prominent warning at `WARN` level.
- Admin can still add credit manually via `POST /admin/billing/accounts/:customer_id/adjust`.
- All manual adjustments are audit-logged in the immutable billing_transactions ledger.

### 2e. Multi-provider simultaneous operation

**Decision:** All billing providers can be active simultaneously (WHMCS + native + Blesta). Each customer is assigned to exactly one.

**Threat mitigations:**
1. `billing_provider` is server-managed only — never in customer-facing API payloads, never writable from customer endpoints.
2. Native billing endpoints return 403 for non-native customers.
3. Provider switch requires balance ≥ 0 (admin override with audit log available).
4. Config validation: if `ALLOW_SELF_REGISTRATION=true`, exactly one enabled provider must be primary.
5. `unmanaged` provider rejects all billing operations — forces explicit operator assignment.

### 2f. Credit top-up configuration

**Decision:** Free-form amounts with admin-configurable min/max and preset suggestions.

- Admin-configurable: `min_topup_amount`, `max_topup_amount` (per currency).
- Admin-configurable preset suggestions (e.g., $5, $10, $25, $50, $100) — stored in `system_settings` or a new `billing_topup_presets` table.
- Customer can enter any amount between min and max.
- Per-currency presets.

### 2g. HA billing scheduler

**Decision:** PostgreSQL advisory lock + unique constraint (defense in depth).

- **Advisory lock** (`pg_try_advisory_lock`): ensures only one controller instance runs the scheduler at a time (efficiency — avoids duplicate work).
- **Unique constraint** on `billing_vm_checkpoints(vm_id, charge_hour)`: makes double-billing physically impossible at the DB level, even if the advisory lock mechanism fails.
- No additional dependency beyond PostgreSQL.
- Advisory lock is an optimization; the unique constraint is the safety net.

---

## 3. New Features Added to Scope

### 3a. Full Invoicing System (new phase, after Stripe)

**Scope:** Sequential invoice numbers, tax-exempt for v1, downloadable PDFs.

**Invoice numbering:**
- Admin-configurable prefix (e.g., "VS-", "INV-").
- Sequential counter per year, stored atomically in DB.
- Gap-free enforcement (accounting compliance).
- UUID primary key for API access (prevents IDOR enumeration).
- Sequential number for display/print only, never in URLs.

**New table:**
```sql
CREATE TABLE billing_invoices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES billing_accounts(id),
    invoice_number  VARCHAR(50) NOT NULL UNIQUE,   -- "VS-2026-00001"
    year            INT NOT NULL,
    sequence        INT NOT NULL,
    amount_cents    BIGINT NOT NULL,
    currency        VARCHAR(3) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'paid'
                    CHECK (status IN ('paid', 'refunded', 'void')),
    payment_id      UUID REFERENCES billing_payments(id),
    line_items      JSONB NOT NULL,                 -- [{description, amount_cents, quantity}]
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    pdf_path        TEXT,                           -- stored in BILLING_INVOICE_PATH (configurable, default /var/lib/virtuestack/invoices/)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(year, sequence)
);

-- Invoice counter for gap-free numbering
CREATE TABLE billing_invoice_counters (
    year    INT PRIMARY KEY,
    counter INT NOT NULL DEFAULT 0
);
```

**PDF generation:** Go library (e.g., `go-pdf/fpdf` or similar). Admin-configurable company details (name, address, logo) stored in `system_settings`.

**Tax:** Exempt for v1. Structure allows adding tax fields later (tax_rate, tax_amount, vat_number on invoices).

**Phase placement:** New Phase 4 (between Stripe and PayPal in the original plan).

### 3b. In-App Notification Center (new phase, before billing)

**Scope:** Full notification center for both customer and admin portals.

**Components:**
- Bell icon with unread count badge.
- Notification drawer/panel with notification list.
- Mark as read (individual and bulk).
- Notification type categories (billing, VM, security, system).
- Real-time delivery via SSE (Server-Sent Events).
- Email + in-app + Telegram channels (individually toggleable per notification type).

**SSE choice rationale (threat modeling):**
- Unidirectional (server → client only) — perfect for notifications.
- JWT auth on initial HTTP request — standard middleware applies.
- Auto-reconnect built into browser's EventSource API.
- No interference with existing VNC/serial WebSocket infrastructure.
- Rate limit: max 2 SSE connections per customer/admin to prevent resource exhaustion.

**New table:**
```sql
CREATE TABLE notifications (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL,            -- customer or admin UUID
    user_type    VARCHAR(10) NOT NULL CHECK (user_type IN ('customer', 'admin')),
    category     VARCHAR(30) NOT NULL,     -- 'billing', 'vm', 'security', 'system'
    title        TEXT NOT NULL,
    message      TEXT NOT NULL,
    severity     VARCHAR(10) NOT NULL DEFAULT 'info'
                 CHECK (severity IN ('info', 'warning', 'error', 'success')),
    is_read      BOOLEAN NOT NULL DEFAULT false,
    metadata     JSONB,                    -- structured data (vm_id, payment_id, etc.)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notifications_user ON notifications(user_id, user_type, is_read);
CREATE INDEX idx_notifications_created ON notifications(created_at);
```

**RLS policy:** Customers see only their own notifications (`user_id = current_setting('app.current_customer_id')::UUID AND user_type = 'customer'`). Admin notifications are filtered by `user_id` in the application layer (admin middleware extracts admin ID from JWT) — not via RLS since admins don't use `app.current_customer_id`.

**Security considerations:**
- Notification content must be plain text + structured metadata (no raw HTML) to prevent XSS.
- Notifications are soft-delete only (immutable audit trail for billing warnings).
- Cap stored notifications per user (e.g., 1000, oldest auto-purged).
- SSE connections authenticated via JWT, rate-limited to 2 per user.
- Admin notifications may contain customer IDs — admin portal already requires 2FA.

**Webhook events for billing:** Add new event types to the existing system_webhooks and customer_webhooks infrastructure:
- `billing.low_balance` — customer balance below threshold
- `billing.auto_suspended` — VMs auto-suspended for non-payment
- `billing.payment_completed` — credit top-up confirmed
- `billing.payment_failed` — payment attempt failed
- `billing.auto_delete_warning` — VM scheduled for auto-deletion

**Phase placement:** New Phase 2 (between Config and Credit Ledger in the revised plan).

### 3c. Customer Billing UI

Full customer billing page in the customer portal:
- Balance display with currency
- Transaction history (paginated, filterable by type)
- Top-up form (free-form amount with preset suggestions, payment method selector)
- Usage breakdown (current billing period, per-VM hourly costs)
- Payment history
- Invoice list with PDF download links

### 3d. Admin Billing UI

Dedicated admin billing section:
- Billing accounts list (all customers, filterable by status/provider)
- Account detail page (balance, transactions, payments, invoices)
- Manual credit adjustment form (with reason field, audit-logged)
- Payments list (all payments across customers, filterable by gateway/status)
- Exchange rates management page (view API rates, set overrides, configure ±% discount)
- Invoice management (view, void, regenerate PDF)

---

## 4. Revised Phase Structure

The original 8 phases (0–7) are revised to 10 phases to accommodate the new scope:

```
Phase 0: Billing Provider Abstraction + WHMCS Refactor
Phase 1: Feature Flags + Config Infrastructure
Phase 2: In-App Notification Center (NEW)
Phase 3: Credit Ledger + Hourly Billing Engine + Exchange Rates
Phase 4: Stripe Integration + Customer Billing UI
Phase 5: Invoicing System (NEW)
Phase 6: PayPal Integration
Phase 7: Crypto Integration
Phase 8: Native Registration + Google/GitHub OAuth
Phase 9: Blesta Provider Stub
```

### Dependency Graph

```
Phase 0 (Abstraction) ─┬─► Phase 1 (Config) ─┬─► Phase 2 (Notifications) ──► Phase 3 (Ledger) ─┬─► Phase 4 (Stripe + UI)
                        │                      │                                                   ├─► Phase 6 (PayPal)
                        │                      │                                                   └─► Phase 7 (Crypto)
                        │                      │
                        │                      └─► Phase 8 (Registration/OAuth)
                        │
                        └─► Phase 9 (Blesta stub)
                        
Phase 4 (Stripe) ──► Phase 5 (Invoicing)
```

**Critical path:** Phase 0 → 1 → 2 → 3 → 4 (minimum viable native billing with Stripe + notifications).

**Parallelizable:** Phases 6, 7, 8 are independent of each other (all depend on Phase 3).

---

## 5. Summary of All Changes to billplan.md

### Bug Fixes (must fix before implementation)
1. Add `idempotency_key` column + index to migration 000074 (billing_transactions)
2. Add `reuse_key` column + index to migration 000075 (billing_payments)
3. Add `billing_vm_checkpoints` table and migration
4. Add customer model migrations for nullable `password_hash` and `auth_provider` column

### Schema Additions
5. `plans.price_hourly_stopped BIGINT` — per-plan stopped VM pricing
6. `plans.currency VARCHAR(3) NOT NULL DEFAULT 'USD'` — multi-currency plans
7. Make `plans.price_monthly` and `plans.price_hourly` nullable (NULL = externally managed)
8. `exchange_rates` table — multi-currency support with API + admin override
9. `billing_vm_checkpoints` table — HA-safe hourly deduction with unique constraint
10. `billing_invoices` + `billing_invoice_counters` tables — invoicing system
11. `notifications` table — in-app notification center

### Design Additions
12. Per-plan stopped VM pricing (configurable)
13. Multi-currency with exchange rate management
14. Auto-delete after configurable days of suspension (native billing only)
15. Startup warning (not failure) when no payment gateways configured
16. All providers simultaneously active (per-customer assignment)
17. Free-form top-up with admin-configurable min/max and presets
18. PostgreSQL advisory lock + unique constraint for HA billing scheduler
19. Full invoicing system (sequential numbers, PDFs, tax-exempt v1)
20. In-app notification center (SSE, both portals, billing webhook events)
21. Full customer billing UI page
22. Dedicated admin billing UI section (accounts, payments, exchange rates, invoices)
23. Billing-specific webhook events for external systems
