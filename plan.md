# Billing & Hourly Credit Feature — Architecture Decision Plan

> **Status:** DRAFT — For review, no code changes  
> **Date:** 2026-03-27  
> **Question:** Should billing/hourly credits with PayPal/Stripe/Crypto be a separate project, a separate module, or coded directly into the customer WebUI and controller?

---

## Executive Summary

**Recommendation: Build as an integrated module within the existing VirtueStack codebase** — not a separate project, and not a loosely-coupled microservice. The current modular-monolith architecture (single Go controller, shared PostgreSQL, NATS task queue) is a natural fit. Payment gateway integrations (Stripe, PayPal, Crypto) should be abstracted behind a Go interface so they can be swapped or extended without touching billing logic.

---

## 1. Current Architecture Assessment

### What Exists Today

| Layer | Billing Readiness | Details |
|-------|-------------------|---------|
| **Database** | 🟡 Partial | `plans` table already has `price_monthly` (BIGINT cents) and `price_hourly` (BIGINT cents). No customer balance, transaction, or invoice tables exist. |
| **Controller** | 🟡 Partial | 28 services in modular-monolith pattern. Plan CRUD exists with pricing fields. No billing service, no credit logic, no payment processing. |
| **NATS Task System** | ✅ Ready | 11 registered async task types with retry/backoff. Billing tasks (hourly charge, invoice generation, payment webhooks) would register identically. |
| **Customer API** | 🔴 None | Zero billing endpoints. No credit balance, transaction history, payment method, or invoice endpoints. |
| **Admin API** | 🟡 Partial | Plan pricing CRUD exists. No customer credit management, no billing administration, no refund processing. |
| **Provisioning API** | 🟡 Partial | `UsageUpdate()` returns resource allocation only — no charge data, no credit sync. |
| **Customer WebUI** | 🔴 None | 2 nav items (VMs, Settings). No billing page, no payment UI, no Stripe/PayPal SDK. 952-line API client with 10 modules — no billing module. |
| **WHMCS Module** | 🟡 Partial | Account lifecycle (create/suspend/terminate) works. `UsageUpdate` is a stub. No hourly billing hooks. |

### Key Architectural Facts

- **Deployment:** 6 Docker containers (Postgres, NATS, Controller, Admin UI, Customer UI, Nginx) on a single bridge network. All traffic enters through Nginx.
- **Controller pattern:** Modular monolith — single Go binary, shared DB pool, shared NATS connection, all services co-located in `internal/controller/services/`.
- **Frontend pattern:** Next.js with client-side auth (`RequireAuth` wrapper), sidebar navigation from `nav-items.ts`, centralized API client in `lib/api-client.ts`.
- **Task pattern:** NATS JetStream with worker pool (4 workers), exponential backoff retry, handler registration in `tasks/handlers.go`.

---

## 2. Three Options Evaluated

### Option A: Separate Project (Standalone Billing Microservice)

A completely independent codebase with its own database, API, and deployment.

| Pros | Cons |
|------|------|
| Independent scaling of billing | Requires inter-service RPC (gRPC/REST) for every VM/customer lookup |
| Separate team can own it | Distributed transactions for credit deduction + VM provisioning |
| Technology freedom (could use different language) | Duplicated auth (JWT validation, RBAC, customer context) |
| Isolated failure domain | Separate CI/CD pipeline, Docker image, deployment config |
| | Data consistency problems (eventual consistency for credits) |
| | Operational complexity (separate DB, migrations, monitoring) |
| | WHMCS module must talk to TWO APIs instead of one |
| | **~3–4× more effort** than integrated approach |

**Verdict: ❌ Not recommended.** VirtueStack is a single-team product with <30 services. The operational overhead of a separate project far outweighs the scaling benefit. Hourly billing is tightly coupled to VM lifecycle events (create, delete, suspend, resize) — splitting it creates distributed transaction nightmares.

### Option B: Separate Go Module (Same Repo, Separate `go.mod`)

