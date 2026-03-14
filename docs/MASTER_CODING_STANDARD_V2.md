# LLM CODE GENERATION RULES — GOLD STANDARD

**Scope:** Every file, function, endpoint, module. No exceptions.
**Violation of any rule = rejected code.**

---

## 1. ABSOLUTE PROHIBITIONS

- NEVER leave `TODO`, `FIXME`, `HACK`, `XXX`, or placeholder comments. Implement completely or do not ship.
- NEVER skip error handling on any code path.
- NEVER hardcode secrets, API keys, passwords, tokens, or environment-specific values.
- NEVER use `eval()`, `exec()`, `Function()`, `vm.runInNewContext()`, or dynamic code execution with user input.
- NEVER trust client-side validation as sole defense.
- NEVER import packages without verifying they exist on official registries and are actively maintained.
- NEVER generate tutorial-style comments explaining basic syntax. Comments explain WHY, never WHAT.
- NEVER duplicate code. If logic exists, extract into shared utilities.
- NEVER use `any` (TypeScript), bare `interface{}` without type assertion (Go), or untyped parameters (PHP).
- NEVER use `console.log`, `fmt.Println`, `print_r`, or debug statements in delivered code.
- NEVER leave commented-out code. Version control handles history.
- NEVER use `import *`. Always import specific names.
- NEVER return `null`/`nil` to indicate an error. Use typed errors or Result types.
- NEVER use error strings for control flow. Use error types with `errors.Is()`/`errors.As()`.
- NEVER ignore returned errors with `_` without an explicit justifying comment.
- NEVER construct SQL with string interpolation. Parameterized queries only, no exceptions.
- NEVER fire-and-forget goroutines. Track all with `sync.WaitGroup` or `errgroup.Group`.
- NEVER use `math/rand` or `Math.random()` for security-sensitive values. Use `crypto/rand` or `crypto.randomBytes`.
- NEVER use MD5, SHA-1, DES, RC4, AES-ECB, bcrypt for new projects, SSL, TLS 1.0, or TLS 1.1.
- NEVER use `unsafe` (Go) without explicit security review.
- NEVER bake secrets into container images. No `COPY .env`, no `ARG PASSWORD=`.
- NEVER run containers as root. All containers use UID >= 1000.
- NEVER use `:latest` or mutable tags for container images. Pin by SHA digest.
- NEVER use mutable tags in CI actions. Pin all CI actions to commit SHAs.
- NEVER suppress linter warnings without a justifying comment and team approval.
- NEVER nest logic deeper than 3 levels. Use early returns and guard clauses.
- NEVER accept more than 4 function parameters. Use an options struct/object.
- NEVER rely on network segmentation as sole security. Authenticate every request.
- NEVER disable certificate validation (`verify=False`, `InsecureSkipVerify`).
- NEVER expose stack traces, file paths, SQL queries, or internal IPs in error responses.
- NEVER log passwords, tokens, API keys, credit card numbers, SSNs, full emails, or session IDs.
- NEVER use deprecated packages when maintained alternatives exist.
- NEVER assume data exists. Validate before access: check slice lengths, nil pointers, empty maps.
- NEVER catch exceptions generically without logging, re-throwing, or handling specifically.
- NEVER write empty catch blocks. Every error path must log or propagate.
- NEVER trust AI-suggested package names without manual registry verification (slopsquatting defense).
- NEVER use `promise.all()` when partial failure is acceptable. Use `Promise.allSettled()`.
- NEVER load entire files into memory. Stream large data.
- NEVER write tests that depend on execution order, real time, or shared mutable state.

---

## 2. STRUCTURAL RULES

### Architecture
- Single Responsibility Principle: each file, class, function has ONE purpose.
- Separate concerns into layers: handlers/controllers (thin) -> services (business logic) -> repositories (data access).
- Centralize cross-cutting concerns (auth, validation, error handling, logging) in middleware.
- Extract shared logic into utility modules. Zero copy-paste across files.

