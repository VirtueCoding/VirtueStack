# Billing & Hourly Credit Feature — Architecture Decision Plan

> **Status:** DRAFT — For review, no code changes  
> **Date:** 2026-03-27  
> **Question:** Should billing/hourly credits with PayPal/Stripe/Crypto be a separate project, a separate module, or coded directly into the customer WebUI and controller?

---

## Executive Summary

**Recommendation: Build as an integrated module within the existing VirtueStack codebase** — not a separate project, and not a loosely-coupled microservice. The current modular-monolith architecture (single Go controller, shared PostgreSQL, NATS task queue) is a natural fit. Payment gateway integrations (Stripe, PayPal, Crypto) should be abstracted behind a Go interface so they can be swapped or extended without touching billing logic.

**Default mode is `whmcs`** — self-billing is **disabled by default**. When disabled, all billing API endpoints return `403 BILLING_DISABLED`, the customer WebUI hides all billing pages/navigation, and the admin billing settings page shows "Self-billing is disabled. Billing is managed by WHMCS." An admin must explicitly set `BILLING_MODE=self` (or `hybrid`) to activate the built-in billing system.

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
│   ├── middleware/
│   │   └── billing.go          # RequireSelfBilling() — returns 403 when BILLING_MODE=whmcs
│   ├── customer/
│   │   ├── billing.go          # GET /credits, GET /transactions, GET /invoices
│   │   └── payment_methods.go  # CRUD payment methods (Stripe tokens, not raw cards)
│   ├── admin/
│   │   ├── billing.go          # Manage customer credits, view all transactions
│   │   └── billing_settings.go # Configure billing rates, tax, gateways
│   └── provisioning/
│       └── billing.go          # Usage reporting for WHMCS sync
├── tasks/
│   ├── billing_hourly.go       # Hourly credit deduction task (only runs when billing enabled)
│   ├── billing_invoice.go      # Monthly invoice generation task
│   └── billing_webhook.go      # Payment gateway webhook processing
└── payment/                    # NEW: Payment gateway abstractions
    ├── gateway.go              # PaymentGateway interface
    ├── stripe.go               # Stripe v85 implementation (Payment Element + Checkout Sessions API)
    ├── paypal.go               # PayPal Orders v2 implementation
    └── crypto.go               # Crypto payment implementation (BTCPay/NOWPayments/etc.)