A Go module within the monorepo (e.g., `modules/billing/`) with its own `go.mod`, compiled separately or linked as a library.

| Pros | Cons |
|------|------|
| Clear code boundary | Separate `go.mod` complicates dependency management |
| Could be versioned independently | Still needs access to shared DB, models, auth middleware |
| | Go multi-module repos are operationally painful (replace directives, version sync) |
| | No real benefit over package-level separation |
| | Payment gateway SDKs would need to be in this module AND tested separately |

**Verdict: ❌ Not recommended.** Go multi-module repos add complexity without meaningful benefit for a feature this tightly integrated. Package-level separation within the existing `go.mod` achieves the same code boundary without the tooling pain.

### Option C: Integrated Module (New Packages in Existing Codebase) ✅

New packages within the existing controller and customer WebUI, following the same patterns as every other feature.

| Pros | Cons |
|------|------|
| Shares existing auth, middleware, DB pool, NATS | Billing code lives in same binary (scales together) |
| Atomic transactions (credit deduction + VM action in same DB tx) | Larger binary size (marginal) |
| Single CI/CD pipeline, single deployment | Payment gateway SDKs added to `go.mod` |
| WHMCS module talks to one API | Must be careful about package boundaries |
| Consistent error handling, logging, audit trail | |
| Reuses existing test infrastructure | |
| **~1× effort** (baseline) | |

**Verdict: ✅ Recommended.** This matches the existing architecture pattern. Every other feature (backups, snapshots, migrations, webhooks, 2FA) was built this way. Billing is no different.

---

## 3. Recommended Architecture

### 3.1 Backend (Go Controller)

```
internal/controller/
├── models/
│   ├── billing.go              # Credit, Transaction, Invoice, PaymentMethod models
│   └── billing_enums.go        # Transaction types, payment statuses, gateway enums
├── repository/
│   ├── billing_repo.go         # Credit balance, transaction history queries
│   ├── invoice_repo.go         # Invoice CRUD, line items
│   └── payment_repo.go         # Payment method storage (tokenized, never raw card)
├── services/
│   ├── billing_service.go      # Core: credit calc, hourly charges, balance checks
│   ├── invoice_service.go      # Invoice generation, PDF rendering
│   └── payment_service.go      # Gateway abstraction, webhook processing
├── api/
│   ├── customer/
│   │   ├── billing.go          # GET /credits, GET /transactions, GET /invoices
│   │   └── payment_methods.go  # CRUD payment methods (Stripe tokens, not raw cards)
│   ├── admin/
│   │   ├── billing.go          # Manage customer credits, view all transactions
│   │   └── billing_settings.go # Configure billing rates, tax, gateways
│   └── provisioning/
│       └── billing.go          # Usage reporting for WHMCS sync
├── tasks/
│   ├── billing_hourly.go       # Hourly credit deduction task
│   ├── billing_invoice.go      # Monthly invoice generation task
│   └── billing_webhook.go      # Payment gateway webhook processing
└── payment/                    # NEW: Payment gateway abstractions
    ├── gateway.go              # PaymentGateway interface
    ├── stripe.go               # Stripe implementation
    ├── paypal.go               # PayPal implementation
    └── crypto.go               # Crypto payment implementation (BTCPay/NOWPayments/etc.)
```

### 3.2 Payment Gateway Interface

```go
// internal/controller/payment/gateway.go
type PaymentGateway interface {
    // CreateCustomer creates a customer record in the payment provider
    CreateCustomer(ctx context.Context, customer CustomerInfo) (providerID string, err error)

    // CreatePaymentIntent initiates a payment (returns client secret for frontend)
    CreatePaymentIntent(ctx context.Context, req PaymentRequest) (*PaymentIntent, error)

    // GetPaymentStatus checks payment status by provider reference
    GetPaymentStatus(ctx context.Context, providerRef string) (*PaymentStatus, error)

    // ProcessWebhook validates and parses incoming webhook from provider
    ProcessWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error)

    // Refund issues a full or partial refund
    Refund(ctx context.Context, paymentRef string, amountCents int64) (*RefundResult, error)

    // Name returns the gateway identifier ("stripe", "paypal", "crypto")
    Name() string
}
```

