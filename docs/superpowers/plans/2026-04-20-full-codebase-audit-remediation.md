# Full Codebase Audit Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audit the entire VirtueStack repository, fix confirmed safe-to-fix findings, and produce verification evidence plus a final residual-risk summary.

**Architecture:** The main session coordinates the audit, owns the canonical finding queue, and applies fixes only after validating reported issues. Independent sub-agents audit separate domains in parallel using a strict finding contract. Remediation proceeds in small batches with targeted verification after each batch.

**Tech Stack:** Go 1.26, Gin, PostgreSQL migrations, NATS JetStream, gRPC, Next.js 16, React 19, TypeScript 5.7, npm workspaces, PHP billing modules, Docker Compose, GitHub Actions, shell scripts.

**Spec:** `docs/superpowers/specs/2026-04-20-full-codebase-audit-design.md`

---

## File Structure

### Audit Artifacts

- Create: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md`  
  Canonical audit log containing baseline results, sub-agent findings, triage decisions, fix batches, verification commands, deferred items, and final summary.

- Modify: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md` throughout execution  
  Append findings and verification evidence after each task. This file is the source of truth for final reporting.

### Files Reviewed During Audit

- Reference: `AGENTS.md`
- Reference: `docs/coding-standard.md`
- Reference: `docs/codemaps/*.md`
- Reference: `Makefile`
- Reference: `.github/workflows/*.yml`
- Reference: `Dockerfile.*`
- Reference: `docker-compose*.yml`
- Reference: `cmd/**`
- Reference: `internal/**`
- Reference: `proto/virtuestack/node_agent.proto`
- Reference: `migrations/*.sql`
- Reference: `webui/**`
- Reference: `modules/**/*.php`
- Reference: `scripts/**`
- Reference: `docs/api-reference.md`
- Reference: `docs/architecture.md`
- Reference: `docs/installation.md`

### Files Modified By Remediation

The exact remediation files are determined only after triage. Before editing any file, the coordinator must:

1. Confirm the finding is `confirmed-fix-now`.
2. Check whether the file has unrelated local changes with `rtk proxy git status --short -- <path>`.
3. Read the surrounding code.
4. Edit with `apply_patch` unless a formatter or package manager performs a mechanical update.
5. Record the file path, reason, and verification command in the audit log.

---

## Task 1: Establish Baseline And Create The Audit Log

**Files:**
- Create: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md`
- Reference: `docs/superpowers/specs/2026-04-20-full-codebase-audit-design.md`
- Reference: `Makefile`
- Reference: `webui/admin/package.json`
- Reference: `webui/customer/package.json`
- Reference: `modules/`

- [ ] **Step 1: Record repository state**

Run:

```bash
rtk proxy git status --short
rtk proxy git log --oneline -5
rtk proxy git rev-parse --show-toplevel
```

Expected: commands complete. Record the exact dirty worktree entries in the audit log and mark unrelated entries as protected from audit commits.

- [ ] **Step 2: Create the audit log skeleton**

Create `docs/superpowers/audits/2026-04-20-full-codebase-audit.md` with this content:

```markdown
# Full Codebase Audit Log

**Date:** 2026-04-20
**Spec:** `docs/superpowers/specs/2026-04-20-full-codebase-audit-design.md`
**Plan:** `docs/superpowers/plans/2026-04-20-full-codebase-audit-remediation.md`
**Mode:** Audit plus remediation

## Baseline

| Command | Result | Notes |
| --- | --- | --- |

## Protected Local Changes

| Path | Status | Handling |
| --- | --- | --- |

## Finding Queue

| ID | Domain | Severity | Status | Path | Summary | Verification |
| --- | --- | --- | --- | --- | --- | --- |

## Triage Notes

## Fix Batches

## Verification Evidence

## Deferred Findings

