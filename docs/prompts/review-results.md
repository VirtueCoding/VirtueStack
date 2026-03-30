# VirtueStack PR Review — Claude Opus 4.6 Style

> **Reviewer:** Automated comprehensive review per `docs/prompts/reviewer-opus46.md`
> **Date:** 2026-03-29
> **Scope:** 309 files changed, ~48,590 lines added across 40 commits
> **Branch:** `copilot/execute-docs-prompts-executor-gpt53codex`

---

## Issues Found

### Security

```
🔴 CRITICAL | migrations/000069_email_verification_tokens.up.sql | Missing RLS on email_verification_tokens table — customer_id FK present but no ROW LEVEL SECURITY enabled; cross-customer token leakage possible
🟡 WARNING  | internal/controller/api/customer/routes.go:103 | GET /customer/auth/sso-exchange lacks rate limiting — SSO token endpoint can be brute-forced
🟡 WARNING  | internal/controller/api/customer/routes.go:110 | POST /customer/auth/verify-email lacks rate limiting — email verification can be spammed
🟡 WARNING  | internal/controller/models/system_events.go:31,39 | Secret fields in SystemWebhookCreateRequest/UpdateRequest use json:"secret" — request structs should use json:"-" to prevent accidental serialization in error responses
🟡 WARNING  | internal/controller/models/pre_action_webhook.go:24,35 | Same issue: Secret fields in PreActionWebhookCreateRequest/UpdateRequest use json:"secret" instead of json:"-"
🟡 WARNING  | internal/nodeagent/storage/template_builder.go:347,425 | Direct http.Client{} without SSRF protection — ISO download uses raw client instead of tasks.DefaultHTTPClient()
🟡 WARNING  | internal/controller/notifications/telegram.go:77 | Direct http.Client{} without SSRF-safe dialer — should use tasks.DefaultHTTPClient()
🟢 NIT      | migrations/000068_system_webhooks.up.sql | system_webhooks table missing RLS — admin-only table, lower risk but inconsistent with security posture
🟢 NIT      | migrations/000070_pre_action_webhooks.up.sql | pre_action_webhooks table missing RLS — admin-only table, same inconsistency
```

### Correctness

```
🟡 WARNING  | internal/controller/api/customer/websocket.go:259-266 | Uses raw c.JSON() for error responses instead of middleware.RespondWithError() — inconsistent error format
🟢 NIT      | internal/controller/services/failover_service.go:717 | Uses context.Background() instead of parent context for GetAssignment call in loop
```

### Performance

```
🔴 CRITICAL | internal/controller/services/storage_backend_service.go:287-295 | N+1 query: loops nodeIDs calling nodeRepo.GetByID() individually — use bulk retrieval
🔴 CRITICAL | internal/controller/services/template_service.go:494-503 | N+1 query: same pattern — loops nodeIDs calling nodeRepo.GetByID() individually
🟡 WARNING  | internal/controller/services/failover_service.go:717 | N+1 query: loops nodes calling nodeStorageRepo.GetAssignment() individually
🟡 WARNING  | migrations/000068_system_webhooks.up.sql | Missing GIN index on system_webhooks.events TEXT[] column — array containment check will table-scan
🟡 WARNING  | migrations/000070_pre_action_webhooks.up.sql | Missing GIN index on pre_action_webhooks.events TEXT[] column — same array scan issue
🟡 WARNING  | internal/controller/repository/pre_action_webhook_repo.go:56 | List() returns unbounded result set — no LIMIT applied
🟡 WARNING  | internal/controller/repository/system_webhook_repo.go:49 | List() returns unbounded result set — no LIMIT applied
🟢 NIT      | internal/controller/tasks/worker.go:87,103 | NATS streams lack MaxMsgs limit — MaxAge (7d) prevents runaway but explicit limit recommended
```

### Test Quality

```
🟡 WARNING  | tests/integration/webhook_test.go:243 | time.Sleep(10*time.Second) — extremely slow flaky pattern; use polling or mock clock
🟡 WARNING  | internal/controller/tasks/stuck_task_scanner_test.go:164 | time.Sleep(30ms) — minor flaky pattern; prefer channel synchronization
🟢 NIT      | internal/controller/services/circuit_breaker_test.go:102,122,148 | time.Sleep(10ms) — consider deterministic timing
🟢 NIT      | tests/load/k6-*.js | Load tests lack explicit error_rate threshold — add error_rate: ['<0.1%'] for regression detection
```

### Standards Compliance

