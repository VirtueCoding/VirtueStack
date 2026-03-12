# VirtueStack Codebase Audit — Fix TODO

**Generated**: 2026-03-12
**Scope**: `cmd/`, `internal/`, `modules/`, `webui/`, `proto/`, `migrations/`, `templates/`, `tests/`, `nginx/`, root config files
**Excluded**: `node_modules/`, `vendor/`, `dist/`, `.next/`, generated/binary files

---

## Summary

| Severity | Stubs/TODO | Bugs | Security | Inconsistencies | **Total** |
|----------|-----------|------|----------|-----------------|-----------|
| Critical | 0 | 2 | 4 | 0 | **6** |
| High | 14 | 11 | 4 | 1 | **30** |
| Medium | 2 | 11 | 0 | 15 | **28** |
| Low | 0 | 1 | 0 | 4 | **5** |
| **Total** | **16** | **25** | **8** | **20** | **69** |

> Note: `tests/` directory does not exist in this repository — no test files to audit. `proto/` was audited and contains no findings. `internal/controller/ws/` is an empty directory. Some findings from the initial pass were consolidated where they share the same root cause or location.

---

## 1. Stubs / TODO / Placeholder / Not Implemented

| # | File | Line(s) | Severity | Category | Description | Suggested Fix |
|---|------|---------|----------|----------|-------------|---------------|
| S1 | `internal/controller/services/backup_service.go` | 650–676 | high | stub | [x] 7 stub functions (`ScheduleBackup`, `DeleteSchedule`, `ListSchedules`, `GetSchedule`, `PauseSchedule`, `ResumeSchedule`, `UpdateScheduleFrequency`) all return `"not implemented"`. | Implement backup scheduling logic against the Ceph RBD snapshot scheduler or a cron-based approach. |
| S2 | `internal/controller/services/webhook.go` | 476–501 | high | stub | [x] 4 stub functions (`RetryDelivery`, `GetDeliveryLogs`, `TestWebhook`, `RotateSecret`) return `"not implemented"`. | Implement webhook delivery retry queue, delivery log persistence, test-fire endpoint, and HMAC secret rotation. |
| S3 | `internal/controller/services/failover_service.go` | 323–347 | high | stub | [x] `releaseRBDLocks()` is a stub — returns nil without releasing any locks. Failover will leave stale locks on Ceph RBD images. | Implement `rbd lock remove` calls via the Ceph CLI or go-ceph library. |
| S4 | `migrations/000003` through `000008` | all | high | stub | [x] 6 migration files contain only `SELECT 1;` — bandwidth indexes, IPAM, templates, backups, audit log, webhooks schemas never created. | Write the actual DDL for each migration to create the required tables/indexes. |
| S5 | `webui/customer/app/settings/page.tsx` | 27–75 | high | stub | [x] API keys and webhooks sections use hardcoded mock data arrays instead of fetching from API. | Replace mock arrays with `fetch()` calls to the customer API endpoints. |
| S6 | `webui/customer/components/novnc-console/vnc-console.tsx` | entire file | high | stub | [x] Fake VNC connection — uses `setTimeout` to simulate connection states. Never opens a real WebSocket to noVNC. | Integrate the `@novnc/novnc` library with a real WebSocket proxy URL from the API. |
| S7 | `internal/controller/api/admin/backups.go` | 49 | high | stub | [x] `ListBackups` returns a hardcoded empty JSON array — never queries the database. | Call `backupRepo.List()` with pagination and return real results. |
| S8 | `internal/controller/api/admin/settings.go` | 41, 72 | high | stub | [x] `GetSettings` returns a hardcoded JSON object; `UpdateSetting` acknowledges but doesn't persist. | Add a `settings` table (or KV store) and wire up read/write through the repository layer. |
| S9 | `internal/controller/api/admin/templates.go` | 83–84 | high | stub | [x] `CreateTemplate` responds with the input but never persists to DB. | Call `templateRepo.Create()` and return the persisted entity. |
| S10 | `internal/controller/api/admin/nodes.go` | 169–170 | high | stub | [x] `UpdateNode` responds success but doesn't persist changes. | Call `nodeRepo.Update()` with the validated input. |
| S11 | `internal/controller/api/admin/customers.go` | 173–174 | high | stub | [x] `UpdateCustomer` name update not persisted to DB. | Call `customerRepo.Update()` with validated fields. |
| S12 | `webui/admin/` — dashboard, plans, nodes, customers, audit-logs pages | various | high | stub | [x] 5 admin pages use hardcoded mock data arrays instead of API calls: `dashboard/page.tsx` (37–80), `plans/page.tsx` (40–121), `nodes/page.tsx` (36–133), `customers/page.tsx` (38–103), `audit-logs/page.tsx` (36–169). | Replace mock data with `apiClient.get()` calls to the corresponding admin API endpoints. |
| S13 | `webui/admin/app/login/page.tsx` | 161–168 | high | stub | [x] "Forgot password" link is disabled with a `TODO` comment. | Implement password reset flow: API endpoint + email token + reset form. |
| S14 | `modules/servers/virtuestack/hooks.php` | 869–897 | high | stub | [x] `getVirtueStackTemplates()` and `getVirtueStackLocations()` return hardcoded arrays instead of fetching from the API. | Use `ApiClient` to call the VirtueStack templates and locations endpoints. |
| S15 | `webui/customer/` — multiple pages | various | medium | stub | [x] Multiple features display "Coming Soon" placeholders (firewall rules, two-factor auth setup, advanced network settings). | Implement or remove the placeholder UI elements; add feature flags if partial rollout is intended. |
| S16 | `webui/admin/app/ip-sets/page.tsx` | 317 | medium | stub | [x] `TODO` comment for CSV/JSON import logic — import button present but non-functional. | Implement file upload handler that parses CSV/JSON and calls the IP set bulk-create API. |

