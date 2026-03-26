<!-- Generated: 2026-03-22 | Files scanned: 115 Go files | Token estimate: ~1100 -->

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
PUT    /vms/:id, /vms/:id/ips/:ipId/rdns

GET    /plans, /plans/:id/usage
POST   /plans
PUT    /plans/:id
DELETE /plans/:id

GET    /templates, /templates/:id
POST   /templates, /templates/:id/import
DELETE /templates/:id
PUT    /templates/:id

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

GET    /provisioning-keys, /provisioning-keys/:id
POST   /provisioning-keys
PUT    /provisioning-keys/:id
DELETE /provisioning-keys/:id       # Requires re-auth
```

## Customer API Routes

**File:** `internal/controller/api/customer/routes.go`

```
POST   /auth/login, /verify-2fa, /refresh, /logout
PUT    /password
GET    /profile, PUT /profile

GET    /vms, /vms/:id
POST   /vms/:id/start, /vms/:id/stop, /vms/:id/restart, /vms/:id/force-stop
POST   /vms/:id/console-token, /vms/:id/serial-token
GET    /vms/:id/metrics, /vms/:id/bandwidth, /vms/:id/network
GET    /vms/:id/ips, /vms/:id/ips/:ipId/rdns
PUT    /vms/:id/ips/:ipId/rdns
DELETE /vms/:id/ips/:ipId/rdns

# ISO Management (NEW)
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

GET    /api-keys
POST   /api-keys, /api-keys/:id/rotate
DELETE /api-keys/:id

GET    /webhooks, /webhooks/:id, /webhooks/:id/deliveries
POST   /webhooks
DELETE /webhooks/:id
PUT    /webhooks/:id
POST   /webhooks/:id/test

GET    /templates
POST   /2fa/initiate, /2fa/enable, /2fa/disable
GET    /2fa/status, /2fa/backup-codes
POST   /2fa/backup-codes/regenerate
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
GET    /vms/:id/rdns           # Get rDNS
PUT    /vms/:id/rdns           # Set rDNS

GET    /tasks/:id              # Task status

GET    /plans                  # List plans (NEW)
GET    /plans/:id              # Get plan (NEW)
```

## Service Layer

**Directory:** `internal/controller/services/`

| Service | File | Purpose |
|---------|------|---------|
| AuthService | `auth_service*.go` | JWT, 2FA, password, login |
| VMService | `vm_service.go` | VM CRUD, power, migration |
| NodeService | `node_service.go` | Node registration, heartbeat |
| PlanService | `plan_service.go` | VPS plans |
| TemplateService | `template_service.go` | OS templates |
| BackupService | `backup_service.go` | Backup CRUD |
| AdminBackupScheduleService | `admin_backup_schedule_service.go` | Mass backup campaigns |
| MigrationService | `migration_service.go` | Live migration |
| FailoverService | `failover_service.go` | HA failover |
| RDNSService | `rdns_service.go` | Reverse DNS |
| RBACService | `rbac_service.go` | Permissions (NEW) |
| IPAMService | `ipam_service.go` | IP allocation |
| BandwidthService | `bandwidth_service.go` | Usage tracking |
| WebhookService | `webhook.go` | Webhook delivery |
| NotificationService | `notification_service.go` | Email/Telegram |

## Repository Layer

**Directory:** `internal/controller/repository/`

| Repository | File | Tables |
|------------|------|--------|
| VMRepo | `vm_repo.go` | vms, snapshots, backups |
| NodeRepo | `node_repo.go` | nodes, node_heartbeats |
| CustomerRepo | `customer_repo.go` | customers, sessions |
| PlanRepo | `plan_repo.go` | plans |
| TemplateRepo | `template_repo.go` | templates |
| IPRepo | `ip_repo.go` | ip_sets, ip_addresses, ipv6_prefixes |
| BackupRepo | `backup_repo.go` | backups, backup_schedules |
| TaskRepo | `task_repo.go` | tasks |
| AuditRepo | `audit_repo.go` | audit_logs |
| WebhookRepo | `webhook_repo.go` | customer_webhooks, webhook_deliveries |
| SettingsRepo | `settings_repo.go` | system_settings |
| FailoverRepo | `failover_repo.go` | failover_requests |
| ConsoleTokenRepo | `console_token_repo.go` | console_tokens (NEW) |

## Middleware Chain

**Directory:** `internal/controller/api/middleware/`

```
Request → Correlation → Metrics → RateLimit → Recovery → Auth → CSRF → Permissions → Validation → Handler
```

| Middleware | File | Purpose |
|------------|------|---------|
| JWTAuth | `auth.go` | JWT validation, claims extraction |
| APIKeyAuth | `auth.go` | Provisioning API key |
| RequireRole | `auth.go` | Admin role check |
| RequireUserType | `auth.go` | Customer type check |
| RequireAdminPermission | `permissions.go` | Admin RBAC check (NEW) |
| RequirePermission | `permissions.go` | Customer API key permissions (NEW) |
| CSRF | `csrf.go` | Double-submit token |
| RateLimit | `ratelimit.go` | Token bucket per IP/user |
| Metrics | `metrics.go` | Prometheus instrumentation |
| Audit | `audit.go` | Audit log writing |
| Recovery | `recovery.go` | Panic recovery |
| Validation | `validation.go` | Request body validation |
| IPAllowlist | `ip_allowlist.go` | IP-based access control |

## Node Agent gRPC Service

**File:** `proto/virtuestack/node_agent.proto`

```protobuf
service NodeAgentService {
  // Lifecycle
  rpc CreateVM, StartVM, StopVM, ForceStopVM, DeleteVM, ReinstallVM, ResizeVM
  // Migration
  rpc MigrateVM, AbortMigration, PostMigrateSetup, CreateDiskSnapshot, TransferDisk, ReceiveDisk
  // Console (streaming)
  rpc StreamVNCConsole, StreamSerialConsole
  // Metrics
  rpc GetVMStatus, GetVMMetrics, GetNodeResources
  // Snapshots
  rpc CreateSnapshot, DeleteSnapshot, RevertSnapshot, ListSnapshots
  // Guest Agent
  rpc GuestExecCommand, GuestSetPassword, GuestFreezeFilesystems, GuestThawFilesystems
  // Bandwidth
  rpc GetBandwidthUsage, SetBandwidthLimit, ResetBandwidthCounters
}
```