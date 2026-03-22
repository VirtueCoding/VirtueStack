# workflow.md -- VirtueStack

> Inspired by [karpathy/autoresearch](https://github.com/karpathy/autoresearch): the human writes the Markdown program, the AI executes the work loop. This file IS the source code of your software engineering process.
>
> **Project:** VirtueStack -- KVM/QEMU VM management platform for VPS hosting providers
> **Repo:** `AbuGosok/VirtueStack` (private, MIT)
> **Reference docs:** `AGENTS.md` (what exists), `CODING_STANDARD.md` (how to write code), `docs/CODEMAPS/*.md` (token-lean summaries)

---

## Philosophy

1. **You program the agent, not the software.** This document defines constraints, objectives, and evaluation criteria. Iterating on this file is where your leverage lives.
2. **Every experiment is logged.** Kept or discarded, every attempt goes into `results.tsv`. This is your institutional memory.
3. **Fixed time budgets make experiments comparable.** If a unit takes 2x its budget, discard and decompose.
4. **The loop never stops to ask.** Ambiguity in this file is a bug in your program. Fix the file, not the agent's behavior.
5. **CODING_STANDARD.md is law.** Any violation of any rule is a blocking defect. No exceptions. The 19 quality gates (QG-01 through QG-19) are the evaluation function.

---

## Architecture Context

```
                 WHMCS Module  |  Admin Portal  |  Customer Portal
                               |  (Next.js 16)  |  (Next.js 16)
                               v                v
                    NGINX (SSL termination, :443)
                               |
              +----------------+----------------+
              |           CONTROLLER             |    Docker
              |  Go+Gin | JWT | PostgreSQL 16+   |    Stack
              |  NATS JetStream | :8080 internal  |
              +----------------+-----------------+
                               |
                       gRPC + mTLS
                               |
              +----------------+-----------------+
              |          NODE AGENT               |    Bare
              |  Go+gRPC | libvirt | Ceph/QCOW    |    Metal
              |  nwfilter | nftables | tc QoS      |
              +----------------------------------+
```

### Domain Map

| Domain | Directory | Key Files | Owner Service |
|--------|-----------|-----------|---------------|
| Controller API (Admin) | `internal/controller/api/admin/` | 14 handler files | AuthService, VMService, NodeService |
| Controller API (Customer) | `internal/controller/api/customer/` | 17 handler files | AuthService, VMService, SnapshotService |
| Controller API (Provisioning) | `internal/controller/api/provisioning/` | 8 handler files | VMService (WHMCS) |
| Middleware | `internal/controller/api/middleware/` | 8 files | Auth, RateLimit, CSRF, Audit |
| Services | `internal/controller/services/` | 23 files | All 13 service classes |
| Repositories | `internal/controller/repository/` | 19 files | All 13 PostgreSQL repos |
| Async Tasks | `internal/controller/tasks/` | 9 handler files | NATS consumers |
| Node Agent (VM) | `internal/nodeagent/vm/` | domain_xml, lifecycle | libvirt integration |
| Node Agent (Storage) | `internal/nodeagent/storage/` | rbd.go, qcow.go | Ceph RBD + QCOW2 |
| Node Agent (Network) | `internal/nodeagent/network/` | nwfilter, bandwidth, abuse_prevention | nftables, tc, dnsmasq |
| Node Agent (Guest) | `internal/nodeagent/guest/` | QEMU Guest Agent | GuestExec, password |
| Shared | `internal/shared/` | errors, config, crypto, logging | Cross-cutting |
| Proto | `proto/virtuestack/node_agent.proto` | 35+ gRPC methods | Controller <-> Agent |
| Migrations | `migrations/` | 32 migration pairs | Schema evolution |
| Admin WebUI | `webui/admin/` | 8 pages, shadcn/ui | Next.js 16 + React 19 |
| Customer WebUI | `webui/customer/` | 3 pages + VM detail | Next.js 16 + React 19 |
| WHMCS Module | `modules/servers/virtuestack/` | 7 PHP files | Billing integration |
| E2E Tests | `tests/e2e/` | 14+ Playwright specs | Admin, Customer, Auth |
| Integration Tests | `tests/integration/` | 5 Go test files | DB + service tests |

### Database (23 tables, 6 domains)

| Domain | Tables | RLS |
|--------|--------|-----|
| Identity & Auth | customers, admins, sessions, customer_api_keys, provisioning_keys | customers, sessions, customer_api_keys |
| Infrastructure | locations, nodes, node_heartbeats, ip_sets, ip_addresses, ipv6_prefixes, vm_ipv6_subnets | ip_addresses |
| VM Resources | plans, templates, vms, snapshots, backups, backup_schedules | vms, snapshots, backups, backup_schedules |
| Operations | tasks, audit_logs (partitioned), system_settings, failover_requests | -- |
| Notifications | notification_preferences, notification_events, customer_webhooks, webhook_deliveries | notification_preferences, notification_events |
| Networking | (shared with Infrastructure) | -- |

### API Tiers

| Tier | Base Path | Auth | Rate Limit | VM Create/Delete |
|------|-----------|------|------------|-------------------|
| Provisioning | `/api/v1/provisioning/*` | API Key + IP whitelist | 1000/min | YES |
| Customer | `/api/v1/customer/*` | JWT + Refresh | 100 read, 30 write/min | NO (read + power only) |
| Admin | `/api/v1/admin/*` | JWT + 2FA (TOTP) | 500/min | YES |

**Critical design decision:** Customers CANNOT create or delete VMs via the Customer API. This prevents abuse (buying one VPS, creating extras for free). Only Provisioning (WHMCS) and Admin APIs can create/delete.

---

## Phase 0: Setup (Run Once)

### 0.1 Branch
use audit/mar21 branch

### 0.2 Verify Environment
```bash
make deps                    # go mod download + verify + tidy
cd webui/admin && npm ci     # Admin portal deps
cd webui/customer && npm ci  # Customer portal deps
make build                   # Build controller + node-agent to bin/
make lint                    # golangci-lint (must be zero warnings)
make vet                     # go vet
make test-race               # Tests with race detector
make vuln                    # govulncheck
make proto                   # Regenerate protobuf (if proto/ changed)
cd webui/admin && npm run lint && npm run type-check
cd webui/customer && npm run lint && npm run type-check
```

### 0.3 Initialize Logs
```bash
mkdir -p .workflow-logs
echo -e "attempt\tunit\taction\tresult\tduration_s\tgates_passed\tnotes" > .workflow-logs/results.tsv
echo -e "commit\tunit\tverdict\tfindings\treviewer" > .workflow-logs/review.tsv
echo -e "bug_id\ttechnique\tseverity\tfile\tline\tdescription\treproduction" > .workflow-logs/bugs.tsv
echo -e "threat_id\tcategory\tattack_vector\tlikelihood\timpact\tmitigation\tstatus" > .workflow-logs/threats.tsv
```

All four logs MUST exist before entering any phase. Every experiment, review finding, bug, and threat gets a row.

---

## Phase 1: Planning

### 1.1 Decompose Into Units

Every work unit is a row in a table:

| Field | Required | Description |
|-------|----------|-------------|
| `unit_id` | yes | `U-001`, `U-002`, ... |
| `domain` | yes | One of: `controller-api`, `controller-service`, `controller-repo`, `controller-task`, `nodeagent-vm`, `nodeagent-storage`, `nodeagent-network`, `nodeagent-guest`, `shared`, `proto`, `migration`, `admin-webui`, `customer-webui`, `whmcs`, `e2e-test`, `integration-test`, `infra` |
| `description` | yes | One sentence. What, not how. |
| `acceptance` | yes | Single measurable criterion. "X returns Y when Z." |
| `files_touched` | yes | Exhaustive list. If you discover more during implementation, stop and update the plan first. |
| `dependencies` | yes | List of `unit_id`s that must be completed first. Empty = independent. |
| `time_budget` | yes | Minutes. Max 60. If >60, decompose further. |
| `security_relevant` | yes | `true` if touches auth, crypto, RLS, input validation, network filters, WHMCS billing, or customer isolation |

### 1.2 Planning Rules

1. **One unit = one concern.** A unit that touches both `controller-api` and `nodeagent-storage` must be split.
2. **Proto cascade.** If `node_agent.proto` changes, add dependent units for: `make proto`, update Node Agent server, update Controller client, update tests. List all four.
3. **Migration discipline.** Every migration unit must specify both `.up.sql` and `.down.sql`. The down migration must be idempotent. Use `SET lock_timeout = '5s'`. Use `CREATE INDEX CONCURRENTLY` for indexes on existing tables.
4. **RLS awareness.** If a unit adds a new table that stores customer data, it MUST include an RLS policy unit. Check the existing RLS list in AGENTS.md S4.2.
5. **Plan limits.** If a unit adds a new customer-facing resource, check whether `plans` table needs a new limit column (like `snapshot_limit`, `backup_limit`, `iso_upload_limit`).
6. **40-line rule.** If a unit's description implies a function >40 lines, decompose into sub-units.
7. **No `security_relevant: false` for Customer API.** Every Customer API change is security-relevant by definition (customer isolation, RLS, ownership checks).

### 1.3 Threat Pre-Assessment

Before implementation begins, scan the plan for threats. For each unit marked `security_relevant: true`, fill one row in `threats.tsv`:

| Field | Values |
|-------|--------|
| `category` | One of: `customer-abuse`, `tenant-escape`, `privilege-escalation`, `billing-fraud`, `resource-exhaustion`, `network-abuse`, `data-exfiltration`, `injection`, `auth-bypass`, `supply-chain` |
| `attack_vector` | Specific attack description |
| `likelihood` | `low`, `medium`, `high` |
| `impact` | `low`, `medium`, `high`, `critical` |
| `mitigation` | What the code must do to prevent this |
| `status` | `planned` (before impl), `implemented` (after impl), `verified` (after Phase 4) |

---

## Phase 2: Implementation (The Work Loop)

### 2.1 Loop Structure

```
for each unit in dependency_order(plan):
    start_timer()
    
    # Implement
    modify files listed in unit.files_touched
    
    # Evaluate (ALL must pass)
    run_evaluation_suite(unit)
    
    if all_gates_pass:
        git add . && git commit -m "feat(unit.domain): unit.description"
        log_result("KEEP", gates_passed, duration)
    else:
        git checkout -- .   # discard everything
        log_result("DISCARD", gates_failed, duration)
        # Analyze failure, adjust approach, retry
        # After 3 discards on same unit: decompose further or escalate
    
    if duration > unit.time_budget * 2:
        log_result("TIMEOUT", gates_status, duration)
        # Decompose unit into smaller pieces
```

### 2.2 Evaluation Suite

Run after every modification. The gate list maps directly to CODING_STANDARD.md quality gates:

#### Go Backend Gates
```bash
# QG-10: Clean
make lint                        # golangci-lint: zero warnings
make vet                         # go vet: zero warnings

# QG-14: Tested  
make test-race                   # All tests pass with race detector

# QG-15: Dependency-Safe
make vuln                        # govulncheck: no known vulnerabilities

# QG-02: Secure (if proto changed)
make proto                       # Regenerate protobuf, verify no drift
```

#### Frontend Gates
```bash
# QG-10: Clean
cd webui/admin && npm run lint
cd webui/customer && npm run lint

# QG-03: Typed
cd webui/admin && npm run type-check
cd webui/customer && npm run type-check

# QG-14: Tested (if E2E tests exist for the feature)
cd tests/e2e && npm test
```

#### Migration Gates
```bash
# QG-13: Compatible
make migrate-up                  # Apply migration
make migrate-down                # Rollback migration
make migrate-up                  # Re-apply (idempotency check)

# Verify RLS
# For any new table with customer_id: confirm RLS policy exists
# For any new column on RLS-enabled table: confirm policy still covers it
```

#### Manual Gates (agent must verify by reading code)

| Gate | CODING_STANDARD Ref | Check |
|------|---------------------|-------|
| QG-01 Readable | S2, S16 | No function >40 lines. No nesting >3 levels. No >4 params. |
| QG-02 Secure | S4 | No hardcoded secrets. mTLS for gRPC. Input validated. Argon2id for passwords. |
| QG-03 Typed | S1 | No `any` (TS). No bare `interface{}` (Go). No `_` error ignoring. |
| QG-04 Structured | S3 | Custom error types from `shared/errors`. Operation journals for multi-step tasks. |
| QG-05 Validated | S5 | All external input validated with `validator` (Go) or Zod (TS). |
| QG-06 DRY | S2 | No duplicated logic. Shared utilities extracted. |
| QG-07 Defensive | S17 | Nil checks before access. Slice bounds checked. Empty map handled. |
| QG-08 Logged | S6 | slog structured logging. Correlation IDs. No PII. No `fmt.Println`. |
| QG-09 Bounded | S3, S19 | HTTP 10s, DB OLTP 5s, gRPC unary 30s, gRPC stream 60s, WebSocket idle 5m. |
| QG-11 Documented | S17 | Doc comments on all exported types/functions. |
| QG-12 Configurable | S1, S17 | All config via env vars. `.env.example` updated. |
| QG-13 Compatible | S15 | API versioning. Additive changes only. Migration has rollback. |
| QG-16 Performant | S7, S8 | No N+1. Indexed columns. Cursor pagination. Connection pooling. |
| QG-17 Provenance | S11 | Pinned versions. Lock files committed. |
| QG-18 Observable | S6 | RED metrics. Health probes. Trace context propagated. |
| QG-19 Deployment | S13 | Non-root containers (Controller/WebUIs). Minimal images. Graceful shutdown. |

### 2.3 VirtueStack-Specific Implementation Rules

#### Context Propagation
Every Go function that does I/O MUST accept `ctx context.Context` as first parameter. Propagate to:
- All repository calls
- All gRPC calls (Controller -> Node Agent)
- All NATS publish/subscribe
- All HTTP client calls

#### Customer Isolation (Defense in Depth)
Three layers, ALL required for customer-facing operations:
1. **Application layer:** `customerID` extracted from JWT, passed to service, verified against resource ownership
2. **Repository layer:** All customer queries include `WHERE customer_id = $1` (never trust caller alone)
3. **Database layer:** PostgreSQL RLS policy on table with `SET app.current_customer_id`

#### Storage Backend Awareness
Every VM operation must check `vm.StorageBackend` (or `plan.StorageBackend` for new VMs):
- `ceph`: Use `storage.RBDBackend` (go-ceph, pool `vs-vms`)
- `qcow`: Use `storage.QCOWBackend` (qemu-img, path `/var/lib/virtuestack/vms/`)
- Migration: Only between nodes supporting the VM's backend. Cross-backend blocked.

#### Plan Limit Enforcement
Customer API endpoints that create resources (snapshots, backups, ISOs) MUST:
1. Get VM (ownership verified)
2. Look up plan via `planRepo.GetByID(vm.PlanID)`
3. Count existing resources
4. Compare against plan limit
5. Return `409 Conflict` if exceeded

Admin/Provisioning APIs are NOT subject to plan limits.

#### Task System
All long-running operations go through NATS JetStream:
1. API handler creates task row in `tasks` table (status: `pending`)
2. Publishes to NATS stream `TASKS`
3. Worker pool (5 workers) picks up task
4. Worker calls Node Agent via gRPC
5. Updates task status + notifies WebSocket subscribers

Task state machine: `pending -> running -> completed | failed`. Also `pending -> cancelled`.

#### WHMCS Integration
Provisioning endpoints are the billing system's interface. Rules:
- Every provisioning action MUST be idempotent (WHMCS retries on timeout)
- `service_id` from WHMCS is the external reference (stored in VM metadata)
- Suspend/Unsuspend must be instant (no async task needed for power operations)
- `ChangePackage` (resize) is an async task -- return task ID, WHMCS polls

### 2.4 Crash Classification

When the evaluation suite fails, classify the failure to guide the fix:

| Failure Type | Typical Cause | Fix Strategy |
|-------------|---------------|---------------|
| `lint:funlen` | Function >40 lines | Extract helper functions |
| `lint:nestif` | Nesting >3 levels | Early returns, guard clauses |
| `lint:gosec` | Security finding | Fix per gosec rule (G101-G601) |
| `lint:govet` | Suspicious construct | Struct alignment, printf args |
| `test:race` | Data race detected | Add mutex, use channel, or use atomic |
| `test:fail` | Logic error | Debug, fix, re-run |
| `vuln:CVE` | Vulnerable dependency | `go get pkg@latest`, verify fix |
| `proto:drift` | Generated code stale | `make proto`, commit generated files |
| `migrate:up` | SQL syntax or constraint | Fix migration SQL |
| `migrate:down` | Rollback incomplete | Ensure down migration reverses ALL up changes |
| `frontend:lint` | ESLint violation | Fix per rule |
| `frontend:types` | TypeScript error | Fix types, no `any` |
| `rls:missing` | New customer table without RLS | Add ALTER TABLE ... ENABLE ROW LEVEL SECURITY + CREATE POLICY |

---

## Phase 3: Code Review (The Review Loop)

After all units are implemented and committed, review every kept commit.

### 3.1 Review Checklist (13 Points)

For each commit, evaluate ALL points. Log to `review.tsv`.

| # | Check | Pass Criteria |
|---|-------|---------------|
| R-01 | Correctness | Does it do what the unit's acceptance criterion says? Test it mentally with 3 inputs. |
| R-02 | Edge Cases | Empty collections, nil pointers, max values, Unicode, zero, negative, concurrent access. (CODING_STANDARD S17: Edge Case Exhaustiveness) |
| R-03 | Error Handling | Every error checked. Wrapped with context (`fmt.Errorf("doing X: %w", err)`). Custom error types from `shared/errors/`. No empty catch blocks. |
| R-04 | Security | See domain-specific security checklists below (Section 3.2). |
| R-05 | Performance | No N+1 queries. No O(n^2) where O(n) exists. No unbounded memory. Indexed columns. |
| R-06 | Readability | Functions <=40 lines. Nesting <=3. Descriptive names. Comments explain WHY not WHAT. |
| R-07 | Tests | Table-driven (Go). Behavior-tested, not implementation-tested. Happy path + error cases + edge cases. |
| R-08 | API Contract | Response shapes match defined types. Pagination includes `total`, `hasMore`. No internal field leakage. |
| R-09 | Scope | Does this commit change ONLY what the unit described? No "while I was here" changes. |
| R-10 | Backward Compat | New fields optional with defaults. No dropped columns in same migration as code change. |
| R-11 | Observability | Structured logging at boundaries. Correlation IDs propagated. Metrics updated if new endpoint. |
| R-12 | Concurrency | Goroutines tracked with WaitGroup/errgroup. Context cancellation propagated. Mutex scope minimized. |
| R-13 | Customer Isolation | All 3 layers verified: application ownership check, repository WHERE clause, RLS policy. |

### 3.2 Domain-Specific Security Checklists

#### Controller API (Admin + Customer + Provisioning)
- [ ] JWT validation on every endpoint (except login/register)
- [ ] 2FA required for Admin API destructive operations
- [ ] Provisioning API: API key validated + IP whitelist checked
- [ ] Customer API: `customerID` from JWT, not from request body
- [ ] Rate limiting applied (100 read/30 write for customer, 500 for admin, 1000 for provisioning)
- [ ] Input validated with `go-playground/validator` before processing
- [ ] SQL queries parameterized (no string interpolation)
- [ ] Error responses expose zero internals (no stack traces, no SQL, no file paths)
- [ ] Audit log entry for every mutation (actor, action, resource, IP, changes)
- [ ] CORS strict origin allowlist (no wildcards in production)

#### Customer API (Additional -- Abuse Prevention)
- [ ] Customer CANNOT create or delete VMs (only Provisioning/Admin can)
- [ ] Ownership verified: `resource.customer_id == jwt.customer_id` on every operation
- [ ] Plan limits enforced before creating snapshots/backups/ISOs
- [ ] Webhook URLs validated (no private IPs: 10.x, 172.16-31.x, 192.168.x, 169.254.x)
- [ ] API key permissions checked: `customer_api_keys.permissions` matches requested action
- [ ] API key scoped to specific VMs: `customer_api_keys.vm_ids` checked if non-empty
- [ ] Session limit enforced (concurrent sessions capped)
- [ ] Failed login tracking (`failed_login_attempts` column) with progressive delay
- [ ] Password reset tokens are single-use and time-limited

#### Node Agent (gRPC)
- [ ] mTLS required for all Controller <-> Agent communication
- [ ] All libvirt operations have timeouts (no hanging on unresponsive hypervisor)
- [ ] Path traversal prevention: VM UUID validated before constructing disk paths
- [ ] QCOW disk paths constrained to `/var/lib/virtuestack/` prefix
- [ ] Ceph pool names hardcoded to `vs-vms`, `vs-images`, `vs-backups` (no user input)
- [ ] Guest agent commands (`GuestExecCommand`) validate command allowlist
- [ ] Bandwidth counters use nftables named counters (no user-controlled counter names)
- [ ] nwfilter anti-spoofing applied: MAC, IP, ARP, DHCP, Router Advertisement
- [ ] Domain XML generation: no user input in raw XML (use libvirt API structs)
- [ ] AppArmor/SELinux profile applied to QEMU processes

#### Database & Migrations
- [ ] RLS enabled on every new table with `customer_id` (direct or via FK to `vms`)
- [ ] RLS policy uses `current_setting('app.current_customer_id')::UUID`
- [ ] Migration has both `.up.sql` and `.down.sql`
- [ ] Down migration is idempotent (use `IF EXISTS`, `DROP INDEX IF EXISTS`)
- [ ] `SET lock_timeout = '5s'` in migrations that ALTER existing tables
- [ ] `CREATE INDEX CONCURRENTLY` for indexes on tables with data
- [ ] Audit log partitioning maintained if adding new partition requirements
- [ ] Soft delete pattern: `deleted_at` column, `WHERE deleted_at IS NULL` in queries
- [ ] Foreign keys have appropriate `ON DELETE` (CASCADE for child records, RESTRICT for references)

#### Frontend (Admin + Customer WebUI)
- [ ] No `any` types (TypeScript strict mode)
- [ ] API responses validated with Zod schemas before rendering
- [ ] XSS prevention: React auto-escaping used, no `dangerouslySetInnerHTML`
- [ ] WebSocket connections cleaned up on component unmount
- [ ] JWT tokens stored in memory only (not localStorage)
- [ ] Refresh token rotation handled (401 -> refresh -> retry)
- [ ] Console components (VNC/Serial) validate WebSocket URL origin
- [ ] File uploads (ISO) use tus protocol with size limits
- [ ] TanStack Query cache invalidated on mutations

#### WHMCS Module (PHP)
- [ ] All API calls to Controller use HTTPS
- [ ] Provisioning operations are idempotent (safe to retry)
- [ ] Error responses from Controller parsed correctly (not silently swallowed)
- [ ] `service_id` mapping is 1:1 with VM (no orphaned VMs on retry)
- [ ] Price calculations use exact arithmetic (no floating point for money)
- [ ] SSO tokens are short-lived and single-use
- [ ] Webhook receiver validates HMAC signature

### 3.3 Review Verdicts

| Verdict | Meaning | Action |
|---------|---------|--------|
| `PASS` | All 13 checks pass, all domain security checks pass | Commit proceeds to merge queue |
| `FIXME` | Non-security issues found | Log findings with file:line. Create new Phase 2 unit to fix. |
| `SECURITY` | Security checklist item failed | **Blocking.** Fix immediately. Do not merge. Create high-priority Phase 2 unit. |
| `REJECT` | Fundamental design issue | Discard commit. Redesign the unit. Return to Phase 1. |

Every `FIXME` and `SECURITY` finding feeds back into Phase 2 as a new unit with `security_relevant: true`.

---

## Phase 4: Bug Hunting (Adversarial Testing)

Apply these 8 techniques in rotation. Each technique has VirtueStack-specific attack vectors.

### Technique 1: Boundary Value Analysis

Test every numeric input at: `min-1`, `min`, `min+1`, `typical`, `max-1`, `max`, `max+1`.

| Parameter | Min | Max | Source |
|-----------|-----|-----|--------|
| VM vCPU | 1 | 256 | plans.vcpu |
| VM Memory MB | 128 | 524288 (512GB) | plans.memory_mb |
| VM Disk GB | 1 | 10240 (10TB) | plans.disk_gb |
| Hostname length | 1 | 63 | RFC 1123 |
| Snapshot limit per VM | 0 | 100 | plans.snapshot_limit |
| Backup limit per VM | 0 | 100 | plans.backup_limit |
| ISO upload limit | 0 | 10 | plans.iso_upload_limit |
| Port speed Mbps | 1 | 10000 | plans.port_speed_mbps |
| API key permissions | [] | all 14 perms | customer_api_keys.permissions |
| Webhook URL length | 1 | 2048 | customer_webhooks.url |
| Password length | 12 | 128 | CODING_STANDARD S4 |
| TOTP code | 000000 | 999999 | 6-digit TOTP |
| Pagination per_page | 1 | 100 | API query param |
| Concurrent sessions | 1 | 3 (admin) | CODING_STANDARD S4 |
| Bandwidth counter | 0 | 2^63-1 (int64) | nftables counters |
| IPv6 prefix | /48 | /48 | ipv6_prefixes.prefix |
| IPv6 subnet | /64 | /64 | vm_ipv6_subnets.subnet |

### Technique 2: State Mutation & Race Conditions

Test concurrent operations that touch the same resource:

| Race Scenario | Expected Outcome |
|--------------|------------------|
| Two `vm.create` tasks for same customer, same IP | Second gets different IP (or fails with conflict) |
| `vm.delete` during `vm.migrate` | Migration fails gracefully, VM cleaned up |
| `vm.stop` during `backup.create` | Backup fails with clear error, no corrupt backup |
| Two `snapshot.create` hitting plan limit simultaneously | Exactly one succeeds, other gets 409 |
| `customer.delete` while customer has running VMs | VMs terminated first, then customer deleted |
| Concurrent `webhook.deliver` retries for same event | Idempotency key prevents duplicate delivery |
| Two admins `node.drain` + `node.failover` on same node | One wins, other gets 409 Conflict |
| `resize` during `snapshot.revert` | One blocked until other completes |
| Concurrent `api-key.rotate` for same key | Exactly one rotation succeeds |
| Token refresh while access token still valid | Both tokens remain valid within their lifetimes |

Use `go test -race -count=5` on all affected test files.

### Technique 3: Fault Injection

Simulate infrastructure failures:

| Fault | Injection Method | Expected Behavior |
|-------|-----------------|--------------------|
| PostgreSQL down | Stop container | Controller returns 503, health probe fails, auto-recovery on restart |
| NATS down | Stop container | Task creation queued in-memory or returns 503, no silent data loss |
| Node Agent unreachable | Block gRPC port | Task fails after timeout (30s unary), retried up to 3 times |
| Ceph OSD down | Kill OSD process | RBD operations return error, VM creation fails with clear message |
| Disk full on QCOW node | Fill `/var/lib/virtuestack/` | QCOW operations fail gracefully, no partial VM creation |
| libvirt daemon crash | `systemctl stop libvirtd` | Node Agent gRPC calls return error, no hanging goroutines |
| NGINX restart | `docker restart nginx` | In-flight WebSocket connections reconnect, no console data loss |
| Certificate expiry | Use expired mTLS cert | gRPC connection refused, clear error logged |
| DNS resolution failure | Block DNS | Webhook delivery fails, queued for retry |
| NATS stream full | Publish until limit | Backpressure applied, API returns 503 for new tasks |

### Technique 4: Customer Abuse & Tenant Escape

**This is the highest-priority technique for a VPS hosting platform.**

| Attack | Vector | Defense to Verify |
|--------|--------|--------------------|
| **Free VM creation** | Customer calls provisioning API directly | Provisioning API requires separate API key + IP whitelist, not JWT |
| **Cross-tenant VM access** | Customer A sends `GET /customer/vms/{B's VM ID}` | Application ownership check + RLS policy returns 404 (not 403) |
| **Cross-tenant snapshot theft** | Customer A calls `POST /snapshots` with B's VM ID | `verifySnapshotOwnership()` checks VM belongs to customer |
| **Cross-tenant backup restore** | Customer A restores B's backup to own VM | `verifyBackupOwnership()` traces backup -> VM -> customer_id |
| **RLS bypass via SQL injection** | Malicious hostname with SQL | Parameterized queries. No string interpolation. |
| **RLS bypass via direct DB** | Attacker gets DB creds | `app_customer` role has RLS enforced. Superuser access requires separate credentials. |
| **JWT forging** | Weak JWT secret | JWT_SECRET from env var, minimum 256 bits. HMAC-SHA256. |
| **JWT customer_id tampering** | Modify JWT payload | Signature verification rejects tampered tokens |
| **Expired token reuse** | Use access token after 15min | Token expiry checked on every request |
| **Refresh token theft** | Steal refresh token | Refresh tokens hashed in DB, rotated on use, bound to session |
| **TOTP replay** | Reuse same TOTP code | +/-1 step tolerance only, used codes tracked per window |
| **TOTP brute force** | Try all 000000-999999 | Account lockout after 5 failed attempts |
| **Backup code reuse** | Use same backup code twice | Single-use, marked as used after consumption |
| **Session hijacking** | Steal session from another IP | IP + user-agent logged per session, alert on mid-session change |
| **API key privilege escalation** | API key with limited permissions calls admin endpoint | Permissions checked per-request against `customer_api_keys.permissions` |
| **API key scope escape** | API key scoped to VM-A accesses VM-B | `vm_ids` array checked if non-empty |
| **Webhook SSRF** | Customer sets webhook URL to `http://169.254.169.254/` | URL validated: block private IPs, cloud metadata, localhost |
| **Webhook SSRF via redirect** | Webhook URL redirects to internal IP | Follow redirects disabled, or re-validate after each redirect |
| **rDNS injection** | Set rDNS to malicious value | rDNS hostname validated against RFC 1123, length limited |
| **Console session hijack** | Access VNC WebSocket for another customer's VM | WebSocket upgrade validates console-token, which is scoped to customer+VM |
| **ISO upload abuse** | Upload massive ISO to exhaust disk | `iso_upload_limit` per plan, file size validated before acceptance |
| **Bandwidth counter manipulation** | Somehow reset own bandwidth counter | Counters managed by Node Agent (server-side), no customer API to reset |
| **Plan limit bypass** | Create snapshots faster than limit check | Database-level count with transaction isolation prevents TOCTOU |
| **Password reset abuse** | Enumerate emails via timing | Constant-time response regardless of email existence |
| **Account enumeration** | Different error for "email not found" vs "wrong password" | Same error message for both: "Invalid credentials" |

### Technique 5: Network Abuse (Node Agent)

| Attack | Defense to Verify |
|--------|--------------------|
| **MAC spoofing** | nwfilter `clean-traffic` + `no-mac-spoofing` applied to every VM's tap interface |
| **IP spoofing** | nwfilter `no-ip-spoofing` + `no-arp-spoofing` rules |
| **ARP spoofing** | nwfilter `no-arp-spoofing` prevents ARP cache poisoning |
| **DHCP starvation** | nwfilter `no-other-l2-traffic` + `allow-dhcp` only for VM's MAC |
| **Router Advertisement spoofing** | nwfilter blocks RA from VM interfaces |
| **Bandwidth overage** | tc HTB qdisc applied when `bandwidth_usage > plan.bandwidth_cap` |
| **Port scanning from VM** | Rate limiting via nftables on outbound connections |
| **DDoS from VM** | Outbound bandwidth cap + nftables rate limits on new connections |
| **DNS amplification** | Outbound UDP rate limiting via nftables |
| **Crypto mining detection** | CPU usage monitoring via node heartbeats, alert on sustained 100% |
| **Tor exit node** | Network policy enforcement (if configured by admin) |
| **nftables rule escape** | Rules applied per tap interface with VM UUID, not user-controlled names |

### Technique 6: Resource Exhaustion

| Attack | Defense to Verify |
|--------|--------------------|
| API request flood | Rate limiting: 100 read/30 write per minute (customer) |
| WebSocket connection flood | Per-IP limit: 10 concurrent connections |
| Large request body | Request size limit in NGINX + Gin |
| Snapshot spam | Plan limit enforcement (`plans.snapshot_limit`) |
| Backup spam | Plan limit enforcement (`plans.backup_limit`) |
| Task queue saturation | NATS JetStream limits + worker pool capped at 5 |
| Connection pool exhaustion | pgx pool: `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` |
| Goroutine leak | `errgroup.SetLimit()`, context cancellation, `goleak` in tests |
| Disk exhaustion (audit logs) | Partition pruning on `audit_logs` (monthly partitions) |
| Memory exhaustion (large responses) | Cursor-based pagination on ALL list endpoints |

### Technique 7: WHMCS Billing Integrity

| Attack | Defense to Verify |
|--------|--------------------|
| Double provisioning | Idempotent `CreateAccount`: check if VM exists for `service_id` before creating |
| Provision without payment | WHMCS calls provisioning API only after payment confirmation |
| Resize without upgrade payment | `ChangePackage` validates new plan matches WHMCS product |
| Cancel without termination | `TerminateAccount` deletes VM. WHMCS webhook confirms termination. |
| Suspend evasion | `SuspendAccount` is synchronous (power off), not async |
| Service ID collision | UUID-based VM IDs, `service_id` stored as metadata, uniqueness enforced |
| Webhook replay | HMAC signature validation + idempotency key on webhook deliveries |
| Pricing manipulation | Price from `plans` table (server-side), not from WHMCS request payload |

### Technique 8: Supply Chain & Dependency

| Check | How |
|-------|-----|
| `govulncheck ./...` | Run on every build. Fail on any finding. |
| `npm audit --production` | Run for both webui/admin and webui/customer |
| Slopsquatting check | Verify every import against official registry (CODING_STANDARD S11) |
| Container image scan | `trivy image` on all built images. Fail on CRITICAL/HIGH. |
| CI action pinning | All GitHub Actions pinned to commit SHA, not tag |
| Lock file integrity | `go.sum` and `package-lock.json` committed and verified |

### 4.1 Bug Severity Classification

| Severity | Criteria | SLA |
|----------|----------|-----|
| `P0-critical` | Tenant escape, data breach, billing fraud, auth bypass | Fix before merge. Block release. |
| `P1-high` | Customer abuse possible, resource exhaustion, RLS gap | Fix before merge. |
| `P2-medium` | Edge case crash, incorrect error message, missing validation | Fix in same sprint. |
| `P3-low` | Performance issue, cosmetic, non-security edge case | Fix when convenient. |

### 4.2 Goodhart Check

After many iterations, verify the agent hasn't gamed metrics while degrading unmeasured properties:

```bash
# Full security audit
make vuln
make lint
make test-race

# Cross-tenant verification
# Manually test: create 2 customers, verify complete isolation
# Customer A sees ZERO of Customer B's resources across ALL endpoints

# RLS verification
# Connect as app_customer role, SET app.current_customer_id to Customer A
# SELECT from all RLS-enabled tables -- must return ONLY Customer A's data

# Docker posture
# Verify: non-root UID, no --privileged, cap_drop ALL, read-only rootfs (Controller/WebUIs)

# mTLS verification  
# Attempt gRPC call without client cert -- must be rejected

# WHMCS idempotency
# Call CreateAccount twice with same service_id -- must return same VM, not create duplicate

# Bandwidth enforcement
# Verify tc qdisc exists on VM tap interface, matches plan.port_speed_mbps

# nwfilter verification
# Verify anti-spoofing rules applied to every VM's tap interface
```

---

## Phase 5: Threat Model Maintenance

The `threats.tsv` file is a living document. Update it throughout the lifecycle.

### 5.1 Threat Categories (VirtueStack-Specific)

| Category | Description | Primary Defense |
|----------|-------------|------------------|
| `customer-abuse` | Customer exploiting platform beyond their plan | Plan limits, rate limiting, monitoring |
| `tenant-escape` | Customer A accessing Customer B's resources | RLS + application ownership + JWT scoping |
| `privilege-escalation` | Customer gaining admin access | JWT role validation, 2FA on admin, RBAC |
| `billing-fraud` | Getting resources without paying | WHMCS integration, idempotent provisioning |
| `resource-exhaustion` | Consuming disproportionate resources | Rate limits, plan limits, quotas |
| `network-abuse` | Using VM for attacks (DDoS, scanning, spoofing) | nwfilter, nftables, bandwidth caps |
| `data-exfiltration` | Extracting other customers' data | RLS, encrypted secrets, audit logging |
| `injection` | SQL injection, XSS, command injection | Parameterized queries, React escaping, input validation |
| `auth-bypass` | Accessing protected resources without auth | JWT validation, API key auth, mTLS |
| `supply-chain` | Compromised dependency | Pinned versions, vulnerability scanning, SBOM |

### 5.2 Threat Lifecycle

```
planned  -->  implemented  -->  verified  -->  monitored
   |               |               |               |
   |               |               |          (ongoing in production)
   |               |               |
   |               |          Phase 4 confirms defense works
   |               |
   |          Phase 2 implements the mitigation
   |
   Phase 1 identifies the threat
```

Every threat in `threats.tsv` must reach `verified` status before merge.

---

## Results Logging

### results.tsv Schema
```
attempt  unit     action                  result   duration_s  gates_passed                        notes
1        U-001    add RLS to new_table    KEEP     180         lint,vet,test,vuln,migrate          Clean pass
2        U-002    add customer endpoint   DISCARD  240         lint,vet                            test:race failed on concurrent snapshot creation
3        U-002    fix race with tx lock   KEEP     120         lint,vet,test,vuln                  Added SELECT FOR UPDATE
4        U-003    add WHMCS resize        KEEP     300         lint,vet,test,vuln                  Idempotency verified
```

### review.tsv Schema
```
commit      unit     verdict    findings                                                    reviewer
abc1234     U-001    PASS       All 13 checks pass. RLS verified.                          agent
def5678     U-002    SECURITY   R-04: verifySnapshotOwnership not called for restore path  agent
ghi9012     U-003    FIXME      R-05: N+1 query in plan lookup during resize batch         agent
```

### bugs.tsv Schema
```
bug_id   technique              severity     file                                          line  description                                          reproduction
B-001    customer-abuse         P0-critical  internal/controller/api/customer/snapshots.go  47    Missing ownership check on restore                  POST /snapshots/{other_customer}/restore
B-002    race-condition         P1-high      internal/controller/services/vm_service.go     128   TOCTOU on IP allocation                             Concurrent vm.create with last available IP
B-003    boundary               P2-medium    internal/nodeagent/storage/rbd.go              89    No validation on disk_gb=0                          CreateVM with plan.disk_gb=0
B-004    billing-integrity      P0-critical  modules/servers/virtuestack/virtuestack.php    312   CreateAccount not idempotent on service_id           Call CreateAccount twice, get 2 VMs
```

### threats.tsv Schema  
```
threat_id  category           attack_vector                                   likelihood  impact    mitigation                                         status
T-001      tenant-escape      Customer A GET /vms/{B's UUID}                  high        critical  Ownership check + RLS + 404 not 403                verified
T-002      billing-fraud      Double-call CreateAccount                       medium      high      Idempotency check on service_id                    verified
T-003      network-abuse      VM sends spoofed ARP packets                   high        high      nwfilter no-arp-spoofing on all tap interfaces     verified
T-004      customer-abuse     Snapshot spam to exhaust storage                medium      medium    plans.snapshot_limit + 409 on exceed               implemented
T-005      resource-exhaust   WebSocket connection flood                      medium      medium    Per-IP limit 10 concurrent + idle timeout 5min     planned
```

---

## Iterate On This File

This workflow is version-controlled. When you discover:
- A new attack vector: add it to the appropriate Technique in Phase 4
- A new quality gate: add it to Phase 2 evaluation
- A new domain: add it to the Domain Map
- A recurring bug pattern: add it to Crash Classification

**The highest-leverage work you can do is improve this file.** Every improvement compounds across all future development cycles.

---

## Quick Reference: Make Targets

```bash
make build                  # Build controller + node-agent
make build-controller       # Controller only
make build-node-agent       # Node Agent only
make test                   # Run all Go tests
make test-race              # Tests with race detector
make test-coverage          # HTML coverage report
make lint                   # golangci-lint
make vet                    # go vet
make vuln                   # govulncheck
make proto                  # Regenerate protobuf
make deps                   # go mod download + verify + tidy
make migrate-up             # Apply migrations
make migrate-down           # Rollback last migration
make migrate-create NAME=x  # New migration pair
```

## Quick Reference: E2E Tests

```bash
./scripts/setup-e2e.sh --start   # Setup + start services
cd tests/e2e && npm test         # All E2E tests
npm run test:admin               # Admin portal tests
npm run test:customer            # Customer portal tests  
npm run test:auth                # Auth tests
./scripts/setup-e2e.sh --clean   # Cleanup
```