---

## 2. Bugs & Logic Errors

| # | File | Line(s) | Severity | Category | Description | Suggested Fix |
|---|------|---------|----------|----------|-------------|---------------|
| B1 | `internal/controller/server.go` | 176, 186 | critical | bug | [x] `nil` passed for the storage backend and nodeAgent client parameters when constructing services. Any call to these services will nil-pointer panic. | Pass the actual initialized `storageBackend` and `nodeAgentClient` instances. |
| B2 | `cmd/controller/main.go` | 90 | critical | bug | [x] `NodeClient` is set to `nil` — the gRPC client to node agents is never connected. All node operations will fail at runtime. | Initialize the gRPC client using `grpc.Dial()` with the configured node agent address. |
| B3 | `internal/controller/services/auth_service.go` | 209 | high | bug | [x] Backup code removal uses `append()` to rebuild slice in-place, but the slice header is not reassigned — the original slice is mutated (may remove wrong element or leave stale entries depending on caller). | Use index-based deletion: `codes = append(codes[:i], codes[i+1:]...)` and reassign to the parent struct. |
| B4 | `internal/controller/services/node_agent_client.go` | 433, 467, 491 | high | bug | [x] `conn.Close()` is called on gRPC connections obtained from the connection pool. Closing pooled connections breaks the pool for subsequent callers. | Return connections to the pool instead of closing them; use a `defer pool.Put(conn)` pattern. |
| B5 | `internal/controller/api/customer/console.go` | 69 | high | bug | [x] Hardcoded `console.virtuestack.io` URL. Will fail in any non-production environment and for any custom domain. | Read the console base URL from configuration/environment variable. |
| B6 | `internal/nodeagent/server.go` | 70-76 | high | bug | [x] Dead code -- checks `ControllerGRPCAddr` but the block is empty or has no effect. The agent never establishes a reverse connection to the controller. | Either implement the controller callback connection or remove the dead check. |
| B7 | `internal/controller/tasks/snapshot_handlers.go` | 119-139 | high | bug | [x] Snapshot update uses a delete-then-recreate pattern (`DeleteSnapshot` + `CreateSnapshot`) instead of an UPDATE query because no `UpdateSnapshot` method exists. Non-atomic: if `CreateSnapshot` fails after `DeleteSnapshot` succeeds, the snapshot record is permanently lost while the RBD snapshot still exists. | Add an `UpdateSnapshot` method to the repository and use it here instead of delete+create. |
| B8 | `internal/controller/notifications/email.go` | 397 | high | bug | [x] `smtp.SendMail` for port 25 extracts the recipient using fragile string splitting: `strings.Split(msg, "\r\nTo: ")[1][:strings.Index(msg, "\r\n")]`. `strings.Index(msg, "\r\n")` searches from the beginning of the full `msg`, not the substring, producing wrong index and likely panic or wrong recipient. | Parse the `To` header properly using `mail.ParseAddress()` or pass the recipient as a parameter instead of re-extracting it from the composed message. |
| B9 | `internal/controller/tasks/webhook_deliver.go` | 145-152 | high | bug | [x] Webhook auto-disable logic logs "webhook auto-disabled due to consecutive failures" but never actually disables the webhook -- `FailCount` is incremented locally but no repo method is called to persist the increment or set `Active = false`. Dead logic. | Call `deps.WebhookRepo.IncrementFailCount(ctx, webhook.ID)` and `deps.WebhookRepo.DisableWebhook(ctx, webhook.ID)` when the threshold is reached. |
| B10 | `internal/controller/models/provisioning_key.go` | 27-36 | high | bug | [x] `IsAllowedIP` only does exact string matching, but `ProvisioningKeyCreateRequest` (line 42) accepts CIDR notation (`validate:"ip\|cidr"`). CIDR entries like `10.0.0.0/24` will never match any individual IP. | Parse CIDR entries with `net.ParseCIDR()` and use `network.Contains(ip)` for range matching. |
| B11 | `webui/customer/lib/auth-context.tsx` | 126 | high | bug | [x] Empty `catch {}` block silently swallows token refresh errors. Users get silently logged out with no error feedback. | Log the error, clear auth state, and redirect to login with an error message. |
| B12 | `modules/servers/virtuestack/virtuestack.php` | 884-889 | high | bug | [x] `ensureCustomer()` returns an empty `customer_id` string when no credentials exist. It never calls the VirtueStack API to create the customer, so all subsequent operations fail silently. | Implement the API call to create the customer and store the returned ID in WHMCS custom fields. |
| B13 | `internal/controller/api/middleware/audit.go` | 84 | high | bug | [x] `c.Errors.Last()` can return `nil` when HTTP status is non-2xx but no Gin error was recorded. Accessing `.Error()` on nil will panic. | Add a nil check: `if lastErr := c.Errors.Last(); lastErr != nil { ... }`. |
| B14 | `internal/controller/repository/notification_repo.go` | 133 | medium | bug | [x] `GetOrCreate` error-checking compares `err.Error()` to a manually reconstructed string -- the comparison will never match, so default notification preferences are never auto-created. | Use `errors.Is()` or check for a specific error type (e.g., `sql.ErrNoRows`) instead of string comparison. |
| B15 | `internal/controller/services/ipmi_client.go` | 108 | medium | bug | [x] `containsPowerOn` uses fragile substring matching on IPMI command output. Different BMC vendors return different strings. | Normalize the output (lowercase + trim) and match against a set of known power-on indicators, or parse the structured IPMI chassis status fields. |
| B16 | `internal/controller/services/rdns_service.go` | 291 | medium | bug | [x] IPv6 reverse DNS zone defaults to `ip6.arpa` without constructing the proper nibble-boundary zone. Will produce incorrect PTR records for most deployments. | Compute the correct reverse zone from the IPv6 prefix (e.g., `/48` → appropriate `ip6.arpa` subdomain). |
| B17 | `internal/controller/services/migration_service.go` | 353 | medium | bug | [x] `CancelMigration` unconditionally assumes the VM was in `running` state before migration began. If the VM was stopped, it will be incorrectly started. | Store the pre-migration VM state and restore to that state on cancellation. |
| B18 | `internal/controller/services/circuit_breaker.go` | entire file | medium | bug | [x] Potential data race — `getEntry()` and `CanAttempt()` read/write shared state without synchronization across goroutines. | Add a `sync.RWMutex` to the circuit breaker entries or use `sync.Map`. |
| B19 | `migrations/000002_bandwidth_tracking.up.sql` | `limit_bytes` column | medium | bug | [x] `limit_bytes` allows NULL, but business logic treats it as required. Inserting a row without `limit_bytes` will cause nil-pointer errors in Go when scanning. | Add `NOT NULL DEFAULT 0` to the column definition. |
| B20 | `internal/controller/api/customer/backups.go` | 56–68 | medium | bug | [x] N+1 query pattern — fetches backup list, then queries each backup's VM individually. Also, pagination `total` count is computed from the page slice length, not the full dataset. | Use a JOIN to fetch backups with VM data in one query; compute total from a `COUNT(*)` query. |
| B21 | `internal/controller/api/customer/snapshots.go` | 57–68 | medium | bug | [x] Same N+1 query pattern as backups — fetches snapshots then queries each VM individually. | Use a JOIN query; fix total count. |
| B22 | `internal/controller/api/customer/metrics.go` | 99 | medium | bug | [x] `vm.BandwidthResetAt` could be nil or zero-value, causing incorrect bandwidth period calculations or panics when used in time arithmetic. | Add a nil/zero check and default to the VM creation date or current billing period start. |
| B23 | `internal/nodeagent/vm/lifecycle.go` | 587 | medium | bug | [x] `getCPUUsage` calls `time.Sleep(100ms)` in the HTTP request path to sample two CPU readings. Blocks the request for 100ms+ per VM. | Move CPU sampling to a background goroutine that updates a cached value periodically; read from cache in the request handler. |
| B24 | `internal/nodeagent/vm/lifecycle.go` | 619 | medium | bug | [x] `getMemoryUsage` always returns 0 for actual memory usage — reads `memory.usage_in_bytes` but the cgroup path is incorrect or the fallback always triggers. | Fix the cgroup v2 path (`memory.current`) and add a fallback to libvirt `virDomainMemoryStats`. |
| B25 | `internal/controller/models/base.go` | 88–96 | low | bug | [x] `parsePositiveInt` does not handle integer overflow — large numeric query strings will silently overflow `int`, potentially producing negative page numbers or offsets. | Add overflow detection (check `result > math.MaxInt/10` before multiplying) or use `strconv.Atoi` with range validation. |

