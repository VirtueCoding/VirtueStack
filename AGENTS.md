# VirtueStack AGENTS.md

Machine-readable reference for LLM agents working on the VirtueStack codebase.

**Audience:** LLMs and AI coding agents only.
**Companion documents:** `docs/coding-standard.md` (rules for generated code), `docs/codemaps/` (token-lean architecture summaries ~4K tokens total).
**Boundary:** This file describes what exists. `docs/coding-standard.md` prescribes how to write code.

---

## 1. PROJECT OVERVIEW

VirtueStack is a KVM/QEMU Virtual Machine management platform for VPS hosting providers.

**Components:** Go backend (Controller + Node Agent), TypeScript/React frontends (Next.js admin and customer portals), PostgreSQL 16 database with Row-Level Security, NATS JetStream message queue, Redis for distributed rate limiting.

**Communication:** Controller ↔ Node Agent via gRPC with mTLS. Controller ↔ WHMCS via Provisioning REST API.

---

## 2. REPOSITORY STRUCTURE

```
cmd/
  controller/main.go                        # Controller entry point
  node-agent/main.go                        # Node Agent entry point
internal/
  controller/
    api/
      admin/                                # Admin API handlers (routes.go + handlers)
      customer/                             # Customer API handlers (routes.go + handlers)
      provisioning/                         # WHMCS provisioning API handlers
      middleware/                           # Auth, rate limit, audit, CSRF, permissions, metrics (19 files)
    audit/masking.go                        # Audit log field masking
    metrics/prometheus.go                   # Controller Prometheus metrics
    models/                                 # Data models (38 files)
    notifications/                          # Email + Telegram notifications (+ templates/)
    redis/client.go                         # Redis client for distributed rate limiting
    repository/                             # Database access layer (30 files)
    services/                               # Business logic (41 files)
    tasks/                                  # Async task handlers (25 files)
    config.go                               # Re-exports shared config
    grpc_client.go                          # gRPC client to Node Agents
    server.go                               # HTTP server wiring, route registration
  nodeagent/
    config.go                               # Node Agent config
    server.go                               # gRPC server setup
    health_server.go                        # Health check server
    grpc_handlers_backup.go                 # Backup gRPC handlers
    grpc_handlers_console.go                # Console gRPC handlers
    grpc_handlers_guest.go                  # Guest Agent gRPC handlers
    grpc_handlers_network.go                # Network gRPC handlers
    grpc_handlers_qcow.go                   # QCOW disk transfer handlers
    grpc_handlers_snapshot.go               # Snapshot gRPC handlers
    grpc_handlers_storage.go                # Storage gRPC handlers
    grpc_handlers_vm_lifecycle.go           # VM lifecycle gRPC handlers
    guest/agent.go                          # QEMU Guest Agent interface
    metrics/prometheus.go                   # Node Agent Prometheus metrics
    network/
      abuse_prevention.go                   # nftables abuse prevention
      bandwidth.go                          # tc QoS + nftables
      dhcp.go                               # dnsmasq management
      dhcp_helpers.go
      ipv6.go                               # IPv6 prefix allocation
      nwfilter.go                           # Anti-spoofing filters
      nwfilter_helpers.go
    storage/
      cloudinit.go                          # Cloud-init ISO generation
      factory.go                            # Storage backend factory (ceph/qcow/lvm)
      interface.go                          # StorageBackend interface
      lvm.go                                # LVM thin provisioning
      lvm_template.go                       # LVM template operations
      qcow.go                               # QCOW2 file-based storage
      qcow_template.go                      # QCOW template operations
      rbd.go                                # Ceph RBD storage
      template.go                           # Template management
      template_builder.go                   # ISO-to-template builder
    vm/
      domain_xml.go                         # libvirt domain XML generation
      lifecycle.go                          # VM start/stop/create/delete
      lifecycle_helpers.go
      metrics.go                            # VM metrics collection
  shared/
    config/config.go                        # ALL config/env var loading (743 lines) — primary config source
    crypto/crypto.go                        # AES-256-GCM encryption
    errors/errors.go                        # Custom error types + sentinel errors
    libvirtutil/libvirtutil.go              # libvirt helper utilities
    logging/logging.go                      # Structured slog setup
    proto/virtuestack/                      # Generated protobuf Go code
    util/
      email.go                              # Email validation
      pointers.go                           # Generic pointer helpers
      ssrf.go                               # SSRF prevention
proto/virtuestack/node_agent.proto          # gRPC service definition (972 lines, 38 RPCs)
migrations/                                 # 65 sequential migrations (000001–000065)
webui/
  admin/                                    # Next.js admin portal (15 pages)
  customer/                                 # Next.js customer portal (6 pages)
modules/servers/virtuestack/                # WHMCS billing module (PHP, 8 files)
tests/
  integration/                              # Go integration tests (7 files, Testcontainers)
  e2e/                                      # Playwright E2E tests (17 spec files)
  load/                                     # k6 load tests
  security/                                 # OWASP ZAP script
configs/
  grafana/                                  # Grafana dashboard JSON
  nodeagent.yaml                            # Node Agent YAML config example
  prometheus/                               # Prometheus alerts + scrape config
scripts/
  backup-config.sh                          # Database backup script
  setup-e2e.sh                              # E2E environment setup
templates/email/                            # Email notification templates (5 files)
nginx/                                      # Nginx reverse proxy config
docker-compose.yml                          # Base Docker Compose
docker-compose.override.yml                 # Development overrides
docker-compose.prod.yml                     # Production overrides
docker-compose.test.yml                     # Testing overrides
Dockerfile.controller
Dockerfile.admin-webui
Dockerfile.customer-webui
Makefile                                    # Build automation
.golangci.yml                               # 25 linters enabled
go.mod                                      # Go 1.26
.env.example                                # Environment variable template
docs/
  api-reference.md                          # API reference
  architecture.md                           # Architecture specification
  coding-standard.md                        # Quality gates (mandatory)
  installation.md                           # Installation guide
  plan.md                                   # Billing architecture decision plan
  codemaps/                                 # Token-lean architecture summaries
  decisions/001-redis-rate-limiting.md      # ADR: Redis rate limiting
  superpowers/security-audit-report.md
```

