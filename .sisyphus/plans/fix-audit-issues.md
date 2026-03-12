# VirtueStack Comprehensive Audit Remediation Plan

**Plan Created:** March 11, 2026
**Source:** `docs/CODEBASE_AUDIT_REPORT.md`
**Total Issues:** 83+ markers across 76+ files
**Estimated Duration:** 8-10 weeks

---

## Executive Summary

This plan addresses every issue identified in the comprehensive codebase audit. Issues are organized into 4 phases with strict dependency ordering — Phase 1 MUST complete before Phase 2, etc.

### Issue Distribution

| Phase | Severity | Count | Duration |
|-------|----------|-------|----------|
| **Phase 1** | CRITICAL | 6 | Weeks 1-2 |
| **Phase 2** | HIGH | 11 | Weeks 3-5 |
| **Phase 3** | MEDIUM | 10 | Weeks 6-7 |
| **Phase 4** | LOW | 40+ | Week 8 |
| **TOTAL** | | **83+** | **8 weeks** |

### Critical Path

```
Phase 1.1 (Password Hashing)
    → Phase 1.2-1.3 (Portal Auth)
        → Phase 1.4-1.6 (API Security)
            → Phase 2.1-2.11 (Core Features)
                → Phase 3.1-3.10 (API/Data)
                    → Phase 4.1-4.3 (Cleanup)
```

---

## Phase 1: Security & Authentication [CRITICAL]

**Status:** BLOCKED - Must complete before ANY Phase 2 work
**Duration:** 2 weeks
**Dependencies:** None

### TODO 1.1: Fix Password Hashing

**File:** `internal/controller/tasks/handlers.go:701-707`

**Current State:**
```go
func hashPassword(password string) string {
    if password == "" {
        return ""
    }
    return "$6$rounds=4096$" + password + "$hashed"
}
```

**Required Changes:**
- Replace placeholder SHA-512 with Argon2id (already in go.mod)
- Implement proper salt generation (16+ bytes crypto/rand)
- Add password strength validation (min 8 chars, complexity requirements)
- Update function to return error for invalid inputs

**Acceptance Criteria:**
- [x] `hashPassword` uses Argon2id with unique salt per password
- [x] Salt is cryptographically random, 16+ bytes
- [x] Empty password returns empty hash without error
- [x] Unit test: `go test ./internal/controller/tasks/... -run TestHashPassword` passes
- [x] Hash verification function added and tested
- [x] bcrypt cost factor >= 12 (or Argon2id equivalent)
- [x] Passwords are non-reversible

**Dependencies:** None
**Blocks:** TODO 1.2, 1.3, 1.5

**QA Scenarios:**
```bash
# Scenario 1: Different hashes for same password
go test ./internal/controller/tasks/... -run TestHashPasswordUniqueness -v
# Expected: Pass - same password produces different hashes due to unique salts

# Scenario 2: Hash verification works
go test ./internal/controller/tasks/... -run TestVerifyPassword -v
# Expected: Pass - verification matches correct password

# Scenario 3: Empty password handling
go test ./internal/controller/tasks/... -run TestHashPasswordEmpty -v
# Expected: Pass - empty string returns empty hash
```

---

### TODO 1.2: Implement Admin Portal Authentication

**File:** `webui/admin/app/login/page.tsx:45-58`

**Current State:**
```typescript
// TODO: Replace with actual API call
// POST /api/v1/admin/auth/login
console.log("Login attempt:", data);
await new Promise((resolve) => setTimeout(resolve, 1000));
console.log("Login successful");
```

**Required Changes:**
- Create API client: `webui/admin/lib/api-client.ts`
- Create auth context: `webui/admin/lib/auth-context.tsx`
- Create auth types: `webui/admin/lib/types/auth.ts`
- Replace mock with real API call to `POST /api/v1/admin/auth/login`
- Implement JWT token storage (httpOnly cookies preferred)
- Implement token refresh mechanism
- Add proper error handling and display

**Acceptance Criteria:**
- [x] Login form calls `POST /api/v1/admin/auth/login` with email/password
- [x] JWT tokens stored securely (httpOnly cookie or secure localStorage)
- [x] Invalid credentials show error message to user
- [x] Token refresh works silently before expiration
- [x] Redirect to dashboard on successful login
- [x] 2FA modal appears when required
- [x] Logout clears tokens and redirects to login

**Dependencies:** TODO 1.1 (backend password hashing must work)
**Blocks:** None directly, but required for secure portal

**QA Scenarios:**
```bash
# Playwright E2E tests
npx playwright test tests/e2e/admin-login.spec.ts

# Scenario 1: Valid login redirects to dashboard
# Scenario 2: Invalid credentials show error
# Scenario 3: Token refresh extends session
# Scenario 4: Logout clears session
```

---

### TODO 1.3: Implement Customer Portal Authentication

**File:** `webui/customer/app/login/page.tsx:45-58`

**Current State:** Same mock pattern as admin portal

**Required Changes:**
- Create API client: `webui/customer/lib/api-client.ts`
- Create auth context: `webui/customer/lib/auth-context.tsx`
- Create auth types: `webui/customer/lib/types/auth.ts`
- Connect to `POST /api/v1/customer/auth/login`
- Shared auth service patterns with admin (code reuse where appropriate)
- Customer-specific token claims handling

**Acceptance Criteria:**
- [x] Login form calls `POST /api/v1/customer/auth/login`
- [x] JWT tokens stored securely
- [x] Customer token has `customer_id` claim (not admin claims)
- [x] Invalid credentials show error message
- [x] Token refresh works
- [x] Redirect to VM list on successful login

**Dependencies:** TODO 1.1, 1.2 (share patterns with admin)
**Blocks:** Phase 2.7, 2.8, 2.9 (frontend controls need auth)

**QA Scenarios:**
```bash
npx playwright test tests/e2e/customer-login.spec.ts
# Same scenarios as admin, but for customer flow
```

---

### TODO 1.4: Secure Provisioning API

**Files:**
- `internal/controller/api/provisioning/routes.go:107`
- `internal/controller/grpc_client.go:202`

**Current State:**
```go
// WARNING: Do not use in production without additional authentication
```