### Naming
- Variables: descriptive, intention-revealing. `userEmail` not `x`, `ue`, `data`.
- Functions: verb + noun. `validateUserInput`, `calculateTotalPrice`.
- Booleans: prefix with `is`, `has`, `can`, `should`. `isAuthenticated`, `hasPermission`.
- Constants: `UPPER_SNAKE_CASE`. `MAX_RETRY_COUNT`, `DEFAULT_TIMEOUT_MS`.
- Allowed abbreviations: `id`, `url`, `http`, `ctx`, `err`, `req`, `res`, `db`, `tx`, `ip`, `vm`, `os`, `io`, `rpc`. All others spelled out.

### Style
- Early returns to reduce nesting. Guard clauses at function top.
- One blank line between logical sections within a function.
- Group imports: stdlib > third-party > local, separated by blank lines.
- Consistent formatting enforced by project linter. Zero warnings.

---

## 3. ERROR HANDLING

### Required Error Types
Define and use typed errors for each category:
- `ValidationError` (400), `AuthenticationError` (401), `ForbiddenError` (403)
- `NotFoundError` (404), `ConflictError` (409), `RateLimitError` (429), `InternalError` (500)

Go: Use sentinel errors + custom types implementing `error`:
```go
var ErrNotFound = errors.New("resource not found")

type ValidationError struct {
  Field   string
  Message string
}
func (e *ValidationError) Error() string {
  return fmt.Sprintf("validation: %s — %s", e.Field, e.Message)
}
```