---

## 3. TECHNOLOGY STACK

### Backend

| Component | Technology | Version |
|-----------|------------|---------|
| Language | Go | 1.26 |
| HTTP Framework | Gin | v1.10.1 |
| Database | PostgreSQL | 16+ |
| Message Queue | NATS JetStream | v1.38.0 |
| Cache/Rate Limit | Redis (go-redis/v9) | — |
| gRPC | google.golang.org/grpc | v1.79.1 |
| PostgreSQL Driver | pgx/v5 | v5.7.2 |
| Migrations | golang-migrate/migrate | v4.19.1 |
| Ceph Bindings | go-ceph | v0.30.0 |
| libvirt Bindings | libvirt-go | v1.10005.0 |
| Password Hashing | argon2id | v1.0.0 |
| JWT | golang-jwt/jwt/v5 | v5.2.2 |
| TOTP | pquerna/otp | v1.4.0 |
| Validation | go-playground/validator | v10.26.0 |
| WebSocket | gorilla/websocket | v1.5.3 |
| Prometheus | client_golang | v1.20.5 |

### Frontend

| Component | Technology | Version |
|-----------|------------|---------|
| Framework | Next.js | 16+ |
| UI Library | React | 19 |
| Language | TypeScript | 5.7 |
| Styling | Tailwind CSS | Latest |
| Components | shadcn/ui (Radix primitives) | Latest |
| Server State | TanStack Query | 5.64 |
| Charts | Recharts | 3.8 |
| VNC Console | noVNC | 1.5 |
| Serial Console | xterm.js | 6.0 |
| Forms | react-hook-form | 7 |
| Validation | Zod | 3.24 |

### Infrastructure

| Component | Technology |
|-----------|------------|
| Hypervisor | KVM/QEMU via libvirt |
| Storage | Ceph RBD, QCOW2, or LVM thin provisioning |
| DNS | PowerDNS (optional) |
| Containers | Docker + Compose |
| Proxy | Nginx 1.25+ |

---

## 4. DATABASE

### 4.1 Overview

- 65 sequential migrations (`migrations/000001–000065`)
- 30+ tables
- Row-Level Security on all customer-facing tables
- Connection pool: 25 max connections, 5 min idle

### 4.2 Key Tables

| Table | Purpose |
|-------|---------|
| `customers` | Customer accounts (email, password_hash, totp, status) |
| `admins` | Admin users (email, password_hash, totp, role) |
| `nodes` | Compute nodes (hostname, grpc_address, status) |
| `locations` | Data center locations |
| `plans` | VPS plan templates (vcpu, memory, disk, limits) |
| `vms` | Virtual machines (customer_id, node_id, plan_id, status, storage_backend) |
| `ip_sets` | IP address pools |
| `ip_addresses` | IP allocations (vm_id, customer_id, rdns_hostname) |
| `ipv6_prefixes` | IPv6 /48 allocations per node |
| `vm_ipv6_subnets` | VM IPv6 /64 subnets |
| `templates` | OS templates |
| `template_node_cache` | Per-node template cache status |
| `tasks` | Async task queue (type, status, payload, result, progress) |
| `backups` | Backup records |
| `snapshots` | VM snapshots |
| `provisioning_keys` | WHMCS API keys |
| `customer_api_keys` | Customer API keys (permissions, vm_ids, allowed_ips) |
| `customer_webhooks` | Webhook configurations |
| `webhook_deliveries` | Webhook delivery log |
| `audit_logs` | Immutable audit trail (partitioned) |
| `node_heartbeats` | Node health metrics |
| `system_settings` | Key-value settings |
| `sessions` | User sessions (refresh_token_hash, expires_at) |
| `failover_requests` | HA failover tracking |
| `storage_backends` | Storage backend registry (type, config, health_status) |
| `node_storage` | Junction: nodes ↔ storage backends |
| `iso_uploads` | ISO file upload tracking |
| `sso_tokens` | One-time SSO tokens for WHMCS integration |

