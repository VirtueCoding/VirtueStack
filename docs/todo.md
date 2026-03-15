# VirtueStack Implementation Plan

**Comprehensive Audit of Planned / Partial / Not Implemented / Stub Code**

**Generated:** March 15, 2026  
**Based on:** llmscope.md, VIRTUESTACK_KICKSTART_V2.md, MASTER_CODING_STANDARD_V2.md, and full codebase analysis

---

## Executive Summary

| Category | Count | Status |
|----------|-------|--------|
| **Critical Priority** | 29 items | Node Agent gRPC methods (stub implementations) |
| **High Priority** | 8 items | Core features (failover, migration, console, password reset) |
| **Medium Priority** | 12 items | Enhancement features (monitoring, notifications, webhooks) |
| **Low Priority** | 6 items | Documentation, polish, optional features |
| **Total Incomplete** | **55 items** | |

**Overall Project Completion:** ~90% (Controller), ~40% (Node Agent), ~85% (WebUI)

---

## 1. CRITICAL PRIORITY - Node Agent gRPC Stubs (29 Methods)

**Status:** Auto-generated stubs return `codes.Unimplemented`  
**Impact:** VM lifecycle operations will not work without these  
**Effort:** Large (3-4 weeks)  
**Files Affected:** `internal/nodeagent/server.go`, `internal/nodeagent/vm/*.go`, `internal/nodeagent/storage/*.go`

### 1.1 VM Lifecycle Methods (7 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 1 | `CreateVM` | `node_agent_grpc.pb.go` | 540-541 | Provision new VM | Requires: RBD clone, cloud-init ISO, libvirt domain creation |
| 2 | `StartVM` | `node_agent_grpc.pb.go` | 543-544 | Start a VM | Requires: libvirt domain create |
| 3 | `StopVM` | `node_agent_grpc.pb.go` | 546-547 | Graceful VM shutdown | Requires: Guest agent shutdown or ACPI |
| 4 | `ForceStopVM` | `node_agent_grpc.pb.go` | 549-550 | Force power off | Requires: libvirt destroy |
| 5 | `DeleteVM` | `node_agent_grpc.pb.go` | 552-553 | Delete VM | Requires: RBD image deletion, domain undefine |
| 6 | `ReinstallVM` | `node_agent_grpc.pb.go` | 555-556 | Reinstall OS | Requires: Disk re-clone from template |
| 7 | `ResizeVM` | `node_agent_grpc.pb.go` | 558-559 | Resize resources | Requires: RBD resize, domain update |

### 1.2 Migration Methods (3 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 8 | `MigrateVM` | `node_agent_grpc.pb.go` | 561-562 | Live migrate VM | Requires: libvirt migrate, RBD lock transfer |
| 9 | `AbortMigration` | `node_agent_grpc.pb.go` | 564-565 | Cancel migration | Requires: libvirt abort job |
| 10 | `PostMigrateSetup` | `node_agent_grpc.pb.go` | 567-568 | Post-migration config | Requires: tc QoS reapply, nwfilter restore |

### 1.3 Console Methods (2 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 11 | `StreamVNCConsole` | `node_agent_grpc.pb.go` | 570-571 | VNC streaming | Requires: libvirt VNC to gRPC bridge |
| 12 | `StreamSerialConsole` | `node_agent_grpc.pb.go` | 573-574 | Serial streaming | Requires: libvirt serial PTY to gRPC bridge |

### 1.4 Metrics & Status Methods (3 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 13 | `GetVMStatus` | `node_agent_grpc.pb.go` | 576-577 | VM running status | Requires: libvirt domain info |
| 14 | `GetVMMetrics` | `node_agent_grpc.pb.go` | 579-580 | VM resource usage | Requires: libvirt domain stats |
| 15 | `GetNodeResources` | `node_agent_grpc.pb.go` | 582-583 | Node capacity | Requires: system resources query |

