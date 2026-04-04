# VirtueStack AGENTS.md

Machine-readable reference for LLM agents working on the VirtueStack codebase.

**Audience:** LLMs and AI coding agents only.
**Companion documents:** `docs/coding-standard.md` (rules for generated code), `docs/codemaps/` (token-lean architecture summaries ~4K tokens total).
**Boundary:** This file describes what exists. `docs/coding-standard.md` prescribes how to write code.

---

## 1. PROJECT OVERVIEW

VirtueStack is a KVM/QEMU Virtual Machine management platform for VPS hosting providers.

**Components:** Go backend (Controller + Node Agent), TypeScript/React frontends (Next.js admin and customer portals), PostgreSQL 18 database with Row-Level Security, NATS JetStream message queue, Redis for distributed rate limiting.

**Communication:** Controller ↔ Node Agent via gRPC with mTLS. Controller ↔ billing systems via Provisioning REST API (neutral — supports WHMCS, Blesta, or any external billing).

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
      provisioning/                         # WHMCS/Blesta provisioning API handlers
      middleware/                           # Auth, rate limit, audit, CSRF, permissions, metrics (19 files)
      webhooks/                             # Payment webhook handlers (Stripe, PayPal, crypto)
    audit/masking.go                        # Audit log field masking
    billing/                                # Billing provider abstraction
      provider.go                           # BillingProvider interface + Registry
      registry.go                           # Provider registry (WHMCS, Blesta, Native)
      whmcs/                                # WHMCS no-op adapter
      blesta/                               # Blesta no-op adapter
      native/                               # Native billing adapter (credit ledger)
    metrics/prometheus.go                   # Controller Prometheus metrics
    models/                                 # Data models (50+ files)
    notifications/                          # Email + Telegram notifications (+ templates/)
    payments/                               # Payment gateway abstraction
      provider.go                           # PaymentProvider interface
      registry.go                           # Gateway registry
      stripe/                               # Stripe Checkout + webhook verification
      paypal/                               # PayPal Orders API + webhook verification
      crypto/                               # BTCPay Server + NOWPayments
    redis/client.go                         # Redis client for distributed rate limiting
    repository/                             # Database access layer (38+ files)
    services/                               # Business logic (65+ files)
    tasks/                                  # Async task handlers (29 files)
    config.go                               # Re-exports shared config
    dependencies.go                         # Dependency wiring and service initialization
    grpc_client.go                          # gRPC client to Node Agents
    response.go                             # Health/readiness and shared HTTP responses
    schedulers.go                           # Background scheduler startup and collectors
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
migrations/                                 # 80 sequential migrations (000001–000080)
webui/
  admin/                                    # Next.js admin portal (18+ pages)
  customer/                                 # Next.js customer portal (9+ pages)
modules/servers/virtuestack/                # WHMCS billing module (PHP, 8 files)
modules/blesta/virtuestack/                 # Blesta billing module (PHP, 12 files)
  virtuestack.php                           # Main module (server provisioning)
  config.json                               # Module metadata
  language/                                 # Language files
  lib/                                      # API client library
  views/                                    # Client area templates
  webhook.php                               # Webhook receiver
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
| Database | PostgreSQL | 18+ |
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
| Stripe | stripe-go | v82 |
| PDF Generation | go-pdf/fpdf | v0.6 |

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
| Proxy | Nginx 1.28+ |

---

## 4. DATABASE

### 4.1 Overview

