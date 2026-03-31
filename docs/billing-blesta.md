# Blesta Billing Integration

## Current Status: Stub

The Blesta billing provider is registered as a **stub adapter**. It satisfies the
`BillingProvider` interface so the billing registry can reference `blesta` as a
valid provider name, but all operational methods return `ErrNotSupported`.

## What Works

| Method | Behavior |
|--------|----------|
| `Name()` | Returns `"blesta"` |
| `ValidateConfig()` | Checks `BLESTA_API_URL` + `BLESTA_API_KEY` are set |
| `OnVMCreated()` | No-op (returns nil) |
| `OnVMDeleted()` | No-op (returns nil) |
| `OnVMResized()` | No-op (returns nil) |
| `GetUserBillingStatus()` | Returns `"active"` (Blesta manages externally) |

## What Returns `ErrNotSupported`

These methods need full implementation for production Blesta support:

| Method | What It Should Do |
|--------|-------------------|
| `CreateUser()` | Call Blesta API to create a client |
| `SuspendForNonPayment()` | Call Blesta API to suspend services |
| `UnsuspendAfterPayment()` | Call Blesta API to unsuspend services |
| `GetBalance()` | Query Blesta API for client credit balance |
| `ProcessTopUp()` | Post credit to Blesta client account |
| `GetUsageHistory()` | Fetch Blesta invoice/transaction history |

## Configuration

| Environment Variable | Required | Description |
|---------------------|----------|-------------|
| `BILLING_BLESTA_ENABLED` | No | Enable Blesta provider (default: `false`) |
| `BILLING_BLESTA_PRIMARY` | No | Set as primary for new registrations (default: `false`) |
| `BLESTA_API_URL` | When enabled | Blesta API base URL (e.g., `https://blesta.example.com/api/`) |
| `BLESTA_API_KEY` | When enabled | Blesta API authentication key |

## Architecture

```
internal/controller/billing/blesta/
  adapter.go        # BillingProvider stub implementation
  adapter_test.go   # Tests
```

The adapter is registered conditionally in `internal/controller/dependencies.go`
when `BILLING_BLESTA_ENABLED=true`.

## Implementing Full Blesta Support

To implement full Blesta support:

1. **Add an HTTP client** to `adapter.go` that calls the Blesta API
   (use the SSRF-safe transport from `internal/shared/util/ssrf.go`)
2. **Implement `CreateUser`** — `POST /api/clients/add.json`
3. **Implement `SuspendForNonPayment`** — `POST /api/services/suspend.json`
4. **Implement `UnsuspendAfterPayment`** — `POST /api/services/unsuspend.json`
5. **Implement `GetBalance`** — `GET /api/clients/get_credits.json`
6. **Implement `ProcessTopUp`** — `POST /api/clients/add_credit.json`
7. **Implement `GetUsageHistory`** — `GET /api/invoices/get_list.json`
8. **Add webhook receiver** for Blesta → VirtueStack event notifications
9. **Write integration tests** against a Blesta sandbox instance

Each method should map to the corresponding Blesta API endpoint documented at
https://docs.blesta.com/display/dev/API

## Customer Assignment

Customers are assigned to Blesta billing via the `billing_provider` column:

```sql
UPDATE customers SET billing_provider = 'blesta' WHERE id = :customer_id;
```

Only admin endpoints can change `billing_provider`. When
`BILLING_BLESTA_PRIMARY=true`, new self-registered customers are automatically
assigned to Blesta.