### 1.5 Snapshot Methods (4 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 16 | `CreateSnapshot` | `node_agent_grpc.pb.go` | 585-586 | Create disk snapshot | Requires: RBD snapshot create |
| 17 | `DeleteSnapshot` | `node_agent_grpc.pb.go` | 588-589 | Delete snapshot | Requires: RBD snapshot delete |
| 18 | `RevertSnapshot` | `node_agent_grpc.pb.go` | 591-592 | Restore from snapshot | Requires: RBD snapshot rollback |
| 19 | `ListSnapshots` | `node_agent_grpc.pb.go` | 594-595 | List all snapshots | Requires: RBD snapshot list |

### 1.6 Guest Agent Methods (5 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 20 | `GuestExecCommand` | `node_agent_grpc.pb.go` | 597-598 | Execute command in VM | **NOT IMPLEMENTED BY DESIGN** - security risk |
| 21 | `GuestSetPassword` | `node_agent_grpc.pb.go` | 600-601 | Set root password | Requires: qemu-guest-agent set-user-password |
| 22 | `GuestFreezeFilesystems` | `node_agent_grpc.pb.go` | 603-604 | Freeze for backup | Requires: qemu-guest-agent fsfreeze |
| 23 | `GuestThawFilesystems` | `node_agent_grpc.pb.go` | 606-607 | Thaw filesystems | Requires: qemu-guest-agent fsthaw |
| 24 | `GuestGetNetworkInterfaces` | `node_agent_grpc.pb.go` | 609-610 | Get guest network info | Requires: qemu-guest-agent network-get-interfaces |

### 1.7 Bandwidth Methods (3 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 25 | `GetBandwidthUsage` | `node_agent_grpc.pb.go` | 612-613 | Get bandwidth stats | Requires: nftables counters read |
| 26 | `SetBandwidthLimit` | `node_agent_grpc.pb.go` | 615-616 | Set tc QoS limit | Requires: tc HTB qdisc manipulation |
| 27 | `ResetBandwidthCounters` | `node_agent_grpc.pb.go` | 618-619 | Reset counters | Requires: nftables counter reset |

### 1.8 Health Methods (2 methods)

| # | Method | File | Line | Description | Implementation Notes |
|---|--------|------|------|-------------|---------------------|
| 28 | `Ping` | `node_agent_grpc.pb.go` | 621-622 | Health check | Simple ping/pong response |
| 29 | `GetNodeHealth` | `node_agent_grpc.pb.go` | 624-625 | Detailed health | Requires: Ceph, libvirt, resource status |

---

## 2. HIGH PRIORITY - Core Features

### 2.1 HA Failover System (llmscope.md: 70% complete)

**Status:** Detection works, auto-recovery pending IPMI stress testing  
**Completion Target:** 95%  
**Effort:** Medium (1-2 weeks)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | IPMI Fencing Integration | ⚠️ PARTIAL | `failover_service.go:104-118` | STONITH execution implemented but needs stress testing with real hardware |
| 2 | Ceph Blocklist Verification | ⚠️ PARTIAL | `failover_service.go:121-133` | Blocklist add implemented, needs verification with real Ceph cluster |
| 3 | VM Redistribution Stress Testing | ❌ NOT DONE | N/A | Need load testing with 100+ VMs failover scenario |
| 4 | Failover Approval Workflow | ✅ DONE | `admin/nodes.go` | Admin approval endpoint exists |
| 5 | Auto-Failover Monitor | ✅ DONE | `failover_monitor.go` | Background monitor implemented |

**Implementation Plan:**
```
Task HA-1: IPMI Stress Testing
- Test IPMI power off with 50+ nodes
- Verify power status detection
- Add retry logic for IPMI failures

Task HA-2: Ceph Blocklist Testing  
- Verify blocklist prevents RBD writes
- Test blocklist cleanup after recovery
- Document Ceph requirements

Task HA-3: VM Redistribution Load Test
- Simulate 100 VM failover
- Measure migration time
- Verify resource allocation accuracy
```

