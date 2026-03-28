<!-- Generated: 2026-03-28 | Files scanned: 165 Go files | Token estimate: ~1200 -->

# Backend Architecture

## API Tiers

```
/api/v1/provisioning/*  → API Key Auth  → WHMCS Integration
/api/v1/customer/*      → JWT + Refresh → Customer Self-Service
/api/v1/admin/*         → JWT + 2FA     → Admin Operations
```

## Admin API Routes

**File:** `internal/controller/api/admin/routes.go`

```
POST   /auth/login, /verify-2fa, /refresh, /logout
GET    /auth/me
POST   /auth/reauth
GET    /auth/permissions              # List available permissions
PUT    /auth/permissions/:admin_id    # Update admin permissions (super_admin only)

GET    /admins                        # List admins (super_admin only)

GET    /nodes, /nodes/:id
POST   /nodes, /nodes/:id/drain, /nodes/:id/failover, /nodes/:id/undrain
DELETE /nodes/:id
PUT    /nodes/:id

GET    /failover-requests, /failover-requests/:id

GET    /vms, /vms/:id, /vms/:id/ips
POST   /vms, /vms/:id/migrate
DELETE /vms/:id
PUT    /vms/:id
GET    /vms/:id/ips/:ipId/rdns
PUT    /vms/:id/ips/:ipId/rdns
DELETE /vms/:id/ips/:ipId/rdns

GET    /plans, /plans/:id/usage
POST   /plans
PUT    /plans/:id
DELETE /plans/:id

GET    /templates, /templates/:id
POST   /templates, /templates/:id/import, /templates/:id/distribute
DELETE /templates/:id
PUT    /templates/:id
POST   /templates/build-from-iso
GET    /templates/:id/cache-status

GET    /ip-sets, /ip-sets/:id, /ip-sets/:id/available
POST   /ip-sets
DELETE /ip-sets/:id
PUT    /ip-sets/:id

GET    /customers, /customers/:id, /customers/:id/audit-logs
POST   /customers
DELETE /customers/:id
PUT    /customers/:id

GET    /audit-logs
GET    /settings
PUT    /settings/:key

GET    /backups
POST   /backups/:id/restore
GET    /backup-schedules, /backup-schedules/:id
POST   /backup-schedules, /backup-schedules/:id/run
DELETE /backup-schedules/:id
PUT    /backup-schedules/:id

GET    /admin-backup-schedules, /admin-backup-schedules/:id
POST   /admin-backup-schedules, /admin-backup-schedules/:id/run
DELETE /admin-backup-schedules/:id
PUT    /admin-backup-schedules/:id

GET    /storage-backends, /storage-backends/:id
POST   /storage-backends
PUT    /storage-backends/:id
DELETE /storage-backends/:id
GET    /storage-backends/:id/nodes
POST   /storage-backends/:id/nodes
DELETE /storage-backends/:id/nodes
GET    /storage-backends/:id/health
POST   /storage-backends/:id/health
POST   /storage-backends/:id/refresh

GET    /provisioning-keys, /provisioning-keys/:id
POST   /provisioning-keys
PUT    /provisioning-keys/:id
DELETE /provisioning-keys/:id       # Requires re-auth
```

## Customer API Routes

**File:** `internal/controller/api/customer/routes.go`

```
GET    /csrf                          # CSRF token
POST   /auth/login, /verify-2fa, /refresh, /logout
GET    /auth/sso-exchange             # SSO token exchange (WHMCS)
POST   /auth/forgot-password, /auth/reset-password

PUT    /password
GET    /profile, PUT /profile

GET    /vms, /vms/:id
POST   /vms/:id/start, /vms/:id/stop, /vms/:id/restart, /vms/:id/force-stop
POST   /vms/:id/console-token, /vms/:id/serial-token
GET    /vms/:id/metrics, /vms/:id/bandwidth, /vms/:id/network
GET    /vms/:id/ips, /vms/:id/ips/:ipId/rdns
PUT    /vms/:id/ips/:ipId/rdns
DELETE /vms/:id/ips/:ipId/rdns

POST   /vms/:id/iso/upload
GET    /vms/:id/iso
DELETE /vms/:id/iso/:isoId
POST   /vms/:id/iso/:isoId/attach, /vms/:id/iso/:isoId/detach

GET    /ws/vnc/:vmId, /ws/serial/:vmId (WebSocket)

GET    /backups, /backups/:id
POST   /backups
DELETE /backups/:id
POST   /backups/:id/restore

GET    /snapshots
POST   /snapshots
DELETE /snapshots/:id
POST   /snapshots/:id/restore

GET    /tasks/:id

GET    /templates

GET    /api-keys
POST   /api-keys, /api-keys/:id/rotate
DELETE /api-keys/:id

GET    /webhooks, /webhooks/:id, /webhooks/:id/deliveries
POST   /webhooks, /webhooks/:id/test
DELETE /webhooks/:id
PUT    /webhooks/:id

POST   /2fa/initiate, /2fa/enable, /2fa/disable
GET    /2fa/status, /2fa/backup-codes
POST   /2fa/backup-codes/regenerate

GET    /notifications/preferences
PUT    /notifications/preferences
```