**Required Changes:**
- Create API key middleware: `internal/controller/api/provisioning/middleware/auth.go`
- Require valid API key in `X-API-Key` header
- Implement rate limiting per API key
- Add request signing for sensitive operations
- Remove all WARNING comments
- Add audit logging for all provisioning calls

**Acceptance Criteria:**
- [x] API key middleware created and applied to all provisioning routes
- [x] Unauthenticated requests return 401 Unauthorized
- [x] Rate limiting returns 429 after threshold exceeded
- [x] No "WARNING: Do not use in production" strings in codebase
- [x] Audit log entries created for all provisioning API calls
- [x] Request signing implemented for VM creation/deletion

**Dependencies:** None
**Blocks:** Phase 2.4 (API key management must work)

**QA Scenarios:**
```bash
# Scenario 1: No API key returns 401
curl -X POST http://localhost:8080/vms -H "Content-Type: application/json" -d '{"hostname":"test"}'
# Expected: 401 Unauthorized

# Scenario 2: Valid API key accepted
curl -X POST http://localhost:8080/vms -H "X-API-Key: valid-key" -H "Content-Type: application/json" -d '{"hostname":"test"}'
# Expected: Request processed (may have validation errors, but not 401)

# Scenario 3: No WARNING strings remain
grep -r "WARNING: Do not use in production" internal/controller/
# Expected: No matches
```

---

### TODO 1.5: Implement Password Reset

**File:** `internal/controller/services/auth_service.go:421-440`

**Current State:**
```go
// TODO: Implement password_resets table and storage
return fmt.Errorf("password reset not yet fully implemented")
```

**Required Changes:**
- Create migration: `migrations/000011_password_resets.up.sql`
- Create down migration: `migrations/000011_password_resets.down.sql`
- Implement `password_resets` table with:
  - `id UUID PRIMARY KEY`
  - `user_id UUID NOT NULL`
  - `token_hash VARCHAR(64) NOT NULL`
  - `expires_at TIMESTAMP NOT NULL`
  - `used_at TIMESTAMP`
- Implement token generation with `crypto/rand` (32 bytes)
- Implement email notification service (or placeholder)
- Set 24-hour expiration
- Implement `auth_service.go` methods:
  - `InitiatePasswordReset(ctx, email)`
  - `ValidateResetToken(ctx, token)`
  - `CompletePasswordReset(ctx, token, newPassword)`

**Acceptance Criteria:**
- [x] `password_resets` table created with migration
- [x] Down migration exists and works
- [x] Token generated with crypto/rand (32+ bytes)
- [x] Email notification triggered (or logged in dev mode)
- [x] 24-hour expiration enforced
- [x] Expired tokens rejected with clear error
- [x] Used tokens invalidated after use
- [x] Full reset flow works end-to-end

**Dependencies:** TODO 1.1 (password hashing)
**Blocks:** None directly

**QA Scenarios:**
```bash
# Scenario 1: Initiate reset creates token
go test ./internal/controller/services/... -run TestInitiatePasswordReset -v

# Scenario 2: Expired token rejected
go test ./internal/controller/services/... -run TestExpiredResetToken -v

# Scenario 3: Used token invalidated
go test ./internal/controller/services/... -run TestUsedTokenInvalidated -v

# Scenario 4: Migration up/down works
make migrate-up && make migrate-down
# Expected: Both succeed
```

---

### TODO 1.6: Fix Customer Updates Not Persisting

**File:** `internal/controller/services/customer_service.go:81-94`

**Current State:**
```go
// TODO: Implement repository.Update method for customers
return fmt.Errorf("customer update not yet implemented in repository")
```

**Required Changes:**
- Implement `repository.Update` method in `internal/controller/repository/customer_repo.go`
- Add field validation (email format, phone format, etc.)
- Implement audit logging for changes
- Return appropriate errors for validation failures

**Acceptance Criteria:**
- [x] `customerRepo.Update(ctx, customer)` method implemented
- [x] Customer profile changes persist to database
- [x] Field validation rejects invalid input (bad email, etc.)
- [x] Audit log entry created for each update
- [x] Unit tests pass: `go test ./internal/controller/repository/... -run TestCustomerUpdate`

**Dependencies:** None
**Blocks:** Customer profile management functionality

**QA Scenarios:**
```bash
# Scenario 1: Update persists
curl -X PATCH http://localhost:8080/api/v1/customer/profile \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email":"new@example.com"}'
# Expected: 200 OK, change persisted in DB

# Scenario 2: Invalid email rejected
curl -X PATCH http://localhost:8080/api/v1/customer/profile \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email":"not-an-email"}'
# Expected: 400 Bad Request
```

---

## Phase 1 Gate: Security Verification

**Before proceeding to Phase 2, verify:**

```bash
# 1. All CRITICAL issues resolved
grep -r "TODO\|FIXME\|PLACEHOLDER\|WARNING.*production" \
  --include="*.go" --include="*.ts" --include="*.tsx" \
  internal/controller/tasks/ internal/controller/api/provisioning/ \
  internal/controller/services/auth_service.go \
  webui/admin/app/login/ webui/customer/app/login/

# Expected: No matches for security-related markers

# 2. Backend builds and tests pass
go build ./... && go test ./... -race

# 3. Frontend builds
cd webui/admin && npm run build
cd ../customer && npm run build

# 4. Auth flow works end-to-end
npx playwright test tests/e2e/auth-flow.spec.ts
```

---

## Phase 2: Core Backend Features [HIGH]

**Status:** BLOCKED by Phase 1
**Duration:** 3 weeks
**Dependencies:** Phase 1 complete

### TODO 2.1: Implement Node Failover

**File:** `internal/controller/services/node_service.go:217-219`

**Current State:**
```go
func (s *NodeService) handleNodeFailure(ctx context.Context, node *models.Node) {
    // TODO: Trigger alert notification
    // TODO: Attempt VM migration to other nodes
    // TODO: Attempt IPMI power cycle if configured
}
```

**Required Changes:**
- Implement alert notification service:
  - Email notifications (via SMTP or existing service)
  - Webhook notifications
  - Telegram/Discord integration (optional)
- Implement VM migration logic:
  - Find healthy nodes with capacity
  - Migrate VMs using libvirt live migration
  - Update VM records in database
