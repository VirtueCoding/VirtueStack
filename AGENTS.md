# VirtueStack AGENTS.md

> **AI Agent Reference Document**  
> This file provides complete technical context for AI agents working on the VirtueStack codebase.  
> For human-friendly overview, see [README.md](README.md).

**Version:** 2.2  
**Last Updated:** March 2026  
**Purpose:** Machine-readable single source of truth for LLM agents working on VirtueStack

> **Companion document:** [`CODING_STANDARD.md`](CODING_STANDARD.md) defines the rules all generated code must follow — prohibitions, error handling patterns, security requirements, testing rules, and quality gates. This document describes *what exists in the project* (architecture, APIs, data models, config).  
>  
> **Boundary:** If it describes *what exists in the project*, it goes here. If it prescribes *how to write code*, it goes in `CODING_STANDARD.md`.

---

## 1. PROJECT OVERVIEW

VirtueStack is a KVM/QEMU Virtual Machine management platform for VPS hosting providers. Architecture: Go backend (Controller + Node Agent), TypeScript/React frontend (Next.js), PostgreSQL database, NATS JetStream message queue.

---

## 2. REPOSITORY STRUCTURE

```
/home/VirtueStack/
├── cmd/                                    # Entry points
│   ├── controller/main.go                 # Controller orchestrator
│   └── node-agent/main.go                 # Node Agent daemon
│
├── internal/                              # Private implementation
│   ├── controller/                        # Controller component (112 Go files)
│   │   ├── api/                          # HTTP API handlers (39 files)
│   │   │   ├── admin/                   # Admin API (14 files)
│   │   │   ├── customer/                # Customer API (17 files)
│   │   │   ├── provisioning/            # WHMCS provisioning API (8 files)
│   │   │   └── middleware/              # Auth, rate limit, audit (8 files)
│   │   ├── services/                    # Business logic (23 files)
│   │   ├── models/                      # Database models (14 files)
│   │   ├── repository/                  # Database access (19 files)
│   │   ├── tasks/                       # Async task handlers (9 files)
│   │   ├── metrics/                     # Prometheus metrics (1 file)
│   │   └── notifications/               # Email, Telegram (2 files)
│   │
│   ├── nodeagent/                         # Node Agent component (18 Go files)
│   │   ├── server.go                    # gRPC server
│   │   ├── vm/                          # VM lifecycle, console, metrics
│   │   ├── storage/                     # RBD, QCOW, template, cloud-init
│   │   ├── network/                     # Bridge, nwfilter, bandwidth, DHCP, IPv6, abuse prevention
│   │   ├── metrics/                     # Node Agent Prometheus metrics
│   │   └── guest/                       # QEMU Guest Agent
│   │
│   └── shared/                            # Shared packages (8 files)
│       ├── config/                      # Configuration
│       ├── crypto/                      # Encryption utilities
│       ├── errors/                      # Custom error types
│       ├── logging/                     # Structured logging
│       ├── util/                        # Shared utilities (pointer helpers)
│       └── proto/                       # Generated protobuf code
│
├── proto/                                  # Protocol Buffer definitions
│   └── virtuestack/
│       └── node_agent.proto              # gRPC service definition
│
├── migrations/                             # Database migrations (25 files)
│   ├── 000001_initial_schema.up.sql
│   ├── 000019_add_storage_backend.up.sql
│   ├── 000020_add_failover_requests.up.sql
│   ├── 000021_add_attached_iso.up.sql
│   ├── 000022_add_missing_rls_and_grants.up.sql
│   ├── 000023_audit_log_default_partition.up.sql
│   ├── 000024_add_missing_indexes.up.sql
│   └── 000025_add_plan_limits.up.sql
│
├── webui/                                  # Web UIs (82 TSX files)
│   ├── admin/                            # Admin panel (8 pages)
│   └── customer/                         # Customer portal (3 pages)
│
├── modules/                                # WHMCS module (7 PHP files)
│   └── servers/virtuestack/
│
├── configs/                                # Configuration examples
│   ├── grafana/                          # Grafana dashboard templates
│   └── prometheus/                       # Prometheus alerting rules
├── scripts/                                # Utility scripts
│   └── backup-config.sh                  # Database backup script
├── tests/                                  # Test suites
│   ├── integration/                      # Go integration tests (5 files)
│   ├── e2e/                             # Playwright tests (4 files)
│   └── load/                            # k6 load tests (1 file)
│
├── docs/                                   # Documentation
│   ├── ARCHITECTURE.md                  # Architecture specification (2292 lines)
│   ├── API.md                           # API reference
│   ├── INSTALL.md                       # Installation guide
│   └── USAGE.md                         # Usage documentation
│
├── AGENTS.md                              # AI Agent reference (this document)
├── CODING_STANDARD.md                     # Quality gates and coding rules
│
├── docker-compose.yml                      # Docker Compose configuration
├── Makefile                               # Build automation
├── go.mod                                 # Go dependencies
└── README.md                              # Project overview
```

---

## 3. TECHNOLOGY STACK