This interface allows adding new payment providers without touching billing logic.

### 3.3 Database Schema (New Tables)

```sql
-- Customer credit balance (one row per customer)
CREATE TABLE customer_credits (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL UNIQUE REFERENCES customers(id),
    balance_cents   BIGINT NOT NULL DEFAULT 0,
    currency        VARCHAR(3) NOT NULL DEFAULT 'USD',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Billing transactions (immutable ledger)
CREATE TABLE billing_transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL REFERENCES customers(id),
    vm_id           UUID REFERENCES vms(id),
    type            VARCHAR(30) NOT NULL,  -- 'hourly_charge', 'credit_topup', 'payment', 'refund', 'adjustment'
    amount_cents    BIGINT NOT NULL,       -- Positive = charge, Negative = credit
    balance_after   BIGINT NOT NULL,       -- Running balance snapshot
    description     TEXT,
    reference_type  VARCHAR(30),           -- 'stripe', 'paypal', 'crypto', 'admin', 'system'
    reference_id    VARCHAR(255),          -- External payment/invoice ID
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Invoices (monthly billing statements)
CREATE TABLE invoices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL REFERENCES customers(id),
    invoice_number  VARCHAR(50) NOT NULL UNIQUE,
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    subtotal_cents  BIGINT NOT NULL DEFAULT 0,
    tax_cents       BIGINT NOT NULL DEFAULT 0,
    total_cents     BIGINT NOT NULL DEFAULT 0,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft', -- draft, issued, paid, void
    paid_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Invoice line items
CREATE TABLE invoice_line_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id      UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    vm_id           UUID REFERENCES vms(id),
    description     TEXT NOT NULL,
    quantity         DECIMAL(10,4) NOT NULL DEFAULT 1,  -- e.g., hours
    unit_price_cents BIGINT NOT NULL,
    total_cents     BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Payment methods (tokenized — NEVER store raw card numbers)
CREATE TABLE payment_methods (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL REFERENCES customers(id),
    gateway         VARCHAR(20) NOT NULL,  -- 'stripe', 'paypal', 'crypto'
    provider_id     VARCHAR(255) NOT NULL, -- Stripe pm_xxx, PayPal vault ID
    type            VARCHAR(30) NOT NULL,  -- 'card', 'paypal', 'bitcoin', 'usdt'
    label           VARCHAR(100),          -- "Visa ending 4242", "PayPal john@..."
    is_default      BOOLEAN NOT NULL DEFAULT false,
    metadata        JSONB,                 -- Gateway-specific metadata
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- RLS policies for customer isolation
ALTER TABLE customer_credits ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_line_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_methods ENABLE ROW LEVEL SECURITY;
-- (policies follow existing pattern: customer_id = current_setting('app.current_customer_id'))
```

### 3.4 Customer WebUI Changes

```
webui/customer/
├── app/
│   └── billing/
│       ├── layout.tsx              # RequireAuth wrapper
│       ├── page.tsx                # Dashboard: credit balance, recent transactions, active VMs cost
│       ├── transactions/page.tsx   # Full transaction history with filters
│       ├── invoices/page.tsx       # Invoice list
│       ├── invoices/[id]/page.tsx  # Invoice detail + PDF download
│       ├── topup/page.tsx          # Add credits (Stripe Elements / PayPal button / Crypto QR)
│       └── payment-methods/page.tsx # Manage saved payment methods
├── components/
│   └── billing/
│       ├── CreditBalance.tsx       # Balance display widget (reusable in sidebar/header)
│       ├── TransactionTable.tsx    # Transaction list with pagination
│       ├── InvoiceTable.tsx        # Invoice list
│       ├── TopupForm.tsx           # Credit top-up form with gateway selection
│       ├── StripePayment.tsx       # Stripe Elements integration
│       ├── PayPalPayment.tsx       # PayPal button integration
│       └── CryptoPayment.tsx       # Crypto payment flow (QR code, address, status polling)
├── lib/
│   ├── api-client.ts              # Add billingApi module (follows existing pattern)
│   └── nav-items.ts               # Add billing nav entry
```