```

**Route registration (billing routes guarded by mode check):**

```go
// internal/controller/api/customer/routes.go
// All billing routes are wrapped with RequireSelfBilling middleware
if billingMode != "whmcs" {
    billing := customerGroup.Group("/billing")
    billing.Use(middleware.RequireSelfBilling(billingMode))
    {
        billing.GET("/credits", h.GetCredits)
        billing.GET("/transactions", h.ListTransactions)
        billing.POST("/topup", h.CreateTopUp)
        // ...
    }
}
// When BILLING_MODE=whmcs, these routes are never registered — 404 Not Found
```

### 3.2 Payment Gateway Interface

```go
// internal/controller/payment/gateway.go
type PaymentGateway interface {
    // CreateCustomer creates a customer record in the payment provider
    // Maps VirtueStack customer → Stripe cus_xxx / PayPal merchant_customer_id
    CreateCustomer(ctx context.Context, customer CustomerInfo) (providerID string, err error)

    // CreatePaymentSession initiates a payment session
    // Stripe: creates a Checkout Session, returns client_secret/session data for Payment Element
    // PayPal: creates an Order via Orders v2, returns order_id for PayPal Buttons
    // Crypto: creates an invoice/quote and returns address/QR/status metadata
    CreatePaymentSession(ctx context.Context, req PaymentRequest) (*PaymentSession, error)

    // CapturePayment finalizes the payment after customer approval
    // Stripe: handled automatically via webhook (primarily checkout.session.completed)
    // PayPal: captures order via POST /v2/checkout/orders/{id}/capture
    CapturePayment(ctx context.Context, providerRef string) (*PaymentResult, error)

    // GetPaymentStatus checks payment status by provider reference
    GetPaymentStatus(ctx context.Context, providerRef string) (*PaymentStatus, error)

    // ProcessWebhook validates signature and parses incoming webhook from provider
    // Stripe: verifies stripe-signature header using webhook.ConstructEvent()
    // PayPal: verifies via POST /v1/notifications/verify-webhook-signature
    ProcessWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error)

    // Refund issues a full or partial refund
    Refund(ctx context.Context, paymentRef string, amountCents int64) (*RefundResult, error)

    // Name returns the gateway identifier ("stripe", "paypal", "crypto")
    Name() string
}
```

This interface allows adding new payment providers without touching billing logic. Each gateway implementation is only instantiated when `BILLING_MODE != "whmcs"` and the gateway's credentials are configured.

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

> **All billing UI is conditionally rendered.** When `BILLING_MODE=whmcs` (default), none of the billing pages, components, or navigation items are visible. The WebUI checks a server-provided feature flag (`billing_enabled`) from `GET /customer/config`.

```
webui/customer/
├── app/
│   └── billing/
│       ├── layout.tsx              # RequireAuth + RequireBillingEnabled wrapper
│       ├── page.tsx                # Dashboard: credit balance, recent transactions, active VMs cost
│       ├── transactions/page.tsx   # Full transaction history with filters
│       ├── invoices/page.tsx       # Invoice list
│       ├── invoices/[id]/page.tsx  # Invoice detail + PDF download
│       ├── topup/page.tsx          # Add credits (Stripe Payment Element / PayPal Buttons / Crypto QR)
│       ├── topup/complete/page.tsx # Post-payment redirect page (Stripe return_url target)
│       └── payment-methods/page.tsx # Manage saved payment methods
├── components/
│   └── billing/
│       ├── CreditBalance.tsx       # Balance display widget (reusable in sidebar/header)
│       ├── TransactionTable.tsx    # Transaction list with pagination
│       ├── InvoiceTable.tsx        # Invoice list
│       ├── TopupForm.tsx           # Credit top-up form with gateway selection
│       ├── StripePayment.tsx       # Stripe Payment Element integration (@stripe/react-stripe-js)
│       ├── PayPalPayment.tsx       # PayPal Buttons integration (@paypal/react-paypal-js)
│       └── CryptoPayment.tsx       # Crypto payment flow (QR code, address, status polling)
├── lib/
│   ├── api-client.ts              # Add billingApi module (follows existing pattern)
│   └── nav-items.ts               # Conditionally add billing nav entry based on config
```

**New npm dependencies (only loaded when billing is enabled):**
- `@stripe/stripe-js` + `@stripe/react-stripe-js` — Stripe Payment Element (PCI-compliant tokenization)
- `@paypal/react-paypal-js` — PayPal Buttons SDK
- No crypto SDK needed client-side (QR code + status polling via backend API)

**Billing-disabled behavior (default):**
```typescript
// /app/billing/layout.tsx
export default function BillingLayout({ children }: { children: React.ReactNode }) {
  const { config } = useAppConfig();
  const router = useRouter();

  useEffect(() => {
    if (!config.billing_enabled) {
      router.replace("/vms"); // Redirect away from billing pages
    }
  }, [config.billing_enabled, router]);

  if (!config.billing_enabled) return null;
  return <RequireAuth>{children}</RequireAuth>;
}
```

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

### 3.7 Payment Processing Flow (Stripe Payment Element + Checkout Sessions API)

```
Customer WebUI                    Controller API                     Stripe
     │                                 │                               │
     │ POST /customer/billing/topup    │                               │
     │ {amount: 5000, gateway: stripe} │                               │
     │ ──────────────────────────────► │                               │
     │                                 │ sc.V1CheckoutSessions.Create()│
     │                                 │ (stripe-go/v85)               │
     │                                 │ ─────────────────────────────►│
     │                                 │ ◄─ client_secret/session data │
     │ ◄── {client_secret: "..."}     │                               │
     │                                 │                               │
     │ <PaymentElement /> renders      │                               │
     │ stripe.confirmPayment({        │                               │
     │   elements,                    │                               │
     │   confirmParams: {             │                               │
     │     return_url: ".../complete" │                               │
     │   }                            │                               │
     │ })                             │                               │
     │ ────────────────────────────────┼──────────────────────────────►│
     │                                 │                               │
     │                                 │ POST /webhooks/stripe         │
     │                                 │ (Stripe-Signature header)     │
     │                                 │ ◄─────────────────────────────│
     │                                 │ webhook.ConstructEvent() ✓    │
     │                                 │ event: checkout.session.      │
     │                                 │        completed              │
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

VirtueStack currently supports WHMCS for billing. **WHMCS mode is the default; self-billing is disabled out of the box.** The built-in billing system must be explicitly enabled by an administrator.

### 4.1 Billing Modes

| Mode | Default | Description | Use Case |
|------|---------|-------------|----------|
| **`whmcs`** | ✅ Yes | WHMCS handles all billing. Self-billing disabled. All billing endpoints return `403 BILLING_DISABLED`. Customer WebUI billing section hidden. `UsageUpdate()` reports hourly costs to WHMCS. | Existing WHMCS customers (majority of deployments) |
| **`self`** | No | Built-in billing handles credits, payments, invoices. No WHMCS dependency. All billing endpoints active. Customer WebUI billing section visible. | Standalone VirtueStack deployments without WHMCS |
| **`hybrid`** | No | WHMCS creates VMs; VirtueStack tracks hourly credits and reports back to WHMCS via `UsageUpdate()`. Billing endpoints active for credit top-up only. | Gradual migration from WHMCS |

**Configuration:** `BILLING_MODE` env var — defaults to `whmcs` (current behavior, self-billing completely disabled).

### 4.2 Self-Billing Disabled Behavior (Default: `BILLING_MODE=whmcs`)

When self-billing is disabled, the system enforces the following restrictions at every layer:

#### Backend (Controller API)

All billing-related endpoints are guarded by a `RequireSelfBilling()` middleware that checks `BILLING_MODE`:

```go
// internal/controller/api/middleware/billing.go
func RequireSelfBilling(billingMode string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if billingMode == "whmcs" {
            middleware.RespondWithError(c, http.StatusForbidden,
                "BILLING_DISABLED",
                "Self-billing is disabled. Billing is managed by WHMCS.")
            return
        }
        c.Next()
    }
}
```

**Affected endpoints (all return `403 BILLING_DISABLED` when `BILLING_MODE=whmcs`):**

| Tier | Endpoints | Behavior |
|------|-----------|----------|
| Customer API | `GET /customer/billing/*` | 403 Forbidden |
| Customer API | `POST /customer/billing/*` | 403 Forbidden |
| Customer API | `GET /customer/payment-methods/*` | 403 Forbidden |
| Customer API | `POST /customer/payment-methods/*` | 403 Forbidden |
| Admin API | `GET /admin/billing/*` | 403 Forbidden |
| Admin API | `POST /admin/billing/*` | 403 Forbidden |
| Webhook endpoints | `POST /webhooks/stripe`, `POST /webhooks/paypal`, `POST /webhooks/crypto` | 403 Forbidden |

**Unaffected endpoints (always active regardless of mode):**

| Tier | Endpoints | Reason |
|------|-----------|--------|
| Admin API | `GET/POST/PUT/DELETE /admin/plans` | Plan pricing fields are informational (used by WHMCS) |
| Provisioning API | `GET /provisioning/plans` | WHMCS reads plan pricing |
| Provisioning API | All VM lifecycle endpoints | WHMCS provisions VMs |

#### Customer WebUI

The billing navigation item and all billing pages are **conditionally rendered** based on a server-provided feature flag:

```typescript
// Customer WebUI checks billing mode via /customer/profile or a dedicated config endpoint
// GET /customer/config returns { billing_enabled: false } when BILLING_MODE=whmcs

// In nav-items.ts — billing nav item only shown when billing_enabled=true
export function getNavItems(config: AppConfig) {
  const items = [
    { href: "/vms", label: "My VMs", icon: Monitor },
    { href: "/settings", label: "Settings", icon: Settings },
  ];
  if (config.billing_enabled) {
    items.splice(1, 0, { href: "/billing", label: "Billing", icon: CreditCard });
  }
  return items;
}

// In billing pages — redirect to /vms if billing is disabled
// /app/billing/layout.tsx checks config.billing_enabled, redirects if false
```

**When `BILLING_MODE=whmcs`:**
- ❌ No "Billing" item in sidebar navigation
- ❌ `/billing/*` routes redirect to `/vms`
- ❌ No credit balance widget in header/sidebar
- ❌ No payment method management
- ✅ VMs page, Settings page, Console — all work normally

#### Admin WebUI

The admin billing management pages are similarly hidden:

- ❌ No "Billing" section in admin sidebar
- ❌ `/billing/*` admin routes show "Self-billing is disabled" message
- ✅ Plan management (including pricing fields) remains visible — pricing is informational for WHMCS
- ✅ Admin can change `BILLING_MODE` via Settings page to enable self-billing

### 4.3 Enabling Self-Billing

To activate self-billing, an administrator must:

1. Set `BILLING_MODE=self` (or `hybrid`) in environment variables or via Admin Settings API
2. Configure at least one payment gateway (Stripe/PayPal/Crypto credentials)
3. Restart the controller (or apply settings via hot-reload if supported)

The billing database tables are always created by migrations regardless of mode — they just sit empty when billing is disabled. This avoids migration ordering issues when switching modes.

---

## 5. Payment Gateway Integration Details (Latest Documentation, March 2026)

> **Note:** All payment gateways are only active when `BILLING_MODE=self` or `BILLING_MODE=hybrid`. When `BILLING_MODE=whmcs` (default), gateway code is never invoked and webhook endpoints return `403`.

### 5.1 Stripe Integration

**Documentation:** https://docs.stripe.com/api (API version `2026-03-25.dahlia`)

#### Recommendation for VirtueStack

- **Default UI:** Use the **Payment Element** inside the customer portal so credit top-ups stay in the VirtueStack UI.
- **Default server-side API:** Prefer the **Checkout Sessions API** for most flows; Stripe explicitly recommends it over Payment Intents for most new integrations.
- **Fallback UX:** Keep **hosted Checkout redirect** as a lower-effort fallback or MVP escape hatch if embedded checkout causes rollout friction.
- **Avoid for v1:** Do not build around the legacy Card Element.
- **Use Payment Intents only if needed later:** Only drop to the lower-level Payment Intents API if VirtueStack eventually needs to fully own tax, discount, shipping, or currency-conversion logic that Checkout Sessions does not cover cleanly.

#### Go SDK — `github.com/stripe/stripe-go/v85`