### Error Response Format
All API error responses use this structure:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Email format is invalid",
    "details": [{"field": "email", "issue": "Must be a valid email address"}],
    "correlationId": "req_abc123"
  }
}
```

### Resilience
- Timeouts on ALL external calls: 10s HTTP, 5s DB OLTP, 30s DB reporting, 5s gRPC unary, 60s gRPC stream.
- Retry with exponential backoff for transient failures. Max 3 retries.
- Circuit breaker: open after 5 consecutive failures, half-open after 30s cooldown, close after 2 successes.
- Graceful degradation: non-critical service failure degrades feature, never crashes system.
- Resource cleanup: `defer` (Go) or `finally` (TS/PHP) for all opened resources immediately after successful open.

### Error Wrapping (Go)
Always wrap with context: `fmt.Errorf("querying users: %w", err)`.
Check errors with `errors.Is()` and `errors.As()`. Never compare error strings.

### Multi-Step Operations
Write state to database before each step. On recovery, replay or roll back.
States: `pending -> step_N_complete -> completed | failed | rolled_back`.

---

## 4. SECURITY

### Access Control
- Enforce server-side only. Default deny.
- Centralized authorization middleware on all routes.
- Record-level ownership enforced: `resource.customer_id == jwt.customer_id`.
- RBAC or ABAC with role hierarchy.
- CORS: strict origin allowlist, no wildcards in production.
- JWTs: 15min access, 7d customer refresh, 4h admin refresh. Rotation on use.
- Rate limit all endpoints. Log and alert all access control failures.
- PostgreSQL RLS for multi-tenant data isolation.

### SSRF Prevention
- Validate and allowlist all URLs before server-side requests.
- Block private IP ranges: `10.x`, `172.16-31.x`, `192.168.x`, `169.254.169.254`.
- Block cloud metadata endpoints.
- Disable HTTP redirects on server-side requests or re-validate after each redirect.
- DNS rebinding protection: resolve hostname, validate IP, then connect.

### Injection Prevention
- Parameterized queries for ALL database operations. No string interpolation.
- Context-aware output encoding: HTML entity, JS Unicode, URL percent, CSS hex.
- Template engines with auto-escaping enabled.
- Input allowlisting over denylisting.

### Authentication
- Use established auth libraries. Never custom crypto or token generation.
- Store refresh tokens server-side (database), never in `localStorage`.
- Session cookies: `HttpOnly`, `Secure`, `SameSite=Strict`.
- CSRF: double-submit cookie pattern for cookie-based auth APIs.
- Password storage: Argon2id only for new code. bcrypt legacy only with migration plan.
- Account lockout after 5-10 failed attempts with progressive delay.
- Minimum 12-character passwords, checked against breached lists.

### Admin Session Hardening
- 4-hour refresh token lifetime.
- Re-authentication for destructive operations.
- Max 3 concurrent admin sessions.
- Log IP and user-agent per session; alert on mid-session changes.

### Cryptographic Standards
| Use Case | Required | Banned |
|----------|----------|--------|
| Password hashing | Argon2id | MD5, SHA-1, SHA-256 plain, DES, bcrypt (new code) |
| Symmetric encryption | AES-256-GCM | AES-ECB, DES, RC4, Blowfish |
| Non-password hashing | SHA-256, SHA-3 | MD5, SHA-1 |
| Token generation | `crypto/rand`, `crypto.randomBytes(32)` | `math/rand`, `Math.random()` |
| TLS | 1.3 (min 1.2) | SSL, TLS 1.0, TLS 1.1 |

### Secrets Management
- All secrets via environment variables or secret manager (Vault, AWS SSM).
- `.env.example` committed with placeholders. `.env` in `.gitignore`.
- Different secrets per environment.
- JWKS rotation: 30 days. DB credentials: 90 days. TLS certs: auto-renew 30 days before expiry.

### Zero-Trust Service Auth
- mTLS for all internal service communication.
- Each service has its own identity (SPIFFE ID or K8s ServiceAccount).
- Short-lived credentials: minutes, not hours.

---

## 5. INPUT VALIDATION

- ALL external data validated server-side before processing: request body, query params, URL params, headers, cookies, file uploads, webhooks, WebSocket messages, third-party API responses.
- Schema validation using established library (Zod/TS, go-playground/validator/Go, Symfony Validator/PHP).
- Type checking, range checking (min/max), length limits, format validation, allowlisting.
- Sanitization AFTER validation, not instead of.

Go example:
```go
type CreateVMRequest struct {
  PlanID     uuid.UUID `json:"plan_id" validate:"required"`
  Hostname   string    `json:"hostname" validate:"required,min=1,max=63,hostname_rfc1123"`
}
```

TypeScript example:
```typescript
const CreateUserSchema = z.object({
  email: z.string().email().max(254),
  name: z.string().min(1).max(100),
  role: z.enum(['user', 'admin', 'moderator']),
});
const result = CreateUserSchema.safeParse(req.body);
if (!result.success) throw new ValidationError(result.error.issues);
```

---

## 6. LOGGING, TRACING & OBSERVABILITY

### Every Log Entry Must Include
`timestamp`, `level`, `message`, `service`, `correlationId`, `traceId`, `spanId`, `duration_ms`.

### Log Levels
| Level | Use |
|-------|-----|
| `error` | Failures requiring immediate attention |
| `warn` | Degraded behavior, circuit breaker opened, approaching limits |
| `info` | Business events: user created, order placed, deployment completed |
| `debug` | Development-only. NEVER in production |

### Log Hygiene
- Mask PII: `u***@e***.com`.
- Rate-limit repetitive error logs: max 1/second per error class.
- Include enough context to reproduce without database access.

### Distributed Tracing (OpenTelemetry)
- Instrument every incoming HTTP/gRPC request, outgoing call, DB query, cache operation, queue publish/consume.
- Propagate W3C Trace Context across all service boundaries.
- Inject `traceId` and `spanId` into every log entry for trace-log correlation.
- Record errors on spans: `span.RecordError(err)`, `span.SetStatus(codes.Error, err.Error())`.

### Metrics
- RED per endpoint: Rate (req/s), Errors (4xx/5xx rate), Duration (p50, p95, p99).
- USE per resource: Utilization, Saturation, Errors.

### Health Probes
- `GET /healthz` — liveness: process alive (200 if running).
- `GET /readyz` — readiness: can accept traffic (checks DB, cache, dependencies).
- `GET /metrics` — Prometheus-compatible metrics.

### Alerting Rules (committed as code)
- Error rate > 5% over 5 minutes.
- p99 latency > 2x baseline over 5 minutes.
- Health check failure > 30 seconds.
- Circuit breaker open events.
- Disk usage > 80%.
- Certificate expiry < 14 days.

---

## 7. DATABASE

- Index every `WHERE`, `JOIN`, `ORDER BY` column.
- No N+1 queries. Use `JOIN`, `IN`, or batch loading.
- Cursor-based pagination on ALL list endpoints.
- Connection pooling mandatory. Go: `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`.
- Statement-level timeouts via `context.WithTimeout`.
- `EXPLAIN ANALYZE` on all new queries against production-scale data before merge.
- Optimistic locking (`version` column) for concurrent updates.

### Zero-Downtime Migrations (expand-contract)
1. **Expand:** Add nullable columns with defaults, create new tables, `CREATE INDEX CONCURRENTLY`. Never rename or drop.
2. **Migrate:** Code writes to old and new. Backfill. Verify integrity.
3. **Contract:** Remove old reads. Drop old columns in separate migration.

Rules:
- Every migration has a rollback script.
- Migrations are idempotent.
- `SET lock_timeout = '5s'` to prevent long table locks.
- Test against production-size dataset before deploy.

---

## 8. PERFORMANCE

- No O(n^2+) where O(n) or O(n log n) exists.
- No unbounded memory allocation. Cap collections, stream large datasets.
- Response time targets: p50 < 100ms, p95 < 500ms, p99 < 1000ms for CRUD.
- gzip/brotli compression for responses > 1KB.
- HTTP cache headers (`ETag`, `Cache-Control`) for read-heavy endpoints.
- Batch endpoints for operations commonly done in loops.
- Go: `sync.Pool` for frequently allocated objects. Watch for goroutine leaks.
- Node.js: avoid closure-based memory leaks. Use `WeakRef`/`WeakMap` for caches.

---

## 9. CONCURRENCY

### Go
- Every goroutine has cancellation via `context.Context`.
- Channel direction in signatures: `chan<-` send-only, `<-chan` receive-only.
- `select` with `context.Done()` in every long-running goroutine.
- Minimize mutex scope. Prefer `defer mu.Unlock()`.
- `go test -race -count=1 ./...` in CI. Use `goleak` in tests.
- Bound concurrency with `errgroup.SetLimit()`.

### TypeScript
- `AbortController` for cancellable async operations.
- Worker threads for CPU-intensive work. Never block event loop.
- Always handle promise rejections.

### Graceful Shutdown (all services)
1. Stop accepting new requests on SIGTERM.
2. Complete in-flight requests within grace period (30s, matching K8s `terminationGracePeriodSeconds`).
3. Drain connections: close idle, wait for active.
4. Flush buffers: logs, traces, metrics.
5. Close resources: DB, queues, file handles.
6. Exit 0 on clean shutdown, non-zero on forced.

---

## 10. TESTING

### Coverage
- 80%+ line coverage on business logic.
- 100% on security-critical paths: auth, authorization, input validation, crypto.

### Structure (Go: table-driven mandatory)
```go
func TestValidateHostname(t *testing.T) {
  tests := []struct {
    name    string
    input   string
    wantErr bool
  }{
    {"valid", "web-01", false},
    {"empty", "", true},
    {"too long", strings.Repeat("a", 64), true},
  }
  for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
      err := ValidateHostname(tt.input)
      if (err != nil) != tt.wantErr {
        t.Errorf("ValidateHostname(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
      }
    })
  }
}
```

### Mandatory Test Cases
- Happy path with valid input.
- Invalid input: missing fields, wrong types, boundary values.
- Edge cases: empty collections, max values, Unicode, concurrent access.
- Auth/authz: unauthenticated, wrong role, expired token, revoked session.
- Error conditions: DB down, timeout, partial failure.
- Idempotency: same request twice = same result.
- Race conditions: `-race` flag in Go CI.

### Banned Test Patterns
- Tests with no assertions.
- Tests depending on execution order.
- Tests using `time.Sleep`. Use fake clocks.
- Tests sharing mutable state between cases.
- Snapshot tests for dynamic data.
- Tests that test implementation details instead of behavior.

---

## 11. DEPENDENCY & SUPPLY CHAIN

- Justify every dependency. If stdlib or <50 lines replaces it, don't add it.
- Verify existence on official registry before importing.
- Maintenance check: updated within 12 months, >1000 weekly downloads, no known CVEs.
- Pin exact versions. No `^` or `~` in production Node. Exact in `go.mod`.
- Audit transitive deps: `npm audit`, `govulncheck`, `pip-audit`.
- Commit lock files: `go.sum`, `package-lock.json`, `composer.lock`.
- Separate dev dependencies from production.
- SBOM generated (CycloneDX 1.5+ or SPDX 3.0+) on every release.
- Artifacts signed via Sigstore/cosign.
- Build provenance: SLSA Level 2+ attestation on production builds.
- Pin CI actions to commit SHAs, not mutable tags.

### Slopsquatting Defense
Before adding ANY AI-suggested package:
1. Search exact name on official registry.
2. Verify >100 stars, >1000 weekly downloads, multiple contributors.
3. Check creation date. Recently created packages after model training cutoff are suspicious.
4. Compare against known legitimate packages for typosquatting.

---

## 12. WEBSOCKET SECURITY

- `wss://` only in production. No `ws://`.
- Validate Origin header against allowlist on upgrade.
- Authenticate on connection via query param or first message, never in URL path.
- Per-IP connection limits: 10 concurrent.
- Idle timeout: 5 minutes.
- Schema-validate every incoming message.
- Max message size: 64KB default.
- Rate limit: 60 messages/min per connection.
- Track connection state server-side. Re-authenticate on token refresh.
- Close code 4001 with reason on auth failure.

