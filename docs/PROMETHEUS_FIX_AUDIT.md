# Prometheus Plan: Fix All VirtueStack Audit Issues

Paste this into opencode as a single prompt to `@prometheus`.

---

```
@prometheus

I have a comprehensive codebase audit report at `docs/CODEBASE_AUDIT_REPORT.md`. The audit found 83+ unfinished markers across 76+ files. The codebase is an AI-generated VPS hosting platform (Go 1.23 backend + Next.js 15 frontend + PostgreSQL + NATS + gRPC).

I need you to create a work plan that fixes EVERY issue in the audit report. Requirements are already fully gathered -- skip the interview phase and go straight to plan generation. All context is below.

## Project Context

- **Repo**: AbuGosok/VirtueStack
- **Backend**: Go 1.23, Gin framework, gRPC (controller + node-agent architecture)
- **Frontend**: Next.js 15 App Router, TypeScript, shadcn/ui, Tailwind CSS (two portals: admin + customer)
- **Database**: PostgreSQL with partitioned audit_logs, golang-migrate
- **Messaging**: NATS JetStream
- **Virtualization**: libvirt/KVM via node-agent
- **Auth**: JWT-based (currently stubbed/mocked)
- **Monorepo structure**: `cmd/`, `internal/`, `webui/admin/`, `webui/customer/`, `migrations/`, `proto/`, `tests/`

## Pre-Gathered Requirements (from audit)

The audit categorizes 83+ issues into 4 severity levels. Here is the COMPLETE list of everything that needs fixing:

---

### PHASE 1: Security & Authentication (CRITICAL -- blocks all production)

**1.1 Fix Password Hashing** [CRITICAL]
- File: `internal/controller/tasks/handlers.go:701-707`
- Current: Placeholder SHA-512 that concatenates plaintext password with static prefix/suffix
- Required: Replace with bcrypt or Argon2id, proper salt generation, password strength validation
- Acceptance: `go test ./internal/controller/tasks/... -run TestHashPassword` passes, passwords are non-reversible, bcrypt cost factor >= 12

**1.2 Implement Admin Portal Authentication** [CRITICAL]
- File: `webui/admin/app/login/page.tsx:45-58`
- Current: Mock login with `setTimeout(1000)`, no API call, no token storage
- Required: Connect to `POST /api/v1/admin/auth/login`, JWT token generation + storage + refresh, proper error handling
- Acceptance: Login form calls real API, tokens stored in httpOnly cookies, invalid credentials show error, token refresh works silently

**1.3 Implement Customer Portal Authentication** [CRITICAL]
- File: `webui/customer/app/login/page.tsx:45-58`
- Current: Same mock pattern as admin
- Required: Connect to `POST /api/v1/customer/auth/login`, shared auth service with admin, customer-specific token handling
- Acceptance: Same criteria as 1.2 but for customer routes

**1.4 Secure Provisioning API** [CRITICAL]
- Files: `internal/controller/api/provisioning/routes.go:107`, `internal/controller/grpc_client.go:202`
- Current: Contains `WARNING: Do not use in production without additional authentication`
- Required: Add API key authentication middleware, rate limiting, request signing, remove all WARNING comments
- Acceptance: Unauthenticated requests return 401, rate limit returns 429 after threshold, no WARNING strings in codebase

**1.5 Implement Password Reset** [CRITICAL]
- File: `internal/controller/services/auth_service.go:421-440`
- Current: Returns `fmt.Errorf("password reset not yet fully implemented")`
- Required: Create `password_resets` table + migration, token generation (crypto/rand), email notification, 24h expiration
- Acceptance: Full reset flow works end-to-end, expired tokens rejected, used tokens invalidated

**1.6 Fix Customer Updates Not Persisting** [CRITICAL]
- File: `internal/controller/services/customer_service.go:81-94`
- Current: Logs change but returns error `"customer update not yet implemented in repository"`
- Required: Implement `repository.Update` method, field validation, audit logging
- Acceptance: Customer profile changes persist to DB, audit log entry created, validation rejects invalid input

---

### PHASE 2: Core Backend Features (HIGH)

**2.1 Node Failover** [HIGH]
- File: `internal/controller/services/node_service.go:217-219`
- Current: Empty function body -- no alerts, no VM migration, no IPMI
- Required: Alert notification service (email/webhook), VM migration to healthy nodes, IPMI power cycle if configured, circuit breaker to prevent flapping
- Acceptance: Node failure triggers alert within 30s, VMs auto-migrate if healthy node available, IPMI cycle attempted if configured

**2.2 VM Migration** [HIGH]
- File: `internal/controller/api/admin/vms.go:340-348`
- Current: Returns `{"message": "VM migration initiated"}` without actually migrating
- Required: Implement migration service with live migration support, pre-migration checks (resource availability, network compatibility), rollback on failure
- Acceptance: VM actually moves to target node, state preserved, rollback if target node lacks resources

**2.3 Template Updates** [HIGH]
- File: `internal/controller/api/admin/templates.go:186-189`
- Current: Logs change but returns `"template update not yet implemented"`
- Required: Implement `repository.Update` for templates, add versioning, audit logging
- Acceptance: Template changes persist, version incremented, audit trail exists

**2.4 API Key Management** [HIGH]
- File: `internal/controller/api/customer/apikeys.go:38-49`
- Current: Always returns empty array `[]`
- Required: Create `customer_api_keys` table + migration, CRUD operations, key hashing (show key only once on creation), permission system
- Acceptance: Customer can create/list/revoke API keys, keys are hashed in DB, permissions enforced

**2.5 Snapshot Operations** [HIGH]
- File: `internal/controller/api/customer/snapshots.go:206-218`
- Current: Returns success but doesn't interact with storage backend
- Required: Connect to backup service, implement create/revert/delete via libvirt, progress tracking, quota enforcement
- Acceptance: Snapshot actually created on disk, revert restores VM state, quota prevents exceeding limit

**2.6 Node Metrics (Disk & Ceph)** [HIGH]
- File: `internal/nodeagent/server.go:226,231,290-291`
- Current: `DiskPercent: 0`, `TotalDiskGB: 0`, `UsedDiskGB: 0`, `CephConnected: false`
- Required: Implement disk usage via statfs/df, Ceph connection health check, cache metrics (5s TTL)
- Acceptance: Disk metrics return real values, Ceph status reflects actual connection state

**2.7 Frontend VM Controls** [HIGH]
- Files: `webui/customer/app/vms/page.tsx:120-135`, `webui/customer/app/vms/[id]/page.tsx:130-145`
- Current: All 4 buttons (start/stop/force-stop/restart) only `console.log`
- Required: Implement API client, connect all buttons to backend, loading states, error handling, confirmation dialog for destructive actions
- Acceptance: Each button triggers real API call, loading spinner during operation, error toast on failure, confirm dialog for stop/force-stop

**2.8 Admin Actions** [HIGH]
- Files: `webui/admin/app/nodes/page.tsx:199,205,212`, `webui/admin/app/customers/page.tsx:146,152,159`, `webui/admin/app/plans/page.tsx:162,172`
- Current: All handlers only `console.log`
- Required: Connect node view/drain/failover, customer suspend/unsuspend/delete, plan edit/delete to backend APIs
- Acceptance: Each action calls real API, confirmation dialogs for destructive actions, success/error feedback

**2.9 VM Management Tabs** [HIGH]
- File: `webui/customer/app/vms/[id]/page.tsx:320,341,359,377`
- Current: 4 tabs show placeholder text: Console, Backups, Snapshots, Settings
- Required: VNC/noVNC console integration, backup management UI connected to API, snapshot list + controls, VM settings configuration panel
- Acceptance: Console tab opens working VNC session, backup/snapshot tabs show real data with CRUD controls, settings tab allows config changes

**2.10 Missing Down Migration** [HIGH]
- Missing: `migrations/000010_webhooks.down.sql`
- Required: Create with DROP TABLE for `webhooks` and `webhook_deliveries`
- Acceptance: `migrate down` from step 10 succeeds without error

**2.11 Hardcoded Audit Log Partitions** [HIGH]
- File: `migrations/000001_initial_schema.up.sql:167-172`
- Current: Only March + April 2026 partitions hardcoded
- Required: Add partitions through 2027+ OR implement automatic partition creation job (pg_partman or cron function)
- Acceptance: Audit logging works beyond April 2026, auto-partition creation tested

---

### PHASE 3: API Layer & Data (MEDIUM)

**3.1 Placeholder Network Metrics** [MEDIUM]
- File: `internal/controller/api/customer/metrics.go:143-195`
- Current: `generatePlaceholderNetworkData()` returns fake RxBytes/TxBytes
- Required: Connect to Prometheus/InfluxDB or aggregate from node-agent reports, implement data retention
- Acceptance: Charts display real bandwidth data, historical data queryable

**3.2 RBAC Re-auth Placeholder** [MEDIUM]
- File: `internal/controller/services/rbac_service.go:147-156`
- Current: `RequireReauthForDestructive` always returns true without checking timestamp
- Required: Track last re-auth timestamp in session, enforce 5-minute window, metadata storage
- Acceptance: Destructive actions within 5 min of re-auth proceed, older sessions prompt re-auth

**3.3 Provisioning Password Update** [MEDIUM]
- File: `internal/controller/api/provisioning/password.go:91-99`
- Current: Returns success without DB update
- Required: Implement `vmRepo.UpdatePassword`, encrypt before storage, validate password
- Acceptance: Password actually updated in DB, old password invalidated

**3.4 gRPC Service Registration** [MEDIUM]
- File: `internal/nodeagent/server.go:134`
- Current: Manual message definitions instead of generated protobuf
- Required: Generate Go code from proto files, register generated service, remove manual definitions
- Acceptance: `protoc` generates code, manual structs deleted, gRPC service works with generated types

**3.5 VM Uptime** [MEDIUM]
- File: `internal/nodeagent/vm/lifecycle.go:438`
- Current: `return 0, nil` always
- Required: Track VM start timestamps (persist across agent restarts), calculate uptime
- Acceptance: Running VM shows accurate uptime, uptime survives agent restart

**3.6 Unimplemented Route Placeholder** [MEDIUM]
- File: `internal/controller/server.go:321-328`
- Current: Returns `"coming in Phase 2"` for unimplemented routes
- Required: Replace with proper 404, or implement the actual routes, add dev flag to hide unfinished
- Acceptance: No route returns "coming in Phase 2" in production mode

**3.7 IP Set Creation** [MEDIUM]
- File: `webui/admin/app/ip-sets/page.tsx:169`
- Current: `console.log("Create new IP set")`
- Required: Implement create dialog, connect to backend, validation
- Acceptance: IP sets can be created from admin UI

**3.8 CSV Export** [MEDIUM]
- File: `webui/admin/app/audit-logs/page.tsx:269`
- Current: `console.log("Export CSV")`
- Required: Implement CSV generation with current filters, trigger download
- Acceptance: CSV downloads with correct filtered data

**3.9 Lint Suppressions** [MEDIUM]
- Files: `internal/controller/repository/ip_repo.go:276,327`, `internal/controller/repository/node_repo.go:154`
- Current: `//nolint:errcheck` on `tx.Rollback`
- Required: Add explanatory comments, log rollback errors at debug level, evaluate necessity
- Acceptance: Each nolint has a comment explaining why, rollback errors logged