**New npm dependencies needed:**
- `@stripe/stripe-js` + `@stripe/react-stripe-js` — Stripe Elements (PCI-compliant tokenization)
- `@paypal/react-paypal-js` — PayPal button SDK
- No crypto SDK needed client-side (QR code + status polling via backend API)

### 3.5 Admin WebUI Changes

```
webui/admin/
├── app/
│   └── billing/
│       ├── page.tsx                # Billing overview: total revenue, outstanding, credits issued
│       ├── customers/[id]/page.tsx # Per-customer billing: balance, transactions, manual credit
│       └── settings/page.tsx       # Configure gateways, tax rates, billing cycle
```

### 3.6 Hourly Billing Flow

```
                         ┌─────────────┐
                         │ NATS Cron   │  (every hour)
                         │ Scheduler   │
                         └──────┬──────┘
                                │ publishes "billing.hourly_sweep"
                                ▼
                    ┌───────────────────────┐
                    │ BillingHourlySweep    │
                    │ Task Handler          │
                    └───────────┬───────────┘
                                │
                    ┌───────────▼───────────┐
                    │ For each running VM:  │
                    │ 1. Get plan hourly    │
                    │    rate               │
                    │ 2. Deduct from        │
                    │    customer credits   │
                    │ 3. Record transaction │
                    │ 4. If balance <= 0:   │
                    │    → suspend VM       │
                    │    → notify customer  │
                    └───────────────────────┘
```

### 3.7 Payment Processing Flow (Stripe Example)

```
Customer WebUI                    Controller API                     Stripe
     │                                 │                               │
     │ POST /customer/billing/topup    │                               │
     │ {amount: 5000, gateway: stripe} │                               │
     │ ──────────────────────────────► │                               │
     │                                 │ stripe.PaymentIntents.Create  │
     │                                 │ ─────────────────────────────►│
     │                                 │ ◄─ client_secret              │
     │ ◄── {client_secret: pi_xxx}     │                               │
     │                                 │                               │
     │ stripe.confirmPayment(secret)   │                               │
     │ ────────────────────────────────┼──────────────────────────────►│
     │                                 │                               │
     │                                 │ POST /webhooks/stripe         │
     │                                 │ ◄─────────────────────────────│
     │                                 │ (payment_intent.succeeded)    │
     │                                 │                               │
     │                                 │ → Add credits to balance      │
     │                                 │ → Record transaction          │
     │                                 │ → Send confirmation email     │
     │                                 │                               │
     │ (WebSocket/polling: balance     │                               │
     │  updated)                       │                               │
```

---

## 4. WHMCS Coexistence Strategy

VirtueStack currently supports WHMCS for billing. The new built-in billing system must coexist:

| Mode | Description | Use Case |
|------|-------------|----------|
| **WHMCS-managed** | WHMCS handles all billing. VirtueStack credit system disabled. `UsageUpdate()` reports hourly costs to WHMCS. | Existing WHMCS customers |
| **Self-managed** | Built-in billing handles credits, payments, invoices. No WHMCS dependency. | Standalone VirtueStack deployments |
| **Hybrid** | WHMCS creates VMs; VirtueStack tracks hourly credits and reports back to WHMCS via `UsageUpdate()`. | Gradual migration |

**Configuration:** `BILLING_MODE` env var: `whmcs` (default, current behavior), `self`, or `hybrid`.

---

## 5. Payment Gateway Considerations

