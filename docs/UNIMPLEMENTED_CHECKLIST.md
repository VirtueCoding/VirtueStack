# VirtueStack Unimplemented Features Checklist

**Generated:** 2026-03-13
**Source:** Codebase scan (not docs)

---

## Priority 0 — Broken / Non-functional

| # | Feature | Files | Issue |
|---|---------|-------|-------|
| 1 | **Console access (VNC)** | `internal/controller/api/customer/console.go` | Token endpoint generates placeholder URLs with comments "In production, this would…". No WebSocket proxy exists in the controller. |
| 2 | **Console access (VNC UI)** | `webui/customer/components/novnc-console/vnc-console.tsx` | Paints a fake "VNC Connected" gradient canvas. Does not use the noVNC library. |
| 3 | **Serial console (UI)** | `webui/customer/components/serial-console/serial-console.tsx` | Entirely local mock (`mockBootSequence`). Not connected to any backend. Not rendered on the VM detail page. |
| 4 | **Customer settings — API Keys UI** | `webui/customer/app/settings/page.tsx`, `webui/customer/lib/api-client.ts` | Frontend `ApiKey` type expects `key`, `created`, `lastUsed`. Backend returns `created_at`, `last_used_at`, `is_active`, `permissions`, `expires_at`. Create/Regenerate buttons have no `onClick` handlers. |
| 5 | **Customer settings — Webhooks UI** | `webui/customer/app/settings/page.tsx`, `webui/customer/lib/api-client.ts` | Frontend `Webhook` type expects `status`, `lastTriggered`. Backend returns `is_active`, `fail_count`, `last_success_at`, `last_failure_at`. Add/Test/Edit buttons have no handlers. |

---

## Priority 1 — Missing Customer-Facing Functionality

| # | Feature | Files | Issue |
|---|---------|-------|-------|
| 6 | **Customer profile update** | `webui/customer/app/settings/page.tsx` | "Save Changes" button has no handler. No customer API route exists for profile update (`PUT /customer/profile`). |
| 7 | **Customer password change** | `webui/customer/app/settings/page.tsx` | "Update Password" button has no handler. Backend `AuthService.ChangePassword()` exists but no API handler exposes it to the customer portal. |
| 8 | **Customer 2FA setup** | `webui/customer/app/settings/page.tsx` | 2FA toggle is local React state only. QR code is a placeholder icon. No API calls to generate TOTP secret or enable 2FA. Backend TOTP verification exists for login but setup/enable flow has no API handler. |
| 9 | **Customer backup codes** | `webui/customer/app/settings/page.tsx` | Shows "Backup codes will be available after enabling 2FA" placeholder. Backend generates and verifies backup codes during auth, but no endpoint exists to display or regenerate them. |
| 10 | **Settings API client** | `webui/customer/lib/api-client.ts` | `settingsApi` only has `getApiKeys()` and `getWebhooks()`. Missing: `createApiKey`, `rotateApiKey`, `deleteApiKey`, `createWebhook`, `updateWebhook`, `deleteWebhook`, `testWebhook`. Backend has all these endpoints. |

---

## Priority 2 — Incomplete / Fallback Behavior

| # | Feature | Files | Issue |
|---|---------|-------|-------|
| 11 | **Resource charts mock fallback** | `webui/customer/components/charts/resource-charts.tsx` | Uses `generateMockData()` initially; silently keeps fake data if metrics fetch fails. Should show error state or loading indicator instead. |
| 12 | **Admin dashboard active alerts** | `webui/admin/app/dashboard/page.tsx` | `activeAlerts: 0 // Mock for now`. No alerts API or page exists. "View All Alerts" button is dead. |
| 13 | **`httpClient` field on WebhookService** | `internal/controller/services/webhook.go` | `Register()` method references `s.httpClient` but `WebhookService` struct has no `httpClient` field. Will fail at compile time. |

---

## Priority 3 — Backend / Ops Gaps

| # | Feature | Files | Issue |
|---|---------|-------|-------|
| 14 | **Automatic node failover** | `internal/controller/services/node_service.go`, `failover_service.go` | Manual admin failover exists (`POST /nodes/:id/failover`). No background heartbeat monitor auto-triggers failover when `consecutive_heartbeat_misses` threshold is exceeded. `UpdateHeartbeatMisses()` exists in the repo but nothing calls it periodically. |
| 15 | **Backup schedule management API** | `internal/controller/services/backup_service.go` | `StartScheduler()`, `CreateBackupSchedule()`, `ListBackupSchedules()`, `DeleteBackupSchedule()` all exist in the service layer. No API routes or admin UI expose schedule CRUD. |
| 16 | **WebSocket console proxy** | (missing) | Architecture docs describe `Controller as WebSocket proxy`, but no websocket proxy code exists in `internal/controller`. Needed to bridge browser WebSocket to node agent gRPC `StreamVNCConsole`/`StreamSerialConsole`. |

---

## Priority 4 — Stale Comments / Cleanup

| # | Item | File | Note |
|---|------|------|------|
| 17 | Stale comment "placeholder registration" | `internal/nodeagent/server.go:100` | Comment says "placeholder registration" but `registerServices()` properly calls `RegisterNodeAgentServiceServer`. Just a stale comment. |
| 18 | Stale comment "simulate response structure" | `internal/controller/services/node_agent_client.go:113` | Comment says "simulate" but code actually calls `GetNodeHealth` via gRPC. Stale wording. |
| 19 | README/USAGE warnings are outdated | `README.md`, `docs/USAGE.md` | Several warnings say features are "stubbed" or "TODO" that have since been implemented. Should be updated. |

---

## Summary

| Priority | Count | Impact |
|----------|-------|--------|
| P0 — Broken | 5 | Crashes or renders fake data on real UI surfaces |
| P1 — Missing | 5 | Customer-facing features with UI but no backend wiring |
| P2 — Fallback | 3 | Silent degradation to mock/hardcoded data |
| P3 — Ops gaps | 3 | Missing operational automation / API surfaces |
| P4 — Cleanup | 3 | Stale comments and docs |
| **Total** | **19** | |
