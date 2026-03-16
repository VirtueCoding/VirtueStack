# VirtueStack Architecture

**Version:** 2.0 — March 2026  
**Status:** Implementation Complete — ~90%  
**Companion:** `CODING_STANDARD.md` — all code MUST pass the 16 Quality Gates defined there.  
**Applies to:** Every component, API, schema, and module described below.

> **SINGLE SOURCE OF TRUTH:** This document defines the VirtueStack system architecture.  
> The Coding Standard defines HOW to code it. This document defines WHAT to build.

---

## TABLE OF CONTENTS

1. [Executive Summary](#1-executive-summary)
2. [Technology Stack](#2-technology-stack)
3. [System Architecture Overview](#3-system-architecture-overview)
4. [Component 1: Node Agent](#4-component-1-node-agent)
5. [Component 2: Controller Orchestrator](#5-component-2-controller-orchestrator)
6. [Component 3: Web UIs](#6-component-3-web-uis)
7. [Component 4: WHMCS Integration Module](#7-component-4-whmcs-integration-module)
8. [Networking Architecture](#8-networking-architecture)
9. [Storage Architecture (Dual Backend)](#9-storage-architecture-dual-backend)
10. [Security Architecture](#10-security-architecture)
11. [Database Schema](#11-database-schema)
12. [API Specifications](#12-api-specifications)
13. [Async Task System](#13-async-task-system)
14. [HA Failover & STONITH/Fencing](#14-ha-failover--stonithfencing)
15. [Backup & Recovery System](#15-backup--recovery-system)
16. [Reverse DNS (rDNS) System](#16-reverse-dns-rdns-system)
17. [Monitoring & Observability](#17-monitoring--observability)
18. [Notification System](#18-notification-system)
19. [Implementation Phases](#19-implementation-phases)
20. [Quality Gates Mapping](#20-quality-gates-mapping)
21. [Graceful Degradation Matrix](#21-graceful-degradation-matrix)
22. [Environment Variables Reference](#22-environment-variables-reference)

---

## 1. EXECUTIVE SUMMARY

### What Is VirtueStack?

VirtueStack is a fully-secured, optimized, modern platform for managing KVM (QEMU) Virtual Machines / Virtual Private Servers. It provides:

- **Node Agents** running on bare-metal Ubuntu 24.04 servers with Ceph RBD (shared) or local QCOW2 (file-based) storage
- **Controller Orchestrator** (Docker) exposing three secured APIs for provisioning, customer, and admin operations
- **Two Web UIs** (Docker) — Admin panel and Customer panel with NoVNC/serial console
- **WHMCS Billing Integration** — automated VM lifecycle management from sales to termination

### Core Philosophy

1. **Security-first**: Every component treats all input as hostile. OWASP 2025 compliance.
2. **Event-driven**: All long-running operations (VM create, migrate, backup) are async via durable message queue.
3. **Shared-nothing nodes**: Nodes are stateless compute. All persistent state lives in Ceph RBD or local QCOW2 files (VM disks) and PostgreSQL (config/metadata).
4. **Defense in depth**: IP anti-spoofing at hypervisor level, mTLS between components, tenant isolation at database level.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Single language for backend | Go 1.25+ | One language for Node Agent + Controller. Single binary deployment. Excellent libvirt/Ceph bindings. |
| Async task execution | NATS JetStream | Durable message queue embedded in Controller. VM provisioning takes 5-60s — APIs cannot block. |
| Console proxying | Controller as WebSocket proxy | Web UIs NEVER talk directly to Node Agents. Controller validates auth, proxies VNC/serial via gRPC stream. |
| HA failover safety | IPMI fencing (optional) + admin confirmation | Without fencing, admin manually confirms dead node. With IPMI credentials, auto-fence before RBD lock release. |
| Network topology | Linux Bridge (default) | Recommended default. OVS migration path documented for advanced QoS/VXLAN needs. |

---

## 2. TECHNOLOGY STACK

### Languages & Frameworks

| Component | Language | Framework/Libraries | Version |
|-----------|----------|-------------------|---------|
| **Node Agent** | Go | `libvirt.org/go/libvirt`, `github.com/ceph/go-ceph`, gRPC | Go 1.25+ |
| **Controller** | Go | Gin, `pgx` (PostgreSQL), `nats.go`, gRPC, gorilla/websocket | Go 1.25+ |
| **Admin WebUI** | TypeScript | React 19, Next.js 16, shadcn/ui, Tailwind CSS, TanStack Query | TS 5.5+ |
| **Customer WebUI** | TypeScript | React 19, Next.js 16, shadcn/ui, Tailwind CSS, TanStack Query | TS 5.5+ |
| **WHMCS Module** | PHP | WHMCS SDK, Guzzle HTTP | PHP 8.3+ |

### Infrastructure

| Component | Technology | Version |
|-----------|-----------|---------|
| **Database** | PostgreSQL | 16+ |
| **Message Queue** | NATS JetStream | 2.10+ |
| **Reverse Proxy** | Nginx | 1.25+ |
| **VM Hypervisor** | KVM/QEMU via libvirt | libvirt 10.x, QEMU 8.x |
| **Storage** | Ceph RBD / QCOW2 file-based | Reef (18.x) or Squid (19.x) / local filesystem |
| **DNS** | PowerDNS (MySQL backend) | 4.9+ |
| **Container Runtime** | Docker + Docker Compose | 26+ |

### Frontend Libraries

| Purpose | Library | Rationale |
|---------|---------|-----------|
| **UI Components** | shadcn/ui + Tailwind CSS | Copy-paste ownership, highly customizable, accessible |
| **Real-time Charts** | uPlot (streaming) + Apache ECharts (dashboards) | uPlot: 150K pts in 90ms. ECharts: rich dashboard visualizations |
| **VNC Console** | @novnc/novnc | Industry standard HTML5 VNC client |
| **Serial Console** | @xterm/xterm + @xterm/addon-fit | Best terminal emulation, used by VS Code |
| **ISO Upload** | tus-js-client | Resumable chunked uploads for multi-GB files |
| **Server State** | TanStack Query | Cache invalidation, optimistic updates, background refetching |
| **Client State** | Zustand | Lightweight, TypeScript-native, no boilerplate |
| **Form Validation** | Zod | Runtime validation with TypeScript type inference |
| **HTTP Client** | ky (browser) | Lightweight fetch wrapper with retries |

### Go Libraries

| Purpose | Library | Rationale |
|---------|---------|-----------|
| **HTTP Framework** | `github.com/gin-gonic/gin` | 48% Go API market share, battle-tested |
| **PostgreSQL** | `github.com/jackc/pgx/v5` | Fastest Go PostgreSQL driver, connection pooling |
| **Migrations** | `github.com/golang-migrate/migrate` | SQL-based migrations, rollback support |
| **Validation** | `github.com/go-playground/validator/v10` | Struct tag validation, custom validators |
| **JWT** | `github.com/golang-jwt/jwt/v5` | HMAC/RSA/ECDSA signing |
| **TOTP** | `github.com/pquerna/otp` | RFC 6238 TOTP, QR code generation |
| **Logging** | `log/slog` (stdlib) | Structured JSON logging, zero dependencies |
| **gRPC** | `google.golang.org/grpc` | Controller↔Node communication |
| **NATS** | `github.com/nats-io/nats.go` | JetStream for durable async tasks |
| **WebSocket** | `github.com/gorilla/websocket` | VNC/serial console proxying |
| **Password Hashing** | `github.com/alexedwards/argon2id` | Argon2id per coding standard |
| **Ceph** | `github.com/ceph/go-ceph` | RBD operations, snapshot management |
| **libvirt** | `libvirt.org/go/libvirt` | Official Go bindings |
| **Crypto** | `crypto/rand`, `crypto/aes` (stdlib) | Token generation, AES-256-GCM encryption |

---

## 3. SYSTEM ARCHITECTURE OVERVIEW

### High-Level Architecture

```
                                    ┌──────────────────────────────────────────────────────────────┐
                                    │                        DOCKER HOST                            │
                                    │                                                              │
    ┌──────────┐                    │  ┌─────────┐    ┌──────────────────┐    ┌────────────────┐   │
    │  WHMCS   │──Provisioning API──┼─►│         │    │                  │    │                │   │
    │ (Billing)│                    │  │  Nginx  │───►│   Controller     │◄──►│  PostgreSQL    │   │
    └──────────┘                    │  │ (TLS +  │    │   (Go/Gin)       │    │  (Config DB)   │   │
                                    │  │  Rate   │    │                  │    │                │   │
    ┌──────────┐                    │  │  Limit) │    │  ┌────────────┐  │    └────────────────┘   │
    │ Customer │──Customer API──────┼─►│         │───►│  │ NATS       │  │                         │
    │ Browser  │                    │  │         │    │  │ JetStream  │  │    ┌────────────────┐   │
    └──────────┘                    │  │         │    │  │ (Task Q)   │  │    │                │   │
                                    │  │         │    │  └────────────┘  │    │  Admin WebUI   │   │
    ┌──────────┐                    │  │         │    │                  │    │  (Next.js)     │   │
    │  Admin   │──Admin API─────────┼─►│         │───►│  ┌────────────┐  │    │                │   │
    │ Browser  │                    │  │         │    │  │ WS Proxy   │  │    └────────────────┘   │
    └──────────┘                    │  └─────────┘    │  │ (VNC/Term) │  │                         │
                                    │                 │  └────────────┘  │    ┌────────────────┐   │
                                    │                 └────────┬─────────┘    │                │   │
                                    │                          │              │ Customer WebUI │   │
                                    │                          │              │  (Next.js)     │   │
                                    │                          │              │                │   │
                                    └──────────────────────────┼──────────────────────────────────┘
                                                               │
                                                    gRPC + mTLS │
                                                               │
                    ┌──────────────────────────────────────────┼──────────────────────────────┐
                    │                                          │                              │
           ┌────────▼────────┐                      ┌──────────▼──────────┐        ┌─────────▼─────────┐
           │   Node Agent 1  │                      │   Node Agent 2      │        │   Node Agent N    │
           │   (Go daemon)   │                      │   (Go daemon)       │        │   (Go daemon)     │
           │                 │                      │                     │        │                   │
           │ ┌─────────────┐ │                      │ ┌─────────────────┐ │        │ ┌───────────────┐ │
           │ │   libvirt    │ │                      │ │   libvirt       │ │        │ │   libvirt     │ │
           │ │   KVM/QEMU  │ │                      │ │   KVM/QEMU     │ │        │ │   KVM/QEMU   │ │
           │ └──────┬──────┘ │                      │ └───────┬─────────┘ │        │ └──────┬────────┘ │
           │        │        │                      │         │           │        │        │          │
           │   VM1  VM2  VM3 │                      │   VM4   VM5   VM6  │        │  VM7   VM8  VM9  │
           │                 │                      │                     │        │                   │
           │ ┌─────────────┐ │                      │ ┌─────────────────┐ │        │ ┌───────────────┐ │
           │ │  Linux Br   │ │                      │ │  Linux Br       │ │        │ │  Linux Br     │ │
           │ │  + nwfilter  │ │                      │ │  + nwfilter     │ │        │ │  + nwfilter   │ │
           │ │  + dnsmasq   │ │                      │ │  + dnsmasq      │ │        │ │  + dnsmasq    │ │
           │ │  + tc QoS    │ │                      │ │  + tc QoS       │ │        │ │  + tc QoS     │ │
           │ └─────────────┘ │                      │ └─────────────────┘ │        │ └───────────────┘ │
           └────────┬────────┘                      └──────────┬──────────┘        └────────┬──────────┘
                    │                                          │                            │
                     └──────────────────────┬───────────────────┘────────────────────────────┘
                                            │
                                   ┌────────▼────────┐
                                   │  Storage Backend │
                                   │                  │
                                   │  Option A: Ceph  │
                                   │  Pool: vs-vms    │
                                   │  Pool: vs-images │
                                   │  Pool: vs-backups│
                                   │                  │
                                   │  Option B: QCOW2 │
                                   │  Local: per-node │
                                   └──────────────────┘
```

### Communication Patterns

| Path | Protocol | Auth | Purpose |
|------|----------|------|---------|
| Browser → Nginx → Controller | HTTPS (TLS 1.3) | JWT (Customer/Admin), API Key (Provisioning) | All API calls |
| Browser → Nginx → WebUI | HTTPS (TLS 1.3) | Session cookie | Serving web application |
| Browser → Controller (WS) | WSS via Nginx | JWT in `Sec-WebSocket-Protocol` | VNC/serial console, real-time status |
| Controller → Node Agent | gRPC over HTTP/2 | mTLS (mutual TLS certificates) | VM lifecycle, console streams, metrics |
| Controller → PostgreSQL | TCP | Password + TLS | Config, state, audit logs |
| Controller → NATS JetStream | TCP | Token auth | Async task queue |
| Controller → PowerDNS MySQL | TCP | Password + TLS | rDNS PTR record management |
| Node Agent → Ceph | TCP (librados) | Cephx auth (`client.virtuestack`) | RBD disk operations |
| Node Agent → Local FS | POSIX | Local filesystem permissions | QCOW2 disk file operations |
| Node Agent → libvirt | Unix socket | Local (qemu:///system) | VM management |
| WHMCS → Controller | HTTPS | API Key (Provisioning API) | VM provisioning/lifecycle |

---

## 4. COMPONENT 1: NODE AGENT

### Overview

The Node Agent is a Go daemon running on each bare-metal Ubuntu 24.04 server. It manages VMs via libvirt, handles storage operations (Ceph RBD or local QCOW2), enforces network security, and streams console data. It is **stateless** — all persistent state lives in the configured storage backend and the Controller's PostgreSQL.

### Installation

- Deployed as a single static Go binary via `scp` or package manager
- Runs as a `systemd` service: `virtuestack-node-agent.service`
- Requires: `libvirt-daemon-system`, `qemu-kvm`, `qemu-utils`, `cloud-image-utils`, `dnsmasq` (`ceph-common` only for Ceph backend)
- Configuration via `/etc/virtuestack/node-agent.yaml` or environment variables

### Go Project Structure

```
cmd/
  node-agent/main.go              # Entrypoint — thin, calls internal/
internal/
  nodeagent/
    server.go                     # gRPC server setup, lifecycle
    config.go                     # Configuration loading
    vm/
      lifecycle.go                # Create, start, stop, delete, reinstall
      migration.go                # Live migration handling
      domain_xml.go               # libvirt XML generation
      console.go                  # VNC/serial console streaming
      metrics.go                  # VM resource stats (CPU/RAM/disk/net)
    storage/
      interface.go                # StorageBackend interface (abstraction)
      rbd.go                      # Ceph RBD operations (clone, resize, snapshot)
      qcow.go                     # File-based QCOW2 operations (qemu-img)
      template.go                 # Template management (RBD clone + cloud-init)
      cloudinit.go                # Cloud-init ISO generation
    network/
      bridge.go                   # Linux bridge management
      nwfilter.go                 # IP/MAC anti-spoofing filters
      bandwidth.go                # tc QoS + bandwidth accounting
      dhcp.go                     # dnsmasq DHCP management
    guest/
      agent.go                    # QEMU Guest Agent communication
  shared/
    errors/                       # Custom error types
    proto/                        # Generated protobuf/gRPC code
proto/
  virtuestack/
    node_agent.proto              # gRPC service definition
```

### gRPC Service Definition

```protobuf
syntax = "proto3";
package virtuestack.nodeagent;

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

### Domain XML Template

Every VM is defined with this libvirt domain XML pattern (generated by `domain_xml.go`):

```xml
<domain type='kvm'>
  <name>vs-{vm_uuid}</name>
  <uuid>{vm_uuid}</uuid>
  <memory unit='MiB'>{memory_mb}</memory>
  <currentMemory unit='MiB'>{memory_mb}</currentMemory>
  <vcpu placement='static'>{vcpu_count}</vcpu>
  <cpu mode='host-model'/>
  <!-- host-model for migration compatibility; host-passthrough if pinned to node -->

  <os>
    <type arch='x86_64' machine='pc-q35-8.2'>hvm</type>
    <boot dev='hd'/>
    <boot dev='cdrom'/>
  </os>

  <features>
    <acpi/><apic/><pae/>
  </features>

  <clock offset='utc'>
    <timer name='rtc' tickpolicy='catchup'/>
    <timer name='pit' tickpolicy='delay'/>
    <timer name='hpet' present='no'/>
  </clock>

  <devices>
    <!-- Primary disk: Ceph RBD (when storage_backend = "ceph") -->
    <disk type='network' device='disk'>
      <driver name='qemu' type='raw' cache='none' io='native' discard='unmap'/>
      <source protocol='rbd' name='vs-vms/vs-{vm_uuid}-disk0'>
        <host name='{ceph_mon_1}' port='6789'/>
        <host name='{ceph_mon_2}' port='6789'/>
        <host name='{ceph_mon_3}' port='6789'/>
      </source>
      <auth username='virtuestack'>
        <secret type='ceph' uuid='{ceph_secret_uuid}'/>
      </auth>
      <target dev='vda' bus='virtio'/>
    </disk>

    <!-- Primary disk: QCOW2 file (when storage_backend = "qcow") -->
    <!-- <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2' cache='writeback' discard='unmap'/>
      <source file='/var/lib/virtuestack/vms/vs-{vm_uuid}-disk0.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk> -->

    <!-- Cloud-init seed ISO (removed after first boot) -->
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='/var/lib/virtuestack/cloud-init/vs-{vm_uuid}-seed.iso'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>

    <!-- Customer ISO (optional, attached via API) -->
    <!-- <disk type='file' device='cdrom'>
      <source file='/var/lib/virtuestack/iso/{customer_iso}'/>
      <target dev='sdb' bus='sata'/>
      <readonly/>
    </disk> -->

    <!-- Network: virtio with anti-spoofing -->
    <interface type='bridge'>
      <source bridge='br0'/>
      <mac address='{mac_address}'/>
      <model type='virtio'/>
      <driver name='vhost' txmode='iothread' ioeventfd='on' queues='4'/>
      <filterref filter='virtuestack-clean-traffic'>
        <parameter name='IP' value='{ipv4_address}'/>
        <parameter name='IPV6' value='{ipv6_address}'/>
        <parameter name='MAC' value='{mac_address}'/>
      </filterref>
      <bandwidth>
        <inbound average='{port_speed_kbps}' peak='{port_speed_kbps_burst}' burst='{burst_kb}'/>
        <outbound average='{port_speed_kbps}' peak='{port_speed_kbps_burst}' burst='{burst_kb}'/>
      </bandwidth>
    </interface>

    <!-- QEMU Guest Agent channel -->
    <channel type='unix'>
      <source mode='bind' path='/var/lib/libvirt/qemu/channel/target/vs-{vm_uuid}.org.qemu.guest_agent.0'/>
      <target type='virtio' name='org.qemu.guest_agent.0'/>
    </channel>

    <!-- VNC console (localhost only, proxied via Controller) -->
    <graphics type='vnc' port='-1' listen='127.0.0.1'>
      <listen type='address' address='127.0.0.1'/>
    </graphics>

    <!-- Serial console -->
    <serial type='pty'>
      <target type='isa-serial' port='0'/>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>

    <!-- Virtio RNG for guest entropy -->
    <rng model='virtio'>
      <backend model='random'>/dev/urandom</backend>
    </rng>

    <!-- Virtio balloon for memory management -->
    <memballoon model='virtio'/>
  </devices>
</domain>
```

### Cloud-Init Provisioning Flow

1. **Admin creates OS template**: Base cloud image → imported into storage backend → protected snapshot (Ceph) or standalone backing file (QCOW2)
2. **VM provisioning request arrives** (via gRPC from Controller)
3. **Clone from template**:
   - Ceph: `rbd clone vs-images/ubuntu-24.04-base@base vs-vms/vs-{uuid}-disk0` (instant, copy-on-write)
   - QCOW2: `qemu-img create -b template.qcow2 -F qcow2 -f qcow2 vm-disk.qcow2` (instant, copy-on-write)
4. **Resize if needed**: `rbd resize ...` or `qemu-img resize ...`
5. **Generate cloud-init ISO** (NoCloud datasource):
   - `meta-data`: instance-id, hostname
   - `user-data`: root password (hashed), SSH keys, packages, timezone
   - `network-config`: static IPv4/IPv6 via Netplan
6. **Define + start VM** via libvirt with domain XML pointing to storage backend + cloud-init ISO
7. **VM boots** → cloud-init configures networking, credentials, hostname
8. **Report success** to Controller via gRPC response

### Cloud-Init Network Config (Netplan v2)

```yaml
network:
  version: 2
  renderer: networkd
  ethernets:
    ens3:
      addresses:
        - 192.0.2.50/24
        - "2001:db8:abcd:00a5::1/64"
      routes:
        - to: 0.0.0.0/0
          via: 192.0.2.1
        - to: "::/0"
          via: "2001:db8:abcd:00a5::ffff"
      nameservers:
        addresses:
          - 1.1.1.1
          - 8.8.8.8
          - "2606:4700:4700::1111"
```

### QEMU Guest Agent — Allowed Commands

**Security constraint**: The Guest Agent runs as root inside the VM. Limit Controller-initiated commands strictly:

| Command | Purpose | When Used |
|---------|---------|-----------|
| `guest-ping` | Health check | Heartbeat, pre-backup check |
| `guest-fsfreeze-freeze` | Freeze filesystems | Before RBD snapshot for consistent backup |
| `guest-fsfreeze-thaw` | Unfreeze filesystems | After RBD snapshot |
| `guest-shutdown` | Graceful shutdown | Stop VM request |
| `guest-set-user-password` | Set root/admin password | Reinstall, password reset |
| `guest-network-get-interfaces` | Get IP/MAC info | IP verification, status display |
| `guest-get-osinfo` | OS details | Dashboard display |

**MUST NOT expose**: `guest-exec`, `guest-file-*` — arbitrary command execution is a privilege escalation vector if the Controller is compromised.

### Bandwidth Management

**Three-layer approach per VM:**

1. **Port Speed Limiting** (libvirt `<bandwidth>` in domain XML):
   - `average`: sustained rate matching plan (100/500/1000/10000 Mbps)
   - Converted to Kbit/s for libvirt XML

2. **Bandwidth Accounting** (nftables named counters per VM tap interface):
   ```
   nft add counter inet vs_accounting vm_{uuid}_rx
   nft add counter inet vs_accounting vm_{uuid}_tx
   ```
   - Polled every 5 minutes by Node Agent
   - Reported to Controller via `GetBandwidthUsage` gRPC call

3. **Overage Throttling** (tc HTB qdisc):
   - When monthly bandwidth cap exceeded → Controller sends `SetBandwidthLimit(5 Mbps)` to Node Agent
   - Node Agent applies `tc` class change on VM's tap interface
   - Reset on 1st of each month via Controller scheduled task
   - **Live Migration Note**: `tc` rules on the `vnet` tap interface are destroyed during live migration. The Controller MUST call a `PostMigrateSetup` gRPC hook on the destination Node Agent immediately after migration completes to re-apply any active `tc` throttles, `nftables` counters, and `nwfilter` rules.

### Boot Order Management

Customer can change boot order via API:

| Boot Order | Domain XML |
|------------|-----------|
| Disk first (default) | `<boot dev='hd'/><boot dev='cdrom'/>` |
| CD-ROM first (ISO install) | `<boot dev='cdrom'/><boot dev='hd'/>` |
| Network boot (PXE) | `<boot dev='network'/><boot dev='hd'/>` |

Implemented by updating the domain XML and restarting the VM.

---

## 5. COMPONENT 2: CONTROLLER ORCHESTRATOR

### Overview

The Controller is a Go application deployed in Docker. It is the central brain of VirtueStack — orchestrating all Node Agents, serving three APIs, managing async tasks, proxying console connections, and maintaining system state.

### Go Project Structure

```
cmd/
  controller/main.go               # Entrypoint
internal/
  controller/
    server.go                      # HTTP server setup (Gin)
    grpc_client.go                 # gRPC client pool to Node Agents
    config.go                      # Configuration loading

    api/
      middleware/
        auth.go                    # JWT + API Key authentication
        rbac.go                    # Role-based access control
        ratelimit.go               # Sliding window rate limiter
        audit.go                   # Audit logging middleware
        correlation.go             # X-Correlation-Id generation
        validation.go              # Request validation middleware

      provisioning/                # Provisioning API handlers (WHMCS)
        handlers.go
        routes.go

      customer/                    # Customer API handlers
        handlers.go
        routes.go

      admin/                       # Admin API handlers
        handlers.go
        routes.go

    ws/
      proxy.go                     # WebSocket proxy (VNC/serial via gRPC stream)
      hub.go                       # WebSocket connection hub
      status.go                    # Real-time VM status broadcasting

    services/
      vm.go                        # VM orchestration business logic
      node.go                      # Node management
      backup.go                    # Backup scheduling & orchestration
      migration.go                 # Migration orchestration (pre-check → execute → verify)
      rdns.go                      # PowerDNS rDNS management
      ipam.go                      # IP Address Management
      template.go                  # OS template management
      plan.go                      # VM plan (product) management
      customer.go                  # Customer account management
      auth.go                      # Authentication service (login, 2FA, sessions)
      notification.go              # Email + Telegram notifications
      webhook.go                   # Webhook delivery service
      failover.go                  # HA failover orchestration

    tasks/
      worker.go                    # NATS JetStream task worker
      types.go                     # Task type definitions
      vm_create.go                 # Async VM creation task
      vm_reinstall.go              # Async VM reinstall task
      backup_create.go             # Async backup creation task
      migration_execute.go         # Async migration task

    models/
      vm.go                        # VM data model
      node.go                      # Node data model
      customer.go                  # Customer data model
      plan.go                      # Plan data model
      ip.go                        # IP allocation model
      backup.go                    # Backup model
      audit.go                     # Audit log model
      task.go                      # Async task model

    repository/
      vm_repo.go                   # VM database operations
      node_repo.go                 # Node database operations
      customer_repo.go             # Customer database operations
      ip_repo.go                   # IP allocation database operations
      audit_repo.go                # Audit log database operations

  shared/
    errors/                        # Custom error types (shared with Node Agent)
    proto/                         # Generated protobuf/gRPC code
```

### Three-Tier API Architecture

All three APIs share the same Go binary but are separated by route prefix and middleware:

```
/api/v1/provisioning/*   → API Key auth → Provisioning handlers
/api/v1/customer/*       → JWT auth → Customer handlers (tenant-isolated)
/api/v1/admin/*          → JWT + 2FA auth → Admin handlers (full access)
/ws/vnc/{vm_id}          → JWT → WebSocket VNC proxy
/ws/serial/{vm_id}       → JWT → WebSocket serial proxy
/ws/status               → JWT → Real-time VM status stream
/health                  → No auth → Liveness check
/ready                   → No auth → Readiness check
/metrics                 → Internal → Prometheus metrics
```

### Authentication Model

| API Tier | Auth Method | Token Lifetime | Additional |
|----------|-------------|----------------|------------|
| **Provisioning** | API Key in `X-API-Key` header | No expiry (revocable) | IP allowlist, rate limit: 1000 req/min |
| **Customer** | JWT access + refresh token | Access: 15min, Refresh: 7d | Tenant isolation enforced |
| **Admin** | JWT access + refresh + TOTP | Access: 15min, Refresh: 4h | 2FA required, re-auth for destructive ops, max 3 sessions |

### Authorization — RBAC Permissions

```go
// Permission definitions
const (
    // Customer permissions
    PermVMList          = "vm:list"
    PermVMStart         = "vm:start"
    PermVMStop          = "vm:stop"
    PermVMRestart       = "vm:restart"
    PermVMConsole       = "vm:console"
    PermVMReinstall     = "vm:reinstall"
    PermVMBootOrder     = "vm:boot_order"
    PermVMISO           = "vm:iso"
    PermVMMetrics       = "vm:metrics"
    PermVMRDNS          = "vm:rdns"
    PermVMBackup        = "vm:backup"
    PermVMSnapshot      = "vm:snapshot"
    PermVMAPIKey        = "vm:api_key"
    PermVMWebhook       = "vm:webhook"

    // Admin-only permissions
    PermVMCreate        = "vm:create"
    PermVMDelete        = "vm:delete"
    PermVMResize        = "vm:resize"
    PermVMMigrate       = "vm:migrate"
    PermNodeManage      = "node:manage"
    PermNodeFailover    = "node:failover"
    PermIPManage        = "ip:manage"
    PermPlanManage      = "plan:manage"
    PermTemplateManage  = "template:manage"
    PermCustomerManage  = "customer:manage"
    PermBackupManage    = "backup:manage"
    PermSettingsManage  = "settings:manage"
    PermAuditView       = "audit:view"
)
```

### VM Plan Constraints

Each plan defines resource limits enforced at VM creation and resize:

| Constraint | Type | Example Values |
|------------|------|---------------|
| `max_vcpu` | integer | 1, 2, 4, 8, 16, 32 |
| `max_memory_mb` | integer | 1024, 2048, 4096, 8192, 16384, 32768 |
| `max_disk_gb` | integer | 20, 40, 80, 160, 320, 640 |
| `port_speed_mbps` | integer | 100, 500, 1000, 10000 (10000 = unlimited plan) |
| `bandwidth_limit_gb` | integer | 500, 1000, 2000, 5000, 0 (0 = unlimited) |
| `bandwidth_overage_speed_mbps` | integer | 5 (throttle speed when cap exceeded) |
| `max_ipv4` | integer | 1, 2, 4, 8 |
| `max_ipv6_prefix` | string | "/64" (one /64 block per VM) |
| `max_snapshots` | integer | 3, 5, 10 → now `snapshot_limit` (DEFAULT 2, enforced on Customer API) |
| `max_backups` | integer | 1, 3, 5 → now `backup_limit` (DEFAULT 2, enforced on Customer API) |
| `max_iso_gb` | integer | 0, 5, 10, 20 (max ISO storage per customer) |
| `max_iso_count` | integer | 0, 1, 3, 5 → now `iso_upload_limit` (DEFAULT 2, enforced on Customer API) |

### Audit Logging

Every mutating API call is logged to an append-only `audit_logs` table:

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Unique log entry ID |
| `timestamp` | TIMESTAMPTZ | When the action occurred |
| `actor_id` | UUID | User/API key that performed the action |
| `actor_type` | ENUM | `admin`, `customer`, `provisioning`, `system` |
| `actor_ip` | INET | Source IP address |
| `action` | VARCHAR(100) | e.g., `vm.create`, `vm.start`, `node.failover.approve` |
| `resource_type` | VARCHAR(50) | `vm`, `node`, `customer`, `backup`, `ip`, `plan` |
| `resource_id` | UUID | Target resource ID |
| `changes` | JSONB | `{ "field": "memory_mb", "old": 1024, "new": 2048 }` |
| `correlation_id` | UUID | Request correlation ID |
| `success` | BOOLEAN | Whether the action succeeded |
| `error_message` | TEXT | Error details if failed |

**Immutability**: `REVOKE UPDATE, DELETE ON audit_logs FROM app_user;`

---

## 6. COMPONENT 3: WEB UIs

### Shared Architecture

Both Admin and Customer WebUIs are Next.js 16 applications served as Docker containers. They share:

- **Same component library** (shadcn/ui + Tailwind CSS)
- **Same auth pattern** (JWT stored in httpOnly cookies)
- **Same WebSocket client** for real-time updates
- **Same noVNC/xterm.js integration** for console access
- **Dark/light theme** with system preference detection

### Admin WebUI Features

| Feature | Description |
|---------|-------------|
| **Dashboard** | Node overview (CPU/RAM/disk usage), VM count, active alerts |
| **Node Management** | List nodes, view health, drain node, trigger failover |
| **VM Management** | Full CRUD, resize, migrate, console, all VMs visible |
| **Plan Management** | Create/edit VM plans with resource constraints and per-VM limits (snapshots, backups, ISO uploads) |
| **Template Management** | Upload base images, create templates, manage template catalog |
| **IP Set Management** | Create IP pools per location/VLAN, assign IPs, manage subnets |
| **Customer Management** | View customers, their VMs, usage, suspend/unsuspend |
| **Backup Management** | Configure backup schedules, view backup status, manual trigger |
| **System Settings** | Storage backend config, Ceph credentials, SMTP config, Telegram bot, webhook URLs |
| **Audit Log Viewer** | Search/filter audit logs by user, action, resource, date range |
| **rDNS Management** | View/edit PTR records for all IPs |

### Customer WebUI Features

| Feature | Description |
|---------|-------------|
| **VM List** | Only their own VMs with status indicators (running/stopped/etc.) |
| **VM Control** | Start, stop, force power off, restart |
| **VNC Console** | Embedded noVNC viewer via Controller WebSocket proxy |
| **Serial Console** | Embedded xterm.js via Controller WebSocket proxy |
| **Reinstall** | Select from available OS templates, confirm destructive action |
| **ISO Management** | Upload ISO (tus), attach/detach ISO to VM, change boot order |
| **Resource Graphs** | CPU, RAM, disk, network usage — real-time (uPlot) + historical (ECharts) |
| **Backup/Snapshot** | List backups, restore from backup, create/revert/delete snapshots |
| **rDNS** | Change PTR record for assigned IPs (rate limited: 10/hour) |
| **API Keys** | Create/revoke VM-scoped API keys for programmatic access |
| **Webhooks** | Configure webhook URLs for VM events (start, stop, backup complete) |
| **2FA Setup** | Enable/disable TOTP, view QR code, manage backup codes |

### WebSocket Console Proxy Flow

```
1. Customer clicks "Console" in WebUI
2. WebUI requests one-time console token: POST /api/v1/customer/vms/{id}/console-token
3. Controller validates ownership, generates short-lived token (60s expiry, single-use)
4. Controller returns: { token: "abc123", type: "vnc"|"serial" }
5. WebUI connects: wss://controller/ws/vnc/{vm_id}?token=abc123
6. Controller validates token, looks up VM's Node Agent
7. Controller opens gRPC bidirectional stream to Node Agent: StreamVNCConsole
8. Node Agent connects to libvirt VNC on localhost:{port}
9. Controller bridges WebSocket ←→ gRPC stream (binary frames)
10. On disconnect: cleanup gRPC stream, log session duration/bytes
```

**WebSocket Security (per MASTER_CODING_STANDARD Section 17):**
- Per-customer limit: max 5 concurrent WebSocket connections
- Per-VM limit: max 2 VNC + 2 serial sessions
- Message size: 64KB per frame
- Idle timeout: 30 minutes
- Max session: 8 hours
- Heartbeat: ping every 30s, close if no pong in 10s
- `wss://` only, no WebSocket compression for VNC

### ISO Upload (tus Protocol)

```
POST /api/v1/customer/vms/{id}/iso/upload
  Headers:
    Tus-Resumable: 1.0.0
    Upload-Length: {file_size_bytes}
    Upload-Metadata: filename {base64}, vm_id {base64}
  Response: 201 Created
    Location: /api/v1/customer/vms/{id}/iso/upload/{upload_id}

PATCH /api/v1/customer/vms/{id}/iso/upload/{upload_id}
  Headers:
    Tus-Resumable: 1.0.0
    Upload-Offset: {current_offset}
    Content-Type: application/offset+octet-stream
  Body: <chunk bytes>
```

**Constraints enforced**:
- Max ISO size per plan (configurable, e.g., 5GB)
- Max ISO count per customer per plan (configurable, e.g., 3)
- File type validation: must be valid ISO 9660 image
- Virus scanning on completed upload (ClamAV)
- Stored in `/var/lib/virtuestack/iso/{customer_id}/` on the VM's current Node

---

## 7. COMPONENT 4: WHMCS INTEGRATION MODULE

### Module Structure

```
modules/servers/virtuestack/
  virtuestack.php                  # Main module file (entry point)
  lib/
    ApiClient.php                  # Controller Provisioning API client
    VirtueStackHelper.php          # Shared utilities
  templates/
    overview.tpl                   # Client area overview (Smarty)
    console.tpl                    # Console embed (iframe to Customer WebUI)
  hooks.php                        # WHMCS hooks (product page customization)
  logo.png                         # Module logo
```

### WHMCS Module Functions

| Function | Purpose | Controller API Call |
|----------|---------|-------------------|
| `virtuestack_CreateAccount` | Provision new VM | `POST /api/v1/provisioning/vms` → Returns HTTP 202 + task_id |
| `virtuestack_SuspendAccount` | Suspend VM (stop + block) | `POST /api/v1/provisioning/vms/{id}/suspend` |
| `virtuestack_UnsuspendAccount` | Unsuspend VM | `POST /api/v1/provisioning/vms/{id}/unsuspend` |
| `virtuestack_TerminateAccount` | Delete VM + release IPs | `DELETE /api/v1/provisioning/vms/{id}` |
| `virtuestack_ChangePackage` | Resize VM resources | `PATCH /api/v1/provisioning/vms/{id}/resize` |
| `virtuestack_ChangePassword` | Reset root password | `POST /api/v1/provisioning/vms/{id}/password` |

### Async Provisioning Flow (WHMCS)

WHMCS expects synchronous responses, but VM creation is async (5-60s). Solution:

```
1. WHMCS calls CreateAccount → Controller returns HTTP 202 + { task_id: "..." }
2. WHMCS module stores task_id in custom field
3. WHMCS module immediately returns "success" to WHMCS (VM marked as "pending")
4. Controller completes VM creation → sends webhook to WHMCS callback URL
5. WHMCS hook receives webhook → updates product status to "active"
6. WHMCS hook stores VM credentials (IP, password) in custom fields

Alternative (polling): WHMCS cron job polls GET /api/v1/provisioning/tasks/{task_id}
until status is "completed" or "failed"
```

### Customer Credentials

On first VM order for a customer:
1. Controller creates Customer API account with auto-generated credentials
2. Returns `customer_api_id` + `customer_api_secret` to WHMCS
3. WHMCS stores encrypted credentials using `encrypt()`/`decrypt()`
4. Subsequent VM orders for same customer reuse existing Customer API credentials

### WHMCS Client Area Integration

The WHMCS module provides a custom client area page that embeds the Customer WebUI:

```html
<!-- templates/overview.tpl -->
<iframe
  src="https://panel.{domain}/customer/vm/{$vm_id}?token={$session_token}"
  style="width: 100%; height: 80vh; border: none; border-radius: 8px;"
  allow="clipboard-read; clipboard-write"
></iframe>
```

The `session_token` is a short-lived JWT generated by WHMCS module from the stored Customer API credentials, allowing seamless SSO without dual login.

---

## 8. NETWORKING ARCHITECTURE

### Network Topology (Linux Bridge — Default)

Each node has one or more physical NICs bonded into a Linux bridge:

```
Physical NIC(s) → bond0 (802.3ad LACP) → br0 (Linux Bridge)
                                            ├── VM tap interfaces (vnet0, vnet1, ...)
                                            └── Node management IP
```

### Bridge Configuration (Netplan)

```yaml
# /etc/netplan/01-virtuestack.yaml
network:
  version: 2
  renderer: networkd
  ethernets:
    ens3f0:
      dhcp4: false
    ens3f1:
      dhcp4: false
  bonds:
    bond0:
      interfaces: [ens3f0, ens3f1]
      parameters:
        mode: 802.3ad
        lacp-rate: fast
        mii-monitor-interval: 100
  bridges:
    br0:
      interfaces: [bond0]
      addresses:
        - 192.0.2.1/24
        - "2001:db8:abcd::1/48"
      routes:
        - to: 0.0.0.0/0
          via: 192.0.2.254
        - to: "::/0"
          via: "2001:db8:abcd::ffff"
      nameservers:
        addresses: [1.1.1.1, 8.8.8.8]
      parameters:
        stp: false
        forward-delay: 0
```

### IPv6 Allocation Scheme

```
Provider-Allocated /48: 2001:db8:abcd::/48 (shared across ALL nodes in a location)

Routing Options:
A) L2 Bridged (Default): The /48 is routed to the L2 VLAN. The upstream router handles radvd/SLAAC. VMs keep IPs when migrating across the VLAN.
B) L3 Routed (BGP): Each /64 is routed by upstream router via BGP (e.g. BIRD/FRR) to the specific node. On migration, the new node announces the /64 and the old node withdraws it.
```

**Routing & Live Migration**:
Because VMs can live-migrate between nodes, IP prefixes MUST NOT be permanently tied to specific physical nodes. 
- VMs use static IPv6 configuration via cloud-init.
- During live migration in an L3 setup, the Controller coordinates BGP route withdrawal on the source and announcement on the destination.

### IP Anti-Spoofing (libvirt nwfilter)

Custom nwfilter chain applied to every VM interface:

```xml
<!-- /etc/libvirt/nwfilter/virtuestack-clean-traffic.xml -->
<filter name='virtuestack-clean-traffic' chain='root'>
  <!-- Prevent MAC spoofing -->
  <filterref filter='no-mac-spoofing'/>

  <!-- Prevent IPv4 spoofing -->
  <rule action='drop' direction='out' priority='400'>
    <ip match='no' srcipaddr='$IP'/>
  </rule>
  <rule action='accept' direction='out' priority='500'>
    <ip srcipaddr='$IP'/>
  </rule>

  <!-- Prevent ARP spoofing -->
  <rule action='drop' direction='out' priority='400'>
    <arp match='no' arpsrcmacaddr='$MAC'/>
  </rule>
  <rule action='drop' direction='out' priority='400'>
    <arp match='no' arpsrcipaddr='$IP'/>
  </rule>

  <!-- Prevent IPv6 spoofing -->
  <rule action='drop' direction='out' priority='400'>
    <ipv6 match='no' srcipaddr='$IPV6'/>
  </rule>

  <!-- Block outbound Router Advertisements (RA spoofing) -->
  <rule action='drop' direction='out' priority='350'>
    <icmpv6 type='134'/>
  </rule>

  <!-- Block outbound Redirect messages -->
  <rule action='drop' direction='out' priority='350'>
    <icmpv6 type='137'/>
  </rule>

  <!-- Allow all other traffic (already validated) -->
  <rule action='accept' direction='inout' priority='1000'>
    <all/>
  </rule>
</filter>
```

### DHCP Server Per Node

Each node runs `dnsmasq` for VMs that expect DHCP (e.g., reinstalled without cloud-init, or customer-configured):

```ini
# /etc/dnsmasq.d/virtuestack.conf
interface=br0
bind-interfaces
no-resolv
server=1.1.1.1
server=8.8.8.8

# Only serve known MAC addresses
dhcp-range=192.0.2.100,192.0.2.200,255.255.255.0,12h

# Static leases managed by Node Agent (auto-generated)
# dhcp-host=52:54:00:aa:bb:cc,192.0.2.50,vm001
# (Node Agent dynamically writes /etc/dnsmasq.d/vs-leases.conf and sends SIGHUP)
```

The Node Agent manages DHCP leases by:
1. Writing MAC→IP mappings to `/etc/dnsmasq.d/vs-leases.conf`
2. Sending `SIGHUP` to dnsmasq to reload without restart
3. Only mapping IPs that are assigned to VMs on this node

### Abuse Prevention (Default Rules)

| Rule | Implementation | Rationale |
|------|---------------|-----------|
| Block outbound SMTP (port 25) | nftables rule on node | Prevent spam abuse |
| Block outbound to 169.254.169.254 | nftables rule on node | Prevent metadata endpoint spoofing |
| Rate limit outbound DNS | nftables limit | Prevent DNS amplification |
| CPU pinning (optional) | libvirt `<cputune>` | Prevent noisy-neighbor CPU abuse |
| Disk I/O limits | libvirt `<blkiotune>` | Prevent storage I/O abuse |

---

## 9. STORAGE ARCHITECTURE (DUAL BACKEND)

VirtueStack supports two storage backends via a common `StorageBackend` interface. The backend is configured per-node and per-plan, allowing mixed deployments (e.g., Ceph for production nodes, QCOW2 for dev/testing).

### Backend Comparison

| Aspect | Ceph RBD | QCOW2 (File-based) |
|--------|----------|---------------------|
| **Storage type** | Shared, network-attached | Local, per-node |
| **Library** | `github.com/ceph/go-ceph` | `qemu-img` CLI |
| **Live migration** | Native (shared storage) | Disk transfer via gRPC stream |
| **Snapshot protection** | Native (`rbd snap protect`) | Not supported (no-op with error) |
| **Pool stats** | Ceph cluster `df` JSON | `syscall.Statfs` on local filesystem |
| **HA failover** | RBD lock release + blocklist | Disk copy to target node |
| **Scaling** | Scales with Ceph OSD nodes | Limited by local disk capacity |
| **Use case** | Production, multi-node clusters | Single-node or dev/test setups |

### Storage Backend Interface

**File:** `internal/nodeagent/storage/interface.go`

```go
type StorageBackend interface {
    CloneFromTemplate(ctx, sourcePool, sourceImage, sourceSnap, targetImage string) error
    CloneSnapshotToPool(ctx, sourcePool, sourceImage, sourceSnap, targetPool, targetImage string) error
    Resize(ctx, imageName string, newSizeGB int) error
    Delete(ctx, imageName string) error
    CreateSnapshot(ctx, imageName, snapshotName string) error
    DeleteSnapshot(ctx, imageName, snapshotName string) error
    ProtectSnapshot(ctx, imageName, snapshotName string) error
    UnprotectSnapshot(ctx, imageName, snapshotName string) error
    ListSnapshots(ctx, imageName string) ([]SnapshotInfo, error)
    GetImageSize(ctx, imageName string) (int64, error)
    ImageExists(ctx, imageName string) (bool, error)
    FlattenImage(ctx, imageName string) error
    GetPoolStats(ctx context.Context) (*PoolStats, error)
    Rollback(ctx, imageName, snapshotName string) error
    GetStorageType() StorageType
}

type StorageType string
const (
    StorageTypeCEPH StorageType = "ceph"
    StorageTypeQCOW StorageType = "qcow"
)
```

### Backend Selection

A VM's storage backend is determined by its **plan** at creation time and is **immutable** — it cannot be changed after the VM is created. Each node can host VMs with **any** storage backend, as long as the node has the necessary configuration (Ceph credentials for Ceph VMs, `storage_path` for QCOW2 VMs).

| Level | Field | Description |
|-------|-------|-------------|
| **Plan** | `plans.storage_backend` | Defines the default backend for VMs created under this plan (`ceph` or `qcow`) |
| **VM** | `vms.storage_backend` | Set from plan at creation time. **Immutable** — cannot be changed or migrated to a different backend |
| **Node** | `nodes.storage_backend` / `nodes.storage_path` | Describes what the node is configured for. A node can have both Ceph and QCOW2 configured simultaneously |

**Key rules:**
1. VM storage backend is inherited from the plan at creation and **cannot be changed**
2. Nodes can host VMs with any backend (Ceph, QCOW2, or both) — no filtering on node selection
3. Migration is only allowed between nodes that support the VM's storage backend
4. Cross-backend migration (e.g., Ceph to QCOW2 or vice versa) is **not supported**

### Configuration (Database Schema)

**Migration:** `migrations/000019_add_storage_backend.up.sql`

```sql
-- Storage backend per plan
ALTER TABLE plans ADD COLUMN storage_backend VARCHAR(20) DEFAULT 'ceph';

-- Storage backend per node (for local QCOW storage)
ALTER TABLE nodes ADD COLUMN storage_backend VARCHAR(20) DEFAULT 'ceph';
ALTER TABLE nodes ADD COLUMN storage_path TEXT;

-- Storage backend per VM (inherits from plan)
ALTER TABLE vms ADD COLUMN storage_backend VARCHAR(20) DEFAULT 'ceph';
ALTER TABLE vms ADD COLUMN disk_path TEXT;  -- For QCOW file path
```

---

### 9.1 Ceph RBD Backend

**File:** `internal/nodeagent/storage/rbd.go`

#### Ceph Pool Design

| Pool | Purpose | Settings |
|------|---------|----------|
| `vs-vms` | Active VM disks | `rbd_default_features = layering,exclusive-lock,object-map` |
| `vs-images` | OS template base images | Protected snapshots, read-only by Node Agents |
| `vs-backups` | Backup exports (optional) | Lower replication factor acceptable |

#### Ceph User & Permissions

```bash
ceph auth get-or-create client.virtuestack \
  mon 'profile rbd' \
  osd 'profile rbd pool=vs-vms, profile rbd pool=vs-images, profile rbd pool=vs-backups' \
  -o /etc/ceph/ceph.client.virtuestack.keyring
```

#### RBD Naming Convention

```
vs-vms/vs-{vm_uuid}-disk0          # Primary disk
vs-vms/vs-{vm_uuid}-disk1          # Additional disk (if plan allows)
vs-images/{os_name}-{version}-base  # Template base image
```

#### Template Creation Workflow (Ceph)

```
1. Admin uploads cloud image: ubuntu-24.04-minimal-cloudimg-amd64.img
2. Controller sends to a Node Agent
3. Node Agent: qemu-img convert -f qcow2 -O raw image.img /tmp/raw.img
4. Node Agent: rbd import /tmp/raw.img vs-images/ubuntu-24.04-base
5. Node Agent: rbd snap create vs-images/ubuntu-24.04-base@base
6. Node Agent: rbd snap protect vs-images/ubuntu-24.04-base@base
7. Controller stores template metadata in PostgreSQL
```

#### VM Provisioning Flow (Ceph)

1. **RBD clone**: `rbd clone vs-images/ubuntu-24.04-base@base vs-vms/vs-{uuid}-disk0` (instant, copy-on-write)
2. **Resize if needed**: `rbd resize vs-vms/vs-{uuid}-disk0 --size {disk_gb}G`
3. **Generate cloud-init ISO** (NoCloud datasource)
4. **Define + start VM** via libvirt with domain XML pointing to RBD + cloud-init ISO
5. **VM boots** → cloud-init configures networking, credentials, hostname

#### RBD Exclusive Locks & Live Migration

- All VM disks have `exclusive-lock` feature enabled
- Only one libvirt instance can mount the RBD image at a time
- During live migration: source libvirt transfers lock to destination automatically
- During HA failover: lock must be forcibly released (see Section 14)

---

### 9.2 QCOW2 File-Based Backend

**File:** `internal/nodeagent/storage/qcow.go`

The QCOW2 backend stores VM disk images as local files on each node's filesystem. It uses `qemu-img` for all disk operations. This is ideal for single-node deployments or environments without a Ceph cluster.

#### Directory Layout

```
/var/lib/virtuestack/
├── templates/                    # OS template images (backing files)
│   ├── ubuntu-24.04-base.qcow2
│   ├── debian-12-base.qcow2
│   └── centos-9-base.qcow2
├── vms/                          # VM disk images
│   ├── vs-{vm_uuid}-disk0.qcow2
│   └── vs-{vm_uuid}-disk1.qcow2
├── cloud-init/                   # Cloud-init seed ISOs
│   └── vs-{vm_uuid}-seed.iso
└── iso/                          # Customer ISO uploads
    └── {customer_id}/
```

#### Template Storage

Templates are stored as standalone QCOW2 files in `/var/lib/virtuestack/templates/`. Each template serves as a **backing file** for new VM disks, enabling instant copy-on-write cloning:

```
Template: /var/lib/virtuestack/templates/ubuntu-24.04-base.qcow2 (read-only backing file)
    ↓ qemu-img create -b ... -F qcow2 -f qcow2
VM disk:  /var/lib/virtuestack/vms/vs-{uuid}-disk0.qcow2 (COW overlay)
```

#### VM Provisioning Flow (QCOW2)

1. **Template clone**: `qemu-img create -b /var/lib/virtuestack/templates/ubuntu-24.04-base.qcow2 -F qcow2 -f qcow2 /var/lib/virtuestack/vms/vs-{uuid}-disk0.qcow2` (instant, copy-on-write)
2. **Resize if needed**: `qemu-img resize /var/lib/virtuestack/vms/vs-{uuid}-disk0.qcow2 {disk_gb}G`
3. **Generate cloud-init ISO** (same as Ceph backend)
4. **Define + start VM** via libvirt with domain XML pointing to the QCOW2 file
5. **VM boots** → cloud-init configures networking, credentials, hostname

#### Snapshot Operations

QCOW2 supports internal snapshots via `qemu-img snapshot`:

| Operation | Command |
|-----------|---------|
| Create | `qemu-img snapshot -c {snap_name} {image_path}` |
| Delete | `qemu-img snapshot -d {snap_name} {image_path}` |
| List | `qemu-img snapshot -l {image_path}` |
| Revert | `qemu-img snapshot -a {snap_name} {image_path}` |

**Limitation**: QCOW2 does not support native snapshot protection. `ProtectSnapshot()` and `UnprotectSnapshot()` return an error. Use external tracking (e.g., database flags) to prevent accidental deletion of important snapshots.

#### Live Migration (QCOW2)

Unlike Ceph RBD (shared storage), QCOW2 migration requires disk transfer between nodes. The Controller orchestrates this via gRPC streaming:

```
1. Source Node: Create internal snapshot (freeze disk state)
2. Source Node: Stream disk via gRPC (TransferDisk RPC)
3. Target Node: Receive disk via gRPC (ReceiveDisk RPC)
4. Target Node: Define VM with received disk
5. Controller: Live migrate VM memory state (libvirt live migration)
6. Source Node: Clean up snapshot and disk
```

**Note**: The gRPC proto includes dedicated disk transfer RPCs:
- `CreateDiskSnapshot` — freeze disk for consistent transfer
- `TransferDisk` — server-streaming RPC to send disk chunks
- `ReceiveDisk` — client-streaming RPC to receive disk chunks
- `DeleteDiskSnapshot` — clean up temporary snapshot

**Important**: A QCOW2 VM can only be migrated to another node with QCOW2 support. Cross-backend migration (Ceph ↔ QCOW2) is not allowed.

#### HA Failover (QCOW2)

Since VM disks are local to each node, HA failover for QCOW2 is more complex:

1. The failed node's disks are **not accessible** from other nodes
2. Failover is only possible if VMs were previously replicated/backed up to shared storage
3. Recovery requires restoring from the latest backup on a surviving node
4. This is why Ceph RBD is recommended for production HA setups

#### Pool Statistics

The QCOW2 backend reports storage capacity via `syscall.Statfs` on the base directory:

```go
var stat syscall.Statfs_t
syscall.Statfs("/var/lib/virtuestack/vms", &stat)
total := int64(stat.Blocks) * int64(stat.Bsize)
free  := int64(stat.Bavail) * int64(stat.Bsize)
```

---

## 10. SECURITY ARCHITECTURE

### Authentication Flows

**Customer Login:**
```
1. POST /api/v1/customer/auth/login { email, password }
2. Server validates credentials (Argon2id hash comparison)
3. If 2FA enabled: return { requires_2fa: true, temp_token: "..." }
4. POST /api/v1/customer/auth/verify-2fa { temp_token, totp_code }
5. Server validates TOTP code (±1 step tolerance)
6. Return: { access_token (15min), refresh_token (7d, httpOnly cookie) }
```

**Admin Login:**
```
1. POST /api/v1/admin/auth/login { email, password }
2. Server validates credentials (Argon2id hash comparison)
3. 2FA ALWAYS required for admin: return { requires_2fa: true, temp_token: "..." }
4. POST /api/v1/admin/auth/verify-2fa { temp_token, totp_code }
5. Return: { access_token (15min), refresh_token (4h, httpOnly cookie) }
6. Concurrent session limit: max 3 active admin sessions
```

**Destructive Operation Re-auth (Admin):**
```
POST /api/v1/admin/nodes/{id}/failover
  Headers: X-Reauth-Token: {fresh_totp_code}
  → Server re-validates TOTP before executing
```

### CSRF Protection

- All APIs are same-origin with `SameSite=Strict` cookies
- WebUI uses `X-Correlation-Id` header for request tracing
- If cross-origin needed in future: double-submit cookie pattern

### Rate Limiting (Sliding Window)

| Endpoint Category | Rate | Per |
|-------------------|------|-----|
| Login attempts | 5 per 15min | IP |
| Provisioning API | 1000 per min | API Key |
| Customer API (read) | 100 per min | User |
| Customer API (write) | 30 per min | User |
| Admin API | 500 per min | User |
| rDNS changes | 10 per hour | Customer |
| Console token requests | 20 per min | Customer |
| ISO uploads | 3 concurrent | Customer |

### mTLS (Controller ↔ Node Agent)

- Controller generates CA cert + per-node client/server certificates
- Node Agent validates Controller's client cert before accepting gRPC calls
- Certificate rotation: yearly, documented in INSTALL.md

---

## 11. DATABASE SCHEMA

### Core Tables

```sql
-- Customers
CREATE TABLE customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,  -- Argon2id
    name VARCHAR(255) NOT NULL,
    whmcs_client_id INTEGER UNIQUE,
    totp_secret_encrypted TEXT,  -- AES-256-GCM encrypted
    totp_enabled BOOLEAN DEFAULT FALSE,
    totp_backup_codes_hash TEXT[],  -- SHA-256 hashed backup codes
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Admin users
CREATE TABLE admins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name VARCHAR(255) NOT NULL,
    totp_secret_encrypted TEXT NOT NULL,  -- 2FA mandatory for admins
    totp_enabled BOOLEAN DEFAULT FALSE,
    totp_backup_codes_hash TEXT[],
    role VARCHAR(20) DEFAULT 'admin' CHECK (role IN ('admin', 'super_admin')),
    max_sessions INTEGER DEFAULT 3,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Nodes
CREATE TABLE nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname VARCHAR(255) NOT NULL UNIQUE,
    grpc_address VARCHAR(255) NOT NULL,  -- host:port for gRPC
    management_ip INET NOT NULL,
    location_id UUID REFERENCES locations(id),
    status VARCHAR(20) DEFAULT 'offline'
      CHECK (status IN ('online', 'degraded', 'offline', 'draining', 'failed')),
    storage_backend VARCHAR(20) DEFAULT 'ceph'
      CHECK (storage_backend IN ('ceph', 'qcow')),
    storage_path TEXT,  -- Base path for QCOW2 storage (e.g., /var/lib/virtuestack/vms)
    total_vcpu INTEGER NOT NULL,
    total_memory_mb INTEGER NOT NULL,
    allocated_vcpu INTEGER DEFAULT 0,
    allocated_memory_mb INTEGER DEFAULT 0,
    ceph_pool VARCHAR(100) DEFAULT 'vs-vms',
    ipmi_address INET,
    ipmi_username_encrypted TEXT,  -- AES-256-GCM
    ipmi_password_encrypted TEXT,  -- AES-256-GCM
    last_heartbeat_at TIMESTAMPTZ,
    consecutive_heartbeat_misses INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Locations (data centers / network zones)
CREATE TABLE locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    region VARCHAR(100),
    country VARCHAR(2),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- VM Plans (product templates)
CREATE TABLE plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    vcpu INTEGER NOT NULL,
    memory_mb INTEGER NOT NULL,
    disk_gb INTEGER NOT NULL,
    port_speed_mbps INTEGER NOT NULL,
    bandwidth_limit_gb INTEGER DEFAULT 0,  -- 0 = unlimited
    bandwidth_overage_speed_mbps INTEGER DEFAULT 5,
    max_ipv4 INTEGER DEFAULT 1,
    max_ipv6_slash64 INTEGER DEFAULT 1,
    max_snapshots INTEGER DEFAULT 3,
    max_backups INTEGER DEFAULT 1,
    max_iso_count INTEGER DEFAULT 1,
    max_iso_gb INTEGER DEFAULT 5,
    storage_backend VARCHAR(20) DEFAULT 'ceph'
      CHECK (storage_backend IN ('ceph', 'qcow')),
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    snapshot_limit INTEGER NOT NULL DEFAULT 2,
    backup_limit INTEGER NOT NULL DEFAULT 2,
    iso_upload_limit INTEGER NOT NULL DEFAULT 2,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Virtual Machines
CREATE TABLE vms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id),
    node_id UUID REFERENCES nodes(id),
    plan_id UUID NOT NULL REFERENCES plans(id),
    hostname VARCHAR(63) NOT NULL,
    status VARCHAR(20) DEFAULT 'provisioning'
      CHECK (status IN (
        'provisioning', 'running', 'stopped', 'suspended',
        'migrating', 'reinstalling', 'error', 'deleted'
      )),
    vcpu INTEGER NOT NULL,
    memory_mb INTEGER NOT NULL,
    disk_gb INTEGER NOT NULL,
    port_speed_mbps INTEGER NOT NULL,
    bandwidth_limit_gb INTEGER DEFAULT 0,
    bandwidth_used_bytes BIGINT DEFAULT 0,
    bandwidth_reset_at TIMESTAMPTZ DEFAULT NOW(),
    storage_backend VARCHAR(20) DEFAULT 'ceph'
      CHECK (storage_backend IN ('ceph', 'qcow')),
    disk_path TEXT,  -- File path for QCOW2 VM disks (e.g., /var/lib/virtuestack/vms/vs-{uuid}-disk0.qcow2)
    mac_address MACADDR NOT NULL UNIQUE,
    template_id UUID REFERENCES templates(id),
    libvirt_domain_name VARCHAR(100),
    root_password_encrypted TEXT,  -- AES-256-GCM (for initial provision only)
    whmcs_service_id INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Enable RLS for multi-tenant isolation
ALTER TABLE vms ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_vms ON vms FOR ALL TO app_customer
  USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- IP Sets (pools of addresses per location)
CREATE TABLE ip_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    location_id UUID REFERENCES locations(id),
    network CIDR NOT NULL,
    gateway INET NOT NULL,
    vlan_id INTEGER,
    ip_version SMALLINT CHECK (ip_version IN (4, 6)),
    node_ids UUID[],  -- Nodes this IP set can be activated on
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- IP Addresses
CREATE TABLE ip_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_set_id UUID NOT NULL REFERENCES ip_sets(id),
    address INET NOT NULL UNIQUE,
    ip_version SMALLINT CHECK (ip_version IN (4, 6)),
    vm_id UUID REFERENCES vms(id),
    customer_id UUID REFERENCES customers(id),
    is_primary BOOLEAN DEFAULT FALSE,
    rdns_hostname VARCHAR(255),
    status VARCHAR(20) DEFAULT 'available'
      CHECK (status IN ('available', 'assigned', 'reserved', 'cooldown')),
    assigned_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    cooldown_until TIMESTAMPTZ,  -- Prevent re-assignment of blacklisted IPs
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- IPv6 Prefix Allocations
CREATE TABLE ipv6_prefixes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES nodes(id),
    prefix CIDR NOT NULL,  -- /48 assigned to node
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE vm_ipv6_subnets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id),
    ipv6_prefix_id UUID NOT NULL REFERENCES ipv6_prefixes(id),
    subnet CIDR NOT NULL,  -- /64 assigned to VM
    subnet_index INTEGER NOT NULL,  -- 0-65535 within the /48
    gateway INET NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- OS Templates
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    os_family VARCHAR(50) NOT NULL,  -- 'ubuntu', 'debian', 'centos', 'windows'
    os_version VARCHAR(20) NOT NULL,
    rbd_image VARCHAR(200) NOT NULL,  -- 'vs-images/ubuntu-24.04-base'
    rbd_snapshot VARCHAR(100) NOT NULL,  -- '@base'
    min_disk_gb INTEGER DEFAULT 10,
    supports_cloudinit BOOLEAN DEFAULT TRUE,
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Async Tasks
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,  -- 'vm.create', 'vm.reinstall', 'backup.create', 'vm.migrate'
    status VARCHAR(20) DEFAULT 'pending'
      CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    payload JSONB NOT NULL,
    result JSONB,
    error_message TEXT,
    progress INTEGER DEFAULT 0,  -- 0-100
    idempotency_key VARCHAR(100) UNIQUE,
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

-- Backups
CREATE TABLE backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id),
    type VARCHAR(20) CHECK (type IN ('full', 'incremental')),
    rbd_snapshot VARCHAR(100),
    diff_from_snapshot VARCHAR(100),  -- For incremental: base snapshot
    storage_path TEXT,  -- Path on mounted backup volume or FTP
    size_bytes BIGINT,
    status VARCHAR(20) DEFAULT 'creating'
      CHECK (status IN ('creating', 'completed', 'failed', 'restoring', 'deleted')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

-- Snapshots (customer-managed, in-place)
CREATE TABLE snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id),
    name VARCHAR(100) NOT NULL,
    rbd_snapshot VARCHAR(100) NOT NULL,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Provisioning API Keys
CREATE TABLE provisioning_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    key_hash TEXT NOT NULL,  -- SHA-256 of the API key
    allowed_ips INET[],
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

-- Customer API Keys (per-VM scoped)
CREATE TABLE customer_api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id),
    name VARCHAR(100) NOT NULL,
    key_hash TEXT NOT NULL,
    vm_ids UUID[],  -- Which VMs this key can access (empty = all)
    permissions TEXT[],  -- Subset of customer permissions
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

-- Customer Webhooks
CREATE TABLE customer_webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id),
    url VARCHAR(2048) NOT NULL,
    secret TEXT NOT NULL,  -- For HMAC signature
    events TEXT[] NOT NULL,  -- ['vm.started', 'vm.stopped', 'backup.completed']
    is_active BOOLEAN DEFAULT TRUE,
    last_triggered_at TIMESTAMPTZ,
    consecutive_failures INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Webhook Delivery Log
CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES customer_webhooks(id),
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    response_code INTEGER,
    response_body TEXT,
    attempt INTEGER DEFAULT 1,
    status VARCHAR(20) DEFAULT 'pending'
      CHECK (status IN ('pending', 'delivered', 'failed', 'retrying')),
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Audit Logs (append-only)
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_id UUID,
    actor_type VARCHAR(20) NOT NULL,
    actor_ip INET,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    changes JSONB,
    correlation_id UUID,
    success BOOLEAN DEFAULT TRUE,
    error_message TEXT
) PARTITION BY RANGE (timestamp);

-- Partition audit logs by month for performance
CREATE TABLE audit_logs_2026_03 PARTITION OF audit_logs
  FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

-- Revoke destructive operations on audit log
REVOKE UPDATE, DELETE ON audit_logs FROM app_user;

-- Node heartbeats
CREATE TABLE node_heartbeats (
    id BIGSERIAL PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cpu_percent REAL,
    memory_percent REAL,
    disk_percent REAL,
    vm_count INTEGER,
    load_average REAL[]
);

-- System settings (key-value with JSONB)
CREATE TABLE system_settings (
    key VARCHAR(255) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by UUID
);

-- Sessions (for concurrent session tracking)
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    user_type VARCHAR(20) NOT NULL,  -- 'admin', 'customer'
    refresh_token_hash TEXT NOT NULL,
    ip_address INET,
    user_agent TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## 12. API SPECIFICATIONS

### Provisioning API (WHMCS)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/provisioning/vms` | Create VM (returns task_id, HTTP 202) |
| DELETE | `/api/v1/provisioning/vms/{id}` | Delete VM |
| POST | `/api/v1/provisioning/vms/{id}/suspend` | Suspend VM |
| POST | `/api/v1/provisioning/vms/{id}/unsuspend` | Unsuspend VM |
| PATCH | `/api/v1/provisioning/vms/{id}/resize` | Resize VM resources |
| POST | `/api/v1/provisioning/vms/{id}/password` | Reset root password |
| GET | `/api/v1/provisioning/tasks/{id}` | Poll task status |
| GET | `/api/v1/provisioning/vms/{id}` | Get VM status + IPs |

### Customer API

> **Security:** VM creation and deletion are restricted to Admin and Provisioning APIs only.
> Customers cannot create or delete VMs through the Customer API to prevent abuse
> (e.g., a customer buying one VPS then creating additional VMs for free).
> All endpoints enforce tenant isolation — customers can only access their own resources.
> Plan-level limits (snapshot_limit, backup_limit, iso_upload_limit) are enforced
> on create operations, returning `409 Conflict` when exceeded.

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/customer/auth/login` | Login |
| POST | `/api/v1/customer/auth/verify-2fa` | Verify TOTP code |
| POST | `/api/v1/customer/auth/refresh` | Refresh access token |
| POST | `/api/v1/customer/auth/logout` | Logout (invalidate session) |
| GET | `/api/v1/customer/profile` | Get profile |
| PUT | `/api/v1/customer/profile/2fa` | Enable/disable 2FA |
| GET | `/api/v1/customer/vms` | List own VMs |
| GET | `/api/v1/customer/vms/{id}` | Get VM details |
| POST | `/api/v1/customer/vms/{id}/start` | Start VM |
| POST | `/api/v1/customer/vms/{id}/stop` | Graceful shutdown |
| POST | `/api/v1/customer/vms/{id}/force-stop` | Force power off |
| POST | `/api/v1/customer/vms/{id}/restart` | Restart VM |
| POST | `/api/v1/customer/vms/{id}/reinstall` | Reinstall from template |
| POST | `/api/v1/customer/vms/{id}/console-token` | Get one-time console token |
| GET | `/api/v1/customer/vms/{id}/metrics` | Get resource metrics |
| GET | `/api/v1/customer/vms/{id}/bandwidth` | Get bandwidth usage |
| PUT | `/api/v1/customer/vms/{id}/boot-order` | Change boot order |
| PUT | `/api/v1/customer/vms/{id}/rdns` | Set rDNS hostname |
| GET | `/api/v1/customer/vms/{id}/backups` | List backups |
| POST | `/api/v1/customer/vms/{id}/backups/{bid}/restore` | Restore from backup |
| GET | `/api/v1/customer/vms/{id}/snapshots` | List snapshots |
| POST | `/api/v1/customer/vms/{id}/snapshots` | Create snapshot |
| POST | `/api/v1/customer/vms/{id}/snapshots/{sid}/revert` | Revert to snapshot |
| DELETE | `/api/v1/customer/vms/{id}/snapshots/{sid}` | Delete snapshot |
| GET | `/api/v1/customer/vms/{id}/iso` | List attached ISOs |
| POST | `/api/v1/customer/vms/{id}/iso/upload` | Upload ISO (tus) |
| POST | `/api/v1/customer/vms/{id}/iso/{iid}/attach` | Attach ISO |
| POST | `/api/v1/customer/vms/{id}/iso/{iid}/detach` | Detach ISO |
| DELETE | `/api/v1/customer/vms/{id}/iso/{iid}` | Delete ISO |
| GET | `/api/v1/customer/api-keys` | List API keys |
| POST | `/api/v1/customer/api-keys` | Create API key |
| DELETE | `/api/v1/customer/api-keys/{id}` | Revoke API key |
| GET | `/api/v1/customer/webhooks` | List webhooks |
| POST | `/api/v1/customer/webhooks` | Create webhook |
| DELETE | `/api/v1/customer/webhooks/{id}` | Delete webhook |
| GET | `/api/v1/customer/templates` | List available templates |

### Admin API

Includes all Customer API endpoints (for any customer's VMs) plus:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/admin/nodes` | List all nodes |
| POST | `/api/v1/admin/nodes` | Register new node |
| GET | `/api/v1/admin/nodes/{id}` | Get node details |
| PUT | `/api/v1/admin/nodes/{id}` | Update node config |
| POST | `/api/v1/admin/nodes/{id}/drain` | Drain node (stop assigning) |
| POST | `/api/v1/admin/nodes/{id}/failover` | Trigger HA failover (re-auth required) |
| GET | `/api/v1/admin/vms` | List all VMs (paginated) |
| POST | `/api/v1/admin/vms` | Create VM manually |
| DELETE | `/api/v1/admin/vms/{id}` | Delete VM |
| PATCH | `/api/v1/admin/vms/{id}/resize` | Resize VM |
| POST | `/api/v1/admin/vms/{id}/migrate` | Live migrate VM |
| GET | `/api/v1/admin/plans` | List plans |
| POST | `/api/v1/admin/plans` | Create plan |
| PUT | `/api/v1/admin/plans/{id}` | Update plan |
| GET | `/api/v1/admin/templates` | List templates |
| POST | `/api/v1/admin/templates` | Create template |
| PUT | `/api/v1/admin/templates/{id}` | Update template |
| GET | `/api/v1/admin/ip-sets` | List IP sets |
| POST | `/api/v1/admin/ip-sets` | Create IP set |
| POST | `/api/v1/admin/ip-sets/{id}/import` | Bulk import IPs |
| GET | `/api/v1/admin/customers` | List customers |
| GET | `/api/v1/admin/audit-logs` | Query audit logs |
| GET | `/api/v1/admin/settings` | Get system settings |
| PUT | `/api/v1/admin/settings` | Update system settings (re-auth required) |
| POST | `/api/v1/admin/backups/trigger` | Trigger manual backup |
| GET | `/api/v1/admin/backups/schedule` | Get backup schedule |
| PUT | `/api/v1/admin/backups/schedule` | Update backup schedule |

### Standard API Response Format

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

**Error (per MASTER_CODING_STANDARD Section 6):**
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

### Idempotency

All create/mutate endpoints accept `Idempotency-Key` header. Controller stores results for 24 hours. Replay returns cached response.

---

## 13. ASYNC TASK SYSTEM

### Architecture

```
API Handler
    ↓ publish
NATS JetStream (durable stream: "TASKS")
    ↓ subscribe
Task Worker Pool (5 workers per Controller instance)
    ↓ execute
Node Agent (gRPC)
    ↓ result
Update PostgreSQL task status + notify WebSocket subscribers
```

### Task Types

| Task | Steps | Typical Duration |
|------|-------|-----------------|
| `vm.create` | RBD clone → resize → cloud-init → define XML → start | 10-30s |
| `vm.reinstall` | Stop → delete RBD → RBD clone → cloud-init → start | 15-45s |
| `vm.migrate` | Pre-check → live migrate → verify → update state | 30-300s |
| `vm.resize` | Stop (if needed) → RBD resize → update XML → start | 5-30s |
| `backup.create` | Freeze FS → RBD snapshot → export → thaw FS → transfer offsite | 60-3600s |
| `backup.restore` | Stop VM → RBD rollback → start VM | 10-60s |

### State Machine

```
pending → running → completed
                  → failed (with error_message)
pending → cancelled
```

Each step within a task is recorded in the `tasks.payload` JSONB field as an operation journal:

```json
{
  "steps": [
    { "name": "rbd_clone", "status": "completed", "started_at": "...", "completed_at": "..." },
    { "name": "rbd_resize", "status": "completed", "started_at": "...", "completed_at": "..." },
    { "name": "cloudinit_generate", "status": "running", "started_at": "..." },
    { "name": "libvirt_define", "status": "pending" },
    { "name": "libvirt_start", "status": "pending" }
  ]
}
```

On failure: Controller can resume from last completed step (idempotent replay).

### Task Idempotency & Timeouts
- **Idempotency**: All task handlers must explicitly check current system state before executing actions. For example, if a `vm.create` task retries after a crash, it must check if the RBD image already exists before attempting `rbd clone` to avoid "File exists" errors.
- **Timeouts**: QEMU guest agent commands (especially `guest-fsfreeze-freeze` during backups) can hang indefinitely if the guest OS is compromised or unresponsive. All guest agent calls MUST enforce strict context timeouts (e.g., 10 seconds). If freeze fails, the backup should fallback to a crash-consistent snapshot.

---

## 14. HA FAILOVER & STONITH/FENCING

### Node Health Monitoring

The Controller checks each Node Agent's heartbeat every **60 seconds** via gRPC `Ping`:

```
Normal: Node responds within 5s → consecutive_misses = 0
Missed: No response within 10s → consecutive_misses += 1
```

### Failure Detection

When `consecutive_heartbeat_misses >= 3` (3 minutes of no response):

1. **Mark node as `failed`** in database
2. **Stop assigning new VMs** to this node
3. **DO NOT automatically migrate VMs** — wait for admin
4. **Send alert**: Email + Telegram notification to all admins
5. **Create failover request** in `failover_requests` table

### Failover Workflow (Admin-Initiated)

```
Admin receives alert → Reviews situation → Approves failover in Admin UI

1. Admin clicks "Approve Failover" (requires TOTP re-authentication)
2. Controller checks: Does node have IPMI credentials configured?

   ── YES (IPMI available) ──────────────────────────────────────
   │ 3a. Controller sends IPMI power-off command to dead node
   │ 4a. Wait 10s for power-off confirmation
   │ 5a. If confirmed: proceed to step 6
   │ 5b. If not confirmed: alert admin, ABORT (manual intervention needed)
   │
   ── NO (IPMI not available) ───────────────────────────────────
   │ 3b. Admin must MANUALLY confirm the node is powered off
   │     (checkbox: "I confirm this node is powered off or physically disconnected")
   │ 4b. Without confirmation: REFUSE to proceed (data corruption risk)

6. Controller MANDATORY blocklist step BEFORE releasing locks:
   - Controller executes on surviving Ceph monitor: `ceph osd blocklist add {failed_node_management_ip}`
   - This prevents ANY Ceph access from the failed node immediately.
   - Wait 5s for blocklist propagation to all OSDs.
   - If blocklist fails: ABORT failover, alert admin to check Ceph health.

7. ONLY after successful blocklist, release RBD locks:
   - `rbd lock remove vs-vms/vs-{uuid}-disk0 --lock-tag "libvirt"`

8. Controller redistributes VMs to surviving nodes:
   - Sort surviving nodes by available memory (descending)
   - For each VM: find first node with sufficient CPU + RAM
   - Define VM domain XML on target node (via gRPC)
   - Start VM on target node
   - Update vm.node_id in database

9. Verify all VMs are running on new nodes
10. Send notification: "Failover complete. N VMs migrated."
11. Log entire operation in audit_logs
```

### IPMI Fencing Implementation

```go
// Supported IPMI commands
func FenceNode(ctx context.Context, node *models.Node) error {
    if node.IPMIAddress == "" {
        return ErrNoIPMIConfigured
    }

    username := decrypt(node.IPMIUsernameEncrypted)
    password := decrypt(node.IPMIPasswordEncrypted)

    // Power off via ipmitool
    cmd := exec.CommandContext(ctx, "ipmitool",
        "-I", "lanplus",
        "-H", node.IPMIAddress.String(),
        "-U", username,
        "-P", password,
        "power", "off",
    )

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("IPMI power-off failed for node %s: %w", node.Hostname, err)
    }

    // Verify power is off
    time.Sleep(10 * time.Second)
    // ... verify power status via ipmitool power status
    return nil
}
```

### Split-Brain Prevention

**Rule**: A VM's disk must NEVER be mounted by two nodes simultaneously.

Enforcement layers for **Ceph RBD**:
1. **Ceph exclusive-lock** feature on all RBD images (primary defense)
2. **IPMI fencing** ensures dead node cannot write to Ceph (secondary defense)
3. **Ceph OSD blocklist**: `ceph osd blocklist add {failed_node_ip}` prevents the node's OSD client from accessing the cluster even if it comes back (tertiary defense)
4. **Controller state**: only one `node_id` per VM record (application-level constraint)

For **QCOW2** nodes: VM disks are local to each node. HA failover requires restoring from the latest backup on a surviving node, as the failed node's local disks are inaccessible. Ceph RBD is recommended for production HA setups.

---

## 15. BACKUP & RECOVERY SYSTEM

### Backup Strategy

| Aspect | Configuration |
|--------|--------------|
| **Schedule** | Monthly staggered (spread across the month to avoid storage I/O spikes) |
| **Method** | Ceph: RBD snapshot → `rbd export` (full) or `rbd export-diff` (incremental); QCOW2: `qemu-img convert` to standalone image |
| **Storage** | Mounted volume path OR offsite FTP (configurable per system) |
| **Retention** | Configurable: e.g., keep last 3 monthly backups per VM |
| **Consistency** | QEMU Guest Agent `fsfreeze` before snapshot (if agent available) |

### Staggered Scheduling

With 1000 VMs and monthly backups:
- Spread across 28 days (avoid month-end): ~36 VMs per day
- Time slot: between 02:00 - 06:00 local time
- Controller calculates each VM's backup slot based on `vm.id` hash:

```go
func calculateBackupSlot(vmID uuid.UUID, monthDays int) (day int, hour int) {
    hash := sha256.Sum256(vmID[:])
    day = int(hash[0]) % monthDays + 1   // 1-28
    hour = int(hash[1]) % 4 + 2          // 2-5 (02:00 - 05:00)
    return day, hour
}
```

### Backup Creation Flow

```
1. Controller task worker picks up backup.create task
2. Controller calls Node Agent: GuestFreezeFilesystems (if guest agent available)
3. Node Agent: rbd snap create vs-vms/vs-{uuid}-disk0@backup-{timestamp}
4. Controller calls Node Agent: GuestThawFilesystems
5. Node Agent: rbd export vs-vms/vs-{uuid}-disk0@backup-{timestamp} /mnt/backups/{uuid}/{timestamp}.raw
   (Or for incremental: rbd export-diff --from-snap @previous @current)
6. Transfer to offsite if configured (FTP/mounted NFS)
7. Update backup record in database
8. Clean up old snapshots per retention policy
```

### Config Data Backup

The Controller's PostgreSQL database is itself backed up:
- `pg_dump` daily to backup volume
- Encrypted with AES-256-GCM before offsite transfer
- Retention: 30 daily backups

---

## 16. REVERSE DNS (rDNS) SYSTEM

### Two Approaches (Documented Per User Request)

#### Approach A: Direct MySQL (User's Original Preference)

**Pros**: No PowerDNS API dependency, full control
**Cons**: Bypasses PowerDNS cache, must manually manage SOA serial, DNSSEC complications

**Implementation**:
1. Controller connects to PowerDNS MySQL database directly
2. INSERT/UPDATE PTR records in `records` table
3. Update SOA serial: `YYYYMMDDNN` format, increment NN
4. After write: `pdns_control purge {zone}` to flush PowerDNS cache

```go
func SetReverseDNS(ctx context.Context, ip net.IP, hostname string) error {
    ptrName := reverseIPToARPA(ip)
    zoneNa := getZoneForIP(ip)  // e.g., "2.0.192.in-addr.arpa" for 192.0.2.0/24

    tx, err := pdnsMysqlDB.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback()

    domainID, err := ensureZoneExists(ctx, tx, zoneName)
    if err != nil {
        return fmt.Errorf("ensure zone: %w", err)
    }

    // Upsert PTR record (parameterized query per QG-05)
    _, err = tx.ExecContext(ctx,
        `INSERT INTO records (domain_id, name, type, content, ttl, auth)
         VALUES (?, ?, 'PTR', ?, 3600, 1)
         ON DUPLICATE KEY UPDATE content = VALUES(content), ttl = VALUES(ttl)`,
        domainID, ptrName, hostname,
    )
    if err != nil {
        return fmt.Errorf("upsert PTR: %w", err)
    }

    if err := incrementSOASerial(ctx, tx, domainID); err != nil {
        return fmt.Errorf("update SOA: %w", err)
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("commit: %w", err)
    }

    // Flush PowerDNS cache for the zone
    return flushPDNSCache(zoneName)
}
```

#### Approach B: PowerDNS HTTP API (Safer Alternative)

**Pros**: Handles caching, DNSSEC, SOA serials automatically
**Cons**: Requires PowerDNS API to be enabled and accessible

```go
func SetReverseDNSViaAPI(ctx context.Context, ip net.IP, hostname string) error {
    ptrName := reverseIPToARPA(ip)
    zoneName := getZoneForIP(ip)

    payload := map[string]any{
        "rrsets": []map[string]any{{
            "name":       ptrName,
            "type":       "PTR",
            "ttl":        3600,
            "changetype": "REPLACE",
            "records": []map[string]any{{
                "content":  hostname,
                "disabled": false,
            }},
        }},
    }

    req, err := http.NewRequestWithContext(ctx, "PATCH",
        fmt.Sprintf("%s/api/v1/servers/localhost/zones/%s", pdnsAPIURL, zoneName),
        jsonBody(payload),
    )
    req.Header.Set("X-API-Key", pdnsAPIKey)
    // ... execute request with timeout + error handling
}
```

**Recommendation**: Start with Direct MySQL (simpler setup), migrate to PowerDNS API when DNSSEC is needed.

### IPv6 rDNS

For IPv6 /64 blocks, create a wildcard PTR or individual entries:

```
# Wildcard: all addresses in /64 resolve to same hostname
*.5.a.0.0.d.c.b.a.8.b.d.0.1.0.0.2.ip6.arpa.  PTR  vm123.example.com.

# Or individual: only specific address resolves
1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.5.a.0.0.d.c.b.a.8.b.d.0.1.0.0.2.ip6.arpa.  PTR  vm123.example.com.
```

**Rate Limiting**: Customer rDNS changes limited to 10 per hour per customer.

---

## 17. MONITORING & OBSERVABILITY

### Health Endpoints

| Endpoint | Purpose | Response |
|----------|---------|----------|
| `GET /health` | Liveness | `{ "status": "ok" }` |
| `GET /ready` | Readiness (DB + NATS + at least 1 node) | `{ "status": "ready", "db": true, "nats": true, "nodes": 3 }` |
| `GET /metrics` | Prometheus scrape | Standard Prometheus metrics |

### Prometheus Metrics (Controller)

| Metric | Type | Labels |
|--------|------|--------|
| `vs_api_requests_total` | Counter | method, path, status, tier |
| `vs_api_request_duration_seconds` | Histogram | method, path, tier |
| `vs_vms_total` | Gauge | status, node_id |
| `vs_nodes_total` | Gauge | status |
| `vs_node_heartbeat_age_seconds` | Gauge | node_id |
| `vs_tasks_total` | Counter | type, status |
| `vs_task_duration_seconds` | Histogram | type |
| `vs_ws_connections_active` | Gauge | type (vnc, serial, status) |
| `vs_backup_duration_seconds` | Histogram | type (full, incremental) |
| `vs_bandwidth_bytes_total` | Counter | vm_id, direction |

### Node Agent Metrics (Prometheus Exporter)

Each Node Agent exposes `:9100/metrics` (node_exporter) plus custom VM metrics:

| Metric | Type | Labels |
|--------|------|--------|
| `vs_vm_cpu_usage_percent` | Gauge | vm_id |
| `vs_vm_memory_usage_bytes` | Gauge | vm_id |
| `vs_vm_disk_read_bytes_total` | Counter | vm_id |
| `vs_vm_disk_write_bytes_total` | Counter | vm_id |
| `vs_vm_network_rx_bytes_total` | Counter | vm_id |
| `vs_vm_network_tx_bytes_total` | Counter | vm_id |
| `vs_vm_status` | Gauge | vm_id, status |

### Structured Logging (per QG-08)

All components use `slog` with JSON output:

```json
{
  "time": "2026-03-11T08:30:00Z",
  "level": "INFO",
  "msg": "vm started",
  "vm_id": "550e8400-e29b-41d4-a716-446655440000",
  "node_id": "node-01",
  "customer_id": "c-123",
  "correlation_id": "req_abc123",
  "duration_ms": 2340
}
```

---

## 18. NOTIFICATION SYSTEM

### Channels

| Channel | Library | Configuration |
|---------|---------|--------------|
| **Email** | Go `net/smtp` + MJML templates | SMTP server, sender address |
| **Telegram** | Telegram Bot API (HTTP) | Bot token, admin chat IDs |

### Event Types

| Event | Admin Notify | Customer Notify | Webhook |
|-------|-------------|----------------|---------|
| VM created | ✓ | ✓ | ✓ |
| VM started | — | Optional | ✓ |
| VM stopped | — | Optional | ✓ |
| VM deleted | ✓ | ✓ | ✓ |
| VM suspended | ✓ | ✓ | ✓ |
| Backup completed | — | Optional | ✓ |
| Backup failed | ✓ | ✓ | ✓ |
| Node offline | ✓ (urgent) | — | — |
| Node failed (3 misses) | ✓ (urgent) | — | — |
| Failover completed | ✓ | ✓ (affected VMs) | ✓ |
| Bandwidth cap exceeded | — | ✓ | ✓ |
| Migration completed | ✓ | Optional | ✓ |

### Webhook Delivery

- Retry schedule: 10s, 60s, 5min, 30min, 2h (5 attempts)
- HMAC-SHA256 signature in `X-Webhook-Signature` header
- Idempotency key in `X-Webhook-Idempotency-Key` header
- Auto-disable webhook after 50 consecutive failures

---

## 19. IMPLEMENTATION PHASES

> **Note:** All implementation phases are now complete. This section is preserved for historical reference and onboarding context.

### Phase 1: Foundation (Weeks 1-4) ✅ Complete

| Task | Component |
|------|-----------|
| PostgreSQL schema + migrations | Controller |
| Go project scaffolding (cmd/, internal/) | Both |
| gRPC protobuf definitions + code generation | Both |
| mTLS certificate generation tooling | Both |
| Basic Node Agent: libvirt connection, VM lifecycle (create/start/stop/delete) | Node Agent |
| Basic Controller: Gin setup, health endpoints, DB connection | Controller |
| NATS JetStream integration for async tasks | Controller |

### Phase 2: Core VM Management (Weeks 5-8) ✅ Complete

| Task | Component |
|------|-----------|
| Storage backend integration (Ceph RBD + QCOW2: clone, resize, snapshot) | Node Agent |
| Cloud-init ISO generation | Node Agent |
| Template management (import, create, list) | Both |
| VM provisioning async task (full flow) | Both |
| IPAM: IP allocation, assignment, release | Controller |
| Admin API: VM CRUD, node management | Controller |
| Customer API: VM list, start/stop/restart | Controller |
| Provisioning API: create/delete/suspend | Controller |
| JWT + Argon2id authentication | Controller |
| Audit logging middleware | Controller |

### Phase 3: Networking & Security (Weeks 9-11) ✅ Complete

| Task | Component |
|------|-----------|
| libvirt nwfilter anti-spoofing | Node Agent |
| Bandwidth accounting (nftables counters) | Node Agent |
| Port speed limiting (libvirt QoS) | Node Agent |
| Bandwidth cap + throttling | Both |
| DHCP server management (dnsmasq) | Node Agent |
| IPv6 /64 allocation + routing | Both |
| SMTP port 25 blocking | Node Agent |
| Rate limiting middleware | Controller |
| RBAC permission enforcement | Controller |

### Phase 4: Advanced Features (Weeks 12-15) ⚠️ Partial

> Note: Live migration, backup system, TOTP 2FA, reinstall workflow, and QEMU Guest Agent are complete.  
> HA failover (70%) and rDNS management (60%) are partially complete.

| Task | Component |
|------|-----------|
| Live migration | Both |
| HA failover (heartbeat + fencing + redistribution) | Both |
| Backup system (staggered scheduling, RBD export, offsite) | Both |
| rDNS management (PowerDNS MySQL) | Controller |
| TOTP 2FA | Controller |
| Reinstall workflow | Both |
| QEMU Guest Agent integration | Node Agent |

### Phase 5: Web UIs (Weeks 16-20) ✅ Complete

| Task | Component |
|------|-----------|
| Next.js project scaffolding (admin + customer) | WebUI |
| Authentication UI (login, 2FA, sessions) | WebUI |
| VM list + detail pages | WebUI |
| VM control actions (start/stop/restart) | WebUI |
| NoVNC console integration | WebUI + Controller WS proxy |
| xterm.js serial console | WebUI + Controller WS proxy |
| Resource monitoring graphs (uPlot + ECharts) | WebUI |
| Admin: node management, plan management, IP sets | WebUI |
| Customer: reinstall, ISO upload, backup/snapshot | WebUI |
| Customer: rDNS, API keys, webhooks | WebUI |
| Responsive design (mobile/tablet) | WebUI |
| Dark/light theme | WebUI |

### Phase 6: Integration & Polish (Weeks 21-24) ✅ Complete

| Task | Component |
|------|-----------|
| WHMCS provisioning module | WHMCS Module |
| WHMCS client area integration (iframe + SSO) | WHMCS Module |
| Email notification templates | Controller |
| Telegram bot integration | Controller |
| Webhook delivery system | Controller |
| Customer webhook management | Both |
| Docker Compose production configuration | DevOps |
| Nginx TLS + rate limiting configuration | DevOps |
| End-to-end testing (Playwright + Go test harness) | Testing |
| Security hardening review | All |
| Documentation (INSTALL.md, USAGE.md, API.md) | Docs |

---

## 20. QUALITY GATES MAPPING

Every component maps to the 16 Quality Gates from `CODING_STANDARD.md`:

| QG | How Applied in VirtueStack |
|----|---------------------------|
| QG-01 Readable | Max 40-line functions, 3-level nesting, Go naming conventions |
| QG-02 Secure | OWASP 2025 compliance, mTLS, nwfilter, WebSocket security (Section 17) |
| QG-03 Typed | Go strict types, TypeScript `strict: true`, PHP PHPStan level 8 |
| QG-04 Structured | Custom error types per category, operation journal for multi-step tasks |
| QG-05 Validated | `go-playground/validator` (Go), Zod (TS), schema validation on all inputs |
| QG-06 DRY | Shared `internal/shared/` package, React component library |
| QG-07 Defensive | Null checks on all gRPC responses, QEMU guest agent timeouts, storage error handling |
| QG-08 Logged | `slog` JSON logging, correlation IDs on all requests, zero PII |
| QG-09 Bounded | 10s HTTP timeout, 5s DB timeout, 30s gRPC unary, 60s gRPC stream, circuit breakers |
| QG-10 Clean | `golangci-lint`, `eslint`, `phpcs` zero warnings |
| QG-11 Documented | This document + INSTALL.md + USAGE.md + API.md + ARCHITECTURE.md |
| QG-12 Configurable | All config via env vars / YAML config file, `.env.example` provided |
| QG-13 Compatible | API versioning (`/api/v1/`), DB migrations with rollback |
| QG-14 Tested | Unit + integration + E2E, `go test -race`, Playwright for WebUI, 80%+ coverage |
| QG-15 Dependency-Safe | All Go/npm/PHP packages verified, pinned versions, `govulncheck`/`npm audit` |
| QG-16 Performant | Pagination on all list endpoints, connection pooling, indexed queries, no N+1 |

### Performance Targets

| Endpoint Category | Target Latency | Notes |
|-------------------|---------------|-------|
| VM list (up to 1000) | < 500ms | Paginated, indexed query |
| VM single action (start/stop) | < 2s | Includes gRPC round-trip to node |
| VM create (async) | < 200ms API response | Returns task_id, actual provisioning is async |
| VNC WebSocket setup | < 500ms | Connection establishment |
| VNC frame latency | < 50ms | After connection established |
| Heartbeat cycle (100 nodes) | < 10s total | Parallel gRPC calls |
| Backup scheduling (10,000 VMs) | < 30s planning | Background job |
| Authentication | < 200ms | Token validation (not password hashing) |
| Simple CRUD reads | < 200ms | Single table with index |

---

## 21. GRACEFUL DEGRADATION MATRIX

| Dependency Down | Impact | Fallback Behavior |
|----------------|--------|-------------------|
| PostgreSQL | All mutations fail | Return 503 for writes; cached reads for status if Redis added later |
| NATS JetStream | Async tasks cannot be queued | Synchronous fallback for critical operations (VM start/stop); reject creates |
| Ceph cluster | VM disk operations fail (Ceph backend only) | Running VMs continue unaffected; new operations rejected with clear error. QCOW2 VMs on other nodes unaffected |
| Local storage full | QCOW2 VM provisioning/resizing fails | Alert admin; running VMs continue unaffected |
| Node Agent (single) | VMs on that node inaccessible | Mark node degraded; alert admin; don't assign new VMs |
| All Node Agents | No VM operations possible | Controller serves cached status; all mutations return 503 |
| PowerDNS MySQL | rDNS updates fail | Queue changes in PostgreSQL; retry on recovery |
| SMTP server | Email notifications fail | Log failed sends; Telegram as backup channel |
| Telegram API | Telegram notifications fail | Log failed sends; email as backup channel |
| Nginx | All external access blocked | Controller can run direct if needed; Nginx auto-restart via Docker |

---

## 22. ENVIRONMENT VARIABLES REFERENCE

### Controller

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `DATABASE_URL` | Yes | PostgreSQL connection string | `postgresql://vs:pass@db:5432/virtuestack?sslmode=require` |
| `NATS_URL` | Yes | NATS server URL | `nats://nats:4222` |
| `JWT_SECRET` | Yes | HMAC secret for JWT signing (min 32 bytes) | `(random 64-char hex)` |
| `ENCRYPTION_KEY` | Yes | AES-256 key for encrypting secrets at rest | `(random 64-char hex)` |
| `PDNS_MYSQL_DSN` | No | PowerDNS MySQL connection | `pdns:pass@tcp(pdns-db:3306)/powerdns` |
| `PDNS_API_URL` | No | PowerDNS HTTP API URL (alternative to MySQL) | `http://pdns:8081` |
| `PDNS_API_KEY` | No | PowerDNS API key | `changeme` |
| `SMTP_HOST` | No | SMTP server hostname | `smtp.example.com` |
| `SMTP_PORT` | No | SMTP port | `587` |
| `SMTP_USERNAME` | No | SMTP auth username | `noreply@example.com` |
| `SMTP_PASSWORD` | No | SMTP auth password | `(secret)` |
| `SMTP_FROM` | No | Email sender address | `VirtueStack <noreply@example.com>` |
| `TELEGRAM_BOT_TOKEN` | No | Telegram bot token | `123456:ABC-DEF` |
| `TELEGRAM_ADMIN_CHAT_IDS` | No | Comma-separated admin chat IDs | `123456789,987654321` |
| `BACKUP_STORAGE_PATH` | No | Mounted volume for backups | `/mnt/backups` |
| `BACKUP_FTP_HOST` | No | Offsite FTP hostname | `backup.example.com` |
| `BACKUP_FTP_USER` | No | FTP username | `virtuestack` |
| `BACKUP_FTP_PASSWORD` | No | FTP password | `(secret)` |
| `LOG_LEVEL` | No | Logging level | `info` (debug, info, warn, error) |
| `LISTEN_ADDR` | No | HTTP listen address | `:8080` |
| `GRPC_LISTEN_ADDR` | No | gRPC listen address (internal) | `:50051` |

### Node Agent

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `CONTROLLER_GRPC_ADDR` | Yes | Controller gRPC address | `controller.internal:50051` |
| `NODE_ID` | Yes | Unique node identifier | `(UUID)` |
| `CEPH_POOL` | No | Default Ceph pool for VMs (Ceph backend only) | `vs-vms` |
| `CEPH_USER` | No | Ceph auth user (Ceph backend only) | `virtuestack` |
| `CEPH_CONF` | No | Ceph config path (Ceph backend only) | `/etc/ceph/ceph.conf` |
| `STORAGE_BACKEND` | No | Storage backend type: `ceph` or `qcow` (default: `ceph`) | `ceph` |
| `STORAGE_PATH` | No | Base path for QCOW2 VM storage (QCOW backend only) | `/var/lib/virtuestack/vms` |
| `TEMPLATE_PATH` | No | Base path for QCOW2 template storage (QCOW backend only) | `/var/lib/virtuestack/templates` |
| `TLS_CERT_FILE` | Yes | mTLS client certificate | `/etc/virtuestack/tls/node.crt` |
| `TLS_KEY_FILE` | Yes | mTLS client key | `/etc/virtuestack/tls/node.key` |
| `TLS_CA_FILE` | Yes | CA certificate | `/etc/virtuestack/tls/ca.crt` |
| `LOG_LEVEL` | No | Logging level | `info` |
| `CLOUDINIT_PATH` | No | Cloud-init ISO storage path | `/var/lib/virtuestack/cloud-init` |
| `ISO_STORAGE_PATH` | No | Customer ISO storage path | `/var/lib/virtuestack/iso` |

---

## APPENDIX A: DOCKER COMPOSE (Production)

```yaml
version: "3.9"

services:
  nginx:
    image: nginx:1.25-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/certs:/etc/nginx/certs:ro
    depends_on:
      - controller
      - admin-webui
      - customer-webui
    restart: unless-stopped

  controller:
    build: ./controller
    environment:
      - DATABASE_URL=${DATABASE_URL}
      - NATS_URL=nats://nats:4222
      - JWT_SECRET=${JWT_SECRET}
      - ENCRYPTION_KEY=${ENCRYPTION_KEY}
    depends_on:
      db:
        condition: service_healthy
      nats:
        condition: service_started
    restart: unless-stopped

  admin-webui:
    build: ./admin-webui
    environment:
      - NEXT_PUBLIC_API_URL=${PUBLIC_API_URL}
      - NEXT_PUBLIC_WS_URL=${PUBLIC_WS_URL}
    restart: unless-stopped

  customer-webui:
    build: ./customer-webui
    environment:
      - NEXT_PUBLIC_API_URL=${PUBLIC_API_URL}
      - NEXT_PUBLIC_WS_URL=${PUBLIC_WS_URL}
    restart: unless-stopped

  db:
    image: postgres:16-alpine
    environment:
      - POSTGRES_DB=virtuestack
      - POSTGRES_USER=${DB_USER}
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${DB_USER}"]
      interval: 5s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  nats:
    image: nats:2.10-alpine
    command: ["-js", "-sd", "/data"]
    volumes:
      - natsdata:/data
    restart: unless-stopped

volumes:
  pgdata:
  natsdata:
```

---

## APPENDIX B: MIGRATION PATH TO OVS

When advanced networking is needed (VXLAN, per-flow QoS, SDN), migrate from Linux Bridge to Open vSwitch:

1. Install `openvswitch-switch` on nodes
2. Create OVS bridge: `ovs-vsctl add-br br0`
3. Add physical port: `ovs-vsctl add-port br0 bond0`
4. Update libvirt network XML: `<interface type='bridge'>` → `<interface type='bridge'>` (same XML, different backend)
5. Update nwfilter to use OVS flow rules instead of ebtables
6. Port speed limiting via OVS QoS instead of tc

**Trigger to migrate**: When you need VXLAN overlay networking between data centers, or per-flow traffic analytics.

---

## APPENDIX C: CIRCUIT BREAKER THRESHOLDS

Per MASTER_CODING_STANDARD Section 6, default is 5 consecutive failures. VirtueStack-specific overrides:

| Service | Threshold | Rationale |
|---------|-----------|-----------|
| Node heartbeat | 3 failures | Matches NODE_FAILED threshold — faster detection needed |
| PowerDNS MySQL | 5 failures | Default — rDNS is not time-critical |
| SMTP | 5 failures | Default — queue and retry |
| Telegram API | 5 failures | Default — fallback to email |
| WHMCS webhook | 5 failures | Default — WHMCS can poll as backup |
| Ceph cluster | 3 failures | Storage failures are critical — alert immediately (Ceph backend only) |
| Local storage | 5 failures | Alert on capacity threshold (QCOW backend only) |

---

**END OF KICKSTART PLAN**

*This document is the single source of truth for VirtueStack system architecture.*
*All implementation MUST follow `CODING_STANDARD.md` Quality Gates QG-01 through QG-16.*
*Revision: 2.0 | Created: March 2026*