### Stripe
- **PCI compliance:** Use Stripe Elements (client-side tokenization) — card numbers never touch VirtueStack servers
- **Webhooks:** `payment_intent.succeeded`, `payment_intent.payment_failed`, `charge.refunded`
- **Recurring:** Optional Stripe Subscriptions for monthly plans (or handle in VirtueStack)
- **Go SDK:** `github.com/stripe/stripe-go/v82`

### PayPal
- **Integration:** PayPal JavaScript SDK (client-side) + REST API (server-side)
- **Webhooks:** `PAYMENT.CAPTURE.COMPLETED`, `PAYMENT.CAPTURE.DENIED`
- **Go SDK:** `github.com/plutov/paypal/v4` or direct REST API calls
- **Consideration:** PayPal disputes/chargebacks need handling (auto-suspend VM)

### Crypto
- **Self-hosted:** BTCPay Server (full control, no KYC, supports BTC/LTC/XMR)
- **Third-party:** NOWPayments, CoinGate, or Coinbase Commerce
- **Flow:** Generate payment address → display QR → poll for confirmation → credit on N confirmations
- **Volatility:** Convert to USD-equivalent at time of payment, credit in cents
- **Consideration:** Confirmation delays (10min–1hr for BTC) — show "pending" status

---

## 6. Security Considerations