### Backend
| Component | Technology | Version |
|-----------|------------|---------|
| Language | Go | 1.25 |
| HTTP Framework | Gin | v1.10.0 |
| Database | PostgreSQL | 16+ |
| Message Queue | NATS JetStream | v1.38.0 |
| gRPC | google.golang.org/grpc | v1.79.1 |
| PostgreSQL Driver | pgx/v5 | v5.7.2 |
| Ceph Bindings | go-ceph | v0.30.0 |
| libvirt Bindings | libvirt-go | v1.10005.0 |
| Password Hashing | argon2id | v1.0.0 |
| JWT | golang-jwt/jwt/v5 | v5.2.1 |
| TOTP | pquerna/otp | v1.4.0 |
| Validation | go-playground/validator | v10.23.0 |
| Migrations | golang-migrate/migrate | v4.19.1 |
| WebSocket | gorilla/websocket | v1.5.3 |

### Frontend
| Component | Technology | Version |
|-----------|------------|---------|
| Framework | Next.js | 16+ |
| UI Library | React | 19+ |
| Language | TypeScript | 5.5+ |
| Styling | Tailwind CSS | Latest |
| Components | shadcn/ui | Latest |
| State | TanStack Query + Zustand | Latest |
| Charts | uPlot + Apache ECharts | Latest |

### Infrastructure
| Component | Technology | Version |
|-----------|------------|---------|
| Hypervisor | KVM/QEMU via libvirt | 10.x / 8.x |
| Storage | Ceph RBD OR QCOW2 | Reef/Squid |
| DNS | PowerDNS (optional) | 4.9+ |
| Container | Docker + Compose | 26+ |
| Proxy | Nginx | 1.25+ |

---

## 4. DATABASE SCHEMA

