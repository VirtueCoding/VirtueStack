# VirtueStack Security Audit Report

**Audit Date:** 2026-03-20
**Branch:** audit/mar20
**Auditor:** Autonomous Security Audit (Claude Code)
**Status:** ✅ All findings resolved

## Executive Summary

This security audit covered the VirtueStack KVM/QEMU VM management platform, analyzing authentication, authorization, cryptography, input validation, SSRF protections, file operations, configuration security, WHMCS PHP module, and E2E test coverage for security scenarios.

**All 5 findings have been fixed and verified.**

### Findings Summary

| Severity | Count | Fixed |
|----------|-------|-------|
| Critical | 0     | -     |
| High     | 0     | -     |
| Medium   | 1     | ✅    |
| Low      | 2     | ✅    |
| Info     | 2     | ✅    |
| **Total**| **5** | **5** |

### Resolved Issues

1. **Rate Limiter Fail-Open (MEDIUM)** ✅ Fixed - Redis rate limiter now fails closed, denying requests when Redis is unavailable.

2. **Cryptographic Fallback to Non-Secure RNG (LOW)** ✅ Fixed - Password generation now panics on `crypto/rand` failure instead of falling back to `math/rand`.

3. **Weak Default NATS Token (LOW)** ✅ Fixed - `NATS_AUTH_TOKEN` is now required; no default fallback exists.

4. **In-Memory Rate Limiting Limitation (INFO)** ✅ Fixed - Added comprehensive security documentation warning about single-instance deployments.

5. **SSO Token in URL (INFO)** ✅ Fixed - Implemented secure opaque token exchange flow with single-use tokens and HttpOnly cookies.

### Key Security Strengths Observed

The codebase demonstrates several strong security practices:

1. **Secret Type with Redaction** - `internal/shared/config/config.go` implements a `Secret` type that returns `[REDACTED]` on string conversion, preventing accidental logging of sensitive values.

2. **Production Configuration Validation** - Strong validation rejects known insecure defaults (JWT secrets, encryption keys, database passwords) when `APP_ENV=production`.

3. **SSRF Protection** - Webhook delivery implements dual-layer SSRF protection:
   - Pre-flight DNS resolution checks for private IPs
   - Connect-time IP validation via custom dialer
   - HTTPS-only enforcement

4. **mTLS Enforcement** - Node Agent gRPC server requires mutual TLS with TLS 1.3 minimum.

5. **Row Level Security** - PostgreSQL RLS policies use `current_setting('app.current_customer_id')::UUID` for multi-tenant isolation.

6. **Constant-Time Comparison** - API key hashing uses `crypto/sha256` with proper constant-time comparison via `subtle.ConstantTimeCompare`.

7. **Command Injection Prevention** - Interface names in bandwidth management (`tc` commands) and nftables rules are validated against `^[a-zA-Z0-9_]+$` regex before shell execution.

8. **Hostname Validation** - RFC 1123 hostname validation prevents YAML injection in cloud-init configurations.

9. **Error Handling** - Recovery middleware logs stack traces server-side only, returning generic error messages to clients.

---

## Detailed Findings

### FINDING-001: Redis Rate Limiter Fails Open on Error

**Severity:** MEDIUM
**Location:** `internal/controller/api/middleware/ratelimit.go:476`
**Category:** Logic
**STRIDE:** Denial of Service, Elevation of Privilege
**CWE:** CWE-693 (Protection Mechanism Failure)

**Description:**
When the Redis backend is unavailable or returns an error, the rate limiter allows the request to proceed ("fail open"). This means that during Redis outages or network partitions, all rate limiting is effectively disabled.

**Evidence:**
```go
if err != nil {
    // On Redis error, fail open (allow request)
    return true, limit, resetAt
}
```

**Impact:**
An attacker could exploit Redis downtime to perform brute force attacks against authentication endpoints, enumeration attacks, or resource exhaustion without rate limiting constraints.

**Recommendation:**
Implement a circuit breaker pattern and fail closed (deny requests) when the rate limit backend is unavailable. Consider a fallback to local in-memory rate limiting as a degraded mode.

---

### FINDING-002: Weak Default NATS Auth Token in Compose File

**Severity:** LOW
**Location:** `docker-compose.yml:55`
**Category:** Configuration
**STRIDE:** Spoofing, Elevation of Privilege
**CWE:** CWE-1188 (Initialization with Hard-Coded Network Resource Configuration)

**Description:**
The Docker Compose file uses `${NATS_AUTH_TOKEN:-changeme}` as a default fallback for the NATS authentication token. If `NATS_AUTH_TOKEN` is not explicitly set, the default `changeme` is used, which is a known weak value.