- 80 sequential migrations (`migrations/000001–000080`)
- 40+ tables
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
| `tasks` | Async task queue (type, status, payload, result, progress, retry_count) |
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
| `system_webhooks` | System-level webhook configurations (node/failover/storage events) |
| `email_verification_tokens` | Email verification tokens for customer registration |
| `pre_action_webhooks` | Pre-action webhook configurations for approval workflows |
| `notifications` | In-app notification center (title, message, read status, customer_id) |
| `billing_transactions` | Immutable credit/debit ledger (customer_id, type, amount, balance_after) |
| `billing_payments` | Payment gateway records (provider, external_id, status, amount, currency) |
| `billing_invoices` | Invoice headers (customer_id, status, total, currency, due_date) |
| `billing_invoice_line_items` | Invoice line items (invoice_id, description, quantity, unit_price) |
| `billing_checkpoints` | Hourly VM usage billing checkpoints (vm_id, billed_at) |
| `exchange_rates` | Currency exchange rates (currency, rate_to_usd, updated_at) |
| `customer_oauth_links` | OAuth provider links (customer_id, provider, provider_user_id) |

### 4.3 Row-Level Security

RLS is enabled on: `vms`, `notification_preferences`, `notification_events`, `password_resets`, `customer_api_keys`, `ip_addresses`, `backups`, `snapshots`, `backup_schedules`, `sessions`, `customers`, `email_verification_tokens`, `system_webhooks`, `pre_action_webhooks`, `billing_transactions`, `billing_payments`, `billing_invoices`, `billing_invoice_line_items`, `notifications`, `customer_oauth_links`.

All policies use `current_setting('app.current_customer_id')::UUID` for customer isolation.

---

## 5. API ARCHITECTURE

### 5.1 Three-Tier System

| Tier | Base Path | Auth | Rate Limit |
|------|-----------|------|------------|
| Admin | `/api/v1/admin/*` | JWT + 2FA + RBAC | 500/min |
| Customer | `/api/v1/customer/*` | JWT or Customer API Key | 100 read/min, 30 write/min |
| Provisioning | `/api/v1/provisioning/*` | API Key (X-API-Key) | 1000/min |

OpenAPI specs are generated from annotations via `make swagger` (`swag init`) and committed at:
- `docs/swagger.json`
- `docs/swagger.yaml`

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

GET    /system-webhooks
POST   /system-webhooks
PUT    /system-webhooks/:id
DELETE /system-webhooks/:id

GET    /pre-action-webhooks
POST   /pre-action-webhooks
PUT    /pre-action-webhooks/:id
DELETE /pre-action-webhooks/:id

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
POST   /auth/verify-email

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
POST   /2fa/backup-codes/regenerate

GET    /notifications/preferences
PUT    /notifications/preferences

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

GET    /auth/oauth/:provider
POST   /auth/oauth/:provider/callback
DELETE /auth/oauth/:provider
GET    /auth/oauth/links
```

### 5.4 Provisioning API Endpoints

**File:** `internal/controller/api/provisioning/routes.go`

Neutral API — uses `external_service_id` and `external_client_id` (not WHMCS-specific). Works with WHMCS, Blesta, or any external billing system.

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

### 5.7 Webhook Endpoints (No Auth — Signature-Verified)

```
POST   /webhooks/stripe          # Stripe webhook (verified via stripe-go)
POST   /webhooks/paypal          # PayPal webhook (verified via PayPal API)
POST   /webhooks/crypto          # BTCPay/NOWPayments webhook (HMAC signature)
```

These endpoints are mounted outside the API tier middleware. Each handler verifies the webhook signature using the respective provider's SDK before processing.

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
                  → failed (with error_message, retry_count incremented)
pending → cancelled
```

### 9.4 Stuck Task Recovery

A background scanner (`StartStuckTaskScanner`) periodically detects tasks stuck in `running` state beyond a configurable threshold and marks them as `failed`. Uses `TaskRepository.FindStuckTasks()`.

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

## 10.5. BILLING ARCHITECTURE

### 10.5.1 Provider Abstraction

**File:** `internal/controller/billing/provider.go`