**3.10 Hardcoded Default Passwords** [MEDIUM]
- Files: `docker-compose.yml:19`, `Makefile:24`, `.env.example:7`
- Current: `${POSTGRES_PASSWORD:-changeme}`
- Required: Remove default fallbacks from production configs, require explicit configuration, add startup validation
- Acceptance: App refuses to start with "changeme" password, `.env.example` documents but doesn't default

---

### PHASE 4: Cleanup & Testing (LOW)

**4.1 Test Hardcoded Credentials** [LOW]
- Files: `tests/integration/auth_test.go`, `tests/e2e/*.spec.ts`
- Required: Use test fixtures/factories, document as test-only, add pre-commit hook

**4.2 Console.log Cleanup** [LOW]
- 25+ instances across frontend
- Required: Remove or replace with structured logger, add eslint rule `no-console`

**4.3 Placeholder Form Text** [LOW]
- Required: Review and localize all placeholder strings

---

## Constraints & Guardrails

1. **Do NOT break existing working functionality** -- each fix must be backward-compatible or include a migration path
2. **Every fix must include its own test** -- unit test for backend, component test or E2E for frontend
3. **Follow existing code patterns** -- match the Gin handler style, repository pattern, Next.js App Router conventions already in the codebase
4. **Database changes require both up AND down migrations** -- no exceptions
5. **Security fixes (Phase 1) must be completed and verified before any Phase 2 work begins**
6. **Each phase must pass `go build ./...` and `npm run build` (both portals) before moving to next phase**
7. **No new dependencies without justification** -- prefer stdlib where possible (especially for crypto)
8. **All TODO/FIXME/PLACEHOLDER markers must be removed as each issue is fixed** -- zero markers remaining when done
9. **gRPC changes must regenerate proto files** -- `make proto` must succeed
10. **Frontend API calls must use a centralized API client** -- no scattered fetch() calls