---

## 3. Security Vulnerabilities

| # | File | Line(s) | Severity | Category | Description | Suggested Fix |
|---|------|---------|----------|----------|-------------|---------------|
| V1 | `docker-compose.override.yml` | 16, 47, 48 | critical | security | [x] Hardcoded default passwords: `devpassword` for Postgres, `dev-jwt-secret-min-32-characters-long` for JWT, and a static encryption key. If override file is accidentally used in production, all secrets are public. | Move all secrets to `.env` file (already gitignored); add a startup check that rejects known-bad default values in non-dev environments. |
| V2 | `internal/controller/services/webhook.go` | 493 | critical | security | [x] `VerifySignature` compares HMAC digests with `==` (byte-by-byte string comparison) instead of `hmac.Equal()`. Vulnerable to timing side-channel attacks. | Replace `==` with `hmac.Equal(expectedMAC, actualMAC)`. |
| V3 | `nginx/nginx.conf` | entire file | critical | security | [x] Base nginx config has no HTTPS/TLS configuration, no security headers (`X-Frame-Options`, `X-Content-Type-Options`, `Strict-Transport-Security`), and no rate limiting. | Add TLS termination (or document that a reverse proxy handles it), add security headers, and add `limit_req` zones. |
| V4 | `modules/servers/virtuestack/templates/` | various | critical | security | [x] Template files output PHP/Smarty variables without HTML escaping (e.g., `{$variable}` instead of `{$variable\|escape:'htmlall'}`). Vulnerable to stored XSS if any variable contains user input. | Apply `\|escape:'htmlall'` (Smarty) or `htmlspecialchars()` (PHP) to all user-controlled output. |
| V5 | `nginx/conf.d/default.conf` | 187 | high | security | [x] Content Security Policy includes `unsafe-inline` and `unsafe-eval` for scripts — effectively disables CSP protection against XSS. | Remove `unsafe-inline` and `unsafe-eval`; use nonces or hashes for inline scripts; refactor inline JS to external files. |
| V6 | `templates/email/*.html` | various | high | security | [x] Go `html/template` variables rendered with `{{.Variable}}` — while Go's html/template auto-escapes in HTML context, some variables are injected into `href` attributes or inline styles where auto-escaping is insufficient. | Audit each template variable context; use `{{.Variable \| urlquery}}` for URL contexts; avoid injecting variables into `javascript:` or `style` attributes. |
| V7 | `modules/servers/virtuestack/webhook.php` | 370–374 | high | security | [x] `verifySignature()` returns `true` when the webhook secret is not configured, completely bypassing signature verification. Any attacker can send forged webhook payloads. | Return `false` (reject the request) when no webhook secret is configured. Log a warning. |
| V8 | `internal/controller/tasks/handlers.go` | 155 | high | security | [x] `VMCreatePayload.Password` field stores the plain-text root password in the task payload, which is persisted to NATS JetStream and the database `tasks` table. Anyone with DB or NATS access can read customer VM passwords. | Hash the password before storing it in the task payload, or encrypt the payload field. Only pass the hash to the cloud-init generator. |