```go
type BillingProvider interface {
    Name() string
    ValidateConfig() error
    OnVMCreated(ctx, customer, vm) error
    OnVMDeleted(ctx, customer, vm) error
    OnVMResized(ctx, customer, vm, newPlan) error
    GetUserBillingStatus(ctx, customer) (string, error)
    CreateUser(ctx, email, name) (string, error)
    SuspendForNonPayment(ctx, customer) error
    UnsuspendAfterPayment(ctx, customer) error
    GetBalance(ctx, customer) (decimal, error)
    ProcessTopUp(ctx, customer, amount) error
    GetUsageHistory(ctx, customer) ([]Transaction, error)
}
```

**Registry:** `internal/controller/billing/registry.go` — maps provider names to implementations, selects provider per-customer via `customers.billing_provider` column.

### 10.5.2 Three Billing Modes

| Mode | Provider | Behavior |
|------|----------|----------|
| WHMCS | `whmcs` | External billing. No-op adapter — WHMCS manages invoices/payments. VirtueStack only provisions. |
| Blesta | `blesta` | External billing. No-op adapter — Blesta manages invoices/payments. VirtueStack only provisions. |
| Native | `native` | Built-in credit-based billing. Credit ledger, payment gateways, invoicing all in VirtueStack. |

Customer's billing provider is set via `customers.billing_provider` column (default: configured primary provider).

### 10.5.3 Payment Gateway Abstraction

**File:** `internal/controller/payments/provider.go`

```go
type PaymentProvider interface {
    Name() string
    CreateCheckoutSession(ctx, amount, currency, customerID, returnURL) (*CheckoutResult, error)
    VerifyWebhookSignature(payload, signature) error
    GetPaymentStatus(ctx, externalID) (PaymentStatus, error)
}
```

**Implementations:**

| Provider | Directory | Features |
|----------|-----------|----------|
| Stripe | `payments/stripe/` | Checkout Sessions, webhook signature verification |
| PayPal | `payments/paypal/` | Orders API, webhook verification via PayPal REST |
| BTCPay | `payments/crypto/` | BTCPay Server integration, HMAC webhooks |
| NOWPayments | `payments/crypto/` | NOWPayments API, IPN callbacks |

### 10.5.4 Credit Ledger

**Service:** `internal/controller/services/billing_ledger_service.go`

Immutable append-only transaction log. Every credit/debit creates a new `billing_transactions` row with `balance_after` computed via `SELECT FOR UPDATE` to prevent race conditions.

Transaction types: `credit` (top-up, admin credit), `debit` (hourly usage, manual charge), `refund`.

### 10.5.5 Hourly Billing Scheduler

**Service:** `internal/controller/services/billing_scheduler.go`

- Runs every hour via background goroutine
- Uses PostgreSQL advisory locks (`pg_try_advisory_lock`) for HA — only one Controller instance runs billing
- Iterates running VMs for `native` billing customers
- Calculates hourly cost from plan pricing
- Creates debit transactions in the credit ledger
- Records `billing_checkpoints` to prevent double-billing
- Auto-suspends VMs when balance reaches zero (configurable)

### 10.5.6 Invoice Generation

**Services:** `billing_invoice_service.go`, `billing_invoice_pdf.go`

- Invoices aggregate billing transactions for a period
- PDF rendering via go-pdf/fpdf with company branding
- Invoice states: `draft`, `issued`, `paid`, `void`
- Admin can void invoices; customers can view and download PDFs

### 10.5.7 Webhook Security

All payment webhook endpoints enforce:
- Request body size limit (64KB)
- Provider-specific signature verification before processing
- Idempotency via `external_id` deduplication on `billing_payments`
- No authentication middleware — webhooks are verified by signature only

### 10.5.8 SSE Real-Time Notifications

**Service:** `internal/controller/services/sse_hub.go`

Server-Sent Events hub for pushing real-time notifications to connected customers. The `/customer/notifications/sse` endpoint maintains a persistent HTTP connection per customer. New notifications (billing events, VM status changes) are broadcast to connected clients instantly.

---

## 11. WEB UIs

### 11.1 Admin Portal (`webui/admin/`)

