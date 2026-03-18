# TDD-Approach Prompt: VirtueStack Audit Fix Execution

## Context

VirtueStack's `todo.md` contains ~248 audit findings against `CODING_STANDARD.md`. ~160 are fixed, leaving **~90 unticked items** across 14 sections. This prompt defines a TDD workflow to validate, implement, and review each fix with code-reviewer delegation at validation and review checkpoints.

---

## Prerequisites (Read Before Each Session)

```
Read and internalize:
- /home/VirtueStack/CLAUDE.md          — Build commands, CI pipeline, project overview
- /home/VirtueStack/AGENTS.md          — Architecture, API endpoints, DB schema, file locations
- /home/VirtueStack/CODING_STANDARD.md — 19 Quality Gates (pass/fail criteria for every fix)
- /home/VirtueStack/todo.md            — Source of truth for all unticked items
```

---

## Available Skills & Commands

Two skill collections are installed: **superpowers** (workflow discipline) and **ECC** (language-specific patterns).

### Superpowers Skills (Workflow Discipline)

| Skill | When to Use |
|-------|-------------|
| `superpowers:test-driven-development` | Before writing any implementation code |
| `superpowers:systematic-debugging` | When encountering bugs, test failures, or unexpected behavior |
| `superpowers:brainstorming` | Before creative work — features, components, behavior changes |
| `superpowers:requesting-code-review` | After completing tasks or before merging |
| `superpowers:receiving-code-review` | When receiving code review feedback |
| `superpowers:verification-before-completion` | Before claiming work is complete |
| `superpowers:dispatching-parallel-agents` | When facing 2+ independent tasks |
| `superpowers:using-git-worktrees` | When starting feature work needing isolation |

### ECC Commands (Slash Commands)

| Command | Purpose | Use In Batch |
|---------|---------|--------------|
| `/tdd` | Enforce TDD workflow, scaffold tests | Step 2 (Red Phase) |
| `/go-test` | TDD for Go: table-driven tests first | Step 2 (Go code) |
| `/go-review` | Comprehensive Go code review | Step 5 (Review) |
| `/go-build` | Fix Go build errors, vet warnings, linter issues | Step 4 (Refactor) |
| `/quality-gate` | Run quality gates against codebase | Step 5 (Review) |
| `/verify` | Comprehensive verification | Step 7 (Before commit) |
| `/code-review` | General code review | Step 5 (Review) |
| `/security-review` | Security-focused review | Batch 1 (Security) |
| `/test-coverage` | Analyze test coverage | Step 2 (Tests) |
| `/build-fix` | Fix build errors across languages | Step 4 (Refactor) |
| `/refactor-clean` | Clean up after refactoring | Step 4 (Refactor) |
| `/checkpoints` | Save/restore session state | Any step |
| `/multi-backend` | Parallel backend-focused development | Parallelization |
| `/multi-frontend` | Parallel frontend-focused development | Batch 9 |
| `/devfleet` | Orchestrate parallel Claude agents | Parallelization |

### ECC Agents

| Agent | When to Dispatch |
|-------|------------------|
| `go-reviewer` | Go code review (Step 5) |
| `go-build-resolver` | Go build errors (Step 4) |
| `security-reviewer` | Security review (Batch 1) |
| `code-reviewer` | General code review (Step 5) |
| `tdd-guide` | TDD guidance (Step 2) |
| `architect` | Architecture decisions (planning) |
| `planner` | Implementation planning |

### ECC Skills (Language/Domain Patterns)

| Skill | Purpose |
|-------|---------|
| `golang-patterns` | Idiomatic Go patterns, best practices |
| `golang-testing` | Go testing patterns: table-driven, subtests, mocks |
| `postgres-patterns` | PostgreSQL patterns: query optimization, indexing |
| `docker-patterns` | Docker/Compose patterns |
| `deployment-patterns` | CI/CD pipeline patterns |
| `security-review` | Security audit patterns |
| `api-design` | REST API design patterns |
| `backend-patterns` | Backend architecture patterns |
| `verification-loop` | Comprehensive verification system |

### Choosing Between Overlapping Skills