- Implement IPMI power cycle:
  - Connect to IPMI interface
  - Send power cycle command
  - Log attempt and result
- Add circuit breaker pattern:
  - Prevent failover flapping
  - Cooldown period between attempts
  - Max retry count

**Acceptance Criteria:**
- [x] Node failure triggers alert within 30 seconds
- [x] VMs auto-migrate to healthy node if available
- [x] IPMI power cycle attempted if configured
- [x] Circuit breaker prevents flapping
- [x] All actions logged to audit table
- [x] Notification service handles failures gracefully

**Dependencies:** Phase 1 complete
**Blocks:** High availability functionality

**QA Scenarios:**
```bash
# Scenario 1: Failure detection
# Simulate node failure, verify alert sent within 30s

# Scenario 2: VM migration
# Simulate failure with running VMs, verify migration initiated

# Scenario 3: Circuit breaker
# Trigger rapid failures, verify cooldown enforced
```

---

### TODO 2.2: Implement VM Migration

**File:** `internal/controller/api/admin/vms.go:340-348`

**Current State:**
```go
func (h *AdminHandler) MigrateVM(c *gin.Context) {
    // Returns success without actually migrating
    c.JSON(http.StatusOK, models.Response{
        Data: gin.H{"message": "VM migration initiated"},
    })
}
```

**Required Changes:**
- Create migration service: `internal/controller/services/migration_service.go`
- Implement pre-migration checks:
  - Target node has sufficient resources
  - Network compatibility (same VLAN/storage)
  - Storage availability
- Implement live migration via libvirt:
  - `virsh migrate --live`
  - Progress tracking
  - State preservation
- Implement rollback on failure:
  - Cancel migration
  - Resume VM on source if possible
  - Log failure reason
- Update migration status in database
- Add migration history tracking

**Acceptance Criteria:**
- [x] VM actually moves to target node
- [x] VM state preserved during migration
- [x] Pre-migration checks prevent invalid migrations
- [x] Rollback works if target lacks resources
- [x] Migration progress trackable via API
- [x] Migration history preserved

**Dependencies:** TODO 2.1 (failover uses migration)
**Blocks:** Live migration functionality

**QA Scenarios:**
```bash
# Scenario 1: Successful migration
curl -X POST http://localhost:8080/api/v1/admin/vms/{id}/migrate \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"target_node_id":"node-2"}'
# Expected: 202 Accepted, VM moves to node-2

# Scenario 2: Pre-migration check fails (insufficient resources)
# Target node has insufficient RAM
# Expected: 400 Bad Request with reason

# Scenario 3: Rollback on failure
# Simulate migration failure
# Expected: VM returns to source node
```

---

### TODO 2.3: Implement Template Updates

**File:** `internal/controller/api/admin/templates.go:186-189`

**Current State:**
```go
func (h *AdminHandler) UpdateTemplate(c *gin.Context) {
    s.logger.Info("template update", ...)
    return models.TemplateUpdate{}, fmt.Errorf("template update not yet implemented")
}
```

**Required Changes:**
- Implement `templateRepo.Update` method
- Add template versioning:
  - Increment version on each update
  - Store version history
  - Allow rollback to previous version
- Implement audit logging for template changes
- Add validation for template fields

**Acceptance Criteria:**
- [x] Template changes persist to database
- [x] Version incremented on each update
- [x] Audit trail exists for all changes
- [x] Validation rejects invalid template data
- [x] Version history preserved

**Dependencies:** None
**Blocks:** Template management functionality

**QA Scenarios:**
```bash
# Scenario 1: Update persists
curl -X PATCH http://localhost:8080/api/v1/admin/templates/{id} \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"ubuntu-24.04-v2"}'
# Expected: 200 OK, change persisted, version incremented

# Scenario 2: Version history tracked
# Update template multiple times
# Expected: All versions accessible via history endpoint
```

---

### TODO 2.4: Implement API Key Management

**File:** `internal/controller/api/customer/apikeys.go:38-49`

**Current State:**
```go
func (h *CustomerHandler) ListAPIKeys(c *gin.Context) {
    keys := []APIKeyResponse{}
    c.JSON(http.StatusOK, models.ListResponse{Data: keys})
}
```

**Required Changes:**
- Create migration: `migrations/000012_customer_api_keys.up.sql`
- Create down migration: `migrations/000012_customer_api_keys.down.sql`
- Table structure:
  ```sql
  CREATE TABLE customer_api_keys (
      id UUID PRIMARY KEY,
      customer_id UUID NOT NULL REFERENCES customers(id),
      name VARCHAR(100) NOT NULL,
      key_hash VARCHAR(128) NOT NULL,
      key_prefix VARCHAR(8) NOT NULL,  -- First 8 chars for identification
      permissions JSONB NOT NULL DEFAULT '{}',
      last_used_at TIMESTAMP,
      expires_at TIMESTAMP,
      created_at TIMESTAMP NOT NULL DEFAULT NOW(),
      revoked_at TIMESTAMP
  );
  ```
- Implement CRUD operations:
  - `CreateAPIKey(ctx, customerID, name, permissions) -> (key, error)`
  - `ListAPIKeys(ctx, customerID) -> []APIKey`
  - `RevokeAPIKey(ctx, keyID) -> error`
  - `ValidateAPIKey(ctx, key) -> (*APIKey, error)`
- Hash keys before storage (show full key only once on creation)
- Implement permission system (read, write, admin scopes)

**Acceptance Criteria:**
- [x] Customer can create API keys
- [x] Keys displayed in list with prefix (not full key)
- [x] Full key shown only once on creation
- [x] Keys hashed in database
- [x] Customer can revoke keys
- [x] Permissions enforced on API calls
- [x] Key expiration enforced

**Dependencies:** Phase 1.4 (provisioning API auth)
**Blocks:** Customer API access functionality

**QA Scenarios:**
```bash
# Scenario 1: Create key
curl -X POST http://localhost:8080/api/v1/customer/api-keys \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"CI/CD Key","permissions":{"vms":["read","reboot"]}}'
# Expected: 201 Created, full key returned once

# Scenario 2: List keys (no full key)
curl http://localhost:8080/api/v1/customer/api-keys \
  -H "Authorization: Bearer $TOKEN"
# Expected: Keys listed with prefix only, no full key

# Scenario 3: Revoke key
curl -X DELETE http://localhost:8080/api/v1/customer/api-keys/{id} \
  -H "Authorization: Bearer $TOKEN"
# Expected: 204 No Content, key no longer works
```