## Final Summary
```

- [ ] **Step 3: Record available tool versions**

Run:

```bash
rtk proxy go version
rtk proxy node --version
rtk proxy npm --version
rtk proxy php --version
rtk proxy docker --version
rtk proxy make --version
```

Expected: each available tool prints a version. If a tool is missing, record `unavailable` and the command output in the Baseline table.

- [ ] **Step 4: Run Go baseline validation**

Run:

```bash
rtk proxy make test
```

Expected: pass or produce a concrete failure. Record the result before any remediation.

- [ ] **Step 5: Run PHP syntax baseline**

Run:

```bash
rtk proxy bash -lc 'find modules/servers/virtuestack modules/blesta/virtuestack -name "*.php" -print0 | xargs -0 -n1 php -l'
```

Expected: every PHP file reports no syntax errors, or the failing file and error are recorded.

- [ ] **Step 6: Run frontend dependency and validation baselines**

Run:

```bash
rtk proxy bash -lc 'cd webui/admin && npm ci && npm run lint && npm run type-check && npm run build'
rtk proxy bash -lc 'cd webui/customer && npm ci && npm run lint && npm run type-check && npm run build'
```

Expected: pass or produce concrete failures. Record each command independently.

- [ ] **Step 7: Run non-destructive Docker and workflow baselines**

Run:

```bash
rtk proxy docker compose -f docker-compose.yml -f docker-compose.override.yml config
rtk proxy docker compose -f docker-compose.yml -f docker-compose.prod.yml config
rtk proxy bash -lc 'find .github/workflows -name "*.yml" -o -name "*.yaml" | sort'
```

Expected: compose configs render if Docker is available; workflow files are listed. Record unavailable Docker as a validation gap, not as a code finding.

- [ ] **Step 8: Commit only the audit log skeleton and baseline entries**

Run:

```bash
rtk proxy git status --short
rtk proxy git add docs/superpowers/audits/2026-04-20-full-codebase-audit.md
rtk proxy git commit -m "docs: start full codebase audit log"
```

Expected: commit includes only the audit log file. Existing unrelated deletions or user changes stay unstaged.

---

## Task 2: Dispatch Parallel Domain Audits

**Files:**
- Modify: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md`
- Reference: `docs/coding-standard.md`
- Reference: `docs/codemaps/*.md`
- Reference: `AGENTS.md`

- [ ] **Step 1: Launch Go controller/shared audit agent**

Prompt:

```text
Audit VirtueStack's Go controller and shared packages for concrete security, correctness, reliability, and docs/coding-standard violations.

Scope:
- internal/controller/api
- internal/controller/middleware
- internal/controller/services
- internal/controller/repository
- internal/controller/tasks
- internal/controller/billing
- internal/controller/payments
- internal/controller/redis
- internal/controller/audit
- internal/shared
- cmd/controller

Focus:
- auth/session/2FA/CSRF/RBAC mistakes
- tenant isolation and ownership checks
- SQL parameterization and transaction safety
- webhook/payment verification
- SSRF and URL fetch paths
- context timeouts and goroutine lifecycle
- nil pointer, race, or stale state risks
- violations of docs/coding-standard.md with concrete impact

Return only actionable findings. For each finding include:
- file path and line or function
- severity: critical, high, medium, low
- issue
- impact
- evidence
- suggested fix
- suggested verification command

Do not modify files.
```

Expected: agent returns concrete findings or states that no actionable findings were found.

- [ ] **Step 2: Launch Node Agent audit agent**

Prompt:

```text
Audit VirtueStack's Node Agent and host-facing Go code for concrete security, correctness, reliability, and docs/coding-standard violations.

Scope:
- cmd/node-agent
- internal/nodeagent
- proto/virtuestack/node_agent.proto
- internal/shared/libvirtutil

Focus:
- unsafe external command usage
- libvirt/QEMU/Ceph/LVM boundary safety
- gRPC input validation and auth assumptions
- VM lifecycle state corruption risks
- storage snapshot/backup/restore correctness
- network anti-spoofing and bandwidth safety
- resource cleanup and context cancellation
- native validation gaps

Return only actionable findings. For each finding include:
- file path and line or function
- severity: critical, high, medium, low
- issue
- impact
- evidence
- suggested fix
- suggested verification command

Do not modify files.
```

Expected: agent returns concrete findings or states that no actionable findings were found.

- [ ] **Step 3: Launch frontend audit agent**

Prompt:

```text
Audit VirtueStack's admin and customer Next.js frontends for concrete security, correctness, reliability, and type/build issues.

Scope:
- webui/admin
- webui/customer
- webui/packages
- webui/package.json
- webui/package-lock.json

Focus:
- auth token handling and logout behavior
- API client error handling
- missing permission checks in UI flows
- unsafe rendering or URL handling
- form validation gaps that affect backend calls
- TypeScript errors, lint errors, dependency lock drift
- broken routes or user workflows visible from code

Return only actionable findings. For each finding include:
- file path and line or component
- severity: critical, high, medium, low
- issue
- impact
- evidence
- suggested fix
- suggested verification command

Do not modify files.
```

Expected: agent returns concrete findings or states that no actionable findings were found.

- [ ] **Step 4: Launch PHP billing module audit agent**

Prompt:

```text
Audit VirtueStack's WHMCS and Blesta PHP billing modules for concrete security, correctness, compatibility, and provisioning issues.

Scope:
- modules/servers/virtuestack
- modules/blesta/virtuestack

Focus:
- provisioning API authentication
- webhook/callback validation
- input validation and output escaping
- secret handling
- HTTP client error handling
- PHP syntax and current-version compatibility
- mismatches with docs/api-reference.md provisioning routes

Return only actionable findings. For each finding include:
- file path and line or function
- severity: critical, high, medium, low
- issue
- impact
- evidence
- suggested fix
- suggested verification command

Do not modify files.
```

Expected: agent returns concrete findings or states that no actionable findings were found.

- [ ] **Step 5: Launch database/migrations audit agent**

Prompt:

```text
Audit VirtueStack's database migrations, models, and repository assumptions for concrete schema, RLS, migration, and tenant-isolation issues.

Scope:
- migrations
- internal/controller/models
- internal/controller/repository
- docs/codemaps/data.md

Focus:
- RLS gaps on customer-facing tables
- policies using app.current_customer_id correctly
- missing constraints or indexes with correctness impact
- migration ordering, reversibility, and drift
- model/repository/schema mismatches
- billing and session data isolation

Return only actionable findings. For each finding include:
- migration or Go file path and line/function
- severity: critical, high, medium, low
- issue
- impact
- evidence
- suggested fix
- suggested verification command

Do not modify files.
```

Expected: agent returns concrete findings or states that no actionable findings were found.

- [ ] **Step 6: Launch infrastructure/supply-chain audit agent**

Prompt:

```text
Audit VirtueStack's Docker, Compose, CI, scripts, and dependency surfaces for concrete security, correctness, and reliability issues.

Scope:
- .github/workflows
- Dockerfile.*
- docker-compose*.yml
- nginx
- scripts
- package.json
- package-lock.json
- webui/package.json
- webui/package-lock.json
- go.mod
- go.sum

Focus:
- mutable action/image versions
- root containers or baked secrets
- unsafe shell script behavior
- production-insecure defaults
- dependency lock drift
- CI commands that do not match repo scripts
- environment validation gaps

Return only actionable findings. For each finding include:
- file path and line or script/function
- severity: critical, high, medium, low
- issue
- impact
- evidence
- suggested fix
- suggested verification command

Do not modify files.
```

Expected: agent returns concrete findings or states that no actionable findings were found.

- [ ] **Step 7: While agents run, perform local quick-scan checks**

Run:

```bash
rtk rg -n "TODO|FIXME|HACK|XXX|console\\.log|fmt\\.Println|print_r|InsecureSkipVerify|verify=False|Math\\.random|math/rand|SELECT \\*|eval\\(|exec\\(|Function\\(" --glob '!webui/**/.next/**' --glob '!node_modules/**'
rtk rg -n "c\\.JSON\\([^\\n]*error|RespondWithError|current_setting\\('app\\.current_customer_id'" internal migrations
rtk rg -n "password|token|secret|api[_-]?key" --glob '!go.sum' --glob '!package-lock.json' --glob '!webui/package-lock.json' .
```

Expected: command output is reviewed manually. Add only concrete actionable findings to the queue.

- [ ] **Step 8: Append sub-agent outputs to the audit log**

For each finding, add one row using the exact values reported and validated for that finding. Use this row shape:

```markdown
| VS-AUDIT-001 | controller | high | reported | `internal/controller/api/customer/auth.go` | Refresh token validation can accept an expired session under the described condition. | `rtk proxy go test ./internal/controller/api/customer -count=1` |
```

Expected: every reported finding has a unique `VS-AUDIT-NNN` ID and enough evidence to triage. The example row above is a format example only and must not be copied into the audit log unless that exact finding is validated from source.

---

## Task 3: Triage The Finding Queue

**Files:**
- Modify: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md`
- Reference: reported files from Task 2

- [ ] **Step 1: Deduplicate findings**

Run:

```bash
rtk rg -n "VS-AUDIT-[0-9]{3}" docs/superpowers/audits/2026-04-20-full-codebase-audit.md
```

Expected: each ID appears in the finding queue and may appear in triage notes. Findings describing the same root cause are merged under the earliest ID.

- [ ] **Step 2: Validate each finding from source**

For each finding, set these shell variables to the exact file and line window from the reported finding, then read the source:

```bash
FINDING_FILE="internal/controller/api/customer/auth.go"
FINDING_START_LINE="1"
FINDING_END_LINE="160"
rtk sed -n "${FINDING_START_LINE},${FINDING_END_LINE}p" "${FINDING_FILE}"
```

Expected: the coordinator confirms the issue from source before changing status. The variable values shown above are safe examples; replace them with the exact location from each finding before running the command.

- [ ] **Step 3: Assign triage status**

Update each finding row to exactly one of:

```text
confirmed-fix-now
confirmed-defer
not-a-finding
```

Expected: every non-empty reported finding has a triage status and a short note under `## Triage Notes`.

