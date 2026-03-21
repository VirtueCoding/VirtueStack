# VirtueStack Security Audit

Autonomous security audit, bug hunting, and threat modeling for VirtueStack — a KVM/QEMU VM management platform. Modeled after [karpathy/autoresearch](https://github.com/karpathy/autoresearch) program.md.

## Setup

To set up a new audit session, work with the user to:

1. **Agree on a run tag**: propose a tag based on today's date (e.g. `audit-mar20`). The branch `audit/<tag>` must not already exist — this is a fresh run.
2. **Create the branch**: `git checkout -b audit/<tag>` from current main.
3. **Read the in-scope files**: Build full context by reading:
   - `AGENTS.md` — complete technical reference: architecture, API endpoints, database schema, gRPC, auth, storage, environment variables.
   - `CODING_STANDARD.md` — quality gates and rules the codebase claims to follow.
   - `docs/CODEMAPS/*.md` — token-lean architecture summaries (~4K tokens total).
4. **Initialize findings.tsv**: Create `findings.tsv` with just the header row. Findings will be recorded after each audit pass.
5. **Confirm and go**: Confirm setup looks good with the user.

Once you get confirmation, kick off the audit loop.

## Scope

The entire VirtueStack codebase is in scope. Priority areas, ordered by risk:

### Tier 1 — Critical (audit first)
| Area | Key Paths | Threat Surface |
|------|-----------|----------------|
| Authentication & Session Management | `internal/controller/services/auth_service*.go`, `internal/controller/api/middleware/auth.go` | JWT forgery, token leakage, session fixation, refresh token reuse, 2FA bypass |
| Authorization & RBAC | `internal/controller/services/rbac_service.go`, all `api/customer/*.go` handlers | Privilege escalation, IDOR, missing ownership checks, cross-customer data access |
| Row Level Security | `migrations/*.sql` (RLS policies) | RLS bypass, missing policies on new tables, `current_setting` injection |
| Provisioning API Keys | `internal/controller/api/middleware/ip_allowlist.go`, `api/provisioning/` | API key timing attacks, IP allowlist bypass, key hash comparison |
| Secrets & Cryptography | `internal/shared/crypto/`, `internal/controller/services/auth_service.go` | Weak encryption, key management, hardcoded secrets, insufficient entropy |

### Tier 2 — High
| Area | Key Paths | Threat Surface |
|------|-----------|----------------|
| Input Validation | `internal/controller/api/middleware/validation.go`, all API handlers | SQL injection, command injection, XSS via stored fields, path traversal |
| Webhook System | `internal/controller/services/webhook.go`, `internal/controller/tasks/webhook_deliver.go` | SSRF via webhook URLs, secret leakage, replay attacks, timing oracle |
| VM Lifecycle & gRPC | `internal/nodeagent/vm/lifecycle.go`, `internal/nodeagent/server.go` | VM escape vectors, unauthorized gRPC calls, mTLS misconfiguration |
| File Operations | `internal/nodeagent/storage/qcow.go`, `internal/controller/api/customer/iso_upload.go` | Path traversal, symlink attacks, arbitrary file read/write, command injection in qemu-img |
| Rate Limiting | `internal/controller/api/middleware/ratelimit.go` | Bypass via header spoofing, per-IP vs per-user confusion, distributed brute force |

### Tier 3 — Medium
| Area | Key Paths | Threat Surface |
|------|-----------|----------------|
| Task System | `internal/controller/tasks/*.go` | Task injection, deserialization, queue poisoning |
| Network / Abuse Prevention | `internal/nodeagent/network/*.go` | nwfilter bypass, bandwidth evasion, nftables rule injection |
| Logging & Audit | `internal/controller/api/middleware/audit.go`, `internal/shared/logging/` | Log injection, PII leakage, missing audit coverage |
| Docker / Deployment | `Dockerfile.*`, `docker-compose*.yml` | Container escape, excessive privileges, exposed ports, default credentials |
| Frontend | `webui/admin/`, `webui/customer/` | XSS, CSRF, open redirects, insecure token storage |
| WHMCS Module | `modules/servers/virtuestack/` | PHP injection, insecure API client, webhook receiver bypass |
| Database Migrations | `migrations/*.sql` | Missing constraints, unsafe defaults, privilege grants |

### Out of Scope
- Denial of service via resource exhaustion (unless trivially exploitable).
- Social engineering vectors.
- Third-party dependency CVEs (use `govulncheck` / `npm audit` separately).

## What You CAN Do

- **Read any file** in the repository. All source code, configs, migrations, tests, and docs are fair game.
- **Run static analysis tools** already available: `go vet`, `make lint`, `make vet`, `make vuln`.
- **Run tests** to understand behavior: `make test`, `go test -run TestName ./path/...`.
- **Grep and search** the codebase extensively. Trace data flows end-to-end.
- **Write proof-of-concept analysis** to demonstrate findings (but do NOT modify production code).
- **Create temporary test files** to verify hypotheses (clean up after).

## What You CANNOT Do

- **Modify production source code**. This is a read-only audit. Do not "fix" issues — only report them.
- **Run the application** against live infrastructure or external services.
- **Install new packages** or tools not already present.
- **Commit findings.tsv** to git (keep it untracked, like autoresearch's results.tsv).

## Audit Methodology

For each area, apply these techniques:

1. **Code Review**: Read the implementation. Look for logic flaws, missing checks, unsafe patterns.
2. **Data Flow Analysis**: Trace user input from HTTP request → middleware → handler → service → repository → database. Identify sanitization gaps.
3. **Threat Modeling (STRIDE)**: For each component, consider Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege.
4. **Pattern Matching**: Search for known-bad patterns:
   - `fmt.Sprintf` in SQL queries (SQL injection)
   - `exec.Command` with user input (command injection)
   - Missing `ctx` timeout/deadline propagation
   - Comparison of secrets without constant-time functions
   - Error messages leaking internal details
   - Missing ownership checks in customer API handlers
   - Hardcoded credentials or keys
5. **Test Review**: Check if security-critical paths have test coverage. Missing tests for auth/authz are findings.
6. **Configuration Review**: Check Dockerfiles, compose files, nginx configs for security misconfigurations.

## Severity Classification

| Severity | Definition | Examples |
|----------|------------|---------|
| **CRITICAL** | Exploitable remotely, no auth required, leads to full compromise | RCE, auth bypass, SQL injection with data exfil |
| **HIGH** | Exploitable with low-privilege auth, leads to significant impact | Privilege escalation, IDOR accessing other customers' VMs, SSRF |
| **MEDIUM** | Requires specific conditions, moderate impact | Missing rate limit on sensitive endpoint, weak crypto config |
| **LOW** | Minor issue, limited impact, defense-in-depth gap | Verbose error messages, missing security headers, log injection |
| **INFO** | Observation, not directly exploitable, best practice suggestion | Missing test coverage, code quality concern, hardening opportunity |

## Output Format

Each finding must include:

```
### [SEVERITY] FINDING-NNN: Title

**Location:** `file/path.go:line` (or range)
**Category:** (e.g., Authentication, Authorization, Injection, Cryptography, Configuration)
**STRIDE:** (which STRIDE threat(s) apply)
**CWE:** CWE-NNN (if applicable)

**Description:**
What the vulnerability is, in 2-4 sentences.

**Evidence:**
Code snippet or grep output showing the issue.

**Impact:**
What an attacker could achieve by exploiting this.

**Recommendation:**
How to fix it (1-3 sentences). Do NOT implement the fix.
```

## Logging Results

When a finding is confirmed, log it to `findings.tsv` (tab-separated, NOT comma-separated).

The TSV has a header row and 7 columns:

```
id	severity	category	location	title	recommended_fix	status
```

1. Finding ID (e.g. FINDING-001)
2. Severity: `critical`, `high`, `medium`, `low`, `info`
3. Category: `auth`, `authz`, `injection`, `crypto`, `config`, `logic`, `disclosure`, `ssrf`, `input-validation`, `other`
4. Primary file location (e.g. `internal/controller/services/auth_service.go:142`)
5. Short title (one line)
6. Recommended fix (brief description of how to remediate, 1-2 sentences)
7. Status: `confirmed`, `suspected`, `false-positive`

Example:

```
id	severity	category	location	title	recommended_fix	status
FINDING-001	critical	auth	internal/controller/services/auth_service_tokens.go:87	JWT secret allows empty string	Add validation to reject empty JWT_SECRET at startup; use crypto/rand for generation	confirmed
FINDING-002	high	authz	internal/controller/api/customer/snapshots.go:45	Missing ownership check on snapshot delete	Add WHERE customer_id = $1 clause to delete query; verify ownership before deletion	confirmed
FINDING-003	medium	crypto	internal/shared/crypto/encrypt.go:23	AES-CBC without HMAC (no authenticated encryption)	Switch to AES-GCM or add HMAC-SHA256 for authenticated encryption	suspected
FINDING-004	low	disclosure	internal/controller/api/middleware/recovery.go:18	Stack trace leaked in 500 response	Return generic error message; log stack traces server-side only	confirmed
FINDING-005	info	config	Dockerfile.controller:12	Running as root in container	Add USER directive to run as non-root user; update file permissions	false-positive
```

## The Audit Loop

The audit runs on a dedicated branch (e.g. `audit/mar20`).

LOOP THROUGH EACH TIER (Tier 1 → Tier 2 → Tier 3):

1. **Select the next area** from the current tier that hasn't been audited yet.
2. **Read all relevant files** for that area. Build a mental model of the code.
3. **Apply audit methodology** (code review, data flow analysis, STRIDE, pattern matching, test review).
4. **For each potential finding:**
   a. Verify it by tracing the code path end-to-end.
   b. Classify severity using the table above.
   c. Write up the finding in the output format.
   d. Log it to `findings.tsv`.
5. **After completing an area**, write a brief summary: what was audited, how many findings, key observations.
6. **Move to the next area**. Repeat until all tiers are complete.

After all areas are audited:

7. **Cross-cutting analysis**: Look for systemic patterns across findings. Are there classes of bugs that repeat? Missing security controls that affect multiple areas?
8. **Write executive summary**: Total findings by severity, top risks, and recommended priorities.
9. **Commit the findings report** (NOT findings.tsv) to the audit branch.

**Grep patterns to run early** (seed your investigation):

```bash
# SQL injection vectors
rg "fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE" --type go

# Command injection vectors
rg "exec.Command\|exec.CommandContext" --type go

# Hardcoded secrets
rg -i "password|secret|key|token" --type go --glob '!*_test.go' --glob '!go.sum' | rg -i "= \"|:= \""

# Missing error handling
rg "_ = " --type go --glob '!*_test.go'

# Unsafe crypto
rg "crypto/md5\|crypto/sha1\|crypto/des\|crypto/rc4" --type go

# Timing-unsafe comparisons on secrets
rg "==.*secret\|==.*token\|==.*key\|==.*hash\|==.*password" --type go --glob '!*_test.go'

# SSRF candidates (user-controlled URLs)
rg "http.Get\|http.Post\|http.NewRequest" --type go

# Missing context timeouts
rg "context.Background\(\)\|context.TODO\(\)" --type go --glob '!*_test.go'

# Path traversal
rg "filepath.Join\|os.Open\|os.Create\|os.ReadFile\|os.WriteFile" --type go

# Sensitive data in logs
rg "slog\.\|log\.\|logger\." --type go | rg -i "password\|secret\|token\|key"
```

**NEVER STOP**: Once the audit loop has begun (after the initial setup), do NOT pause to ask the human if you should continue. Do NOT ask "should I keep going?" or "is this a good stopping point?". The human might be away from the computer and expects you to continue working *indefinitely* until you are manually stopped. You are autonomous. If you finish all tiers, go deeper — re-examine findings for exploit chains, check for race conditions, review error handling paths, look for logic bugs in state machines (VM lifecycle, task state machine). The loop runs until the human interrupts you, period.

As an example use case, a user might leave you running while they sleep. You should be able to audit dozens of files per hour, producing a comprehensive security report by morning.