The official Stripe Go SDK v85 uses the new `stripe.Client` pattern (introduced in v82.1). The legacy `client.API` and package-level functions are deprecated.

```go
import "github.com/stripe/stripe-go/v85"

// Initialize client (NEVER use global stripe.Key — use per-request client)
sc := stripe.NewClient(os.Getenv("STRIPE_SECRET_KEY"))

// Preferred path: create a Checkout Session for the top-up flow
// and return the client secret / session data needed by the Payment Element.
//
// Pseudocode only — exact parameters depend on whether VirtueStack uses
// embedded/custom UI mode or falls back to hosted redirect mode.
session, err := sc.V1CheckoutSessions.Create(ctx, params)
// Return session client/session data to frontend
```

#### Frontend — Stripe Payment Element (recommended over Card Element)

Stripe recommends the **Payment Element** (not the legacy Card Element) for all new integrations. Payment Element supports 100+ payment methods, Apple Pay, Google Pay, Link, and auto-handles 3D Secure / SCA.

**NPM packages:**
- `@stripe/stripe-js` — Stripe.js loader
- `@stripe/react-stripe-js` — React bindings for Stripe Elements

```tsx
// webui/customer/components/billing/StripePayment.tsx
import { loadStripe } from '@stripe/stripe-js';
import { Elements, PaymentElement, useStripe, useElements } from '@stripe/react-stripe-js';

const stripePromise = loadStripe(process.env.NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY!);

function CheckoutForm({ clientSecret }: { clientSecret: string }) {
  const stripe = useStripe();
  const elements = useElements();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!stripe || !elements) return;

    const { error } = await stripe.confirmPayment({
      elements,
      confirmParams: {
        return_url: `${window.location.origin}/billing/topup/complete`,
      },
    });
    if (error) { /* handle error */ }
  };

  return (
    <form onSubmit={handleSubmit}>
      <PaymentElement />
      <button type="submit" disabled={!stripe}>Pay</button>
    </form>
  );
}

// Wrap in Elements provider with clientSecret from server
<Elements stripe={stripePromise} options={{ clientSecret }}>
  <CheckoutForm clientSecret={clientSecret} />
</Elements>
```

**Fallback — hosted Stripe Checkout redirect (lower effort, lower UX control):**
If VirtueStack wants the fastest possible rollout, Checkout Sessions can also redirect the customer to a Stripe-hosted payment page instead of rendering the Payment Element in-portal:

```go
// Server-side: create Checkout Session
params := &stripe.CheckoutSessionCreateParams{
    LineItems: []*stripe.CheckoutSessionLineItemParams{{
        PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
            Currency:    stripe.String("usd"),
            UnitAmount:  stripe.Int64(5000), // $50.00
            ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
                Name: stripe.String("VirtueStack Credit Top-Up"),
            },
        },
        Quantity: stripe.Int64(1),
    }},
    Mode:       stripe.String("payment"),
    SuccessURL: stripe.String("https://portal.example.com/billing?topup=success"),
}
session, err := sc.V1CheckoutSessions.Create(ctx, params)
// Redirect customer to session.URL
```

#### Webhook Handling

**Key events to subscribe to:**

| Event | Trigger | Action |
|-------|---------|--------|
| `payment_intent.succeeded` | Payment confirmed | Credit customer balance, record transaction |
| `payment_intent.payment_failed` | Payment declined/failed | Notify customer, log failure |
| `charge.refunded` | Refund processed | Deduct from customer balance, record transaction |
| `charge.dispute.created` | Chargeback initiated | Auto-suspend VMs, notify admin, record transaction |
| `checkout.session.completed` | Checkout Session payment done | Credit customer (if using Checkout Sessions) |
| `checkout.session.async_payment_succeeded` | Delayed payment method (ACH) succeeded | Credit customer |
| `checkout.session.async_payment_failed` | Delayed payment method failed | Notify customer |

**Implementation note:** if VirtueStack standardizes on the Checkout Sessions API, `checkout.session.completed` should be treated as the primary fulfillment signal. `payment_intent.*` events are still useful for reconciliation and failure visibility, but they should not be the only signal wired into the credit ledger.

**Webhook signature verification (required for security):**

```go
import "github.com/stripe/stripe-go/v85/webhook"

func handleStripeWebhook(c *gin.Context) {
    payload, _ := io.ReadAll(c.Request.Body)
    sigHeader := c.GetHeader("Stripe-Signature")
    endpointSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")

    event, err := webhook.ConstructEvent(payload, sigHeader, endpointSecret)
    if err != nil {
        middleware.RespondWithError(c, http.StatusBadRequest,
            "WEBHOOK_SIGNATURE_INVALID", "Invalid webhook signature")
        return
    }

    switch event.Type {
    case "payment_intent.succeeded":
        // Credit customer balance
    case "charge.dispute.created":
        // Auto-suspend VMs, notify admin
    }
    c.JSON(http.StatusOK, gin.H{"received": true})
}
```

### 5.2 PayPal Integration