- [ ] **Step 4: Sort fix order**

Order `confirmed-fix-now` findings by this priority:

```text
critical security > high security > build/test blockers > correctness/data integrity > reliability > standards violations with concrete impact > docs drift coupled to code
```

Expected: `## Fix Batches` lists planned batches with finding IDs and verification commands.

---

## Task 4: Remediate Confirmed Findings In Batches

**Files:**
- Modify: files listed in the current `## Fix Batches` entry after triage
- Modify: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md`
- Test: validation commands listed in the current `## Fix Batches` entry after triage

- [ ] **Step 1: Before each batch, protect unrelated changes**

Run:

```bash
rtk proxy git status --short
rtk proxy git diff --stat
```

Expected: unrelated local changes are identified and left unstaged. If a target file has unrelated edits, read the diff and preserve those changes.

- [ ] **Step 2: For behavior changes, write or update the smallest relevant test first**

Use the existing test style in the touched package. Valid targeted test commands for this repository include:

```bash
rtk proxy go test ./internal/controller/api/customer -count=1
rtk proxy go test ./internal/controller/services -count=1
rtk proxy bash -lc 'cd webui/admin && npm run type-check'
rtk proxy bash -lc 'find modules/servers/virtuestack modules/blesta/virtuestack -name "*.php" -print0 | xargs -0 -n1 php -l'
```

Expected: new failing tests are used when the finding is testable without external services. Static/config fixes may use deterministic validation instead of a failing unit test.

- [ ] **Step 3: Apply the smallest fix**

Use `apply_patch` for manual edits. Use package managers, formatters, or generators only when they are the established way to update the artifact being fixed.

Expected: changes are limited to files required by the finding and any directly coupled tests or docs.

- [ ] **Step 4: Run targeted verification**

Run the verification command recorded for the batch. Examples:

```bash
rtk proxy go test ./internal/shared/util -count=1
rtk proxy go test ./internal/controller/... -count=1
rtk proxy bash -lc 'cd webui/customer && npm run lint && npm run type-check'
rtk proxy bash -lc 'cd webui/admin && npm run build'
rtk proxy bash -lc 'find modules/servers/virtuestack modules/blesta/virtuestack -name "*.php" -print0 | xargs -0 -n1 php -l'
rtk proxy docker compose -f docker-compose.yml -f docker-compose.prod.yml config
```

Expected: targeted verification passes or failure is recorded with the next corrective action.

- [ ] **Step 5: Run broader validation for the touched area**

Use this matrix:

```text
Go controller/shared: rtk proxy make test
Node Agent native code: rtk proxy go test ./internal/nodeagent/... when native headers are available; otherwise rtk proxy go test ./internal/shared/... and static review evidence
Admin frontend: rtk proxy bash -lc 'cd webui/admin && npm run lint && npm run type-check && npm run build'
Customer frontend: rtk proxy bash -lc 'cd webui/customer && npm run lint && npm run type-check && npm run build'
PHP modules: rtk proxy bash -lc 'find modules/servers/virtuestack modules/blesta/virtuestack -name "*.php" -print0 | xargs -0 -n1 php -l'
Docker/Compose: rtk proxy docker compose -f docker-compose.yml -f docker-compose.prod.yml config
Scripts: rtk proxy bash -n scripts/validate-env.sh && rtk proxy bash -n scripts/setup-e2e.sh && rtk proxy bash -n scripts/backup-config.sh && rtk proxy bash -n install.sh
```

Expected: broader validation passes, or unavailable infrastructure is recorded as a gap.

- [ ] **Step 6: Update the audit log**

Append to `## Fix Batches`:

```markdown
### Batch 1: Customer Auth Refresh Token Validation

- Findings fixed: `VS-AUDIT-001`, `VS-AUDIT-002`
- Files changed:
  - `internal/controller/api/customer/auth.go`
- Verification:
  - `rtk proxy go test ./internal/controller/api/customer -count=1`: pass
- Notes: No residual risk identified for this batch.
```