> **Database rules** (indexing, migrations, connection pooling, zero-downtime migrations) are defined in [`CODING_STANDARD.md §7`](CODING_STANDARD.md#7-database).

### 4.1 Core Tables (22 tables)

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `customers` | Customer accounts | id, email, password_hash, totp_secret_encrypted, status |
| `admins` | Admin users | id, email, password_hash, totp_secret_encrypted, role |
| `nodes` | Compute nodes | id, hostname, grpc_address, management_ip, status, storage_backend |
| `locations` | Data center locations | id, name, region, country |
| `plans` | VPS plan templates | id, name, vcpu, memory_mb, disk_gb, port_speed_mbps, storage_backend, snapshot_limit, backup_limit, iso_upload_limit |
| `vms` | Virtual machines | id, customer_id, node_id, plan_id, hostname, status, storage_backend, disk_path |
| `ip_sets` | IP address pools | id, name, location_id, network, gateway |
| `ip_addresses` | IP allocations | id, ip_set_id, address, vm_id, customer_id, rdns_hostname |
| `ipv6_prefixes` | IPv6 /48 allocations | id, node_id, prefix |
| `vm_ipv6_subnets` | VM IPv6 /64 subnets | id, vm_id, ipv6_prefix_id, subnet |
| `templates` | OS templates | id, name, os_family, rbd_image, rbd_snapshot |
| `tasks` | Async task queue | id, type, status, payload, result, progress |
| `backups` | Backup records | id, vm_id, type, status, storage_path, size_bytes |
| `snapshots` | VM snapshots | id, vm_id, name, rbd_snapshot |
| `provisioning_keys` | WHMCS API keys | id, name, key_hash, allowed_ips |
| `customer_api_keys` | Customer API keys | id, customer_id, name, key_hash, permissions |
| `customer_webhooks` | Webhook configurations | id, customer_id, url, secret, events |
| `webhook_deliveries` | Webhook delivery log | id, webhook_id, event_type, status, attempt |
| `audit_logs` | Immutable audit trail | id, timestamp, actor_id, action, resource_type, changes |
| `node_heartbeats` | Node health metrics | id, node_id, timestamp, cpu_percent, memory_percent |
| `system_settings` | Key-value settings | key, value, description |
| `sessions` | Concurrent sessions | id, user_id, refresh_token_hash, expires_at |
| `failover_requests` | HA failover tracking | id, source_node_id, target_node_id, status, requested_at, completed_at |

### 4.2 Row Level Security

```sql
-- Customer isolation policy on vms table
ALTER TABLE vms ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_vms ON vms FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Additional customer isolation policies
ALTER TABLE notification_preferences ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_notification_preferences ON notification_preferences FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

ALTER TABLE notification_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_notification_events ON notification_events FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

ALTER TABLE password_resets ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_password_resets ON password_resets FOR ALL TO app_customer
    USING (user_id = current_setting('app.current_customer_id')::UUID);
```

### 4.3 Key Migrations

| Migration | Purpose |
|-----------|---------|
| 000001_initial_schema | Base tables, indexes, RLS policies |
| 000002_bandwidth_tracking | Bandwidth usage tracking |
| 000009_notification_preferences | Notification settings |
| 000010_webhooks | Webhook system |
| 000011_password_resets | Password reset workflow |
| 000012_template_versioning | Template versioning |
| 000013_customer_api_key_expires_at | API key expiration |
| 000014_audit_log_partitions | Audit log partitioning |
| 000015_session_reauth | Session re-authentication |
| 000016_plan_pricing_slug | Plan pricing identifiers |
| 000017_customer_phone | Customer phone numbers |
| 000018_add_failed_login_attempts | Security tracking |
| 000019_add_storage_backend | QCOW storage backend support |
| 000020_add_failover_requests | HA failover request persistence |
| 000021_add_attached_iso | VM attached ISO tracking |
| 000022_add_missing_rls_and_grants | RLS policies for notification_preferences, notification_events, password_resets + missing GRANTs |
| 000023_audit_log_default_partition | Default partition for audit_logs + future partitions |
| 000024_add_missing_indexes | Missing indexes on vms(plan_id,hostname), nodes(location_id), FK constraints, redundant index cleanup |
| 000025_add_plan_limits | Plan-level limits for snapshots (DEFAULT 2), backups (DEFAULT 2), ISO uploads (DEFAULT 2) |

---

## 5. API ARCHITECTURE

> **API design rules** (versioning, rate limiting, HTTP status codes) are defined in [`CODING_STANDARD.md §15`](CODING_STANDARD.md#15-api-design).

### 5.1 Three-Tier API System

| Tier | Base Path | Auth | Rate Limit |
|------|-----------|------|------------|
| Provisioning | `/api/v1/provisioning/*` | API Key | 1000/min |
| Customer | `/api/v1/customer/*` | JWT + Refresh | 100 read/min, 30 write/min |
| Admin | `/api/v1/admin/*` | JWT + 2FA | 500/min |

### 5.2 Admin API Endpoints

**File:** `internal/controller/api/admin/routes.go`

```go
// Auth
POST /auth/login
POST /auth/verify-2fa
POST /auth/refresh
POST /auth/logout

// Nodes
GET    /nodes
POST   /nodes
GET    /nodes/:id
PUT    /nodes/:id
DELETE /nodes/:id
POST   /nodes/:id/drain
POST   /nodes/:id/failover
POST   /nodes/:id/undrain

// VMs
GET    /vms
POST   /vms
GET    /vms/:id
PUT    /vms/:id
DELETE /vms/:id
POST   /vms/:id/migrate

// Plans
GET    /plans
POST   /plans
PUT    /plans/:id
DELETE /plans/:id

// Templates
GET    /templates
POST   /templates
PUT    /templates/:id
DELETE /templates/:id
POST   /templates/:id/import

// IP Sets
GET    /ip-sets
POST   /ip-sets
GET    /ip-sets/:id
PUT    /ip-sets/:id
DELETE /ip-sets/:id
GET    /ip-sets/:id/available

// Customers
GET    /customers
GET    /customers/:id
PUT    /customers/:id
DELETE /customers/:id
GET    /customers/:id/audit-logs

// Audit & Settings
GET    /audit-logs
GET    /settings
PUT    /settings/:key

// Backups
GET    /backups
POST   /backups/:id/restore
GET    /backup-schedules
POST   /backup-schedules
GET    /backup-schedules/:id
PUT    /backup-schedules/:id
DELETE /backup-schedules/:id
```

### 5.3 Customer API Endpoints

**File:** `internal/controller/api/customer/routes.go`

> **Security:** VM creation and deletion are restricted to Admin and Provisioning APIs only.
> Customers cannot create or delete VMs through the Customer API to prevent abuse
> (e.g., a customer buying one VPS then creating additional VMs for free).

```go
// Auth
POST /auth/login
POST /auth/verify-2fa
POST /auth/refresh
POST /auth/logout
PUT  /password

// Profile
GET  /profile
PUT  /profile

// VMs (read-only — create/delete via Admin and Provisioning APIs only)
GET    /vms
GET    /vms/:id
POST   /vms/:id/start
POST   /vms/:id/stop
POST   /vms/:id/restart
POST   /vms/:id/force-stop

// Console
POST /vms/:id/console-token
POST /vms/:id/serial-token

// Monitoring
GET /vms/:id/metrics
GET /vms/:id/bandwidth
GET /vms/:id/network

// Backups
GET    /backups
POST   /backups
GET    /backups/:id
DELETE /backups/:id
POST   /backups/:id/restore

// Snapshots
GET    /snapshots
POST   /snapshots
DELETE /snapshots/:id
POST   /snapshots/:id/restore

// API Keys
GET    /api-keys
POST   /api-keys
POST   /api-keys/:id/rotate
DELETE /api-keys/:id

// Webhooks
GET    /webhooks
POST   /webhooks
GET    /webhooks/:id
PUT    /webhooks/:id
DELETE /webhooks/:id
GET    /webhooks/:id/deliveries

// Templates & Notifications
GET /templates
GET /notifications/preferences
PUT /notifications/preferences

// 2FA
POST /2fa/initiate
POST /2fa/enable
POST /2fa/disable
GET  /2fa/status
GET  /2fa/backup-codes
POST /2fa/backup-codes/regenerate

// WebSocket
GET /ws/vnc/:vmId
GET /ws/serial/:vmId
```

### 5.4 Provisioning API Endpoints

**File:** `internal/controller/api/provisioning/routes.go`

```go
// VM Lifecycle
POST   /vms
GET    /vms/:id
GET    /vms/by-service/:service_id
DELETE /vms/:id
POST   /vms/:id/suspend
POST   /vms/:id/unsuspend
POST   /vms/:id/resize
POST   /vms/:id/password
POST   /vms/:id/password/reset
POST   /vms/:id/power
GET    /vms/:id/status

// Task Polling
GET /tasks/:id
```

---

## 6. AUTHENTICATION SYSTEM

> **Security and auth rules** (cryptography standards, session hardening, secrets management, zero-trust) are defined in [`CODING_STANDARD.md §4`](CODING_STANDARD.md#4-security).

### 6.1 Authentication Methods

**File:** `internal/controller/api/middleware/auth.go`

| Method | Purpose | Implementation |
|--------|---------|----------------|
| JWT Auth | Customer/Admin sessions | `middleware.JWTAuth()` - Validates Bearer token, extracts claims |
| API Key Auth | Provisioning API | `middleware.APIKeyAuth()` - Validates X-API-Key header |
| 2FA/TOTP | Admin access | `middleware.Require2FA()` - TOTP verification with ±1 step tolerance |

### 6.2 JWT Token Configuration

**File:** `internal/controller/services/auth_service.go`

```go
// Token lifetimes
AccessTokenExpiry:  15 minutes
RefreshTokenExpiry: 7 days (customer), 4 hours (admin)

// Claims structure
type JWTClaims struct {
    UserID   string `json:"user_id"`
    Email    string `json:"email"`
    Role     string `json:"role"`      // "customer", "admin", "super_admin"
    UserType string `json:"user_type"` // "customer" or "admin"
}
```

### 6.3 Password Hashing

**File:** `internal/controller/services/auth_service.go`

```go
// Argon2id configuration
argon2id.CreateHash(password, argon2id.DefaultParams)
// Memory: 64MB, Iterations: 3, Parallelism: 4, SaltLength: 16, KeyLength: 32
```

### 6.4 RBAC Permissions

**File:** `internal/controller/services/rbac_service.go`

```go
// Customer permissions
PermVMList, PermVMStart, PermVMStop, PermVMRestart
PermVMConsole, PermVMReinstall, PermVMBootOrder
PermVMISO, PermVMMetrics, PermVMRDNS
PermVMBackup, PermVMSnapshot, PermVMAPIKey, PermVMWebhook

// Admin-only permissions
PermVMCreate, PermVMDelete, PermVMResize, PermVMMigrate
PermNodeManage, PermNodeFailover, PermIPManage, PermPlanManage
PermTemplateManage, PermCustomerManage, PermBackupManage
PermSettingsManage, PermAuditView
```

---

## 7. STORAGE LAYER (DUAL BACKEND)

### 7.1 Storage Interface

**File:** `internal/nodeagent/storage/interface.go`

```go
type StorageBackend interface {
    CloneFromTemplate(ctx context.Context, templateName, vmUUID string, sizeGB int) error
    CloneSnapshotToPool(ctx context.Context, pool, snapshotName, targetName string) error
    Resize(ctx context.Context, imageName string, newSizeGB int) error
    CreateSnapshot(ctx context.Context, imageName, snapshotName string) error
    DeleteSnapshot(ctx context.Context, imageName, snapshotName string) error
    ProtectSnapshot(ctx context.Context, imageName, snapshotName string) error
    UnprotectSnapshot(ctx context.Context, imageName, snapshotName string) error
    ListSnapshots(ctx context.Context, imageName string) ([]string, error)
    GetImageSize(ctx context.Context, imageName string) (int64, error)
    ImageExists(ctx context.Context, imageName string) (bool, error)
    FlattenImage(ctx context.Context, imageName string) error
    GetPoolStats(ctx context.Context) (PoolStats, error)
    Rollback(ctx context.Context, imageName, snapshotName string) error
    Delete(ctx context.Context, imageName string) error
    GetStorageType() StorageType
}
```

### 7.2 Ceph RBD Implementation

**File:** `internal/nodeagent/storage/rbd.go`

- Library: `github.com/ceph/go-ceph`
- Pools: `vs-vms` (VMs), `vs-images` (templates), `vs-backups` (backups)
- Features: `layering,exclusive-lock,object-map`
- RBD naming: `vs-vms/vs-{vm_uuid}-disk0`

### 7.3 QCOW2 File Implementation

**File:** `internal/nodeagent/storage/qcow.go`

- Uses `qemu-img` commands
- Template storage: `/var/lib/virtuestack/templates/`
- VM storage: `/var/lib/virtuestack/vms/`
- Supports same operations as RBD via interface

### 7.4 Configuration

**File:** `migrations/000019_add_storage_backend.up.sql`

```sql
-- Storage backend per plan
ALTER TABLE plans ADD COLUMN storage_backend VARCHAR(20) DEFAULT 'ceph';

-- Storage backend per node (for local QCOW storage)
ALTER TABLE nodes ADD COLUMN storage_backend VARCHAR(20) DEFAULT 'ceph';
ALTER TABLE nodes ADD COLUMN storage_path TEXT;

-- Storage backend per VM (inherits from plan, immutable after creation)
ALTER TABLE vms ADD COLUMN storage_backend VARCHAR(20) DEFAULT 'ceph';
ALTER TABLE vms ADD COLUMN disk_path TEXT;  -- For QCOW file path
```

**File:** `migrations/000025_add_plan_limits.up.sql`

```sql
-- Plan-level resource limits enforced on customer API
ALTER TABLE plans ADD COLUMN snapshot_limit INT NOT NULL DEFAULT 2;
ALTER TABLE plans ADD COLUMN backup_limit INT NOT NULL DEFAULT 2;
ALTER TABLE plans ADD COLUMN iso_upload_limit INT NOT NULL DEFAULT 2;
```

### 7.5 Backend Selection Rules

1. **VM storage backend is set from plan at creation and is immutable** — cannot be changed or migrated to a different backend
2. **Nodes can host VMs with any backend** (ceph, qcow, or both) — node selection does not filter by storage_backend
3. **Migration is only allowed between nodes supporting the VM's backend** — cross-backend migration (ceph ↔ qcow) is blocked

### 7.6 Plan Resource Limits

**Files:** `internal/controller/api/customer/snapshots.go`, `backups.go`, `iso_upload.go`

Plans enforce per-VM resource limits on the Customer API. These limits are checked before creating snapshots, backups, or uploading ISOs:

| Limit | DB Column | Default | Enforced In |
|-------|-----------|---------|-------------|
| Snapshots per VM | `plans.snapshot_limit` | 2 | `customer/snapshots.go` `CreateSnapshot` |
| Backups per VM | `plans.backup_limit` | 2 | `customer/backups.go` `CreateBackup` |
| ISO uploads per VM | `plans.iso_upload_limit` | 2 | `customer/iso_upload.go` `UploadISO` |

Limit enforcement flow:
1. Get VM (ownership already verified)
2. Look up VM's plan via `planRepo.GetByID(vm.PlanID)`
3. Count existing resources via `backupRepo.CountSnapshotsByVM()` / `CountBackupsByVM()` / filesystem `.iso` count
4. Compare count against plan limit; return `409 Conflict` if exceeded

Admin API and Provisioning API are **not** subject to plan limits.

### 7.7 Customer API Isolation

All `/customer/` endpoints enforce strict customer-only access:

- **VMs:** All operations pass `customerID` + `isAdmin=false` to the service layer. Accessing another customer's VM returns `404 Not Found`.
- **Snapshots:** `verifySnapshotOwnership()` confirms the VM belongs to the customer.
- **Backups:** `verifyBackupOwnership()` confirms the VM belongs to the customer.
- **ISOs:** `GetVM()` with ownership check before any ISO operation.
- **RLS:** PostgreSQL Row Level Security policies enforce `customer_id` isolation at the database level as a defense-in-depth measure.

---

## 8. VM LIFECYCLE

### 8.1 VM States

**File:** `internal/controller/models/vm.go`

```go
const (
    VMStatusProvisioning  = "provisioning"
    VMStatusRunning       = "running"
    VMStatusStopped       = "stopped"
    VMStatusSuspended     = "suspended"
    VMStatusMigrating     = "migrating"
    VMStatusReinstalling  = "reinstalling"
    VMStatusError         = "error"
    VMStatusDeleted       = "deleted"
)
```

### 8.2 Node Agent gRPC Methods

**File:** `proto/virtuestack/node_agent.proto`

```protobuf
service NodeAgentService {
    // VM Lifecycle
    rpc CreateVM(CreateVMRequest) returns (CreateVMResponse);
    rpc StartVM(VMIdentifier) returns (VMOperationResponse);
    rpc StopVM(StopVMRequest) returns (VMOperationResponse);
    rpc ForceStopVM(VMIdentifier) returns (VMOperationResponse);
    rpc DeleteVM(VMIdentifier) returns (VMOperationResponse);
    rpc ReinstallVM(ReinstallVMRequest) returns (CreateVMResponse);
    rpc ResizeVM(ResizeVMRequest) returns (VMOperationResponse);

    // Migration
    rpc MigrateVM(MigrateVMRequest) returns (MigrateVMResponse);
    rpc AbortMigration(VMIdentifier) returns (VMOperationResponse);
    rpc PostMigrateSetup(PostMigrateSetupRequest) returns (VMOperationResponse);
    rpc PrepareMigratedVM(PrepareMigratedVMRequest) returns (VMOperationResponse);

    // Disk Transfer (for QCOW migration)
    rpc CreateDiskSnapshot(CreateDiskSnapshotRequest) returns (CreateDiskSnapshotResponse);
    rpc DeleteDiskSnapshot(DeleteDiskSnapshotRequest) returns (VMOperationResponse);
    rpc TransferDisk(TransferDiskRequest) returns (stream DiskChunk);
    rpc ReceiveDisk(stream DiskChunk) returns (ReceiveDiskResponse);

    // Console (bidirectional streaming)
    rpc StreamVNCConsole(stream VNCFrame) returns (stream VNCFrame);
    rpc StreamSerialConsole(stream SerialData) returns (stream SerialData);

    // Metrics & Status
    rpc GetVMStatus(VMIdentifier) returns (VMStatusResponse);
    rpc GetVMMetrics(VMIdentifier) returns (VMMetricsResponse);
    rpc GetNodeResources(Empty) returns (NodeResourcesResponse);

    // Snapshots
    rpc CreateSnapshot(SnapshotRequest) returns (SnapshotResponse);
    rpc DeleteSnapshot(SnapshotIdentifier) returns (VMOperationResponse);
    rpc RevertSnapshot(SnapshotIdentifier) returns (VMOperationResponse);
    rpc ListSnapshots(VMIdentifier) returns (SnapshotListResponse);

    // Guest Agent
    rpc GuestExecCommand(GuestExecRequest) returns (GuestExecResponse);
    rpc GuestSetPassword(GuestPasswordRequest) returns (VMOperationResponse);
    rpc GuestFreezeFilesystems(VMIdentifier) returns (VMOperationResponse);
    rpc GuestThawFilesystems(VMIdentifier) returns (VMOperationResponse);
    rpc GuestGetNetworkInterfaces(VMIdentifier) returns (GuestNetworkResponse);

    // Bandwidth
    rpc GetBandwidthUsage(VMIdentifier) returns (BandwidthUsageResponse);
    rpc SetBandwidthLimit(BandwidthLimitRequest) returns (VMOperationResponse);
    rpc ResetBandwidthCounters(VMIdentifier) returns (VMOperationResponse);

    // Health
    rpc Ping(Empty) returns (PingResponse);
    rpc GetNodeHealth(Empty) returns (NodeHealthResponse);
}
```

### 8.3 Domain XML Generation

**File:** `internal/nodeagent/vm/domain_xml.go`

Key features:
- KVM acceleration with Q35 chipset
- Virtio devices (disk, network, RNG, balloon)
- Ceph RBD or QCOW2 disk configuration
- Cloud-init ISO attachment
- VNC console (localhost only, Controller proxies)
- Serial console via pty
- Network bandwidth limits via libvirt QoS
- nwfilter anti-spoofing reference

---

## 9. ASYNC TASK SYSTEM

> **Task coding rules** (error handling, resilience, multi-step operations) are defined in [`CODING_STANDARD.md §3`](CODING_STANDARD.md#3-error-handling).

### 9.1 Architecture

```
API Handler → NATS JetStream (durable stream: "TASKS")
    ↓ subscribe
Task Worker Pool (5 workers per Controller)
    ↓ execute
Node Agent (gRPC)
    ↓ result
Update PostgreSQL task status + notify WebSocket subscribers
```

### 9.2 Task Types

**File:** `internal/controller/tasks/handlers.go`

| Task Type | Handler File | Purpose |
|-----------|--------------|---------|
| `vm.create` | vm_service.go | Async VM provisioning |
| `vm.reinstall` | tasks/vm_reinstall.go | OS reinstallation |
| `vm.resize` | tasks/vm_resize.go | Resource resize |
| `vm.migrate` | tasks/migration_execute.go | Live migration |
| `backup.create` | tasks/backup_create.go | Backup creation |
| `backup.restore` | backup_service.go | Backup restoration |
| `snapshot.create` | tasks/snapshot_handlers.go | Snapshot creation |
| `snapshot.revert` | tasks/snapshot_handlers.go | Snapshot revert |
| `webhook.deliver` | tasks/webhook_deliver.go | Webhook delivery |

### 9.3 Task State Machine

```
pending → running → completed
              → failed (with error_message)
pending → cancelled
```

---

## 10. NETWORKING

### 10.1 Network Stack

**Files:**
- `internal/nodeagent/network/nwfilter.go` - Anti-spoofing filters
- `internal/nodeagent/network/bandwidth.go` - tc QoS + nftables
- `internal/nodeagent/network/dhcp.go` - dnsmasq management
- `internal/nodeagent/network/ipv6.go` - IPv6 prefix allocation
- `internal/nodeagent/network/abuse_prevention.go` - nftables abuse prevention

### 10.2 NWFilter Anti-Spoofing

Prevents:
- MAC spoofing
- IP spoofing (IPv4/IPv6)
- ARP spoofing
- DHCP spoofing
- Router Advertisement spoofing

### 10.3 Bandwidth Management

Three-layer approach:
1. **Port Speed Limiting** - libvirt `<bandwidth>` in domain XML
2. **Bandwidth Accounting** - nftables named counters per VM tap interface
3. **Overage Throttling** - tc HTB qdisc when cap exceeded

---

## 11. WEB UIs

### 11.1 Admin Portal

**Path:** `webui/admin/`

| Page | File | Features |
|------|------|----------|
| Login | `app/login/page.tsx` | JWT auth + 2FA |
| Dashboard | `app/dashboard/page.tsx` | Node overview, alerts |
| VMs | `app/vms/page.tsx` | Full VM CRUD |
| Nodes | `app/nodes/page.tsx` | Node management, drain, failover |
| Customers | `app/customers/page.tsx` | Customer management |
| Plans | `app/plans/page.tsx` | Plan management with resource limit editing (snapshot, backup, ISO) |
| IP Sets | `app/ip-sets/page.tsx` | IP pool management |
| Audit Logs | `app/audit-logs/page.tsx` | Audit trail viewer |
| Settings | `app/settings/page.tsx` | System settings management |

### 11.2 Customer Portal

**Path:** `webui/customer/`

| Page | File | Features |
|------|------|----------|
| Login | `app/login/page.tsx` | JWT auth |
| VM List | `app/vms/page.tsx` | List own VMs |
| VM Detail | `app/vms/[id]/page.tsx` | Control, console, metrics |
| Settings | `app/settings/page.tsx` | Profile, 2FA, API keys |

### 11.3 Key Components

- **VNC Console:** `components/novnc-console/vnc-console.tsx` - noVNC integration
- **Serial Console:** `components/serial-console/serial-console.tsx` - xterm.js
- **Resource Charts:** `components/charts/resource-charts.tsx` - uPlot + ECharts
- **ISO Upload:** `components/file-upload/iso-upload.tsx` - tus protocol

---

## 12. WHMCS INTEGRATION

### 12.1 Module Structure

**Path:** `modules/servers/virtuestack/`

| File | Purpose |
|------|---------|
| `virtuestack.php` | Main module (946 lines) |
| `lib/ApiClient.php` | Controller API client |
| `lib/VirtueStackHelper.php` | Utilities |
| `hooks.php` | WHMCS hooks |
| `webhook.php` | Webhook receiver |
| `templates/overview.tpl` | Client area template |

### 12.2 WHMCS Functions

```php
virtuestack_CreateAccount()   // Provision VM
virtuestack_SuspendAccount()  // Suspend VM
virtuestack_UnsuspendAccount() // Unsuspend VM
virtuestack_TerminateAccount() // Delete VM
virtuestack_ChangePackage()   // Resize VM
virtuestack_ChangePassword()  // Reset password
virtuestack_ClientArea()      // Embed Customer WebUI
virtuestack_UsageUpdate()       // Usage metering (stub)
virtuestack_SingleSignOn()      // WebUI SSO (stub)
virtuestack_AdminServicesTabFieldsSave()  // Admin tab save (stub)
```

---

## 13. IMPLEMENTATION PATTERNS

> **Coding rules for these patterns** (error wrapping, validation, naming, structure) are defined in [`CODING_STANDARD.md`](CODING_STANDARD.md) — see §2 (Structural Rules), §3 (Error Handling), §5 (Input Validation).

### 13.1 Error Handling

**File:** `internal/shared/errors/errors.go`

```go
// Custom error types
type ValidationError struct { Field, Message string }
type AuthenticationError struct{}
type ForbiddenError struct{}
type NotFoundError struct{ ResourceType, ResourceID string }
type ConflictError struct{}
type RateLimitError struct{}
type InternalError struct{ Message string }
```

### 13.2 API Response Format

**Success:**
```json
{
  "data": { ... },
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 150,
    "total_pages": 8
  }
}
```

**Error:**
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Hostname format is invalid",
    "details": [{ "field": "hostname", "issue": "Must be RFC 1123 compliant" }],
    "correlation_id": "req_abc123"
  }
}
```

### 13.3 Repository Pattern

**File:** `internal/controller/repository/vm_repo.go`

```go
type VMRepository struct {
    db *pgxpool.Pool
}

func (r *VMRepository) GetByID(ctx context.Context, id string) (*models.VM, error)
func (r *VMRepository) List(ctx context.Context, filter models.VMListFilter) ([]*models.VM, error)
func (r *VMRepository) Create(ctx context.Context, vm *models.VM) error
func (r *VMRepository) Update(ctx context.Context, vm *models.VM) error
func (r *VMRepository) Delete(ctx context.Context, id string) error
```

---

## 14. QUALITY GATES COMPLIANCE

**Reference:** `CODING_STANDARD.md`

| QG | Status | Implementation |
|----|--------|----------------|
| QG-01 Readable | ✅ | Max 40-line functions, clear naming |
| QG-02 Secure | ✅ | OWASP 2025, mTLS, input validation |
| QG-03 Typed | ✅ | Go strict types, TypeScript strict |
| QG-04 Structured | ✅ | Custom errors, operation journals |
| QG-05 Validated | ✅ | go-playground/validator, Zod |
| QG-06 DRY | ✅ | Shared packages, component reuse |
| QG-07 Defensive | ✅ | Null checks, timeouts, error handling |
| QG-08 Logged | ✅ | slog JSON, correlation IDs |
| QG-09 Bounded | ✅ | HTTP/gRPC/DB timeouts |
| QG-10 Clean | ✅ | golangci-lint configured |
| QG-11 Documented | ✅ | Comprehensive docs |
| QG-12 Configurable | ✅ | Env vars + YAML |
| QG-13 Compatible | ✅ | API versioning, migrations |
| QG-14 Tested | ✅ | Integration + E2E tests |
| QG-15 Dependency-Safe | ✅ | Pinned versions |
| QG-16 Performant | ✅ | Pagination, connection pooling |
| QG-17 Provenance-Verified | ✅ | SBOM, cosign signatures, SLSA Level 2+ |
| QG-18 Observable | ✅ | Prometheus metrics, distributed tracing, health probes |
| QG-19 Deployment-Safe | ✅ | Non-root containers (Controller, WebUIs), minimal images, graceful shutdown (Node Agent runs as host binary with elevated privileges) |

---

## 15. ENVIRONMENT VARIABLES

### Controller

| Variable | Required | Description |
|----------|----------|-------------|
| DATABASE_URL | Yes | PostgreSQL connection string |
| NATS_URL | Yes | NATS server URL |
| JWT_SECRET | Yes | HMAC secret for JWT signing |
| ENCRYPTION_KEY | Yes | AES-256 key for secret encryption |
| PDNS_MYSQL_DSN | No | PowerDNS MySQL connection |
| SMTP_HOST | No | SMTP server hostname |
| TELEGRAM_BOT_TOKEN | No | Telegram bot token |
| LOG_LEVEL | No | Logging level (debug/info/warn/error) |
| LISTEN_ADDR | No | HTTP listen address (default :8080) |
| NATS_AUTH_TOKEN | No | NATS server authentication token |

### Node Agent

| Variable | Required | Description |
|----------|----------|-------------|
| CONTROLLER_GRPC_ADDR | Yes | Controller gRPC address |
| NODE_ID | Yes | Unique node identifier |
| CEPH_POOL | No | Default Ceph pool |
| CEPH_USER | No | Ceph auth user |
| STORAGE_BACKEND | No | Storage backend: `ceph` or `qcow` (default: `ceph`) |
| STORAGE_PATH | No | Base path for QCOW2 VM storage |
| TEMPLATE_PATH | No | Base path for QCOW2 template storage |
| TLS_CERT_FILE | Yes | mTLS client certificate |
| TLS_KEY_FILE | Yes | mTLS client key |
| TLS_CA_FILE | Yes | CA certificate |

---

## 16. WHAT'S LEFT TO IMPLEMENT

All planned features are complete. The platform is fully implemented.

### Future Enhancements (Low Priority)

1. IPv6 BGP announcement coordination

---

## 17. FOR LLM AGENTS: HOW TO CONTINUE

### When Adding Features

1. **Check Architecture Plan:** Reference `docs/ARCHITECTURE.md`
2. **Follow Coding Standard:** Reference `CODING_STANDARD.md`
3. **Use Existing Patterns:**
   - Storage: Add to `StorageBackend` interface, implement for both backends
   - APIs: Add handler in appropriate tier (`api/admin/`, `api/customer/`, `api/provisioning/`)
   - Services: Business logic in `internal/controller/services/`
   - Models: Data structures in `internal/controller/models/`
   - Repositories: Database access in `internal/controller/repository/`

### Common Tasks

**Adding a New Storage Operation:**
1. Add method to `StorageBackend` interface (`internal/nodeagent/storage/interface.go`)
2. Implement in `rbd.go` and `qcow.go`
3. Update domain XML generation if needed (`internal/nodeagent/vm/domain_xml.go`)
4. Add gRPC handler in `server.go`

**Node Selection for VMs:**
- Nodes can host VMs with any storage backend (ceph or qcow)
- Node selection picks the least-loaded online node regardless of backend
- The VM's storage_backend is immutable after creation
- Migration is blocked if target node doesn't support the VM's backend

**Adding a New API Endpoint:**
1. Add handler in appropriate `api/{tier}/` directory
2. Add route in `routes.go`
3. Add middleware (auth, validation, audit)
4. Call appropriate service method
5. Return standardized response format

**Adding a New Async Task:**
1. Define task type in `models/task.go`
2. Create handler in `tasks/{task_name}.go`
3. Register in `worker.go`
4. Publish from API handler via NATS

### Key File References

| Purpose | File(s) |
|---------|---------|
| Storage Backend | `internal/nodeagent/storage/interface.go`, `rbd.go`, `qcow.go` |
| VM Lifecycle | `internal/nodeagent/vm/lifecycle.go`, `internal/controller/services/vm_service.go` |
| Admin API | `internal/controller/api/admin/*.go` |
| Customer API | `internal/controller/api/customer/*.go` |
| Provisioning API | `internal/controller/api/provisioning/*.go` |
| Middleware | `internal/controller/api/middleware/*.go` |
| Metrics (Controller) | `internal/controller/metrics/prometheus.go` |
| Metrics Middleware | `internal/controller/api/middleware/metrics.go` |
| Metrics (Node Agent) | `internal/nodeagent/metrics/prometheus.go` |
| Abuse Prevention | `internal/nodeagent/network/abuse_prevention.go` |
| ISO Upload | `internal/controller/api/customer/iso_upload.go` |
| VM Resize Task | `internal/controller/tasks/vm_resize.go` |
| Failover | `internal/controller/repository/failover_repo.go`, `internal/controller/models/failover.go`, `internal/controller/api/admin/failover.go`, `internal/controller/services/heartbeat_checker.go` |
| Heartbeat Checker | `internal/controller/services/heartbeat_checker.go` |
| rDNS Service | `internal/controller/services/rdns_service.go`, `internal/controller/api/admin/rdns.go`, `internal/controller/api/provisioning/rdns.go` |
| Models | `internal/controller/models/*.go` |
| Repositories | `internal/controller/repository/*.go` |
| Services | `internal/controller/services/*.go` |
| Tasks | `internal/controller/tasks/*.go` |
| Shared Utilities | `internal/shared/util/pointers.go` |
| gRPC Proto | `proto/virtuestack/node_agent.proto` |
| Database | `migrations/*.sql` |
| Backup Script | `scripts/backup-config.sh` |

---

## 18. BUILD & DEPLOYMENT

### Testing Methodology

VirtueStack uses a hybrid testing approach:

- **Docker stack** (Controller, NATS, PostgreSQL, Admin UI, Customer UI, Nginx) — run via `make docker-build && make docker-up`. This replicates the production runtime (multi-stage build, non-root user, network isolation, service wiring).
- **Node Agent** — build and run directly on the host via `make build-node-agent`. The Node Agent connects to the host's libvirt daemon and is not containerized during testing. It requires KVM/libvirt, mTLS certificates, and direct hardware access.

For integration and E2E testing, use the Docker stack for the Controller side and run the Node Agent binary separately on a real KVM node. Unit tests (`make test`) may run outside Docker since they test logic in isolation.

### Build Commands

```bash
# Build all binaries
make build

# Build specific components
make build-controller
make build-node-agent

# Run tests
make test
make test-race

# Database migrations
make migrate-up
make migrate-down
make migrate-create NAME=feature_name

# Generate proto code
make proto

# Docker deployment
docker compose up -d
```

### Docker Services

| Service | Image | Ports | Dependencies |
|---------|-------|-------|--------------|
| postgres | postgres:16-alpine | Internal | - |
| nats | nats:2.10-alpine | Internal | - (supports --auth token) |
| controller | virtuestack/controller | Internal | postgres, nats |
| admin-webui | virtuestack/admin-webui | Internal | controller |
| customer-webui | virtuestack/customer-webui | Internal | controller |
| nginx | nginx:1.25-alpine | 80, 443 | all |

> **Note:** The Node Agent is not a Docker service in this stack. Build it with `make build-node-agent` and run directly on each hypervisor node.

---

**END OF LLM SCOPE DOCUMENT**

*This document is a living reference. Update as implementation progresses.*
*For architecture details, see `ARCHITECTURE.md`.*
*For coding standards, see `CODING_STANDARD.md`.*
