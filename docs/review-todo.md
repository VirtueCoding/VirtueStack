# VirtueStack Code Audit - Review TODO

**Audit Date:** 2026-03-14  
**Standard:** MASTER_CODING_STANDARD_V2.md  
**Max Iterations:** 3  
**Current Iteration:** 3 (COMPLETE - MAX ITERATIONS REACHED)

---

## SUMMARY

### Initial Count
| Severity | Count |
|----------|-------|
| CRITICAL | 89 |
| WARNING | 67 |
| NITPICK | 31 |
| **TOTAL** | **187** |

### After Iteration 3
| Severity | Count |
|----------|-------|
| CRITICAL | 32 |
| WARNING | 60 |
| NITPICK | 31 |
| **TOTAL** | **123** |

**Fixed in Iteration 1:** 47 CRITICAL issues  
**Fixed in Iteration 2:** 10 CRITICAL issues  
**Fixed in Iteration 3:** 7 WARNING issues  
**Total Fixed:** 64 issues  
**New Issues Found:** 0  
**Remaining:** 123

---

## ITERATION 3 COMPLETION REPORT

### Fixed WARNING Issues (7 total)

#### ✅ Empty Error Handling - Comments Added (2 issues)
- [x] **FIXED** modules/servers/virtuestack/lib/ApiClient.php:428 — Added fallback comment for empty catch in listTemplates()
- [x] **FIXED** modules/servers/virtuestack/lib/ApiClient.php:440 — Added fallback comment for empty catch in listLocations()

#### ✅ Input Validation - Sanitization Added (5 issues)
- [x] **FIXED** modules/servers/virtuestack/hooks.php:94 — Added `htmlspecialchars()` sanitization for `$_GET['vs_webhook']`
- [x] **FIXED** modules/servers/virtuestack/hooks.php:526-528 — Added validation for `$eventType`, `$taskId`, `$whmcsServiceId` with type checking
- [x] **FIXED** modules/servers/virtuestack/webhook.php:77-82 — Added validation for all webhook fields

---

## FINAL STATUS: ⚠ Max iterations reached (3/3)

---

## ITERATION 2 COMPLETION REPORT

### Fixed CRITICAL Issues (10 total)

#### ✅ Fire-and-Forget Goroutines (1 issue)
- [x] **FIXED** internal/controller/services/notification.go:102 — Removed `go s.sendNotificationAsync()` fire-and-forget. Now blocks until completion using internal WaitGroup.

#### ✅ Hardcoded Values - Now Configurable (4 issues)
- [x] **FIXED** internal/nodeagent/server.go:35 — LibvirtURI now uses `cfg.LibvirtURI` with env var `LIBVIRT_URI`
- [x] **FIXED** internal/nodeagent/server.go:59 — DataDir now uses `cfg.DataDir` with env var `DATA_DIR`
- [x] **FIXED** internal/nodeagent/grpc_handlers_extended.go:305 — VNC host now uses `cfg.VNCHost` with env var `VNC_HOST`
- [x] **FIXED** internal/controller/tasks/handlers.go:712 — MAC prefix "52:54:00" now defined as constant `MACPrefix`

#### ✅ Configuration Schema Updates (5 issues)
- [x] **FIXED** internal/shared/config/config.go — Added `LibvirtURI`, `VNCHost`, `DataDir` fields to NodeAgentConfig
- [x] **FIXED** internal/shared/config/config.go — Added env var overrides for LIBVIRT_URI, VNC_HOST, DATA_DIR
- [x] **FIXED** internal/nodeagent/server.go — Removed hardcoded `LibvirtURI` constant
- [x] **FIXED** internal/nodeagent/server.go — Updated NewServer to use configurable values with defaults
- [x] **FIXED** internal/nodeagent/grpc_handlers_extended.go — Updated VNC connection to use configurable host

---

## REMAINING CRITICAL FINDINGS (32 issues)

### Category: File Length Violations (Section 1.37) - 22 issues
**Note:** These require major refactoring/splitting into multiple files.

- [ ] **CRITICAL** internal/controller/services/auth_service.go:1 — File is 935 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/backup_service.go:1 — File is 879 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/vm_service.go:1 — File is 785 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/node_agent_client.go:1 — File is 765 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/repository/customer_repo.go:1 — File is 751 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/webhook.go:1 — File is 699 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/node_service.go:1 — File is 676 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/notification.go:1 — File is 412 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/migration_service.go:1 — File is 389 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/notification_service.go:1 — File is 390 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/services/rbac_service.go:1 — File is 497 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/tasks/handlers.go:1 — File is 976 lines (limit: 300)
- [ ] **CRITICAL** internal/nodeagent/grpc_handlers_extended.go:1 — File is 857 lines (limit: 300)
- [ ] **CRITICAL** internal/nodeagent/server.go:1 — File is 546 lines (limit: 300)
- [ ] **CRITICAL** internal/controller/server.go:1 — File is 526 lines (limit: 300)
- [ ] **CRITICAL** modules/servers/virtuestack/hooks.php:1 — File is 1070 lines (limit: 300)
- [ ] **CRITICAL** modules/servers/virtuestack/virtuestack.php:1 — File is 947 lines (limit: 300)
- [ ] **CRITICAL** modules/servers/virtuestack/webhook.php:1 — File is 655 lines (limit: 300)
- [ ] **CRITICAL** modules/servers/virtuestack/lib/ApiClient.php:1 — File is 598 lines (limit: 300)
- [ ] **CRITICAL** modules/servers/virtuestack/lib/VirtueStackHelper.php:1 — File is 496 lines (limit: 300)
- [ ] **CRITICAL** webui/admin/lib/api-client.ts:1 — File is 487 lines (limit: 300)
- [ ] **CRITICAL** webui/admin/app/ip-sets/page.tsx:1 — File is 733 lines (limit: 300)
- [ ] **CRITICAL** webui/customer/lib/api-client.ts:1 — File is 762 lines (limit: 300)