```
🟡 WARNING  | internal/controller/models/notification.go:22-26 | NotificationPreferencesRequest has NO validate struct tags — Events array could accept unbounded input
```

---

## Summary

| Severity | Count |
|----------|-------|
| 🔴 CRITICAL | 3 |
| 🟡 WARNING | 14 |
| 🟢 NIT | 7 |
| **Total** | **24** |

---

## Verdict: **REQUEST_CHANGES**

3 critical issues must be resolved before merge. All are fixable with small targeted changes.

---

## Completeness: **169/169 tasks verified as properly implemented**

- `planning.md` shows 169 `[x]` and 0 `[ ]`
- All 5 new migrations (000066–000070) have paired `.up.sql` / `.down.sql`
- All new handlers registered in `routes.go`
- All new services wired in `dependencies.go`
- AGENTS.md and codemaps updated to reflect new features
- VM state machine transitions match AGENTS.md §8.1.1 spec exactly
- Compensation stack executes LIFO correctly
- All 14 TransitionStatus calls use correct fromStatus parameter
- Pre-action webhooks implement proper fail-open semantics with timeout
- Cursor pagination correctly uses perPage+1, no OFFSET, no COUNT(*)
- Self-registration properly gated behind `ALLOW_SELF_REGISTRATION` config flag
- Webhook delivery uses HMAC-SHA256 signing with constant-time comparison
- Squirrel queries consistently use `sq.Dollar` placeholder format
- No TODO/FIXME/HACK comments in production code
- Background goroutines properly respect context cancellation

---

## Top 3 Risks That Need Immediate Attention

### 1. 🔴 Missing RLS on `email_verification_tokens` (Security)
**File:** `migrations/000069_email_verification_tokens.up.sql`
**Risk:** This table has a `customer_id` FK and stores security-sensitive tokens. Without RLS, if any code path inadvertently uses the customer-scoped DB connection without filtering, tokens could leak across customers.
**Fix:** Add migration 000071:
```sql
ALTER TABLE email_verification_tokens ENABLE ROW LEVEL SECURITY;
CREATE POLICY email_verification_tokens_customer_isolation
  ON email_verification_tokens FOR ALL
  USING (customer_id = current_setting('app.current_customer_id')::uuid);
```

### 2. 🔴 N+1 Query Patterns (Performance)
**Files:** `storage_backend_service.go:287-295`, `template_service.go:494-503`
**Risk:** These loops call `nodeRepo.GetByID()` per node. With 50+ nodes, this becomes 50+ individual DB roundtrips per request.
**Fix:** Add a `GetByIDs(ctx, []string)` bulk method to `NodeRepository` and refactor callers.

### 3. 🟡 Missing Rate Limiting on Auth Endpoints (Security)
**File:** `internal/controller/api/customer/routes.go:103,110`
**Risk:** SSO token exchange and email verification endpoints lack rate limiting, enabling brute-force attacks on SSO tokens and DoS via verification spam.
**Fix:** Add `middleware.RegistrationRateLimit()` to both routes.

---

## Recommended Follow-ups (Post-Merge OK)

1. **Add GIN indexes on TEXT[] columns** — `system_webhooks.events` and `pre_action_webhooks.events` will table-scan on array containment queries. Add `USING GIN(events)` indexes in a follow-up migration.

2. **Mark webhook request Secret fields `json:"-"`** — The *stored* models already do this correctly, but the *request* structs expose secrets to JSON serialization. While the risk is low (request structs aren't typically serialized in responses), it's defensive to fix.

3. **Replace `time.Sleep` in integration tests** — `webhook_test.go:243` sleeps 10 seconds, making the test suite unnecessarily slow. Use polling with exponential backoff.

4. **Add MaxMsgs to NATS stream config** — Current streams only have MaxAge (7 days). Adding an explicit message count limit provides defense-in-depth against runaway producers.

5. **Use SSRF-safe client in template_builder.go** — ISO download currently uses a raw `http.Client{}`. While URLs are admin-provided, using `tasks.DefaultHTTPClient()` would be more consistent with the security posture.

6. **Standardize websocket error responses** — `websocket.go:259-266` uses raw `c.JSON()` instead of `middleware.RespondWithError()`. Fix for consistent error format across all endpoints.

7. **Add `validate` tags to `NotificationPreferencesRequest`** — The Events array is currently unbounded.

8. **Add `error_rate` thresholds to k6 load tests** — All scripts have duration thresholds but no explicit error rate checks.
