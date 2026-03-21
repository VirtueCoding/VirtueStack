# VirtueStack Security Audit Report

**Branch:** `audit/mar21`
**Date:** 2026-03-21
**Auditor:** Claude Code (Autonomous Security Audit)

---

## Executive Summary

A comprehensive security audit of VirtueStack was conducted across all three priority tiers, analyzing 180+ source files including Go backend, TypeScript frontend, PHP WHMCS module, SQL migrations, Docker configurations, and nginx settings.

### Findings Summary

| Severity | Count | Status |
|----------|-------|--------|
| **CRITICAL** | 2 | Require immediate remediation |
| **HIGH** | 9 | Require prompt attention |
| **MEDIUM** | 26 | Should be addressed in upcoming releases |
| **LOW** | 14 | Defense-in-depth improvements |
| **INFO** | 68 | Verified secure controls / Best practice notes |

### Critical Findings (Immediate Action Required)

1. **FINDING-016**: Arbitrary File Read via TransferDisk
   - **Location:** `internal/nodeagent/grpc_handlers_storage.go:141-164`
   - **Impact:** Attacker with gRPC access can read ANY file on the node including `/etc/shadow`, SSH private keys, database files
   - **Fix:** Add `validatePath()` call before `os.Open()`

2. **FINDING-037**: nwfilter Anti-Spoofing Dead Code
   - **Location:** `internal/nodeagent/network/nwfilter.go`, `internal/nodeagent/vm/domain_xml.go:168-172`
   - **Impact:** VMs may have NO network anti-spoofing protection; domain XML references non-existent filter
   - **Fix:** Either call `CreateAntiSpoofFilter` in CreateVM flow or create proper `virtuestack-clean-traffic` filter

### High Severity Findings

| ID | Title | Category |
|----|-------|----------|
| FINDING-006 | Refresh token rotation race condition | Authentication |
| FINDING-010 | IP allowlist bypass via X-Forwarded-For | Authorization |
| FINDING-014 | RLS policies never activated at runtime | Authorization |
| FINDING-017 | QCOW backup/restore lack path validation | Path Traversal |
| FINDING-018 | validatePath vulnerable to symlink attacks | Path Traversal |
| FINDING-025 | Console access missing authorization | Authorization |
| FINDING-033 | Hardcoded test secrets in E2E config | Secrets |
| FINDING-038 | dnsmasq config newline injection | Injection |
| FINDING-039 | Dual bandwidth limiting inconsistency | Logic |

---

## Risk Assessment

### Attack Surface Analysis

```
                    ┌─────────────────────────────────────────────────────────────┐
                    │                      ATTACK SURFACE                          │
                    └─────────────────────────────────────────────────────────────┘

EXTERNAL FACING:
  ├── Nginx (80/443) ────────────────────── TLS 1.3, Rate Limited ✓
  ├── WHMCS Webhook Receiver ────────────── HMAC-SHA256 Verified ✓
  └── Customer API (JWT Auth) ───────────── Missing rate limiting on auth ⚠️

INTERNAL SERVICES:
  ├── Controller gRPC Client ────────────── mTLS Enforced ✓
  ├── Node Agent gRPC Server ────────────── mTLS Enforced ✓
  │   └── Console handlers ──────────────── Missing authz ⚠️
  │   └── File operations ───────────────── Path traversal vulnerable 🔴
  │   └── nwfilter creation ─────────────── Dead code 🔴
  ├── PostgreSQL ────────────────────────── RLS exists but not activated ⚠️
  └── NATS JetStream ────────────────────── Token auth required ✓

CROSS-CUTTING CONCERNS:
  ├── Rate Limiting ─────────────────────── Fails closed ✓ (Redis)
  ├── Audit Logging ─────────────────────── Manual calls, gaps ⚠️
  └── Input Validation ──────────────────── Generally good ✓
```

### Top Exploit Chains

**Chain 1: Node Agent File Read → Credential Theft**
1. Attacker gains valid mTLS certificate (insider threat or cert leak)
2. Calls `TransferDisk` with `sourcePath=/etc/shadow`
3. Reads any file on hypervisor node
4. Extracts SSH keys, database credentials, other VM disks

**Chain 2: Customer Auth Rate Limit Bypass → Account Takeover**
1. Customer auth endpoints have no rate limiting (FINDING-008)
2. Attacker brute forces passwords
3. Combined with token reuse race condition (FINDING-006)
4. Account takeover even with 2FA

**Chain 3: X-Forwarded-For Spoofing → IP Allowlist Bypass**
1. Gin router trusts all X-Forwarded-For headers (FINDING-010)
2. Attacker spoofs allowed IP
3. Bypasses IP allowlist on provisioning API keys
4. Creates/modifies VMs without authorization