---

### TODO 2.5: Implement Snapshot Operations

**File:** `internal/controller/api/customer/snapshots.go:206-218`

**Current State:**
```go
func (h *CustomerHandler) RevertSnapshot(c *gin.Context) {
    h.logger.Info("snapshot restore initiated", ...)
    c.JSON(http.StatusAccepted, models.Response{...})
}
```

**Required Changes:**
- Connect to backup service (NATS task queue)
- Implement snapshot creation:
  - Call libvirt snapshot API
  - Store snapshot metadata in DB
  - Track progress via NATS
- Implement snapshot revert:
  - Validate VM state
  - Call libvirt revert API
  - Update VM state
- Implement snapshot deletion:
  - Remove from libvirt
  - Delete metadata from DB
  - Free storage
- Implement quota enforcement:
  - Check snapshot count limit
  - Check storage size limit
- Add progress tracking endpoint

**Acceptance Criteria:**
- [x] Snapshot actually created on disk
- [x] Revert restores VM to snapshot state
- [x] Delete removes snapshot from storage
- [x] Quota prevents exceeding limits
- [x] Progress trackable via API
- [x] All operations logged

**Dependencies:** NATS task system working
**Blocks:** Backup/snapshot functionality

**QA Scenarios:**
```bash
# Scenario 1: Create snapshot
curl -X POST http://localhost:8080/api/v1/customer/vms/{id}/snapshots \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"before-update"}'
# Expected: 202 Accepted, snapshot created on disk

# Scenario 2: Quota enforcement
# Create max snapshots, try one more
# Expected: 403 Forbidden with quota message

# Scenario 3: Revert snapshot
curl -X POST http://localhost:8080/api/v1/customer/snapshots/{id}/revert \
  -H "Authorization: Bearer $TOKEN"
# Expected: 202 Accepted, VM reverted
```

---

### TODO 2.6: Implement Node Metrics (Disk & Ceph)

**File:** `internal/nodeagent/server.go:226,231,290-291`

**Current State:**
```go
DiskPercent: 0, // TODO: Implement disk usage calculation
TotalDiskGB: 0,
UsedDiskGB: 0,
CephConnected: false, // TODO: Implement Ceph connection check
```

**Required Changes:**
- Implement disk usage calculation:
  - Use `syscall.Statfs` for disk stats
  - Calculate percentage: `(used / total) * 100`
  - Handle multiple mount points (pool storage)
- Implement Ceph connection health check:
  - Connect to Ceph monitor
  - Check cluster health status
  - Report `CephConnected: true/false`
  - Report Ceph pool usage if connected
- Add metric caching:
  - Cache for 5 seconds (TTL)
  - Refresh on cache expiry
  - Prevent excessive system calls

**Acceptance Criteria:**
- [x] Disk metrics return real values
- [x] Ceph status reflects actual connection state
- [x] Metrics cached for 5 seconds
- [x] Handles missing Ceph gracefully
- [x] Unit tests with mock filesystems

**Dependencies:** None
**Blocks:** Monitoring dashboards

**QA Scenarios:**
```bash
# Scenario 1: Disk metrics populated
curl http://localhost:50051/node/resources
# Expected: DiskPercent > 0, TotalDiskGB > 0, UsedDiskGB > 0

# Scenario 2: Ceph status accurate
# With Ceph configured:
# Expected: CephConnected: true
# Without Ceph:
# Expected: CephConnected: false (not error)
```

---

### TODO 2.7: Implement Frontend VM Controls

**Files:**
- `webui/customer/app/vms/page.tsx:120-135`
- `webui/customer/app/vms/[id]/page.tsx:130-145`

**Current State:**
```typescript
const handleStart = () => {
    console.log(`Starting VM: ${vmId}`);
    // TODO: Implement API call
};
// Similar for handleStop, handleForceStop, handleRestart
```

**Required Changes:**
- Create centralized API client: `webui/customer/lib/api-client.ts`
- Implement VM control methods:
  - `startVM(id)`
  - `stopVM(id)`
  - `forceStopVM(id)`
  - `restartVM(id)`
- Add loading states to buttons
- Add error handling with toast notifications
- Add confirmation dialog for destructive actions (stop, force-stop)
- Refresh VM status after action

**Acceptance Criteria:**
- [x] Each button triggers real API call
- [x] Loading spinner shown during operation
- [x] Error toast displayed on failure
- [x] Confirmation dialog for stop/force-stop
- [x] VM status refreshes after action
- [x] Success toast on completion

**Dependencies:** TODO 1.3 (customer auth), API endpoints working
**Blocks:** Customer VM management

**QA Scenarios:**
```bash
# Playwright tests
npx playwright test tests/e2e/vm-controls.spec.ts

# Scenario 1: Start VM
# Click Start button
# Expected: Loading state, then VM running

# Scenario 2: Stop VM with confirmation
# Click Stop button
# Expected: Confirmation dialog appears
# Confirm: VM stops

# Scenario 3: Error handling
# Stop VM that's already stopped
# Expected: Error toast shown
```

---

### TODO 2.8: Implement Admin Actions

**Files:**
- `webui/admin/app/nodes/page.tsx:199,205,212`
- `webui/admin/app/customers/page.tsx:146,152,159`
- `webui/admin/app/plans/page.tsx:162,172`

**Current State:**
```typescript
const handleView = (node: Node) => {
    console.log("View node:", node);
};
const handleDrain = (node: Node) => {
    console.log("Drain node:", node);
};
const handleFailover = (node: Node) => {
    console.log("Failover node:", node);
};
```

**Required Changes:**
- Create admin API client: `webui/admin/lib/api-client.ts`
- Implement node actions:
  - `viewNode(id)` - Navigate to node details
  - `drainNode(id)` - Migrate all VMs off node
  - `failoverNode(id)` - Trigger failover
- Implement customer actions:
  - `suspendCustomer(id)` - Suspend account
  - `unsuspendCustomer(id)` - Reactivate account
  - `deleteCustomer(id)` - Delete account (with confirmation)