## Provisioning API Routes

**File:** `internal/controller/api/provisioning/routes.go`

```
POST   /vms                    # Create VM (async, returns task_id)
GET    /vms/:id                # Get VM by ID
GET    /vms/by-service/:id     # Get VM by WHMCS service ID
DELETE /vms/:id                # Delete VM (async)
POST   /vms/:id/suspend        # Billing suspend
POST   /vms/:id/unsuspend      # Unsuspend
POST   /vms/:id/resize         # Resize resources
POST   /vms/:id/password       # Set root password
POST   /vms/:id/password/reset # Reset password
POST   /vms/:id/power          # Power operations
GET    /vms/:id/status         # VM status
GET    /vms/:id/usage          # Bandwidth/disk usage
GET    /vms/:id/rdns           # Get rDNS
PUT    /vms/:id/rdns           # Set rDNS

GET    /tasks/:id              # Task status polling

POST   /customers              # Create-or-get customer by email

POST   /sso-tokens             # Create SSO bootstrap token

GET    /plans                  # List plans
GET    /plans/:id              # Get plan
```

## Service Layer

**Directory:** `internal/controller/services/` (41 files)

| Service | File | Purpose |
|---------|------|---------|
| AuthService | `auth_service*.go` (6 files) | JWT, 2FA, password, login, tokens |
| VMService | `vm_service.go` | VM CRUD, power, health checks |
| NodeService | `node_service.go` | Node registration, drain, failover |
| PlanService | `plan_service.go` | VPS plans |
| TemplateService | `template_service.go` | OS templates, build, distribute |
| BackupService | `backup_service.go` | Backup CRUD |
| AdminBackupScheduleService | `admin_backup_schedule_service.go` | Mass backup campaigns |
| MigrationService | `migration_service.go` | Live VM migration |
| FailoverService | `failover_service.go` | HA failover |
| FailoverMonitor | `failover_monitor.go` | Monitors node health for failover |
| HeartbeatChecker | `heartbeat_checker.go` | Node heartbeat monitoring |
| RDNSService | `rdns_service.go` | Reverse DNS via PowerDNS |
| RBACService | `rbac_service.go` | Admin permission checks |
| IPAMService | `ipam_service.go` | IP allocation |
| BandwidthService | `bandwidth_service.go` | Usage tracking |
| StorageBackendService | `storage_backend_service.go` | Storage backend registry |
| CustomerService | `customer_service.go` | Customer management |
| WebhookService | `webhook.go` | Webhook delivery |
| NotificationService | `notification_service.go` | Email/Telegram |
| CircuitBreaker | `circuit_breaker.go` | Resilience pattern |
| IPMIClient | `ipmi_client.go` | IPMI power management |
| NodeAgentClient | `node_agent_client.go` | gRPC client wrapper |

## Repository Layer

**Directory:** `internal/controller/repository/` (30 files)

| Repository | File | Tables |
|------------|------|--------|
| VMRepo | `vm_repo.go` | vms |
| NodeRepo | `node_repo.go` | nodes, node_heartbeats |
| CustomerRepo | `customer_repo.go` | customers, sessions |
| PlanRepo | `plan_repo.go` | plans |
| TemplateRepo | `template_repo.go` | templates |
| TemplateCacheRepo | `template_cache_repo.go` | template_node_cache |
| IPRepo | `ip_repo.go` | ip_sets, ip_addresses, ipv6_prefixes |
| BackupRepo | `backup_repo.go` | backups, snapshots, backup_schedules |
| AdminBackupScheduleRepo | `admin_backup_schedule_repo.go` | admin_backup_schedules |
| TaskRepo | `task_repo.go` | tasks |
| AuditRepo | `audit_repo.go` | audit_logs |
| WebhookRepo | `webhook_repo.go` | customer_webhooks, webhook_deliveries |
| SettingsRepo | `settings_repo.go` | system_settings |
| FailoverRepo | `failover_repo.go` | failover_requests |
| ConsoleTokenRepo | `console_token_repo.go` | console_tokens |
| SSOTokenRepo | `sso_token_repo.go` | sso_tokens |
| CustomerAPIKeyRepo | `customer_api_key_repo.go` | customer_api_keys |
| ProvisioningKeyRepo | `provisioning_key_repo.go` | provisioning_keys |
| ISOUploadRepo | `iso_upload_repo.go` | iso_uploads |
| StorageBackendRepo | `storage_backend_repo.go` | storage_backends |
| NodeStorageRepo | `node_storage_repo.go` | node_storage (junction) |
| NotificationRepo | `notification_repo.go` | notification_preferences |
| BandwidthRepo | `bandwidth_repo.go` | bandwidth views |
| Pagination | `cursor/pagination.go` | Cursor-based pagination helper |

