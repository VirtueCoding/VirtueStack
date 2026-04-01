# Blesta Module, API Neutralization & Documentation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Neutralize the Provisioning API to be billing-system-agnostic, create a fully functional Blesta PHP server module (mirroring the WHMCS module), update the Go Blesta adapter to a proper no-op, and update all project documentation for the entire billing branch.

**Architecture:** The Provisioning API currently uses WHMCS-specific field names (`whmcs_service_id`, `whmcs_client_id`). These will be renamed to generic `external_service_id` and `external_client_id` with a new `billing_provider` request field. Both the WHMCS and Blesta PHP modules call VirtueStack's Provisioning REST API — VirtueStack never calls Blesta/WHMCS. The Go-side Blesta adapter is a no-op (like WHMCS) since billing is managed externally. The Blesta PHP module follows the same patterns as the WHMCS module: create VMs, suspend/unsuspend, terminate, resize, SSO, webhooks.

**Tech Stack:** Go (API neutralization), PHP 8.1+ (Blesta module), Blesta Module SDK (`Module` base class, `ModuleFields`, `Loader`, `View`), cURL HTTP client, HMAC-SHA256 webhook verification

**Build/test commands:**
- Go build: `export PATH="/home/hiron/go/bin:/home/hiron/gopath/bin:$PATH" && go build -o /dev/null ./cmd/controller/`
- Go test: `go test ./internal/controller/... ./internal/shared/...`
- PHP syntax: `find modules/blesta -name '*.php' -exec php -l {} \;`

**Branch:** `feat/billing-system` (continue existing work)

---

## File Structure

### New Files
- `migrations/000080_neutralize_billing_fields.up.sql` — Add external_client_id, external_service_id columns
- `migrations/000080_neutralize_billing_fields.down.sql` — Rollback
- `modules/blesta/virtuestack/config.json` — Blesta module metadata
- `modules/blesta/virtuestack/virtuestack.php` — Main Blesta module class
- `modules/blesta/virtuestack/lib/ApiClient.php` — HTTP client for Provisioning API
- `modules/blesta/virtuestack/lib/VirtueStackHelper.php` — Utility functions
- `modules/blesta/virtuestack/language/en_us/virtuestack.php` — Language strings
- `modules/blesta/virtuestack/views/default/add_row.pdt` — Server config form
- `modules/blesta/virtuestack/views/default/edit_row.pdt` — Server edit form
- `modules/blesta/virtuestack/views/default/manage.pdt` — Package config form
- `modules/blesta/virtuestack/views/default/tab_client_service.pdt` — Client service view
- `modules/blesta/virtuestack/views/default/tab_admin_service.pdt` — Admin service view
- `modules/blesta/virtuestack/views/default/tab_client_console.pdt` — VNC/Serial console
- `modules/blesta/virtuestack/webhook.php` — Webhook receiver endpoint

### Modified Files
- `internal/controller/models/vm.go` — Rename WHMCSServiceID → ExternalServiceID
- `internal/controller/models/customer.go` — Rename WHMCSClientID → ExternalClientID
- `internal/controller/repository/vm_repo.go` — Use external_service_id column
- `internal/controller/repository/customer_repo.go` — Use external_client_id column
- `internal/controller/api/provisioning/handler.go` — Neutral comments + field names
- `internal/controller/api/provisioning/customers.go` — external_client_id + billing_provider
- `internal/controller/api/provisioning/vms.go` — external_service_id
- `internal/controller/api/provisioning/status.go` — Rename handler method
- `internal/controller/api/provisioning/sso.go` — external_service_id
- `internal/controller/api/provisioning/routes.go` — Update comments
- `internal/controller/api/provisioning/suspend.go` — Update comments
- `internal/controller/api/provisioning/resize.go` — Update comments
- `internal/controller/api/provisioning/password.go` — Update comments
- `internal/controller/api/provisioning/usage.go` — Update comments
- `internal/controller/api/provisioning/plans.go` — Update comments
- `modules/servers/virtuestack/virtuestack.php` — Use new field names
- `modules/servers/virtuestack/lib/ApiClient.php` — Use new field names
- `modules/servers/virtuestack/webhook.php` — Use new field names
- `internal/controller/billing/blesta/adapter.go` — Proper no-op (like WHMCS)
- `internal/controller/billing/blesta/adapter_test.go` — Updated tests
- `README.md` — Add billing system features
- `AGENTS.md` — Add billing documentation sections
- `CLAUDE.md` — Add billing commands/references
- `docs/architecture.md` — Add billing architecture section
- `docs/api-reference.md` — Add billing API endpoints
- `docs/billing-blesta.md` — Complete rewrite (stub → full module docs)
- `docs/codemaps/backend.md` — Add billing packages
- `docs/codemaps/data.md` — Add billing tables
- `docs/codemaps/dependencies.md` — Add billing dependencies
- `.gitignore` — Comprehensive update

---

## Phase A: Provisioning API Neutralization

### Task 1: Database Migration — Neutral Column Names

Add `external_service_id` and `external_client_id` columns using expand-contract pattern. Copy data from old WHMCS-specific columns.

**Files:**
- Create: `migrations/000080_neutralize_billing_fields.up.sql`
- Create: `migrations/000080_neutralize_billing_fields.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- migrations/000080_neutralize_billing_fields.up.sql
SET lock_timeout = '5s';

-- Neutral column for external billing system's client ID (replaces whmcs_client_id).
ALTER TABLE customers ADD COLUMN IF NOT EXISTS external_client_id INT;
UPDATE customers SET external_client_id = whmcs_client_id WHERE whmcs_client_id IS NOT NULL AND external_client_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_customers_external_client_id ON customers(external_client_id) WHERE external_client_id IS NOT NULL;

-- Neutral column for external billing system's service/order ID (replaces whmcs_service_id).
ALTER TABLE vms ADD COLUMN IF NOT EXISTS external_service_id INT;
UPDATE vms SET external_service_id = whmcs_service_id WHERE whmcs_service_id IS NOT NULL AND external_service_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_vms_external_service_id ON vms(external_service_id) WHERE external_service_id IS NOT NULL;
```

- [ ] **Step 2: Create down migration**

```sql
-- migrations/000080_neutralize_billing_fields.down.sql
SET lock_timeout = '5s';

DROP INDEX IF EXISTS idx_vms_external_service_id;
ALTER TABLE vms DROP COLUMN IF EXISTS external_service_id;

DROP INDEX IF EXISTS idx_customers_external_client_id;
ALTER TABLE customers DROP COLUMN IF EXISTS external_client_id;
```

- [ ] **Step 3: Commit**

```bash
git add migrations/000080_*
git commit -m "feat(db): add neutral external_client_id and external_service_id columns

Expand-contract step 1: add new billing-system-neutral columns alongside
the existing whmcs_client_id and whmcs_service_id columns. Data is copied
from old columns. Code will switch to new columns in subsequent commits.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: Go Models and Repositories — Neutral Field Names

Update Go models to use `ExternalClientID`/`ExternalServiceID` and repositories to query the new columns. Keep old DB columns intact (expand-contract).

**Files:**
- Modify: `internal/controller/models/customer.go`
- Modify: `internal/controller/models/vm.go`
- Modify: `internal/controller/repository/customer_repo.go`
- Modify: `internal/controller/repository/vm_repo.go`

- [ ] **Step 1: Update Customer model**

In `internal/controller/models/customer.go`, rename the field:
```go
// Old:
WHMCSClientID        *int     `json:"whmcs_client_id,omitempty" db:"whmcs_client_id"`
// New:
ExternalClientID     *int     `json:"external_client_id,omitempty" db:"external_client_id"`
```

- [ ] **Step 2: Update VM model**

In `internal/controller/models/vm.go`, rename both occurrences:
```go
// In VM struct — old:
WHMCSServiceID        *int      `json:"whmcs_service_id,omitempty" db:"whmcs_service_id"`
// New:
ExternalServiceID     *int      `json:"external_service_id,omitempty" db:"external_service_id"`

