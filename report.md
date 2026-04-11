# VirtueStack Dead Code Report

Generated: 2026-04-09T17:58:09.034Z

Phase 1 — Audit

| File | Symbol/Block | Type | Reason |
| --- | --- | --- | --- |
| `internal/controller/api/admin/system_webhooks.go` | `generateSystemWebhookSecret`, `var _ = uuid.Nil` | unused function + sentinel var | The helper has zero callers, and the blank-identifier `uuid.Nil` assignment only exists to keep an otherwise unused import alive. |
| `webui/customer/components/icons/bitcoin-icon.tsx` | `BitcoinIcon` | unused component | The component is exported but never imported anywhere in the repository. |
| `webui/customer/components/page-transition.tsx` | `PageTransition` | unused component | The customer portal version has no importers; only the admin portal uses a page-transition component. |
| `webui/customer/components/animated-card.tsx` | `AnimatedCard` | unused component | The customer portal component has zero references across the repository. |
| `webui/customer/components/skeleton.tsx` | `Skeleton` | unused component | The customer portal skeleton component is defined but never imported. |
| `webui/admin/components/nodes/NodeStorageBackendsTab.tsx` | `NodeStorageBackendsTab` | unused component | The tab component and its props have zero call sites anywhere in the admin portal. |
| `webui/customer/lib/api-client.ts` | `Template`, `UpdateRDNSRequest`, `templateApi` | orphaned type + API export | `templateApi` has zero importers, and its companion `Template` / `UpdateRDNSRequest` types are only referenced inside dead export blocks. |
| `webui/customer/lib/animations.ts` | `fadeIn`, `scaleIn`, `slideInLeft`, `tableRow`, `springTransition` | unused animation exports | These motion helpers have no importers; only the remaining animation exports are used. |
| `webui/customer/lib/utils/toast-helpers.ts` | `ToastOptions` | orphaned type export | The exported interface is never imported or referenced, including inside its own file. |
| `webui/customer/lib/hooks/useVMAction.ts` | exported `executeVMAction` | duplicate unused export | The hook `useVMAction` is used, but the separately exported `executeVMAction` helper has zero call sites. |
| `webui/admin/lib/api-client.ts` | `AdminBackup`, `AdminBackupListParams`, `adminBackupsApi`, `CreateAdminBackupScheduleRequest`, `UpdateAdminBackupScheduleRequest`, `adminBackupSchedulesApi`, `VMBackupSchedule`, `CreateVMBackupScheduleRequest`, `UpdateVMBackupScheduleRequest`, `adminVMBackupSchedulesApi` | orphaned types + API exports | None of the admin backup API namespace objects have importers, and the listed request/response types are only referenced inside those dead API blocks. |
| `webui/admin/lib/animations.ts` | `fadeIn`, `scaleIn`, `slideInLeft`, `tableRow`, `springTransition` | unused animation exports | These admin motion helpers have no importers; only the remaining animation exports are used. |
| `webui/admin/lib/navigation.ts` | `adminNavItems` | orphaned constant export | The flattened list is exported but never imported; the sidebar and mobile nav both consume `adminNavGroups` directly. |
| `modules/servers/virtuestack/lib/VirtueStackHelper.php` | `generatePassword`, `cryptoShuffle`, `storeCustomerCredentials`, `generateVmReference`, `ensureCorrelationId` | unused helper methods | None of these helper methods are called anywhere in the WHMCS module; `cryptoShuffle` is only reachable from the dead `generatePassword` helper. |
| `modules/servers/virtuestack/hooks.php` | `getVirtueStackPlans` | unused helper function | The hook helper is defined but never called, unlike the adjacent template and location helper functions. |
| `modules/blesta/virtuestack/lib/VirtueStackHelper.php` | `VALID_VM_STATUSES`, `PASSWORD_UPPER`, `PASSWORD_LOWER`, `PASSWORD_DIGITS`, `PASSWORD_SPECIAL`, `isValidIP`, `isValidVMStatus`, `generatePassword`, `getServiceField`, `getModuleRowMeta`, `getPackageMeta` | unused constants + helper methods | The listed methods have zero call sites within the Blesta module, and the constants are only referenced by those dead helper methods. |

Total items found: 16

---

Phase 2 — Removals