## Middleware Chain

**Directory:** `internal/controller/api/middleware/` (19 files)

```
Request → Correlation → Metrics → RateLimit → Recovery → Auth → CSRF → Permissions → Validation → Handler
```

| Middleware | File | Purpose |
|------------|------|---------|
| JWTAuth | `auth.go` | JWT validation, claims extraction |
| JWTOrCustomerAPIKeyAuth | `auth.go` | Dual auth for customer endpoints |
| APIKeyAuth | `auth.go` | Provisioning API key |
| RequireRole | `auth.go` | Admin role check |
| RequireUserType | `auth.go` | Customer type check |
| RequireAdminPermission | `permissions.go` | Admin RBAC check |
| RequirePermission | `permissions.go` | Customer API key permissions |
| CSRF | `csrf.go` | Double-submit token |
| RateLimit | `ratelimit.go` | Sliding window per IP/user (in-memory or Redis) |
| Metrics | `metrics.go` | Prometheus instrumentation |
| Audit | `audit.go` | Audit log writing |
| Recovery | `recovery.go` | Panic recovery, error response |
| Validation | `validation.go` | Request body validation |
| IPAllowlist | `ip_allowlist.go` | IP-based access control |
| Correlation | `correlation.go` | Request correlation ID |

## Node Agent gRPC Service

**File:** `proto/virtuestack/node_agent.proto` (972 lines, 38 RPC methods)

```protobuf
service NodeAgentService {
  // Lifecycle (7)
  rpc CreateVM, StartVM, StopVM, ForceStopVM, DeleteVM, ReinstallVM, ResizeVM
  // Migration (7)
  rpc MigrateVM, AbortMigration, PostMigrateSetup, PrepareMigratedVM
  rpc CreateDiskSnapshot, DeleteDiskSnapshot, TransferDisk, ReceiveDisk
  // Backup (2)
  rpc CreateLVMBackup, RestoreLVMBackup
  // Console (2 — bidirectional streaming)
  rpc StreamVNCConsole, StreamSerialConsole
  // Metrics (3)
  rpc GetVMStatus, GetVMMetrics, GetNodeResources
  // Snapshots (4)
  rpc CreateSnapshot, DeleteSnapshot, RevertSnapshot, ListSnapshots
  // Guest Agent (5)
  rpc GuestExecCommand, GuestSetPassword, GuestFreezeFilesystems, GuestThawFilesystems, GuestGetNetworkInterfaces
  // Bandwidth (3)
  rpc GetBandwidthUsage, SetBandwidthLimit, ResetBandwidthCounters
  // Health (2)
  rpc Ping, GetNodeHealth
  // Template (2)
  rpc BuildTemplateFromISO, EnsureTemplateCached
}
```

## Async Task System

**Directory:** `internal/controller/tasks/` (25 files)

**Worker:** 4 concurrent workers, NATS JetStream durable consumer, 5min ack timeout, 3 max retries

| Task Type | Handler | Purpose |
|-----------|---------|---------|
| `vm.create` | `handlers_vm_create.go` | VM provisioning |
| `vm.reinstall` | `vm_reinstall.go` | OS reinstallation |
| `vm.resize` | `vm_resize.go` | Resource resize |
| `vm.migrate` | `migration_execute.go` | Live migration |
| `vm.delete` | `handlers_vm_delete.go` | VM deletion |
| `backup.create` | `backup_create.go` | Backup creation |
| `backup.restore` | `handlers_backup_restore.go` | Backup restoration |
| `snapshot.create` | `snapshot_handlers.go` | Snapshot creation |
| `snapshot.revert` | `snapshot_handlers.go` | Snapshot revert |
| `snapshot.delete` | `snapshot_handlers.go` | Snapshot deletion |
| `template.build_from_iso` | `template_build.go` | Build template from ISO |
| `template.distribute` | `template_distribute.go` | Distribute template to nodes |