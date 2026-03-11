# Phase 6: Integration & Polish — Implementation Plan

**Created:** 2026-03-11
**Status:** Ready for Execution (0/6 major tasks)
**Session:** ses_32379495fffeKMxQd4CWs040cE
**Reference:** `docs/VIRTUESTACK_KICKSTART_V2.md` Sections 7, 17, 18

---

## Phase 6 Overview

Final integration phase bridging billing system to VirtueStack and preparing for production deployment. This phase implements the WHMCS module, notification systems, webhooks, Docker Compose configuration, end-to-end testing, and comprehensive documentation.

**Estimated Duration:** 4 weeks
**Dependencies:** Phase 5 (Web UIs) completion
**Success Criteria:** Production-ready deployment with full test coverage

---

## Task Breakdown

### 6.1: WHMCS Provisioning Module

**Goal:** Create WHMCS module for automated VM lifecycle management
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 7

**Files to Create:**

| File | Description |
|------|-------------|
| `modules/servers/virtuestack/virtuestack.php` | Main WHMCS module entry point |
| `modules/servers/virtuestack/lib/ApiClient.php` | Controller Provisioning API HTTP client |
| `modules/servers/virtuestack/lib/VirtueStackHelper.php` | Shared utility functions |
| `modules/servers/virtuestack/templates/overview.tpl` | Client area overview template |
| `modules/servers/virtuestack/templates/console.tpl` | Console iframe template |
| `modules/servers/virtuestack/hooks.php` | WHMCS hooks for product page customization |
| `modules/servers/virtuestack/logo.png` | Module logo |

**Functions to Implement:**

| Function | Controller API | Async Handling |
|----------|---------------|----------------|
| `virtuestack_CreateAccount` | POST /api/v1/provisioning/vms | HTTP 202 + task_id polling |
| `virtuestack_SuspendAccount` | POST /vms/{id}/suspend | Synchronous |
| `virtuestack_UnsuspendAccount` | POST /vms/{id}/unsuspend | Synchronous |
| `virtuestack_TerminateAccount` | DELETE /vms/{id} | Synchronous |
| `virtuestack_ChangePackage` | PATCH /vms/{id}/resize | Async (task_id) |
| `virtuestack_ChangePassword` | POST /vms/{id}/password | Synchronous |

**Success Criteria:**
- [ ] CreateAccount provisions VM via Controller API
- [ ] Customer credentials encrypted and stored securely
- [ ] Client area displays Customer WebUI in iframe
- [ ] SSO token authentication works correctly
- [ ] All WHMCS lifecycle functions operational

**Dependencies:** Phase 5 completion (Customer API + WebUI)
**Estimated Effort:** 5 days
**Atomic Commit:** `feat(whmcs): add provisioning module with SSO integration`

---

### 6.2: Notification System

**Goal:** Implement email and Telegram notifications for VM events
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 18

**Files to Create:**

| File | Description |
|------|-------------|
| `internal/controller/services/notification.go` | Notification orchestration service |
| `internal/controller/notifications/email.go` | SMTP email provider |
| `internal/controller/notifications/telegram.go` | Telegram Bot API provider |
| `internal/controller/api/customer/notifications.go` | Customer preference API |
| `templates/email/` | Email templates directory |
| `migrations/000009_notification_preferences.up.sql` | DB migration |

**Event Types & Recipients:**

| Event | Admin | Customer | Webhook |
|-------|-------|----------|---------|
| VM created | ✓ | ✓ | ✓ |
| VM deleted | ✓ | ✓ | ✓ |
| VM suspended | ✓ | ✓ | ✓ |
| Backup failed | ✓ | ✓ | ✓ |
| Node offline | ✓ (urgent) | — | — |
| Bandwidth cap exceeded | — | ✓ | ✓ |

**Success Criteria:**
- [ ] Email notifications sent for all event types
- [ ] Telegram alerts delivered to admin chat
- [ ] Customer preferences stored and respected
- [ ] Templates render correctly (HTML + text)

**Dependencies:** 6.1 (WHMCS module for customer context)
**Estimated Effort:** 4 days
**Atomic Commit:** `feat(notifications): add email and telegram providers`

---

### 6.3: Webhook Delivery System

**Goal:** Reliable webhook delivery with retry logic and HMAC signatures
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 18

**Files to Create:**

| File | Description |
|------|-------------|
| `internal/controller/services/webhook.go` | Webhook orchestration service |
| `internal/controller/tasks/webhook_deliver.go` | Delivery task handler |
| `internal/controller/repository/webhook_repo.go` | Webhook repository |
| `internal/controller/api/customer/webhooks.go` | Customer webhook management API |

**Retry Schedule:**
| Attempt | Delay |
|---------|-------|
| 1st retry | 10 seconds |
| 2nd retry | 60 seconds |
| 3rd retry | 5 minutes |
| 4th retry | 30 minutes |
| 5th retry | 2 hours (final) |

