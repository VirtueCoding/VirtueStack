# Full Codebase Audit and Remediation Design

**Date:** 2026-04-20
**Project:** VirtueStack
**Mode:** Fresh audit plus remediation
**Execution style:** Autonomous after approval to start implementation

## Problem Statement

VirtueStack is a multi-language VPS management platform with security-sensitive surfaces across the controller, node agent, web frontends, billing modules, database migrations, scripts, Docker artifacts, and deployment documentation. A useful full codebase audit must cover more than build failures. It must look for concrete security, correctness, reliability, and standards issues while avoiding speculative style churn.

There are two main risks:

1. Missing issues because the repository spans several independent technical domains.
2. Introducing regressions through broad edits that are not tied to confirmed findings.

The audit should therefore split review work by domain, require evidence for every finding, fix only confirmed issues, and verify each coherent fix batch with repository-native checks.

## Goals

- Perform a fresh repo-wide audit without relying on prior audit specifications or plans as the operating baseline.
- Use parallel domain review where work can proceed independently.
- Remediate confirmed, safe-to-fix findings during the same audit run.
- Preserve unrelated local worktree changes and avoid restoring, deleting, or reformatting files outside the audit scope.
- Produce a final record of fixed findings, deferred findings, verification commands, and remaining validation gaps.

## Non-Goals

- Broad style cleanup without correctness, security, reliability, or standards impact.
- Rewriting architecture for elegance alone.
- Manual edits to generated artifacts unless regeneration is the deliberate fix.
- Adding new audit tooling or dependencies unless an existing repo command cannot reasonably validate a touched area.
- Proving the absolute absence of defects. The completion bar is limited to the completed audit methods and validations.

## Audit Architecture

The main session is the coordinator. It owns the canonical finding queue, validates reported issues, applies fixes, and runs verification.

Parallel sub-agents may audit independent domains and report findings using a strict output contract. Sub-agents do not decide what gets fixed. The coordinator deduplicates and validates their reports before remediation.

The audit domains are:

1. **Go controller and shared packages**
   - Admin, customer, provisioning APIs
   - Middleware, auth, sessions, CSRF, RBAC, rate limiting
   - Services, repositories, billing, payments, tasks
   - Shared config, crypto, logging, errors, SSRF utilities

2. **Node Agent and host-facing Go**
   - gRPC handlers and proto contract usage
   - VM lifecycle, storage, network, guest agent, console paths
   - libvirt, QEMU, Ceph, LVM, QCOW command boundaries
   - Resource cleanup, context propagation, privilege and input safety

3. **Frontend applications**
   - Admin and customer Next.js apps
   - API clients, auth flows, permission handling, form validation
   - Type safety, dependency drift, build/lint/type-check failures
   - Obvious user-facing workflow breakage

4. **PHP billing modules**
   - WHMCS and Blesta module request handling
   - Provisioning API client behavior
   - Input validation, secret handling, webhook/callback safety
   - PHP syntax and compatibility issues

5. **Database and migrations**
   - Schema consistency, constraints, indexes, RLS policies
   - Migration ordering and reversibility where practical
   - Multi-tenant isolation assumptions using `app.current_customer_id`
   - Drift between models, repositories, and migrations

6. **Docker, scripts, CI, and supply chain**
   - Dockerfile hardening, non-root users, secret handling
   - Compose safety, exposed ports, production defaults
   - Shell script robustness and unsafe command handling
   - Dependency audit paths and CI workflow safety

7. **Documentation and API consistency**
   - Docs that directly describe routes, configuration, validation, deployment, or operations
   - Drift between documented behavior and implemented behavior
   - Documentation updates only when coupled to a confirmed fix

## Finding Criteria

A finding is actionable only when it has all of the following:

- Specific file path and location or clearly scoped artifact.
- Concrete issue and practical impact.
- Evidence from code, docs, tests, or command output.
- Explanation of why it affects security, correctness, reliability, buildability, or `docs/coding-standard.md`.
- Suggested remediation path.
- Suggested verification command or review method.