**Impact:**
In misconfigured deployments where `NATS_AUTH_TOKEN` is not set, an attacker with network access to the NATS server could authenticate using the default token and inject malicious task messages.

**Recommendation:**
Remove the default fallback and require `NATS_AUTH_TOKEN` to be set explicitly in all environments. Fail fast at startup if not configured.

---

### FINDING-003: In-Memory Rate Limiting Doesn't Protect Distributed Deployments

**Severity:** INFO
**Location:** `internal/controller/api/middleware/ratelimit.go:148`
**Category:** Configuration
**STRIDE:** Denial of Service, Elevation of Privilege

**Description:**
The default rate limiting implementation uses in-memory storage. In deployments with multiple controller instances behind a load balancer, each instance maintains its own rate limit counter, allowing attackers to circumvent limits by distributing requests across instances.

**Impact:**
In multi-instance production deployments without Redis, attackers can exceed intended rate limits by distributing requests across instances.

**Recommendation:**
Deploy with Redis-backed rate limiting in multi-instance production environments. Document this requirement in the deployment guide.

---

### FINDING-004: Password Generation Falls Back to math/rand on crypto/rand Failure

**Severity:** LOW
**Location:** `internal/controller/api/provisioning/vms.go:102-103`
**Category:** Cryptography
**STRIDE:** Tampering, Elevation of Privilege
**CWE:** CWE-1241 (Use of Predictable Algorithm in Random Number Generator)

**Description:**
The `generateRandomPassword` function catches errors from `crypto/rand.Int` and falls back to `math/rand.Intn` for character selection. While crypto/rand failure is rare, this fallback generates predictable passwords if the system entropy source is exhausted.

**Evidence:**
```go
n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(charset))))
if err != nil {
    // Fallback: use math/rand seeded by time (acceptable only here as last resort)
    return charset[mathrand.Intn(len(charset))]
}
```

**Impact:**
If `crypto/rand` fails due to system entropy exhaustion, generated passwords become predictable based on the default `math/rand` seed, potentially allowing password guessing attacks.

**Recommendation:**
Panic or return an error instead of falling back to non-cryptographic RNG. `crypto/rand` failure indicates a serious system issue that should halt operations rather than silently degrade security.

---

### FINDING-005: SSO Token Passed via URL Query Parameter

**Severity:** INFO
**Location:** `modules/servers/virtuestack/lib/VirtueStackHelper.php:445-448`
**Category:** Configuration
**STRIDE:** Information Disclosure, Spoofing
**CWE:** CWE-598 (Use of GET Request Method With Sensitive Query Strings)

**Description:**
The WHMCS module generates SSO URLs that include the JWT token as a query parameter (`?sso_token=...`). This exposes the authentication token to:
- Browser history
- Referer headers when navigating to external links
- Server access logs
- Proxy logs

**Evidence:**
```php
public static function buildWebuiUrl(string $webuiUrl, string $vmId, string $ssoToken): string
{
    $webuiUrl = rtrim($webuiUrl, '/');
    return "{$webuiUrl}/vm/{$vmId}?sso_token={$ssoToken}";
}
```

**Impact:**
An attacker with access to browser history, logs, or network traffic could capture SSO tokens and impersonate users. The default 1-hour token expiry limits but does not eliminate this risk.

**Fix Applied:**
Implemented a full opaque token exchange flow for WHMCS SSO:

1. **Provisioning API** now issues short-lived single-use opaque SSO tokens stored server-side
2. **Customer auth exchange endpoint** consumes the opaque token, sets the standard HttpOnly session cookies, and redirects to the clean customer WebUI path
3. **WHMCS module** now builds SSO URLs that carry only the opaque bootstrap token rather than a JWT containing customer identity claims

**Result:**
- No JWT bearer token is exposed in browser history
- No JWT bearer token is leaked via reverse-proxy logs or referer headers
- Tokens are short-lived and single-use even while delivered via query string
- Successful issuance and redemption are audit logged server-side

---

## Security Controls Verified

The following security controls were verified as properly implemented:

| Control | Status | Notes |
|---------|--------|-------|
| JWT Authentication | ✅ Verified | HMAC-SHA256 only, proper validation |
| API Key Authentication | ✅ Verified | Constant-time comparison, SHA-256 hashing |
| 2FA Enforcement | ✅ Verified | Admin accounts require 2FA, temp tokens blocked from non-2FA endpoints |
| RLS Policies | ✅ Verified | Proper `current_setting` usage with UUID cast |
| SQL Injection | ✅ Verified | All queries use parameterized `$1, $2` placeholders |
| SSRF Protection | ✅ Verified | Dual-layer: DNS pre-flight + connect-time validation |
| Command Injection | ✅ Verified | Input validation before exec.Command, no shell metacharacters |
| Path Traversal | ✅ Verified | validatePath function with filepath.Clean and prefix check |
| mTLS | ✅ Verified | TLS 1.3 minimum, RequireAndVerifyClientCert |
| Secret Redaction | ✅ Verified | Secret type with [REDACTED] string representation |
| Production Validation | ✅ Verified | Rejects known insecure defaults in production mode |
| WHMCS Webhook Signature | ✅ Verified | Uses `hash_equals` for constant-time comparison |
| nftables Rule Injection | ✅ Verified | Interface names validated with `^[a-zA-Z0-9_]+$` regex |
| Task System Authentication | ✅ Verified | NATS auth required, controlled task types |

---

## Recommendations

### Priority 1 (Address Soon)
1. Implement fail-closed behavior for Redis rate limiter with circuit breaker pattern
2. Remove default NATS auth token fallback from docker-compose.yml

### Priority 2 (Address Eventually)
1. Panic on crypto/rand failure instead of falling back to math/rand
2. Add documentation for Redis-backed rate limiting requirement in multi-instance deployments
3. Implement POST-based SSO token exchange flow for WHMCS integration

### Priority 3 (Defense in Depth)
1. Consider adding security headers middleware (CSP, X-Content-Type-Options, etc.)
2. Add audit logging for security-sensitive operations (password changes, 2FA enable/disable)

---

## Additional Observations

### Frontend Security (WebUI)
- Uses `sessionStorage` for auth state (preferred over `localStorage` for XSS mitigation)
- No `dangerouslySetInnerHTML` usage found, relying on React's default XSS protections
- Auth tokens stored in memory/sessionStorage, cleared on logout
- CSRF token included in API requests via X-CSRF-Token header

### Task System Security
- NATS authentication required for publishing tasks
- Task types are controlled constants (e.g., `models.TaskTypeVMCreate`), not user-provided
- Payloads created by internal services with validation
- 5-minute timeout on all task handlers
- Unknown task types are rejected and logged

### WHMCS PHP Module Security
- No SQL injection vectors found (uses Capsule ORM with parameter binding)
- No dangerous PHP functions (`eval`, `exec`, `system`, `shell_exec`, `passthru`)
- Webhook signature verification uses `hash_equals` (constant-time comparison)
- Input validation with `htmlspecialchars` and type checking
- Request size limit (64KB) on webhook endpoint
- SSO token exposure documented with recommended fix

### E2E Test Coverage (Security Scenarios)
- SQL injection tests on login form
- Rate limiting tests on authentication endpoints
- CSRF protection verification
- Secure cookie attribute verification (HttpOnly, Secure, SameSite)
- 2FA flow tests for admin and customer
- Password reset flow tests
- Session management tests

### Admin API Unit Test Coverage (NEW - 2026-03-20)

Added comprehensive unit tests for admin API input validation:

| Package | Test File | Tests | Coverage |
|---------|-----------|-------|----------|
| `api/admin` | `auth_test.go` | 13 | Login, Verify2FA, RefreshToken, Logout, Me validation |
| `api/admin` | `customers_test.go` | 9 | ListCustomers, GetCustomer, UpdateCustomer, DeleteCustomer |
| `api/admin` | `nodes_test.go` | 15 | All node CRUD operations, drain/failover/undrain |

**Total: 37 tests** covering:
- UUID validation (prevents injection via malformed IDs)
- Email and password validation (prevents weak credentials, format attacks)
- TOTP code validation (length, numeric-only)
- Query parameter limits (prevents DoS via long strings)
- Error response format consistency (API contract)

### VM State Management
- Status checks in place for sensitive operations (console, ISO attach)
- Potential for concurrent operation conflicts (e.g., migration during reinstall) - business logic concern, not security

### Abuse Prevention
- nftables rules block outbound SMTP (port 25) to prevent spam
- Metadata endpoint (169.254.169.254) blocked to prevent SSRF
- Interface names validated before rule creation

---

## Methodology

This audit applied:
- Static code review of Go backend (all Tiers 1-3)
- Frontend code review for XSS/storage patterns
- WHMCS PHP module review for injection vectors and authentication
- STRIDE threat modeling for each component
- Data flow analysis from HTTP request to database
- Pattern matching for known vulnerability classes
- Configuration review for Docker and deployment files
- E2E test coverage validation for security scenarios
