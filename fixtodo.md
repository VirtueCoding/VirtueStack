# VirtueStack Fix Plan — Full Execution Checklist

> Generated from comprehensive audit. All findings validated against source code.
> 27 false positives identified and excluded. ~333 validated findings remain.
> Each item has a `[ ]` checkbox. An LLM agent should check off items as they are completed.
> Follow CODING_STANDARD.md and WORKFLOW.md for all changes.
>
> ## FALSE POSITIVES EXCLUDED (validated, do NOT implement)
>
> ### Go Backend (8)
> - `node_agent_client.go` ReleaseConnection no-op — design uses persistent cached connections, not a pool; no code calls it
> - `webhook_repo.go` dynamic SQL — column names are hardcoded string literals, not user-controlled
> - `websocket.go` token not validated — auth is via JWT middleware; `token` param is VM ID for gRPC, not an auth credential
> - `websocket.go` errChan buffer size — `gCancel()` ensures at most one error is ever sent; buffer of 2 is larger than needed
> - `grpc_client.go` InsecureNodeClient no compile guard — only used with explicit env flag check, not a build issue
> - `customer.go` unused `_ = vms` — works correctly, just wasteful; a Count method is cleaner but not broken
> - `lifecycle.go` CPU sampler goroutine — VMs don't get deleted often; this is a very slow leak, not a bug
> - `server.go` `context.Background()` in pool stats — single-host Docker, practical risk is very low
>
> ### Infrastructure (12)
> - `ssl/key.pem` in git — only `ssl/.gitkeep` is tracked; actual keys are gitignored
> - `webui/*/nginx-app.conf` proxy to localhost — static file server config, not reverse proxy; `try_files` serves correctly
> - `.env.example` missing — file exists at `/home/VirtueStack/.env.example` (103 lines)
> - Docker env vars for secrets — standard Docker Compose pattern for non-Swarm deployments
> - CORS configuration — explicit origin allowlist (no `AllowAllOrigins`), not overly permissive
> - Migration 001 missing transaction — file has `BEGIN;` on line 4 and `COMMIT;` on line 436
> - Templates table RLS — admin-managed shared resource, not customer-isolated data
> - Docker network enable_icc — explicit config is defense-in-depth documentation
> - go.mod no toolchain directive — `GOTOOLCHAIN=local` intentionally set in Dockerfile
> - go.mod pkg/errors — transitive dependency only, no direct imports
> - go.mod mysql driver — intentionally used for PowerDNS MySQL direct access
> - Dockerfile no STOPSIGNAL — Go binaries handle SIGTERM correctly by default
>
> ### WHMCS (3)
> - `webhook.php` fragile init.php path — standard WHMCS module directory structure convention
> - `webhook.php` log rotation sort — ISO timestamp format (`Y-m-d-His`) is lexicographically sortable, `usort()` is correct
> - `overview.tpl` iframe lazy loading — valid UX optimization; browsers load immediately when in viewport
>
> ### Tests (8)
> - `suite_test.go` global mutable suite variable — standard Go `TestMain` pattern; variable is read-only after init
> - `vm_lifecycle_test.go` `assert.Empty(errors)` — assertion works correctly for nil/empty slice
> - `auth_2fa_test.go` mock2FACustomerRepo — `CustomerRepository` is a concrete struct, not an interface
> - `customer_repo_test.go` custom contains() — reinventing `strings.Contains` is style, not a correctness issue
> - `customer_repo_test.go` mockCustomerDB hardcoded index — sufficient for the single tested method
> - `vm_repo_test.go` VMRepository mock only mocks Exec — sufficient for `UpdatePassword` which only uses Exec
> - `auth_test.go` ChangePassword nil authService — intentional test isolation for JSON binding failure path
> - `playwright.config.ts` missing — file exists at `tests/e2e/playwright.config.ts` (187 lines)

---

## PHASE 1: CRITICAL — Security, Data Integrity, Build

### 1.1 Duplicate Migration Number
- File: `migrations/000020_add_attached_iso.up.sql` → rename to `000021`
- [ ] Rename `migrations/000020_add_attached_iso.up.sql` to `migrations/000021_add_attached_iso.up.sql`
- [ ] Rename `migrations/000020_add_attached_iso.down.sql` to `migrations/000021_add_attached_iso.down.sql`
- [ ] Verify no other duplicate migration numbers exist
- [ ] Confirm `migrations/` directory sorts correctly after rename

### 1.2 Missing RLS Policies on Customer-Facing Tables
- Tables: `notification_preferences`, `notification_events`, `password_resets`
- [ ] Create migration `000022_add_missing_rls_policies.up.sql`:
  - [ ] `ALTER TABLE notification_preferences ENABLE ROW LEVEL SECURITY;`
  - [ ] `CREATE POLICY customer_notification_preferences ON notification_preferences FOR ALL TO app_customer USING (customer_id = current_setting('app.current_customer_id')::UUID);`
  - [ ] `ALTER TABLE notification_events ENABLE ROW LEVEL SECURITY;`
  - [ ] `CREATE POLICY customer_notification_events ON notification_events FOR ALL TO app_customer USING (customer_id = current_setting('app.current_customer_id')::UUID);`
  - [ ] `ALTER TABLE password_resets ENABLE ROW LEVEL SECURITY;`
  - [ ] `CREATE POLICY customer_password_resets ON password_resets FOR ALL TO app_customer USING (customer_id = current_setting('app.current_customer_id')::UUID);`
- [ ] Create corresponding `000022_add_missing_rls_policies.down.sql`
- [ ] Verify `app_customer` role has proper access to these tables

### 1.3 Missing GRANT Permissions on New Tables
- Tables: `backup_schedules`, `failed_login_attempts`, `failover_requests`
- [ ] Add to migration `000022` (or separate migration):
  - [ ] `GRANT SELECT, INSERT, UPDATE, DELETE ON backup_schedules TO app_user;`
  - [ ] `GRANT SELECT, INSERT, UPDATE, DELETE ON failed_login_attempts TO app_user;`
  - [ ] `GRANT SELECT, INSERT, UPDATE, DELETE ON failover_requests TO app_user;`
  - [ ] `GRANT SELECT, INSERT, UPDATE ON backup_schedules TO app_customer;`
- [ ] Create corresponding down migration

### 1.4 CGO_ENABLED in Dockerfile
- File: `Dockerfile.controller`
- [ ] Note: Controller does NOT directly import go-ceph or libvirt-go — verify with `grep -r` that these are only used by Node Agent
- [ ] If Controller truly doesn't import CGO packages, CGO_ENABLED=0 is correct — verify and add a comment
- [ ] If Controller DOES import them (e.g., through shared packages), fix: set `CGO_ENABLED=1` and add required C library build deps (libvirt-dev, librbd-dev) to the build stage, and runtime deps to the runtime stage
- [ ] Ensure `Dockerfile.node-agent` (if exists) has proper CGO settings for go-ceph and libvirt-go

### 1.5 docker-compose Migration Mount
- File: `docker-compose.yml`
- [ ] Remove `./migrations:/docker-entrypoint-initdb.d:ro` from postgres service
- [ ] Add a `migrate` service or init command that runs `golang-migrate` properly (sequential up only)
- [ ] Document that migrations should be run via `make migrate-up` or a dedicated migration container, not via PG init

### 1.6 Audit Log Partition Exhaustion
- File: `migrations/000001_initial_schema.up.sql`, `migrations/000014_audit_log_partitions.up.sql`
- [ ] Create migration to add a DEFAULT partition: `CREATE TABLE audit_logs_default PARTITION OF audit_logs DEFAULT;`
- [ ] Document or add a cron job / pg_partman config for auto-creating monthly partitions
- [ ] Add a CHECK that new partitions are created at least 3 months ahead

### 1.7 Frontend Auth Ghost Session
- File: `webui/admin/lib/auth-context.tsx`
- [ ] In `initAuth()`, when no stored session exists but API call succeeds, do NOT set `isAuthenticated: true` with blank user
- [ ] Instead: fetch the admin profile via `/api/v1/admin/auth/profile` (or equivalent) to get real user data, OR set `isAuthenticated: false` and require re-login
- [ ] Same fix for `webui/customer/lib/auth-context.tsx` if applicable