| Task | Superpowers | ECC | Recommendation |
|------|-------------|-----|----------------|
| TDD workflow | `test-driven-development` | `/tdd`, `/go-test` | Use `/go-test` for Go (more specific) |
| Code review | `requesting-code-review` | `/go-review`, `/code-review` | Use `/go-review` for Go, superpowers for process |
| Debugging | `systematic-debugging` | — | Use superpowers (structured approach) |
| Security | — | `/security-review`, `security-review` | Use ECC (specialized) |
| Verification | `verification-before-completion` | `/verify`, `verification-loop` | Use both: ECC for scope, superpowers for discipline |

---

## Code Navigation with codebase-memory-mcp

**IMPORTANT:** Use the codebase-memory-mcp tools for all code exploration and impact analysis. Do NOT rely on grep/glob alone for understanding code relationships.

### When to Use codebase-memory-mcp

| Scenario | Tool | Purpose |
|----------|------|---------|
| **Finding callers before refactoring** | `trace_call_path(function_name="FunctionName", direction="inbound")` | Ensures all callers are identified and updated |
| **Impact analysis before changes** | `detect_changes(scope="all", depth=3)` | Understands blast radius of uncommitted changes |
| **Structural search for patterns** | `search_graph(name_pattern=".*Handler.*", label="Function")` | Finds functions/classes by name regex |
| **Architecture overview** | `get_architecture(aspects=["all"])` | Languages, packages, entry points, routes, hotspots |

### Required Usage Points

1. **Step 1 (Validate):** Before validating an item, use `detect_changes` to understand what depends on the code being modified.

2. **Step 2 (Write Tests):** Use `trace_call_path(direction="inbound")` to find all callers of the function being tested, ensuring characterization tests cover all call sites.

3. **Step 3 (Implement Fix):** After refactoring, use `trace_call_path(direction="outbound")` to verify the new function calls what it should.

4. **Step 5 (Review):** Use `get_architecture` and `search_graph` to generate comprehensive review context with impact analysis.

### Example Workflow

```
# Before refactoring MigrateVM function:
1. Index repo: index_repository(repo_path="/home/VirtueStack")
2. Find callers: trace_call_path(function_name="MigrateVM", direction="inbound", depth=3)
3. Get blast radius: detect_changes(scope="unstaged", depth=2)
4. Proceed with refactoring knowing all affected code
```

### Available Tools (14 total)

| Tool | Purpose |
|------|---------|
| `index_repository` | Index repo into knowledge graph |
| `index_status` | Check indexing status |
| `list_projects` | List all indexed projects |
| `search_graph` | Structural search with filters (label, name_pattern, relationship) |
| `search_code` | Grep-like text search within indexed files |
| `trace_call_path` | BFS call chain traversal (inbound/outbound/both) |
| `detect_changes` | Git diff → affected symbols + blast radius with risk classification |
| `query_graph` | Execute Cypher-like graph queries |
| `get_graph_schema` | Node/edge counts and relationship patterns |
| `get_code_snippet` | Read source code by qualified name |
| `get_architecture` | Codebase overview (languages, packages, entry points, routes, hotspots, clusters) |
| `manage_adr` | CRUD for Architecture Decision Records |
| `ingest_traces` | Ingest OpenTelemetry JSON traces |
| `delete_project` | Remove a project and all its graph data |

---

## Per-Batch Workflow (7 Steps)

### Step 1: Validate — Delegate code-reviewer FIRST

**Before validating, use codebase-memory-mcp:**
```
1. index_repository() — Ensure graph is current
2. detect_changes(scope="unstaged", depth=3) — Understand blast radius
3. trace_call_path(function_name="FunctionName", direction="inbound") — Find all callers
```

**Choose validation approach:**

| Scenario | Use | Command/Skill |
|----------|-----|---------------|
| Security items (Batch 1) | ECC security-reviewer agent | `security-reviewer` |
| Go code review | ECC go-reviewer agent | `go-reviewer` |
| General validation | superpowers code-reviewer | `superpowers:code-reviewer` |

