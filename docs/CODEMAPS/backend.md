<!-- Generated: 2026-03-19 | Files scanned: 112 Go files | Token estimate: ~950 -->

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

GET    /nodes, /nodes/:id
POST   /nodes, /nodes/:id/drain, /nodes/:id/failover, /nodes/:id/undrain
DELETE /nodes/:id

GET    /vms, /vms/:id, /vms/:id/ips
POST   /vms, /vms/:id/migrate
DELETE /vms/:id
PUT    /vms/:id, /vms/:id/ips/:ipId/rdns

GET    /plans
POST   /plans
PUT    /plans/:id
DELETE /plans/:id

GET    /templates, /templates/:id
POST   /templates, /templates/:id/import
DELETE /templates/:id

GET    /ip-sets, /ip-sets/:id, /ip-sets/:id/available
POST   /ip-sets
DELETE /ip-sets/:id
PUT    /ip-sets/:id

GET    /customers, /customers/:id, /customers/:id/audit-logs
DELETE /customers/:id
PUT    /customers/:id

GET    /audit-logs
GET    /settings
PUT    /settings/:key

GET    /backups
POST   /backups/:id/restore
GET    /backup-schedules, /backup-schedules/:id
POST   /backup-schedules
DELETE /backup-schedules/:id
PUT    /backup-schedules/:id
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

GET    /templates
POST   /2fa/initiate, /2fa/enable, /2fa/disable
GET    /2fa/status, /2fa/backup-codes
POST   /2fa/backup-codes/regenerate
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
| MigrationService | `migration_service.go` | Live migration |
| FailoverService | `failover_service.go` | HA failover |
| RDNSService | `rdns_service.go` | Reverse DNS |
| RBACService | `rbac_service.go` | Permissions |
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

## Middleware Chain

**Directory:** `internal/controller/api/middleware/`

```
Request → Correlation → Metrics → RateLimit → Recovery → Auth → CSRF → Validation → Handler
```

| Middleware | File | Purpose |
|------------|------|---------|
| JWTAuth | `auth.go` | JWT validation, claims extraction |
| APIKeyAuth | `auth.go` | Provisioning API key |
| RequireRole | `auth.go` | Admin role check |
| RequireUserType | `auth.go` | Customer type check |
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