- Implement plan actions:
  - `editPlan(id)` - Navigate to edit form
  - `deletePlan(id)` - Delete plan (with confirmation)
- Add confirmation dialogs for destructive actions
- Add success/error feedback

**Acceptance Criteria:**
- [x] Each action calls real API endpoint
- [x] Confirmation dialogs for destructive actions
- [x] Success toast on completion
- [x] Error toast on failure
- [x] List refreshes after action

**Dependencies:** TODO 1.2 (admin auth), backend endpoints working
**Blocks:** Admin management functionality

**QA Scenarios:**
```bash
npx playwright test tests/e2e/admin-actions.spec.ts

# Scenario 1: Drain node
# Click Drain button
# Expected: Confirmation, then API call, then success toast

# Scenario 2: Suspend customer
# Click Suspend button
# Expected: Confirmation, then customer suspended

# Scenario 3: Delete with confirmation
# Click Delete button
# Cancel: No action
# Confirm: Item deleted
```

---

### TODO 2.9: Implement VM Management Tabs

**File:** `webui/customer/app/vms/[id]/page.tsx:320,341,359,377`

**Current State:**
```typescript
<CardContent className="p-6">
    <p className="text-muted-foreground">Console access placeholder</p>
</CardContent>
// Similar for Backups, Snapshots, Settings tabs
```

**Required Changes:**
- **Console Tab:**
  - Integrate noVNC client
  - Connect to WebSocket proxy
  - Add authentication for console session
  - Display connection status
- **Backups Tab:**
  - List existing backups
  - Create backup button
  - Restore backup button
  - Delete backup button
  - Schedule configuration
- **Snapshots Tab:**
  - List existing snapshots
  - Create snapshot button
  - Revert snapshot button
  - Delete snapshot button
- **Settings Tab:**
  - VM name/description edit
  - Resource adjustment (if allowed)
  - Network configuration
  - Rescue mode toggle

**Acceptance Criteria:**
- [x] Console tab opens working VNC session
- [x] Backup tab shows real backups with CRUD controls
- [x] Snapshot tab shows real snapshots with controls
- [x] Settings tab allows config changes
- [x] All tabs handle errors gracefully

**Dependencies:** TODO 2.5 (snapshots), TODO 2.7 (VM controls), backend APIs
**Blocks:** Full VM management UI

**QA Scenarios:**
```bash
npx playwright test tests/e2e/vm-tabs.spec.ts

# Scenario 1: Console connection
# Click Console tab
# Expected: VNC client loads, shows VM screen

# Scenario 2: Backup list
# Click Backups tab
# Expected: List of backups displayed

# Scenario 3: Settings change
# Edit VM name in Settings
# Expected: Change persisted
```

---

### TODO 2.10: Create Missing Down Migration

**File:** `migrations/000010_webhooks.down.sql` (MISSING)

**Required Changes:**
Create the down migration file:
```sql
-- 000010_webhooks.down.sql
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;
```

**Acceptance Criteria:**
- [x] File exists at `migrations/000010_webhooks.down.sql`
- [x] `migrate down` from step 10 succeeds
- [x] `migrate up` after down succeeds

**Dependencies:** None
**Blocks:** Migration rollback capability

**QA Scenarios:**
```bash
# Scenario 1: Down migration works
make migrate-down VERSION=9
# Expected: Success, webhooks tables dropped

# Scenario 2: Up migration after down
make migrate-up
# Expected: Success, webhooks tables recreated
```

---

### TODO 2.11: Fix Hardcoded Audit Log Partitions

**File:** `migrations/000001_initial_schema.up.sql:167-172`

**Current State:**
```sql
-- Hardcoded partitions for March and April 2026
CREATE TABLE audit_logs_2026_03 PARTITION OF audit_logs ...
CREATE TABLE audit_logs_2026_04 PARTITION OF audit_logs ...
```

**Required Changes:**
Option A (Quick): Add partitions through 2027:
```sql
-- Add partitions through 2027
CREATE TABLE audit_logs_2026_05 PARTITION OF audit_logs ...
-- ... continue for all months through 2027_12
```

Option B (Better): Implement automatic partition creation:
- Create PostgreSQL function to auto-create partitions
- Create cron job or pg_partman configuration
- Add monitoring for partition exhaustion

**Acceptance Criteria:**
- [x] Audit logging works beyond April 2026
- [x] Partitions exist through at least 2027
- [x] OR automatic partition creation tested
- [x] No runtime errors for missing partitions

**Dependencies:** None
**Blocks:** Long-term audit logging

**QA Scenarios:**
```bash
# Scenario 1: Future dates work
# Insert audit log with future timestamp
# Expected: No partition error

# Scenario 2: Partition coverage
SELECT * FROM audit_logs WHERE created_at > '2027-01-01';
# Expected: No error (partition exists)
```

---

## Phase 2 Gate: Core Features Verification

**Before proceeding to Phase 3, verify:**

```bash
# 1. All HIGH issues resolved
grep -r "TODO\|FIXME\|PLACEHOLDER" \
  --include="*.go" --include="*.ts" --include="*.tsx" \
  internal/controller/services/ internal/nodeagent/ \
  webui/customer/app/vms/ webui/admin/app/

# Expected: No markers in listed files

# 2. Backend builds and tests pass
go build ./... && go test ./... -race

# 3. Frontend builds
cd webui/admin && npm run build
cd ../customer && npm run build

# 4. E2E tests pass
npx playwright test tests/e2e/
```

---

## Phase 3: API Layer & Data [MEDIUM]

**Status:** BLOCKED by Phase 2
**Duration:** 2 weeks
**Dependencies:** Phase 2 complete

### TODO 3.1: Replace Placeholder Network Metrics

**File:** `internal/controller/api/customer/metrics.go:143-195`

**Current State:**
```go
func generatePlaceholderNetworkData(period string) []NetworkPoint {
    // Generates fake RxBytes/TxBytes
}
```

**Required Changes:**
- Connect to time-series database (Prometheus/InfluxDB)
- Implement real bandwidth aggregation queries
- Add data retention policies
- Implement caching for frequently accessed data
- Fallback to node-agent aggregated data if TSDB unavailable

