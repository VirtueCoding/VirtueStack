# Phase 4: Advanced Features — Implementation Plan

**Created:** 2026-03-11
**Status:** Completed (5/5 tasks)
**Session:** ses_32782a8a3ffe50jpztvXn3PBxL
**Reference:** `docs/VIRTUESTACK_KICKSTART_V2.md`

---

## Phase 4 Overview

Implement advanced cluster-level features: Live Migration, HA Failover (with STONITH/Fencing), Backup Scheduling, rDNS Management, and QEMU Guest Agent integration.

---

## Phase 4 Tasks

### Phase 4.1: Live Migration Orchestration
**Goal:** Implement zero-downtime VM migration between nodes
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 13 (Tasks)

- [x] **4.1.1**: `internal/controller/services/migration_service.go` — Migration pre-checks and orchestration
  - Validate target node resources
  - Publish `vm.migrate` async task
- [x] **4.1.2**: `internal/controller/tasks/migration_execute.go` — Migration task handler
  - Call source Node Agent `MigrateVM` (libvirt peer-to-peer migration)
  - Call destination Node Agent `PostMigrateSetup` to re-apply `tc` throttling and `nwfilter`
  - Update VM state (node_id) in database

### Phase 4.2: HA Failover & STONITH
**Goal:** Implement automated/manual failover for dead nodes
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 14

- [x] **4.2.1**: `internal/controller/services/failover_service.go` — Failover logic
  - Check `consecutive_heartbeat_misses >= 3`
  - Execute STONITH: IPMI power off if credentials exist
  - **CRITICAL**: Execute `ceph osd blocklist add <failed_node_ip>`
  - Release RBD locks: `rbd lock remove`
  - Redistribute VMs to surviving nodes and start them

### Phase 4.3: Backup Scheduling & Task
**Goal:** Implement automated monthly staggered backups
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 15

- [x] **4.3.1**: `internal/controller/services/backup_service.go` — (Update) Add scheduling logic
  - Cron-style scheduling to spread backups across the month
- [x] **4.3.2**: `internal/controller/tasks/backup_create.go` — Backup task handler
  - `guest-fsfreeze-freeze` (with strict 10s timeout!)
  - Create Ceph RBD snapshot and protect it
  - `guest-fsfreeze-thaw`
  - Clone to `vs-backups` pool or export qcow2
  - Fallback to crash-consistent backup if freeze times out

### Phase 4.4: rDNS Management (PowerDNS)
**Goal:** Manage reverse DNS PTR records directly in PowerDNS MySQL
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 16

- [x] **4.4.1**: `internal/controller/services/rdns_service.go` — PowerDNS integration
  - Connect to PowerDNS MySQL database
  - Upsert PTR records for IPv4/IPv6
  - Increment SOA serial
  - Flush PowerDNS cache (if API configured)

### Phase 4.5: QEMU Guest Agent & Reinstall
**Goal:** Safe execution of guest agent commands and OS reinstall flow
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 4

- [x] **4.5.1**: `internal/nodeagent/guest/agent.go` — Guest agent wrapper
  - Implement `ping`, `fsfreeze`, `shutdown`, `set-user-password`
  - **CRITICAL**: Enforce hard 10s timeouts on all operations to prevent hanging Controller tasks
- [x] **4.5.2**: `internal/controller/tasks/vm_reinstall.go` — Reinstall task handler
  - Stop VM, delete old RBD disk
  - Clone fresh from template, regenerate cloud-init
  - Start VM

---

## Safety & Idempotency Rules

1. **Ceph Blocklisting**: Never release RBD locks during failover without successfully running `ceph osd blocklist add` on the failed node.
2. **Migration State**: Remember that libvirt migration destroys `vnet` interfaces on the source and recreates them on the destination. All `tc` limits must be re-applied on the destination using the `PostMigrateSetup` hook.
3. **Guest Agent Unreliability**: Guest OS environments are untrusted and can hang. Never block indefinitely on `qemu-ga`.

---

## Session History

| Session ID | Date | Work Completed |
|------------|------|----------------|
| ses_32782a8a3ffe50jpztvXn3PBxL | 2026-03-11 | Completed Phase 1-3. Fixed deep architectural flaws. Prepared Phase 4 plan. |