---

## 13. CONTAINER & DEPLOYMENT

### Image Hardening
- Minimal base: distroless or Alpine. Never full OS images.
- Multi-stage builds: runtime stage has no compiler, source code, or build tools.
- Non-root execution: UID >= 1000. No `--privileged`. Drop all capabilities.
- Scan images with `trivy` or `grype` in CI. Fail build on CRITICAL/HIGH.

### Kubernetes
- `runAsNonRoot: true`, `readOnlyRootFilesystem: true`.
- `capabilities: drop: ["ALL"]`, `allowPrivilegeEscalation: false`.
- Resource requests AND limits for CPU and memory.
- Network policies: default deny, explicit allow.
- RBAC: minimal service account permissions.
- Secrets encrypted at rest.
- Liveness and readiness probes on every container.
- `seccompProfile: RuntimeDefault`.

### Deployment
- Rolling update: `maxSurge: 1`, `maxUnavailable: 0`.
- Readiness probe passes before receiving traffic.
- DB migrations as separate job BEFORE deployment.
- Feature flags for safe rollback without redeployment.

---

## 14. FEATURE FLAGS

- Every flag has an owner and expiry date. No permanent flags.
- Clean up within 30 days of 100% rollout.
- Server-evaluated only. Never send flag logic to client.
- Rollout stages: 1% -> 10% -> 50% -> 100% with monitoring between each.
- Instant disable without deployment.