### 2.2 PowerDNS rDNS Integration (llmscope.md: 60% complete)

**Status:** Service implemented, integration pending  
**Completion Target:** 100%  
**Effort:** Small (3-5 days)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | rDNS Service Implementation | ✅ DONE | `rdns_service.go` | Full PowerDNS MySQL integration |
| 2 | Customer API Endpoints | ❌ MISSING | N/A | Need endpoints in customer API |
| 3 | Admin API Endpoints | ❌ MISSING | N/A | Need bulk rDNS management |
| 4 | WebUI Integration | ❌ MISSING | N/A | Frontend forms needed |
| 5 | SOA Serial Management | ✅ DONE | `rdns_service.go:186-238` | Auto-increment implemented |

**Implementation Plan:**
```
Task RDNS-1: Customer API Endpoints
- GET /vms/:id/rdns - Get current PTR records
- PUT /vms/:id/rdns - Update PTR record
- Add rate limiting (10/hour per customer)

Task RDNS-2: Admin API Endpoints  
- GET /admin/rdns - List all PTR records
- PUT /admin/rdns/:ip_id - Admin override
- Bulk operations support

Task RDNS-3: WebUI Integration
- Customer: rDNS tab in VM detail
- Admin: rDNS management page
- Validation for hostname format
```

### 2.3 Live Migration System

**Status:** API stubbed, implementation pending  
**Completion Target:** 100%  
**Effort:** Large (2-3 weeks)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | Migration Service | ✅ DONE | `migration_service.go` | Orchestration logic complete |
| 2 | Migration Task Handler | ⚠️ PARTIAL | `tasks/migration_execute.go` | Skeleton exists, needs implementation |
| 3 | gRPC MigrateVM | ❌ NOT DONE | `node_agent_grpc.pb.go:561` | Node agent stub |
| 4 | Progress Tracking | ⚠️ PARTIAL | `migration_service.go` | Basic progress, needs % completion |
| 5 | Rollback on Failure | ⚠️ PARTIAL | `migration_service.go:451-512` | Cancel implemented, auto-rollback pending |

**Implementation Plan:**
```
Task MIG-1: Node Agent Migration
- Implement MigrateVM in nodeagent/vm/migration.go
- Handle Ceph RBD lock transfer
- Support live vs cold migration

Task MIG-2: Migration Task Handler
- Implement migration_execute.go
- Add progress reporting
- Handle pre/post migration setup

Task MIG-3: Enhanced Progress Tracking
- Add percentage completion
- Estimated time remaining
- Bandwidth usage tracking

Task MIG-4: Rollback Automation
- Automatic rollback on failure
- State cleanup
- Customer notification
```

### 2.4 Console Access System

**Status:** WebSocket proxy implemented, VNC/Serial streaming pending  
**Completion Target:** 100%  
**Effort:** Medium (1-2 weeks)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | WebSocket Proxy | ✅ DONE | `websocket.go:259-365` | VNC/Serial proxy complete |
| 2 | Console Token API | ✅ DONE | `console.go:26-88` | Token generation implemented |
| 3 | gRPC StreamVNCConsole | ❌ NOT DONE | `node_agent_grpc.pb.go:570` | Node agent stub |
| 4 | gRPC StreamSerialConsole | ❌ NOT DONE | `node_agent_grpc.pb.go:573` | Node agent stub |
| 5 | WebUI VNC Component | ✅ DONE | `vnc-console.tsx` | noVNC integration complete |
| 6 | WebUI Serial Component | ✅ DONE | `serial-console.tsx` | xterm.js integration complete |

**Implementation Plan:**
```
Task CON-1: Node Agent VNC Streaming
- Implement StreamVNCConsole in nodeagent/vm/console.go
- Bridge libvirt VNC to gRPC stream
- Handle authentication

Task CON-2: Node Agent Serial Streaming  
- Implement StreamSerialConsole in nodeagent/vm/console.go
- Bridge libvirt serial PTY to gRPC stream
- Handle resize events

Task CON-3: Integration Testing
- End-to-end console test
- Multiple concurrent sessions
- Reconnection handling
```