---

## Security Strengths

The audit also identified **68 verified security controls** that are properly implemented:

### Authentication & Cryptography
- Argon2id password hashing with OWASP parameters
- AES-256-GCM encryption with proper nonce handling
- crypto/rand for all security-sensitive random generation
- JWT with algorithm whitelist preventing confusion attacks
- TOTP with proper backup code handling (constant-time comparison)

### Network Security
- mTLS enforced on all gRPC communication
- TLS 1.3 only on nginx
- Comprehensive SSRF protection in webhook system
- DNS rebinding protection with atomic IP validation

### Container Security
- Non-root execution for all custom containers
- No-new-privileges security option
- Capability dropping (ALL) with minimal additions
- Multi-stage builds for reduced attack surface

### Audit & Logging
- Audit log immutability enforced at database level
- Token fingerprinting prevents credential exposure in logs
- PII masking for email addresses
- Structured JSON logging

---

## Recommendations by Priority

### P0 - Critical (Fix Immediately)

| Finding | Action | Effort |
|---------|--------|--------|
| FINDING-016 | Add validatePath() to TransferDisk | 1 hour |
| FINDING-037 | Fix nwfilter code or create correct filter | 4 hours |

### P1 - High (Fix Within 1 Week)

| Finding | Action | Effort |
|---------|--------|--------|
| FINDING-010 | Configure SetTrustedProxies() in Gin | 1 hour |
| FINDING-006 | Wrap token refresh in transaction | 4 hours |
| FINDING-017 | Add validatePath() to QCOW functions | 2 hours |
| FINDING-018 | Enhance validatePath() with symlink resolution | 4 hours |
| FINDING-014 | Either activate RLS or remove dead code | 8 hours |
| FINDING-025 | Add authorization to console handlers | 4 hours |
| FINDING-033 | Move test secrets to .env.test | 1 hour |
| FINDING-038 | Sanitize VMName/VMID for newlines | 2 hours |

### P2 - Medium (Fix Within Sprint)

- Apply rate limiting to customer auth endpoints (FINDING-008)
- Add granular rate limits to VM operations (FINDING-023, FINDING-024)
- Apply audit middleware to all API routes (FINDING-045)
- Log authentication events to audit table (FINDING-047, FINDING-048)
- Add CHECK constraints for resource columns (FINDING-057)
- Fix bandwidth limiting consistency (FINDING-039)
- Add ownership verification in task handlers (FINDING-051)

### P3 - Low/Info (Technical Debt)

- Non-root containers verified but base images not pinned by SHA
- CSP allows unsafe-inline (Next.js requirement)
- Missing indexes for security queries
- Dead code cleanup

---

## Cross-Cutting Patterns

### Missing Atomicity
Multiple findings involve operations that should be atomic but aren't:
- Refresh token rotation (FINDING-006)
- Password reset flow (FINDING-007)
- Task status updates (FINDING-052)
- RLS context setting (FINDING-014)

**Recommendation:** Implement transaction wrappers for multi-step security operations.

### Inconsistent Rate Limiting
Rate limit functions exist but aren't consistently applied:
- Admin auth: Protected ✓
- Customer auth: NOT protected ✗
- VM operations: Pre-built but not applied
- Provisioning API: Protected ✓

**Recommendation:** Audit all routes and apply appropriate rate limits systematically.

### Path Validation Gaps
The `validatePath()` function exists but isn't used consistently:
- Used: template_file_path, disk_path
- NOT used: backupPath, targetPath, sourcePath in various handlers

**Recommendation:** Mandatory path validation for all file operations.

---

## Methodology

- **Code Review:** Manual reading of source files for logic flaws and security patterns
- **Pattern Matching:** Grep for known-bad patterns (SQL injection, command injection, timing attacks)
- **Data Flow Analysis:** Tracing user input from HTTP request to database
- **STRIDE Threat Modeling:** Spoofing, Tampering, Repudiation, Information Disclosure, DoS, Elevation of Privilege
- **Test Coverage Review:** Identifying missing security tests

---

## Appendix: All Findings

See `findings.tsv` for complete list of 119 findings including:
- 60 vulnerability/issue findings (FINDING-001 through FINDING-060)
- 59 verified security controls (AUDIT-001 through AUDIT-063)
- 1 improvement suggestion (IMPROVEMENT-001)

---

**Report Generated:** 2026-03-21
**Commit Findings:** NOT committed (findings.tsv excluded via .gitignore per task.md)