**For security items (Batch 1):**
```
Dispatch ECC `security-reviewer` agent with:
- File paths and line numbers from todo.md
- CODING_STANDARD.md QG-02 requirements
- Request: "Validate if this security issue still exists"
```

**For Go code (Batches 2-8):**
```
Dispatch ECC `go-reviewer` agent with:
- File paths and line numbers
- Request: "Check if these issues are still present in current codebase"
```

**General prompt template:**
```
Validate whether these audit findings are STILL real issues in the current codebase.
For each item:
1. Read the file at the specified path and line numbers
2. Determine if already fixed (by prior work or shifted code)
3. If still present, confirm exact current line numbers
4. If fixed, mark SKIP with evidence

Items to validate:
[PASTE ITEMS FROM THE CURRENT BATCH]

Output per item:
- ITEM: [description]
  STATUS: STILL_PRESENT | ALREADY_FIXED | PARTIALLY_FIXED
  CURRENT_LOCATION: [file:line-line]
  EVIDENCE: [what you found]
```

### Step 2: Write Failing Tests (TDD Red Phase)

**Use codebase-memory-mcp to understand test scope:**
```
trace_call_path(function_name="FunctionToRefactor", direction="inbound", depth=3)
# This reveals all call sites that must be covered by characterization tests
```

**For Go code, invoke ECC /go-test or /tdd command:**
```
/go-test — Enforces TDD workflow for Go with table-driven tests
/tdd — General TDD workflow scaffolding
```

**Or use superpowers skill:**
```
superpowers:test-driven-development — Structured TDD approach
```

For each STILL_PRESENT item:

| Item Type | Test Strategy | Expected Initial State |
|-----------|--------------|----------------------|
| **Function extraction (QG-01)** | Write characterization tests capturing current behavior (happy path + error paths) | Tests PASS now, must PASS after refactoring |
| **Security fix (QG-02)** | Write negative test proving vulnerability exists | Tests FAIL now (red), proving the gap |
| **Typed structs (QG-03)** | Write JSON round-trip test: marshal old return value, assert field types | Tests FAIL now (wrong types) |
| **Missing tests (QG-14)** | Write the test coverage itself | Tests FAIL (red) or are new |
| **DRY extraction (QG-06)** | Write dual-callsite test: both callers produce same output via shared fn | Tests FAIL (shared fn doesn't exist yet) |
| **Determinism (QG-17)** | Write test with injected fake clock | Tests FAIL (no clock interface yet) |
| **Performance (QG-16)** | Write pagination contract test | Tests FAIL (cursor pagination not implemented) |

**Rules:**
- Table-driven tests per `CODING_STANDARD.md` Section 10
- Use `testify` (require/assert) matching existing patterns
- Hand-written mocks (no codegen) matching `auth_2fa_test.go` pattern
- Run `go test -run TestNewFunction ./path/...` to verify expected state (red or green for characterization)

### Step 3: Implement Fix (TDD Green Phase)

**After refactoring, verify all callers updated:**
```
trace_call_path(function_name="NewExtractedFunction", direction="inbound")
# Confirms new function is properly wired to all callers
```

Implement the **minimum change** to make failing tests pass:

- **QG-01**: Extract helper functions. Do NOT change behavior. Same preconditions/postconditions.
- **QG-02**: Implement the defense. Match CODING_STANDARD requirements exactly.
- **QG-03**: Define typed struct, update signature, update all callers. JSON wire format must NOT change.
- **QG-06**: Extract shared function, update both call sites to use it.
- **QG-17**: Add Clock interface, default to `time.Now`, accept injected clock in tests.

Run `go test -run TestNewFunction ./path/...` after each change to verify GREEN.

### Step 4: Refactor (TDD Refactor Phase)

- Improve naming if initial extraction used placeholder names
- Ensure doc comments on all new exported types/functions
- Verify import grouping: stdlib > third-party > local
- Run full package tests: `go test -race ./path/...`
- Run `make lint` — zero warnings required

### Step 5: Review — Delegate code-reviewer AFTER

**Use codebase-memory-mcp for comprehensive review context:**
```
detect_changes(scope="all", depth=2)
get_architecture(aspects=["hotspots", "boundaries"])
# Generates token-efficient review context with impact analysis
```

**Choose review approach:**

| Scenario | Use | Command/Skill |
|----------|-----|---------------|
| Security fixes (Batch 1) | ECC `/security-review` | Slash command |
| Go code (Batches 2-8) | ECC `/go-review` | Slash command |
| Quality gates check | ECC `/quality-gate` | Slash command |
| General review | superpowers `requesting-code-review` | Skill |
| Verification | ECC `/verify` | Slash command |

**For Go code, run /go-review:**
```
/go-review
# Comprehensive Go code review checking:
# - Idiomatic patterns
# - Error handling
# - Concurrency issues
# - Performance concerns
# - CODING_STANDARD.md QG compliance
```

**For security-sensitive code, run /security-review:**
```
/security-review
# Checks for:
# - Input validation
# - Authentication/authorization
# - Secrets handling
# - SQL injection, XSS
# - OWASP Top 10
```

**Dispatch superpowers:code-reviewer agent for process review:**
```
Review the implementation against CODING_STANDARD.md quality gates.
Check each changed file against ALL 19 QGs. Focus on:
- QG-01: Functions <= 40 lines, nesting <= 3, params <= 4
- QG-02: No new security issues
- QG-03: No bare interface{}/any without justification
- QG-04: All errors handled with typed errors
- QG-06: No duplication introduced
- QG-07: No silent failures
- QG-10: No WHAT-not-WHY comments
- QG-11: Doc comments on exports

Output:
- PASS: [QGs that pass]
- FAIL: [QG-XX] [file:line] [violation description]
- SUGGESTION: [optional non-blocking improvement]
```

### Step 6: Correct Based on Review

For each FAIL from Step 5:
1. Make the correction
2. Run tests to verify nothing broke
3. If correction requires a new test, go back to Step 2 for that item

### Step 7: Commit

**Run verification commands before commit:**

```bash
# ECC verification (comprehensive)
/verify

# Quality gate check
/quality-gate

# Or run manually:
make test                    # or: go test -race ./...
make lint
# For frontend: cd webui/admin && npm run lint && npm run type-check && npm run build
```

**Then commit:**
```bash
git add [specific files]
git commit -m "fix(scope): description (QG-XX)"
```

---

## Batch Execution Order (10 Batches)

Execute in this order — security first, structural second, cosmetic last:

### Batch 1: Security Hardening (CRITICAL)

**Items:**
- [ ] `docker-compose.prod.yml:124-125` — `sslmode=disable` in production (QG-02)
- [ ] `tasks/webhook_deliver.go:45` — EncryptionKey as plaintext string (QG-02)
- [ ] `proto/node_agent.proto` — no field validation annotations (QG-05, QG-02)
- [ ] `nodeagent/server.go` — GuestExecCommand no command allowlist (QG-02)
- [ ] `nginx/conf.d/default.conf:67` — CSP script-src may break Next.js (QG-02)
- [ ] `configs/prometheus/prometheus.yml` — metrics scraped over plain HTTP (QG-02, QG-18)
- [ ] `migrations/000011_password_resets.up.sql` — RLS gap between grant and enable (QG-02)

**TDD:** Write tests proving each vulnerability exists (FAIL), implement fix, verify tests PASS.

### Batch 2: Service Layer Decomposition (QG-01 — Services)

**Items:**
- [ ] `services/migration_service.go` MigrateVM (146 lines), findBestTargetNode (80 lines)
- [ ] `services/failover_service.go` ApproveFailover (124 lines), releaseRBDLocks (75 lines)
- [ ] `services/backup_service.go` runSchedulerTick (79 lines)
- [ ] `services/template_service.go` Import (70 lines)
- [ ] `services/backup_service.go:30` BackupNodeAgentClient 12-method interface (QG-01)
- [ ] `services/bandwidth_service.go` CheckAllVMs loads all VMs (QG-16)

**Also (non-TDD):**
- [ ] Multiple files — magic numbers (QG-12, QG-01)
- [ ] `services/rdns_service_test.go` — only 3 tests (QG-14)

**TDD:** Write characterization tests for each function BEFORE extracting. Tests PASS before and after.

### Batch 3: Task Handler Decomposition (QG-01 — Tasks)

**Items:**
- [ ] `tasks/handlers.go` handleVMCreate (209 lines)
- [ ] `tasks/vm_reinstall.go` handleVMReinstall (227 lines)
- [ ] `tasks/migration_execute.go` handleVMMigrate (171 lines)
- [ ] `tasks/` 19+ functions exceeding 40 lines
- [ ] `tasks/backup_create.go:138,240` — 9 parameters (QG-01)
- [ ] `tasks/migration_execute.go:195,240,273,365,437` — 8 parameters (QG-01)
- [ ] `notifications/email.go:104-275` loadTemplates (172 lines)

**Also:**
- [ ] All task handlers — task results as `map[string]any{}` (QG-03)
- [ ] All task handler files — no operation journaling (QG-04)

**TDD:** Extend `handlers_test.go` with characterization tests. Use mock NodeAgentClient pattern.

### Batch 4: Admin API Decomposition (QG-01/03 — Admin API)

**Items:**
- [ ] `api/admin/` 30+ functions exceeding 40 lines
- [ ] `api/admin/` 15 occurrences of ad-hoc `gin.H{}` (QG-03)

**Also (non-TDD):**
- [ ] `api/admin/failover.go, nodes.go, rdns.go` — missing doc comments (QG-11)

**TDD:** Write HTTP-level tests using `httptest` + `gin.TestMode` pattern from `auth_test.go`.

### Batch 5: Customer API Decomposition + DRY (QG-01/06 — Customer API)

**Items:**
- [ ] `api/customer/` 39 functions exceeding 40 lines
- [ ] `websocket.go:305-485` nesting > 3 levels
- [ ] `websocket.go:271-485` proxyVNCStream/proxySerialStream near-identical (QG-06)
- [ ] `backups.go:266-269, snapshots.go:265-268` identical ownership verifiers (QG-06)
- [ ] `iso_upload.go:43-174` UploadISO (132 lines)
- [ ] `routes.go:94-216` RegisterCustomerRoutes (123 lines)

**Also:**
- [ ] `auth_test.go` — only ChangePassword covered (QG-14)
- [ ] `provisioning/password.go:182-219` — validatePasswordStrength returns `fmt.Errorf` not typed error (QG-04)

**Also (non-TDD):**
- [ ] `profile.go, twofa.go` — missing doc comments (QG-11)

**TDD:** Extend `auth_test.go` to cover ISO upload, 2FA, API keys, power ops, backups, snapshots, rDNS, webhooks.

### Batch 6: Provisioning API + Middleware (QG-01)

**Items:**
- [ ] `api/provisioning/` 14 functions exceeding 40 lines (ResizeVM 117, PowerOperation 80, SetPassword 73)

**TDD:** Write HTTP-level tests for handler decomposition. Verify request/response contract preserved.

### Batch 7: Node Agent Decomposition + DRY (QG-01/06)

**Items:**
- [ ] `nodeagent/` 20 functions exceeding 40 lines
- [ ] Duplicate XML domain interface parsing struct (4 places) (QG-06)
- [ ] Duplicate `isLibvirtError` (2 packages) (QG-06)
- [ ] `lifecycle.go:871-968` getNetworkStatsFromXML/getNetworkStatsFullFromXML near-duplicate (QG-06)
- [ ] `lifecycle.go:735-777` getMemoryUsage nesting depth (QG-01)
- [ ] `domain_xml.go:103` hardcoded emulator path (QG-12)

**Also (non-TDD):**
- [ ] `metrics/prometheus.go, vm/metrics.go` — missing doc comments (QG-11)

**TDD:** Extend `domain_xml_test.go` patterns. Write tests for each function before extracting.

### Batch 8: Typed Structs + Error System (QG-03/10/17)

**Items:**
- [ ] `circuit_breaker.go:230`, `node_service.go:454`, `backup_service.go:53,129`, etc. — map[string]interface{} returns (QG-03)
- [ ] Task results as `map[string]any{}` — define `VMCreateResult`, `BackupCreateResult`, etc. (QG-03)
- [ ] `shared/errors/errors.go:176` As() bare any (QG-03)
- [ ] `shared/errors/errors.go:170-178` Is/As re-exports confusion (QG-10)
- [ ] `models/task.go:54,61,71,91` time.Now() in model methods (QG-17)

**TDD:** Write JSON round-trip tests proving wire format unchanged after type migration. Write Clock injection tests.

### Batch 9: Frontend Decomposition + DRY (QG-01/06/15)

**Items:**
- [ ] `customer/settings/page.tsx` (1,374 lines) — extract ProfileTab, SecurityTab, ApiKeysTab, WebhooksTab
- [ ] `customer/vms/[id]/page.tsx` (1,486 lines) — extract VMControls, VMBackupsTab, etc.
- [ ] `admin/ip-sets/page.tsx` (742 lines) — extract dialogs
- [ ] `admin/plans/page.tsx` (486 lines) — extract PlanEditDialog
- [ ] Duplicate navItems, formatBytes, VM action pattern, onError toast, profile-fetching (QG-06)
- [ ] `package.json` ^ ranges (admin: 95, customer: 37) (QG-15)
- [ ] Missing `webui/admin/.env.example` (QG-12)
- [ ] Various: `import *`, eslint-disable, feature flags, 2FA user object

**TDD:** No frontend test infrastructure exists. Verification via `npm run lint && npm run type-check && npm run build`. TypeScript compiler IS the test.

### Batch 10: Infrastructure, Migrations, Observability (QG-13/15/17/18/19)

**Items:**
- [ ] All Dockerfiles — mutable image tags, pin to SHA digest (QG-15, QG-19)
- [ ] `.github/workflows/ci.yml` — no SBOM/signing/SLSA (QG-17)
- [ ] `docker-compose.yml:13-61` — Postgres/NATS lack user directives (QG-19)
- [ ] Missing Prometheus alerts: health check, circuit breaker, disk, cert expiry (QG-18)
- [ ] Grafana missing error rate panel (QG-18)
- [ ] Prometheus static targets only (QG-18)
- [ ] `.env.example` version mismatch (QG-12)
- [ ] Docker network subnet /16 oversized (QG-12)
- [ ] Dockerfile.controller:91 — Alpine vs distroless (QG-19)
- [ ] Migration 008 webhook indexes superseded comment (QG-13)
- [ ] Migration 001+025 duplicate plan limit columns (QG-07)
- [ ] Migration 009 notification_events ON DELETE behavior (QG-07)
- [ ] Migration 011 password_resets RLS gap (QG-02)
- [ ] Migration 020 failover_requests FK ON DELETE (QG-07)
- [ ] Audit log partition automation (QG-16)
- [ ] All repository list methods — cursor-based pagination (QG-16)

**TDD:** Migrations tested via `make migrate-up && make migrate-down`. Infra verified by `docker compose build`. Cursor pagination requires new tests per repo.

---

## Items That Do NOT Need TDD

These are verified by tooling, not tests:

| Category | Items | Verification |
|----------|-------|-------------|
| Doc comments (QG-11) | ~8 items across models, API, nodeagent, shared | `make lint` (revive exported rule) |
| Config files | .env.example, docker-compose user directives, Prometheus configs | Manual review + `docker compose config` |
| CI pipeline | SBOM, signing, action pinning | CI pipeline pass |
| Frontend lint fixes | `import *`, eslint-disable, WHAT comments | `npm run lint` |
| Migration annotations | Comment on superseded indexes, ON DELETE documentation | Code review |

---

## Testing Infrastructure Reference

| Resource | Location | Pattern |
|----------|----------|---------|
| Go unit test example | `internal/controller/services/auth_2fa_test.go` | Table-driven, hand-written mocks, testify |
| HTTP handler test example | `internal/controller/api/middleware/auth_test.go` | gin.TestMode, httptest, mock handler |
| Integration test suite | `tests/integration/suite_test.go` | testcontainers (postgres+nats), global fixtures |
| Domain XML tests | `internal/nodeagent/vm/domain_xml_test.go` | XML comparison, config-driven |
| Crypto/util tests | `internal/shared/crypto/crypto_test.go` | Table-driven, boundary values |

**Test Fixtures:**
```go
TestCustomerID = "00000000-0000-0000-0000-000000000001"
TestAdminID    = "00000000-0000-0000-0000-000000000002"
TestPlanID     = "00000000-0000-0000-0000-000000000003"
TestTemplateID = "00000000-0000-0000-0000-000000000004"
TestNodeID     = "00000000-0000-0000-0000-000000000005"
TestVMID       = "00000000-0000-0000-0000-000000000006"
```

---

## Parallelization Opportunities

```
Batch 1 (Security) ──sequential──> Batch 2 (Services) ──> Batch 3 (Tasks)
                                         |
                                         v
                                    Batch 8 (Types)
                                         |
                                    ┌────┼────┐
                                    v    v    v
                              Batch 4  Batch 5  Batch 6    (parallelizable)
                                    └────┬────┘
                                         v
                                    Batch 7 (Node Agent)   (independent)
                                         |
                                    Batch 9 (Frontend)     (independent)
                                         |
                                    Batch 10 (Infra)       (last)
```

Batches 4, 5, 6 modify different `api/` subdirectories and can run in parallel worktrees.
Batch 7 and Batch 9 are independent of controller changes.

### Parallelization with ECC Commands

| Scenario | ECC Command | Purpose |
|----------|-------------|---------|
| Backend parallel work | `/multi-backend` | Parallel backend-focused development |
| Frontend parallel work | `/multi-frontend` | Parallel frontend-focused development |
| Multi-model collaboration | `/multi-workflow` | Multi-model collaborative development |
| Agent orchestration | `/devfleet` | Orchestrate parallel Claude agents |

### Using git worktrees for parallel batches

Invoke `superpowers:using-git-worktrees` skill before starting parallel batch execution:
```
1. Create worktree for Batch 4 (Admin API)
2. Create worktree for Batch 5 (Customer API)
3. Create worktree for Batch 6 (Provisioning API)
4. Execute in parallel using separate Claude sessions
5. Merge worktrees back to main
```

### Using /devfleet for parallel agents

```
/devfleet
# Orchestrates multiple Claude Code agents in parallel
# Useful for running Batches 4, 5, 6 simultaneously
```

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Refactoring introduces behavioral change | Characterization tests lock current behavior BEFORE extraction |
| Type changes break JSON wire format | JSON round-trip tests compare old output with new typed output |
| Security fix breaks existing functionality | Tests cover BOTH secure path (reject bad) AND happy path (accept good) |
| Frontend decomposition breaks rendering | TypeScript type-checker + production build as safety net |
| Migration corrupts data | Every migration has down.sql; integration tests apply up+down+up |

---

## Quick Reference: Skills/Commands per Batch

| Batch | Primary Skills/Commands |
|-------|------------------------|
| **1: Security** | `/security-review`, `security-reviewer` agent, `superpowers:systematic-debugging` |
| **2: Services** | `/go-test`, `/go-review`, `golang-patterns`, `golang-testing` |
| **3: Tasks** | `/go-test`, `/go-review`, `superpowers:test-driven-development` |
| **4: Admin API** | `/go-test`, `/go-review`, `/multi-backend` (if parallel) |
| **5: Customer API** | `/go-test`, `/go-review`, `/multi-backend` (if parallel) |
| **6: Provisioning** | `/go-test`, `/go-review`, `/multi-backend` (if parallel) |
| **7: Node Agent** | `/go-test`, `/go-review`, `golang-patterns` |
| **8: Typed Structs** | `/go-test`, `/go-review`, `superpowers:test-driven-development` |
| **9: Frontend** | `/multi-frontend`, `frontend-patterns`, `superpowers:brainstorming` |
| **10: Infra** | `docker-patterns`, `deployment-patterns`, `/verify` |

---

## Session Management

| Command | Purpose |
|---------|---------|
| `/checkpoint` | Save current session state |
| `/save-session` | Save session to dated file |
| `/resume-session` | Load most recent session |
| `/sessions` | Manage session history |
| `/instinct-status` | Show learned patterns |
| `/learn` | Extract reusable patterns from session |