**Acceptance Criteria:**
- [x] Charts display real bandwidth data
- [x] Historical data queryable
- [x] Data retention configured
- [x] Performance acceptable (<500ms query time)

**Dependencies:** None
**Blocks:** Accurate monitoring

**QA Scenarios:**
```bash
# Scenario 1: Real data returned
curl http://localhost:8080/api/v1/customer/vms/{id}/metrics/network?period=24h
# Expected: Real bandwidth values, not placeholder

# Scenario 2: Performance
# Query should complete in <500ms
```

---

### TODO 3.2: Implement RBAC Re-auth Check

**File:** `internal/controller/services/rbac_service.go:147-156`

**Current State:**
```go
func (s *RBACService) RequireReauthForDestructive(ctx context.Context, userID, action string) (bool, error) {
    // Always returns true without checking timestamp
}
```

**Required Changes:**
- Track last re-auth timestamp in session metadata
- Implement 5-minute re-auth window check
- Store re-auth time in `sessions` table or JWT claims
- Return false if within window, true if re-auth needed

**Acceptance Criteria:**
- [x] Destructive actions within 5 min of re-auth proceed
- [x] Older sessions prompt for re-auth
- [x] Timestamp stored securely
- [x] Works across session refreshes

**Dependencies:** Session management
**Blocks:** Security compliance

**QA Scenarios:**
```bash
# Scenario 1: Fresh session requires re-auth
# Login, wait 6 minutes, attempt destructive action
# Expected: Re-auth required

# Scenario 2: Recent re-auth allows action
# Re-authenticate, immediately perform destructive action
# Expected: Action allowed
```

---

### TODO 3.3: Implement Provisioning Password Update

**File:** `internal/controller/api/provisioning/password.go:91-99`

**Current State:**
```go
// Returns success without DB update
return models.Response{Data: gin.H{"message": "Password updated successfully"}}
```

**Required Changes:**
- Implement `vmRepo.UpdatePassword(ctx, vmID, encryptedPassword)`
- Add password validation
- Encrypt password before storage
- Invalidate old password

**Acceptance Criteria:**
- [x] Password actually updated in database
- [x] Old password invalidated
- [x] New password validated for strength
- [x] Password encrypted before storage

**Dependencies:** TODO 1.1 (password hashing)
**Blocks:** VNC/console password management

**QA Scenarios:**
```bash
# Scenario 1: Password updated
curl -X POST http://localhost:8080/vms/{id}/password \
  -H "X-API-Key: $KEY" \
  -d '{"password":"NewSecure123!"}'
# Expected: 200 OK, password changed in DB
```

---

### TODO 3.4: Generate gRPC Protobuf Code

**File:** `internal/nodeagent/server.go:134`

**Current State:**
```go
// TODO: Register the generated proto service when available.
```

**Required Changes:**
- Ensure proto files exist in `proto/`
- Generate Go code: `protoc --go_out=. --go-grpc_out=. proto/*.proto`
- Register generated service
- Remove manual message definitions
- Update imports

**Acceptance Criteria:**
- [x] `make proto` generates code successfully
- [x] Generated types match proto definitions
- [x] Manual structs deleted
- [x] gRPC service works with generated types

**Dependencies:** Proto files complete
**Blocks:** Type-safe gRPC

**QA Scenarios:**
```bash
# Scenario 1: Proto generation
make proto
# Expected: No errors, files generated

# Scenario 2: Build succeeds
go build ./...
# Expected: No compilation errors
```

---

### TODO 3.5: Implement VM Uptime Tracking

**File:** `internal/nodeagent/vm/lifecycle.go:438`

**Current State:**
```go
return 0, nil // Placeholder - in production, track VM start times
```

**Required Changes:**
- Track VM start timestamps (persist across agent restarts)
- Calculate uptime from start time
- Store start time in:
  - VM metadata in libvirt
  - OR local file
  - OR database
- Handle VM migrations (preserve uptime)

**Acceptance Criteria:**
- [x] Running VM shows accurate uptime
- [x] Uptime survives agent restart
- [x] Uptime preserved across migration
- [x] Stopped VM shows last uptime

**Dependencies:** None
**Blocks:** Accurate VM status

**QA Scenarios:**
```bash
# Scenario 1: Uptime accurate
# Start VM, wait 5 minutes
# Expected: Uptime ~300 seconds

# Scenario 2: Survives restart
# Restart node-agent
# Expected: Uptime still accurate (not reset to 0)
```

---

### TODO 3.6: Replace Route Placeholder Handler

**File:** `internal/controller/server.go:321-328`

**Current State:**
```go
func (s *Server) placeholderHandler(name string) gin.HandlerFunc {
    return func(c *gin.Context) {
        respondJSON(c, http.StatusOK, gin.H{
            "message": fmt.Sprintf("%s API - coming in Phase 2", name),
        })
    }
}
```

**Required Changes:**
- Either implement the actual routes
- Or return proper 404/501 errors
- Add dev flag to hide unfinished routes
- Document unimplemented endpoints

**Acceptance Criteria:**
- [x] No route returns "coming in Phase 2" in production mode
- [x] Unimplemented routes return 501 Not Implemented
- [x] Or proper 404 for truly missing routes
- [x] Dev mode shows helpful messages

**Dependencies:** Route implementation decisions
**Blocks:** Professional API responses

**QA Scenarios:**
```bash
# Scenario 1: Production mode
APP_ENV=production go run ./cmd/controller
curl http://localhost:8080/api/v1/unimplemented
# Expected: 501 Not Implemented or 404
```

---

### TODO 3.7: Implement IP Set Creation

**File:** `webui/admin/app/ip-sets/page.tsx:169`

**Current State:**
```typescript
const handleCreate = () => {
    console.log("Create new IP set");
};
```

**Required Changes:**
- Create IP set dialog component
- Connect to backend API
- Add validation for IP ranges
- Add success/error feedback

**Acceptance Criteria:**
- [x] IP sets can be created from admin UI
- [x] Dialog validates IP format
- [x] Success toast on creation
- [x] Error toast on failure

**Dependencies:** Backend IP set API working
**Blocks:** IP set management

**QA Scenarios:**
```bash
npx playwright test tests/e2e/ip-sets.spec.ts
# Create IP set, verify in list
```