### 2.5 Password Reset Workflow

**Status:** Table exists, API endpoints missing  
**Completion Target:** 100%  
**Effort:** Small (1 week)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | Database Schema | ✅ DONE | `000011_password_resets.up.sql` | Table and indexes created |
| 2 | Repository Methods | ✅ DONE | `customer_repo.go:608-668` | CRUD operations implemented |
| 3 | Auth Service Methods | ✅ DONE | `auth_service.go:437-533` | Reset logic implemented |
| 4 | Customer API Endpoint | ❌ MISSING | N/A | POST /auth/forgot-password needed |
| 5 | Customer API Reset Endpoint | ❌ MISSING | N/A | POST /auth/reset-password needed |
| 6 | Admin API Endpoints | ❌ MISSING | N/A | Admin-initiated reset |
| 7 | WebUI Forgot Password | ❌ MISSING | N/A | Forgot password page |
| 8 | Email Templates | ✅ DONE | `notifications/email.go:148-247` | Password reset template exists |

**Implementation Plan:**
```
Task PWD-1: Customer API Endpoints
- POST /auth/forgot-password - Request reset email
- POST /auth/reset-password - Submit new password
- Rate limiting: 3 attempts per hour

Task PWD-2: Admin API Endpoints
- POST /admin/customers/:id/reset-password - Admin reset
- Force logout after reset

Task PWD-3: WebUI Pages
- /forgot-password - Email input form
- /reset-password - Token + new password form
- Email template customization

Task PWD-4: Email Integration
- Wire up email service
- Token generation and hashing
- Expiration handling (24 hours)
```

---

## 3. MEDIUM PRIORITY - Enhancement Features

### 3.1 Prometheus Metrics Endpoint

**Status:** Basic endpoint exists, custom metrics pending  
**Completion Target:** 100%  
**Effort:** Small (3-5 days)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | /metrics Endpoint | ✅ DONE | `server.go:338` | promhttp.Handler() registered |
| 2 | VM Metrics | ❌ MISSING | N/A | Custom VM metrics collection |
| 3 | Node Metrics | ❌ MISSING | N/A | Per-node resource metrics |
| 4 | API Metrics | ❌ MISSING | N/A | Request latency, error rates |
| 5 | Business Metrics | ❌ MISSING | N/A | VM creation rate, backup success |

**Implementation Plan:**
```
Task MET-1: VM Metrics Collection
- active_vms gauge
- vm_cpu_usage histogram  
- vm_memory_usage histogram
- vm_disk_usage histogram

Task MET-2: Node Metrics
- node_cpu_percent gauge
- node_memory_percent gauge
- node_storage_percent gauge
- node_online gauge

Task MET-3: API Metrics Middleware
- request_duration_seconds histogram
- request_total counter (with status code)
- request_size_bytes histogram
- response_size_bytes histogram

Task MET-4: Business Metrics
- vm_created_total counter
- vm_deleted_total counter
- backup_created_total counter
- backup_failed_total counter
- task_duration_seconds histogram (by task type)
```

### 3.2 Notification System

**Status:** Service implemented, delivery pending  
**Completion Target:** 100%  
**Effort:** Medium (1 week)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | Notification Service | ✅ DONE | `notification.go` | Core service implemented |
| 2 | Email Provider | ✅ DONE | `notifications/email.go` | SMTP with templates |
| 3 | Telegram Provider | ✅ DONE | `notifications/telegram.go` | Bot integration |
| 4 | Event Triggers | ⚠️ PARTIAL | `notification.go:320-406` | Some events wired, most not |
| 5 | Customer Preferences | ✅ DONE | `notification_preferences.go` | DB and API complete |
| 6 | Webhook Delivery | ✅ DONE | `webhook.go`, `webhook_deliver.go` | Full implementation |