| Concern | Mitigation |
|---------|------------|
| **PCI DSS** | Never store card numbers. Stripe Elements / PayPal vault handle tokenization. Only store provider token IDs. |
| **Payment webhooks** | Verify webhook signatures (Stripe: `stripe-signature` header; PayPal: webhook ID verification). |
| **Credit manipulation** | All credit operations go through `billing_service.go` with DB transactions. No direct balance updates from API. |
| **Negative balance** | Check balance before hourly deduction. Suspend VMs at configurable threshold (e.g., -$5.00 grace). |
| **Race conditions** | Use PostgreSQL `SELECT ... FOR UPDATE` on `customer_credits` row during deductions. |
| **Audit trail** | `billing_transactions` table is append-only (no UPDATE/DELETE). All mutations logged in `audit_logs`. |
| **Crypto address reuse** | Generate unique payment address per top-up. Never reuse addresses. |
| **Currency handling** | All amounts in integer cents (BIGINT). No floating-point. Round consistently (banker's rounding). |

---

## 7. Implementation Phases

### Phase 1: Database & Core Models (1–2 days)
- [ ] Create migration: `customer_credits`, `billing_transactions`, `invoices`, `invoice_line_items`, `payment_methods`
- [ ] Add RLS policies for customer isolation
- [ ] Add billing models in `internal/controller/models/billing.go`
- [ ] Add billing repository in `internal/controller/repository/billing_repo.go`
- [ ] Add `BILLING_MODE` configuration with `whmcs` default

### Phase 2: Billing Service & Hourly Engine (2–3 days)
- [ ] Implement `billing_service.go` — credit operations, balance checks, transaction recording
- [ ] Implement hourly billing task in `tasks/billing_hourly.go`
- [ ] Register billing tasks in `tasks/handlers.go`
- [ ] Add billing scheduler (NATS cron or ticker-based) for hourly sweeps
- [ ] Implement auto-suspend on zero/negative balance
- [ ] Implement low-balance email/notification warnings

### Phase 3: Payment Gateway Interface & Stripe (2–3 days)
- [ ] Define `PaymentGateway` interface in `internal/controller/payment/gateway.go`
- [ ] Implement Stripe gateway in `payment/stripe.go`
- [ ] Add Stripe webhook handler (signature verification, idempotency)
- [ ] Add payment method CRUD (customer-facing)
- [ ] Add credit top-up endpoint

### Phase 4: Customer API & WebUI (2–3 days)
- [ ] Add customer billing API endpoints (credits, transactions, invoices, payment methods, top-up)
- [ ] Add billing nav item to customer portal
- [ ] Build billing dashboard page (balance, recent charges, active VM costs)
- [ ] Build transaction history page
- [ ] Build Stripe Elements top-up form
- [ ] Build payment method management page

### Phase 5: Admin API & WebUI (1–2 days)
- [ ] Add admin billing endpoints (view/adjust customer credits, view transactions)
- [ ] Build admin billing overview page
- [ ] Build per-customer billing management page
- [ ] Add manual credit adjustment with audit logging

### Phase 6: PayPal Integration (1–2 days)
- [ ] Implement PayPal gateway in `payment/paypal.go`
- [ ] Add PayPal webhook handler
- [ ] Add PayPal button to top-up form (gateway selection)

### Phase 7: Crypto Integration (2–3 days)
- [ ] Choose provider (BTCPay Server recommended for self-hosting)
- [ ] Implement crypto gateway in `payment/crypto.go`
- [ ] Add crypto payment flow (address generation, QR display, confirmation polling)
- [ ] Handle confirmation delays and pending states

### Phase 8: Invoice Generation (1–2 days)
- [ ] Implement invoice service (monthly summarization of hourly charges)
- [ ] Add invoice generation task (monthly NATS cron)
- [ ] Add PDF generation (optional, can use HTML-to-PDF)
- [ ] Add invoice download endpoint

### Phase 9: WHMCS Sync (1 day)
- [ ] Update `UsageUpdate()` to report hourly costs when in hybrid mode
- [ ] Add credit sync endpoint for WHMCS provisioning API
- [ ] Document WHMCS configuration for each billing mode

### Phase 10: Testing & Hardening (2–3 days)
- [ ] Unit tests for billing service (credit operations, edge cases)
- [ ] Unit tests for payment gateway implementations (mocked provider APIs)
- [ ] Integration tests for hourly billing sweep
- [ ] E2E tests for customer billing flow
- [ ] Load test for concurrent hourly billing (1000+ VMs)
- [ ] Security review of payment webhook handling

**Estimated total: 15–24 days**

---

## 8. Decision Matrix

| Criterion | Separate Project | Separate Go Module | Integrated Module ✅ |
|-----------|:---:|:---:|:---:|
| Development effort | 3–4× | 1.5× | 1× |
| Data consistency | Eventual | Same DB | **ACID transactions** |
| Auth reuse | Duplicate | Shared | **Shared** |
| Deployment complexity | High | Medium | **Low** |
| Code boundary | Strong | Medium | Package-level |
| Independent scaling | Yes | No | No |
| Team independence | Yes | Partial | No |
| WHMCS integration | 2 APIs | 1 API | **1 API** |
| Matches existing patterns | No | Partial | **Yes** |
| Time to production | 2–3 months | 1.5–2 months | **3–5 weeks** |

---

## 9. Final Recommendation

**Build billing as an integrated module within the existing VirtueStack codebase.** Specifically:

1. **Backend:** New packages under `internal/controller/` (services, repository, models, tasks) + new `internal/controller/payment/` package for gateway abstractions.
2. **Frontend:** New pages under `webui/customer/app/billing/` and `webui/admin/app/billing/`, following existing patterns.
3. **Database:** New tables via standard migrations in `migrations/`.
4. **Payment gateways:** Abstracted behind a Go interface — Stripe first, then PayPal, then Crypto.

This approach:
- **Reuses** 100% of existing infrastructure (auth, middleware, DB pool, NATS, audit logging, RLS)
- **Follows** established patterns (every VirtueStack feature is built this way)
- **Avoids** distributed transaction complexity (credit deduction is atomic with VM operations)
- **Ships** in 3–5 weeks vs. 2–3 months for a separate service
- **Coexists** with WHMCS via configurable billing mode

The only scenario where a separate service makes sense is if billing needs to scale independently to thousands of concurrent payment webhook events — which is unlikely for a VPS hosting platform. If that need arises later, the `PaymentGateway` interface and package-level separation make extraction straightforward.