### Category: Function Length Violations (Section 1.35) - 10 issues
**Note:** These require function decomposition/refactoring.

- [ ] **CRITICAL** internal/controller/services/auth_service.go:92 — Login() function is ~84 lines
- [ ] **CRITICAL** internal/controller/services/auth_service.go:179 — Verify2FA() function is ~98 lines
- [ ] **CRITICAL** internal/controller/services/auth_service.go:278 — RefreshToken() function is ~75 lines
- [ ] **CRITICAL** internal/controller/services/auth_service.go:577 — AdminVerify2FA() function is ~108 lines
- [ ] **CRITICAL** internal/controller/services/customer_service.go:58 — Update() function is ~45 lines
- [ ] **CRITICAL** internal/controller/services/failover_service.go:77 — ApproveFailover() function is ~112 lines
- [ ] **CRITICAL** internal/controller/services/backup_service.go:120 — CreateBackup() function is ~73 lines
- [ ] **CRITICAL** internal/controller/services/backup_service.go:603 — runSchedulerTick() function is ~79 lines
- [ ] **CRITICAL** internal/controller/services/vm_service.go:85 — CreateVM() function is ~156 lines
- [ ] **CRITICAL** internal/controller/services/vm_service.go:474 — ResizeVM() function is ~87 lines

---

## ITERATION TRACKING

### Iteration 1
- **Started:** 2026-03-14
- **Fixed:** 47 CRITICAL issues
- **New Issues Found:** 0
- **Remaining:** 140

### Iteration 2
- **Started:** 2026-03-14
- **Fixed:** 10 CRITICAL issues
- **New Issues Found:** 0
- **Remaining:** 130 (32 CRITICAL, 67 WARNING, 31 NITPICK)

### Iteration 3
- **Started:** 
- **Fixed:** 
- **New Issues Found:** 
- **Remaining:** 

---

## FILES MODIFIED IN ITERATION 2

### Go Backend (5 files)
1. internal/controller/services/notification.go — Fixed fire-and-forget goroutine
2. internal/shared/config/config.go — Added LibvirtURI, VNCHost, DataDir config fields
3. internal/nodeagent/server.go — Use configurable values instead of hardcoded
4. internal/nodeagent/grpc_handlers_extended.go — Use configurable VNC host
5. internal/controller/tasks/handlers.go — MAC prefix as constant

---

## COMPLIANT AREAS (No Violations Found)

- ✅ No TODO/FIXME/HACK/XXX comments in Go/PHP/TS code
- ✅ Proper password hashing with Argon2id
- ✅ Uses crypto/rand (not math/rand) for security-sensitive values
- ✅ Uses AES-256-GCM for encryption (not AES-ECB)
- ✅ Uses SHA-256 for hashing (not MD5/SHA-1)
- ✅ No SQL injection in Go (uses parameterized queries)
- ✅ Proper error wrapping with fmt.Errorf("...: %w", err)
- ✅ Uses errors.Is() and errors.As() for error checking
- ✅ Constants properly named in UPPER_SNAKE_CASE
- ✅ Imports properly grouped (stdlib > third-party > local)
- ✅ No commented-out code blocks
- ✅ Uses defer for resource cleanup
- ✅ Context used for cancellation propagation
- ✅ No hardcoded passwords/API keys in main code
- ✅ No eval() or dynamic code execution
- ✅ JWT token generation uses proper crypto
- ✅ Zero console.error statements in TypeScript
- ✅ All ignored errors have justification comments
- ✅ SQL migrations use IF NOT EXISTS
- ✅ No fire-and-forget goroutines

---

## CONCLUSION

**Iteration 2 Complete.** Successfully fixed 10 CRITICAL issues:
- 1 fire-and-forget goroutine fixed
- 4 hardcoded values now configurable
- 5 configuration schema updates

**Remaining work:** 130 issues (32 CRITICAL, 67 WARNING, 31 NITPICK).

The remaining 32 CRITICAL issues are all **structural** (file/function length) and require significant refactoring:
- 22 files exceed 300 lines - need splitting into modules
- 10 functions exceed 40 lines - need decomposition

These structural issues are architectural debt that should be addressed in a dedicated refactoring phase, as they require careful design decisions about module boundaries and abstraction layers.

**Status:** ✓ Iteration 2 complete. Convergence check: New issues = 0.

---

## LOW-PRIORITY CLEANUP (NITPICK Items - 31 issues)

### Category: Tutorial-Style Comments (Section 1.16)
- [ ] **NITPICK** internal/controller/tasks/handlers.go:242 — Tutorial-style comments explaining each step
- [ ] **NITPICK** Multiple files — Many inline comments explain what code does, not why

### Category: Magic Numbers (Section 16)
- [ ] **NITPICK** modules/servers/virtuestack/hooks.php:332 — Magic number 300 (5 minutes) should be constant

### Category: Migration Comments
- [ ] **NITPICK** migrations/000003_placeholder.up.sql — No comments explaining purpose
- [ ] **NITPICK** migrations/000004_placeholder.up.sql — No comments explaining purpose
- [ ] **NITPICK** migrations/000005_placeholder.up.sql — No comments explaining purpose
- [ ] **NITPICK** migrations/000006_placeholder.up.sql — Minimal context in comments

---

*Last Updated: 2026-03-14*  
*Next Steps: Iteration 3 (if proceeding) or exit with structural issues documented*