### 4.3 Row-Level Security

RLS is enabled on: `vms`, `notification_preferences`, `notification_events`, `password_resets`, `customer_api_keys`, `ip_addresses`, `backups`, `snapshots`, `backup_schedules`, `sessions`, `customers`.

All policies use `current_setting('app.current_customer_id')::UUID` for customer isolation.

---

## 5. API ARCHITECTURE

### 5.1 Three-Tier System

| Tier | Base Path | Auth | Rate Limit |
|------|-----------|------|------------|
| Admin | `/api/v1/admin/*` | JWT + 2FA + RBAC | 500/min |
| Customer | `/api/v1/customer/*` | JWT or Customer API Key | 100 read/min, 30 write/min |
| Provisioning | `/api/v1/provisioning/*` | API Key (X-API-Key) | 1000/min |

### 5.2 Admin API Endpoints

**File:** `internal/controller/api/admin/routes.go`

```
POST   /auth/login
POST   /auth/verify-2fa
POST   /auth/refresh
POST   /auth/logout
GET    /auth/me
POST   /auth/reauth
GET    /auth/permissions
PUT    /auth/permissions/:admin_id

GET    /admins

GET    /nodes
POST   /nodes
GET    /nodes/:id
PUT    /nodes/:id
DELETE /nodes/:id
POST   /nodes/:id/drain
POST   /nodes/:id/failover
POST   /nodes/:id/undrain

GET    /failover-requests
GET    /failover-requests/:id

GET    /vms
POST   /vms
GET    /vms/:id
PUT    /vms/:id
DELETE /vms/:id
GET    /vms/:id/ips
POST   /vms/:id/migrate
GET    /vms/:id/ips/:ipId/rdns
PUT    /vms/:id/ips/:ipId/rdns
DELETE /vms/:id/ips/:ipId/rdns

GET    /plans
POST   /plans
PUT    /plans/:id
DELETE /plans/:id
GET    /plans/:id/usage

GET    /templates
POST   /templates
GET    /templates/:id
PUT    /templates/:id
DELETE /templates/:id
POST   /templates/:id/import
POST   /templates/:id/distribute
POST   /templates/build-from-iso
GET    /templates/:id/cache-status

GET    /ip-sets
POST   /ip-sets
GET    /ip-sets/:id
PUT    /ip-sets/:id
DELETE /ip-sets/:id
GET    /ip-sets/:id/available

GET    /customers
POST   /customers
GET    /customers/:id
PUT    /customers/:id
DELETE /customers/:id
GET    /customers/:id/audit-logs

GET    /audit-logs
GET    /settings
PUT    /settings/:key

GET    /backups
POST   /backups/:id/restore

GET    /backup-schedules
POST   /backup-schedules
GET    /backup-schedules/:id
PUT    /backup-schedules/:id
DELETE /backup-schedules/:id
POST   /backup-schedules/:id/run

GET    /admin-backup-schedules
POST   /admin-backup-schedules
GET    /admin-backup-schedules/:id
PUT    /admin-backup-schedules/:id
DELETE /admin-backup-schedules/:id
POST   /admin-backup-schedules/:id/run

GET    /storage-backends
POST   /storage-backends
GET    /storage-backends/:id
PUT    /storage-backends/:id
DELETE /storage-backends/:id
GET    /storage-backends/:id/nodes
POST   /storage-backends/:id/nodes
DELETE /storage-backends/:id/nodes
GET    /storage-backends/:id/health
POST   /storage-backends/:id/health
POST   /storage-backends/:id/refresh

GET    /provisioning-keys
POST   /provisioning-keys
GET    /provisioning-keys/:id
PUT    /provisioning-keys/:id
DELETE /provisioning-keys/:id
```

### 5.3 Customer API Endpoints

**File:** `internal/controller/api/customer/routes.go`

Dual auth: JWT (full access) or Customer API Key (scoped permissions). JWT-only endpoints: account management, 2FA, webhooks, API keys. VM creation/deletion restricted to Admin and Provisioning APIs only.