## Scope

- **IN**: Every issue listed in CODEBASE_AUDIT_REPORT.md (83+ markers across 6 CRITICAL, 12 HIGH, 25 MEDIUM, 40+ LOW)
- **OUT**: New features not mentioned in the audit, UI redesign, performance optimization beyond what's broken, CI/CD pipeline setup

## Testing Strategy

- **Backend**: `go test ./...` must pass with >80% coverage on fixed files
- **Frontend**: `npm run build` must succeed with zero type errors; critical flows covered by Playwright E2E
- **Integration**: Auth flow tested end-to-end (login -> token -> protected route -> refresh -> logout)
- **Security**: No `grep -r "changeme\|placeholder\|TODO\|FIXME" --include="*.go" --include="*.ts" --include="*.tsx"` matches in non-test files after completion

## Definition of Done

The work plan is complete when:
1. Zero TODO/FIXME/PLACEHOLDER/STUB markers remain in non-test source files
2. `go build ./...` and `go test ./...` pass
3. Both `webui/admin` and `webui/customer` build with `npm run build`
4. All 6 critical security issues verified fixed
5. All API endpoints return real data (no mock/placeholder responses)
6. All frontend buttons trigger real API calls (no console.log stubs)
7. All migrations have matching up + down files
8. Audit log partitions extend beyond April 2026

Generate the work plan to `.sisyphus/plans/fix-audit-issues.md`. This is a large plan -- organize TODOs within each phase, maintain strict dependency ordering (Phase 1 before Phase 2, etc.), and include file paths + acceptance criteria for every single item.
```

---

## How to Use

1. Open the VirtueStack repo in opencode
2. Copy everything between the triple-backtick fences above
3. Paste into opencode chat
4. Prometheus will skip the interview (requirements are pre-gathered) and consult Metis, then generate the plan
5. When prompted, choose **High Accuracy Review** (Momus loop) -- this is a large plan and benefits from verification
6. After plan is finalized, run `/start-work fix-audit-issues` to begin execution via Atlas

## Expected Output

Prometheus will generate `.sisyphus/plans/fix-audit-issues.md` containing:
- 4 phases with strict ordering
- ~40 concrete TODOs with file paths
- Acceptance criteria for each TODO
- Dependency graph between tasks
- Estimated scope per phase