**Implementation Plan:**
```
Task NOT-1: Wire Up Event Triggers
- VM created/deleted/suspended
- Backup completed/failed  
- Node offline alerts
- Bandwidth exceeded

Task NOT-2: Email Template Completion
- vm-created template
- vm-deleted template
- backup-failed template
- bandwidth-exceeded template

Task NOT-3: Notification UI
- Customer notification settings page
- Real-time notification dropdown
- Mark as read/unread
```

### 3.3 Backup & Snapshot UI Integration

**Status:** Backend complete, frontend placeholders  
**Completion Target:** 100%  
**Effort:** Medium (1 week)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | Backup Service | ✅ DONE | `backup_service.go` | Full implementation |
| 2 | Snapshot Service | ✅ DONE | `snapshot_service.go` | Full implementation |
| 3 | Backup Task Handler | ✅ DONE | `backup_create.go` | Async creation |
| 4 | Snapshot Task Handler | ✅ DONE | `snapshot_handlers.go` | Create/revert/delete |
| 5 | Customer API | ✅ DONE | `backups.go`, `snapshots.go` | All endpoints |
| 6 | WebUI Integration | ⚠️ PLACEHOLDER | `backups/`, `snapshots/` | Components exist, not wired |

**Implementation Plan:**
```
Task BAK-1: Backup UI Wiring
- VM detail backup tab
- Create backup dialog
- Restore backup workflow
- Backup list with status

Task BAK-2: Snapshot UI Wiring
- VM detail snapshot tab  
- Create snapshot button
- Restore snapshot confirmation
- Snapshot timeline view

Task BAK-3: Progress Indicators
- Real-time backup progress
- Snapshot creation status
- Estimated completion time
```

### 3.4 API Key Management UI

**Status:** Backend complete, frontend pending  
**Completion Target:** 100%  
**Effort:** Small (3-4 days)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | API Key Service | ✅ DONE | `apikey_service.go` | Full implementation |
| 2 | Customer API | ✅ DONE | `apikeys.go` | CRUD endpoints |
| 3 | WebUI Integration | ❌ MISSING | N/A | Needs settings page |

**Implementation Plan:**
```
Task KEY-1: API Key UI
- Settings > API Keys section
- Create key dialog with scopes
- Revoke key confirmation
- Show last used timestamp

Task KEY-2: Key Security
- Mask key in UI (show only prefix)
- Copy to clipboard button
- Usage statistics
```

### 3.5 Grafana Dashboard Templates

**Status:** Not implemented  
**Completion Target:** 100%  
**Effort:** Small (2-3 days)

**Implementation Plan:**
```
Task GRAF-1: Dashboard Templates
- Node overview dashboard
- VM metrics dashboard  
- API performance dashboard
- Business metrics dashboard

Task GRAF-2: Alerting Rules
- Node offline alert
- High CPU usage alert
- Disk space alert
- API error rate alert
```

### 3.6 ISO Upload System

**Status:** Backend partially implemented, frontend pending  
**Completion Target:** 100%  
**Effort:** Medium (1 week)

| # | Item | Status | File | Details |
|---|------|--------|------|---------|
| 1 | tus Protocol Handler | ⚠️ PARTIAL | `iso_upload.go` | Server-side upload |
| 2 | ISO Validation | ❌ MISSING | N/A | ISO 9660 validation |
| 3 | Virus Scanning | ❌ MISSING | N/A | ClamAV integration |
| 4 | WebUI Upload | ❌ MISSING | N/A | tus-js-client integration |
| 5 | ISO Management API | ⚠️ PARTIAL | `vms.go` | Attach/detach endpoints |

**Implementation Plan:**
```
Task ISO-1: Complete Backend
- ISO format validation
- Virus scanning (ClamAV)
- Storage quota enforcement

Task ISO-2: WebUI Upload
- Drag-and-drop upload
- Progress bar
- Resumable uploads
- File size validation

Task ISO-3: ISO Management
- List customer ISOs
- Delete ISO
- Attach to VM workflow
```

---

## 4. LOW PRIORITY - Documentation & Polish