**Documentation:** https://developer.paypal.com/docs/api/orders/v2/, https://developer.paypal.com/docs/checkout/

#### Recommendation for VirtueStack

- **Use PayPal Checkout (standard buttons) for v1.** It covers the primary PayPal/Venmo/Pay Later use cases with the least code and the least PCI surface area.
- **Do not use Expanded Checkout / Enterprise Checkout in v1.** Stripe already covers card entry better via the Payment Element; duplicating advanced card flows in PayPal adds complexity without clear product value.
- **Recommended role split:** Stripe handles cards, Apple Pay, Google Pay, Link, and local methods; PayPal handles PayPal wallet, Venmo, and Pay Later.

#### Go SDK — `github.com/plutov/paypal/v4`

Community-maintained Go client for PayPal REST APIs. Supports Orders v2, Webhooks, Vault, Payouts, and Invoicing.

```go
import "github.com/plutov/paypal/v4"

// Initialize client
c, err := paypal.NewClient(
    os.Getenv("PAYPAL_CLIENT_ID"),
    os.Getenv("PAYPAL_CLIENT_SECRET"),
    paypal.APIBaseLive, // or paypal.APIBaseSandBox for testing
)

// Create Order (server-side, called when customer clicks PayPal button)
order, err := c.CreateOrder(ctx, paypal.OrderIntentCapture,
    []paypal.PurchaseUnitRequest{{
        Amount: &paypal.PurchaseUnitAmount{
            Currency: "USD",
            Value:    "50.00",
        },
        Description: "VirtueStack Credit Top-Up",
    }},
    nil, // payment source
    &paypal.ApplicationContext{
        ReturnURL: "https://portal.example.com/billing/topup/complete",
        CancelURL: "https://portal.example.com/billing/topup",
    },
)
// Return order.ID to frontend

// Capture Order (server-side, after customer approves in PayPal popup)
capture, err := c.CaptureOrder(ctx, orderID, paypal.CaptureOrderRequest{})
```

#### Frontend — PayPal JavaScript SDK

PayPal offers 4 integration tiers. For VirtueStack, **PayPal Checkout (Standard)** is recommended — it provides PayPal buttons with minimal frontend code.

**NPM package:** `@paypal/react-paypal-js`

```tsx
// webui/customer/components/billing/PayPalPayment.tsx
import { PayPalScriptProvider, PayPalButtons } from "@paypal/react-paypal-js";

function PayPalTopup({ amount }: { amount: number }) {
  return (
    <PayPalScriptProvider options={{
      clientId: process.env.NEXT_PUBLIC_PAYPAL_CLIENT_ID!,
      currency: "USD",
      components: "buttons",
    }}>
      <PayPalButtons
        style={{ layout: "vertical", color: "blue", shape: "rect" }}
        createOrder={async () => {
          // Call VirtueStack backend to create PayPal order
          const res = await billingApi.createPayPalOrder(amount);
          return res.order_id; // PayPal order ID
        }}
        onApprove={async (data) => {
          // Call VirtueStack backend to capture payment
          await billingApi.capturePayPalOrder(data.orderID);
          // Redirect to success page or refresh balance
        }}
        onError={(err) => {
          console.error("PayPal error:", err);
        }}
      />
    </PayPalScriptProvider>
  );
}
```

#### PayPal Integration Flow (Orders v2 API)

```
Customer WebUI                    Controller API                     PayPal
     │                                 │                               │
     │ Customer clicks PayPal button   │                               │
     │                                 │                               │
     │ POST /customer/billing/topup    │                               │
     │ {amount: 5000, gateway: paypal} │                               │
     │ ──────────────────────────────► │                               │
     │                                 │ POST /v2/checkout/orders      │
     │                                 │ (OrderIntentCapture)          │
     │                                 │ ─────────────────────────────►│
     │                                 │ ◄─ { id: "ORDER_ID" }        │
     │ ◄── { order_id: "ORDER_ID" }    │                               │
     │                                 │                               │
     │ PayPal popup → customer approves│                               │
     │ ────────────────────────────────┼──────────────────────────────►│
     │                                 │                               │
     │ onApprove callback fires        │                               │
     │ POST /customer/billing/         │                               │
     │       paypal/capture            │                               │
     │ {order_id: "ORDER_ID"}          │                               │
     │ ──────────────────────────────► │                               │
     │                                 │ POST /v2/checkout/orders/     │
     │                                 │       {id}/capture            │
     │                                 │ ─────────────────────────────►│
     │                                 │ ◄─ { status: "COMPLETED" }   │
     │                                 │                               │
     │                                 │ → Add credits to balance      │
     │                                 │ → Record transaction          │
     │                                 │ → Send confirmation email     │
     │ ◄── { balance: 5000 }           │                               │
```

#### Webhook Handling

PayPal webhooks use the Webhooks Management API v1. VirtueStack should subscribe to:

| Event Type | Trigger | Action |
|-----------|---------|--------|
| `CHECKOUT.ORDER.APPROVED` | Buyer approves order | Begin capture flow |
| `PAYMENT.CAPTURE.COMPLETED` | Payment captured | Credit customer (backup to capture API call) |
| `PAYMENT.CAPTURE.DENIED` | Capture denied | Notify customer, log failure |
| `PAYMENT.CAPTURE.REFUNDED` | Refund processed | Deduct from balance, record transaction |
| `CUSTOMER.DISPUTE.CREATED` | Dispute/chargeback | Auto-suspend VMs, notify admin |

**Webhook signature verification:** PayPal verifies webhooks via `POST /v1/notifications/verify-webhook-signature` (server-to-server call, not local verification like Stripe).

```go
// PayPal webhook verification is done server-side via API call
verifyReq := paypal.VerifyWebhookSignatureRequest{
    WebhookID:       os.Getenv("PAYPAL_WEBHOOK_ID"),
    TransmissionID:  c.GetHeader("PAYPAL-TRANSMISSION-ID"),
    TransmissionTime: c.GetHeader("PAYPAL-TRANSMISSION-TIME"),
    CertURL:         c.GetHeader("PAYPAL-CERT-URL"),
    AuthAlgo:        c.GetHeader("PAYPAL-AUTH-ALGO"),
    TransmissionSig: c.GetHeader("PAYPAL-TRANSMISSION-SIG"),
    WebhookEvent:    webhookEventBody,
}
result, err := paypalClient.VerifyWebhookSignature(ctx, verifyReq)
// result.VerificationStatus == "SUCCESS"
```

### 5.3 Crypto Integration

> Crypto integration is the lowest priority. Implement after Stripe and PayPal are stable.

- **BTCPay Server is not BTC-only, but it is Bitcoin-first.** Core BTCPay focuses on BTC; some altcoins are available via community-maintained integrations/plugins (for example LTC, DOGE, XMR plugin, and USDt on Tron plugin). It is **not** a native ERC-20/BEP-20 solution.
- **If ERC-20/BEP-20 stablecoins are required, use a separate provider.** BTCPay should be treated as the self-hosted BTC/LTC/XMR path, not the default stablecoin strategy.
- **Recommended third-party options:** NOWPayments or CoinGate
- **Flow:** Generate payment address → display QR → poll for confirmation → credit on N confirmations
- **Volatility:** Convert to USD-equivalent at time of payment, credit in cents
- **Consideration:** Confirmation delays (10min–1hr for BTC) — show "pending" status in UI
- **No client SDK needed:** QR code rendering + status polling via VirtueStack backend API

#### Crypto provider recommendation matrix

| Provider | Best fit | Strengths | Limits / Cautions |
|----------|----------|-----------|-------------------|
| **BTCPay Server** | Self-hosted BTC-first deployments | Full control, no processor fees, strong fit for BTC/LTC/XMR-style payments | Altcoin support is limited and community-maintained; not a native ERC-20/BEP-20 strategy |
| **NOWPayments** | Broad stablecoin / multi-chain support | Very wide asset coverage, supports common stablecoin rails, simple API | Third-party dependency, merchant compliance/KYC, processor fees |
| **CoinGate** | Crypto with stronger fiat/stablecoin settlement options | Hosted checkout, reporting, conversion/settlement options | Third-party dependency, processor fees, less self-sovereign than BTCPay |

#### Launch recommendation for VirtueStack crypto

1. **Do not start with every network.** Limit the first release to a short, supportable list.
2. **Suggested launch set:** BTC + one or two stablecoin rails with low fees and strong wallet support (for example TRC20 or Polygon), rather than Ethereum mainnet first.
3. **Treat asset and chain as separate identifiers.** `USDT-TRC20`, `USDT-ERC20`, and `USDT-BEP20` must never be modeled as a single “USDT” payment method.
4. **If self-hosting is a product requirement:** offer BTCPay as an optional admin-selectable provider, not the only crypto path.

### 5.4 SDK Version Summary

| Gateway | Server SDK (Go) | Client SDK (npm) | API Version |
|---------|----------------|-------------------|-------------|
| **Stripe** | `github.com/stripe/stripe-go/v85` | `@stripe/stripe-js` + `@stripe/react-stripe-js` | `2026-03-25.dahlia` |
| **PayPal** | `github.com/plutov/paypal/v4` | `@paypal/react-paypal-js` | Orders v2 + Webhooks v1 |
| **Crypto** | BTCPay Server API (HTTP) or provider SDK / REST wrapper | None (QR code + polling) | Provider-dependent |

---

## 6. Security Considerations