// In CreateVMRequest struct — old:
WHMCSServiceID *int     `json:"whmcs_service_id,omitempty"`
// New:
ExternalServiceID *int  `json:"external_service_id,omitempty"`
```

- [ ] **Step 3: Update CustomerRepository**

In `internal/controller/repository/customer_repo.go`:

1. Update the `CustomerDB` interface method:
```go
// Old:
UpdateWHMCSClientID(ctx context.Context, id string, whmcsClientID int) error
// New:
UpdateExternalClientID(ctx context.Context, id string, externalClientID int) error
```

2. Update the scan target in `scanCustomer()`:
```go
// Old: &c.WHMCSClientID
// New: &c.ExternalClientID
```

3. Update column list in customerSelectCols:
```go
// Old: whmcs_client_id
// New: external_client_id
```

4. Update the Create method's INSERT columns and values:
```go
// Old: whmcs_client_id references → external_client_id
// customer.WHMCSClientID → customer.ExternalClientID
```

5. Rename the update method:
```go
// Old:
func (r *CustomerRepository) UpdateWHMCSClientID(ctx context.Context, id string, whmcsClientID int) error {
    const q = `UPDATE customers SET whmcs_client_id = $1, updated_at = NOW() WHERE id = $2 AND status != 'deleted'`
// New:
func (r *CustomerRepository) UpdateExternalClientID(ctx context.Context, id string, externalClientID int) error {
    const q = `UPDATE customers SET external_client_id = $1, updated_at = NOW() WHERE id = $2 AND status != 'deleted'`
```

- [ ] **Step 4: Update VMRepository**

In `internal/controller/repository/vm_repo.go`:

1. Update `vmSelectCols` — replace `whmcs_service_id` with `external_service_id`
2. Update `scanVM()` — replace `&vm.WHMCSServiceID` with `&vm.ExternalServiceID`
3. Update the Create method's INSERT columns — replace `whmcs_service_id` with `external_service_id`
4. Update the Create method's values — replace `vm.WHMCSServiceID` with `vm.ExternalServiceID`
5. Rename the lookup method:
```go
// Old:
func (r *VMRepository) GetByWHMCSServiceID(ctx context.Context, serviceID int) (*models.VM, error) {
    const q = `SELECT ` + vmSelectCols + ` FROM vms WHERE whmcs_service_id = $1 AND deleted_at IS NULL`
// New:
func (r *VMRepository) GetByExternalServiceID(ctx context.Context, serviceID int) (*models.VM, error) {
    const q = `SELECT ` + vmSelectCols + ` FROM vms WHERE external_service_id = $1 AND deleted_at IS NULL`
```

- [ ] **Step 5: Fix all compilation errors**

Run `grep -rn "WHMCSServiceID\|WHMCSClientID\|GetByWHMCSServiceID\|UpdateWHMCSClientID\|whmcs_client_id\|whmcs_service_id" --include="*.go" internal/` to find all remaining references and update them:
- `internal/controller/api/provisioning/vms.go` — `WHMCSServiceID` → `ExternalServiceID`
- `internal/controller/api/provisioning/customers.go` — `WHMCSClientID` → `ExternalClientID`
- `internal/controller/api/provisioning/status.go` — `GetByWHMCSServiceID` → `GetByExternalServiceID`
- `internal/controller/api/provisioning/sso.go` — `WHMCSServiceID` → `ExternalServiceID`
- Any other files referencing these symbols

- [ ] **Step 6: Build and verify**

```bash
export PATH="/home/hiron/go/bin:/home/hiron/gopath/bin:$PATH"
go build -o /dev/null ./cmd/controller/
```
Expected: Build succeeds with no errors.

- [ ] **Step 7: Run tests**

```bash
go test ./internal/controller/... ./internal/shared/...
```
Expected: All tests pass (except pre-existing libvirtutil failure).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor(api): neutralize Provisioning API field names

Rename WHMCS-specific fields to billing-system-neutral names:
- WHMCSClientID → ExternalClientID (customer model/repo)
- WHMCSServiceID → ExternalServiceID (VM model/repo)
- GetByWHMCSServiceID → GetByExternalServiceID
- UpdateWHMCSClientID → UpdateExternalClientID

The Provisioning API now uses external_client_id and external_service_id
in JSON, making it compatible with any billing system (WHMCS, Blesta, etc).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 3: Provisioning Handlers — Add billing_provider Field + Neutral Comments

Update the CreateOrGetCustomer handler to accept `billing_provider` in the request (defaulting to `"whmcs"` for backward compatibility). Update all handler/route comments to be billing-neutral.

**Files:**
- Modify: `internal/controller/api/provisioning/customers.go`
- Modify: `internal/controller/api/provisioning/handler.go`
- Modify: `internal/controller/api/provisioning/sso.go`
- Modify: `internal/controller/api/provisioning/routes.go`
- Modify: `internal/controller/api/provisioning/vms.go`
- Modify: `internal/controller/api/provisioning/status.go`
- Modify: `internal/controller/api/provisioning/suspend.go`
- Modify: `internal/controller/api/provisioning/resize.go`
- Modify: `internal/controller/api/provisioning/password.go`
- Modify: `internal/controller/api/provisioning/usage.go`
- Modify: `internal/controller/api/provisioning/plans.go`

- [ ] **Step 1: Update CreateCustomerRequest in customers.go**

Add `BillingProvider` field and rename `WHMCSClientID`:
```go
type CreateCustomerRequest struct {
    Email            string  `json:"email" validate:"required,email,max=254"`
    Name             string  `json:"name" validate:"required,max=255"`
    ExternalClientID *int    `json:"external_client_id,omitempty" validate:"omitempty,gt=0"`
    BillingProvider  *string `json:"billing_provider,omitempty" validate:"omitempty,oneof=whmcs blesta native"`
}
```

- [ ] **Step 2: Update CreateOrGetCustomer handler in customers.go**

Update the handler to use the new field names and determine billing provider:
```go
// In the create path, determine billing provider:
billingProvider := models.BillingProviderWHMCS // default for backward compat
if req.BillingProvider != nil {
    billingProvider = *req.BillingProvider
}

customer := &models.Customer{
    Email:            req.Email,
    Name:             req.Name,
    PasswordHash:     &passwordHash,
    AuthProvider:     models.AuthProviderLocal,
    ExternalClientID: req.ExternalClientID,
    BillingProvider:  util.StringPtr(billingProvider),
    Status:           models.CustomerStatusActive,
}
```

Rename `updateWHMCSClientID` → `updateExternalClientID` and update all references within.

- [ ] **Step 3: Update SSO token request in sso.go**

```go
// Old:
WHMCSServiceID *int `json:"whmcs_service_id,omitempty" validate:"omitempty,gt=0"`
// New:
ExternalServiceID *int `json:"external_service_id,omitempty" validate:"omitempty,gt=0"`
```

Update the error message, log fields, and the lookup call from `GetByWHMCSServiceID` → `GetByExternalServiceID`.

- [ ] **Step 4: Update all Go comments to be billing-neutral**

Replace all comments mentioning "WHMCS" with billing-neutral language. Use `grep -rn "WHMCS\|whmcs" --include="*.go" internal/controller/api/provisioning/` to find all occurrences. Examples:

```go
// handler.go — old:
// Package provisioning provides HTTP handlers for the WHMCS Provisioning API.
// new:
// Package provisioning provides HTTP handlers for the Provisioning API.
// These endpoints integrate with external billing systems (WHMCS, Blesta, etc.).

// ProvisioningHandler handles WHMCS provisioning API requests.
// →
// ProvisioningHandler handles Provisioning API requests from external billing systems.

// ProvisioningCreateVMRequest represents the WHMCS provisioning create VM request.
// →
// ProvisioningCreateVMRequest represents a VM creation request from a billing module.
```

Apply similar neutralization to: `vms.go`, `status.go`, `suspend.go`, `resize.go`, `password.go`, `usage.go`, `plans.go`, `routes.go`, `sso.go`.

**Important:** In `resize.go`, update the error message:
```go
// Old: "plan_id is required for resize operations. Contact WHMCS to upgrade your service."
// New: "plan_id is required for resize operations"
```

- [ ] **Step 5: Build and test**

```bash
export PATH="/home/hiron/go/bin:/home/hiron/gopath/bin:$PATH"
go build -o /dev/null ./cmd/controller/ && go test ./internal/controller/... ./internal/shared/...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor(api): make Provisioning API billing-system-neutral

- Add billing_provider field to customer creation (defaults to whmcs)
- Rename all JSON fields: whmcs_client_id → external_client_id,
  whmcs_service_id → external_service_id
- Update all handler comments to reference billing systems generically
- SSO token request uses external_service_id

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 4: Update WHMCS PHP Module for Neutral Field Names

Update the WHMCS PHP module to use the new neutral API field names. This is mostly find-and-replace.

**Files:**
- Modify: `modules/servers/virtuestack/virtuestack.php`
- Modify: `modules/servers/virtuestack/lib/ApiClient.php`
- Modify: `modules/servers/virtuestack/webhook.php`

- [ ] **Step 1: Update ApiClient.php**

In `modules/servers/virtuestack/lib/ApiClient.php`:

1. In `createVM()` — change the required field and idempotency key:
```php
// Old:
$required = ['customer_id', 'plan_id', 'template_id', 'hostname', 'whmcs_service_id'];
$idempotencyKey = 'whmcs-service-' . $params['whmcs_service_id'];
// New:
$required = ['customer_id', 'plan_id', 'template_id', 'hostname', 'external_service_id'];
$idempotencyKey = 'billing-service-' . $params['external_service_id'];
```

2. In `createSSOToken()` — change the payload field:
```php
// Old:
$payload = ['whmcs_service_id' => $serviceId];
// New:
$payload = ['external_service_id' => $serviceId];
```

- [ ] **Step 2: Update virtuestack.php**

In `modules/servers/virtuestack/virtuestack.php`:

1. In `virtuestack_CreateAccount()` — update the VM creation params:
```php
// Old:
'whmcs_service_id' => $serviceId,
// New:
'external_service_id' => $serviceId,
```

2. In `ensureCustomer()` or customer creation — update the customer params:
```php
// Old:
'whmcs_client_id' => $clientId,
// New:
'external_client_id' => $clientId,
'billing_provider' => 'whmcs',
```

- [ ] **Step 3: Update webhook.php**

In `modules/servers/virtuestack/webhook.php`:
```php
// Old:
$whmcsServiceId = isset($data['whmcs_service_id']) && is_int($data['whmcs_service_id']) ? $data['whmcs_service_id'] : 0;
// New:
$externalServiceId = isset($data['external_service_id']) && is_int($data['external_service_id']) ? $data['external_service_id'] : 0;
```

Update all subsequent uses of `$whmcsServiceId` → `$externalServiceId` in the webhook handler.

- [ ] **Step 4: Verify PHP syntax**

```bash
find modules/servers/virtuestack -name '*.php' -exec php -l {} \;
```
Expected: No syntax errors in any file.

- [ ] **Step 5: Commit**

```bash
git add modules/servers/virtuestack/
git commit -m "refactor(whmcs): adapt WHMCS module to neutral API field names

Update all API calls to use external_client_id, external_service_id,
and billing_provider instead of WHMCS-specific field names.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Phase B: Blesta PHP Module

### Task 5: Blesta Module Scaffold — config.json + Language File

Create the Blesta module directory structure with config.json and English language file.

**Files:**
- Create: `modules/blesta/virtuestack/config.json`
- Create: `modules/blesta/virtuestack/language/en_us/virtuestack.php`

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p modules/blesta/virtuestack/{lib,language/en_us,views/default}
```

- [ ] **Step 2: Create config.json**

```json
{
    "name": "Virtuestack",
    "version": "1.0.0",
    "authors": [
        {
            "name": "VirtueStack",
            "url": "https://github.com/AbuGosok/VirtueStack"
        }
    ],
    "description": "VirtueStack KVM/QEMU VPS provisioning module for Blesta. Creates, manages, suspends, and terminates virtual machines via the VirtueStack Provisioning API."
}
```

- [ ] **Step 3: Create language file**

Create `modules/blesta/virtuestack/language/en_us/virtuestack.php` with all UI strings used by the module. This file must define `$lang` array entries for:
- Module metadata (`Virtuestack.name`, `Virtuestack.description`)
- Module row (server) fields (`Virtuestack.add_row.*`, `Virtuestack.edit_row.*`)
- Package fields (`Virtuestack.package_fields.*`)
- Service fields (`Virtuestack.service_fields.*`)
- Tab labels (`Virtuestack.tab_client_service`, `Virtuestack.tab_admin_service`, `Virtuestack.tab_client_console`)
- Status labels and error messages
- Button labels (Start, Stop, Restart, Console, etc.)

Reference: Follow Blesta module language file conventions with `$lang['Virtuestack.section.key'] = 'Value';` format.

- [ ] **Step 4: Commit**

```bash
git add modules/blesta/
git commit -m "feat(blesta): add module scaffold with config and language file

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 6: Blesta API Client

Port the WHMCS ApiClient to work with Blesta's module system. The client calls VirtueStack's Provisioning API via HTTPS with `X-API-Key` authentication.

**Files:**
- Create: `modules/blesta/virtuestack/lib/ApiClient.php`

- [ ] **Step 1: Create ApiClient.php**

Create `modules/blesta/virtuestack/lib/ApiClient.php` implementing a `VirtueStackApiClient` class with these requirements:

**Constructor:** Takes `$apiUrl` (string, must start with `https://`), `$apiKey` (string). Sets up cURL defaults: SSL verify on, follow redirects off, 30s timeout, JSON accept header, `X-API-Key` header, `User-Agent: VirtueStack-Blesta/1.0`.

**Core method:** `request($method, $path, $params = [], $idempotencyKey = null)` — makes HTTP request, returns decoded JSON response or throws on error. Response body limit: 1MB. Includes `Idempotency-Key` header when provided.

**Public API methods** (mirror the WHMCS ApiClient, see `modules/servers/virtuestack/lib/ApiClient.php` for reference):

| Method | HTTP | Endpoint | Purpose |
|--------|------|----------|---------|
| `healthCheck()` | GET | `/health` | Test connection |
| `createVM($params)` | POST | `/provisioning/vms` | Create VM (requires `customer_id`, `plan_id`, `template_id`, `hostname`, `external_service_id`). Auto-generates `Idempotency-Key: blesta-service-{external_service_id}` |
| `deleteVM($vmId)` | DELETE | `/provisioning/vms/{vmId}` | Terminate VM |
| `suspendVM($vmId)` | POST | `/provisioning/vms/{vmId}/suspend` | Suspend |
| `unsuspendVM($vmId)` | POST | `/provisioning/vms/{vmId}/unsuspend` | Unsuspend |
| `resizeVM($vmId, $planId)` | POST | `/provisioning/vms/{vmId}/resize` | Resize with plan_id |
| `getVMInfo($vmId)` | GET | `/provisioning/vms/{vmId}` | Full VM details |
| `getVMStatus($vmId)` | GET | `/provisioning/vms/{vmId}/status` | Power state |
| `getVMUsage($vmId)` | GET | `/provisioning/vms/{vmId}/usage` | Bandwidth/disk |
| `getVMByServiceId($serviceId)` | GET | `/provisioning/vms/by-service/{serviceId}` | Lookup by billing service ID |
| `powerOperation($vmId, $op)` | POST | `/provisioning/vms/{vmId}/power` | start/stop/restart |
| `resetPassword($vmId)` | POST | `/provisioning/vms/{vmId}/password/reset` | Reset root password |
| `setPassword($vmId, $pw)` | POST | `/provisioning/vms/{vmId}/password` | Set specific password |
| `getTask($taskId)` | GET | `/provisioning/tasks/{taskId}` | Poll async task |
| `createSSOToken($serviceId, $vmId)` | POST | `/provisioning/sso-tokens` | Create SSO token |
| `createCustomer($email, $name, $clientId)` | POST | `/provisioning/customers` | Create/get customer (sends `billing_provider: "blesta"`, `external_client_id: $clientId`) |
| `listPlans()` | GET | `/provisioning/plans` | List available plans |

**Error handling:** Throw exceptions with descriptive messages including HTTP status code and response body. Log errors for debugging.

- [ ] **Step 2: Verify PHP syntax**

```bash
php -l modules/blesta/virtuestack/lib/ApiClient.php
```

- [ ] **Step 3: Commit**

```bash
git add modules/blesta/virtuestack/lib/ApiClient.php
git commit -m "feat(blesta): add API client for VirtueStack Provisioning API

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 7: Blesta Helper Library

Create utility functions for webhook signature verification, input validation, password generation, SSO URL building, and service field access.

**Files:**
- Create: `modules/blesta/virtuestack/lib/VirtueStackHelper.php`

- [ ] **Step 1: Create VirtueStackHelper.php**

Create `modules/blesta/virtuestack/lib/VirtueStackHelper.php` implementing a `VirtueStackHelper` class with these static methods:

**Webhook Verification:**
- `verifyWebhookSignature($body, $signature, $secret)` — HMAC-SHA256 verification using `hash_equals()` for timing-safe comparison. Always compute HMAC even when secret is empty (anti-timing attack). Reference: `modules/servers/virtuestack/lib/shared_functions.php:165-185`.

**Input Validation:**
- `isValidUUID($value)` — Validate UUID v4 format via regex
- `isValidIP($value)` — Validate IPv4/IPv6 via `filter_var()`
- `isValidVMStatus($value)` — Check against allowed enum: `running`, `stopped`, `suspended`, `provisioning`, `migrating`, `reinstalling`, `error`, `deleted`

**Password Generation:**
- `generatePassword($length = 16)` — Crypto-safe password with at least 1 uppercase, 1 lowercase, 1 digit, 1 special char. Use `random_int()` for all randomness. Use Fisher-Yates shuffle with `random_int()` (never `str_shuffle()`). Reference: `modules/servers/virtuestack/lib/VirtueStackHelper.php:135-161`.

**SSO URL Building:**
- `buildSSOUrl($webuiUrl, $token)` — Build `{webuiUrl}/api/v1/customer/auth/sso-exchange?token={urlencoded_token}`
- `buildConsoleUrl($webuiUrl, $token)` — Same as SSO URL (console type resolved by WebUI after auth)

**Service Field Helpers (Blesta-specific):**
- `getServiceField($service, $fieldName)` — Extract a field value from Blesta's service fields array
- `getModuleRowMeta($moduleRow, $key)` — Extract meta value from module row
- `getPackageMeta($package, $key)` — Extract meta value from package

- [ ] **Step 2: Verify PHP syntax**

```bash
php -l modules/blesta/virtuestack/lib/VirtueStackHelper.php
```

- [ ] **Step 3: Commit**

```bash
git add modules/blesta/virtuestack/lib/VirtueStackHelper.php
git commit -m "feat(blesta): add helper library for webhooks, validation, SSO

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 8: Blesta Main Module — Meta and Configuration Methods

Create the main module class with constructor, install/uninstall, module row (server) management, and package (product) configuration.

**Files:**
- Create: `modules/blesta/virtuestack/virtuestack.php`

- [ ] **Step 1: Create module class skeleton**

Create `modules/blesta/virtuestack/virtuestack.php` with class `Virtuestack extends \Module`. Include:

**Constructor:**
```php
public function __construct()
{
    Loader::loadComponents($this, ['Input', 'Record']);
    Language::loadLang('virtuestack', null, dirname(__FILE__) . DS . 'language' . DS);
    $this->loadConfig(dirname(__FILE__) . DS . 'config.json');
}
```

**Module row methods** (server connection config):

`addModuleRow(array &$vars)` — Validates and saves server connection: hostname, port (default 443), API key, use_ssl (default true), webui_url. Calls `healthCheck()` on the API to verify connectivity. Returns module row ID on success, sets Input errors on failure.

`editModuleRow($module_row, array &$vars)` — Same validation, updates existing row.

`deleteModuleRow($module_row)` — No-op (allow deletion).

`manageAddRow(array &$vars)` — Loads `add_row.pdt` view, passes vars for form rendering.

`manageEditRow($module_row, array &$vars)` — Loads `edit_row.pdt` view, merges existing meta into vars.

`getModuleRowFields()` — Internal helper: returns expected meta field names.

**Package field methods** (per-product config):

`getPackageFields($vars = null)` — Returns `ModuleFields` with:
- `plan_id` (text, 36 chars) — VirtueStack Plan UUID (required)
- `template_id` (text, 36 chars) — Default OS Template UUID (required)
- `location_id` (text, 36 chars) — Location UUID (optional)
- `hostname_prefix` (text, 20 chars) — Auto-hostname prefix (default "vps")

`validatePackage(array $vars = null)` — Validate plan_id and template_id are valid UUIDs.

**API client helper:**
`getApi($module_row)` — Creates and returns `VirtueStackApiClient` from module row meta (hostname, port, api_key, use_ssl). Caches per module_row ID.

- [ ] **Step 2: Verify PHP syntax**

```bash
php -l modules/blesta/virtuestack/virtuestack.php
```

- [ ] **Step 3: Commit**

```bash
git add modules/blesta/virtuestack/virtuestack.php
git commit -m "feat(blesta): add module class with server and package config

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 9: Blesta Module — Service Lifecycle Methods

Add service lifecycle methods: addService, cancelService, suspendService, unsuspendService, editService.

**Files:**
- Modify: `modules/blesta/virtuestack/virtuestack.php`

- [ ] **Step 1: Implement addService**

`addService($package, array $vars = null, $parent_package = null, $parent_service = null, $status = 'pending')`:

1. Get module row (server) via `$this->getModuleRow($package->module_row)`
2. Get API client via `$this->getApi($module_row)`
3. Extract package meta: `plan_id`, `template_id`, `location_id`, `hostname_prefix`
4. Determine service ID from `$vars['service_id']` (Blesta's auto-assigned ID, used as `external_service_id`)
5. **Idempotency check**: Call `$api->getVMByServiceId($serviceId)` — if VM exists, store its fields and return early
6. **Ensure customer**: Call `$api->createCustomer($email, $name, $clientId)` with `billing_provider: "blesta"`. Store `virtuestack_customer_id` in service fields
7. **Build hostname**: Use `$vars['hostname']` if set, otherwise `"{prefix}-{serviceId}"`
8. **Create VM**: Call `$api->createVM([...])` with `external_service_id: $serviceId`
9. **Store service fields**: Return array with keys `vm_id`, `task_id`, `provisioning_status` (= "pending"), `virtuestack_customer_id`
10. On API error: `$this->Input->setErrors(...)` and return

**Service fields** returned from addService (stored by Blesta):
```php
return [
    ['key' => 'vm_id', 'value' => $result['vm_id'], 'encrypted' => 0],
    ['key' => 'vm_ip', 'value' => '', 'encrypted' => 0],
    ['key' => 'vm_status', 'value' => 'provisioning', 'encrypted' => 0],
    ['key' => 'provisioning_status', 'value' => 'pending', 'encrypted' => 0],
    ['key' => 'task_id', 'value' => $result['task_id'], 'encrypted' => 0],
    ['key' => 'virtuestack_customer_id', 'value' => $customerId, 'encrypted' => 0],
    ['key' => 'provisioning_error', 'value' => '', 'encrypted' => 0],
];
```

- [ ] **Step 2: Implement cancelService**

`cancelService($package, $service, ...)`:
1. Get vm_id from service fields
2. If no vm_id, return (nothing to cancel)
3. Call `$api->deleteVM($vmId)`
4. Update service field `provisioning_status` → "terminated"
5. On error: set Input errors

- [ ] **Step 3: Implement suspendService**

`suspendService($package, $service, ...)`:
1. Get vm_id from service fields
2. Call `$api->suspendVM($vmId)`
3. Update service field `provisioning_status` → "suspended", `vm_status` → "suspended"

- [ ] **Step 4: Implement unsuspendService**

`unsuspendService($package, $service, ...)`:
1. Get vm_id from service fields
2. Call `$api->unsuspendVM($vmId)`
3. Update service field `provisioning_status` → "active", `vm_status` → "stopped"

- [ ] **Step 5: Implement editService (resize)**

`editService($package, $service, $vars, ...)`:
1. If package meta `plan_id` has changed (upgrade/downgrade):
   - Get vm_id from service fields
   - Call `$api->resizeVM($vmId, $newPlanId)`
   - Store new task_id if returned
2. If no plan change, no-op

- [ ] **Step 6: Verify PHP syntax and commit**

```bash
php -l modules/blesta/virtuestack/virtuestack.php
git add modules/blesta/virtuestack/virtuestack.php
git commit -m "feat(blesta): add service lifecycle methods (create, cancel, suspend, unsuspend, resize)

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 10: Blesta Module — Service Info, Tabs, SSO, Password

Add service information displays, admin/client tabs, SSO integration, and password management.

**Files:**
- Modify: `modules/blesta/virtuestack/virtuestack.php`

- [ ] **Step 1: Implement getAdminTabs and getClientTabs**

```php
public function getAdminTabs($package)
{
    return [
        'tabAdminService' => Language::_('Virtuestack.tab_admin_service', true),
    ];
}

public function getClientTabs($package)
{
    return [
        'tabClientService' => Language::_('Virtuestack.tab_client_service', true),
        'tabClientConsole' => Language::_('Virtuestack.tab_client_console', true),
    ];
}
```

- [ ] **Step 2: Implement tabAdminService**

`tabAdminService($package, $service, ...)`:
1. Get API client and vm_id
2. Call `$api->getVMInfo($vmId)` to get current VM details
3. Call `$api->getVMStatus($vmId)` for live status
4. Handle POST actions (power operations: start, stop, restart, sync_status)
5. Load `tab_admin_service.pdt` view with VM info (id, ip, status, node, plan, resources)

- [ ] **Step 3: Implement tabClientService**

`tabClientService($package, $service, ...)`:
1. Get API client and vm_id
2. If provisioning_status is "pending", check task status and show provisioning progress
3. If active, show VM overview with:
   - SSO iframe URL (embedded VirtueStack customer portal)
   - Or power control buttons + VM info
4. Handle POST for power operations (start, stop, restart)
5. Load `tab_client_service.pdt` view

For the SSO iframe approach:
```php
$sso = $api->createSSOToken($serviceId, $vmId);
$iframeUrl = VirtueStackHelper::buildSSOUrl($webuiUrl, $sso['token']);
$this->view->set('iframe_url', $iframeUrl);
```

- [ ] **Step 4: Implement tabClientConsole**

`tabClientConsole($package, $service, ...)`:
1. Create SSO token via API
2. Build console URL
3. Load `tab_client_console.pdt` view with console URL

- [ ] **Step 5: Implement password reset helper**

Add private method `resetPassword($module_row, $service)`:
1. Get vm_id from service fields
2. Call `$api->resetPassword($vmId)`
3. Return new password

- [ ] **Step 6: Implement getServiceName**

```php
public function getServiceName($service)
{
    foreach ($service->fields as $field) {
        if ($field->key === 'vm_id') {
            return $field->value;
        }
    }
    return null;
}
```

- [ ] **Step 7: Verify PHP syntax and commit**

```bash
php -l modules/blesta/virtuestack/virtuestack.php
git add modules/blesta/virtuestack/virtuestack.php
git commit -m "feat(blesta): add service tabs, SSO, and password management

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 11: Blesta View Templates

Create all .pdt view templates for the Blesta module.

**Files:**
- Create: `modules/blesta/virtuestack/views/default/add_row.pdt`
- Create: `modules/blesta/virtuestack/views/default/edit_row.pdt`
- Create: `modules/blesta/virtuestack/views/default/manage.pdt`
- Create: `modules/blesta/virtuestack/views/default/tab_client_service.pdt`
- Create: `modules/blesta/virtuestack/views/default/tab_admin_service.pdt`
- Create: `modules/blesta/virtuestack/views/default/tab_client_console.pdt`

- [ ] **Step 1: Create add_row.pdt (server config form)**

Form fields for adding a new VirtueStack server:
- Hostname (text input, required)
- Port (text input, default 443)
- API Key (password input, required)
- Use SSL (checkbox, default checked)
- WebUI URL (text input, optional — for SSO redirects, e.g. `https://panel.example.com`)

Use Blesta's `$this->Form->fieldText()`, `$this->Form->fieldPassword()`, `$this->Form->fieldCheckbox()` helpers. Reference: Standard Blesta module row view pattern.

- [ ] **Step 2: Create edit_row.pdt**

Same layout as add_row.pdt but pre-populated with existing values from `$vars`. The API key field shows a placeholder indicating the current key is set (don't display the actual key). Only update the key if a new value is entered.

- [ ] **Step 3: Create manage.pdt (package config form)**

Form fields for configuring a package/product:
- Plan ID (text input, 36 chars, required) — VirtueStack plan UUID
- Template ID (text input, 36 chars, required) — Default OS template UUID
- Location ID (text input, 36 chars, optional) — Data center location UUID
- Hostname Prefix (text input, 20 chars, default "vps")

Use Blesta's `ModuleFields` pattern. Pre-populate from `$vars->meta` when editing.

- [ ] **Step 4: Create tab_admin_service.pdt**

Admin view of a provisioned VM service:
- VM ID, IP address, status badge (colored: running=green, stopped=red, suspended=yellow, provisioning=blue)
- Node ID, plan details (vCPU, memory, disk)
- Power control buttons: Start, Stop, Restart, Sync Status
- Each button submits a POST form with `action` parameter
- If provisioning is pending, show task progress/status instead

- [ ] **Step 5: Create tab_client_service.pdt**

Client view of their VM service:
- **If provisioning pending:** Show "Your server is being provisioned..." with task status
- **If active:** Embed VirtueStack customer portal in an iframe via SSO URL
  - iframe with `sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"`
  - `allow="clipboard-read; clipboard-write"`
  - Minimum height 600px, full width
  - Link to open in new tab
- **If suspended/error:** Show status message

- [ ] **Step 6: Create tab_client_console.pdt**

Fullscreen VNC/Serial console view:
- Full-viewport iframe loading the console SSO URL
- Minimal chrome (no headers/footers)
- Back button to return to service view

- [ ] **Step 7: Verify PHP syntax and commit**

```bash
find modules/blesta/virtuestack/views -name '*.pdt' -exec php -l {} \;
git add modules/blesta/virtuestack/views/
git commit -m "feat(blesta): add view templates for server, package, and service UI

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 12: Blesta Webhook Handler

Create a standalone webhook receiver endpoint for processing VirtueStack events (VM created, deleted, suspended, etc).

**Files:**
- Create: `modules/blesta/virtuestack/webhook.php`

- [ ] **Step 1: Create webhook.php**

Create `modules/blesta/virtuestack/webhook.php` as a standalone PHP endpoint that:

1. **Bootstrap Blesta**: Load Blesta's initialization (`init.php`) to access database and module infrastructure
2. **Validate request**: POST only, `application/json` content type, max 64KB body
3. **Read and verify signature**: Read `X-VirtueStack-Signature` header, verify HMAC-SHA256 against stored webhook secret using `VirtueStackHelper::verifyWebhookSignature()`
4. **Parse event**: Decode JSON body, extract `event`, `external_service_id` (was `whmcs_service_id`), `vm_id`, `task_id`, `data`
5. **Whitelist events**: Only process known events:
   - `vm.created` — Store vm_id, IP, password; set provisioning_status=active; trigger welcome email
   - `vm.creation_failed` — Set provisioning_status=error, store error message
   - `vm.deleted` — Clear vm_id/ip, set provisioning_status=terminated
   - `vm.suspended` — Set provisioning_status=suspended
   - `vm.unsuspended` — Set provisioning_status=active
   - `vm.resized` — Log the resize
   - `vm.started` — Set vm_status=running
   - `vm.stopped` — Set vm_status=stopped
   - `vm.reinstalled` — Set provisioning_status=active, vm_status=running
   - `vm.migrated` — Update node_id
   - `task.completed` — Clear task_id
   - `task.failed` — Store error message
6. **Service lookup**: Find the Blesta service by `external_service_id` or by querying service fields for matching `vm_id`/`task_id`
7. **Update service fields**: Use Blesta's `Record` or `ModuleManager` to update the service fields
8. **Respond**: 200 OK on success, 400/401/500 on errors

**Logging**: Write to Blesta's module log via `$this->log()` or file-based logging similar to WHMCS (`logs/webhook.log` with rotation).

**Security**: Constant-time signature comparison, body size limit, event whitelist, UUID validation on all IDs.

- [ ] **Step 2: Verify PHP syntax**

```bash
php -l modules/blesta/virtuestack/webhook.php
```

- [ ] **Step 3: Commit**

```bash
git add modules/blesta/virtuestack/webhook.php
git commit -m "feat(blesta): add webhook handler for VirtueStack events

Handles VM lifecycle events (created, deleted, suspended, etc) and
task completion events. Uses HMAC-SHA256 signature verification.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Phase C: Go Adapter Cleanup

### Task 13: Go Blesta Adapter — Proper No-Op (Like WHMCS)

Update the Go Blesta adapter from a stub with ErrNotSupported to a proper no-op adapter (matching the WHMCS adapter pattern). Blesta manages billing externally via the Provisioning API.

**Files:**
- Modify: `internal/controller/billing/blesta/adapter.go`
- Modify: `internal/controller/billing/blesta/adapter_test.go`
- Modify: `internal/controller/dependencies.go`

- [ ] **Step 1: Rewrite adapter.go to match WHMCS pattern**

Replace the entire adapter implementation. Model it after `internal/controller/billing/whmcs/adapter.go`:

```go
package blesta

import (
    "context"
    "log/slog"

    "github.com/AbuGosok/VirtueStack/internal/controller/billing"
    sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// BlestaConfig holds configuration for the Blesta billing provider adapter.
type BlestaConfig struct {
    APIURL string
    APIKey string
    Logger *slog.Logger
}

// Adapter implements billing.BillingProvider as a no-op for Blesta.
// Blesta manages all billing externally via the Provisioning REST API
// (PHP module → POST /provisioning/vms etc). The Go adapter exists only
// so customers with billing_provider='blesta' are handled gracefully.
type Adapter struct {
    logger *slog.Logger
}

func NewAdapter(cfg BlestaConfig) *Adapter {
    return &Adapter{
        logger: cfg.Logger.With("component", "billing-blesta"),
    }
}

var _ billing.BillingProvider = (*Adapter)(nil)

func (a *Adapter) Name() string { return "blesta" }

func (a *Adapter) ValidateConfig() error { return nil }

// CreateUser echoes back the customer ID. Blesta creates customers
// via POST /provisioning/customers — the Go side just acknowledges.
func (a *Adapter) CreateUser(_ context.Context, req billing.CreateUserRequest) (*billing.UserResult, error) {
    return &billing.UserResult{
        CustomerID: req.CustomerID,
        ProviderID: req.CustomerID,
    }, nil
}

// GetUserBillingStatus always returns active. Blesta manages suspension
// via POST /provisioning/vms/:id/suspend.
func (a *Adapter) GetUserBillingStatus(_ context.Context, customerID string) (*billing.BillingStatus, error) {
    return &billing.BillingStatus{
        CustomerID: customerID,
        Provider:   "blesta",
        IsActive:   true,
        Status:     "active",
        Message:    "Billing managed by Blesta",
    }, nil
}

// VM lifecycle hooks are no-ops — Blesta manages VM billing externally.
func (a *Adapter) OnVMCreated(_ context.Context, _ billing.VMRef) error  { return nil }
func (a *Adapter) OnVMDeleted(_ context.Context, _ billing.VMRef) error  { return nil }
func (a *Adapter) OnVMResized(_ context.Context, _ billing.VMRef, _, _ string) error { return nil }

// Suspend/Unsuspend are no-ops — Blesta drives these via Provisioning API.
func (a *Adapter) SuspendForNonPayment(_ context.Context, _ string) error   { return nil }
func (a *Adapter) UnsuspendAfterPayment(_ context.Context, _ string) error  { return nil }

// Balance, top-up, and usage are managed by Blesta — not available from Go side.
func (a *Adapter) GetBalance(_ context.Context, _ string) (*billing.Balance, error) {
    return nil, sharederrors.ErrNotSupported
}

func (a *Adapter) ProcessTopUp(_ context.Context, _ billing.TopUpRequest) (*billing.TopUpResult, error) {
    return nil, sharederrors.ErrNotSupported
}

func (a *Adapter) GetUsageHistory(_ context.Context, _ string, _ billing.PaginationOpts) (*billing.UsageHistory, error) {
    return nil, sharederrors.ErrNotSupported
}
```

- [ ] **Step 2: Update adapter_test.go**

Update tests to match the new behavior:
- `CreateUser` now returns a `UserResult` (not `ErrNotSupported`)
- `SuspendForNonPayment` / `UnsuspendAfterPayment` now return nil (not `ErrNotSupported`)
- `GetBalance` / `ProcessTopUp` / `GetUsageHistory` still return `ErrNotSupported`
- Remove the `ValidateConfig` tests that check for missing URL/key errors (no longer validated here — config validation happens in `shared/config/config.go`)

- [ ] **Step 3: Update dependencies.go**

Remove the `"(stub)"` from the log message:
```go
// Old:
s.logger.Info("blesta billing provider registered (stub)")
// New:
s.logger.Info("blesta billing provider registered")
```

The adapter no longer needs APIURL/APIKey since it's a no-op. But keep passing them for potential future use. The BlestaConfig struct still accepts them but the Adapter ignores them.

- [ ] **Step 4: Build and test**

```bash
export PATH="/home/hiron/go/bin:/home/hiron/gopath/bin:$PATH"
go build -o /dev/null ./cmd/controller/ && go test ./internal/controller/billing/blesta/... -v
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(blesta): convert adapter from stub to proper no-op

Blesta manages billing externally via the Provisioning API (PHP module).
The Go adapter is now a no-op like the WHMCS adapter: CreateUser echoes
back IDs, lifecycle hooks return nil, balance/topup return ErrNotSupported.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Phase D: Documentation

### Task 14: README.md Update

Add billing system features and Blesta module to the README.

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add billing features to README**

Add a **Billing & Payments** section to the features list covering:
- Multi-provider billing (WHMCS, Blesta, Native credit-based)
- Credit ledger with immutable transaction log
- Payment gateways: Stripe, PayPal, BTCPay, NOWPayments
- Hourly VM usage billing with automatic charging
- Invoice generation with PDF export
- In-app notification center with SSE real-time delivery
- OAuth registration (Google, GitHub) with PKCE

Add Blesta to the **Billing Integration** section alongside WHMCS:
- WHMCS module (`modules/servers/virtuestack/`)
- Blesta module (`modules/blesta/virtuestack/`)
- Both call the neutral Provisioning API

Update the **Tech Stack** section to include billing dependencies:
- Stripe Go SDK, PayPal REST API, BTCPay/NOWPayments
- go-pdf/fpdf for invoice PDF generation

Update the **Project Status** to mention billing system completion.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README with billing system features

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 15: AGENTS.md Update

Comprehensive update to the LLM reference document covering all billing additions.

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Update Section 2 (Repository Structure)**

Add to the directory tree:
```
internal/
  controller/
    billing/
      provider.go                           # BillingProvider interface + types
      registry.go                           # Thread-safe provider registry
      hook.go                               # VMLifecycleHook interface
      registry_adapter.go                   # RegistryHookAdapter
      whmcs/adapter.go                      # WHMCS no-op adapter
      native/adapter.go                     # Native billing adapter (uses ledger)
      blesta/adapter.go                     # Blesta no-op adapter
    payments/
      provider.go                           # PaymentProvider interface
      registry.go                           # Payment gateway registry
      stripe/provider.go                    # Stripe Checkout + webhooks
      paypal/                               # PayPal Orders API + webhooks
      crypto/                               # BTCPay + NOWPayments
modules/
  servers/virtuestack/                      # WHMCS billing module (PHP)
  blesta/virtuestack/                       # Blesta billing module (PHP)
```

- [ ] **Step 2: Update Section 3 (Technology Stack)**

Add to backend table:
```
| Stripe | stripe-go | v82 |
| PDF Generation | go-pdf/fpdf | v0.6 |
```

- [ ] **Step 3: Update Section 4 (Database)**

Add billing tables to the key tables list:
```
| billing_transactions | Immutable credit ledger (customer_id, type, amount, balance_after) |
| billing_payments | Payment records (gateway, status, amount, currency) |
| billing_invoices | Monthly invoices with line items |
| billing_invoice_line_items | Invoice line items (VM usage, hours, amounts) |
| billing_checkpoints | Hourly billing dedup checkpoints |
| exchange_rates | Currency exchange rates |
| notifications | In-app notification center |
| customer_oauth_links | OAuth provider links (Google, GitHub) |
```

Update migration count to 80 (000001-000080).

- [ ] **Step 4: Update Section 5 (API Architecture)**

Add billing endpoints to Admin API:
```
GET    /billing/transactions
GET    /billing/balance
POST   /billing/credit
GET    /billing/payments
POST   /billing/refund/:paymentId
GET    /billing/config
GET    /exchange-rates
PUT    /exchange-rates/:currency
GET    /invoices
GET    /invoices/:id
GET    /invoices/:id/pdf
POST   /invoices/:id/void
```

Add billing endpoints to Customer API:
```
GET    /billing/balance
GET    /billing/transactions
GET    /billing/usage
POST   /billing/top-up
GET    /billing/payments
GET    /billing/top-up/config
POST   /billing/payments/paypal/capture
GET    /invoices
GET    /invoices/:id
GET    /invoices/:id/pdf
GET    /notifications
POST   /notifications/:id/read
POST   /notifications/read-all
GET    /notifications/sse
```

Add webhook endpoints:
```
POST   /webhooks/stripe    (no auth — signature verified)
POST   /webhooks/paypal    (no auth — signature verified)
POST   /webhooks/crypto    (no auth — HMAC verified)
```

Note: Provisioning API field names are now neutral (`external_service_id`, `external_client_id`, `billing_provider`).

- [ ] **Step 5: Add new Section: Billing Architecture**

Add between existing sections (after API Architecture or after Async Task System):

Document the billing system architecture:
- Provider abstraction layer (BillingProvider interface, Registry)
- Three billing modes: WHMCS (external), Blesta (external), Native (built-in)
- Payment gateway abstraction (PaymentProvider interface)
- Credit ledger (immutable append-only, SELECT FOR UPDATE on balance)
- Hourly billing scheduler with advisory locks for HA
- Invoice generation with PDF export
- Webhook security (body size limits, signature verification)
- SSE-based real-time notifications

- [ ] **Step 6: Update Section 11 (Web UIs)**

Add billing pages:
```
Admin pages: + billing (transactions + payments tabs), invoices
Customer pages: + billing (balance, top-up, transactions, payments), invoices
```

- [ ] **Step 7: Add Blesta section to Section 12 (WHMCS Integration) or as new section**

Rename section to "Billing Module Integration" and add Blesta:
```
modules/blesta/virtuestack/
  virtuestack.php           — Main Blesta module class
  lib/ApiClient.php         — Controller Provisioning API client
  lib/VirtueStackHelper.php — Webhook verification, SSO, validation
  webhook.php               — Webhook receiver endpoint
  config.json               — Module metadata
  language/en_us/           — Language strings
  views/default/            — UI templates (.pdt)
```

Document key Blesta functions: addService, cancelService, suspendService, unsuspendService, editService, tabClientService, tabAdminService, tabClientConsole.

- [ ] **Step 8: Update Section 19 (Adding Features)**

Add "New Billing Provider" guide:
1. Create PHP module in `modules/{system}/virtuestack/`
2. Implement API client calling Provisioning endpoints
3. Create Go no-op adapter in `internal/controller/billing/{name}/`
4. Register in `dependencies.go`
5. Add config struct in `config.go`
6. Add provider name to `billing_provider` validation

Add "New Payment Gateway" guide:
1. Implement `PaymentProvider` interface in `internal/controller/payments/{name}/`
2. Register in `PaymentRegistry`
3. Add webhook handler in `internal/controller/api/webhooks/`
4. Add config struct and env vars

- [ ] **Step 9: Update Section 20 (Key File References)**

Add billing file references:
```
| Billing provider interface | internal/controller/billing/provider.go |
| Billing registry | internal/controller/billing/registry.go |
| Payment provider interface | internal/controller/payments/provider.go |
| Stripe provider | internal/controller/payments/stripe/provider.go |
| PayPal provider | internal/controller/payments/paypal/provider.go |
| Crypto providers | internal/controller/payments/crypto/ |
| Credit ledger service | internal/controller/services/billing_ledger_service.go |
| Billing scheduler | internal/controller/services/billing_scheduler.go |
| Invoice service | internal/controller/services/billing_invoice_service.go |
| Payment service | internal/controller/services/payment_service.go |
| WHMCS module | modules/servers/virtuestack/ |
| Blesta module | modules/blesta/virtuestack/ |
```

- [ ] **Step 10: Commit**

```bash
git add AGENTS.md
git commit -m "docs: update AGENTS.md with billing system documentation

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 16: CLAUDE.md + docs/architecture.md + docs/api-reference.md

Update remaining documentation files.

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/architecture.md`
- Modify: `docs/api-reference.md`

- [ ] **Step 1: Update CLAUDE.md**

Add to the **Build & Development Commands** section:
```bash
# PHP module syntax check
find modules/ -name '*.php' -exec php -l {} \;
```

Add to **Test Coverage** section:
```
| internal/controller/billing/* | adapter_test.go per provider | Billing provider behavior |
| internal/controller/payments/* | provider tests per gateway | Payment gateway integration |
| internal/controller/services | billing_*_test.go | Billing ledger, scheduler, invoices |
```

Add to **Key References**:
```
- `internal/controller/billing/provider.go` — Billing provider interface
- `internal/controller/payments/provider.go` — Payment gateway interface
- `modules/blesta/virtuestack/` — Blesta billing module
```

Update migration count to 80.

- [ ] **Step 2: Update docs/architecture.md**

Add a **Billing System** section describing:
- Three billing modes (WHMCS, Blesta, Native)
- Provider abstraction layer
- Payment gateway integration
- Credit ledger architecture
- Hourly billing flow
- Invoice generation
- Provisioning API neutrality (external_service_id, external_client_id, billing_provider)

- [ ] **Step 3: Update docs/api-reference.md**

Add billing API endpoint documentation:
- Admin billing endpoints (transactions, payments, refunds, invoices, exchange rates)
- Customer billing endpoints (balance, top-up, payments, invoices)
- Webhook endpoints (Stripe, PayPal, crypto)
- Provisioning API updates (neutral field names)

Include request/response examples for key endpoints.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md docs/architecture.md docs/api-reference.md
git commit -m "docs: update CLAUDE.md, architecture, and API reference for billing

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 17: Codemaps + billing-blesta.md + .gitignore

Update code maps, rewrite Blesta documentation, and create comprehensive .gitignore.

**Files:**
- Modify: `docs/codemaps/backend.md`
- Modify: `docs/codemaps/data.md`
- Modify: `docs/codemaps/dependencies.md`
- Modify: `docs/billing-blesta.md`
- Modify: `.gitignore`

- [ ] **Step 1: Update docs/codemaps/backend.md**

Add billing packages to the package map:
```
internal/controller/billing/     — Provider abstraction (interface, registry, hooks)
internal/controller/billing/whmcs/ — WHMCS no-op adapter
internal/controller/billing/native/ — Native billing adapter (uses ledger)
internal/controller/billing/blesta/ — Blesta no-op adapter
internal/controller/payments/    — Payment gateway abstraction
internal/controller/payments/stripe/ — Stripe Checkout provider
internal/controller/payments/paypal/ — PayPal Orders provider
internal/controller/payments/crypto/ — BTCPay + NOWPayments providers
```

Add billing services:
```
billing_ledger_service.go   — Credit/debit with immutable ledger
billing_scheduler.go        — Hourly VM usage billing
billing_invoice_service.go  — Invoice generation + PDF
billing_invoice_pdf.go      — PDF rendering (fpdf)
exchange_rate_service.go    — Currency exchange rates
payment_service.go          — Payment orchestration
in_app_notification_service.go — Notification CRUD + SSE
sse_hub.go                  — Server-Sent Events hub
oauth_service.go            — OAuth account management
oauth_provider.go           — Google/GitHub OAuth providers
```

Add billing handlers:
```
api/admin/billing.go        — Admin billing management
api/admin/invoices.go       — Admin invoice management
api/customer/billing.go     — Customer billing (balance, top-up)
api/customer/billing_paypal.go — PayPal capture
api/customer/invoices.go    — Customer invoices
api/customer/in_app_notifications.go — Notifications
api/customer/oauth.go       — OAuth login/link
api/webhooks/stripe.go      — Stripe webhook handler
api/webhooks/paypal.go      — PayPal webhook handler
api/webhooks/crypto_handler.go — Crypto webhook handler
```

- [ ] **Step 2: Update docs/codemaps/data.md**

Add billing tables with columns and relationships:
- `billing_transactions` (immutable ledger)
- `billing_payments` (payment records with gateway info)
- `billing_invoices` + `billing_invoice_line_items`
- `billing_checkpoints` (hourly billing dedup)
- `exchange_rates`
- `notifications` (with RLS)
- `customer_oauth_links` (with RLS)

Update migration count to 80. Note the new neutral columns: `customers.external_client_id`, `vms.external_service_id`.

- [ ] **Step 3: Update docs/codemaps/dependencies.md**

Add billing-related dependencies:
```
stripe-go           — Stripe payment integration
go-pdf/fpdf         — Invoice PDF generation
```

- [ ] **Step 4: Rewrite docs/billing-blesta.md**

Replace the stub documentation with full module documentation:

```markdown
# Blesta Billing Integration

## Overview

VirtueStack provides a Blesta server module at `modules/blesta/virtuestack/` that enables automated VPS provisioning from Blesta. The module calls VirtueStack's Provisioning REST API to create, manage, and terminate virtual machines.

## Architecture

Blesta → VirtueStack Provisioning API (one-way). Blesta drives all operations:
- Service creation → POST /provisioning/vms
- Suspension → POST /provisioning/vms/:id/suspend
- Termination → DELETE /provisioning/vms/:id
- SSO → POST /provisioning/sso-tokens

VirtueStack sends events back to Blesta via webhooks (vm.created, vm.deleted, etc).

## Installation

1. Copy `modules/blesta/virtuestack/` to your Blesta installation's `components/modules/` directory
2. In Blesta admin: Settings → Modules → Available → Install "VirtueStack"
3. Add a server: Settings → Modules → VirtueStack → Add Server
   - Hostname: Your VirtueStack controller hostname
   - Port: 443
   - API Key: Create one in VirtueStack admin → Provisioning Keys
   - WebUI URL: Your customer portal URL (for SSO)
4. Create a package: Packages → Add → Module: VirtueStack
   - Plan ID: UUID from VirtueStack admin → Plans
   - Template ID: UUID from VirtueStack admin → Templates
   - Location ID: (optional) UUID from VirtueStack admin → Nodes
5. Configure webhook: Set the webhook URL in VirtueStack to point to
   `https://your-blesta.com/components/modules/virtuestack/webhook.php`

## Module Files

| File | Purpose |
|------|---------|
| virtuestack.php | Main module class (lifecycle, tabs, SSO) |
| lib/ApiClient.php | HTTP client for Provisioning API |
| lib/VirtueStackHelper.php | Webhook verification, validation, SSO |
| webhook.php | Webhook receiver endpoint |
| config.json | Module metadata |
| language/en_us/virtuestack.php | English language strings |
| views/default/*.pdt | UI templates |

## Configuration

| Setting | Location | Description |
|---------|----------|-------------|
| Hostname | Server config | VirtueStack controller hostname |
| Port | Server config | API port (default 443) |
| API Key | Server config | Provisioning API key |
| WebUI URL | Server config | Customer portal URL for SSO |
| Plan ID | Package meta | VirtueStack plan UUID |
| Template ID | Package meta | Default OS template UUID |
| Location ID | Package meta | Data center location UUID |

## Go-Side Adapter

The Go Blesta adapter (`internal/controller/billing/blesta/adapter.go`) is a no-op.
Blesta manages all billing externally. The adapter exists so customers with
`billing_provider='blesta'` are handled gracefully by the registry.

Enabled via: `BILLING_BLESTA_ENABLED=true`
```

- [ ] **Step 5: Update .gitignore**

Replace `.gitignore` with comprehensive content:

```gitignore
# Build output
bin/

# Go
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
coverage.html
coverage.out

# Environment
.env
.env.local
.env.*.local

# IDE
.idea/
.vscode/settings.json
.vscode/launch.json
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Node.js (WebUI deps installed in subdirectories)
node_modules/

# Test artifacts
tests/e2e/test-results/
tests/e2e/playwright-report/

# Generated
docs/swagger.json
docs/swagger.yaml

# Invoices (generated at runtime)
invoices/

# Logs
*.log

# Certificates (keep ssl/ templates but ignore generated certs)
*.pem
*.key
*.crt
!ssl/

# Docker data
docker-data/
```

**Note:** Remove `CLAUDE.md` and `AGENTS.md` from .gitignore — these are project documentation that should be tracked in version control.

- [ ] **Step 6: Commit**

```bash
git add docs/codemaps/ docs/billing-blesta.md .gitignore
git commit -m "docs: update codemaps, Blesta docs, and .gitignore

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Dependencies

```
Task 1 (migration) → Task 2 (models/repos) → Task 3 (handlers) → Task 4 (WHMCS update)
Task 3 → Task 5-12 (Blesta module uses neutral API)
Task 3 → Task 13 (Go adapter)
Task 13 → Task 14-17 (documentation references final state)
```

Tasks 5-12 (Blesta module) are sequential within the phase.
Tasks 14-17 (documentation) can run after all code is complete.