---

### TODO 3.8: Implement CSV Export

**File:** `webui/admin/app/audit-logs/page.tsx:269`

**Current State:**
```typescript
const handleExportCSV = () => {
    console.log("Export CSV");
};
```

**Required Changes:**
- Implement CSV generation with current filters
- Trigger browser download
- Include all visible columns
- Handle large datasets (streaming or pagination)

**Acceptance Criteria:**
- [x] CSV downloads with correct filtered data
- [x] All visible columns included
- [x] Filename includes date and filters
- [x] Large exports don't timeout

**Dependencies:** None
**Blocks:** Audit log export

**QA Scenarios:**
```bash
# Scenario 1: CSV export
# Apply filters, click Export CSV
# Expected: File downloads with correct data
```

---

### TODO 3.9: Document Lint Suppressions

**Files:**
- `internal/controller/repository/ip_repo.go:276,327`
- `internal/controller/repository/node_repo.go:154`

**Current State:**
```go
defer tx.Rollback(ctx) //nolint:errcheck
```

**Required Changes:**
- Add explanatory comment for each suppression
- Log rollback errors at debug level
- Evaluate if suppressions are still needed
- Consider proper error handling instead

**Acceptance Criteria:**
- [x] Each `//nolint` has a comment explaining why
- [x] Rollback errors logged at debug level
- [x] OR proper error handling implemented

**Dependencies:** None
**Blocks:** Code quality

**QA Scenarios:**
```bash
# Scenario 1: Comments present
grep -B1 "nolint:errcheck" internal/controller/repository/*.go
# Expected: Comment above each suppression
```

---

### TODO 3.10: Remove Hardcoded Default Passwords

**Files:**
- `docker-compose.yml:19`
- `Makefile:24`
- `.env.example:7`

**Current State:**
```yaml
POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-changeme}
```

**Required Changes:**
- Remove default fallbacks from production configs
- Require explicit configuration
- Add startup validation to prevent weak passwords
- Update `.env.example` to document required fields without defaults

**Acceptance Criteria:**
- [x] No "changeme" defaults in docker-compose.yml, Makefile
- [x] `.env.example` documents but doesn't default
- [x] App refuses to start with weak passwords
- [x] Clear error message for missing config

**Dependencies:** None
**Blocks:** Security compliance

**QA Scenarios:**
```bash
# Scenario 1: No default passwords
grep -r "changeme" docker-compose.yml Makefile
# Expected: No matches

# Scenario 2: Startup validation
POSTGRES_PASSWORD=changeme go run ./cmd/controller
# Expected: Error about weak password
```

---

## Phase 3 Gate: API & Data Verification

```bash
# 1. All MEDIUM issues resolved
grep -r "TODO\|FIXME\|PLACEHOLDER" \
  --include="*.go" --include="*.ts" --include="*.tsx" \
  internal/controller/api/customer/metrics.go \
  internal/controller/services/rbac_service.go \
  internal/controller/api/provisioning/password.go \
  internal/nodeagent/server.go \
  internal/nodeagent/vm/lifecycle.go \
  internal/controller/server.go

# Expected: No matches

# 2. Builds and tests pass
go build ./... && go test ./... -race
cd webui/admin && npm run build
cd ../customer && npm run build
```

---

## Phase 4: Cleanup & Testing [LOW]

**Status:** BLOCKED by Phase 3
**Duration:** 1 week
**Dependencies:** All previous phases complete

### TODO 4.1: Fix Test Hardcoded Credentials

**Files:**
- `tests/integration/auth_test.go`
- `tests/e2e/*.spec.ts`

**Required Changes:**
- Create test fixtures/factories for credentials
- Document as test-only
- Add pre-commit hook to catch hardcoded passwords
- Use environment variables for test credentials

**Acceptance Criteria:**
- [x] No hardcoded passwords in test files
- [x] Test fixtures generate credentials
- [x] Pre-commit hook catches hardcoded secrets
- [x] Documentation updated

**Dependencies:** None
**Blocks:** Security best practices

---

### TODO 4.2: Remove Console.log Statements

**Scope:** 25+ instances across frontend

**Required Changes:**
- Remove or replace with structured logger
- Add eslint rule `no-console`
- Keep only intentional console usage (error boundaries, etc.)

**Acceptance Criteria:**
- [x] No `console.log` in production code
- [x] ESLint rule `no-console: "error"` in eslint config
- [x] Exception comments for intentional usage

**Dependencies:** None
**Blocks:** Code quality

---

### TODO 4.3: Review Placeholder Form Text

**Scope:** Multiple form inputs

**Required Changes:**
- Review all placeholder strings
- Localize if i18n is implemented
- Ensure placeholders are helpful and accurate
- Remove placeholder email/names

**Acceptance Criteria:**
- [x] No "you@example.com" placeholders
- [x] No "John Doe" placeholder names
- [x] Placeholders are helpful and accurate

**Dependencies:** None
**Blocks:** UX quality

---

## Phase 4 Gate: Final Verification

```bash
# Final cleanup verification
grep -r "TODO\|FIXME\|PLACEHOLDER\|STUB\|changeme\|console\.log" \
  --include="*.go" --include="*.ts" --include="*.tsx" \
  --exclude-dir=node_modules \
  --exclude-dir=.git \
  --exclude="*_test.go" \
  --exclude="*.spec.ts" \
  .

# Expected: Zero matches in non-test source files

# Build verification
go build ./... && go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total
# Expected: >80% coverage

# Frontend verification
cd webui/admin && npm run build && npm run lint
cd ../customer && npm run build && npm run lint
# Expected: Zero errors

# E2E verification
npx playwright test
# Expected: All tests pass
```

---

## Summary

### Total TODOs by Phase

| Phase | TODOs | Severity | Duration |
|-------|-------|----------|----------|
| 1 | 6 | CRITICAL | 2 weeks |
| 2 | 11 | HIGH | 3 weeks |
| 3 | 10 | MEDIUM | 2 weeks |
| 4 | 3 | LOW | 1 week |
| **TOTAL** | **30** | | **8 weeks** |

### Dependency Graph