```
GET    /csrf

POST   /auth/login
POST   /auth/verify-2fa
POST   /auth/refresh
POST   /auth/logout
GET    /auth/sso-exchange
POST   /auth/forgot-password
POST   /auth/reset-password

PUT    /password
GET    /profile
PUT    /profile

GET    /vms
GET    /vms/:id
POST   /vms/:id/start
POST   /vms/:id/stop
POST   /vms/:id/restart
POST   /vms/:id/force-stop
POST   /vms/:id/console-token
POST   /vms/:id/serial-token
GET    /vms/:id/metrics
GET    /vms/:id/bandwidth
GET    /vms/:id/network
GET    /vms/:id/ips
GET    /vms/:id/ips/:ipId/rdns
PUT    /vms/:id/ips/:ipId/rdns
DELETE /vms/:id/ips/:ipId/rdns
POST   /vms/:id/iso/upload
GET    /vms/:id/iso
DELETE /vms/:id/iso/:isoId
POST   /vms/:id/iso/:isoId/attach
POST   /vms/:id/iso/:isoId/detach

GET    /ws/vnc/:vmId
GET    /ws/serial/:vmId

GET    /backups
POST   /backups
GET    /backups/:id
DELETE /backups/:id
POST   /backups/:id/restore

GET    /snapshots
POST   /snapshots
DELETE /snapshots/:id
POST   /snapshots/:id/restore

GET    /tasks/:id
GET    /templates

GET    /api-keys
POST   /api-keys
POST   /api-keys/:id/rotate
DELETE /api-keys/:id

GET    /webhooks
POST   /webhooks
GET    /webhooks/:id
PUT    /webhooks/:id
DELETE /webhooks/:id
POST   /webhooks/:id/test
GET    /webhooks/:id/deliveries

POST   /2fa/initiate
POST   /2fa/enable
POST   /2fa/disable
GET    /2fa/status
GET    /2fa/backup-codes
POST   /2fa/backup-codes/regenerate

GET    /notifications/preferences
PUT    /notifications/preferences
```

### 5.4 Provisioning API Endpoints

**File:** `internal/controller/api/provisioning/routes.go`

```
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
GET    /vms/:id/usage
GET    /vms/:id/rdns
PUT    /vms/:id/rdns

GET    /tasks/:id
POST   /customers
POST   /sso-tokens

GET    /plans
GET    /plans/:id
```

### 5.5 Error Response Format