---

## 15. API DESIGN

### Versioning
- URL path versioning preferred: `/api/v1/resource`.
- Support N-1 versions minimum.
- Deprecation: announce 6 months before removal. Include `Sunset` and `Deprecation` headers.
- Migration guide required for every breaking change.
- `X-API-Version` header in all responses.
- Additive changes (new fields, endpoints, optional params) are NOT breaking.

### Rate Limiting
- Global: 100 req/min per IP for public endpoints.
- Auth: 5 attempts per 15min for login.
- API: tiered by plan with `X-RateLimit-Limit`, `X-RateLimit-Remaining` headers.
- Sliding window algorithm.
- Stricter limits on expensive operations.

### HTTP Status Codes
| Code | When |
|------|------|
| 200 | Successful read/update |
| 201 | Created |
| 204 | Successful delete |
| 400 | Validation error |
| 401 | Missing/invalid auth |
| 403 | Valid auth, insufficient permissions |
| 404 | Not found |
| 409 | State conflict |
| 422 | Syntactically valid, semantically wrong |
| 429 | Rate limited |
| 500 | Unexpected server failure |
| 503 | Temporarily unavailable |

---

## 16. AI-GENERATED CODE REJECTION PATTERNS

Reject any code matching these patterns:

| Pattern | Detection | Fix |
|---------|-----------|-----|
| Tutorial comments | Comments explain WHAT not WHY | Delete or rewrite to explain WHY |
| God functions | Function > 40 lines | Decompose into focused functions |
| Optimistic data access | `users[0]` without length check | Guard clause before access |
| Empty catch blocks | `catch (e) {}` or bare `recover()` | Handle, log, or propagate |
| Hardcoded config | `"localhost:6379"` | Environment variable via config |
| Phantom imports | Package doesn't exist on registry | Verify or use stdlib |
| SQL interpolation | `fmt.Sprintf("SELECT... %s")` | Parameterized query |
| Catch-all suppression | Generic catch returning null | Specific error handling |
| Missing cleanup | Resource opened without defer/finally | `defer Close()` immediately |
| Over-engineering | Factory -> Impl -> Repo -> RepoImpl for 3 endpoints | Simplify to actual need |
| Copy-paste handlers | 5 handlers with identical auth+validation | Extract to middleware |
| Outdated deps | `dgrijalva/jwt-go`, `pkg/errors`, npm `request` | Use current maintained alternatives |
| Slopsquatted packages | AI-hallucinated package name | Verify on registry first |
| Verbose variable names that add no clarity | `theUserObjectFromDatabase` | `user` |
| Magic numbers | `if retries > 3` | `if retries > MaxRetries` |