Expected: each batch has evidence before it is committed. The example batch above shows the required format; use the real batch number, finding IDs, files, commands, and notes from the fixes just completed.

- [ ] **Step 7: Commit the batch**

Run:

```bash
rtk proxy git status --short
rtk proxy git add docs/superpowers/audits/2026-04-20-full-codebase-audit.md
BATCH_FILES="internal/controller/api/customer/auth.go internal/controller/api/customer/auth_test.go"
rtk proxy git add -- ${BATCH_FILES}
rtk proxy git commit -m "fix(audit): remediate confirmed finding batch"
```

Expected: commit includes only files for the batch and the audit log. The `BATCH_FILES` value above is an example; set it to the real files listed in the current batch before running `git add`. Unrelated local changes remain unstaged.

- [ ] **Step 8: Repeat until no `confirmed-fix-now` findings remain**

Run:

```bash
rtk rg -n "confirmed-fix-now|reported" docs/superpowers/audits/2026-04-20-full-codebase-audit.md
```

Expected: no unresolved `reported` findings remain. No `confirmed-fix-now` finding remains without a corresponding fixed batch or explicit reclassification note.

---

## Task 5: Run Final Focused Review

**Files:**
- Modify: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md`
- Reference: files changed during Task 4

- [ ] **Step 1: Review final git diff**

Run:

```bash
rtk proxy git status --short
rtk proxy git diff --stat HEAD
rtk proxy git diff HEAD -- docs/superpowers/audits/2026-04-20-full-codebase-audit.md
```

Expected: only intentional uncommitted audit-log updates remain, or the tree is clean except unrelated protected local changes.

- [ ] **Step 2: Re-scan changed files for standards violations**

Run:

```bash
rtk proxy bash -lc 'git diff --name-only HEAD~20..HEAD | sort'
rtk proxy bash -lc 'git diff --name-only HEAD~20..HEAD | grep -E "\\.(go|ts|tsx|php|sh|yml|yaml|sql|Dockerfile)$" | xargs -r rg -n "TODO|FIXME|HACK|XXX|console\\.log|fmt\\.Println|print_r|InsecureSkipVerify|verify=False|Math\\.random|math/rand"'
```

Expected: no introduced placeholder/debug/security-pattern hits. Any pre-existing or false-positive hit is recorded with rationale.

- [ ] **Step 3: Run final validation set**

Run commands relevant to all touched domains:

```bash
rtk proxy make test
rtk proxy bash -lc 'find modules/servers/virtuestack modules/blesta/virtuestack -name "*.php" -print0 | xargs -0 -n1 php -l'
rtk proxy bash -lc 'cd webui/admin && npm run lint && npm run type-check && npm run build'
rtk proxy bash -lc 'cd webui/customer && npm run lint && npm run type-check && npm run build'
rtk proxy docker compose -f docker-compose.yml -f docker-compose.prod.yml config
```

Expected: commands pass where tooling and infrastructure are available. Missing native headers, unavailable Docker daemon, or pre-existing failures are recorded as validation gaps.

- [ ] **Step 4: Update final audit summary**

Fill `## Final Summary` with:

```markdown
## Final Summary

- Fixed findings:
  - `VS-AUDIT-001`: summary
- Deferred findings:
  - `VS-AUDIT-002`: reason
- Not findings:
  - `VS-AUDIT-003`: reason
- Verification commands:
  - `command`: pass/fail/unavailable with reason
- Residual risk:
  - concise remaining risk
```

Expected: final summary is specific and matches the finding queue.

- [ ] **Step 5: Commit final audit log updates**

Run:

```bash
rtk proxy git add docs/superpowers/audits/2026-04-20-full-codebase-audit.md
rtk proxy git commit -m "docs: finalize full codebase audit log"
```

Expected: final audit log is committed if it changed after the last fix batch.

---

## Task 6: Completion Report

**Files:**
- Reference: `docs/superpowers/audits/2026-04-20-full-codebase-audit.md`
- Reference: git commits created during execution

- [ ] **Step 1: Gather commit and status evidence**

Run:

```bash
rtk proxy git status --short
rtk proxy git log --oneline -10
```

Expected: status shows no uncommitted audit changes. Any unrelated protected local changes are identified.

- [ ] **Step 2: Prepare final response**

Final response must include:

```text
- Audit log path
- Summary of fixed findings
- Summary of deferred findings
- Verification commands run and their results
- Validation gaps
- Note about unrelated protected local changes, if still present
```

Expected: user can understand exactly what changed, what was checked, and what risk remains without reading command output.