**Success Criteria:**
- [ ] Webhooks delivered with HMAC signature
- [ ] Retry schedule followed correctly
- [ ] Delivery history tracked in database
- [ ] Auto-disable after 50 consecutive failures
- [ ] Customer CRUD operations functional

**Dependencies:** 6.2 (Notification system for event triggers)
**Estimated Effort:** 4 days
**Atomic Commit:** `feat(webhooks): implement delivery system with retry logic`

---

### 6.4: Docker Compose Production Config

**Goal:** Production-ready Docker Compose with all services
**Reference:** Infrastructure requirements from kickstart

**Files to Create:**

| File | Description |
|------|-------------|
| `docker-compose.yml` | Main production compose file |
| `docker-compose.override.yml` | Development overrides |
| `docker-compose.prod.yml` | Production-specific settings |
| `nginx/nginx.conf` | Main nginx configuration |
| `nginx/conf.d/default.conf` | Site configuration |
| `Dockerfile.controller` | Multi-stage Controller build |
| `Dockerfile.admin-webui` | Admin WebUI production build |
| `Dockerfile.customer-webui` | Customer WebUI production build |

**Services:** postgres, nats, controller, admin-webui, customer-webui, nginx

**Success Criteria:**
- [ ] `docker-compose up` starts all services successfully
- [ ] TLS 1.3 enforced on all HTTPS endpoints
- [ ] Rate limiting active and functional
- [ ] WebSocket proxying works for console access
- [ ] Images optimized for size (multi-stage builds)

**Dependencies:** 6.3 (All backend components complete)
**Estimated Effort:** 3 days
**Atomic Commit:** `feat(docker): production docker-compose with TLS and rate limiting`

---

### 6.5: End-to-End Testing

**Goal:** Comprehensive E2E test suite for critical user paths
**Reference:** Quality Gates from `MASTER_CODING_STANDARD_V2.md`

**Files to Create:**

| File | Description |
|------|-------------|
| `tests/integration/suite_test.go` | Go test suite setup |
| `tests/integration/vm_lifecycle_test.go` | Full VM lifecycle tests |
| `tests/integration/auth_test.go` | Authentication flow tests |
| `tests/e2e/playwright.config.ts` | Playwright configuration |
| `tests/e2e/auth.spec.ts` | Authentication E2E tests |
| `tests/e2e/customer-vm.spec.ts` | Customer VM management tests |
| `tests/e2e/admin-node.spec.ts` | Admin node management tests |
| `tests/security/owasp_zap.sh` | OWASP ZAP baseline scan |
| `tests/load/k6-vm-operations.js` | k6 load tests |

**Success Criteria:**
- [ ] All Go integration tests pass (`go test ./...`)
- [ ] All Playwright tests pass (`npx playwright test`)
- [ ] OWASP ZAP scan shows no high/critical issues
- [ ] Load tests handle 200 concurrent users
- [ ] Test coverage ≥ 80%

**Dependencies:** 6.4 (Working Docker Compose deployment)
**Estimated Effort:** 5 days
**Atomic Commit:** `test(e2e): comprehensive test suite with Playwright and k6`

---

### 6.6: Documentation

**Goal:** Complete setup, usage, and API documentation
**Reference:** Documentation requirements from kickstart

**Files to Create:**

| File | Description |
|------|-------------|
| `docs/INSTALL.md` | Complete installation guide |
| `docs/USAGE.md` | Feature usage walkthrough |
| `docs/API.md` | API reference documentation |
| `docs/ARCHITECTURE.md` | System architecture overview |
| `docs/TROUBLESHOOTING.md` | Common issues and solutions |

**Success Criteria:**
- [ ] INSTALL.md is complete and tested
- [ ] USAGE.md covers all major features
- [ ] API.md is accurate with all endpoints
- [ ] Documentation reviewed for clarity
- [ ] Fresh install possible from INSTALL.md alone

**Dependencies:** All previous tasks complete
**Estimated Effort:** 3 days
**Atomic Commit:** `docs: complete installation, usage, and API documentation`

---

## Dependency Graph

```
6.1 (WHMCS) ────────────────────────┐
                                     │
6.2 (Notifications) ───┬────────────┼──────┐
                       │            │      │
6.3 (Webhooks) ◄───────┘            │      │
                                    │      │
6.4 (Docker) ◄──────────────────────┘      │
                                           │
6.5 (E2E Tests) ◄──────────────────────────┘
                                           │
6.6 (Documentation) ◄──────────────────────┘
```

## Execution Timeline

| Week | Tasks | Deliverables |
|------|-------|--------------|
| Week 1 | 6.1, 6.2 start | WHMCS module, notification core |
| Week 2 | 6.2 finish, 6.3 | Email/Telegram, webhook system |
| Week 3 | 6.4, 6.5 start | Docker Compose, E2E tests |
| Week 4 | 6.5 finish, 6.6 | Security tests, documentation |

## Next Steps

1. Create todo list for Phase 6 tasks
2. Begin with Task 6.1: WHMCS Provisioning Module
3. Execute in dependency order
4. Verify each task before proceeding
