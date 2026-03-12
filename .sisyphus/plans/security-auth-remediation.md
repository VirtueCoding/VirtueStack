# VirtueStack Security & Authentication Remediation Plan
## Phase 1: Critical Security Fixes (Weeks 1-2)

**Plan Created:** March 11, 2026  
**Priority:** CRITICAL  
**Estimated Duration:** 2 weeks  
**Dependencies:** None  

---

## TL;DR

> **Quick Summary:** Fix critical security vulnerabilities including insecure password hashing, missing authentication, and production-unsafe code before any deployment.
> 
> **Deliverables:**
> - Secure password hashing with Argon2id
> - JWT authentication for both admin and customer portals
> - Secured provisioning API with proper authentication
> - Removed default passwords from all configurations
> 
> **Estimated Effort:** Medium  
> **Parallel Execution:** YES - 4 waves  
> **Critical Path:** Task 1 → Task 2 → Task 5 → Task 9

---

## Context

### Original Request
Remediate critical security issues identified in CODEBASE_AUDIT_REPORT.md to make the codebase safe for deployment.

### Issues Being Addressed

| ID | Issue | Severity | File |
|----|-------|----------|------|
| 1.1 | Insecure password hashing | CRITICAL | `internal/controller/tasks/handlers.go:701-707` |
| 1.2 | No auth (Admin Portal) | CRITICAL | `webui/admin/app/login/page.tsx:45-58` |
| 1.3 | No auth (Customer Portal) | CRITICAL | `webui/customer/app/login/page.tsx:45-58` |
| 1.4 | Production warnings | CRITICAL | `internal/controller/api/provisioning/routes.go:107` |
| 1.5 | Default passwords | HIGH | `docker-compose.yml`, `Makefile`, `.env.example` |

### Current State
- Password hashing uses reversible SHA-512 placeholder
- Login forms mock success without API calls
- Provisioning API has developer warnings about production safety
- Default "changeme" passwords in configuration files

### Target State
- Argon2id password hashing with proper salting
- Full JWT authentication flow for both portals
- Secured provisioning API with API key auth
- No default passwords, explicit configuration required

---

## Work Objectives

### Core Objective
Eliminate all CRITICAL security vulnerabilities to enable safe deployment.

### Concrete Deliverables
- `internal/controller/tasks/handlers.go` - Secure hashPassword function
- `webui/admin/app/login/page.tsx` - Connected to auth API
- `webui/customer/app/login/page.tsx` - Connected to auth API
- `webui/admin/lib/auth.ts` - Auth context and token management
- `webui/customer/lib/auth.ts` - Auth context and token management
- `internal/controller/api/middleware/auth.go` - Enhanced auth middleware
- `internal/controller/api/provisioning/middleware/auth.go` - API key validation
- Updated configuration files without default passwords

### Definition of Done
- [ ] All password hashing uses Argon2id
- [ ] Login forms connect to backend auth API
- [ ] JWT tokens properly generated, stored, and refreshed
- [ ] Provisioning API requires valid API key
- [ ] No "changeme" defaults in any config file
- [ ] All existing tests pass
- [ ] Security test suite added and passing

### Must Have
- Argon2id password hashing implementation
- JWT token generation and validation
- Frontend auth context with token storage
- API key middleware for provisioning endpoints
- Configuration validation on startup

### Must NOT Have (Guardrails)
- Do NOT remove existing 2FA/TOTP support
- Do NOT break existing API authentication middleware
- Do NOT change database schema for this phase
- Do NOT add new dependencies without review
- Do NOT leave any TODO comments in auth code

---

## Verification Strategy

### Test Decision
- **Infrastructure exists:** YES (Go tests + Playwright E2E)
- **Automated tests:** YES (TDD approach)
- **Framework:** `go test` for backend, Playwright for frontend
- **TDD:** Each task follows RED → GREEN → REFACTOR

### QA Policy
Every task MUST include agent-executed QA scenarios:
- **Backend API:** Use Bash (curl) — Send requests, assert status + response
- **Frontend:** Use Playwright — Navigate, interact, assert DOM
- **Security:** Use Bash — Attempt exploits, verify blocked

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — backend security foundations):
├── Task 1: Fix password hashing [quick]
├── Task 2: Add auth middleware tests [quick]
├── Task 3: Create shared auth types [quick]
└── Task 4: Add config validation [quick]

Wave 2 (After Wave 1 — authentication implementation):
├── Task 5: Implement admin auth API [deep]
├── Task 6: Implement customer auth API [deep]
├── Task 7: Secure provisioning API [unspecified-high]
└── Task 8: Add rate limiting middleware [unspecified-high]

Wave 3 (After Wave 2 — frontend integration):
├── Task 9: Admin portal auth context [visual-engineering]
├── Task 10: Customer portal auth context [visual-engineering]
├── Task 11: Connect login forms to API [visual-engineering]
└── Task 12: Add token refresh logic [quick]

Wave 4 (After Wave 3 — configuration & cleanup):
├── Task 13: Remove default passwords [quick]
├── Task 14: Add startup validation [quick]
├── Task 15: Security test suite [deep]
└── Task 16: Final integration test [deep]