### 1.8 Frontend Route-Level Auth Guards
- Files: `webui/admin/app/dashboard/page.tsx`, `webui/admin/app/nodes/page.tsx`, `webui/admin/app/plans/page.tsx`, `webui/admin/app/customers/page.tsx`, `webui/admin/app/ip-sets/page.tsx`, `webui/admin/app/audit-logs/page.tsx`
- Files: `webui/customer/app/vms/page.tsx`, `webui/customer/app/vms/[id]/page.tsx`, `webui/customer/app/settings/page.tsx`
- [ ] Create a shared `RequireAuth` wrapper component or `useRequireAuth()` hook that:
  - [ ] Reads `isAuthenticated` and `isLoading` from auth context
  - [ ] While loading: show skeleton/spinner
  - [ ] When not authenticated: redirect to `/login` using `router.push('/login')`
  - [ ] When authenticated: render children
- [ ] Wrap each protected page with this guard
- [ ] Alternatively: add the guard in the `dashboard/layout.tsx` and `vms/layout.tsx` files to protect all child routes

### 1.9 WHMCS CSRF — GET → POST for VM Power Operations
- File: `modules/servers/virtuestack/templates/overview.tpl`
- [ ] Replace all `<a href="...&modop=custom&a=startVM">` links with `<form method="POST">` with submit buttons
- [ ] Add WHMCS CSRF token field: `<input type="hidden" name="token" value="{$token}">`
- [ ] Add JavaScript confirmation dialog before submission for destructive actions (stop, restart)
- [ ] Apply same fix to all power operation links (start, stop, restart, console-open)

### 1.10 WHMCS Stored XSS
- File: `modules/servers/virtuestack/hooks.php`, line ~940
- [ ] In `formatProvisioningStatus()`, escape the default case output: `htmlspecialchars(ucfirst($status), ENT_QUOTES, 'UTF-8')`
- [ ] Verify all other output in hooks.php uses proper escaping

---

## PHASE 2: QCOW Backend Full Implementation

> The 14 stub gRPC client methods make QCOW storage completely non-functional.
> This phase implements all of them.

### 2.1 Proto: Add QCOW-Specific gRPC Methods
- File: `proto/virtuestack/node_agent.proto`
- [ ] Review existing proto to see which methods already exist vs which are missing
- [ ] Add QCOW-specific RPC methods if not already in proto:
  - [ ] `rpc CreateQCOWSnapshot(CreateQCOWSnapshotRequest) returns (QCOWSnapshotResponse);`
  - [ ] `rpc DeleteQCOWSnapshot(DeleteQCOWSnapshotRequest) returns (VMOperationResponse);`
  - [ ] `rpc CreateQCOWBackup(CreateQCOWBackupRequest) returns (BackupResponse);`
  - [ ] `rpc RestoreQCOWBackup(RestoreQCOWBackupRequest) returns (VMOperationResponse);`
  - [ ] `rpc DeleteQCOWBackupFile(DeleteQCOWBackupFileRequest) returns (VMOperationResponse);`
  - [ ] `rpc GetQCOWDiskInfo(GetQCOWDiskInfoRequest) returns (QCOWDiskInfoResponse);`
- [ ] Add corresponding Request/Response message types
- [ ] Run `make proto` to regenerate Go code

### 2.2 Node Agent: QCOW Storage Backend Methods
- File: `internal/nodeagent/storage/qcow.go`
- [ ] Implement `CloneFromTemplate(ctx, templateName, vmUUID, sizeGB)` — qemu-img create from template base
- [ ] Implement `CloneFromBackup(ctx, backupPath, vmUUID, sizeGB)` — qemu-img convert from backup
- [ ] Implement `Delete(ctx, imageName)` — qemu-img remove / os.Remove
- [ ] Implement `CreateQCOWSnapshot(ctx, diskPath, snapshotName)` — qemu-img snapshot -c
- [ ] Implement `DeleteQCOWSnapshot(ctx, diskPath, snapshotName)` — qemu-img snapshot -d
- [ ] Implement `CreateQCOWBackup(ctx, diskPath, backupPath)` — qemu-img convert -O qcow2 with compression
- [ ] Implement `RestoreQCOWBackup(ctx, backupPath, diskPath)` — qemu-img convert backup → disk
- [ ] Implement `DeleteQCOWBackupFile(ctx, backupPath)` — os.Remove
- [ ] Implement `GetQCOWDiskInfo(ctx, diskPath)` — qemu-img info parsing
- [ ] Implement `GenerateCloudInit(ctx, vmUUID, hostname, userData, networkConfig)` — generate ISO via cloud-init tooling
- [ ] Implement `ProtectSnapshot` / `UnprotectSnapshot` — qcow2 has no snapshot protection; implement as log warning or QCOW2 metadata flag
- [ ] Implement `CloneSnapshot` — qemu-img convert from snapshot
- [ ] Implement `GetVMNodeID()` — return this node's ID from config
- [ ] All methods must use proper error handling, timeouts, and context cancellation

### 2.3 Node Agent: QCOW gRPC Handlers
- File: `internal/nodeagent/grpc_handlers_extended.go` (or new file)
- [ ] Register all new QCOW RPC methods in the gRPC server
- [ ] Implement handler for each QCOW method that:
  - [ ] Validates request parameters
  - [ ] Calls the corresponding `storage/qcow.go` method
  - [ ] Returns proper gRPC responses with error mapping

### 2.4 Node Agent: Cloud-Init ISO Generation (QCOW Path)
- File: `internal/nodeagent/storage/cloudinit.go`
- [ ] Verify cloud-init ISO generation works for QCOW path (currently may only support Ceph path)
- [ ] Ensure ISO is written to the correct path for QCOW VMs
- [ ] Ensure ISO cleanup after first boot works for QCOW VMs

### 2.5 Controller: Implement 14 Stub gRPC Client Methods
- File: `internal/controller/services/node_agent_client.go`
- [ ] `CloneFromBackup(nodeID, backupPath, vmUUID, sizeGB)` — call gRPC `CreateQCOWBackup` or equivalent restore method
- [ ] `DeleteDisk(nodeID, vmUUID, storageBackend, diskPath)` — call gRPC `DeleteVM` or storage-specific delete
- [ ] `CloneFromTemplate(nodeID, templateName, vmUUID, sizeGB, storageBackend)` — call gRPC storage method
- [ ] `GenerateCloudInit(nodeID, vmUUID, hostname, rootPasswordHash, sshKeys, userData, networkConfig)` — call gRPC method
- [ ] `ProtectSnapshot(nodeID, vmUUID, snapshotName, storageBackend)` — implement or log for QCOW
- [ ] `UnprotectSnapshot(nodeID, vmUUID, snapshotName, storageBackend)` — implement or log for QCOW
- [ ] `CloneSnapshot(nodeID, vmUUID, snapshotName, newVMUUID, storageBackend)` — call gRPC snapshot clone
- [ ] `GetVMNodeID(nodeID)` — return the nodeID parameter directly (it IS the node ID)
- [ ] `CreateQCOWSnapshot(nodeID, vmUUID, snapshotName)` — call gRPC
- [ ] `DeleteQCOWSnapshot(nodeID, vmUUID, snapshotName)` — call gRPC
- [ ] `CreateQCOWBackup(nodeID, vmUUID, backupPath)` — call gRPC
- [ ] `RestoreQCOWBackup(nodeID, vmUUID, backupPath)` — call gRPC
- [ ] `DeleteQCOWBackupFile(nodeID, backupPath)` — call gRPC
- [ ] `GetQCOWDiskInfo(nodeID, vmUUID)` — call gRPC
- [ ] Each method must properly handle errors, timeouts, and context