```
Phase 1 (Security)
    ├── 1.1 Password Hashing ─┬─→ 1.2 Admin Auth
    │                         ├─→ 1.3 Customer Auth
    │                         └─→ 1.5 Password Reset
    ├── 1.4 Provisioning API ────→ 2.4 API Keys
    └── 1.6 Customer Updates ────→ (unblocks customer management)

Phase 2 (Core Features) [BLOCKED by Phase 1]
    ├── 2.1 Node Failover ───────→ 2.2 VM Migration
    ├── 2.3 Template Updates
    ├── 2.5 Snapshots
    ├── 2.6 Node Metrics
    ├── 2.7 VM Controls ──────────→ 2.9 VM Tabs
    ├── 2.8 Admin Actions
    ├── 2.10 Down Migration
    └── 2.11 Audit Partitions

Phase 3 (API/Data) [BLOCKED by Phase 2]
    ├── 3.1 Network Metrics
    ├── 3.2 RBAC Re-auth
    ├── 3.3 Password Update ─────→ 1.1 (depends)
    ├── 3.4 gRPC Proto
    ├── 3.5 VM Uptime
    ├── 3.6 Route Placeholder
    ├── 3.7 IP Set Creation
    ├── 3.8 CSV Export
    ├── 3.9 Lint Suppressions
    └── 3.10 Default Passwords

Phase 4 (Cleanup) [BLOCKED by Phase 3]
    ├── 4.1 Test Credentials
    ├── 4.2 Console.log
    └── 4.3 Placeholder Text
```

### Verification Commands

```bash
# Complete verification suite
make verify-all:
    # Backend
    go build ./...
    go test ./... -race -coverprofile=coverage.out
    go tool cover -func=coverage.out | grep total

    # Frontend
    cd webui/admin && npm run build && npm run lint
    cd ../customer && npm run build && npm run lint

    # E2E
    npx playwright test

    # Security markers
    grep -r "TODO\|FIXME\|PLACEHOLDER\|STUB\|WARNING.*production\|changeme" \
      --include="*.go" --include="*.ts" --include="*.tsx" \
      --exclude="*_test.go" --exclude="*.spec.ts" \
      . || echo "No markers found - PASS"
```

---

## Appendix: File Change Summary

### Backend Files (Go)

| File | Phase | Change Type |
|------|-------|-------------|
| `internal/controller/tasks/handlers.go` | 1 | Fix password hashing |
| `internal/controller/api/provisioning/routes.go` | 1 | Add auth middleware |
| `internal/controller/api/provisioning/middleware/auth.go` | 1 | NEW - API key auth |
| `internal/controller/grpc_client.go` | 1 | Remove warnings |
| `internal/controller/services/auth_service.go` | 1 | Implement password reset |
| `internal/controller/services/customer_service.go` | 1 | Implement update |
| `internal/controller/repository/customer_repo.go` | 1 | Add Update method |
| `internal/controller/services/node_service.go` | 2 | Implement failover |
| `internal/controller/services/migration_service.go` | 2 | NEW - VM migration |
| `internal/controller/api/admin/vms.go` | 2 | Wire migration |
| `internal/controller/api/admin/templates.go` | 2 | Implement update |
| `internal/controller/repository/template_repo.go` | 2 | Add Update method |
| `internal/controller/api/customer/apikeys.go` | 2 | Implement CRUD |
| `internal/controller/repository/apikey_repo.go` | 2 | NEW - API key repo |
| `internal/controller/api/customer/snapshots.go` | 2 | Implement operations |
| `internal/nodeagent/server.go` | 2 | Add disk/ceph metrics |
| `internal/controller/api/customer/metrics.go` | 3 | Real network data |
| `internal/controller/services/rbac_service.go` | 3 | Re-auth check |
| `internal/controller/api/provisioning/password.go` | 3 | Implement update |
| `internal/nodeagent/vm/lifecycle.go` | 3 | Uptime tracking |
| `internal/controller/server.go` | 3 | Fix placeholder handler |

### Frontend Files (TypeScript/React)

| File | Phase | Change Type |
|------|-------|-------------|
| `webui/admin/lib/api-client.ts` | 1 | NEW - API client |
| `webui/admin/lib/auth-context.tsx` | 1 | NEW - Auth context |
| `webui/admin/lib/types/auth.ts` | 1 | NEW - Auth types |
| `webui/admin/app/login/page.tsx` | 1 | Connect to API |
| `webui/customer/lib/api-client.ts` | 1 | NEW - API client |
| `webui/customer/lib/auth-context.tsx` | 1 | NEW - Auth context |
| `webui/customer/lib/types/auth.ts` | 1 | NEW - Auth types |
| `webui/customer/app/login/page.tsx` | 1 | Connect to API |
| `webui/customer/app/vms/page.tsx` | 2 | Implement controls |
| `webui/customer/app/vms/[id]/page.tsx` | 2 | Implement tabs + controls |
| `webui/admin/app/nodes/page.tsx` | 2 | Implement actions |
| `webui/admin/app/customers/page.tsx` | 2 | Implement actions |
| `webui/admin/app/plans/page.tsx` | 2 | Implement actions |
| `webui/admin/app/ip-sets/page.tsx` | 3 | Implement creation |
| `webui/admin/app/audit-logs/page.tsx` | 3 | Implement CSV export |

### Database Migrations

| File | Phase | Purpose |
|------|-------|---------|
| `migrations/000011_password_resets.up.sql` | 1 | NEW - Password reset tokens |
| `migrations/000011_password_resets.down.sql` | 1 | NEW - Rollback |
| `migrations/000012_customer_api_keys.up.sql` | 2 | NEW - Customer API keys |
| `migrations/000012_customer_api_keys.down.sql` | 2 | NEW - Rollback |
| `migrations/000010_webhooks.down.sql` | 2 | NEW - Missing down migration |
| `migrations/000001_initial_schema.up.sql` | 2 | Extend audit partitions |

### Configuration Files

| File | Phase | Change |
|------|-------|--------|
| `docker-compose.yml` | 1, 3 | Remove default passwords |
| `Makefile` | 1, 3 | Remove default passwords |
| `.env.example` | 1, 3 | Document required fields |
| `eslint.config.js` | 4 | Add no-console rule |

---

*Plan generated from CODEBASE_AUDIT_REPORT.md - Complete remediation of all 83+ issues.*