---

## 4. Inconsistencies & Code Quality

| # | File | Line(s) | Severity | Category | Description | Suggested Fix |
|---|------|---------|----------|----------|-------------|---------------|
| Q1 | `nginx/conf.d/default.conf` | 26-29, 140-144 | high | inconsistency | [x] Duplicate `/health` location blocks -- nginx will use the last one, making the first block dead code. Confusing and error-prone. | Remove the duplicate block; keep the one with the correct upstream. |
| Q2 | `internal/controller/services/ipam_service.go` | 19 | medium | inconsistency | [x] Constant `IPv6CooldownPeriod` is used for IPv4 cooldown logic — misleading name. | Rename to `IPCooldownPeriod` or create separate constants for IPv4 and IPv6. |
| Q3 | `internal/controller/services/plan_service.go` | 155 | medium | inconsistency | [x] `generatePlanID()` function is defined but never called anywhere. | Remove the dead code or wire it into plan creation. |
| Q4 | `internal/controller/services/template_service.go` | 274 | medium | inconsistency | [x] `generateTemplateID()` function is defined but never called. | Remove the dead code or wire it into template creation. |
| Q5 | `webui/` (admin + customer) | various | medium | inconsistency | [x] 40+ empty `catch {}` blocks across both frontends — errors are silently swallowed with no logging or user feedback. | Add `console.error(err)` at minimum; surface user-facing errors via toast/notification. |
| Q6 | `webui/customer/components/charts/resource-charts.tsx` | 123 | medium | inconsistency | [x] Unsafe type assertion (`as any`) to bypass TypeScript checking. | Define a proper interface for the chart data and use typed props. |
| Q7 | `.golangci.yml` | 23 | medium | inconsistency | [x] `exportloopref` linter is listed but was deprecated in Go 1.22+ (the loop variable capture bug was fixed in the language). | Remove `exportloopref` from the linter list. |
| Q8 | `Dockerfile.controller` | 11 | medium | inconsistency | [x] Default `GO_VERSION=1.25.4` — this Go version does not exist (latest stable as of audit is 1.23.x). Build will fail with default args. Also inconsistent with `go.mod` which declares `go 1.24.0`. | Update to a valid Go version (e.g., `1.23.6`) and align across `go.mod`, Dockerfile, and docker-compose. |
| Q9 | `docker-compose.override.yml` | 36 | medium | inconsistency | [x] `GO_VERSION: "1.25.4"` — same non-existent version, inconsistent with `go.mod` (`go 1.24.0`) and any real Go installation. | Align with `Dockerfile.controller` fix above. |
| Q10 | `docker-compose.yml` | entire file | medium | inconsistency | [x] No resource limits (`mem_limit`, `cpus`) on any service. A single runaway container can starve the host. | Add `deploy.resources.limits` for each service, especially the controller and node-agent. |
| Q11 | `internal/controller/api/customer/power.go` | 194–205 | medium | inconsistency | [x] Hand-rolled `containsString()` reimplements `strings.Contains()` / `slices.Contains()`. | Replace with `slices.Contains()` (Go 1.21+) or `strings.Contains()`. |
| Q12 | `webui/admin/` — plans, nodes, customers pages | various | medium | inconsistency | [x] `getStatusBadgeVariant()` helper is copy-pasted identically across 3+ pages. | Extract to a shared utility module (`lib/status-badge.ts`) and import. |
| Q13 | `internal/controller/tasks/handlers.go` | 350 | medium | inconsistency | [x] Hardcoded nameservers `["8.8.8.8", "8.8.4.4"]` in cloud-init config. Should be configurable per deployment. | Read nameservers from the system configuration or environment variables. |
| Q14 | `internal/controller/tasks/handlers.go` | 384–386 | medium | inconsistency | [x] Hardcoded `CephUser: "virtuestack"` with empty `CephSecretUUID` and `CephMonitors`. Comments say "would be configured per-cluster" but no config mechanism exists. | Add Ceph auth fields to the node or cluster config and pass them through. |
| Q15 | `internal/controller/notifications/telegram.go` | 203–249 | medium | inconsistency | [x] `EscapeMarkdown` and `EscapeMarkdownV2` are identical functions — exact same replacer with the same characters. One should escape only the original Markdown chars, the other should escape the MarkdownV2 set. | Differentiate the two: `EscapeMarkdown` should only escape `_`, `*`, `` ` ``, `[`; `EscapeMarkdownV2` should escape the full MarkdownV2 set per Telegram API docs. |
| Q16 | `internal/controller/api/customer/console.go` | 150–153 | medium | inconsistency | [x] `generateConsoleToken` creates a token by concatenating the VM ID prefix with a UUID fragment (`vmID[:8] + uuid.New().String()[:24]`). While not trivially guessable (UUID component), embedding the VM ID makes the token partially predictable and leaks the VM identity. | Use a full `crypto/rand` 32-byte token with no embedded identifiers. |
| Q17 | `internal/controller/tasks/handlers.go` | 205 | low | inconsistency | [x] `"vm.delete"` is registered as a raw string literal while all other task types use `models.TaskType*` constants (e.g., `models.TaskTypeVMCreate`). | Use `models.TaskTypeVMDelete` constant instead of the string literal. |
| Q18 | `internal/controller/models/plan.go` | 16–17 | low | inconsistency | [x] `PriceMonthly` and `PriceHourly` use `float64` for monetary values. Floating-point representation causes rounding errors in billing calculations. | Use integer cents (e.g., `PriceMonthlyMillicents int64`) or a decimal library (`shopspring/decimal`). |
| Q19 | `internal/shared/config/config.go` | entire file | low | inconsistency | [x] `validatePasswords()` validates the DB password length but does not check `JWT_SECRET` or `ENCRYPTION_KEY` length/strength. | Add validation for JWT secret (min 32 chars) and encryption key (min 32 chars, high entropy). |
| Q20 | `go.mod` | 3, 5 | low | inconsistency | [x] `go 1.24.0` with `toolchain go1.24.13` — both version numbers do not exist as of audit date. Inconsistent with Dockerfile's `GO_VERSION=1.25.4` and `.golangci.yml`. | Align all Go version references to an actual released version. |

---

## Appendix: Additional Notes

### Directories Not Present in Repository
- `tests/` — This directory does not exist in the repository. No test files were found to audit.
- `internal/controller/ws/` — Empty directory, no files to audit.

### Files Audited with No Findings
The following in-scope files were reviewed and found to have no reportable issues:
- `cmd/node-agent/main.go` — Clean entry point, no stubs/bugs/security/inconsistency issues.
- `internal/shared/crypto/crypto.go` — Encryption utilities are correctly implemented.
- `internal/shared/logging/logging.go` — Standard structured logging setup.
- `internal/shared/errors/errors.go` — Error type definitions are clean.
- `internal/shared/proto/` — Generated `.pb.go` files excluded per scope rules.
- `modules/servers/virtuestack/lib/ApiClient.php` — HTTP client implementation is functional.
- `modules/servers/virtuestack/lib/VirtueStackHelper.php` — Helper utilities are functional.
- `docker-compose.prod.yml` — Production compose overrides are clean.
- `Dockerfile.admin-webui` — Standard multi-stage Node.js build, no issues.
- `Dockerfile.customer-webui` — Standard multi-stage Node.js build, no issues.
- `Makefile` — Build targets are correctly defined.
- `.env.example` — Template environment file with placeholder values (not real secrets).
- `.air.toml` — Hot-reload configuration is standard.
- `.dockerignore` — Correctly excludes build artifacts and dependencies.
- `proto/` — Protobuf definitions are clean and well-structured.
- `internal/controller/models/` — Model definitions are generally clean. Issues captured in B10, B25, Q18.

### Files Not Audited (out of scope per exclusion rules)
- `node_modules/`, `vendor/`, `dist/`, `.next/`, `.pnpm-store/`
- `go.sum`, `*.lock` files
- `certs/`, `cloud-init/`, `iso/`, `backups/`
- `.idea/`, `.vscode/`, `.git/`, `.sisyphus/`
- Generated protobuf Go files (`*.pb.go`)

### PHP Module — Deprecated API Usage
The WHMCS module at `modules/servers/virtuestack/hooks.php` (lines 375–384) uses the deprecated `mysql_fetch_assoc()` function. This should be migrated to WHMCS's Capsule ORM (`Illuminate\Database\Capsule\Manager`).

### Project Maturity
Per the project's own README, VirtueStack is ~50–60% complete. Many of the stubs identified above are expected given this stage. The critical and high-severity bugs and security issues should be prioritized for immediate attention regardless of overall completion status.