The audit excludes speculative claims, preference-only feedback, and changes whose only benefit is stylistic consistency.

## Triage Model

Each finding is classified as one of:

- `confirmed-fix-now`: Real, in scope, and safe to remediate during this audit run.
- `confirmed-defer`: Real, but too invasive, infrastructure-dependent, or product-dependent to fix safely in this run.
- `not-a-finding`: Duplicate, stale, unsupported, already fixed, or outside the agreed scope.

The coordinator should favor fixing concrete problems, but should not touch code based on weak evidence.

## Remediation Workflow

1. Establish the current baseline: worktree state, available tools, and repo-native validation commands.
2. Dispatch parallel domain audits with the finding output contract.
3. Build a canonical queue in a markdown audit log under `docs/superpowers/audits/`.
4. Triage reported findings into the three statuses above.
5. Fix `confirmed-fix-now` items in small coherent batches.
6. Verify each batch with commands relevant to the changed area.
7. Re-review sensitive fixes when they touch auth, tenant isolation, payment/webhook handling, VM lifecycle, host command execution, or migrations.
8. Run a final focused pass over changed areas and unresolved high-risk domains.
9. Finish with a concise summary of fixed issues, deferred issues, commands run, and validation gaps.

## Verification Strategy

Use repository-native checks wherever possible:

| Area | Verification |
| --- | --- |
| Go controller/shared | `make test`, targeted `go test`, and `make vet` when available |
| Node Agent/native Go | Targeted build or tests when libvirt/Ceph headers are available; otherwise static review plus strongest non-native checks |
| Admin frontend | `npm ci`, `npm run lint`, `npm run type-check`, `npm run build` from `webui/admin` |
| Customer frontend | `npm ci`, `npm run lint`, `npm run type-check`, `npm run build` from `webui/customer` |
| PHP modules | `php -l` across touched module files or module tree |
| Scripts | Shell syntax checks and targeted execution in safe modes where available |
| Docker/Compose | Docker build or compose config validation when Docker is available |
| Migrations | Static migration review; migration execution only when a local database or Docker environment is available |
| Documentation | Link/path checks by inspection and consistency checks against code |

Pre-existing failures must be recorded and not attributed to later changes. If a validation command cannot run because required tooling or infrastructure is unavailable, the final report must name that gap explicitly.

## Autonomy Rules

The user expects to be away during the actual audit. After approval to proceed into implementation, the coordinator should continue autonomously until the audit is complete.

The coordinator may decide:

- Which parallel domain audits to launch.
- Which confirmed findings are safe to fix immediately.
- Which validation commands are appropriate after each batch.
- Whether a finding should be deferred because it is too invasive or infrastructure-dependent.
- How to sequence fixes to preserve momentum and reduce regression risk.

The coordinator should stop and ask the user only when:

- A requested action would be destructive to unrelated local changes.
- A fix requires product or business policy that cannot be inferred from existing docs or code.
- A necessary secret, credential, or external account is unavailable.
- The only viable path is a large architectural rewrite beyond the approved audit scope.

## Completion Bar

The audit is complete when:

- Every `confirmed-fix-now` finding from completed audit passes is fixed.
- Relevant validations for touched areas pass, or gaps are explicitly documented.
- A final focused review does not surface additional in-scope confirmed issues that are practical to fix in the current session.
- The final report distinguishes fixed findings, deferred findings, `not-a-finding` items where useful, commands run, and residual risk.

## Deliverables

- A markdown audit log under `docs/superpowers/audits/` with the canonical finding queue and triage decisions.
- Code, config, script, migration, or documentation fixes for confirmed safe-to-fix issues.
- Verification evidence for each fix batch.
- A final concise completion summary.

## Transition To Planning

After this design is reviewed and approved, the next step is a detailed implementation plan. That plan should specify baseline commands, sub-agent dispatch prompts, finding log format, fix batching order, verification checkpoints, and final reporting steps.