Pages: `login`, `dashboard`, `vms`, `nodes`, `customers`, `plans`, `templates`, `ip-sets`, `storage-backends`, `settings`, `settings/permissions`, `provisioning-keys`, `failover-requests`, `backup-schedules`, `audit-logs`, `billing`, `invoices`, `exchange-rates`

Dev port: 3000. Production port: 3001.

### 11.2 Customer Portal (`webui/customer/`)

Pages: `login`, `forgot-password`, `reset-password`, `vms`, `vms/[id]`, `settings`, `billing`, `invoices`

Includes: notification bell with SSE real-time updates, OAuth login buttons (Google, GitHub).

Dev port: 3001. Production port: 3002.

### 11.3 Frontend Stack

TanStack Query for server state (no Zustand). Recharts for charts. noVNC for VNC console. xterm.js for serial console. react-hook-form + Zod for forms. shadcn/ui (Radix) for UI components.

---

## 12. BILLING MODULE INTEGRATION

### 12.1 WHMCS Module

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

### 12.2 Blesta Module

**Path:** `modules/blesta/virtuestack/`

| File | Purpose |
|------|---------|
| `virtuestack.php` | Main module — server provisioning (create, suspend, unsuspend, terminate) |
| `config.json` | Module metadata (name, version, authors) |
| `language/en_us/virtuestack.php` | English language strings |
| `lib/ApiClient.php` | Controller Provisioning API client |
| `lib/VirtueStackHelper.php` | URL builder, SSO helpers |
| `views/default/tab_client_*.pdt` | Client area tab templates |
| `views/default/tab_admin_*.pdt` | Admin area tab templates |
| `webhook.php` | Webhook receiver for async task completion |

Key methods: `addService` (create VM), `suspendService`, `unsuspendService`, `cancelService` (delete VM), `changeServicePackage` (resize), `getAdminTabs`, `getClientTabs`.

### 12.3 Provisioning API Neutrality

The Provisioning API uses neutral field names to support any billing system:

| Field | Old Name | Purpose |
|-------|----------|---------|
| `external_service_id` | `whmcs_service_id` | Billing system's service/hosting ID |
| `external_client_id` | `whmcs_client_id` | Billing system's customer/client ID |
| `billing_provider` | — | Provider name: `whmcs`, `blesta`, `native` |