### 2.6 Controller: Ceph Config for Migrations
- File: `internal/controller/services/node_agent_client.go`
- [ ] Fix `cephMonitors()`, `cephUser()`, `cephSecretUUID()` to return actual Ceph config
- [ ] These should come from the Controller's configuration or the Node's config (fetched via gRPC `GetNodeHealth` or similar)
- [ ] OR: change the migration flow so that the destination Node Agent handles Ceph config internally in `PrepareMigratedVM`, rather than the Controller passing it
- [ ] Verify migration works end-to-end for Ceph-backed VMs

### 2.7 Controller: ReinstallVM Fix — Preserve Resources
- File: `internal/nodeagent/grpc_handlers_extended.go`
- [ ] Update `ReinstallVMRequest` in proto to include `vcpu` and `memory_mb` fields
- [ ] Update the handler to use `req.GetVcpu()` and `req.GetMemoryMb()` instead of hardcoded 1/1024
- [ ] Update the controller's reinstall task (`internal/controller/tasks/vm_reinstall.go`) to pass vCPU and memory from the VM record
- [ ] Run `make proto` to regenerate
- [ ] Test reinstall preserves original resource allocation

### 2.8 Controller: Fix QCOW Paths in Task Handlers
- File: `internal/controller/tasks/vm_reinstall.go`
- [ ] When storage backend is QCOW, use the correct method calls (not Ceph-specific ones)
- [ ] Ensure disk path is computed correctly for QCOW: `/var/lib/virtuestack/vms/vs-{uuid}-disk0.qcow2`
- [ ] File: `internal/controller/tasks/handlers.go` (VM create)
- [ ] Same: use storage-backend-aware method calls for disk operations
- [ ] File: `internal/controller/tasks/backup_create.go`
- [ ] For QCOW: call QCOW-specific snapshot/backup methods
- [ ] File: `internal/controller/tasks/snapshot_handlers.go`
- [ ] For QCOW: call QCOW-specific snapshot methods