All handlers use `middleware.RespondWithError(c, status, code, message)`. Never use `c.JSON()` for errors.

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid hostname format",
    "correlation_id": "req_abc123"
  }
}
```

### 5.6 Success Response Format

```json
{
  "data": { ... },
  "meta": { "page": 1, "per_page": 20, "total": 150, "total_pages": 8 }
}
```

---

## 6. AUTHENTICATION & AUTHORIZATION

### 6.1 Auth Methods

| Method | Middleware | Purpose |
|--------|-----------|---------|
| JWT | `middleware.JWTAuth()` | Customer/Admin sessions (Bearer token or cookie) |
| JWT + API Key | `middleware.JWTOrCustomerAPIKeyAuth()` | Customer API dual auth (X-API-Key header) |
| API Key | `middleware.APIKeyAuth()` | Provisioning API (X-API-Key header) |
| 2FA/TOTP | `middleware.Require2FA()` | Admin access (±1 step tolerance) |

### 6.2 JWT Configuration

- Access token: 15 minutes
- Refresh token: 7 days (customer), 4 hours (admin)
- Claims: user_id, email, role (`customer`/`admin`/`super_admin`), user_type (`customer`/`admin`)
- Password hashing: argon2id (64MB memory, 3 iterations, parallelism 4)

### 6.3 Admin RBAC Permissions

**File:** `internal/controller/models/permission.go`

27 permissions across 12 resources:

```
plans:read, plans:write, plans:delete
nodes:read, nodes:write, nodes:delete
customers:read, customers:write, customers:delete
vms:read, vms:write, vms:delete
settings:read, settings:write
backups:read, backups:write
ipsets:read, ipsets:write, ipsets:delete
templates:read, templates:write
rdns:read, rdns:write
audit_logs:read
storage_backends:read, storage_backends:write, storage_backends:delete
```

Roles: `super_admin` (all permissions), `admin` (configurable), `viewer` (read-only).

### 6.4 Customer API Key Permissions

7 scopes: `vm:read`, `vm:write`, `vm:power`, `backup:read`, `backup:write`, `snapshot:read`, `snapshot:write`.

- JWT requests: full access (permissions = nil)
- API key requests: limited to granted scopes
- Optional `vm_ids` field restricts key to specific VMs
- Optional `allowed_ips` whitelist (IPv4, IPv6, CIDR)
- Missing permission returns HTTP 403

---

## 7. STORAGE LAYER

### 7.1 Three Storage Backends

**Factory:** `internal/nodeagent/storage/factory.go`

| Backend | File | Technology |
|---------|------|------------|
| Ceph RBD | `rbd.go` | go-ceph library. Pools: `vs-vms`, `vs-images`, `vs-backups` |
| QCOW2 | `qcow.go` | `qemu-img` commands, file-based |
| LVM | `lvm.go` | Thin provisioning, CoW snapshots |

### 7.2 StorageBackend Interface

**File:** `internal/nodeagent/storage/interface.go`

Methods: `CloneFromTemplate`, `CloneSnapshotToPool`, `Resize`, `CreateSnapshot`, `DeleteSnapshot`, `ProtectSnapshot`, `UnprotectSnapshot`, `ListSnapshots`, `GetImageSize`, `ImageExists`, `FlattenImage`, `GetPoolStats`, `Rollback`, `Delete`, `GetStorageType`.

### 7.3 Storage Backend Registry

Admin-managed via `/admin/storage-backends/*` API. Junction table `node_storage` links backends to nodes. Migration service verifies backend compatibility via this junction table.

### 7.4 Backend Selection Rules

1. VM storage backend is set from plan at creation and is **immutable**
2. Admin registers storage backends and assigns them to nodes
3. Migration is blocked if target node doesn't support the VM's backend

### 7.5 Plan Resource Limits

| Limit | DB Column | Default |
|-------|-----------|---------|
| Snapshots per VM | `plans.snapshot_limit` | 2 |
| Backups per VM | `plans.backup_limit` | 2 |
| ISO uploads per VM | `plans.iso_upload_limit` | 2 |

Enforced on Customer API only. Admin/Provisioning APIs are exempt.

### 7.6 Template Distribution

- Ceph nodes: shared pool, templates available immediately
- QCOW/LVM nodes: lazy-pull via gRPC `EnsureTemplateCached` RPC
- Template build from ISO: supports local path (`iso_path`) and URL download (`iso_url`)
- Cache status tracked in `template_node_cache` table

---

## 8. VM LIFECYCLE

### 8.1 VM States

**File:** `internal/controller/models/vm.go`

`provisioning`, `running`, `stopped`, `suspended`, `migrating`, `reinstalling`, `error`, `deleted`

### 8.1.1 VM State Transition Rules

State transitions are enforced by:
- `models.ValidateVMTransition(from, to)` in `internal/controller/models/vm.go`
- `VMRepository.TransitionStatus(ctx, vmID, fromStatus, toStatus)` in `internal/controller/repository/vm_repo.go`

Allowed transitions:

| From | To |
|------|----|
| `provisioning` | `running`, `error` |
| `running` | `stopped`, `suspended`, `migrating`, `reinstalling`, `error` |
| `stopped` | `running`, `deleted`, `reinstalling`, `migrating`, `error` |
| `suspended` | `running`, `stopped`, `deleted` |
| `migrating` | `running`, `error` |
| `reinstalling` | `running`, `error` |
| `error` | `stopped`, `deleted` |

`deleted` is terminal (no outbound transitions).

### 8.2 gRPC Service (38 RPCs)

**File:** `proto/virtuestack/node_agent.proto` (972 lines)

| Category | RPCs |
|----------|------|
| Lifecycle (7) | CreateVM, StartVM, StopVM, ForceStopVM, DeleteVM, ReinstallVM, ResizeVM |
| Migration (8) | MigrateVM, AbortMigration, PostMigrateSetup, PrepareMigratedVM, CreateDiskSnapshot, DeleteDiskSnapshot, TransferDisk, ReceiveDisk |
| Backup (2) | CreateLVMBackup, RestoreLVMBackup |
| Console (2) | StreamVNCConsole, StreamSerialConsole (bidirectional streaming) |
| Metrics (3) | GetVMStatus, GetVMMetrics, GetNodeResources |
| Snapshots (4) | CreateSnapshot, DeleteSnapshot, RevertSnapshot, ListSnapshots |
| Guest Agent (5) | GuestExecCommand, GuestSetPassword, GuestFreezeFilesystems, GuestThawFilesystems, GuestGetNetworkInterfaces |
| Bandwidth (3) | GetBandwidthUsage, SetBandwidthLimit, ResetBandwidthCounters |
| Health (2) | Ping, GetNodeHealth |
| Template (2) | BuildTemplateFromISO, EnsureTemplateCached |

### 8.3 Domain XML

**File:** `internal/nodeagent/vm/domain_xml.go`

KVM acceleration, Q35 chipset, virtio devices (disk, network, RNG, balloon), Ceph RBD / QCOW2 / LVM disk config, cloud-init ISO, VNC console (localhost-only, proxied by Controller), serial console via pty, libvirt QoS bandwidth limits, nwfilter anti-spoofing.

---

## 9. ASYNC TASK SYSTEM

### 9.1 Architecture

```
API Handler → NATS JetStream (stream: "TASKS", subject: "tasks.>")
    ↓ durable consumer: "task-worker"
Task Worker Pool (4 workers per Controller)
    ↓ execute (5min ack timeout, 3 max retries)
Node Agent (gRPC) + PostgreSQL updates
```

### 9.2 Task Types (12)

**File:** `internal/controller/models/task.go`

| Task Type | Purpose |
|-----------|---------|
| `vm.create` | VM provisioning |
| `vm.reinstall` | OS reinstallation |
| `vm.resize` | Resource resize |
| `vm.migrate` | Live migration |
| `vm.delete` | VM deletion |
| `backup.create` | Backup creation |
| `backup.restore` | Backup restoration |
| `snapshot.create` | Snapshot creation |
| `snapshot.revert` | Snapshot revert |
| `snapshot.delete` | Snapshot deletion |
| `template.build_from_iso` | Build template from ISO |
| `template.distribute` | Distribute template to nodes |

### 9.3 Task State Machine

```
pending → running → completed
                  → failed (with error_message)
pending → cancelled
```

---

## 10. NETWORKING

**Files:** `internal/nodeagent/network/`

| File | Purpose |
|------|---------|
| `nwfilter.go` + `nwfilter_helpers.go` | Anti-spoofing (MAC, IP, ARP, DHCP, RA) |
| `bandwidth.go` | tc QoS + nftables accounting |
| `dhcp.go` + `dhcp_helpers.go` | dnsmasq management |
| `ipv6.go` | IPv6 prefix allocation |
| `abuse_prevention.go` | nftables abuse prevention |

Bandwidth management: port speed limiting (libvirt `<bandwidth>` in domain XML), nftables counters per VM tap, tc HTB qdisc for overage throttling.

---

## 11. WEB UIs

### 11.1 Admin Portal (`webui/admin/`)

Pages: `login`, `dashboard`, `vms`, `nodes`, `customers`, `plans`, `templates`, `ip-sets`, `storage-backends`, `settings`, `settings/permissions`, `provisioning-keys`, `failover-requests`, `backup-schedules`, `audit-logs`

Dev port: 3000. Production port: 3001.

### 11.2 Customer Portal (`webui/customer/`)

Pages: `login`, `forgot-password`, `reset-password`, `vms`, `vms/[id]`, `settings`

Dev port: 3001. Production port: 3002.

### 11.3 Frontend Stack

TanStack Query for server state (no Zustand). Recharts for charts. noVNC for VNC console. xterm.js for serial console. react-hook-form + Zod for forms. shadcn/ui (Radix) for UI components.

---

## 12. WHMCS INTEGRATION

**Path:** `modules/servers/virtuestack/`

| File | Purpose |
|------|---------|
| `virtuestack.php` | Main module (35 functions) |
| `lib/ApiClient.php` | Controller API client |
| `lib/VirtueStackHelper.php` | URL builder, SSO helpers |
| `lib/shared_functions.php` | Shared utilities |
| `hooks.php` | WHMCS event hooks |
| `webhook.php` | Webhook receiver |
| `templates/overview.tpl` | Client area template |
| `templates/console.tpl` | Console template |

Key functions: `CreateAccount`, `SuspendAccount`, `UnsuspendAccount`, `TerminateAccount`, `ChangePackage`, `ChangePassword`, `ClientArea`, `TestConnection`, `SingleSignOn`, `UsageUpdate`.

SSO flow: `POST /provisioning/sso-tokens` creates one-time opaque token → browser redirected to `GET /customer/auth/sso-exchange?token={opaque}` → Controller sets HttpOnly session cookies → redirect to customer UI.

---

## 13. NOTIFICATION SYSTEM

### Email Templates

Internal (`internal/controller/notifications/templates/`): `bandwidth-exceeded`, `backup-failed`, `base`, `node-offline`, `vm-suspended`, `default`, `vm-deleted`, `vm-created`, `password-reset`.

Static (`templates/email/`): `backup-failed`, `node-offline`, `vm-created`, `vm-deleted`, `vm-suspended`.

---

## 14. ENVIRONMENT VARIABLES

**Primary source:** `internal/shared/config/config.go` (743 lines)

### Controller

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `NATS_URL` | Yes | NATS server URL |
| `NATS_AUTH_TOKEN` | Yes | NATS authentication token |
| `JWT_SECRET` | Yes | HMAC secret for JWT signing |
| `ENCRYPTION_KEY` | Yes | 64 hex characters for AES-256-GCM |
| `LISTEN_ADDR` | No | HTTP listen address (default `:8080`) |
| `APP_ENV` | No | `development` or `production` |
| `LOG_LEVEL` | No | `debug`/`info`/`warn`/`error` |
| `REDIS_URL` | No | Required for production HA rate limiting |
| `PROVISIONING_ALLOWED_IPS` | No | Comma-separated allowed IPs |
| `PDNS_MYSQL_DSN` | No | PowerDNS MySQL connection |
| `PDNS_API_URL` | No | PowerDNS API URL (alternative to DSN) |
| `PDNS_API_KEY` | No | PowerDNS API key |
| `SMTP_HOST` | No | SMTP server hostname |
| `SMTP_PORT` | No | SMTP port (default 587) |
| `SMTP_USERNAME` | No | SMTP auth username |
| `SMTP_PASSWORD` | No | SMTP auth password |
| `SMTP_FROM` | No | Sender email address |
| `SMTP_REQUIRE_TLS` | No | Enforce STARTTLS |
| `TELEGRAM_BOT_TOKEN` | No | Telegram bot token |
| `TELEGRAM_ADMIN_CHAT_IDS` | No | Telegram admin chat IDs |
| `BACKUP_STORAGE_PATH` | No | Backup storage path |
| `BACKUP_FTP_*` | No | FTP backup configuration |

### Node Agent

| Variable | Required | Description |
|----------|----------|-------------|
| `CONTROLLER_GRPC_ADDR` | Yes | Controller gRPC address |
| `NODE_ID` | Yes | Unique node identifier (UUID) |
| `STORAGE_BACKEND` | No | `ceph`, `qcow`, or `lvm` (default `ceph`) |
| `CEPH_POOL` | No | Default Ceph pool |
| `CEPH_USER` | No | Ceph auth user |
| `CEPH_CONF` | No | Ceph config path |
| `STORAGE_PATH` | No | Base path for QCOW2 storage |
| `LVM_VOLUME_GROUP` | No | LVM volume group name |
| `LVM_THIN_POOL` | No | LVM thin pool name |
| `TLS_CERT_FILE` | Yes | mTLS client certificate |
| `TLS_KEY_FILE` | Yes | mTLS client key |
| `TLS_CA_FILE` | Yes | CA certificate |
| `CLOUDINIT_PATH` | No | Cloud-init storage path |
| `ISO_STORAGE_PATH` | No | ISO storage path |
| `LOG_LEVEL` | No | Logging level |
| `SHUTDOWN_TIMEOUT` | No | Graceful shutdown timeout |

---

## 15. BUILD & TEST COMMANDS

### Go Backend

```bash
make build                # Build controller + node-agent (output: bin/)
make build-controller     # Controller only (always works without native libs)
make build-node-agent     # Requires libvirt/Ceph dev headers
make test                 # Controller/shared unit tests only
make test-race            # Same with race detector
make test-integration     # Docker/Testcontainers integration tests
make test-native          # Node Agent tests (requires native libs)
make test-all             # All test suites
make test-coverage        # HTML coverage report
make lint                 # golangci-lint (25 linters, see .golangci.yml)
make vet                  # go vet
make proto                # Regenerate protobuf Go code
make deps                 # go mod download + verify + tidy
make certs                # Generate mTLS certificates
```

Single test: `go test -race -run TestFunctionName ./internal/controller/services/...`

### Frontend (webui/admin and webui/customer)

```bash
npm ci            # Install dependencies (not npm install)
npm run build     # Production build
npm run lint      # ESLint
npm run type-check # tsc --noEmit
npm run dev       # Dev server
```

### E2E Tests (tests/e2e)

```bash
pnpm install      # NOT npm
pnpm test         # Run all Playwright tests
```

### Database Migrations

```bash
make migrate-up                       # Apply all pending
make migrate-down                     # Rollback last
make migrate-create NAME=feature_name # New migration pair
```

### Docker

```bash
make docker-build   # Build images
make docker-up      # Start stack
make docker-down    # Stop stack
```

### Build Gotchas

1. **Node Agent requires native libs:** `sudo apt install -y pkg-config libvirt-dev librados-dev librbd-dev`. Without them, `make build-node-agent`, `make vet`, and `make test-native` fail.
2. **Package manager split:** `webui/` uses npm (`package-lock.json`); `tests/e2e/` uses pnpm (`pnpm-lock.yaml`).
3. **golangci-lint not pre-installed:** CI installs via GitHub Action. Local: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.0.2`
4. **Migration 000031 is a no-op:** Original `CREATE INDEX CONCURRENTLY` incompatible with golang-migrate transactions.

---

## 16. DOCKER SERVICES

**File:** `docker-compose.yml`

| Service | Image | Notes |
|---------|-------|-------|
| postgres | postgres:16-alpine | Internal network |
| nats | nats:2.10-alpine | JetStream enabled, `--auth` token |
| controller | virtuestack/controller | Depends on postgres, nats |
| admin-webui | virtuestack/admin-webui | Depends on controller |
| customer-webui | virtuestack/customer-webui | Depends on controller |
| nginx | nginx:1.25-alpine | Ports 80, 443 |

Network: `virtuestack-internal` (172.20.0.0/24). Node Agent runs on host, not in Docker.

Required `.env` variables: `POSTGRES_PASSWORD`, `NATS_AUTH_TOKEN`, `JWT_SECRET`, `ENCRYPTION_KEY`.

---

## 17. CI PIPELINE

**Files:** `.github/workflows/ci.yml`, `.github/workflows/e2e.yml`

### ci.yml (push/PR to main)

1. Go lint + test (with PostgreSQL 16 + NATS service containers)
   - Includes buf proto breaking-change check against `main`
2. PHP syntax validation
3. Admin frontend: `npm ci` + lint + type-check + build
4. Customer frontend: `npm ci` + lint + type-check + build
5. Docker image builds + Trivy security scan
6. Security: `govulncheck` + `npm audit`

### e2e.yml

Playwright E2E tests against Docker stack.

---

## 18. IMPLEMENTATION PATTERNS

### Handler Pattern

```go
func (h *Handler) CreateResource(c *gin.Context) {
    var req models.CreateResourceRequest
    if err := middleware.BindAndValidate(c, &req); err != nil { ... }
    resource, err := h.service.Create(c.Request.Context(), &req)
    if err != nil {
        middleware.RespondWithError(c, status, code, msg)
        return
    }
    h.logAuditEvent(c, "resource.create", "resource", resource.ID, changes, true)
    c.JSON(http.StatusCreated, models.Response{Data: resource})
}
```

### Service Constructor Pattern

```go
type VMServiceConfig struct {
    VMRepo        *repository.VMRepository
    Logger        *slog.Logger
}
func NewVMService(cfg VMServiceConfig) *VMService { ... }
```

### Repository Pattern

Repositories use pgx/v5 with parameterized queries. Customer-scoped methods set `app.current_customer_id` PostgreSQL session variable for RLS.

### Error Types

**File:** `internal/shared/errors/errors.go`

Sentinel errors: `ErrNotFound`, `ErrUnauthorized`, `ErrValidation`, `ErrConflict`, `ErrLimitExceeded`.
Typed errors: `ValidationError`, `AuthenticationError`, `ForbiddenError`, `NotFoundError`, `ConflictError`, `RateLimitError`, `InternalError`, `OperationError`.

### HTTP Server Config

Read timeout: 10s. Write timeout: 30s. Idle timeout: 120s.

---

## 19. ADDING FEATURES

### New API Endpoint

1. Add handler in `internal/controller/api/{admin|customer|provisioning}/`
2. Register route in `routes.go` with middleware
3. Add service method in `internal/controller/services/`
4. Add repository methods in `internal/controller/repository/`
5. Add/update models in `internal/controller/models/`
6. Add migration if schema changes: `make migrate-create NAME=feature_name`
7. Write table-driven tests with testify

### New Storage Operation

1. Add method to `StorageBackend` interface (`internal/nodeagent/storage/interface.go`)
2. Implement in `rbd.go`, `qcow.go`, and `lvm.go`
3. Add gRPC method to `proto/virtuestack/node_agent.proto`
4. Regenerate: `make proto`
5. Implement gRPC handler in appropriate `grpc_handlers_*.go` file

### New Async Task

1. Define task type constant in `internal/controller/models/task.go`
2. Create handler in `internal/controller/tasks/{task_name}.go`
3. Register handler in `internal/controller/tasks/worker.go`
4. Publish from API handler via `taskPublisher.PublishTask()`

### New Database Migration

```bash
make migrate-create NAME=add_feature_column
```

Follow expand-contract pattern. Never use `CREATE INDEX CONCURRENTLY` inside migrations.

---

## 20. KEY FILE REFERENCES

| Purpose | File(s) |
|---------|---------|
| Config loading | `internal/shared/config/config.go` |
| Server wiring | `internal/controller/server.go` |
| Admin API routes | `internal/controller/api/admin/routes.go` |
| Customer API routes | `internal/controller/api/customer/routes.go` |
| Provisioning API routes | `internal/controller/api/provisioning/routes.go` |
| Auth middleware | `internal/controller/api/middleware/auth.go` |
| Error responses | `internal/controller/api/middleware/recovery.go` |
| Permission model | `internal/controller/models/permission.go` |
| VM model + states | `internal/controller/models/vm.go` |
| Task types | `internal/controller/models/task.go` |
| Custom errors | `internal/shared/errors/errors.go` |
| Storage interface | `internal/nodeagent/storage/interface.go` |
| Storage factory | `internal/nodeagent/storage/factory.go` |
| gRPC proto | `proto/virtuestack/node_agent.proto` |
| Domain XML | `internal/nodeagent/vm/domain_xml.go` |
| Task worker | `internal/controller/tasks/worker.go` |
| Controller metrics | `internal/controller/metrics/prometheus.go` |
| Node Agent metrics | `internal/nodeagent/metrics/prometheus.go` |
| Redis client | `internal/controller/redis/client.go` |
| Audit masking | `internal/controller/audit/masking.go` |
| Models (all) | `internal/controller/models/*.go` (38 files) |
| Repositories (all) | `internal/controller/repository/*.go` (30 files) |
| Services (all) | `internal/controller/services/*.go` (41 files) |
| Tasks (all) | `internal/controller/tasks/*.go` (25 files) |
| Migrations | `migrations/000001–000065` |
| Architecture summaries | `docs/codemaps/*.md` |
| Coding standard | `docs/coding-standard.md` |
| E2E test guide | `tests/e2e/README.md` |
| Linter config | `.golangci.yml` |