Both WHMCS and Blesta modules pass their respective service/client IDs through these neutral fields.

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
| `GUEST_OP_HMAC_SECRET` | Yes | Shared HMAC secret for guest-agent operations; must be at least 32 bytes and match the node agent |
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
| `BILLING_PROVIDER` | No | Default billing provider (`whmcs`/`blesta`/`native`, default: `whmcs`) |
| `STRIPE_SECRET_KEY` | No | Stripe API secret key (required for Stripe payments) |
| `STRIPE_WEBHOOK_SECRET` | No | Stripe webhook signing secret |
| `PAYPAL_CLIENT_ID` | No | PayPal client ID (required for PayPal payments) |
| `PAYPAL_CLIENT_SECRET` | No | PayPal client secret |
| `PAYPAL_WEBHOOK_ID` | No | PayPal webhook ID for verification |
| `BTCPAY_URL` | No | BTCPay Server URL |
| `BTCPAY_API_KEY` | No | BTCPay Server API key |
| `BTCPAY_STORE_ID` | No | BTCPay Server store ID |
| `BTCPAY_WEBHOOK_SECRET` | No | BTCPay webhook HMAC secret |
| `NOWPAYMENTS_API_KEY` | No | NOWPayments API key |
| `NOWPAYMENTS_IPN_SECRET` | No | NOWPayments IPN HMAC secret |
| `OAUTH_GOOGLE_CLIENT_ID` | No | Google OAuth client ID |
| `OAUTH_GOOGLE_CLIENT_SECRET` | No | Google OAuth client secret |
| `OAUTH_GITHUB_CLIENT_ID` | No | GitHub OAuth client ID |
| `OAUTH_GITHUB_CLIENT_SECRET` | No | GitHub OAuth client secret |
| `BLESTA_API_URL` | No | Blesta API base URL |
| `BLESTA_API_KEY` | No | Blesta API key |

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
| `GUEST_OP_HMAC_SECRET` | Yes | Shared HMAC secret for guest-agent operations; must be at least 32 bytes and match the controller |
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
3. **golangci-lint not pre-installed:** CI installs via GitHub Action. Local: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.11.4`
4. **Migration 000031 is a no-op:** Original `CREATE INDEX CONCURRENTLY` incompatible with golang-migrate transactions.

---

## 16. DOCKER SERVICES

**File:** `docker-compose.yml`

| Service | Image | Notes |
|---------|-------|-------|
| postgres | postgres:18-alpine | Internal network |
| nats | nats:2.12-alpine | JetStream enabled, `--auth` token |
| controller | virtuestack/controller | Depends on postgres, nats |
| admin-webui | virtuestack/admin-webui | Depends on controller |
| customer-webui | virtuestack/customer-webui | Depends on controller |
| nginx | nginx:1.28-alpine | Ports 80, 443 |

Network: `virtuestack-internal` (172.20.0.0/24). Node Agent runs on host, not in Docker.

Required `.env` variables: `POSTGRES_PASSWORD`, `NATS_AUTH_TOKEN`, `JWT_SECRET`, `ENCRYPTION_KEY`.

---

## 17. CI PIPELINE

**Files:** `.github/workflows/ci.yml`, `.github/workflows/e2e.yml`

### ci.yml (push/PR to main)

1. Go lint + test (with PostgreSQL 18 + NATS service containers)
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

### New Billing Provider

1. Create adapter directory: `internal/controller/billing/{provider}/`
2. Implement `BillingProvider` interface in `adapter.go`
3. Register in `internal/controller/dependencies.go` (conditionally, based on env vars)
4. Add environment variable validation in `ValidateConfig()`
5. Add provider name to `billing_provider` column allowed values
6. Write adapter tests

### New Payment Gateway

1. Create provider directory: `internal/controller/payments/{gateway}/`
2. Implement `PaymentProvider` interface
3. Register in payment registry (`internal/controller/payments/registry.go`)
4. Add webhook handler in `internal/controller/api/webhooks/`
5. Mount webhook route in `internal/controller/server.go`
6. Add signature verification logic
7. Add environment variables to `internal/shared/config/config.go`
8. Write provider tests

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
| Billing provider interface | `internal/controller/billing/provider.go` |
| Billing registry | `internal/controller/billing/registry.go` |
| Payment provider interface | `internal/controller/payments/provider.go` |
| Payment registry | `internal/controller/payments/registry.go` |
| Credit ledger service | `internal/controller/services/billing_ledger_service.go` |
| Billing scheduler | `internal/controller/services/billing_scheduler.go` |
| Invoice service | `internal/controller/services/billing_invoice_service.go` |
| Invoice PDF | `internal/controller/services/billing_invoice_pdf.go` |
| SSE hub | `internal/controller/services/sse_hub.go` |
| OAuth service | `internal/controller/services/oauth_service.go` |
| Stripe webhook handler | `internal/controller/api/webhooks/stripe.go` |
| PayPal webhook handler | `internal/controller/api/webhooks/paypal.go` |
| Crypto webhook handler | `internal/controller/api/webhooks/crypto_handler.go` |
| Models (all) | `internal/controller/models/*.go` (50+ files) |
| Repositories (all) | `internal/controller/repository/*.go` (38+ files) |
| Services (all) | `internal/controller/services/*.go` (65+ files) |
| Tasks (all) | `internal/controller/tasks/*.go` (29 files) |
| Migrations | `migrations/000001–000080` |
| Architecture summaries | `docs/codemaps/*.md` |
| Coding standard | `docs/coding-standard.md` |
| E2E test guide | `tests/e2e/README.md` |
| Linter config | `.golangci.yml` |