### 2.9 Controller: Domain XML QCOW Disk Path
- File: `internal/nodeagent/vm/domain_xml.go`
- [ ] Verify QCOW disk paths are generated correctly (file:// instead of rbd://)
- [ ] Verify cloud-init ISO path is correct for QCOW VMs
- [ ] Verify the disk XML structure for QCOW (type='file' instead of type='network')

### 2.10 Controller: Backup/Restore QCOW Support
- File: `internal/controller/services/backup_service.go`
- [ ] Fix the nil nodeAgent early-return that silently "completes" backups
- [ ] When storage backend is QCOW, use QCOW backup methods
- [ ] When storage backend is Ceph, use Ceph backup methods (RBD clone/snapshot)
- [ ] Ensure backup paths are correct for each backend

### 2.11 End-to-End QCOW Verification
- [ ] Trace the full VM create flow for QCOW: API → task → gRPC → storage → libvirt
- [ ] Trace the full VM reinstall flow for QCOW
- [ ] Trace the full backup create flow for QCOW
- [ ] Trace the full snapshot create/revert flow for QCOW
- [ ] Trace the full migration flow (if applicable for QCOW)
- [ ] Verify disk cleanup on VM deletion for QCOW

---

## PHASE 3: HIGH — Core Functionality Fixes

### 3.1 Prometheus Metrics Collector Fix
- File: `internal/controller/server.go`, lines ~592-617
- [ ] Replace `len(vms)` with the second return value from `vmRepo.List()` (the total count)
- [ ] Fix the discarded second return value: `vms, totalCount, err := vmRepo.List(...)`
- [ ] Use `totalCount` (not `len(vms)`) for `controllermetrics.VMsTotal.WithLabelValues(status).Set(float64(totalCount))`
- [ ] Apply same fix to node status counts (use second return value from nodeRepo.List)
- [ ] Verify Prometheus exposes correct counts via `/metrics` endpoint

### 3.2 Prometheus Memory Alert Expression
- File: `configs/prometheus/alerts.yml`, line 47
- [ ] Change `vs_vm_memory_usage_bytes / vs_vm_memory_usage_bytes > 0.9` to `vs_vm_memory_usage_bytes / vs_vm_memory_limit_bytes > 0.9`
- [ ] Verify the metric `vs_vm_memory_limit_bytes` exists in the Node Agent metrics
- [ ] If the metric name is different, adjust accordingly

### 3.3 Task Worker: Implement Concurrency
- File: `internal/controller/tasks/worker.go`
- [ ] Implement proper worker pool using `errgroup.SetLimit(numWorkers)`
- [ ] Remove `_ = numWorkers` and use the parameter
- [ ] Each NATS message handler should be launched in the errgroup
- [ ] Ensure `ManualAck` is used correctly — ack after handler completes, not before
- [ ] Add per-task timeout: wrap handler with `context.WithTimeout(ctx, 5*time.Minute)`
- [ ] Test that multiple tasks process concurrently

### 3.4 Swallowed Errors in Cleanup Paths
- Files: `services/backup_service.go`, `tasks/handlers.go`, `tasks/backup_create.go`, `tasks/snapshot_handlers.go`, `services/vm_service.go`, `services/failover_service.go`
- [ ] For each `_ = someOperation()` in cleanup paths:
  - [ ] Replace with proper error logging: `if err := someOperation(); err != nil { logger.Error("cleanup failed", "operation", "name", "err", err) }`
- [ ] Critical locations (must fix):
  - [ ] `backup_service.go:206` — backup status update when nodeAgent is nil → return error, not nil
  - [ ] `handlers.go:437` — disk cleanup on VM create failure → log error
  - [ ] `handlers.go:474-475` — disk + IP cleanup on VM create failure → log error
  - [ ] `vm_service.go:233,235` — soft delete + IP release on task publish failure → log error
  - [ ] `tasks/backup_create.go:169,201-202` — snapshot and backup file cleanup → log error
  - [ ] `tasks/migration_execute.go:133` — status revert on failure → log error
  - [ ] `services/failover_service.go:127` — status update during failover → log error
- [ ] Fix the backup_service.go nil nodeAgent case to return an error instead of silently succeeding

### 3.5 Frontend: Sidebar Logout Buttons
- File: `webui/admin/components/sidebar.tsx`
  - [ ] Import `useAuth` hook
  - [ ] Destructure `logout` from `useAuth()`
  - [ ] Add `onClick={() => logout()}` to the "Log out" `<DropdownMenuItem>`
- File: `webui/customer/components/sidebar.tsx`
  - [ ] Destructure `logout` from `useAuth()` (currently only destructures `user`)
  - [ ] Add `onClick={() => logout()}` to the "Log out" `<DropdownMenuItem>`

### 3.6 Frontend: Sidebar Route Mismatches
- File: `webui/admin/components/sidebar.tsx`
- [ ] Fix navigation hrefs:
  - [ ] `/dashboard/nodes` → `/nodes`
  - [ ] `/dashboard/plans` → `/plans`
  - [ ] `/dashboard/ip-sets` → `/ip-sets`
  - [ ] `/dashboard/customers` → `/customers`
  - [ ] `/dashboard/audit-logs` → `/audit-logs`
- [ ] OR: move page files under `app/dashboard/` subdirectory to match current routes
- [ ] Verify routes match the actual Next.js app directory structure
- [ ] Do the same check and fix for `webui/admin/components/mobile-nav.tsx`

### 3.7 Frontend: Customer Sidebar Route Mismatches
- File: `webui/customer/components/sidebar.tsx`
- [ ] Fix or remove links to non-existent routes:
  - [ ] `/vms/api-keys` — remove or change to point to settings page API keys tab
  - [ ] `/vms/billing` — remove (page doesn't exist)
- [ ] Fix "Account Settings" menu item to have proper navigation (`href="/settings"` or onClick)
- [ ] Do the same check for `webui/customer/components/mobile-nav.tsx`

### 3.8 Frontend: handleSaveSettings No-Op
- File: `webui/customer/app/vms/[id]/page.tsx`
- [ ] Add actual API call inside `handleSaveSettings`:
  - [ ] Check if VM update API endpoint exists in the customer API
  - [ ] Call the update API with the new VM name/settings
  - [ ] Only show success toast after API returns successfully
  - [ ] Show error toast if API call fails
  - [ ] If no customer VM update endpoint exists, disable/hide the settings editing UI until backend is added

### 3.9 Frontend: VNC Console Token Exposure
- File: `webui/customer/components/novnc-console/vnc-console.tsx`
- [ ] In the "connecting" state (lines ~415-424), remove or mask the `getWsUrl()` output
- [ ] Replace with a generic "Establishing VNC connection..." message without the URL

### 3.10 Frontend: tempToken in sessionStorage
- Files: `webui/admin/lib/auth-context.tsx`, `webui/customer/lib/auth-context.tsx`
- [ ] Remove `tempToken` from the serialized auth state in sessionStorage
- [ ] Keep `tempToken` only in React state (memory-only)
- [ ] Update the hydration/deserialization logic to not expect `tempToken` from sessionStorage
- [ ] If a page refresh occurs during 2FA flow, require re-login (this is expected and secure)

### 3.11 WHMCS: Webhook Handler GET → POST
- File: `modules/servers/virtuestack/hooks.php`
- [ ] Remove the `ClientAreaPage` hook that checks `$_GET['vs_webhook']` (lines ~94-102)
- [ ] The dedicated `webhook.php` file already handles POST-based webhook delivery correctly
- [ ] OR: change the hook to check `$_SERVER['REQUEST_METHOD'] === 'POST'` and verify the request body/signature

### 3.12 WHMCS: Plaintext Password Fallback
- File: `modules/servers/virtuestack/webhook.php`
- [ ] Change `encryptPassword()` to throw an exception or return empty string when `encrypt()` is unavailable, NOT return plaintext
- [ ] Log CRITICAL error and refuse to store the password

### 3.13 WHMCS: mysqli_fetch_compat
- File: `modules/servers/virtuestack/hooks.php`, line ~380
- [ ] Replace `mysqli_fetch_assoc($result)` with WHMCS-compatible `mysql_fetch_assoc($result)` or use Capsule query builder
- [ ] The `getPendingProvisioningServices()` function should be rewritten to use Capsule for driver compatibility

### 3.14 WHMCS: Missing task_status.php
- [ ] Either create `modules/servers/virtuestack/task_status.php` that displays provisioning status
- [ ] OR update the `virtuestack_getProvisioningStatusUrl()` function to return a valid URL (e.g., the overview page with the task ID)

### 3.15 WHMCS: NOW() Literal String
- File: `modules/servers/virtuestack/virtuestack.php`, lines ~860-861
- [ ] Replace `'created_at' => 'NOW()'` with `'created_at' => date('Y-m-d H:i:s')`
- [ ] Replace `'updated_at' => 'NOW()'` with `'updated_at' => date('Y-m-d H:i:s')`

### 3.16 Migration 010 Down — Restore Original Tables
- File: `migrations/000010_webhooks.down.sql`
- [ ] Add table recreation for `customer_webhooks` and `webhook_deliveries` matching the original schema from migration 001
- [ ] Add back the indexes that migration 008 created (or note that migration 008 down handles that)

---

## PHASE 4: MEDIUM — Reliability & Quality

### 4.1 WebSocket Origin Validation Hardening
- File: `internal/controller/api/customer/websocket.go`, lines ~69-72
- [ ] After `url.Parse(origin)`, validate that `parsed.Scheme` is `http` or `https`
- [ ] Reject `javascript:`, `file:`, `data:` and other non-HTTP schemes
- [ ] Log warning when a non-HTTP origin is rejected

### 4.2 time.After Timer Leak
- File: `internal/controller/services/failover_service.go`, line ~287
- [ ] Replace `case <-time.After(10 * time.Second):` with:
  ```go
  timer := time.NewTimer(10 * time.Second)
  defer timer.Stop()
  select {
  case <-timer.C:
  case <-ctx.Done():
      return ...
  }
  ```

### 4.3 VNC/Serial Console Goroutine Cleanup
- File: `internal/nodeagent/grpc_handlers_extended.go`
- [ ] For both VNC (lines ~388-430) and Serial (lines ~466-499) handlers:
  - [ ] Create a `context.WithCancel` derived from the stream context
  - [ ] Pass the cancel function to both goroutines
  - [ ] In each goroutine's loop, add a `select` with `ctx.Done()` to break on cancellation
  - [ ] On return, call `cancel()` to ensure both goroutines are signaled

### 4.4 WebSocket IP Tracker Bounded Map
- File: `internal/controller/api/customer/websocket.go`
- [ ] Add periodic cleanup (e.g., every 5 minutes) for entries with zero count (shouldn't exist but defensive)
- [ ] Add a max entries limit (e.g., 100000) and log warning when exceeded
- [ ] OR: use a sync.Map with TTL-based cleanup library

### 4.5 Frontend: Non-Functional UI Elements
- File: `webui/admin/app/dashboard/page.tsx`
  - [ ] Add onClick handlers to "Quick Action" buttons or mark them as "Coming Soon"
  - [ ] Add onClick to "View Logs" button or mark as "Coming Soon"
  - [ ] Remove or implement "Active Alerts" mock value — either fetch real alerts or remove the card
- File: `webui/admin/app/nodes/page.tsx`
  - [ ] Add "Add Node" button onClick handler with navigation to node creation form/dialog
  - [ ] Change "View" button to navigate to a detail page or open a detail dialog
- File: `webui/admin/app/customers/page.tsx`
  - [ ] Add "Add Customer" button onClick handler
  - [ ] Change "View" button to navigate to customer detail page
- File: `webui/admin/app/plans/page.tsx`
  - [ ] Add "Create Plan" button onClick handler
  - [ ] Implement plan edit dialog content (currently returns null)
- File: `webui/admin/app/ip-sets/page.tsx`
  - [ ] Add onClick handlers to "View" and "Edit" buttons
- File: `webui/admin/app/audit-logs/page.tsx`
  - [ ] Implement "Export CSV" button functionality
  - [ ] Implement "Advanced Filters" and "Date Range" dialogs

### 4.6 Frontend: Missing Admin VMs Page
- [ ] Create `webui/admin/app/vms/page.tsx` with VM management UI
- [ ] Support list, detail, create, delete, migrate actions
- [ ] Add "VMs" link to the admin sidebar

### 4.7 Frontend: Customer Login /signup Link
- File: `webui/customer/app/login/page.tsx`
- [ ] Remove the "Create account" link pointing to `/signup` (customers are created via WHMCS, not self-registration)
- [ ] OR create the signup page if self-registration is intended

### 4.8 Frontend: Missing Metrics Tab
- File: `webui/customer/app/vms/[id]/page.tsx`
- [ ] Add a "Metrics" tab to the VM detail page
- [ ] Use the existing `ResourceCharts` component with `vmApi.getMetrics()` and `vmApi.getBandwidth()` data
- [ ] Add real-time updates (polling or WebSocket)

### 4.9 Frontend: IP Sets Create Dialog Double-Unwrap
- File: `webui/admin/app/ip-sets/page.tsx`
- [ ] Fix `apiClient.post<{ data: IPSet }>(...)` — the API client already unwraps `response.data`
- [ ] Change to `apiClient.post<IPSet>(...)` and access `response.id` directly

### 4.10 Frontend: Division by Zero Guards
- File: `webui/admin/app/ip-sets/page.tsx`, line ~86
  - [ ] Add guard: `if (total === 0) return 0;`
- File: `webui/admin/app/nodes/page.tsx`, lines ~229, 257
  - [ ] Add guards for CPU and memory percentage calculations when totals are 0

### 4.11 WHMCS: Code Deduplication
- [ ] Extract shared functions from `hooks.php`, `webhook.php`, and `virtuestack.php` into a common include file:
  - [ ] `getCustomFieldId()`
  - [ ] `updateServiceField()`
  - [ ] `findServiceByTaskId()`
  - [ ] `verifyWebhookSignature()`
  - [ ] `getWebhookSecret()`
  - [ ] `updateServiceDedicatedIp()`
  - [ ] `createCustomField()`
- [ ] Update all three files to `require_once` the shared file

### 4.12 WHMCS: Webhook Event Type Consistency
- [ ] Reconcile event types between `webhook.php` and `hooks.php`
- [ ] Ensure both handlers support the same set of events
- [ ] Document which handler is the primary receiver

### 4.13 WHMCS: SSO Token in URL
- File: `modules/servers/virtuestack/lib/VirtueStackHelper.php`
- [ ] Implement POST-based token exchange instead of URL query parameter
- [ ] Generate a short-lived opaque token, store server-side, redirect to WebUI
- [ ] WebUI exchanges the opaque token for a proper JWT via POST
- [ ] Remove token from URL to prevent leakage in logs/history

### 4.14 NATS Authentication
- Files: `docker-compose.yml`, `docker-compose.prod.yml`
- [ ] Add `--auth <token>` or `--user <user> --pass <pass>` to NATS server command
- [ ] Add corresponding `NATS_TOKEN` or `NATS_USER`/`NATS_PASS` environment variables to Controller and Node Agent configs
- [ ] Update Controller NATS connection to include authentication

### 4.15 HTTP → HTTPS Redirect
- File: `nginx/conf.d/default.conf`
- [ ] Add `return 301 https://$host$request_uri;` to the HTTP server block
- [ ] OR: add individual `location` blocks that redirect specific paths

### 4.16 HTTP Block Security Headers
- File: `nginx/conf.d/default.conf`
- [ ] Add `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, `Content-Security-Policy` to the HTTP block
- [ ] Do NOT add HSTS on HTTP block (not applicable)

### 4.17 Missing Prometheus Scrape Config
- [ ] Create `configs/prometheus/prometheus.yml` with scrape targets:
  - [ ] Controller: `localhost:8080/metrics` (every 15s)
  - [ ] Node Agents: discovered via DNS or static config `localhost:9091/metrics`
- [ ] Add Prometheus service to `docker-compose.yml` (or document external setup)

### 4.18 Frontend: Admin Sidebar Hardcoded User Info
- File: `webui/admin/components/sidebar.tsx`
- [ ] Import `useAuth` and destructure `user`
- [ ] Replace hardcoded "Admin" with `user.email` or `user.name`
- [ ] Remove unused `useState` import

### 4.19 Frontend: Admin Forgot Password Non-Existent Endpoint
- File: `webui/admin/app/forgot-password/page.tsx`
- [ ] Verify if backend has a forgot-password endpoint for admins
- [ ] If not: remove the forgot password page or add the backend endpoint
- [ ] If yes: update the API call to match the actual endpoint path

### 4.20 Frontend: Admin Settings Menu Item
- File: `webui/admin/components/sidebar.tsx`
- [ ] Add onClick handler or href to "Settings" menu item
- [ ] Navigate to a settings page or show a dialog

---

## PHASE 5: LOW — Polish & Edge Cases

### 5.1 Dead Code Cleanup
- [ ] Remove `_ = s.router.Group("/api/v1")` in `internal/controller/server.go:406`
- [ ] Remove unused `useState` import in `webui/admin/components/sidebar.tsx`
- [ ] Remove unused `ApiError` and `ApiResponse` interfaces in both frontend api-client files
- [ ] Remove unused `generateCSSVariables`, `hsl`, `hsla` exports from theme files
- [ ] Remove `_ = provisioningKeyRepo` in `internal/controller/server.go:175`

### 5.2 Console.error in Production
- File: `webui/customer/app/settings/page.tsx`, line 155
- [ ] Remove `console.error("Failed to get 2FA status:", error)` or replace with proper error handling

### 5.3 Serial Console Mock Default Props
- File: `webui/customer/components/serial-console/serial-console.tsx`
- [ ] Remove default values `vmId = "vm-001"` and `vmName = "web-server-prod"`
- [ ] Make both parameters required (no defaults)

### 5.4 VNC Console Eager Import
- File: `webui/customer/components/novnc-console/vnc-console.tsx`
- [ ] Replace top-level `import("@novnc/novnc/lib/rfb")` with dynamic `import()` inside the `connect()` function
- [ ] This loads the library only when the user opens the console

### 5.5 VNC Console setInterval Cleanup
- File: `webui/customer/components/novnc-console/vnc-console.tsx`
- [ ] Clear the `setInterval` in the cleanup function of the effect that creates it
- [ ] Use a ref to store the interval ID and clear it on unmount

### 5.6 Clipboard API Error Handling
- File: `webui/customer/app/settings/page.tsx`, line ~495
- [ ] Add `.catch()` to `navigator.clipboard.writeText(text)` with a fallback or user notification

### 5.7 WHMCS: Log File in Web-Accessible Directory
- File: `modules/servers/virtuestack/webhook.php`
- [ ] Move log directory outside web root, or add `.htaccess` denying access
- [ ] OR: configure web server to block access to the logs directory

### 5.8 WHMCS: CDN Without SRI
- File: `modules/servers/virtuestack/templates/console.tpl`
- [ ] Add `integrity` and `crossorigin="anonymous"` attributes to the Font Awesome CDN link
- [ ] Or host the CSS locally

### 5.9 WHMCS: Origin Validation endsWith Bypass
- File: `modules/servers/virtuestack/templates/console.tpl`
- [ ] Replace `event.origin.endsWith(domain)` with `event.origin === 'https://' + domain` (exact match)

### 5.10 WHMCS: Unused provisioning_started_at
- File: `modules/servers/virtuestack/hooks.php`
- [ ] Add timeout check: if `provisioning_started_at` is older than X minutes, mark as failed
- [ ] OR remove the field if no timeout mechanism will be implemented

### 5.11 CalculateNextRetry Edge Case
- File: `internal/controller/services/webhook.go`, line ~557
- [ ] Add guard: `if attemptCount < 1 { return time.Minute }`
- [ ] Add max retry cap to prevent unreasonably long durations

### 5.12 ISO Checksum Caching
- File: `internal/controller/api/customer/iso_upload.go`
- [ ] Store SHA-256 checksum at upload time (compute once, save alongside the ISO)
- [ ] Serve the cached checksum in the list response instead of recomputing

### 5.13 subtleConstantTimeCompare
- File: `internal/shared/crypto/crypto.go`, lines ~265-268
- [ ] Replace the custom implementation with `crypto/subtle.ConstantTimeCompare(a, b)` from stdlib
- [ ] This handles length mismatches in constant time

### 5.14 closeStorage Error Handling
- File: `internal/nodeagent/server.go`, lines ~395-404
- [ ] Capture and log the error from `closer.Close()`

### 5.15 Frontend: useToast in useEffect Dependencies
- Files: Multiple admin pages
- [ ] Remove `toast` from useEffect dependency arrays (shadcn/ui useToast returns stable reference, but it's fragile)
- [ ] Use `useRef` for toast if needed inside effects

### 5.16 Frontend: Unused _ = vms in Admin Customers
- File: `webui/admin/app/api/admin/customers.go`, line ~100
- [ ] Replace VM list query with a proper COUNT query

---

## PHASE 6: TESTS — Coverage Gaps

### 6.1 Middleware Tests (Security-Critical)
- [ ] Create `internal/controller/api/middleware/auth_test.go`:
  - [ ] Test JWT auth with valid token
  - [ ] Test JWT auth with expired token → 401
  - [ ] Test JWT auth with missing token → 401
  - [ ] Test JWT auth with invalid signature → 401
  - [ ] Test API key auth with valid key
  - [ ] Test API key auth with invalid key → 401
  - [ ] Test API key auth with missing key → 401
  - [ ] Test 2FA requirement middleware
  - [ ] Test RBAC permission checks
- [ ] Create `internal/controller/api/middleware/ratelimit_test.go`:
  - [ ] Test rate limit enforcement
  - [ ] Test rate limit headers in response

### 6.2 Crypto Package Tests
- [ ] Create `internal/shared/crypto/crypto_test.go`:
  - [ ] Test AES-256-GCM encrypt/decrypt roundtrip
  - [ ] Test decrypt with wrong key returns error
  - [ ] Test decrypt with tampered ciphertext returns error
  - [ ] Test GenerateRandomString length and uniqueness
  - [ ] Test GenerateEncryptionKey length
  - [ ] Test constant-time comparison function

### 6.3 Node Agent Tests
- [ ] Create `internal/nodeagent/vm/domain_xml_test.go`:
  - [ ] Test Ceph RBD domain XML generation
  - [ ] Test QCOW file domain XML generation
  - [ ] Test bandwidth limits in XML
  - [ ] Test nwfilter reference in XML
  - [ ] Test cloud-init ISO attachment
- [ ] Create `internal/nodeagent/storage/qcow_test.go` (if possible to mock qemu-img):
  - [ ] Test QCOW storage backend interface methods
- [ ] Create `internal/nodeagent/network/nwfilter_test.go`:
  - [ ] Test nwfilter XML generation
- [ ] Create `internal/nodeagent/network/bandwidth_test.go`:
  - [ ] Test bandwidth limit calculation

### 6.4 Provisioning API Tests
- [ ] Create `internal/controller/api/provisioning/handlers_test.go`:
  - [ ] Test VM create with valid payload
  - [ ] Test VM create with invalid payload → 400
  - [ ] Test VM suspend/unsuspend
  - [ ] Test VM resize
  - [ ] Test task polling endpoint
  - [ ] Test API key authentication on all endpoints

### 6.5 Admin API Handler Tests
- [ ] Create test files for key admin handlers:
  - [ ] `internal/controller/api/admin/nodes_test.go`
  - [ ] `internal/controller/api/admin/vms_test.go`
  - [ ] `internal/controller/api/admin/plans_test.go`
  - [ ] `internal/controller/api/admin/customers_test.go`

### 6.6 Fix Existing Test Issues
- [ ] Fix `auth_2fa_test.go`: Replace `generateValidTOTPCode` with actual TOTP generation using the test secret
- [ ] Fix `backup_test.go`: Remove conditional assertions that can never fail
- [ ] Fix `webhook_test.go`: Add proper assertions for HTTPS enforcement test
- [ ] Fix `vm_lifecycle_test.go`: Add actual assertion for hostname uniqueness test
- [ ] Fix integration test suite: add mock implementations for nil dependencies (taskPublisher, nodeAgentClient, ipamService) so critical paths are actually tested

### 6.7 Missing Admin VMs Page Test
- [ ] Create test for the new admin VMs page (after implementing it in Phase 4)

### 6.8 Node Agent Prometheus Metrics Tests
- [ ] Create `internal/nodeagent/metrics/prometheus_test.go`:
  - [ ] Test metric registration
  - [ ] Test metric values update correctly

---

## PHASE 7: ADDITIONAL VALIDATED FINDINGS (from Medium/Low deep validation)

### 7.1 Go Backend — Additional Validated Findings

#### 7.1.1 CSRF Token Generation Panic
- File: `internal/controller/api/middleware/csrf.go`, lines ~162-165
- [ ] Replace `panic("failed to generate CSRF token")` with error return to handler
- [ ] Handler should return 500 Internal Server Error instead of crashing the process

#### 7.1.2 Hardcoded Default JWT Secret
- File: `internal/shared/config/config.go`, line ~431
- [ ] The default `"dev-jwt-secret-min-32-characters-long"` is only blocked in production
- [ ] Add warning log when default is used in any non-production environment
- [ ] Consider rejecting known-bad defaults regardless of environment

#### 7.1.3 Hardcoded Pagination Metadata in Webhook List
- File: `internal/controller/api/customer/webhooks.go`, line ~116
- [ ] `Meta: models.NewPaginationMeta(1, 20, len(responses))` is hardcoded
- [ ] Pass actual pagination params from request query string
- [ ] Use total count from repository, not `len(responses)`

#### 7.1.4 CalculateNextRetry Panic on attemptCount=0
- File: `internal/controller/services/webhook.go`, line ~557
- [ ] `1<<uint(attemptCount-1)` panics when attemptCount is 0 (uint overflow)
- [ ] Add guard: `if attemptCount < 1 { return time.Minute }`

#### 7.1.5 Dead Code Cleanup — Go
- [ ] Remove `_ = s.router.Group("/api/v1")` in `internal/controller/server.go:406`
- [ ] Remove `_ = provisioningKeyRepo` in `internal/controller/server.go:175`

### 7.2 Frontend — Additional Validated Findings

#### 7.2.1 User Set With Blank ID After Login
- Files: `webui/admin/lib/auth-context.tsx` lines ~125-133, `webui/customer/lib/auth-context.tsx` lines ~117-125
- [ ] After successful login, extract user ID from login response and set it properly
- [ ] Currently `id: ""` is set; real ID only populated after page reload via `initAuth`

#### 7.2.2 User Set With Blank ID/Email After 2FA Verify
- Files: `webui/admin/lib/auth-context.tsx` lines ~157-168, `webui/customer/lib/auth-context.tsx` lines ~151-159
- [ ] After 2FA verification, extract user ID and email from verify response
- [ ] Currently both `id: ""` and `email: ""` are set

#### 7.2.3 Customer Login Uses `<a>` Instead of Next.js `<Link>`
- File: `webui/customer/app/login/page.tsx`, line 104
- [ ] Replace `<a href="/signup">` with Next.js `<Link href="/signup">` (or remove entirely if /signup is removed)

#### 7.2.4 Admin Login Uses window.location.reload()
- File: `webui/admin/app/login/page.tsx`, line 117
- [ ] Replace `window.location.reload()` with state-based approach (`requires2FA = false`)

#### 7.2.5 Dashboard Error State Unreachable (Dead Code)
- File: `webui/admin/app/dashboard/page.tsx`, lines ~53-102, 139-145
- [ ] `Promise.allSettled` never throws; the `catch` block and `error` state are dead code
- [ ] Either remove the error state display or use `Promise.all` with a try/catch

#### 7.2.6 Serial Console Auto-Connects on Mount
- File: `webui/customer/components/serial-console/serial-console.tsx`, lines ~48-159
- [ ] Add a "Connect" button; do not auto-connect on component mount
- [ ] This matches the VNC console pattern which requires user click

#### 7.2.7 Serial Console Reboot Doesn't Actually Reboot
- File: `webui/customer/components/serial-console/serial-console.tsx`, lines ~181-191
- [ ] The "Reboot" button only writes a message to the terminal display
- [ ] Add an API call to actually reboot the VM (or at minimum send ACPI shutdown via serial)

#### 7.2.8 VMMetrics Interface Unit Mismatch
- File: `webui/customer/lib/api-client.ts`, lines ~236-247
- [ ] `network_rx_bytes` and `network_tx_bytes` are cumulative byte counters
- [ ] ResourceCharts divides by 1024*1024 and labels as "Mbps" — this is wrong (should be MB, not Mbps)
- [ ] Should calculate delta between samples to get per-second rates

#### 7.2.9 VM Description State Never Loads From API
- File: `webui/customer/app/vms/[id]/page.tsx`, line 165
- [ ] `vmDescription` is initialized to `""` but never populated from API response
- [ ] VM model has no `description` field; either add backend support or remove the UI input

#### 7.2.10 Missing Network/rDNS Tab in VM Detail
- File: `webui/customer/app/vms/[id]/page.tsx`
- [ ] No UI for managing reverse DNS or viewing IPv6 details
- [ ] `FEATURE_FLAGS.enableNetworkConfig` is `false` — the network config section in Settings is hidden

#### 7.2.11 Missing VM List Auto-Refresh
- File: `webui/customer/app/vms/page.tsx`
- [ ] Add polling (e.g., every 30 seconds) or WebSocket listener for real-time VM status updates

#### 7.2.12 Missing Dashboard Auto-Refresh
- File: `webui/admin/app/dashboard/page.tsx`
- [ ] Add polling interval for dashboard stats

#### 7.2.13 Admin API Client Missing Many Documented Endpoints
- File: `webui/admin/lib/api-client.ts`
- [ ] Add client methods for: Node CRUD (create, update, delete), VM CRUD (create, get, update, delete, migrate), Settings (get/put), Backup management, Template management, Failover requests, VM IP/rDNS endpoints

#### 7.2.14 Audit Logs API No Pagination
- File: `webui/admin/lib/api-client.ts`, lines ~362-365
- [ ] Add `page` and `per_page` query parameters to `getAuditLogs()`

#### 7.2.15 IP Sets Import Dialog Missing File Size Validation
- File: `webui/admin/app/ip-sets/page.tsx`, lines ~274-354
- [ ] Add `importFile.size` check before processing (e.g., max 1MB)

#### 7.2.16 Missing sr-only Labels on Icon-Only Buttons
- Files: `webui/admin/app/nodes/page.tsx` lines ~278-315, `webui/admin/app/customers/page.tsx` lines ~233-284
- [ ] Add `<span className="sr-only">Button Name</span>` inside icon-only action buttons

#### 7.2.17 Notifications Bell Button Non-Functional
- Files: `webui/admin/app/dashboard/layout.tsx` line ~50, `webui/customer/app/vms/layout.tsx` line ~49
- [ ] Add onClick handler, badge count, and notification dropdown panel

#### 7.2.18 VNC Console RFB Library Eager Import
- File: `webui/customer/components/novnc-console/vnc-console.tsx`, lines ~32-38
- [ ] Move `import("@novnc/novnc/lib/rfb")` into the `connect()` function as dynamic import

#### 7.2.19 VNC Console setInterval Leak on Unmount
- File: `webui/customer/components/novnc-console/vnc-console.tsx`, lines ~87-98
- [ ] Store interval/timeout IDs in refs, clear on unmount via useEffect cleanup

#### 7.2.20 Console.error in Production
- File: `webui/customer/app/settings/page.tsx`, line 155
- [ ] Remove `console.error("Failed to get 2FA status:", error)`

#### 7.2.21 Missing key Prop in Backup Codes List
- File: `webui/customer/app/settings/page.tsx`, line ~1351
- [ ] Use the code value itself as key instead of array `index`

#### 7.2.22 params.id Non-Null Assertion
- File: `webui/customer/app/vms/[id]/page.tsx`, line 136
- [ ] Add runtime check: `if (!params.id) { redirect('/vms'); return null }`

#### 7.2.23 Clipboard API Without Error Handling
- File: `webui/customer/app/settings/page.tsx`, line ~495
- [ ] Add `.catch()` to `navigator.clipboard.writeText(text)` with fallback notification

#### 7.2.24 Admin Forgot Password Non-Existent Endpoint
- File: `webui/admin/app/forgot-password/page.tsx`, line 42
- [ ] Backend has no `/admin/auth/forgot-password` endpoint — remove page or implement endpoint

### 7.3 Infrastructure — Additional Validated Findings

#### 7.3.1 Docker Image Tags Not Pinned
- Files: `docker-compose.yml` lines ~73, 112, 142
- [ ] Add default values: `${CONTROLLER_TAG:-latest}` → `${CONTROLLER_TAG:-v2.1.0}` (or similar)
- [ ] Or better: fail with clear error if tags are not set

#### 7.3.2 No TLS Between Controller and PostgreSQL
- File: `docker-compose.yml` line ~77
- [ ] Docker bridge networks are NOT encrypted (despite prod comment claiming otherwise)
- [ ] Either switch to overlay network with `opt encrypted`, or add `sslmode=require` with proper cert setup

#### 7.3.3 limit_conn Zone Defined But Never Used
- File: `nginx/conf.d/rate-limit.conf` line ~40, `nginx/conf.d/default.conf`
- [ ] Add `limit_conn conn_limit 10;` to appropriate `location` blocks in default.conf

#### 7.3.4 Missing Indexes on Frequently Queried Columns
- File: `migrations/000001_initial_schema.up.sql`
- [ ] Add index on `vms.plan_id`
- [ ] Add index on `vms.hostname`
- [ ] Add index on `nodes.location_id`

#### 7.3.5 notification_preferences Redundant Index
- File: `migrations/000009_notification_preferences.up.sql`, line ~22
- [ ] Remove `CREATE INDEX idx_notification_preferences_customer_id` (UNIQUE constraint already creates index)

#### 7.3.6 Missing FK Constraints
- File: `migrations/000001_initial_schema.up.sql`
- [ ] Add `REFERENCES admins(id)` to `provisioning_keys.created_by` (line ~109)
- [ ] Add `REFERENCES admins(id)` to `system_settings.updated_by` (line ~119)

#### 7.3.7 .gitignore Doesn't Exclude .env.production / .env.staging
- File: `.gitignore`
- [ ] Add `.env.production` and `.env.staging` patterns

#### 7.3.8 X-XSS-Protection Header Deprecated
- File: `nginx/conf.d/default.conf`, line ~175; `webui/*/nginx-app.conf`
- [ ] Remove `X-XSS-Protection` header (CSP already provides equivalent protection)

#### 7.3.9 Nginx Health Check Doesn't Verify Backend
- Files: `webui/customer/nginx-app.conf`, `webui/admin/nginx-app.conf`
- [ ] Consider adding actual health check (e.g., curl to upstream or file existence check)

#### 7.3.10 Backup Script -pbkdf2 Dead Code
- File: `scripts/backup-config.sh`, lines ~67-72
- [ ] `-pbkdf2 -iter 100000` are ignored when `-K` (raw hex key) is provided
- [ ] Remove the misleading `-pbkdf2 -iter 100000` flags, or document that `-K` takes precedence

### 7.4 WHMCS — Additional Validated Findings

#### 7.4.1 Webhook Log in Web-Accessible Directory
- File: `modules/servers/virtuestack/webhook.php`, line ~629
- [ ] Move logs directory outside web root or add `.htaccess` denying access

#### 7.4.2 Sensitive Params Logged on CreateAccount
- File: `modules/servers/virtuestack/virtuestack.php`, line ~109
- [ ] Add `serverpassword` to the sanitize filter in `VirtueStackHelper::sanitizeLog()`

#### 7.4.3 sendAdminNotification Only Logs, Never Sends Email
- File: `modules/servers/virtuestack/hooks.php`, lines ~1093-1098
- [ ] Replace `logAdminNotification()` with `sendMessage()` using an admin email template

#### 7.4.4 DailyCronJob Fallback Too Infrequent
- File: `modules/servers/virtuestack/hooks.php`, lines ~38-53
- [ ] Change from `DailyCronJob` to `CronJob` (runs every 5 minutes) for provisioning status sync

#### 7.4.5 provisioning_started_at Stored But Never Used
- File: `modules/servers/virtuestack/hooks.php`, lines ~61-82
- [ ] Add timeout check: if provisioning_started_at > 30 minutes, mark VM as failed
- [ ] OR remove the field

#### 7.4.6 pollTask Blocks PHP Process Up to 3 Minutes
- File: `modules/servers/virtuestack/lib/ApiClient.php`, lines ~368-411
- [ ] Add warning comment or refactor to use async polling pattern

#### 7.4.7 listTemplates/Locations Use error_log Instead of Module Log
- File: `modules/servers/virtuestack/lib/ApiClient.php`, lines ~446, 460
- [ ] Replace `error_log()` with `$this->log()` to use WHMCS module logging

#### 7.4.8 POST Requests With Empty Body Send Content-Type
- File: `modules/servers/virtuestack/lib/ApiClient.php`, lines ~524-527
- [ ] Only set `Content-Type: application/json` when body is non-empty

#### 7.4.9 Full Task Result Logged on Migration Event
- File: `modules/servers/virtuestack/webhook.php`, line ~321
- [ ] Sanitize or truncate logged result before writing to activity log

#### 7.4.10 Console Template Input Validation
- File: `modules/servers/virtuestack/templates/console.tpl`, line ~12
- [ ] Validate console type against whitelist: `['vnc', 'serial']`

#### 7.4.11 Missing WHMCS Lifecycle Functions
- File: `modules/servers/virtuestack/virtuestack.php`
- [ ] Implement `virtuestack_UsageUpdate` for bandwidth billing
- [ ] Implement `virtuestack_SingleSignOn` (metadata label already defined)
- [ ] Implement `virtuestack_AdminServicesTabFieldsSave` for admin tab edit save

#### 7.4.12 ShoppingCartCheckoutCompletePage No-Op
- File: `modules/servers/virtuestack/hooks.php`, lines ~131-144
- [ ] Remove the no-op JavaScript hook or implement actual analytics tracking

#### 7.4.13 AdminServicesListTable Adds Headers But No Row Data
- File: `modules/servers/virtuestack/hooks.php`, lines ~195-196
- [ ] Add corresponding `AdminServicesListTableRow` hook to populate VPS Status and VM ID columns

#### 7.4.14 Static Variable Rate-Limiting Per-Process
- File: `modules/servers/virtuestack/hooks.php`, lines ~327-334
- [ ] Replace PHP `static $lastRun` with database/file-based throttle (e.g., store timestamp in a config option)

#### 7.4.15 Template/Location API Calls Iterate All Services
- File: `modules/servers/virtuestack/hooks.php`, lines ~948-1052
- [ ] Use a single server-level API call instead of iterating all services
- [ ] Cache results with a TTL (e.g., 5 minutes)

#### 7.4.16 Missing force_stop Power Operation
- File: `modules/servers/virtuestack/lib/ApiClient.php`, line ~428
- [ ] Add `'force_stop'` to the power operation whitelist

#### 7.4.17 log() Doesn't Sanitize $data Parameter
- File: `modules/servers/virtuestack/lib/VirtueStackHelper.php`, lines ~387-395
- [ ] Add `$data = self::sanitizeLog($data);` before passing to `logModuleCall`

#### 7.4.18 JWT Secret Config Not Documented
- File: `modules/servers/virtuestack/virtuestack.php`, line ~684
- [ ] Add `jwt_secret` to `ConfigOptions()` array with description and validation

### 7.5 Tests — Additional Validated Findings

#### 7.5.1 Integration SetupTest/TeardownTest Swallow Errors
- File: `tests/integration/suite_test.go`, lines ~314-357
- [ ] Add error logging to all `_ = suite.DBPool.Exec(...)` calls in setup/teardown
- [ ] Consider failing the test on setup errors (use `t.Fatal(err)`)

#### 7.5.2 TimingAttackPrevention Test Is Non-Deterministic
- File: `tests/integration/auth_test.go`, lines ~519-534
- [ ] Either remove this test or implement proper statistical analysis (Mann-Whitney U test)

#### 7.5.3 CustomerCannotAccessOtherCustomerVMs Test Doesn't Set RLS Context
- File: `tests/integration/auth_test.go`, lines ~547-568
- [ ] Add `SET LOCAL app.current_customer_id = '<customer_id>'` before the GetByID call
- [ ] Or switch connection to `app_customer` role for this test

#### 7.5.4 AdminRoleInToken Test Never Parses JWT
- File: `tests/integration/auth_test.go`, lines ~570-582
- [ ] Add JWT parsing and verification of `role` and `user_type` claims

#### 7.5.5 HostnameUniqueness Test Has No Assertion
- File: `tests/integration/vm_lifecycle_test.go`, lines ~318-330
- [ ] Add actual second VM creation with duplicate hostname
- [ ] Assert it fails or returns an error

#### 7.5.6 RestoreFromBackup Conditional Assertion
- File: `tests/integration/backup_test.go`, lines ~95-119
- [ ] Replace `if err == nil { ... }` with `require.NoError(t, err)` or proper error handling

#### 7.5.7 RestoreFromSnapshot / BackupWhileRestoring Discard Errors
- File: `tests/integration/backup_test.go`, lines ~465-479, ~581-597
- [ ] Add assertions: `assert.NoError(t, err)` or `require.NoError(t, err)`

#### 7.5.8 Webhook Tests Discard Errors
- File: `tests/integration/webhook_test.go`
- [ ] `UnavailableEndpoint` (line ~217): Add `assert.Error(t, err)` assertion
- [ ] `HTTPSOnly` (line ~555): Replace `_ = err` with proper assertion that HTTP URLs are rejected
- [ ] `SuccessfulDelivery` (line ~147): Pass `payload` instead of `nil` to `Deliver`
- [ ] `FailedDeliveryWithRetry` (line ~177): Use the created payload, assert retry happened

#### 7.5.9 Duplicate Failover Monitor Tests
- File: `internal/controller/services/failover_monitor_test.go`, lines ~123-172
- [ ] Remove one of the duplicate tests (`TestFailoverMonitorMetricsAfterFailover` or `TestProcessNodeFailoverRecordsMetrics`)

#### 7.5.10 TestGetNodesNeedingFailoverFiltering Reimplements Logic
- File: `internal/controller/services/failover_monitor_test.go`, lines ~192-217
- [ ] Refactor to call the actual `FailoverMonitor` method instead of reimplementing filtering

#### 7.5.11 E2E Tests Need Fixes
- File: `tests/e2e/auth.spec.ts`
  - [ ] Fix shadowed `loginPage` variable at line ~443 (module scope never initialized)
  - [ ] Fix logout test (lines ~171-186) to authenticate first
  - [ ] Fix CSRF test (lines ~409-418) assertion to actually verify CSRF token exists
  - [ ] Uncomment `Secure` cookie check (line ~436)
- File: `tests/e2e/customer-vm.spec.ts`
  - [ ] Remove conditional assertions that can never fail (lines ~196-227)
  - [ ] Replace hardcoded VM IDs with proper test setup/teardown
- File: `tests/e2e/admin-vm.spec.ts`
  - [ ] Add cleanup in `afterAll` to delete VMs created during tests
- File: `tests/load/k6-vm-operations.js`
  - [ ] Add cleanup for VMs created during load test

#### 7.5.12 Proto Snapshot/SnapshotResponse Redundancy
- File: `proto/virtuestack/node_agent.proto`
- [ ] Unify `Snapshot` and `SnapshotResponse` messages (they are structurally identical)
- [ ] Use `Snapshot` in `SnapshotListResponse` instead of a separate message type

#### 7.5.13 Proto PostMigrateSetupRequest Uses vm_uuid Inconsistent Naming
- File: `proto/virtuestack/node_agent.proto`, line ~421
- [ ] Rename `vm_uuid` to `vm_id` for consistency with all other messages

#### 7.5.14 GuestExecCommand Path Validation Allows Symlinks
- File: `internal/nodeagent/grpc_handlers_extended.go`, lines ~637-648
- [ ] Use `filepath.EvalSymlinks()` to resolve symlinks before checking path prefix

#### 7.5.15 ZAP Scan Missing Authentication and Minimal Coverage
- File: `tests/security/owasp_zap.sh`
- [ ] Configure ZAP with authenticated scanning (admin + customer credentials)
- [ ] Expand OpenAPI spec to cover all documented API endpoints

#### 7.5.16 hashPassword Accepts Empty Password
- File: `internal/controller/tasks/handlers_test.go`, lines ~52-55
- [ ] Either: change test to expect error for empty password
- [ ] Or: add `validatePasswordStrength` call inside `hashPassword` to reject empty passwords

#### 7.5.17 RBACService Test Passes nil auditRepo
- File: `internal/controller/services/rbac_service_test.go`, line ~46
- [ ] Create a mock `AuditRepository` for the test

---

## SUMMARY

| Phase | Category | Items |
|-------|----------|-------|
| 1 | Critical (Security/Data/Build) | 10 sections, ~60 checkboxes |
| 2 | QCOW Backend Implementation | 11 sections, ~65 checkboxes |
| 3 | High (Core Functionality) | 16 sections, ~80 checkboxes |
| 4 | Medium (Reliability/Quality) | 20 sections, ~90 checkboxes |
| 5 | Low (Polish) | 16 sections, ~30 checkboxes |
| 6 | Tests | 8 sections, ~35 checkboxes |
| 7 | Additional Validated Findings | 22 sections, ~90 checkboxes |
| **Total** | | **~450 checkboxes** |