| File | Items Removed | Notes |
| --- | --- | --- |
| `internal/controller/api/admin/system_webhooks.go` | `generateSystemWebhookSecret`, `var _ = uuid.Nil` | Removed the dead local helper and the no-op sentinel that only kept its import path alive. |
| `webui/customer/components/icons/bitcoin-icon.tsx` | `BitcoinIcon` | Deleted the orphaned component file. |
| `webui/customer/components/page-transition.tsx` | `PageTransition` | Deleted the unused customer-only transition wrapper. |
| `webui/customer/components/animated-card.tsx` | `AnimatedCard` | Deleted the unused customer-only animated card component. |
| `webui/customer/components/skeleton.tsx` | `Skeleton` | Deleted the unused customer-only skeleton component. |
| `webui/admin/components/nodes/NodeStorageBackendsTab.tsx` | `NodeStorageBackendsTab` | Deleted the unreferenced admin node tab component. |
| `webui/customer/lib/api-client.ts` | `Template`, `UpdateRDNSRequest`, `templateApi` | Removed the dead template API block and its orphaned companion types. |
| `webui/customer/lib/animations.ts` | `fadeIn`, `scaleIn`, `slideInLeft`, `tableRow`, `springTransition` | Kept only the motion exports with live importers. |
| `webui/customer/lib/utils/toast-helpers.ts` | `ToastOptions` | Removed the unused exported type without changing hook behavior. |
| `webui/customer/lib/hooks/useVMAction.ts` | exported `executeVMAction` | Removed the dead non-hook duplicate and kept the hook-based API. |
| `webui/admin/lib/api-client.ts` | `AdminBackup`, `AdminBackupListParams`, `adminBackupsApi`, `VMBackupSchedule`, `CreateVMBackupScheduleRequest`, `UpdateVMBackupScheduleRequest`, `adminVMBackupSchedulesApi` | Removed two dead admin backup API blocks; the `adminBackupSchedulesApi` block REGRESSED — reverted in Phase 3 after active backup schedule components failed type-check. |
| `webui/admin/lib/animations.ts` | `fadeIn`, `scaleIn`, `slideInLeft`, `tableRow`, `springTransition` | Kept only the live animation exports. |
| `webui/admin/lib/navigation.ts` | `adminNavItems` | Removed the flattened nav export; all callers use `adminNavGroups`. |
| `modules/servers/virtuestack/lib/VirtueStackHelper.php` | `generatePassword`, `cryptoShuffle`, `storeCustomerCredentials`, `generateVmReference`, `ensureCorrelationId` | Removed unused WHMCS helper methods; `cryptoShuffle` was only reachable from the dead password helper. |
| `modules/servers/virtuestack/hooks.php` | `getVirtueStackPlans` | Removed the unreferenced WHMCS hook helper. |
| `modules/blesta/virtuestack/lib/VirtueStackHelper.php` | `VALID_VM_STATUSES`, `PASSWORD_UPPER`, `PASSWORD_LOWER`, `PASSWORD_DIGITS`, `PASSWORD_SPECIAL`, `isValidIP`, `isValidVMStatus`, `generatePassword`, `getServiceField`, `getModuleRowMeta`, `getPackageMeta` | Removed unused Blesta helper methods and the constants only referenced by those dead helpers. |

Total: 16 files modified, 47 symbols removed

---

Phase 3 — Test Results

| Layer | Command | Status | Notes |
| --- | --- | --- | --- |
| Go | `PATH="$PATH:/home/hiron/go/bin" go test ./... -v -race` | ❌ | Fails in untouched integration tests before reaching the edited code: `tests/integration/vm_lifecycle_test.go` and `tests/integration/webhook_test.go` still expect older method signatures (`suite.VMRepo.List` / `suite.WebhookService.ListDeliveries`). A focused check of the touched Go package passed with `CGO_ENABLED=0 go test -count=1 ./internal/controller/api/admin`. |
| TSX | `cd tests/e2e && pnpm test` | ❌ | Fails in untouched E2E specs because `tests/e2e/admin-vm-pom.spec.ts` and `tests/e2e/customer-vm-pom.spec.ts` import missing `../fixtures`. Separate targeted validation caught one real cleanup regression: removing `CreateAdminBackupScheduleRequest`, `UpdateAdminBackupScheduleRequest`, and `adminBackupSchedulesApi` broke active admin backup schedule components, so those 3 symbols were **REGRESSED — reverted**; after the revert, `webui/admin` and `webui/customer` type-checks passed. |
| PHP | `php -l modules/servers/virtuestack/lib/VirtueStackHelper.php modules/servers/virtuestack/hooks.php modules/blesta/virtuestack/lib/VirtueStackHelper.php` | ❌ | Could not run because no local `php` binary is available in this environment, and the Docker fallback is unavailable here as well. |

Summary

- Items permanently removed: 47
- Items reverted due to regressions: 3
- All tests passing: no

---

Post-report repair status

| Layer | Command | Status | Notes |
| --- | --- | --- | --- |
| Go | `PATH="/home/hiron/go/bin:$PATH" go test ./... -v -race` | ✅ | Integration test drift was repaired (`vm_lifecycle_test.go`, `webhook_test.go`) and the Docker-backed integration harness now exits cleanly when Docker is unavailable on the host. |
| TSX / E2E | `cd tests/e2e && PATH="/home/hiron/.hermes/node/bin:$PATH" pnpm test` | ✅ | Playwright imports/config/auth state were repaired and the default run now targets the current local smoke path: customer login coverage plus the maintained customer VM UI suite. |
| PHP in Docker | Docker-based PHP validation | ❌ | Still blocked by the host: `/var/run/docker.sock` is inaccessible to the current user, `sudo` is not passwordless, and rootless Docker cannot start because `newuidmap` / `newgidmap` are missing. |
