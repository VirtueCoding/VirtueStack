# VirtueStack — Comprehensive Implementation Plan

> **Generated**: March 2026
> **Source**: Full codebase audit cross-referenced with `docs/USAGE.md`, `docs/VIRTUESTACK_KICKSTART_V2.md`, and `README.md`
> **Scope**: Every TODO, FIXME, stub, placeholder, partial, planned, and unimplemented item across the entire project

---

## Status Legend

| Icon | Meaning |
|------|---------|
| ⚠️ | **Planned** — Designed but not yet implemented |
| 🔧 | **Partial** — Backend exists, frontend not wired |
| 🐛 | **Bug / Code Smell** — Existing code needs fixing |
| 🧪 | **Test Gap** — Missing test coverage |
| 🏗️ | **Infrastructure** — Deployment / config gap |

---

## Table of Contents

1. [Go Backend — TODO / FIXME / Stub Comments](#1-go-backend--todo--fixme--stub-comments)
2. [Node Agent — Unimplemented gRPC Methods (19)](#2-node-agent--unimplemented-grpc-methods-19)
3. [Frontend — Mock Data & Hardcoded Values](#3-frontend--mock-data--hardcoded-values)
4. [Admin Portal — API Wiring (🔧 Partial Features)](#4-admin-portal--api-wiring--partial-features)
5. [Customer Portal — API Wiring (🔧 Partial Features)](#5-customer-portal--api-wiring--partial-features)
6. [Planned Features — Not Yet Implemented (⚠️)](#6-planned-features--not-yet-implemented-)
7. [WHMCS Integration](#7-whmcs-integration)
8. [Test Coverage Gaps](#8-test-coverage-gaps)
9. [Infrastructure & Deployment Gaps](#9-infrastructure--deployment-gaps)
10. [Security Hardening](#10-security-hardening)
11. [Implementation Priority & Phasing](#11-implementation-priority--phasing)

---

## 1. Go Backend — TODO / FIXME / Stub Comments

### 1.1 🐛 Billing Amounts Use Float Instead of Integer Minor Units

- **File**: `internal/controller/models/plan.go:16`
- **Code Comment**: `// TODO: Store billing amounts in integer minor units (cents) to avoid floating-point issues`
- **Current State**: Plan pricing fields use `float64`
- **Risk**: Floating-point precision errors in billing calculations
- **Fix**: Convert all monetary fields to `int64` (cents). Update all consumers: plan creation, update, display, WHMCS sync, provisioning API.
- **Affected Files**:
  - `internal/controller/models/plan.go` — Model definition
  - `internal/controller/repository/plan_repo.go` — DB queries
  - `internal/controller/api/admin/plans.go` — Admin plan CRUD handlers
  - `internal/controller/api/provisioning/handlers.go` — Provisioning resize
  - `webui/admin/` — Plan management UI (display formatting)
  - `webui/customer/` — Plan display in VM creation
  - `modules/servers/virtuestack/` — WHMCS price sync

### 1.2 🐛 Weak Default Password Warning in Config

- **File**: `internal/shared/config/config.go:426-437`
- **Current State**: Prints WARNING to stderr when default/weak passwords detected but allows startup
- **Fix**: In production mode (`ENVIRONMENT=production`), refuse to start with default passwords. Add validation that blocks startup with `devpassword`, `admin123`, `customer123`, or any password under 12 characters.

### 1.3 ⚠️ Placeholder Handler Function (Dead Code)

- **File**: `internal/controller/server.go:370-389`
- **Current State**: `placeholderHandler()` function exists but is never called — all routes have real handlers
- **Fix**: Remove dead code. The function returns 501 in production, 200 in dev, but no route uses it.

### 1.4 ⚠️ Backup Code Usage Not Implemented in Auth

- **File**: `internal/controller/services/auth_service.go:228`
- **Current State**: 2FA backup/recovery codes are generated and stored, but the code path for *using* a backup code to authenticate when TOTP device is lost is not implemented
- **Fix**: Implement `VerifyBackupCode()` method that:
  1. Accepts a backup code instead of TOTP code
  2. Validates against stored hashed backup codes
  3. Invalidates the used code (one-time use)
  4. Returns auth tokens on success
  5. Add corresponding API endpoint (POST `/auth/verify-backup-code`)

### 1.5 ⚠️ Webhook Registration Stub

- **File**: `internal/controller/services/webhook.go:480`
- **Current State**: `WebhookService.Register()` method returns `"not implemented"` error
- **Fix**: Implement webhook event registration that:
  1. Validates the webhook URL (HTTPS, reachable)
  2. Sends a test ping/verification request
  3. Stores the webhook with selected event subscriptions
  4. Returns the created webhook with its signing secret
- **Note**: The webhook *delivery* system (`webhook_deliver.go`) is implemented; only the `Register()` convenience method is a stub.

### 1.6 🐛 Template Service Placeholder Values

- **File**: `internal/controller/services/template_service.go:139`
- **Current State**: When no storage backend is configured, placeholder values are returned instead of an error
- **Fix**: Return a clear error when storage backend is not configured rather than silently returning placeholder data. Log a warning at startup if template storage is not configured.

### 1.7 🐛 Webhook Auto-Disable Without Notification

- **File**: `internal/controller/services/webhook_deliver.go`
- **Current State**: Webhooks are automatically disabled after 50 consecutive delivery failures, but the webhook owner is never notified
- **Fix**: When auto-disabling a webhook:
  1. Send a notification to the customer (email/in-app) that their webhook was disabled
  2. Include the failure reason and last attempted delivery details
  3. Provide instructions to re-enable after fixing the endpoint

---

## 2. Node Agent — Unimplemented gRPC Methods (19)

> **Proto file**: `proto/virtuestack/node_agent.proto` (29 RPC methods defined)
> **Server file**: `internal/nodeagent/server.go` (embeds `UnimplementedNodeAgentServiceServer`)
> **Client file**: `internal/controller/services/node_agent_client.go` (client calls all 29 methods — will fail for unimplemented ones)
> **Generated stubs**: `internal/shared/proto/virtuestack/node_agent_grpc.pb.go` (returns `codes.Unimplemented` for all 19)

**Implemented (10):** Ping, GetNodeHealth, GetVMStatus, GetVMMetrics, GetNodeResources, CreateVM, StartVM, StopVM, ForceStopVM, DeleteVM

### 2.1 ⚠️ VM Lifecycle — ReinstallVM

- **Proto**: `rpc ReinstallVM(ReinstallVMRequest) returns (ReinstallVMResponse)`
- **Implementation**: Stop VM → destroy disk → recreate from template → start VM
- **Dependencies**: Template storage backend, libvirt domain management
- **Complexity**: Medium

### 2.2 ⚠️ VM Lifecycle — ResizeVM

- **Proto**: `rpc ResizeVM(ResizeVMRequest) returns (ResizeVMResponse)`
- **Implementation**: Stop VM → resize CPU/RAM/disk via libvirt → update domain XML → start VM
- **Dependencies**: libvirt domain editing, Ceph RBD resize (for disk)
- **Complexity**: Medium
- **Note**: Disk can only grow, not shrink

### 2.3 ⚠️ Live Migration — MigrateVM

- **Proto**: `rpc MigrateVM(MigrateVMRequest) returns (stream MigrateVMResponse)`
- **Implementation**: Initiate libvirt live migration to target node with progress streaming
- **Dependencies**: Shared storage (Ceph), libvirt migration API, network connectivity between nodes
- **Complexity**: High
- **Streaming**: Server-side stream reporting migration progress percentage

### 2.4 ⚠️ Live Migration — AbortMigration

- **Proto**: `rpc AbortMigration(AbortMigrationRequest) returns (AbortMigrationResponse)`
- **Implementation**: Cancel an in-progress libvirt migration
- **Dependencies**: Active migration tracking, libvirt abort API
- **Complexity**: Medium

### 2.5 ⚠️ Live Migration — PostMigrateSetup

- **Proto**: `rpc PostMigrateSetup(PostMigrateSetupRequest) returns (PostMigrateSetupResponse)`
- **Implementation**: Post-migration cleanup on destination node (network, storage remapping)
- **Dependencies**: Completed migration, network configuration
- **Complexity**: Medium

### 2.6 ⚠️ Console Access — StreamVNCConsole

- **Proto**: `rpc StreamVNCConsole(stream VNCConsoleRequest) returns (stream VNCConsoleResponse)`
- **Implementation**: Bidirectional stream proxying VNC traffic between client and libvirt VNC socket
- **Dependencies**: libvirt VNC graphics, WebSocket proxy for NoVNC frontend
- **Complexity**: High
- **Streaming**: Bidirectional — client sends keyboard/mouse, server sends framebuffer updates

### 2.7 ⚠️ Console Access — StreamSerialConsole

- **Proto**: `rpc StreamSerialConsole(stream SerialConsoleRequest) returns (stream SerialConsoleResponse)`
- **Implementation**: Bidirectional stream proxying serial console I/O
- **Dependencies**: libvirt serial PTY device, terminal emulation
- **Complexity**: High
- **Streaming**: Bidirectional — client sends keystrokes, server sends terminal output

### 2.8 ⚠️ Snapshots — CreateSnapshot

- **Proto**: `rpc CreateSnapshot(CreateSnapshotRequest) returns (CreateSnapshotResponse)`
- **Implementation**: Create Ceph RBD snapshot or libvirt internal snapshot
- **Dependencies**: Ceph RBD or qcow2 snapshot support
- **Complexity**: Medium

### 2.9 ⚠️ Snapshots — DeleteSnapshot

- **Proto**: `rpc DeleteSnapshot(DeleteSnapshotRequest) returns (DeleteSnapshotResponse)`
- **Implementation**: Remove snapshot from storage backend
- **Dependencies**: Snapshot tracking, Ceph RBD or qcow2
- **Complexity**: Low

### 2.10 ⚠️ Snapshots — RevertSnapshot

- **Proto**: `rpc RevertSnapshot(RevertSnapshotRequest) returns (RevertSnapshotResponse)`
- **Implementation**: Stop VM → revert to snapshot state → optionally restart
- **Dependencies**: Snapshot must exist, VM must be stopped
- **Complexity**: Medium

### 2.11 ⚠️ Snapshots — ListSnapshots

- **Proto**: `rpc ListSnapshots(ListSnapshotsRequest) returns (ListSnapshotsResponse)`
- **Implementation**: List all snapshots for a VM from storage backend
- **Dependencies**: Ceph RBD or libvirt snapshot listing
- **Complexity**: Low

### 2.12 ⚠️ QEMU Guest Agent — GuestExecCommand

- **Proto**: `rpc GuestExecCommand(GuestExecRequest) returns (GuestExecResponse)`
- **Implementation**: Execute command inside VM via QEMU Guest Agent
- **Dependencies**: QEMU Guest Agent installed in VM, libvirt QGA channel
- **Complexity**: Medium
- **Security**: Must validate/whitelist allowed commands

### 2.13 ⚠️ QEMU Guest Agent — GuestSetPassword

- **Proto**: `rpc GuestSetPassword(GuestSetPasswordRequest) returns (GuestSetPasswordResponse)`
- **Implementation**: Set root/user password inside VM via QEMU Guest Agent
- **Dependencies**: QEMU Guest Agent, libvirt QGA channel
- **Complexity**: Low

### 2.14 ⚠️ QEMU Guest Agent — GuestFreezeFilesystems

- **Proto**: `rpc GuestFreezeFilesystems(GuestFreezeRequest) returns (GuestFreezeResponse)`
- **Implementation**: Freeze guest filesystems for consistent snapshot (fsfreeze)
- **Dependencies**: QEMU Guest Agent
- **Complexity**: Low
- **Use Case**: Called before CreateSnapshot for data consistency

### 2.15 ⚠️ QEMU Guest Agent — GuestThawFilesystems

- **Proto**: `rpc GuestThawFilesystems(GuestThawRequest) returns (GuestThawResponse)`
- **Implementation**: Thaw frozen guest filesystems after snapshot
- **Dependencies**: QEMU Guest Agent
- **Complexity**: Low

### 2.16 ⚠️ QEMU Guest Agent — GuestGetNetworkInterfaces

- **Proto**: `rpc GuestGetNetworkInterfaces(GuestNetworkRequest) returns (GuestNetworkResponse)`
- **Implementation**: Query guest for its network interface configuration via QGA
- **Dependencies**: QEMU Guest Agent
- **Complexity**: Low

### 2.17 ⚠️ Bandwidth — GetBandwidthUsage

- **Proto**: `rpc GetBandwidthUsage(BandwidthUsageRequest) returns (BandwidthUsageResponse)`
- **Implementation**: Read VM network interface counters from libvirt/iptables/nftables
- **Dependencies**: libvirt domain interface stats or nftables counters
- **Complexity**: Medium

### 2.18 ⚠️ Bandwidth — SetBandwidthLimit

- **Proto**: `rpc SetBandwidthLimit(BandwidthLimitRequest) returns (BandwidthLimitResponse)`
- **Implementation**: Apply traffic shaping rules (tc/nftables) to VM's virtual NIC
- **Dependencies**: Linux tc (traffic control) or nftables rate limiting
- **Complexity**: Medium

### 2.19 ⚠️ Bandwidth — ResetBandwidthCounters

- **Proto**: `rpc ResetBandwidthCounters(ResetBandwidthRequest) returns (ResetBandwidthResponse)`
- **Implementation**: Zero out bandwidth usage counters (for billing cycle reset)
- **Dependencies**: Counter reset mechanism matching GetBandwidthUsage implementation
- **Complexity**: Low

---

## 3. Frontend — Mock Data & Hardcoded Values

All items below must be replaced with real API calls using the existing backend endpoints.

### 3.1 🐛 Mock Boot Sequence in Serial Console

- **File**: `webui/customer/components/serial-console/serial-console.tsx:26-36`
- **Current State**: `mockBootSequence` array with fake boot log lines displayed in the serial console component
- **Fix**: Replace with real serial console data from the `StreamSerialConsole` gRPC method (via WebSocket proxy). Depends on Section 2.7 being implemented first.

### 3.2 🐛 Hardcoded User Profile Data

- **File**: `webui/customer/app/settings/page.tsx:125,137`
- **Current State**: Hardcoded `"John Doe"` and `"john@example.com"` in settings page
- **Fix**: Fetch from `GET /api/v1/customer/profile` endpoint. Wire up the profile update form to `PUT /api/v1/customer/profile`.

### 3.3 🐛 Hardcoded Sidebar Email

- **File**: `webui/customer/components/sidebar.tsx:104`
- **Current State**: Hardcoded `"customer@example.com"` displayed in sidebar user section
- **Fix**: Read from auth context / JWT claims. The JWT already contains user email.

### 3.4 🐛 Mock Dashboard Alert Count

- **File**: `webui/admin/app/dashboard/page.tsx:67`
- **Current State**: `activeAlerts: 0, // Mock for now`
- **Fix**: Fetch real alert count from backend. Requires implementing an alerts/notification query endpoint or aggregating from existing node health data.

### 3.5 🐛 Hardcoded Feature Flags

- **File**: `webui/customer/app/vms/[id]/page.tsx:62-65`
- **Current State**: `FEATURE_FLAGS` object with all values set to `false` — disables console, snapshots, resize, reinstall UI sections
- **Fix**: Either fetch feature flags from backend settings API, or enable individual flags as their corresponding backend features are implemented (Sections 2.6-2.11).

### 3.6 🐛 Placeholder OS and IPv6 Display

- **File**: `webui/customer/app/vms/[id]/page.tsx:701-703,727`
- **Current State**: Shows `"Unknown"` for OS type and `"Not assigned"` for IPv6 when data is missing
- **Fix**: The VM detail API returns OS template info — ensure the frontend reads `vm.template.os_family` or `vm.template.name`. For IPv6, display actual assignment from `vm.ip_addresses` array filtered by address family.

### 3.7 🐛 Simulated ISO Upload Progress

- **File**: `webui/customer/components/file-upload/iso-upload.tsx:57-71`
- **Current State**: Fake upload progress using `setInterval` — simulates 0-100% progress without actually uploading
- **Fix**: Implement real file upload using `XMLHttpRequest` or `fetch` with progress events against a file upload API endpoint. Requires backend ISO upload endpoint implementation.

### 3.8 🐛 Mock IP Sets Data (Admin)

- **File**: `webui/admin/app/ip-sets/page.tsx:60,166`
- **Current State**: `mockIPSets` array with hardcoded IP pool data
- **Fix**: Wire to `GET /api/v1/admin/ip-sets` endpoint. The backend API is fully implemented.

### 3.9 🐛 Mock Resource Chart Data

- **File**: `webui/customer/components/charts/resource-charts.tsx:40,69,241,245`
- **Current State**: Mock data generator functions creating random CPU, RAM, disk, and bandwidth chart data
- **Fix**: Fetch real metrics from `GET /api/v1/customer/vms/:id/metrics` and `GET /api/v1/customer/vms/:id/bandwidth` endpoints. Both backend endpoints are implemented.

---

## 4. Admin Portal — API Wiring (🔧 Partial Features)

> **Status per USAGE.md**: All admin features below have "Backend exists but frontend integration is pending." The admin UI is built with Next.js 15 / shadcn/ui but uses static/mock data instead of calling the live API.

### 4.1 🔧 Node Management

- **USAGE.md**: `### Node Management 🔧`
- **Backend**: `GET/POST/PUT/DELETE /api/v1/admin/nodes`, `POST /nodes/:id/drain`, `POST /nodes/:id/failover` — all implemented
- **Frontend dir**: `webui/admin/app/nodes/`
- **Work needed**:
  - [ ] Wire node list page to `GET /api/v1/admin/nodes`
  - [ ] Wire node registration form to `POST /api/v1/admin/nodes`
  - [ ] Wire node edit to `PUT /api/v1/admin/nodes/:id`
  - [ ] Wire node delete to `DELETE /api/v1/admin/nodes/:id`
  - [ ] Wire drain action to `POST /api/v1/admin/nodes/:id/drain`
  - [ ] Wire failover action to `POST /api/v1/admin/nodes/:id/failover`
  - [ ] Add real-time node health status from node heartbeat data
  - [ ] Display actual VM count per node from backend

### 4.2 🔧 VM Management (Admin)

- **USAGE.md**: `### VM Management (Admin) 🔧`
- **Backend**: Full CRUD + migrate endpoint — all implemented
- **Frontend dir**: `webui/admin/app/vms/`
- **Work needed**:
  - [ ] Wire VM list to `GET /api/v1/admin/vms` with pagination/filtering
  - [ ] Wire VM creation form to `POST /api/v1/admin/vms`
  - [ ] Wire VM detail view to `GET /api/v1/admin/vms/:id`
  - [ ] Wire VM update to `PUT /api/v1/admin/vms/:id`
  - [ ] Wire VM delete to `DELETE /api/v1/admin/vms/:id`
  - [ ] Wire migration trigger to `POST /api/v1/admin/vms/:id/migrate`
  - [ ] Show real VM status (running/stopped/migrating) from backend
  - [ ] Display real resource usage from node agent metrics

### 4.3 🔧 Plan Management

- **USAGE.md**: `### Plan Management 🔧`
- **Backend**: Full CRUD — all implemented
- **Frontend dir**: `webui/admin/app/plans/`
- **Work needed**:
  - [ ] Wire plan list to `GET /api/v1/admin/plans`
  - [ ] Wire plan creation form to `POST /api/v1/admin/plans`
  - [ ] Wire plan edit to `PUT /api/v1/admin/plans/:id`
  - [ ] Wire plan delete to `DELETE /api/v1/admin/plans/:id`
  - [ ] Display plan usage count (how many VMs use each plan)

### 4.4 🔧 Template Management

- **USAGE.md**: `### Template Management 🔧`
- **Backend**: Full CRUD + import — all implemented
- **Frontend dir**: `webui/admin/app/templates/`
- **Work needed**:
  - [ ] Wire template list to `GET /api/v1/admin/templates`
  - [ ] Wire template creation form to `POST /api/v1/admin/templates`
  - [ ] Wire template edit to `PUT /api/v1/admin/templates/:id`
  - [ ] Wire template delete to `DELETE /api/v1/admin/templates/:id`
  - [ ] Wire template import to `POST /api/v1/admin/templates/:id/import`
  - [ ] Show template size and usage statistics

### 4.5 🔧 IP Management

- **USAGE.md**: `### IP Management 🔧`
- **Backend**: Full CRUD + available IPs — all implemented
- **Frontend dir**: `webui/admin/app/ip-sets/`
- **Work needed**:
  - [ ] Replace `mockIPSets` with `GET /api/v1/admin/ip-sets` (see Section 3.8)
  - [ ] Wire IP set creation to `POST /api/v1/admin/ip-sets`
  - [ ] Wire IP set detail view to `GET /api/v1/admin/ip-sets/:id`
  - [ ] Wire IP set edit to `PUT /api/v1/admin/ip-sets/:id`
  - [ ] Wire IP set delete to `DELETE /api/v1/admin/ip-sets/:id`
  - [ ] Wire available IP listing to `GET /api/v1/admin/ip-sets/:id/available`

### 4.6 🔧 Customer Management

- **USAGE.md**: `### Customer Management 🔧`
- **Backend**: Full CRUD + audit logs — all implemented
- **Frontend dir**: `webui/admin/app/customers/`
- **Work needed**:
  - [ ] Wire customer list to `GET /api/v1/admin/customers`
  - [ ] Wire customer detail to `GET /api/v1/admin/customers/:id`
  - [ ] Wire customer update to `PUT /api/v1/admin/customers/:id`
  - [ ] Wire customer delete to `DELETE /api/v1/admin/customers/:id`
  - [ ] Wire customer audit log to `GET /api/v1/admin/customers/:id/audit-logs`
  - [ ] Show customer's active VM count and resource usage

### 4.7 🔧 Audit Logs

- **USAGE.md**: `### Audit Logs 🔧` — "Backend audit logging is working. Admin UI has the interface but is not yet wired to the live API."
- **Backend**: `GET /api/v1/admin/audit-logs` — implemented with filtering
- **Frontend dir**: `webui/admin/app/audit-logs/`
- **Work needed**:
  - [ ] Wire audit log list to `GET /api/v1/admin/audit-logs`
  - [ ] Implement date range filter
  - [ ] Implement action type filter
  - [ ] Implement resource type filter
  - [ ] Implement actor filter
  - [ ] Display JSON diff of changes per log entry

---

## 5. Customer Portal — API Wiring (🔧 Partial Features)

> **Status per USAGE.md**: Customer authentication is fully working (login → JWT). Protected API endpoints are verified. The UI is built but not yet wired to the live backend API.

### 5.1 🔧 VM Management (Customer)

- **USAGE.md**: `### VM Management (Customer) 🔧`
- **Backend**: Full CRUD + power actions + metrics — all implemented
- **Frontend dir**: `webui/customer/app/vms/`
- **Work needed**:
  - [ ] Wire VM list to `GET /api/v1/customer/vms`
  - [ ] Wire VM creation to `POST /api/v1/customer/vms`
  - [ ] Wire VM detail view to `GET /api/v1/customer/vms/:id`
  - [ ] Wire VM delete to `DELETE /api/v1/customer/vms/:id`
  - [ ] Wire power actions (start/stop/restart/force-stop) to respective endpoints
  - [ ] Wire real-time metrics to `GET /api/v1/customer/vms/:id/metrics` (replace mock charts — Section 3.9)
  - [ ] Wire bandwidth data to `GET /api/v1/customer/vms/:id/bandwidth`
  - [ ] Wire network history to `GET /api/v1/customer/vms/:id/network`
  - [ ] Replace hardcoded feature flags with real feature availability (Section 3.5)
  - [ ] Replace "Unknown" OS / "Not assigned" IPv6 with real data (Section 3.6)

### 5.2 🔧 Webhook Configuration

- **USAGE.md**: `### Webhook Configuration 🔧` — "Backend webhook tables and delivery system exist. Customer UI has the interface but is not yet wired to the live API."
- **Backend**: Full CRUD + delivery history — all implemented
- **Frontend dir**: `webui/customer/app/webhooks/` (or settings section)
- **Work needed**:
  - [ ] Wire webhook list to `GET /api/v1/customer/webhooks`
  - [ ] Wire webhook creation to `POST /api/v1/customer/webhooks`
  - [ ] Wire webhook detail to `GET /api/v1/customer/webhooks/:id`
  - [ ] Wire webhook update to `PUT /api/v1/customer/webhooks/:id`
  - [ ] Wire webhook delete to `DELETE /api/v1/customer/webhooks/:id`
  - [ ] Wire delivery history to `GET /api/v1/customer/webhooks/:id/deliveries`
  - [ ] Display webhook secret securely (show once on creation)

### 5.3 🔧 Settings / Profile Page

- **Backend**: Profile endpoints exist
- **Frontend dir**: `webui/customer/app/settings/`
- **Work needed**:
  - [ ] Replace hardcoded "John Doe" / "john@example.com" with API data (Section 3.2)
  - [ ] Wire profile update form to backend
  - [ ] Wire password change to backend
  - [ ] Wire 2FA setup/disable to backend

---

## 6. Planned Features — Not Yet Implemented (⚠️)

These features are designed in `docs/VIRTUESTACK_KICKSTART_V2.md` and documented in `docs/USAGE.md` but have no working end-to-end implementation.

### 6.1 ⚠️ Console Access (NoVNC + Serial)

- **USAGE.md**: `### Console Access ⚠️` — "Not yet implemented. NoVNC/serial console integration is planned but not wired up."
- **Architecture** (from KICKSTART_V2): Controller generates time-limited console tokens → NoVNC/xterm.js frontend connects via WebSocket → Controller proxies to node agent gRPC stream
- **Backend needed**:
  - [ ] Implement `StreamVNCConsole` gRPC method on node agent (Section 2.6)
  - [ ] Implement `StreamSerialConsole` gRPC method on node agent (Section 2.7)
  - [ ] Build WebSocket proxy in controller that bridges browser WebSocket ↔ gRPC stream
  - [ ] Console token generation exists (`POST /vms/:id/console-token`) — verify it produces usable tokens
- **Frontend needed**:
  - [ ] Integrate NoVNC client library for graphical console
  - [ ] Integrate xterm.js for serial console (replace mock boot sequence — Section 3.1)
  - [ ] Connect to WebSocket proxy using console tokens
  - [ ] Add fullscreen mode, clipboard support, special key sending
  - [ ] Enable console feature flag (Section 3.5)

### 6.2 ⚠️ Backup Management (Admin + Customer)

- **USAGE.md**: `### Backup Management (Admin) ⚠️` — "Backend tables and API stubs exist. Full backup workflow (create, restore, delete) is not yet implemented end-to-end."
- **USAGE.md**: `### Backup Management (Customer) ⚠️` — "Backend tables exist. Customer-facing backup workflow is not yet implemented end-to-end."
- **Backend needed**:
  - [ ] Implement actual backup creation logic (Ceph RBD export or qcow2 full/incremental backup)
  - [ ] Implement backup storage to configured backend (FTP/S3 — see `.env.example` BACKUP_* variables)
  - [ ] Implement backup restore logic (stop VM → restore disk → start VM)
  - [ ] Implement backup scheduling (cron-based or NATS-triggered)
  - [ ] Implement backup retention policy enforcement
  - [ ] Wire backup size calculation to actual storage
- **Admin Frontend** (`webui/admin/app/backups/`):
  - [ ] Wire backup list to `GET /api/v1/admin/backups`
  - [ ] Wire backup restore to `POST /api/v1/admin/backups/:id/restore`
  - [ ] Display backup status (in-progress, completed, failed)
- **Customer Frontend** (`webui/customer/app/backups/`):
  - [ ] Wire backup list to `GET /api/v1/customer/backups`
  - [ ] Wire backup creation to `POST /api/v1/customer/backups`
  - [ ] Wire backup detail to `GET /api/v1/customer/backups/:id`
  - [ ] Wire backup delete to `DELETE /api/v1/customer/backups/:id`
  - [ ] Wire backup restore to `POST /api/v1/customer/backups/:id/restore`

### 6.3 ⚠️ Snapshot Management

- **USAGE.md**: `### Snapshot Management ⚠️` — "Backend tables exist. Snapshot workflow is not yet implemented end-to-end."
- **Backend needed**:
  - [ ] Implement `CreateSnapshot` gRPC method (Section 2.8)
  - [ ] Implement `DeleteSnapshot` gRPC method (Section 2.9)
  - [ ] Implement `RevertSnapshot` gRPC method (Section 2.10)
  - [ ] Implement `ListSnapshots` gRPC method (Section 2.11)
  - [ ] Implement `GuestFreezeFilesystems` for consistent snapshots (Section 2.14)
  - [ ] Implement `GuestThawFilesystems` post-snapshot (Section 2.15)
  - [ ] Controller service layer to orchestrate freeze → snapshot → thaw
- **Customer Frontend** (`webui/customer/app/snapshots/`):
  - [ ] Wire snapshot list to `GET /api/v1/customer/snapshots`
  - [ ] Wire snapshot creation to `POST /api/v1/customer/snapshots`
  - [ ] Wire snapshot delete to `DELETE /api/v1/customer/snapshots/:id`
  - [ ] Wire snapshot restore to `POST /api/v1/customer/snapshots/:id/restore`
  - [ ] Enable snapshot feature flag (Section 3.5)

### 6.4 ⚠️ Notification Preferences & Delivery

- **USAGE.md**: `### Notification Preferences ⚠️` — "Backend tables exist. Notification delivery (email/Telegram) is not yet implemented."
- **Backend needed**:
  - [ ] Implement SMTP email sender service (config vars exist in `.env.example`: `SMTP_HOST`, `SMTP_PORT`, etc.)
  - [ ] Implement Telegram bot notification sender (config var: `TELEGRAM_BOT_TOKEN`)
  - [ ] Implement notification dispatch logic — on events (vm.created, backup.completed, etc.) check user preferences and send via configured channels
  - [ ] Implement notification templates (email HTML, Telegram markdown)
  - [ ] Implement bandwidth alert threshold notifications
  - [ ] Implement invoice reminder notifications (for WHMCS integration)
- **Customer Frontend** (`webui/customer/app/settings/` or `/notifications/`):
  - [ ] Wire notification preferences to `GET /api/v1/customer/notifications/preferences`
  - [ ] Wire preference update to `PUT /api/v1/customer/notifications/preferences`
  - [ ] Wire notification event list to `GET /api/v1/customer/notifications/events`
  - [ ] Wire event type listing to `GET /api/v1/customer/notifications/events/types`
  - [ ] Add email configuration UI (verify email)
  - [ ] Add Telegram bot connection UI (link account)

### 6.5 ⚠️ API Key Management

- **USAGE.md**: `### API Keys ⚠️` — "Backend API key table exists. Customer-facing API key management UI is not yet wired to the live API."
- **Backend**: API key CRUD endpoints exist and are implemented
- **Customer Frontend** (`webui/customer/app/api-keys/`):
  - [ ] Wire API key list to `GET /api/v1/customer/api-keys`
  - [ ] Wire API key creation to `POST /api/v1/customer/api-keys`
  - [ ] Wire API key revocation to `DELETE /api/v1/customer/api-keys/:id`
  - [ ] Display key value only once on creation (security requirement)
  - [ ] Show key permissions, creation date, last used date
  - [ ] Add expiration date selector

### 6.6 ⚠️ Password Reset Workflow

- **README**: "Password reset workflow — Table exists, workflow TODO"
- **Backend needed**:
  - [ ] Implement password reset request endpoint (`POST /auth/forgot-password`)
  - [ ] Generate time-limited reset token, store in DB
  - [ ] Send reset email with token link (depends on Section 6.4 email service)
  - [ ] Implement reset confirmation endpoint (`POST /auth/reset-password`) that validates token and sets new password
- **Frontend needed**:
  - [ ] Add "Forgot Password?" link on login pages
  - [ ] Build password reset request form (email input)
  - [ ] Build password reset confirmation form (new password + token from URL)

### 6.7 ⚠️ VM Live Migration (End-to-End)

- **README**: "VM live migration — API stubbed, needs implementation"
- **Backend needed**:
  - [ ] Implement `MigrateVM` gRPC method (Section 2.3)
  - [ ] Implement `AbortMigration` gRPC method (Section 2.4)
  - [ ] Implement `PostMigrateSetup` gRPC method (Section 2.5)
  - [ ] Controller orchestration: select target node → pre-checks → initiate migration → monitor progress → post-setup → update DB records
  - [ ] Implement migration progress tracking via streaming gRPC
- **Admin Frontend**:
  - [ ] Wire migration trigger button to `POST /api/v1/admin/vms/:id/migrate`
  - [ ] Show migration progress indicator
  - [ ] Show migration history per VM

### 6.8 ⚠️ Automatic Node Failover

- **README**: "Automatic node failover — Detection works, auto-recovery TODO"
- **Backend needed**:
  - [ ] Node health detection exists — implement the auto-recovery logic
  - [ ] When node goes offline: identify all VMs on that node
  - [ ] For each VM: select new target node → fence old node → start VM on new node
  - [ ] Implement fencing mechanism (IPMI/iLO power off, or network fence)
  - [ ] Update DB records: node status, VM assignments
  - [ ] Send notifications to affected customers
  - [ ] Admin alert for failed nodes

### 6.9 ⚠️ VM Resize (End-to-End)

- **Backend needed**:
  - [ ] Implement `ResizeVM` gRPC method (Section 2.2)
  - [ ] Controller orchestration: validate new plan → stop VM → resize → start VM → update DB
  - [ ] Handle disk-only-grows constraint
- **Frontend needed**:
  - [ ] Enable resize feature flag (Section 3.5)
  - [ ] Build resize UI (plan selection, confirmation)
  - [ ] Show resize progress

### 6.10 ⚠️ VM Reinstall (End-to-End)

- **Backend needed**:
  - [ ] Implement `ReinstallVM` gRPC method (Section 2.1)
  - [ ] Controller orchestration: validate template → stop VM → destroy disk → recreate from template → set password → start VM
- **Frontend needed**:
  - [ ] Enable reinstall feature flag (Section 3.5)
  - [ ] Build reinstall UI (template selection, password, confirmation warning)

### 6.11 ⚠️ Reverse DNS (rDNS) Management

- **`.env.example`**: Contains `POWERDNS_API_URL` and `POWERDNS_API_KEY` variables (commented out)
- **KICKSTART_V2**: Describes rDNS management for IP addresses
- **Backend needed**:
  - [ ] Implement PowerDNS API client service
  - [ ] Implement rDNS record creation/update/delete for VM IP addresses
  - [ ] Auto-create rDNS on VM provisioning
  - [ ] Allow customer to update rDNS for their IPs
- **Frontend needed**:
  - [ ] Add rDNS editing UI in VM detail / network section
  - [ ] Validate rDNS hostname format

### 6.12 ⚠️ ISO Upload & Custom ISO Boot

- **Frontend**: `webui/customer/components/file-upload/iso-upload.tsx` exists with simulated upload (Section 3.7)
- **Backend needed**:
  - [ ] Implement ISO upload endpoint (multipart file upload with progress)
  - [ ] Store uploaded ISO in configured storage backend
  - [ ] Implement ISO attachment to VM (modify libvirt domain XML to add CDROM)
  - [ ] Implement ISO detachment
  - [ ] Implement ISO listing and deletion
- **Frontend needed**:
  - [ ] Replace simulated upload with real upload (Section 3.7)
  - [ ] Add ISO management page (list, upload, delete)
  - [ ] Add "Boot from ISO" option in VM creation/settings

---

## 7. WHMCS Integration

> **USAGE.md**: `## WHMCS Integration 🔧` — "WHMCS PHP module files exist and pass syntax checks. The provisioning API backend is working and authenticated. Full WHMCS ↔ VirtueStack integration has not been tested end-to-end with a live WHMCS instance."

### 7.1 🔧 TestConnection Placeholder

- **File**: `modules/servers/virtuestack/virtuestack.php`
- **Current State**: `TestConnection` function is a placeholder — doesn't perform a real API health check
- **Fix**: Implement actual API call to controller health endpoint (`GET /health`) to verify connectivity, authentication, and API version compatibility.

### 7.2 🔧 Webhook Handler Incomplete Event Coverage

- **File**: `modules/servers/virtuestack/hooks.php`
- **Current State**: `handleProvisioningWebhook()` doesn't handle all event types
- **Fix**: Add handlers for all webhook event types: `vm.created`, `vm.deleted`, `vm.started`, `vm.stopped`, `vm.reinstalled`, `vm.migrated`, `backup.completed`, `backup.failed`. Update WHMCS service status accordingly.

### 7.3 🔧 Webhook Payload Error Handling

- **File**: `modules/servers/virtuestack/webhook.php`
- **Current State**: Lacks comprehensive error handling for malformed payloads
- **Fix**: Add JSON validation, HMAC signature verification, idempotency key deduplication, and proper error responses for malformed requests.

### 7.4 🔧 Helper Functions May Fail Without Active Services

- **File**: `modules/servers/virtuestack/hooks.php`
- **Current State**: `getVirtueStackTemplates()` and `getVirtueStackLocations()` may fail without active services configured
- **Fix**: Add graceful fallbacks — return empty arrays when no services are configured. Add caching to avoid repeated API calls.

### 7.5 🔧 Webhook Go Backend — Register Stub

- **File**: `internal/controller/services/webhook.go:480`
- **Cross-reference**: Same as Section 1.5 — the `Register()` method is a stub
- **Fix**: Same as Section 1.5.

### 7.6 🔧 Webhook Auto-Disable Without Admin Notification

- **File**: `internal/controller/services/webhook_deliver.go`
- **Cross-reference**: Same as Section 1.7
- **Fix**: Same as Section 1.7. Additionally, for WHMCS webhooks, notify the WHMCS admin panel.

### 7.7 🔧 End-to-End WHMCS Testing

- **Current State**: Never tested with a live WHMCS instance
- **Work needed**:
  - [ ] Set up WHMCS test environment
  - [ ] Test full provisioning lifecycle: order → create → suspend → unsuspend → terminate
  - [ ] Test upgrade/downgrade flow
  - [ ] Test webhook delivery and WHMCS status sync
  - [ ] Test template and location synchronization
  - [ ] Test error scenarios (API down, network issues, invalid credentials)
  - [ ] Document WHMCS configuration steps with screenshots

---

## 8. Test Coverage Gaps

> **Current test infrastructure**: Integration tests (Go), unit tests (Go, limited), E2E tests (Playwright). All existing tests pass. The gaps are in *untested areas*, not broken tests.

### 8.1 🧪 Missing Unit Tests — API Handlers

- **Files**: `internal/controller/api/admin/*.go`, `internal/controller/api/customer/*.go`, `internal/controller/api/provisioning/*.go`
- **Current State**: Zero unit tests for any API handler
- **Work needed**:
  - [ ] Unit tests for all admin handlers using `httptest` + mock services
  - [ ] Unit tests for all customer handlers
  - [ ] Unit tests for all provisioning handlers
  - [ ] Test request validation, error responses, auth middleware

### 8.2 🧪 Missing Unit Tests — Core Services

- **Files**: `internal/controller/services/*.go`
- **Current State**: Only `rbac_service_test.go` exists. No tests for VMService, BackupService, WebhookService, BandwidthService, TemplateService, AuthService, NodeAgentClient.
- **Work needed**:
  - [ ] Unit tests for VMService (CRUD, power actions, state transitions)
  - [ ] Unit tests for BackupService (create, restore, delete, retention)
  - [ ] Unit tests for WebhookService (register, deliver, retry, auto-disable)
  - [ ] Unit tests for BandwidthService (tracking, alerts, reset)
  - [ ] Unit tests for TemplateService (CRUD, import, storage)
  - [ ] Unit tests for AuthService (login, 2FA, backup codes, token refresh)
  - [ ] Unit tests for NodeAgentClient (connection, retry, error handling)

### 8.3 🧪 Missing Unit Tests — Shared Utilities

- **Files**: `internal/shared/crypto/`, `internal/shared/logging/`, `internal/shared/errors/`, `internal/shared/config/`
- **Work needed**:
  - [ ] Crypto utility tests (encryption, hashing, token generation)
  - [ ] Config parsing tests (env vars, defaults, validation)
  - [ ] Error formatting tests
  - [ ] Logging tests

### 8.4 🧪 Missing Unit Tests — Node Agent

- **Files**: `internal/nodeagent/*.go`
- **Work needed**:
  - [ ] Unit tests for implemented gRPC methods (Ping, GetNodeHealth, CreateVM, etc.)
  - [ ] Test libvirt interaction mocking
  - [ ] Test error handling for libvirt failures

### 8.5 🧪 Missing Unit Tests — Task Workers

- **Files**: `internal/controller/tasks/*.go`
- **Current State**: Only password hashing function tests exist
- **Work needed**:
  - [ ] Unit tests for all task handler types
  - [ ] Test NATS message processing
  - [ ] Test task retry logic
  - [ ] Test task failure handling

### 8.6 🧪 Missing Integration Tests

- **Current**: Tests exist for VM lifecycle, auth, backup, webhook
- **Missing**:
  - [ ] Node registration and health monitoring integration test
  - [ ] Plan CRUD integration test
  - [ ] Template CRUD integration test
  - [ ] IP set management integration test
  - [ ] Customer management integration test
  - [ ] Audit log integration test
  - [ ] Bandwidth tracking integration test
  - [ ] API key authentication integration test

### 8.7 🧪 Missing E2E Tests for Unimplemented Features

- **Current**: E2E tests exist for auth, customer VM management, admin VM management, customer backups
- **Will be needed** (after feature implementation):
  - [ ] E2E test for console access (VNC + serial)
  - [ ] E2E test for snapshot management
  - [ ] E2E test for API key management
  - [ ] E2E test for webhook configuration
  - [ ] E2E test for notification preferences
  - [ ] E2E test for VM resize and reinstall

### 8.8 🧪 Integration Test — Permission Placeholder

- **File**: `tests/integration/auth_test.go:570`
- **Current State**: Permission test has placeholder assertion
- **Fix**: Implement full RBAC permission testing — verify each role can only access authorized endpoints

---

## 9. Infrastructure & Deployment Gaps

### 9.1 🏗️ Missing SSL/TLS Directory

- **File**: `docker-compose.yml`
- **Current State**: References `${SSL_CERT_PATH:-./ssl/cert.pem}` and `${SSL_KEY_PATH:-./ssl/key.pem}` but no `ssl/` directory exists
- **Fix**:
  - [ ] Create `ssl/` directory with `.gitkeep`
  - [ ] Add `ssl/*.pem` to `.gitignore`
  - [ ] Document certificate generation (Let's Encrypt or self-signed for dev)
  - [ ] Add SSL setup script to `scripts/`

### 9.2 🏗️ Default Passwords in Docker Compose Override

- **File**: `docker-compose.override.yml`
- **Current State**: `POSTGRES_PASSWORD:-devpassword` — development default leaks into any environment using the override
- **Fix**:
  - [ ] Remove default passwords from override file
  - [ ] Require passwords via `.env` file only
  - [ ] Add startup validation that rejects default passwords in production mode (see Section 1.2)

### 9.3 🏗️ Unimplemented Feature Config in .env.example

- **File**: `.env.example`
- **Current State**: Contains commented-out configuration for unimplemented features:
  - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD` — Email notifications (Section 6.4)
  - `TELEGRAM_BOT_TOKEN` — Telegram notifications (Section 6.4)
  - `POWERDNS_API_URL`, `POWERDNS_API_KEY` — Reverse DNS (Section 6.11)
  - `BACKUP_STORAGE_TYPE`, `BACKUP_FTP_*`, `BACKUP_S3_*` — Backup storage (Section 6.2)
- **Fix**: Uncomment and document each variable as its corresponding feature is implemented. Add validation in config loading.

### 9.4 🏗️ Missing CI/CD Pipeline

- **Current State**: No `.github/workflows/`, `.gitlab-ci.yml`, or any CI/CD configuration
- **Work needed**:
  - [ ] Create CI pipeline: lint, build, test (Go + frontend)
  - [ ] Create CD pipeline: Docker image build and push
  - [ ] Add database migration check step
  - [ ] Add security scanning (gosec, npm audit)

### 9.5 🏗️ Missing Kubernetes / Helm Configuration

- **Current State**: Docker Compose only — no Kubernetes manifests or Helm charts
- **Work needed** (if K8s deployment is desired):
  - [ ] Helm chart for controller
  - [ ] Helm chart for admin-webui
  - [ ] Helm chart for customer-webui
  - [ ] StatefulSet for PostgreSQL
  - [ ] StatefulSet for NATS
  - [ ] Ingress configuration
  - [ ] ConfigMap/Secret management

### 9.6 🏗️ Missing CODEBASE_AUDIT_REPORT.md

- **File**: `docs/CODEBASE_AUDIT_REPORT.md`
- **Current State**: Referenced in `README.md` 7 times (security checklist, project status, deployment notes) but the file does not exist
- **Fix**: Either create the audit report document or update all README references to point to this `docs/todo.md` instead

### 9.7 🏗️ Provisioning API IP Allowlisting

- **README Security Checklist**: "Secure provisioning API endpoints with IP allowlisting" — marked as incomplete
- **Fix**:
  - [ ] Implement IP allowlist middleware for `/api/v1/provisioning/*` routes
  - [ ] Make allowlist configurable via `.env` (e.g., `PROVISIONING_ALLOWED_IPS=10.0.0.1,10.0.0.2`)
  - [ ] Default to deny-all in production if not configured

---

## 10. Security Hardening

### 10.1 🐛 Replace Self-Signed Certificates

- **README Security Checklist**: "Replace self-signed certificates with proper CA-signed certs" — marked as incomplete
- **Fix**: Document and script Let's Encrypt certificate provisioning for production deployment

### 10.2 🐛 Remove Default Credentials from Documentation

- **README Quick Start**: Documents `admin@virtuestack.local / admin123` and `customer@virtuestack.local / customer123`
- **Fix**: These are fine for documentation, but ensure the seed data / migration that creates these accounts is ONLY run in development mode. Production should require explicit admin account creation.

### 10.3 🐛 QEMU Guest Agent Command Whitelisting

- **Cross-reference**: Section 2.12 — `GuestExecCommand` must validate/whitelist allowed commands
- **Fix**: Implement a strict command whitelist. Never allow arbitrary command execution via guest agent.

### 10.4 🐛 Rate Limiting Verification

- **README**: Rate limiting is listed as a platform setting
- **Fix**: Verify rate limiting middleware is actually applied to all public endpoints (auth, API). Test under load.

---

## 11. Implementation Priority & Phasing

### Phase 1: Critical Fixes & Low-Hanging Fruit (1-2 weeks)

These items require minimal effort and fix real bugs or dead code. **Only items with zero backend dependencies.**

| # | Item | Section | Effort | Notes |
|---|------|---------|--------|-------|
| 1 | Fix float pricing → integer cents | 1.1 | Medium | Pure backend model change |
| 2 | Block weak passwords in production | 1.2 | Low | Config validation only |
| 3 | Remove dead `placeholderHandler` | 1.3 | Trivial | Dead code removal |
| 4 | Fix template service placeholder values | 1.6 | Low | Return error instead of fake data |
| 5 | Replace hardcoded sidebar email | 3.3 | Trivial | Read from JWT claims (no API call) |
| 6 | Fix OS/IPv6 display placeholders | 3.6 | Trivial | Data already in VM API response |
| 7 | Create missing CODEBASE_AUDIT_REPORT.md or update refs | 9.6 | Low | Documentation only |
| 8 | Create ssl/ directory + docs | 9.1 | Trivial | Infrastructure only |
| 9 | Remove default passwords from override | 9.2 | Trivial | Config cleanup |

**Moved OUT of Phase 1 (have backend dependencies):**

| Item | Why | Moved To |
|------|-----|----------|
| Replace hardcoded user profile (3.2) | Needs `GET /api/v1/customer/profile` verified | Phase 2 (API wiring) |
| Replace mock IP sets (3.8) | Needs API client setup in admin frontend first | Phase 2 (API wiring) |
| Replace mock chart data (3.9) | Needs API client setup in customer frontend first | Phase 2 (API wiring) |
| Replace mock dashboard alerts (3.4) | **Backend alerts endpoint doesn't exist** — needs implementation | Phase 4 (planned features) |
| Add webhook auto-disable notification (1.7) | Email/Telegram delivery not implemented yet (Section 6.4) | Phase 4 (after notification service) |

### Phase 2: Frontend ↔ Backend API Wiring (2-4 weeks)

Wire all existing UI to the working backend API. No new backend features needed.

| # | Item | Section | Effort |
|---|------|---------|--------|
| 1 | Admin — Node Management wiring | 4.1 | Medium |
| 2 | Admin — VM Management wiring | 4.2 | Medium |
| 3 | Admin — Plan Management wiring | 4.3 | Low |
| 4 | Admin — Template Management wiring | 4.4 | Low |
| 5 | Admin — IP Management wiring | 4.5 | Low |
| 6 | Admin — Customer Management wiring | 4.6 | Medium |
| 7 | Admin — Audit Logs wiring | 4.7 | Low |
| 8 | Customer — VM Management wiring | 5.1 | Medium |
| 9 | Customer — Webhook Configuration wiring | 5.2 | Medium |
| 10 | Customer — Settings/Profile wiring | 5.3 | Low |
| 11 | Customer — API Key Management wiring | 6.5 | Low |
| 12 | Replace hardcoded user profile with API call | 3.2 | Low | Depends on `GET /api/v1/customer/profile` — verify endpoint exists, then wire |
| 13 | Replace mock IP sets with API data | 3.8 | Low | Needs admin API client setup (item #5) first |
| 14 | Replace mock chart data with API data | 3.9 | Low | Needs customer API client setup (item #8) first |

> **Dependency note:** Items 12-14 were moved here from Phase 1 because they require working API client connections. Item 12 depends on Settings/Profile wiring (#10). Items 13-14 depend on admin/customer wiring being set up first.

### Phase 3: Core Node Agent gRPC Implementation (3-5 weeks)

Implement the 19 missing gRPC methods, starting with the most critical.

| Priority | Methods | Section | Effort |
|----------|---------|---------|--------|
| **P0** | ReinstallVM, ResizeVM | 2.1-2.2 | Medium |
| **P0** | CreateSnapshot, DeleteSnapshot, RevertSnapshot, ListSnapshots | 2.8-2.11 | Medium |
| **P1** | StreamVNCConsole, StreamSerialConsole | 2.6-2.7 | High |
| **P1** | GuestSetPassword, GuestExecCommand | 2.12-2.13 | Medium |
| **P1** | GetBandwidthUsage, SetBandwidthLimit, ResetBandwidthCounters | 2.17-2.19 | Medium |
| **P2** | MigrateVM, AbortMigration, PostMigrateSetup | 2.3-2.5 | High |
| **P2** | GuestFreezeFilesystems, GuestThawFilesystems, GuestGetNetworkInterfaces | 2.14-2.16 | Low |

> **Internal dependency note:** `GuestFreezeFilesystems`/`GuestThawFilesystems` (2.14-2.15) are needed for consistent snapshots. Consider implementing them alongside P0 Snapshot methods (2.8-2.11) even though they're prioritized P2.

### Phase 4: Planned Features (4-8 weeks)

Implement features that are designed but have no working implementation.

| Priority | Feature | Section | Effort | Dependencies |
|----------|---------|---------|--------|-------------|
| **P0** | Console Access (NoVNC + Serial) | 6.1 | High | Phase 3: StreamVNCConsole/StreamSerialConsole (2.6-2.7) |
| **P0** | Backup Management (end-to-end) | 6.2 | High | — |
| **P0** | Snapshot Management (end-to-end) | 6.3 | Medium | Phase 3: Snapshot gRPC methods (2.8-2.11) |
| **P1** | Notification Delivery (email + Telegram) | 6.4 | Medium | — (**implement early — others depend on this**) |
| **P1** | Password Reset Workflow | 6.6 | Medium | ⚠️ Notification Delivery (6.4) must be done first |
| **P1** | Auth Backup Codes (use flow) | 1.4 | Low | — |
| **P1** | Webhook Registration method | 1.5 | Low | — |
| **P1** | Webhook auto-disable notification | 1.7 | Low | ⚠️ Notification Delivery (6.4) must be done first |
| **P1** | Replace mock dashboard alerts with real alerting | 3.4 | Medium | ⚠️ Backend alerts endpoint must be created first |
| **P2** | VM Live Migration (end-to-end) | 6.7 | High | Phase 3: MigrateVM/AbortMigration/PostMigrateSetup (2.3-2.5) |
| **P2** | Automatic Node Failover | 6.8 | High | — |
| **P2** | VM Resize (end-to-end) | 6.9 | Medium | Phase 3: ResizeVM (2.2) |
| **P2** | VM Reinstall (end-to-end) | 6.10 | Medium | Phase 3: ReinstallVM (2.1) |
| **P3** | Reverse DNS Management | 6.11 | Medium | — |
| **P3** | ISO Upload & Custom Boot | 6.12 | Medium | — |

> **Ordering note within Phase 4:** Implement Notification Delivery (6.4) FIRST among P1 items — Password Reset (6.6) and Webhook auto-disable notification (1.7) both depend on it. Console Access (6.1) and Snapshot Management (6.3) depend on Phase 3 gRPC implementations.

### Phase 5: WHMCS & Integration (2-3 weeks)

| # | Item | Section | Effort |
|---|------|---------|--------|
| 1 | Fix TestConnection placeholder | 7.1 | Low |
| 2 | Complete webhook event handlers | 7.2 | Medium |
| 3 | Add webhook payload validation | 7.3 | Low |
| 4 | Fix helper function graceful fallbacks | 7.4 | Low |
| 5 | End-to-end WHMCS testing | 7.7 | High |

> **Internal ordering:** Item 5 (E2E testing) must be done last — it depends on items 1-4 being complete.

### Phase 6: Testing & Quality (Ongoing)

| Priority | Item | Section | Effort |
|----------|------|---------|--------|
| **P0** | Unit tests for API handlers | 8.1 | High |
| **P0** | Unit tests for core services | 8.2 | High |
| **P1** | Unit tests for shared utilities | 8.3 | Medium |
| **P1** | Unit tests for node agent | 8.4 | Medium |
| **P1** | Unit tests for task workers | 8.5 | Medium |
| **P2** | Additional integration tests | 8.6 | Medium |
| **P2** | Fix auth permission test placeholder | 8.8 | Low |
| **P3** | E2E tests for new features | 8.7 | High |

> **Cross-phase note:** Node agent unit tests (8.4) should be written alongside Phase 3 gRPC implementations. E2E tests (8.7) depend on Phase 4 features being complete.

### Phase 7: Infrastructure & Security (1-2 weeks)

| # | Item | Section | Effort |
|---|------|---------|--------|
| 1 | IP allowlisting for provisioning API | 9.7 | Low |
| 2 | SSL certificate automation | 10.1 | Medium |
| 3 | Production credential enforcement | 10.2 | Low |
| 4 | Guest agent command whitelisting | 10.3 | Low |
| 5 | Rate limiting verification | 10.4 | Low |
| 6 | CI/CD pipeline | 9.4 | Medium |
| 7 | Kubernetes/Helm (optional) | 9.5 | High |

> **Cross-phase dependency:** Item 4 (Guest agent command whitelisting) depends on Phase 3 `GuestExecCommand` (2.13) being implemented first — can't whitelist commands for a method that doesn't exist yet.

---

## Summary Statistics

| Category | Count |
|----------|-------|
| Go Backend TODO/FIXME/Stub items | 7 |
| Unimplemented gRPC methods | 19 |
| Frontend mock/hardcoded data items | 9 |
| Admin portal features needing API wiring | 7 |
| Customer portal features needing API wiring | 3 |
| Planned features not yet implemented | 12 |
| WHMCS integration items | 7 |
| Test coverage gaps | 8 categories |
| Infrastructure gaps | 7 |
| Security hardening items | 4 |
| **Total individual items** | **~83+** |

---

*This document should be updated as items are completed. Mark items with ✅ when done.*