### 4.1 Documentation Completion

| # | Item | Status | Priority | Effort |
|---|------|--------|----------|--------|
| 1 | API.md Update | ⚠️ PARTIAL | Low | 2 days |
| 2 | INSTALL.md Refinement | ⚠️ PARTIAL | Low | 1 day |
| 3 | Troubleshooting Guide | ❌ MISSING | Low | 2 days |
| 4 | Migration Guide | ❌ MISSING | Low | 1 day |
| 5 | Security Best Practices | ⚠️ PARTIAL | Low | 2 days |
| 6 | WHMCS Integration Guide | ⚠️ PARTIAL | Low | 1 day |

### 4.2 Testing Improvements

| # | Item | Status | Priority | Effort |
|---|------|--------|----------|--------|
| 1 | E2E Test Coverage | ⚠️ PARTIAL | Low | 1 week |
| 2 | Load Testing Suite | ⚠️ PARTIAL | Low | 3 days |
| 3 | Chaos Engineering Tests | ❌ MISSING | Low | 1 week |
| 4 | Security Penetration Tests | ⚠️ PARTIAL | Low | 1 week |

### 4.3 Monitoring & Observability

| # | Item | Status | Priority | Effort |
|---|------|--------|----------|--------|
| 1 | Distributed Tracing | ❌ MISSING | Low | 3 days |
| 2 | Health Check Endpoints | ✅ DONE | Low | - |
| 3 | Log Aggregation | ⚠️ PARTIAL | Low | 2 days |
| 4 | Error Tracking (Sentry) | ❌ MISSING | Low | 1 day |

### 4.4 Optional Features

| # | Item | Status | Priority | Effort |
|---|------|--------|----------|--------|
| 1 | BGP Route Coordination | ❌ MISSING | Low | 1 week |
| 2 | SMTP Port 25 Blocking | ❌ MISSING | Low | 2 days |
| 3 | Metadata Endpoint Security | ❌ MISSING | Low | 1 day |
| 4 | Advanced Network Security | ❌ MISSING | Low | 3 days |

---

## 5. IMPLEMENTATION ROADMAP

### Phase 1: Foundation (Weeks 1-4)
**Goal:** Get core VM operations working

| Week | Tasks | Deliverables |
|------|-------|--------------|
| 1 | Node Agent VM Lifecycle (7 methods) | VM create/start/stop/delete working |
| 2 | Node Agent Metrics & Status (3 methods) | Resource monitoring working |
| 3 | Node Agent Snapshots (4 methods) | Backup/snapshot creation working |
| 4 | Node Agent Guest Agent (4 methods) | Password reset, freeze/thaw working |

### Phase 2: Console & Migration (Weeks 5-7)
**Goal:** Enable console access and live migration

| Week | Tasks | Deliverables |
|------|-------|--------------|
| 5 | Console Streaming (2 methods) | VNC/Serial console working end-to-end |
| 6 | Live Migration (3 methods) | VM migration between nodes working |
| 7 | Bandwidth Management (3 methods) | QoS and bandwidth tracking working |

### Phase 3: Core Features (Weeks 8-11)
**Goal:** Complete failover, password reset, notifications

| Week | Tasks | Deliverables |
|------|-------|--------------|
| 8 | HA Failover Completion | Auto-failover with stress testing |
| 9 | Password Reset Workflow | Full forgot/reset password flow |
| 10 | Notification System | Email/Telegram/webhook notifications |
| 11 | rDNS Integration | PTR record management complete |

### Phase 4: UI & Polish (Weeks 12-14)
**Goal:** Complete frontend integration and polish

| Week | Tasks | Deliverables |
|------|-------|--------------|
| 12 | WebUI Feature Wiring | Backup, snapshot, API key UI complete |
| 13 | Metrics & Monitoring | Prometheus, Grafana integration |
| 14 | Testing & Documentation | E2E tests, docs, deployment guides |

---

## 6. TECHNICAL DEBT & REFACTORING

