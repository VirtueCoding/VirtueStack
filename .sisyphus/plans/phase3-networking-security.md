# Phase 3: Networking & Security — Implementation Plan

**Created:** 2026-03-11
**Status:** In Progress (0/8 tasks)
**Session:** ses_32782a8a3ffe50jpztvXn3PBxL
**Reference:** `docs/VIRTUESTACK_KICKSTART_V2.md` lines 680-750 (Networking), 1100-1117 (Rate Limiting)

---

## Phase 3 Overview

Implement networking layer (libvirt nwfilter), bandwidth accounting, DHCP server per node, IPv6 SLAAC, rate limiting, and RBAC enforcement.

---

## Phase 3 Tasks

### Phase 3.1: Libvirt Network Filters (nwfilter)
**Goal:** Implement MAC/IP spoofing protection via libvirt nwfilter
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` lines 680-720

- [ ] **3.1.1**: `internal/nodeagent/network/nwfilter.go` — Network filter management
  - Generate nwfilter XML for VMs
  - MAC spoofing protection (allow only VM's assigned MAC)
  - IP spoofing protection (allow only assigned IPs)
  - ARP spoofing protection
  - Apply filter via libvirt on VM start

### Phase 3.2: Bandwidth Accounting
**Goal:** Track and enforce bandwidth limits per VM
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` lines 720-750

- [ ] **3.2.1**: `internal/controller/services/bandwidth_service.go` — Bandwidth tracking service
  - Poll VM stats from node agents
  - Store monthly usage in DB
  - Calculate overage
  - Trigger throttling at 5Mbps when limit exceeded
  - Reset counters monthly

- [ ] **3.2.2**: `internal/nodeagent/network/bandwidth.go` — Node agent bandwidth enforcement
  - Read VM network stats via libvirt
  - Apply tc (traffic control) rules for throttling
  - Remove throttling when reset

### Phase 3.3: DHCP Server per Node
**Goal:** Run DHCP server on each node for VMs expecting DHCP
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` lines 750-780

- [ ] **3.3.1**: `internal/nodeagent/network/dhcp.go` — DHCP server management
  - dnsmasq integration for DHCP
  - Static leases from IPAM (MAC → IP mapping)
  - Host-only network bridge
  - Start/stop with VM lifecycle

### Phase 3.4: IPv6 SLAAC/RA
**Goal:** IPv6 Stateless Address Auto-configuration with Router Advertisements
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` lines 780-810

- [ ] **3.4.1**: `internal/nodeagent/network/ipv6.go` — IPv6 network management
  - Router Advertisement daemon (radvd)
  - /48 prefix per node, /64 per VM
  - SLAAC for automatic VM addressing
  - ND proxy for external access

### Phase 3.5: Rate Limiting Middleware
**Goal:** API rate limiting per endpoint per user
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` lines 1099-1117

- [ ] **3.5.1**: Update `internal/controller/api/middleware/ratelimit.go`
  - Per-endpoint rate limits (from config)
  - Sliding window algorithm (Redis-backed)
  - Different limits for customer vs admin
  - Return 429 with Retry-After header

### Phase 3.6: RBAC Enforcement
**Goal:** Role-based access control for admin actions
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` lines 1040-1060

- [ ] **3.6.1**: `internal/controller/services/rbac_service.go` — RBAC service
  - Permission checking (canDeleteVM, canCreateNode, etc.)
  - Role definitions (super_admin, admin, readonly)
  - Audit logging on permission denied
  - Re-auth required for destructive operations

### Phase 3.7: Database Migration
**Goal:** Add bandwidth tracking tables

- [ ] **3.7.1**: `migrations/000002_bandwidth_tracking.up.sql`
  - bandwidth_usage table (vm_id, month, bytes_in, bytes_out, reset_at)
  - bandwidth_throttle table (vm_id, throttled_since, throttle_until)

### Phase 3.8: Verification
**Goal:** Ensure networking integration works

- [ ] **3.8.1**: Verify nwfilter XML generation
- [ ] **3.8.2**: Verify bandwidth tracking flow
- [ ] **3.8.3**: Verify rate limiting middleware
- [ ] **3.8.4**: Cross-file consistency check

---

## Architecture References

### Network Filter (nwfilter) Rules
```xml
<filter name='vs-anti-spoof'>
  <!-- Allow traffic from assigned MAC -->
  <rule action='accept' direction='in'>
    <mac match='yes' srcmacaddr='$MAC'/>
  </rule>
  <!-- Drop all other inbound -->
  <rule action='drop' direction='in'/>
  
  <!-- Allow ARP replies with assigned IP -->
  <rule action='accept' direction='inout'>
    <arp match='yes' arpsrcmacaddr='$MAC' arpsrcipaddr='$IP'/>
  </rule>
  
  <!-- Allow IP traffic from assigned IP -->
  <rule action='accept' direction='in'>
    <ip match='yes' srcipaddr='$IP'/>
  </rule>
</filter>
```

### Bandwidth Throttling (tc)
```bash
# Apply 5Mbps throttle
tc qdisc add dev vnet0 root tbf rate 5mbit burst 32kbit latency 400ms

# Remove throttle
tc qdisc del dev vnet0 root
```

### Rate Limits (per VIRTUESTACK_KICKSTART_V2.md)
| Endpoint | Customer | Admin |
|----------|----------|-------|
| POST /vms | 10/min | 100/min |
| DELETE /vms/{id} | 5/min | 50/min |
| POST /vms/{id}/start | 20/min | 200/min |
| POST /vms/{id}/stop | 20/min | 200/min |
| GET /vms | 60/min | 300/min |

---

## Session History

| Session ID | Date | Work Completed |
|------------|------|----------------|
| ses_32782a8a3ffe50jpztvXn3PBxL | 2026-03-11 | Phase 2 complete (87 files), starting Phase 3 |