---

## 17. ADDITIONAL RULES FOR HIGHEST QUALITY OUTPUT

### Correctness Over Cleverness
- Write the simplest correct solution. Avoid clever one-liners that sacrifice readability.
- Prefer explicit over implicit. If behavior is not obvious from reading the code, it is wrong.
- Every branch in a conditional must be intentional. No "this else should never happen" without a panic/assertion.

### Defensive Boundary Thinking
- Treat every function as a trust boundary. Validate inputs even for internal functions called from validated contexts. Data flows change over time.
- Every type conversion and cast must handle failure. No unchecked casts.
- Integer arithmetic: check for overflow on multiplication, division by zero, negative values where unsigned expected.

### Determinism
- Functions with the same inputs must produce the same outputs (pure functions where possible).
- Avoid implicit ordering dependencies: map iteration order, goroutine scheduling, file system ordering.
- Time-dependent logic uses injected clocks, never `time.Now()` directly in business logic.

### API Contract Discipline
- Response shapes are defined by explicit types/schemas. Never return ad-hoc objects.
- Every field in a response type is intentional. No leaking internal fields to external consumers.
- Pagination responses always include `total`, `hasMore`, and cursor/offset metadata.

### Immutability by Default
- Prefer immutable data structures. Mutate only when performance requires it and document why.
- Go: return new structs instead of mutating pointers unless performance-critical.
- TypeScript: use `readonly`, `as const`, `Readonly<T>`. Prefer `map`/`filter`/`reduce` over mutation.
- Never modify function arguments. Copy first if mutation is needed.

### Fail Fast, Fail Loud
- Validate preconditions at function entry. Return/throw immediately on violation.
- Configuration errors must crash at startup, not at first request.
- Missing required environment variables: panic at initialization with clear message naming the variable.

### Composition Over Inheritance
- Prefer interfaces/composition over class hierarchies.
- Go: embed interfaces in structs. TypeScript: use composition and dependency injection.
- Max inheritance depth: 2 levels. Deeper hierarchies must be refactored to composition.

### Concurrency Correctness
- Shared mutable state must be protected by a mutex OR communicated via channels. Never both for the same data.
- Document the concurrency model of every exported type: "safe for concurrent use" or "not safe, caller must synchronize."
- Context cancellation must propagate to ALL child operations. Never detach context without explicit justification.

### Edge Case Exhaustiveness
- Every `switch`/`select` must have a `default` case that either handles unknown values or panics with the unhandled value.
- Every numeric range must handle: zero, negative, max value, overflow.
- Every string must handle: empty, whitespace-only, max length, Unicode (emoji, RTL, zero-width chars).
- Every collection must handle: empty, single element, max capacity.
- Every time value must handle: past, future, epoch zero, timezone differences, DST transitions.