Critical Path: Task 1 → Task 5 → Task 9 → Task 15
Parallel Speedup: ~60% faster than sequential
Max Concurrent: 4
```

### Dependency Matrix

| Task | Depends On | Blocks |
|------|------------|--------|
| 1 | — | 5, 6 |
| 2 | — | 5, 6, 7 |
| 3 | — | 5, 6, 9, 10 |
| 4 | — | 14 |
| 5 | 1, 2, 3 | 9, 11 |
| 6 | 1, 2, 3 | 10, 11 |
| 7 | 2 | 15 |
| 8 | — | 15 |
| 9 | 3, 5 | 11, 12 |
| 10 | 3, 6 | 11, 12 |
| 11 | 5, 6, 9, 10 | 15, 16 |
| 12 | 9, 10 | 15 |
| 13 | — | 14 |
| 14 | 4, 13 | 15 |
| 15 | 7, 8, 11, 12, 14 | 16 |
| 16 | 15 | — |

### Agent Dispatch Summary

- **Wave 1:** 4 tasks → all `quick`
- **Wave 2:** 4 tasks → 2 `deep`, 2 `unspecified-high`
- **Wave 3:** 4 tasks → 3 `visual-engineering`, 1 `quick`
- **Wave 4:** 4 tasks → 2 `quick`, 2 `deep`

---

## TODOs

- [ ] 1. Fix Password Hashing (Argon2id)

  **What to do:**
  - Replace the placeholder `hashPassword` function in `internal/controller/tasks/handlers.go`
  - Use the existing `argon2id` package already in go.mod
  - Implement proper salt generation (16+ bytes random)
  - Add password strength validation (min 8 chars, complexity)
  - Update function signature to return error for invalid inputs

  **Must NOT do:**
  - Do NOT change function signature for existing callers (maintain backward compatibility)
  - Do NOT break existing cloud-init password generation
  - Do NOT remove the function entirely - callers depend on it

  **Recommended Agent Profile:**
  - **Category:** `quick`
  - **Skills:** [] (standard Go development)

  **Parallelization:**
  - **Can Run In Parallel:** YES
  - **Parallel Group:** Wave 1 (with Tasks 2, 3, 4)
  - **Blocks:** Tasks 5, 6
  - **Blocked By:** None

  **References:**
  - `internal/controller/tasks/handlers.go:701-707` - Current placeholder to replace
  - `internal/shared/crypto/crypto.go` - Existing crypto utilities
  - `go.mod:6` - argon2id package already imported
  - `docs/MASTER_CODING_STANDARD_V2.md` - Password handling guidelines

  **Acceptance Criteria:**
  - [ ] `hashPassword` uses Argon2id with proper salt
  - [ ] Salt is randomly generated for each password
  - [ ] Function handles empty string edge case
  - [ ] Unit test for hashing and verification added
  - [ ] Existing callers continue to work

  **QA Scenarios:**
  ```
  Scenario: Password hashing produces different hashes for same password
    Tool: Bash (go test)
    Steps:
      1. Run: cd /home/VirtueStack && go test ./internal/controller/tasks/... -run TestHashPassword
      2. Verify test passes with different hashes for same input
    Expected Result: All tests pass
    Evidence: .sisyphus/evidence/task-01-password-hash.txt

  Scenario: Empty password returns empty hash without error
    Tool: Bash (go test)
    Steps:
      1. Run: cd /home/VirtueStack && go test ./internal/controller/tasks/... -run TestHashPasswordEmpty
    Expected Result: Test passes, empty string returns empty
    Evidence: .sisyphus/evidence/task-01-empty-password.txt
  ```

  **Commit:** YES
  - Message: `fix(security): replace placeholder password hashing with Argon2id`
  - Files: `internal/controller/tasks/handlers.go`, `internal/controller/tasks/handlers_test.go`

---

- [ ] 2. Add Auth Middleware Tests

  **What to do:**
  - Create comprehensive tests for existing auth middleware
  - Test JWT token validation
  - Test expired token handling
  - Test invalid token handling
  - Test 2FA temp token handling
  - These tests will verify existing code works and catch regressions

  **Must NOT do:**
  - Do NOT modify existing middleware code yet
  - Do NOT break any existing tests
  - Do NOT skip testing error paths

  **Recommended Agent Profile:**
  - **Category:** `quick`
  - **Skills:** [] (standard Go testing)

  **Parallelization:**
  - **Can Run In Parallel:** YES
  - **Parallel Group:** Wave 1 (with Tasks 1, 3, 4)
  - **Blocks:** Tasks 5, 6, 7
  - **Blocked By:** None

  **References:**
  - `internal/controller/api/middleware/auth.go` - Middleware to test
  - `tests/integration/auth_test.go` - Existing auth tests for patterns
  - `internal/controller/services/auth_service.go` - Auth service implementation

  **Acceptance Criteria:**
  - [ ] Test file created: `internal/controller/api/middleware/auth_test.go`
  - [ ] Tests cover JWT validation, expiration, invalid tokens
  - [ ] Tests cover 2FA temp token handling
  - [ ] All tests pass: `go test ./internal/controller/api/middleware/...`

  **QA Scenarios:**
  ```
  Scenario: All auth middleware tests pass
    Tool: Bash (go test)
    Steps:
      1. Run: cd /home/VirtueStack && go test ./internal/controller/api/middleware/... -v
    Expected Result: All tests pass with >90% coverage
    Evidence: .sisyphus/evidence/task-02-auth-tests.txt
  ```

  **Commit:** YES
  - Message: `test(middleware): add comprehensive auth middleware tests`
  - Files: `internal/controller/api/middleware/auth_test.go`

---

- [ ] 3. Create Shared Auth Types

  **What to do:**
  - Create shared TypeScript types for authentication in frontend
  - Define LoginRequest, LoginResponse, TokenResponse types
  - Define User, Session types
  - Ensure types match backend API response shapes
  - Create in both admin and customer portals

  **Must NOT do:**
  - Do NOT duplicate types between portals unnecessarily
  - Do NOT add fields that don't exist in backend
  - Do NOT use `any` types

  **Recommended Agent Profile:**
  - **Category:** `quick`
  - **Skills:** [`frontend-ui-ux`]

  **Parallelization:**
  - **Can Run In Parallel:** YES
  - **Parallel Group:** Wave 1 (with Tasks 1, 2, 4)
  - **Blocks:** Tasks 5, 6, 9, 10
  - **Blocked By:** None

  **References:**
  - `docs/API.md:60-150` - Auth API documentation
  - `internal/controller/models/customer.go` - Backend customer model
  - `internal/controller/api/middleware/auth.go` - JWT claims structure

  **Acceptance Criteria:**
  - [ ] `webui/admin/lib/types/auth.ts` created
  - [ ] `webui/customer/lib/types/auth.ts` created
  - [ ] Types match backend API responses exactly
  - [ ] No `any` types used
  - [ ] TypeScript compilation passes

  **QA Scenarios:**
  ```
  Scenario: TypeScript compilation succeeds
    Tool: Bash (npm)
    Steps:
      1. Run: cd /home/VirtueStack/webui/admin && npm run build
      2. Run: cd /home/VirtueStack/webui/customer && npm run build
    Expected Result: Both build without type errors
    Evidence: .sisyphus/evidence/task-03-types-compile.txt
  ```

  **Commit:** YES
  - Message: `feat(webui): add shared authentication types`
  - Files: `webui/admin/lib/types/auth.ts`, `webui/customer/lib/types/auth.ts`

---

- [ ] 4. Add Configuration Validation

  **What to do:**
  - Add startup validation for required environment variables
  - Validate JWT_SECRET is set and minimum length
  - Validate ENCRYPTION_KEY is set
  - Validate DATABASE_URL format
  - Fail fast on missing critical configuration
  - Add warning for default/weak passwords

  **Must NOT do:**
  - Do NOT break existing config loading
  - Do NOT add new required env vars without updating .env.example
  - Do NOT log secrets in error messages

  **Recommended Agent Profile:**
  - **Category:** `quick`
  - **Skills:** [] (standard Go development)

  **Parallelization:**
  - **Can Run In Parallel:** YES
  - **Parallel Group:** Wave 1 (with Tasks 1, 2, 3)
  - **Blocks:** Task 14
  - **Blocked By:** None

  **References:**
  - `internal/shared/config/config.go` - Config loading
  - `.env.example` - Required variables
  - `cmd/controller/main.go` - Startup location for validation

  **Acceptance Criteria:**
  - [ ] Validation function in `internal/shared/config/validation.go`
  - [ ] Called at startup in main.go
  - [ ] Fatal error on missing JWT_SECRET
  - [ ] Warning logged for weak/default passwords
  - [ ] Tests for validation function

  **QA Scenarios:**
  ```
  Scenario: App fails to start without JWT_SECRET
    Tool: Bash
    Steps:
      1. Run: JWT_SECRET= go run ./cmd/controller 2>&1
      2. Verify error message about missing JWT_SECRET
    Expected Result: App exits with error
    Evidence: .sisyphus/evidence/task-04-validation.txt
  ```

  **Commit:** YES
  - Message: `feat(config): add startup validation for critical environment variables`
  - Files: `internal/shared/config/validation.go`, `internal/shared/config/validation_test.go`, `cmd/controller/main.go`

---

- [ ] 5. Implement Admin Auth API

  **What to do:**
  - Verify `/api/v1/admin/auth/login` endpoint works correctly
  - Ensure JWT token is generated with correct claims
  - Ensure 2FA flow works (temp token → verify → access token)
  - Add proper error responses for invalid credentials
  - Add rate limiting to prevent brute force

  **Must NOT do:**
  - Do NOT change existing token format (breaks clients)
  - Do NOT remove 2FA support
  - Do NOT log passwords or tokens

  **Recommended Agent Profile:**
  - **Category:** `deep`
  - **Skills:** [] (complex backend work)

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Task 6)
  - **Parallel Group:** Wave 2 (with Tasks 6, 7, 8)
  - **Blocks:** Tasks 9, 11
  - **Blocked By:** Tasks 1, 2, 3

  **References:**
  - `internal/controller/api/admin/routes.go` - Route definitions
  - `internal/controller/services/auth_service.go` - Auth service
  - `docs/API.md:60-100` - Auth API spec
  - `tests/integration/auth_test.go` - Existing integration tests

  **Acceptance Criteria:**
  - [ ] Login returns valid JWT for correct credentials
  - [ ] Login returns 401 for incorrect credentials
  - [ ] 2FA flow returns temp token, then access token after verification
  - [ ] Rate limiting prevents >5 failed attempts per minute
  - [ ] Integration tests pass

  **QA Scenarios:**
  ```
  Scenario: Admin login returns valid JWT
    Tool: Bash (curl)
    Steps:
      1. Run: curl -X POST http://localhost:8080/api/v1/admin/auth/login \
         -H "Content-Type: application/json" \
         -d '{"email":"admin@test.com","password":"testpass"}'
      2. Parse response, verify token exists
      3. Decode JWT, verify claims (sub, exp, role)
    Expected Result: 200 OK with valid JWT token
    Evidence: .sisyphus/evidence/task-05-admin-login.txt

  Scenario: Invalid credentials return 401
    Tool: Bash (curl)
    Steps:
      1. Run: curl -X POST http://localhost:8080/api/v1/admin/auth/login \
         -H "Content-Type: application/json" \
         -d '{"email":"admin@test.com","password":"wrongpass"}'
    Expected Result: 401 Unauthorized
    Evidence: .sisyphus/evidence/task-05-invalid-creds.txt
  ```

  **Commit:** YES
  - Message: `feat(auth): ensure admin auth API works with rate limiting`
  - Files: `internal/controller/api/admin/auth.go`, `internal/controller/api/middleware/ratelimit.go`

---

- [ ] 6. Implement Customer Auth API

  **What to do:**
  - Verify `/api/v1/customer/auth/login` endpoint works correctly
  - Ensure JWT token is generated with customer claims
  - Ensure 2FA flow works for customers
  - Add proper error responses
  - Add rate limiting

  **Must NOT do:**
  - Do NOT share code inappropriately with admin auth (separate concerns)
  - Do NOT expose admin-only fields in customer tokens
  - Do NOT log sensitive data

  **Recommended Agent Profile:**
  - **Category:** `deep`
  - **Skills:** [] (complex backend work)

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Task 5)
  - **Parallel Group:** Wave 2 (with Tasks 5, 7, 8)
  - **Blocks:** Tasks 10, 11
  - **Blocked By:** Tasks 1, 2, 3

  **References:**
  - `internal/controller/api/customer/routes.go` - Route definitions
  - `internal/controller/api/customer/auth.go` - Auth handler
  - `internal/controller/services/auth_service.go` - Shared auth service
  - `docs/API.md:100-150` - Customer auth spec

  **Acceptance Criteria:**
  - [ ] Login returns valid JWT for correct customer credentials
  - [ ] Login returns 401 for incorrect credentials
  - [ ] Customer token has customer_id claim, not admin claims
  - [ ] 2FA flow works for customers
  - [ ] Rate limiting in place

  **QA Scenarios:**
  ```
  Scenario: Customer login returns valid JWT with customer claims
    Tool: Bash (curl)
    Steps:
      1. Create test customer
      2. Run: curl -X POST http://localhost:8080/api/v1/customer/auth/login \
         -H "Content-Type: application/json" \
         -d '{"email":"customer@test.com","password":"testpass"}'
      3. Decode JWT, verify customer_id claim present, no admin claims
    Expected Result: 200 OK with valid customer JWT
    Evidence: .sisyphus/evidence/task-06-customer-login.txt
  ```

  **Commit:** YES
  - Message: `feat(auth): ensure customer auth API works with rate limiting`
  - Files: `internal/controller/api/customer/auth.go`

---

- [ ] 7. Secure Provisioning API

  **What to do:**
  - Add API key authentication middleware for provisioning endpoints
  - Require valid API key in X-API-Key header
  - Remove "WARNING: Do not use in production" comments
  - Add rate limiting per API key
  - Add audit logging for all provisioning API calls

  **Must NOT do:**
  - Do NOT break WHMCS integration endpoints
  - Do NOT change endpoint paths
  - Do NOT remove functionality, only secure it

  **Recommended Agent Profile:**
  - **Category:** `unspecified-high`
  - **Skills:** [] (security-focused work)

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Tasks 5, 6, 8)
  - **Parallel Group:** Wave 2
  - **Blocks:** Task 15
  - **Blocked By:** Task 2

  **References:**
  - `internal/controller/api/provisioning/routes.go:107` - Warning location
  - `internal/controller/api/provisioning/handler.go` - Handler to secure
  - `internal/controller/grpc_client.go:202` - Warning location
  - `docs/API.md` - Provisioning API documentation

  **Acceptance Criteria:**
  - [ ] API key middleware created and applied
  - [ ] All provisioning endpoints require valid API key
  - [ ] Invalid/missing API key returns 401
  - [ ] Warning comments removed
  - [ ] Rate limiting per API key implemented
  - [ ] Audit logging added

  **QA Scenarios:**
  ```
  Scenario: Provisioning API rejects requests without API key
    Tool: Bash (curl)
    Steps:
      1. Run: curl -X POST http://localhost:8080/vms \
         -H "Content-Type: application/json" \
         -d '{"hostname":"test"}'
    Expected Result: 401 Unauthorized
    Evidence: .sisyphus/evidence/task-07-no-apikey.txt

  Scenario: Provisioning API accepts valid API key
    Tool: Bash (curl)
    Steps:
      1. Create valid API key in database
      2. Run: curl -X POST http://localhost:8080/vms \
         -H "Content-Type: application/json" \
         -H "X-API-Key: valid-key" \
         -d '{"hostname":"test"}'
    Expected Result: Request processed (may return validation error, but not 401)
    Evidence: .sisyphus/evidence/task-07-valid-apikey.txt
  ```

  **Commit:** YES
  - Message: `fix(security): secure provisioning API with API key authentication`
  - Files: `internal/controller/api/provisioning/middleware/auth.go`, `internal/controller/api/provisioning/routes.go`

---

- [ ] 8. Add Rate Limiting Middleware

  **What to do:**
  - Create reusable rate limiting middleware
  - Implement sliding window rate limiting
  - Different limits for auth endpoints (stricter) vs API endpoints
  - Use Redis or in-memory for rate limit counters
  - Add X-RateLimit headers to responses

  **Must NOT do:**
  - Do NOT break existing middleware chain
  - Do NOT rate limit health check endpoints
  - Do NOT use external Redis if not already configured

  **Recommended Agent Profile:**
  - **Category:** `unspecified-high`
  - **Skills:** [] (performance/security)

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Tasks 5, 6, 7)
  - **Parallel Group:** Wave 2
  - **Blocks:** Task 15
  - **Blocked By:** None

  **References:**
  - `internal/controller/api/middleware/` - Existing middleware
  - `internal/controller/api/middleware/ratelimit.go` - May exist partially
  - `golang.org/x/time/rate` - Token bucket implementation

  **Acceptance Criteria:**
  - [ ] Rate limiting middleware created
  - [ ] Configurable limits per endpoint type
  - [ ] X-RateLimit-Limit, X-RateLimit-Remaining headers
  - [ ] 429 response when limit exceeded
  - [ ] Tests for rate limiting

  **QA Scenarios:**
  ```
  Scenario: Rate limit enforced after threshold
    Tool: Bash (curl loop)
    Steps:
      1. Run 10 rapid login requests
      2. Verify 429 returned after limit
    Expected Result: 429 Too Many Requests after limit
    Evidence: .sisyphus/evidence/task-08-rate-limit.txt
  ```

  **Commit:** YES
  - Message: `feat(middleware): add configurable rate limiting`
  - Files: `internal/controller/api/middleware/ratelimit.go`, `internal/controller/api/middleware/ratelimit_test.go`

---

- [ ] 9. Admin Portal Auth Context

  **What to do:**
  - Create React context for authentication state
  - Implement token storage in localStorage or httpOnly cookie
  - Create useAuth hook for components
  - Handle token refresh automatically
  - Redirect to login on 401 responses

  **Must NOT do:**
  - Do NOT store tokens in plain localStorage if httpOnly cookie available
  - Do NOT expose token in React DevTools
  - Do NOT create duplicate code with customer portal

  **Recommended Agent Profile:**
  - **Category:** `visual-engineering`
  - **Skills:** [`frontend-ui-ux`]

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Task 10)
  - **Parallel Group:** Wave 3 (with Tasks 10, 11, 12)
  - **Blocks:** Tasks 11, 12
  - **Blocked By:** Tasks 3, 5

  **References:**
  - `webui/admin/app/layout.tsx` - Where to wrap with provider
  - `webui/admin/lib/types/auth.ts` - Auth types (Task 3)
  - `docs/API.md` - Token refresh endpoint

  **Acceptance Criteria:**
  - [ ] `webui/admin/lib/auth-context.tsx` created
  - [ ] AuthProvider wraps application
  - [ ] useAuth hook returns user, login, logout, isLoading
  - [ ] Token stored securely
  - [ ] Automatic token refresh on 401

  **QA Scenarios:**
  ```
  Scenario: Auth context provides user state
    Tool: Playwright
    Steps:
      1. Navigate to admin portal
      2. Verify auth context renders without error
      3. Verify loading state shown before auth check
    Expected Result: Page renders, no auth errors
    Evidence: .sisyphus/evidence/task-09-auth-context.png
  ```

  **Commit:** YES
  - Message: `feat(admin): add authentication context and hooks`
  - Files: `webui/admin/lib/auth-context.tsx`, `webui/admin/app/providers.tsx`

---

- [ ] 10. Customer Portal Auth Context

  **What to do:**
  - Create React context for authentication state (similar to admin)
  - Implement token storage
  - Create useAuth hook
  - Handle token refresh
  - Redirect to login on 401

  **Must NOT do:**
  - Do NOT copy-paste from admin without adapting
  - Do NOT share auth state between admin and customer portals
  - Do NOT store admin-specific data in customer tokens

  **Recommended Agent Profile:**
  - **Category:** `visual-engineering`
  - **Skills:** [`frontend-ui-ux`]

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Task 9)
  - **Parallel Group:** Wave 3 (with Tasks 9, 11, 12)
  - **Blocks:** Tasks 11, 12
  - **Blocked By:** Tasks 3, 6

  **References:**
  - `webui/customer/app/layout.tsx` - Where to wrap with provider
  - `webui/customer/lib/types/auth.ts` - Auth types (Task 3)
  - `docs/API.md` - Token refresh endpoint

  **Acceptance Criteria:**
  - [ ] `webui/customer/lib/auth-context.tsx` created
  - [ ] AuthProvider wraps application
  - [ ] useAuth hook returns user, login, logout, isLoading
  - [ ] Token stored securely
  - [ ] Automatic token refresh on 401

  **QA Scenarios:**
  ```
  Scenario: Customer auth context provides user state
    Tool: Playwright
    Steps:
      1. Navigate to customer portal
      2. Verify auth context renders
      3. Verify redirect to login when not authenticated
    Expected Result: Redirect to /login for protected routes
    Evidence: .sisyphus/evidence/task-10-customer-auth.png
  ```

  **Commit:** YES
  - Message: `feat(customer): add authentication context and hooks`
  - Files: `webui/customer/lib/auth-context.tsx`, `webui/customer/app/providers.tsx`

---

- [ ] 11. Connect Login Forms to API

  **What to do:**
  - Replace mock login in admin portal with actual API call
  - Replace mock login in customer portal with actual API call
  - Handle API errors and display to user
  - Show loading state during authentication
  - Redirect to dashboard on successful login
  - Handle 2FA flow if required

  **Must NOT do:**
  - Do NOT remove form validation
  - Do NOT break accessibility (focus management, error announcements)
  - Do NOT hardcode API URLs (use environment config)

  **Recommended Agent Profile:**
  - **Category:** `visual-engineering`
  - **Skills:** [`frontend-ui-ux`]

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Tasks 9, 10, 12)
  - **Parallel Group:** Wave 3
  - **Blocks:** Tasks 15, 16
  - **Blocked By:** Tasks 5, 6, 9, 10

  **References:**
  - `webui/admin/app/login/page.tsx:45-58` - Current mock to replace
  - `webui/customer/app/login/page.tsx:45-58` - Current mock to replace
  - `webui/admin/lib/auth-context.tsx` - Auth context (Task 9)
  - `docs/API.md` - Login API specification

  **Acceptance Criteria:**
  - [ ] Admin login form calls `/api/v1/admin/auth/login`
  - [ ] Customer login form calls `/api/v1/customer/auth/login`
  - [ ] Loading state shown during authentication
  - [ ] Error message displayed for invalid credentials
  - [ ] Redirect to dashboard on success
  - [ ] 2FA modal appears when required

  **QA Scenarios:**
  ```
  Scenario: Admin login with valid credentials
    Tool: Playwright
    Steps:
      1. Navigate to http://localhost:3000/login
      2. Fill email: "admin@test.com"
      3. Fill password: "correctpass"
      4. Click submit
      5. Wait for redirect to /dashboard
    Expected Result: Redirected to dashboard, user logged in
    Evidence: .sisyphus/evidence/task-11-admin-login-flow.png

  Scenario: Admin login with invalid credentials
    Tool: Playwright
    Steps:
      1. Navigate to http://localhost:3000/login
      2. Fill email: "admin@test.com"
      3. Fill password: "wrongpass"
      4. Click submit
    Expected Result: Error message shown, stays on login page
    Evidence: .sisyphus/evidence/task-11-invalid-login.png
  ```

  **Commit:** YES
  - Message: `feat(auth): connect login forms to authentication API`
  - Files: `webui/admin/app/login/page.tsx`, `webui/customer/app/login/page.tsx`

---

- [ ] 12. Add Token Refresh Logic

  **What to do:**
  - Implement automatic token refresh before expiration
  - Add refresh endpoint handling
  - Queue requests during refresh
  - Handle refresh failure (redirect to login)
  - Add token expiration check on app load

  **Must NOT do:**
  - Do NOT refresh token on every request
  - Do NOT create refresh race conditions
  - Do NOT expose refresh token in memory longer than needed

  **Recommended Agent Profile:**
  - **Category:** `quick`
  - **Skills:** [`frontend-ui-ux`]

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Tasks 9, 10, 11)
  - **Parallel Group:** Wave 3
  - **Blocks:** Task 15
  - **Blocked By:** Tasks 9, 10

  **References:**
  - `webui/admin/lib/auth-context.tsx` - Where to add refresh logic
  - `docs/API.md` - Token refresh endpoint spec

  **Acceptance Criteria:**
  - [ ] Token refreshed 5 minutes before expiration
  - [ ] Pending requests queued during refresh
  - [ ] Failed refresh redirects to login
  - [ ] Works for both admin and customer portals

  **QA Scenarios:**
  ```
  Scenario: Token refreshes before expiration
    Tool: Playwright
    Steps:
      1. Login to admin portal
      2. Wait 6 minutes (token expires in 5)
      3. Navigate to another page
      4. Verify still authenticated
    Expected Result: Token refreshed, user stays logged in
    Evidence: .sisyphus/evidence/task-12-token-refresh.png
  ```

  **Commit:** YES
  - Message: `feat(auth): add automatic token refresh`
  - Files: `webui/admin/lib/auth-context.tsx`, `webui/customer/lib/auth-context.tsx`

---

- [ ] 13. Remove Default Passwords

  **What to do:**
  - Remove `${POSTGRES_PASSWORD:-changeme}` from docker-compose.yml
  - Remove `changeme` from Makefile DATABASE_URL
  - Update .env.example to show required fields
  - Add documentation that passwords must be set
  - Keep .env.example as template with placeholder comments

  **Must NOT do:**
  - Do NOT commit actual passwords
  - Do NOT break development environment without documentation
  - Do NOT change .env.example format significantly

  **Recommended Agent Profile:**
  - **Category:** `quick`
  - **Skills:** [] (configuration work)

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Task 14)
  - **Parallel Group:** Wave 4 (with Tasks 14, 15, 16)
  - **Blocks:** Task 14
  - **Blocked By:** None

  **References:**
  - `docker-compose.yml:19` - Default password to remove
  - `Makefile:24` - Default DATABASE_URL to update
  - `.env.example` - Template to update

  **Acceptance Criteria:**
  - [ ] No default passwords in docker-compose.yml
  - [ ] No default passwords in Makefile
  - [ ] .env.example documents required fields
  - [ ] README updated with setup instructions

  **QA Scenarios:**
  ```
  Scenario: No default passwords in config files
    Tool: Bash (grep)
    Steps:
      1. Run: grep -r "changeme" docker-compose.yml Makefile
    Expected Result: No matches found
    Evidence: .sisyphus/evidence/task-13-no-defaults.txt
  ```

  **Commit:** YES
  - Message: `fix(security): remove default passwords from configuration`
  - Files: `docker-compose.yml`, `Makefile`, `.env.example`, `README.md`

---

- [ ] 14. Add Startup Validation

  **What to do:**
  - Call validation function from Task 4 at startup
  - Fail immediately if JWT_SECRET missing
  - Fail immediately if DATABASE_URL missing
  - Warn but continue for optional configs
  - Add validation to both controller and node-agent

  **Must NOT do:**
  - Do NOT prevent development mode from starting
  - Do NOT add validation that requires database connection
  - Do NOT validate every possible config (only critical ones)

  **Recommended Agent Profile:**
  - **Category:** `quick`
  - **Skills:** [] (configuration work)

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Tasks 13, 15, 16)
  - **Parallel Group:** Wave 4
  - **Blocks:** Task 15
  - **Blocked By:** Tasks 4, 13

  **References:**
  - `internal/shared/config/validation.go` - Validation function (Task 4)
  - `cmd/controller/main.go` - Controller startup
  - `cmd/node-agent/main.go` - Node agent startup

  **Acceptance Criteria:**
  - [ ] Controller validates config on startup
  - [ ] Node agent validates config on startup
  - [ ] Missing JWT_SECRET causes immediate exit
  - [ ] Warning logged for optional missing configs

  **QA Scenarios:**
  ```
  Scenario: Controller fails without JWT_SECRET
    Tool: Bash
    Steps:
      1. Set JWT_SECRET=""
      2. Run: go run ./cmd/controller
    Expected Result: Exit with error message about JWT_SECRET
    Evidence: .sisyphus/evidence/task-14-validation.txt
  ```

  **Commit:** YES
  - Message: `feat(config): add startup validation for critical configuration`
  - Files: `cmd/controller/main.go`, `cmd/node-agent/main.go`

---

- [ ] 15. Security Test Suite

  **What to do:**
  - Create comprehensive security test suite
  - Test password hashing strength
  - Test JWT token validation
  - Test rate limiting effectiveness
  - Test API key validation
  - Test for common vulnerabilities (OWASP Top 10 relevant)
  - Add to CI/CD pipeline

  **Must NOT do:**
  - Do NOT skip slow tests (mark appropriately)
  - Do NOT create flaky tests
  - Do NOT test against production endpoints

  **Recommended Agent Profile:**
  - **Category:** `deep`
  - **Skills:** [] (security testing)

  **Parallelization:**
  - **Can Run In Parallel:** YES (with Tasks 13, 14, 16)
  - **Parallel Group:** Wave 4
  - **Blocks:** Task 16
  - **Blocked By:** Tasks 7, 8, 11, 12, 14

  **References:**
  - `tests/integration/` - Existing integration tests
  - `tests/security/` - Security test directory
  - `docs/MASTER_CODING_STANDARD_V2.md` - Security guidelines

  **Acceptance Criteria:**
  - [ ] `tests/security/auth_test.go` created
  - [ ] Tests for password hashing
  - [ ] Tests for JWT validation
  - [ ] Tests for rate limiting
  - [ ] Tests for API key auth
  - [ ] All tests pass in CI

  **QA Scenarios:**
  ```
  Scenario: All security tests pass
    Tool: Bash (go test)
    Steps:
      1. Run: go test ./tests/security/... -v
    Expected Result: All tests pass
    Evidence: .sisyphus/evidence/task-15-security-tests.txt
  ```

  **Commit:** YES
  - Message: `test(security): add comprehensive security test suite`
  - Files: `tests/security/auth_test.go`, `tests/security/ratelimit_test.go`, `tests/security/apikey_test.go`

---

- [ ] 16. Final Integration Test

  **What to do:**
  - Create end-to-end test for complete auth flow
  - Test admin login → dashboard access → API calls
  - Test customer login → VM operations → logout
  - Test session expiration and refresh
  - Test 2FA flow end-to-end
  - Verify no regressions in existing functionality

  **Must NOT do:**
  - Do NOT test against production database
  - Do NOT create test data that persists
  - Do NOT skip cleanup after tests

  **Recommended Agent Profile:**
  - **Category:** `deep`
  - **Skills:** [] (integration testing)

  **Parallelization:**
  - **Can Run In Parallel:** NO (final verification)
  - **Parallel Group:** Wave 4 Final
  - **Blocks:** None
  - **Blocked By:** Task 15

  **References:**
  - `tests/e2e/auth.spec.ts` - Existing E2E auth tests
  - `tests/integration/` - Integration test patterns

  **Acceptance Criteria:**
  - [ ] E2E test for admin auth flow
  - [ ] E2E test for customer auth flow
  - [ ] E2E test for token refresh
  - [ ] E2E test for 2FA flow
  - [ ] All existing tests still pass
  - [ ] No regressions detected

  **QA Scenarios:**
  ```
  Scenario: Complete admin auth flow works
    Tool: Playwright
    Steps:
      1. Navigate to admin login
      2. Enter valid credentials
      3. Complete 2FA if required
      4. Verify dashboard loads
      5. Make API call (e.g., list customers)
      6. Verify data returned
      7. Logout
      8. Verify redirect to login
    Expected Result: Full flow completes without error
    Evidence: .sisyphus/evidence/task-16-e2e-admin.png

  Scenario: Complete customer auth flow works
    Tool: Playwright
    Steps:
      1. Navigate to customer login
      2. Enter valid credentials
      3. Verify VM list loads
      4. Click VM details
      5. Verify VM details page loads
      6. Logout
    Expected Result: Full flow completes without error
    Evidence: .sisyphus/evidence/task-16-e2e-customer.png
  ```

  **Commit:** YES
  - Message: `test(e2e): add comprehensive auth flow integration tests`
  - Files: `tests/e2e/auth-flow.spec.ts`

---

## Final Verification Wave

> 4 review agents run in PARALLEL. ALL must APPROVE. Rejection → fix → re-run.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Verify all CRITICAL issues from audit report are addressed. Check each Must Have. Verify no Must NOT Have violations.
  Output: `CRITICAL Issues [6/6] | Must Have [5/5] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go test ./...` + `npm run build` for both webuis. Check for security anti-patterns (hardcoded secrets, weak crypto, etc.).
  Output: `Build [PASS/FAIL] | Tests [N pass/N fail] | Security [CLEAN/N issues] | VERDICT`

- [ ] F3. **Security Manual QA** — `unspecified-high`
  Attempt common attacks: SQL injection in login, brute force rate limit bypass, token manipulation, missing auth headers.
  Output: `Attacks Tested [N] | Blocked [N] | Vulnerabilities [0/N] | VERDICT`

- [ ] F4. **Documentation Check** — `deep`
  Verify README updated, .env.example correct, API documentation reflects changes, no security warnings remain.
  Output: `README [UPDATED/NEEDS UPDATE] | Config [CORRECT/ISSUES] | Warnings [0/N remain] | VERDICT`

---

## Commit Strategy

| Commit | Message | Files | Pre-commit |
|--------|---------|-------|------------|
| 1 | `fix(security): replace placeholder password hashing with Argon2id` | handlers.go, handlers_test.go | `go test` |
| 2 | `test(middleware): add comprehensive auth middleware tests` | auth_test.go | `go test` |
| 3 | `feat(webui): add shared authentication types` | auth.ts (both portals) | `npm run build` |
| 4 | `feat(config): add startup validation for critical environment variables` | validation.go, main.go | `go test` |
| 5 | `feat(auth): ensure admin auth API works with rate limiting` | auth.go, ratelimit.go | `go test` |
| 6 | `feat(auth): ensure customer auth API works with rate limiting` | auth.go | `go test` |
| 7 | `fix(security): secure provisioning API with API key authentication` | middleware/auth.go, routes.go | `go test` |
| 8 | `feat(middleware): add configurable rate limiting` | ratelimit.go | `go test` |
| 9 | `feat(admin): add authentication context and hooks` | auth-context.tsx | `npm run build` |
| 10 | `feat(customer): add authentication context and hooks` | auth-context.tsx | `npm run build` |
| 11 | `feat(auth): connect login forms to authentication API` | login/page.tsx (both) | `npm run build` |
| 12 | `feat(auth): add automatic token refresh` | auth-context.tsx (both) | `npm run build` |
| 13 | `fix(security): remove default passwords from configuration` | docker-compose.yml, Makefile, .env.example | — |
| 14 | `feat(config): add startup validation for critical configuration` | main.go (both) | `go test` |
| 15 | `test(security): add comprehensive security test suite` | tests/security/*.go | `go test` |
| 16 | `test(e2e): add comprehensive auth flow integration tests` | tests/e2e/auth-flow.spec.ts | `npx playwright test` |

---

## Success Criteria

### Verification Commands
```bash
# Backend tests
go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total # Expect >80%

# Frontend builds
cd webui/admin && npm run build
cd webui/customer && npm run build

# E2E tests
npx playwright test tests/e2e/auth-flow.spec.ts

# Security tests
go test ./tests/security/... -v
```

### Final Checklist
- [ ] All 16 tasks completed and verified
- [ ] No CRITICAL issues remaining from audit
- [ ] All Must Have requirements implemented
- [ ] No Must NOT Have violations
- [ ] All tests pass (unit, integration, E2E, security)
- [ ] No default passwords in any configuration
- [ ] No production warnings in code
- [ ] Documentation updated (README, .env.example)
- [ ] Both portals can login successfully
- [ ] Rate limiting prevents brute force
- [ ] Provisioning API requires API key

---

*Plan generated from CODEBASE_AUDIT_REPORT.md Phase 1 recommendations.*