| Concern | Mitigation |
|---------|------------|
| **PCI DSS** | Never store card numbers. Stripe Payment Element / PayPal Vault handle tokenization client-side. Only store provider token IDs (`pm_xxx`, PayPal vault ID). VirtueStack servers never see raw card data. |
| **Stripe webhook verification** | Verify `Stripe-Signature` header using `webhook.ConstructEvent()` from stripe-go/v85. Reject requests with invalid signatures. Use endpoint-specific secrets (`whsec_xxx`). |
| **PayPal webhook verification** | Verify via server-to-server call: `POST /v1/notifications/verify-webhook-signature`. Validate `PAYPAL-TRANSMISSION-SIG`, `PAYPAL-CERT-URL`, `PAYPAL-AUTH-ALGO` headers. |
| **Billing mode enforcement** | When `BILLING_MODE=whmcs` (default): all billing endpoints return 403, webhook endpoints return 403, billing tasks don't run, customer WebUI hides billing section. No credit/payment code paths are reachable. |
| **Credit manipulation** | All credit operations go through `billing_service.go` with DB transactions. No direct balance updates from API. Balance changes are always paired with a transaction record. |
| **Negative balance** | Check balance before hourly deduction. Suspend VMs at configurable threshold (e.g., -$5.00 grace). |
| **Race conditions** | Use PostgreSQL `SELECT ... FOR UPDATE` on `customer_credits` row during deductions. Stripe PaymentIntents and PayPal Orders provide built-in idempotency. |
| **Audit trail** | `billing_transactions` table is append-only (no UPDATE/DELETE). All mutations logged in `audit_logs`. Payment gateway references stored for reconciliation. |
| **Crypto address reuse** | Generate unique payment address per top-up. Never reuse addresses. |
| **Webhook replay / double-credit risk** | Store processed webhook event IDs with a unique constraint. Credit top-ups must be idempotent by provider event ID and provider payment/order/session ID. |
| **Crypto underpayment / overpayment** | Define quote expiry, acceptable tolerance, minimum confirmations, and a manual-review path before automatically crediting mismatched amounts. |
| **Token symbol ambiguity** | Persist asset and network separately (`USDT` + `TRC20`, not just `USDT`) to prevent customers sending funds on the wrong chain. |
| **Currency handling** | All amounts in integer cents (BIGINT). No floating-point. Round consistently (banker's rounding). |
| **Chargeback/dispute handling** | Auto-suspend customer VMs on `charge.dispute.created` (Stripe) or `CUSTOMER.DISPUTE.CREATED` (PayPal). Notify admin immediately. Record dispute in billing transactions. |
| **3D Secure / SCA** | Stripe Payment Element handles SCA automatically. PayPal handles authentication via its checkout flow. No additional server-side work needed. |

---

## 7. Implementation Phases

> **Guiding principle:** WHMCS mode is default. Self-billing is opt-in. Every billing feature must check `BILLING_MODE` before executing.

### Phase 1: Database, Core Models & Billing Mode Guard (1–2 days)
- [ ] Create migration: `customer_credits`, `billing_transactions`, `invoices`, `invoice_line_items`, `payment_methods` (tables created regardless of mode — they sit empty when billing disabled)
- [ ] Add RLS policies for customer isolation
- [ ] Add billing models in `internal/controller/models/billing.go`
- [ ] Add billing repository in `internal/controller/repository/billing_repo.go`
- [ ] Add `BILLING_MODE` configuration with **`whmcs` default** — parsed in `internal/shared/config/`
- [ ] Implement `RequireSelfBilling()` middleware in `internal/controller/api/middleware/billing.go`
- [ ] Add `GET /customer/config` endpoint returning `{ billing_enabled: bool }` feature flag
- [ ] Ensure all billing routes are **not registered** when `BILLING_MODE=whmcs`

### Phase 2: Billing Service & Hourly Engine (2–3 days)
- [ ] Implement `billing_service.go` — credit operations, balance checks, transaction recording
- [ ] Implement hourly billing task in `tasks/billing_hourly.go` (only schedules when billing enabled)
- [ ] Register billing tasks in `tasks/handlers.go` (conditional on billing mode)
- [ ] Add billing scheduler (NATS cron or ticker-based) for hourly sweeps
- [ ] Implement auto-suspend on zero/negative balance
- [ ] Implement low-balance email/notification warnings

### Phase 3: Payment Gateway Interface & Stripe (2–3 days)
- [ ] Define `PaymentGateway` interface in `internal/controller/payment/gateway.go`
- [ ] Implement Stripe gateway using `stripe-go/v85` — **Payment Element UI with Checkout Sessions API as the default path**
- [ ] Add Stripe webhook handler with `webhook.ConstructEvent()` signature verification
- [ ] Subscribe to: `payment_intent.succeeded`, `payment_intent.payment_failed`, `charge.refunded`, `charge.dispute.created`
- [ ] Add payment method CRUD using Stripe Payment Methods API (tokenized `pm_xxx` references only)
- [ ] Add credit top-up endpoint returning `client_secret` for frontend Payment Element
- [ ] Guard all webhook endpoints with `RequireSelfBilling()` middleware

### Phase 4: Customer API & WebUI (2–3 days)
- [ ] Add customer billing API endpoints (credits, transactions, invoices, payment methods, top-up) — all behind `RequireSelfBilling()`
- [ ] Add `GET /customer/config` returning `{ billing_enabled }` feature flag
- [ ] Add conditional billing nav item to customer portal (`nav-items.ts` checks `billing_enabled`)
- [ ] Build billing layout with `RequireBillingEnabled` guard (redirects to `/vms` when disabled)
- [ ] Build billing dashboard page (balance, recent charges, active VMs cost)
- [ ] Build transaction history page
- [ ] Build Stripe Payment Element top-up form (`@stripe/react-stripe-js`, Payment Element)
- [ ] Build payment method management page
- [ ] **Verify billing pages are invisible when `BILLING_MODE=whmcs`**

### Phase 5: Admin API & WebUI (1–2 days)
- [ ] Add admin billing endpoints (view/adjust customer credits, view transactions)
- [ ] Build admin billing overview page
- [ ] Build per-customer billing management page
- [ ] Add manual credit adjustment with audit logging

### Phase 6: PayPal Integration (1–2 days)
- [ ] Implement PayPal gateway using `plutov/paypal/v4` — `CreateOrder()`, `CaptureOrder()` (Orders v2 API)
- [ ] Add PayPal webhook handler with `VerifyWebhookSignature()` (server-to-server verification)
- [ ] Subscribe to: `PAYMENT.CAPTURE.COMPLETED`, `PAYMENT.CAPTURE.DENIED`, `PAYMENT.CAPTURE.REFUNDED`, `CUSTOMER.DISPUTE.CREATED`
- [ ] Add PayPal Buttons to top-up form using `@paypal/react-paypal-js` (`PayPalButtons` component) — **do not add Expanded Checkout card fields in v1**
- [ ] Implement PayPal capture endpoint (called from `onApprove` callback)

### Phase 7: Crypto Integration (2–3 days)
- [ ] Choose provider strategy: BTCPay Server for optional self-hosted BTC-first deployments, NOWPayments/CoinGate for ERC-20/BEP-20 stablecoin acceptance
- [ ] Implement crypto gateway in `payment/crypto.go`
- [ ] Add crypto payment flow (address generation, QR display, confirmation polling)
- [ ] Launch with a small, explicit network list (for example BTC + one or two low-fee stablecoin rails), not every supported chain
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
- [ ] **Unit tests for `RequireSelfBilling()` middleware — verify 403 when `BILLING_MODE=whmcs`**
- [ ] **E2E test: verify billing nav/pages are hidden when `BILLING_MODE=whmcs` (default)**
- [ ] **E2E test: verify billing nav/pages appear when `BILLING_MODE=self`**
- [ ] Integration tests for hourly billing sweep
- [ ] E2E tests for customer billing flow (Stripe test cards, PayPal sandbox)
- [ ] Load test for concurrent hourly billing (1000+ VMs)
- [ ] Security review of payment webhook handling
- [ ] Stripe test cards: `4242 4242 4242 4242` (success), `4000 0025 0000 3155` (3DS required), `4000 0000 0000 9995` (declined)
- [ ] PayPal sandbox accounts for end-to-end payment testing

**Estimated total: 16–26 days**

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

**Build billing as an integrated module within the existing VirtueStack codebase, with WHMCS mode as the default and self-billing disabled out of the box.** Specifically:

1. **Default mode:** `BILLING_MODE=whmcs` — self-billing is completely disabled. All billing endpoints return 403, customer WebUI hides billing section, hourly billing tasks don't run. Zero impact on existing WHMCS-managed deployments.
2. **Backend:** New packages under `internal/controller/` (services, repository, models, tasks) + new `internal/controller/payment/` package for gateway abstractions. All billing routes guarded by `RequireSelfBilling()` middleware.
3. **Frontend:** New pages under `webui/customer/app/billing/` and `webui/admin/app/billing/`, conditionally rendered based on `billing_enabled` feature flag from server. When disabled, billing nav items and pages are invisible.
4. **Database:** New tables via standard migrations in `migrations/` — created regardless of mode (empty when billing disabled, avoids migration issues on mode switch).
5. **Payment gateways:** Abstracted behind a Go interface — Stripe v85 first (**Payment Element + Checkout Sessions API**), then PayPal (Orders v2 + standard buttons), then Crypto (BTCPay for optional self-hosted BTC-first deployments, plus a separate provider if ERC-20/BEP-20 stablecoins are required).

This approach:
- **Preserves** existing WHMCS-managed deployments with zero changes (default mode)
- **Reuses** 100% of existing infrastructure (auth, middleware, DB pool, NATS, audit logging, RLS)
- **Follows** established patterns (every VirtueStack feature is built this way)
- **Avoids** distributed transaction complexity (credit deduction is atomic with VM operations)
- **Ships** in 3–5 weeks vs. 2–3 months for a separate service
- **Coexists** with WHMCS via configurable billing mode — admin explicitly opts in to self-billing

The only scenario where a separate service makes sense is if billing needs to scale independently to thousands of concurrent payment webhook events — which is unlikely for a VPS hosting platform. If that need arises later, the `PaymentGateway` interface and package-level separation make extraction straightforward.