### Resource Lifecycle
- Every resource that is opened/acquired must have a corresponding close/release in the same function or via ownership transfer to a clearly documented owner.
- Connection pools: set max idle, max open, max lifetime. Monitor pool exhaustion metrics.
- File handles: never hold across async boundaries. Open, use, close within the same scope.

### Code Review Readiness
- Every PR/commit should be reviewable in under 15 minutes. If larger, split into stacked PRs.
- Self-review before submitting: re-read every line as if you are the reviewer.
- No "while I was here" changes. Each commit addresses one concern.

### Backward Compatibility Defaults
- Adding a field to a struct/interface: always optional with zero-value default.
- Removing a field: deprecate first, remove in next major version.
- Changing a field type: never. Add new field, deprecate old.
- Database columns: add as nullable with default. Never drop in same migration as code change.

### Zero-Allocation Hot Paths
- In performance-critical paths: preallocate slices with known capacity, reuse buffers via `sync.Pool`, avoid interface boxing.
- Measure with benchmarks before and after. No premature optimization without profiling data.

### Commit Discipline
- Atomic commits: each commit compiles, passes tests, and is independently deployable.
- Commit message: imperative mood, <72 char subject, body explains WHY not WHAT.
- Never commit generated files, build artifacts, or local configuration.

### Logging Discipline
- Log at the boundary where the error is HANDLED, not where it is propagated. One log per error, not one at every level.
- Structured fields over string interpolation: `slog.Error("failed", "user_id", id, "err", err)` not `log.Printf("failed for user %s: %v", id, err)`.
- Request-scoped logger with correlation ID injected via middleware, passed through context.

### Testing Discipline
- Test behavior, not implementation. Tests should not break when refactoring internals.
- One assertion per test case (or one logical assertion group). Tests that assert 10 things test nothing well.
- Test names describe the scenario: `TestCreateUser_DuplicateEmail_ReturnsConflict`.
- Mock only external boundaries (DB, HTTP, queues). Never mock the unit under test.

### Documentation As Code
- Doc comments on all exported types and functions (Go), all public APIs (TS/PHP).
- Go: doc comment starts with the function/type name.
- TypeScript: JSDoc with `@param`, `@returns`, `@throws`, `@example`.
- OpenAPI 3.1 spec maintained alongside REST APIs.

---

## 18. QUICK REFERENCE

### Default Timeouts
| Resource | Timeout |
|----------|---------|
| HTTP client | 10s |
| DB query OLTP | 5s |
| DB query reporting | 30s |
| gRPC unary | 5s |
| gRPC streaming | 60s |
| Background job step | 5min |
| Graceful shutdown | 30s |
| WebSocket idle | 5min |
| Circuit breaker cooldown | 30s |

### Circuit Breaker Defaults
| Parameter | Value |
|-----------|-------|
| Failure threshold | 5 consecutive |
| Cooldown (half-open) | 30s |
| Success to close | 2 consecutive |

---

## 19. DELIVERY GATE

Before committing, every item must be true:

- [ ] No `TODO`, `FIXME`, placeholders, commented-out code, or debug statements.
- [ ] All 19 quality gates pass: Readable, Secure, Typed, Structured, Validated, DRY, Defensive, Logged, Bounded, Clean, Documented, Configurable, Compatible, Tested, Dependency-Safe, Performant, Provenance-Verified, Observable, Deployment-Safe.
- [ ] Linter passes with zero warnings.
- [ ] All tests pass including race detector.
- [ ] 80%+ coverage on business logic, 100% on security paths.
- [ ] Every import verified on official registry.
- [ ] Lock files committed.
- [ ] `.env.example` updated.
- [ ] Error responses expose zero internals.
- [ ] No secrets in code, logs, or container images.
- [ ] All external calls have timeouts and error handling.
- [ ] Breaking changes have versioning and migration guide.

**Any violation of any rule in this document is a blocking defect. No exceptions.**