### 6.1 Known Issues

| # | Issue | Location | Priority | Action |
|---|-------|----------|----------|--------|
| 1 | Reverse connection not implemented | `nodeagent/server.go:121` | Low | Document as known limitation |
| 2 | Template storage fallback | `server.go:29-45` | Low | Implement proper storage backend |
| 3 | gRPC connection pool stub | `grpc_client.go:112` | Low | Connection pooling optimization |

### 6.2 Performance Optimizations

| # | Item | Current | Target | Action |
|---|------|---------|--------|--------|
| 1 | DB Query Optimization | N+1 in some lists | JOIN queries | Audit repositories |
| 2 | Cache Layer | None | Redis | Add caching for hot data |
| 3 | Image Optimization | No compression | WebP | Optimize WebUI assets |

---

## 7. QUALITY GATES COMPLIANCE

Per `MASTER_CODING_STANDARD_V2.md`, all new code must pass:

| Gate | Status | Notes |
|------|--------|-------|
| QG-01 Readable | ✅ PASS | Follow existing patterns |
| QG-02 Secure | ✅ PASS | Input validation, auth |
| QG-03 Typed | ✅ PASS | Go strict, TypeScript strict |
| QG-04 Structured | ✅ PASS | Custom errors |
| QG-05 Validated | ✅ PASS | go-playground/validator |
| QG-06 DRY | ✅ PASS | Extract shared logic |
| QG-07 Defensive | ✅ PASS | Null checks, timeouts |
| QG-08 Logged | ✅ PASS | slog structured logging |
| QG-09 Bounded | ✅ PASS | HTTP/gRPC timeouts |
| QG-10 Clean | ✅ PASS | golangci-lint |
| QG-11 Documented | ⚠️ PARTIAL | Add godoc comments |
| QG-12 Configurable | ✅ PASS | Env vars + YAML |
| QG-13 Compatible | ✅ PASS | API versioning |
| QG-14 Tested | ⚠️ PARTIAL | 80%+ coverage required |
| QG-15 Dependency-Safe | ✅ PASS | Pinned versions |
| QG-16 Performant | ⚠️ PARTIAL | Add benchmarks |

---

## 8. DEPENDENCIES & BLOCKERS

### 8.1 External Dependencies

| Dependency | Version | Purpose | Status |
|------------|---------|---------|--------|
| Ceph | Reef/Squid | VM storage | Required for testing |
| PowerDNS | 4.9+ | rDNS | Optional |
| IPMI | 2.0 | Node fencing | Required for HA |
| ClamAV | Latest | Virus scanning | Optional |

### 8.2 Blockers

None identified. All work can proceed in parallel with proper coordination.

---

## 9. SUCCESS CRITERIA

### 9.1 Definition of Done

- [ ] All 29 Node Agent gRPC methods implemented
- [ ] HA failover tested with real hardware
- [ ] Live migration working end-to-end
- [ ] VNC/Serial console accessible from WebUI
- [ ] Password reset workflow complete
- [ ] Prometheus metrics reporting
- [ ] All Quality Gates passing
- [ ] 80%+ test coverage on new code
- [ ] Documentation updated
- [ ] E2E tests passing

### 9.2 Production Readiness Checklist

- [ ] Security audit passed
- [ ] Load testing completed
- [ ] Disaster recovery tested
- [ ] Monitoring alerts configured
- [ ] Runbooks created
- [ ] Support documentation complete
- [ ] Team training completed

---

## 10. REFERENCES

- [LLM Scope Document](llmscope.md) - Architecture overview
- [Kickstart V2](VIRTUESTACK_KICKSTART_V2.md) - Detailed architecture specification
- [Master Coding Standard](MASTER_CODING_STANDARD_V2.md) - Quality gates and coding rules
- [API Documentation](API.md) - API reference
- [README.md](../README.md) - Project overview and quick start

---

*This document is a living reference. Update as implementation progresses.*
*Last updated: March 15, 2026*
