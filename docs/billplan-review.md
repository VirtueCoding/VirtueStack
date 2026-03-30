# billplan.md — Design Review

## Critical Issues (must fix before coding)

1. **The plan’s `billing_provider` backfill strategy will misclassify existing customers.**  
   **Where in the plan:** sections 9c, 10.1, 10.2.  
   **What is wrong:** the plan says `billing_provider` should default to `"whmcs"` for existing users. That is not safe in this codebase because self-registration already exists and creates customers with `whmcs_client_id = NULL`; existing non-WHMCS customers are therefore possible today. Defaulting every existing row to `"whmcs"` would silently misclassify ownership of native/manual users.  
   **What the fix should be:** do not globally default existing rows to `"whmcs"`. Backfill from existing data: `whmcs_client_id IS NOT NULL => 'whmcs'`; rows with `whmcs_client_id IS NULL` need either an explicit `"unmanaged"`/`"manual"` state or an operator-reviewed migration path before enabling native billing.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/registration.go:18-189`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/customer.go:14-28`

2. **The WHMCS “provider adapter” is modeled around an outbound WHMCS API that the controller does not have.**  
   **Where in the plan:** sections 9c and 9d.  
   **What is wrong:** the plan proposes WHMCS config with `api_url`/`api_key` and a controller-side `whmcs/client.go`. In the real code, the controller does not call WHMCS. The data flow is the opposite: the external WHMCS PHP module calls VirtueStack’s provisioning API, and VirtueStack sends generic system webhooks back to WHMCS. Putting a WHMCS API client into the controller invents a new trust boundary, creates new secrets to manage, and is not justified by the current integration.  
   **What the fix should be:** represent WHMCS as an ingress-facing adapter around the existing provisioning API ownership model, not as a controller-side outbound client. Keep WHMCS API credentials on the WHMCS side unless a new controller-to-WHMCS use case is explicitly added and justified.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/routes.go:123-157`, `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/lib/ApiClient.php:22-124`, `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/webhook.php:32-111`

3. **The proposed `BillingProvider` interface is the wrong abstraction boundary for the current code.**  
   **Where in the plan:** section 9b.  
   **What is wrong:** most of the proposed methods are native-billing concerns (`GetBalance`, `ProcessTopUp`, usage history) or payment/webhook concerns (`HandleWebhook`), while the WHMCS implementation would be almost entirely no-ops because WHMCS already drives VM lifecycle via `/api/v1/provisioning/*`. That is a strong signal the interface is mixing separate responsibilities.  
   **What the fix should be:** split the design into at least two boundaries:  
   - a **billing ownership / customer routing** layer that decides who owns a customer and VM lifecycle billing side effects;  
   - a **native billing/accounting** layer for balances, ledger, suspension policy;  
   - a **payment provider** layer for Stripe/PayPal/crypto webhooks and session creation.  
   Keep the existing provisioning API as the WHMCS ingress surface instead of trying to force WHMCS into the same method set as native billing.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/handler.go:15-71`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/customers.go:14-160`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/vms.go:17-116`

4. **The plan contradicts itself on single-provider vs. multi-provider configuration.**  
   **Where in the plan:** section 5 vs. section 9c.  
   **What is wrong:** section 5 introduces a global `BILLING_PROVIDER=whmcs|native|disabled`, but section 9c introduces a multi-provider registry with several providers enabled simultaneously and a `primary` selector. Those are materially different operating models.  
   **What the fix should be:** choose one model. Given the stated goal (“multiple billing sources running simultaneously”), the plan should standardize on per-provider `enabled` flags plus a single `primary` provider for new users, and explicitly say configuration changes are deploy-time only and require process restart.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/docs/billplan.md` sections 5 and 9c, `/home/runner/work/VirtueStack/VirtueStack/internal/shared/config/config.go:120-147`

5. **The ledger/payment design is not safe against duplicate credit posting or concurrent balance races.**  
   **Where in the plan:** sections 6.1, 6.2, 6.4, 12.3.  
   **What is wrong:** the plan stores `balance_cents` plus an immutable ledger, but it does not define idempotency keys, unique constraints for external payment IDs, or locking semantics for concurrent updates. Webhook retries, duplicate delivery, or concurrent top-up + hourly charge paths can post credits twice or write inconsistent `balance_after` values.  
   **What the fix should be:** add idempotency at the schema level and transaction level:  
   - unique constraints on external payment identities;  
   - a first-class idempotency key on ledger entries;  
   - row-level locking (`SELECT ... FOR UPDATE`) or a single-writer strategy for balance mutation;  
   - explicit duplicate-webhook behavior.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/docs/billplan.md` sections 6.1, 6.2, 6.4, 12.3

6. **The OAuth collision policy for WHMCS users is unsafe and underspecified.**  
   **Where in the plan:** sections 8.5 and 8.6.  
   **What is wrong:** the plan says a WHMCS-created customer can auto-link OAuth if the email matches. In this codebase, WHMCS customers are already real `customers` rows and may be intended to access the portal only via SSO. Auto-linking a Google/GitHub identity to the same email without an existing authenticated session or an explicit linking ceremony can grant alternate login paths that the hosting operator did not intend.  
   **What the fix should be:** define one unambiguous policy for WHMCS-linked accounts, e.g. “OAuth auto-link is forbidden for `whmcs_client_id IS NOT NULL` unless the user is already authenticated via portal session and performs an explicit link action.”  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/customer.go:14-28`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/customers.go:14-160`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/sso.go:14-94`

7. **The proposed billing authorization model does not fit the current permission model.**  
   **Where in the plan:** sections 11.1 and 11.4.  
   **What is wrong:** the plan proposes customer API key scopes `billing:read` / `billing:write` and admin permissions `billing:read` / `billing:write`, but the current code has neither. Customer API keys are intentionally limited to VM/backup/snapshot scopes, and admin permissions are fixed to plans/nodes/customers/vms/settings/backups/ipsets/templates/rdns/audit/storage.  
   **What the fix should be:** either make billing routes JWT-only in the first implementation, or explicitly add new customer scopes and admin permission constants plus route/middleware changes to support them.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/apikeys.go:50-59`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/routes.go:9-18`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/permission.go:7-98`

## High-Risk Items (fix during implementation)

1. **The plan understates how much registration/email verification already exists.**  
   **Where in the plan:** sections 4.1, 8.1, 8.2.  
   **What is wrong:** the plan frames self-registration as partially implemented, but the code already has working `Register` and `VerifyEmail` handlers, route gating, email verification token generation, and activation logic. That changes the migration and backfill risk substantially.  
   **What the fix should be:** rewrite those sections to distinguish “already implemented” from “future OAuth/account-linking work.”  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/registration.go:18-229`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/routes.go:93-111`

2. **The plan misses existing WHMCS integration points around template/location discovery.**  
   **Where in the plan:** sections 1 and 9a.  
   **What is wrong:** the WHMCS PHP side does more than call the provisioning endpoints listed in section 1. It also uses `listTemplates()` and `listLocations()` for order/product configuration. Those calls currently hit `/admin/templates` or `/provisioning/templates` and `/provisioning/locations` or `/admin/locations` as fallbacks.  
   **What the fix should be:** explicitly include template/location discovery in the WHMCS audit, and call out that location endpoints do not currently exist on the controller side.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/lib/ApiClient.php:538-563`, `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/hooks.php:744-865`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/admin/templates.go:26-38`

3. **Hourly billing has no downtime reconciliation strategy.**  
   **Where in the plan:** section 6.2.  
   **What is wrong:** the plan says a scheduler runs every hour, but it does not say what happens if the controller is down for six hours or if multiple controller instances race after restart. Native billing cannot be correct without reconciliation semantics.  
   **What the fix should be:** define whether charges are event-sourced, catch-up billed from durable timestamps, or intentionally skipped after downtime; add a single-writer/leader requirement or a deduplicated schedule table.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/schedulers.go` (existing schedulers pattern), `/home/runner/work/VirtueStack/VirtueStack/docs/billplan.md` section 6.2

4. **The plan puts money-moving customer routes on dual-auth paths even though current JWT-only route partitioning says they should not be.**  
   **Where in the plan:** sections 11.1 and 11.4.  
   **What is wrong:** current customer routing deliberately keeps account-management, API key management, 2FA, and customer webhook management on a JWT-only group with CSRF enabled. Money movement belongs with those sensitive account routes, not the VM/API-key dual-auth group.  
   **What the fix should be:** make top-up/payment-method/account-balance mutation endpoints JWT-only first. Read-only history endpoints can be revisited later if API-key support is truly required.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/routes.go:36-73`

5. **The plan references middleware that does not exist by name.**  
   **Where in the plan:** section 12.5.  
   **What is wrong:** the plan refers to `middleware.CSRFProtection()`, but the existing middleware is `middleware.CSRF(middleware.DefaultCSRFConfig())`, with `SkipCSRFForAPIKey(...)` for dual-auth paths.  
   **What the fix should be:** update the plan to use the real middleware names and route grouping pattern.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/middleware/csrf.go:31-140`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/routes.go:51-69`

6. **The payment webhook routing section is internally inconsistent.**  
   **Where in the plan:** sections 9b, 11.2.  
   **What is wrong:** section 9b says `HandleWebhook` belongs on `BillingProvider`, but section 11.2 says `/api/v1/webhooks/{provider}` dispatches via `billingRegistry.ForProvider(provider).HandleWebhook()`. There is no `ForProvider` method in the proposed billing registry (section 9c only defines `ForCustomer`), and payment-provider dispatch belongs more naturally on the payment registry from section 7.  
   **What the fix should be:** move payment webhook dispatch to `PaymentProvider` / payment registry, keep billing-provider logic for ownership/accounting only.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/docs/billplan.md` sections 7, 9b, 9c, 11.2

7. **The plan’s rollback story is too vague for a Phase 0 refactor that touches auth and provisioning.**  
   **Where in the plan:** sections 9d and 13.  
   **What is wrong:** “run existing tests” is not a rollback procedure. If Phase 0 breaks WHMCS provisioning in production, the operator needs an exact rollback path.  
   **What the fix should be:** define rollback as “revert controller code to previous release; leave additive column unused; do not enable native provider flags until smoke tests succeed; verify WHMCS `TestConnection`, `CreateAccount`, `SuspendAccount`, `SingleSignOn`, and cron polling.”  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/virtuestack.php:216-332`, `:332-447`, `:1073-1150`

## Medium-Risk Items (track and address)

1. **Section 1 understates the WHMCS hook surface.**  
   `hooks.php` registers 12 hooks, not 11, and `virtuestack.php` contains more entrypoints/helpers than the plan’s summary implies.  
   **Fix:** update the counts and explicitly label which functions are provisioning entrypoints vs helpers.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/hooks.php:39-405`, `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/virtuestack.php`

2. **The plan’s top-level `/api/v1/webhooks/{provider}` path may confuse two existing webhook concepts.**  
   The codebase already has customer outbound webhooks at `/api/v1/customer/webhooks` and admin system/pre-action webhook management.  
   **Fix:** use a clearly separate namespace such as `/api/v1/payments/webhooks/{provider}` or `/api/v1/integrations/payments/{provider}`.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/webhooks.go:95-220`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/admin/routes.go:318-334`

3. **The plan assumes new admin billing permissions without considering RBAC migration cost.**  
   Adding `billing:read` / `billing:write` changes permission constants, admin defaults, UI, and permission editing flows.  
   **Fix:** budget explicit RBAC migration work or keep billing under existing settings/backups permissions until a separate RBAC expansion is designed.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/permission.go:7-147`

4. **Dependency choices are underspecified, which prevents meaningful pre-implementation CVE review.**  
   The plan implies `github.com/stripe/stripe-go/v82` and `golang.org/x/oauth2`, but does not pin versions for several other integrations and suggests direct HTTP for PayPal/crypto.  
   **Fix:** add a dependency appendix with pinned versions, maintenance status, and security review results before coding.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/go.mod:5-36`

5. **The plan reuses `whmcs_client_id` and `billing_provider` as overlapping sources of truth without defining precedence.**  
   **Fix:** state explicitly that `billing_provider` is authoritative after migration, and `whmcs_client_id` is only an external reference for the WHMCS provider.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/customer.go:14-28`

6. **The plan does not mention operator monitoring for billing failures.**  
   The codebase already has metrics, notification, and webhook delivery patterns that should be reused.  
   **Fix:** add billing metrics, alert rules, and operator notifications as a first-class phase requirement.  
   **Evidence:** `/home/runner/work/VirtueStack/VirtueStack/internal/controller/metrics/`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/notifications/`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/services/webhook.go:55-64`

## Factual Corrections

| Plan claim | Actual codebase reality | File path |
|---|---|---|
| `hooks.php` registers 11 WHMCS hooks | `hooks.php` contains 12 `add_hook(...)` registrations | `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/hooks.php:39-405` |
| The provisioning API exposes 19 endpoints | This is **correct** if rdns GET/PUT are counted separately: 14 VM endpoints + 1 task + 1 sso-tokens + 1 customers + 2 plans = 19 | `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/routes.go:90-120` |
| Only plan endpoints are relevant for WHMCS product configuration | The WHMCS module also fetches templates and locations | `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/lib/ApiClient.php:538-563`, `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/hooks.php:744-865` |
| Self-registration is only partially present | `Register` and `VerifyEmail` handlers are implemented and routed behind `AllowSelfRegistration` | `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/registration.go:37-189`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/routes.go:108-111` |
| Future billing/customer API keys can use `billing:read` / `billing:write` | No such scopes exist today; current customer API keys support only VM/backup/snapshot scopes | `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/apikeys.go:50-59` |
| Admin billing endpoints can rely on `billing:read` / `billing:write` permissions | No such admin permissions exist in current RBAC constants | `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/permission.go:7-98` |
| Existing config pattern already accommodates billing/OAuth flags | Current controller config only has `AllowSelfRegistration` and `RegistrationEmailVerification` in this area | `/home/runner/work/VirtueStack/VirtueStack/internal/shared/config/config.go:120-147`, `:214-235` |
| `Customer` can simply gain nullable OAuth-only password semantics later | Current `Customer.PasswordHash` is a required `string`, not `*string`, and there is no `AuthProvider` field | `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/customer.go:14-28` |
| Existing CSRF middleware can be referenced as `CSRFProtection()` | The actual middleware is `CSRF(DefaultCSRFConfig())`, with `SkipCSRFForAPIKey(...)` for dual-auth groups | `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/middleware/csrf.go:31-140`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/routes.go:51-69` |
| A controller-side WHMCS API client naturally belongs in the provider registry | The current controller has no outbound WHMCS client; WHMCS is the client and the controller is the server | `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/routes.go:123-157`, `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/lib/ApiClient.php:22-124` |

## Threat Model Summary

| Threat | Likelihood | Impact | Mitigation status in plan |
|---|---|---|---|
| Duplicate payment webhook / replay causes duplicate credit posting | H | H | Incomplete |
| Concurrent top-up and hourly charge corrupt `balance_cents` / `balance_after` | M | H | Missing |
| User/provider escalation by changing `billing_provider` ownership | M | H | Incomplete |
| OAuth email collision grants unintended access to WHMCS-owned account | H | H | Missing |
| Crypto underpayment / expired rate window leaves ambiguous account state | M | M | Incomplete |
| Gateway outage (Stripe/PayPal/crypto) blocks top-ups with no operator guidance | H | M | Missing |
| Rolling deploy / config toggle splits traffic across different provider ownership logic | M | M | Missing |
| Wallet/invoice exhaustion from repeated top-up initiations | M | M | Missing |
| Webhook spoofing against Stripe/PayPal/crypto endpoints | M | H | Covered but incomplete |
| Controller downtime causes missed or duplicated hourly billing | H | M | Missing |

## Missing from Plan

1. **An authoritative ownership model for `billing_provider` vs `whmcs_client_id`.**
2. **An explicit backfill algorithm for existing customers that does not assume every existing row is WHMCS-owned.**
3. **A reconciliation strategy for scheduler downtime, multi-instance execution, and restart catch-up billing.**
4. **Schema-level idempotency requirements for payments and ledger entries.**
5. **A hard policy for WHMCS-account OAuth linking and SSO coexistence.**
6. **A dependency appendix with pinned versions and pre-implementation security review.**
7. **A production rollback procedure for Phase 0 beyond “run tests.”**
8. **A monitoring/alerting plan: metrics, dashboards, alert rules, and operator notifications for billing failures.**
9. **A sandbox/integration testing matrix for Stripe, PayPal, crypto providers, and WHMCS regression tests.**
10. **A clear route namespace decision for inbound payment webhooks that does not overlap conceptually with existing customer/system webhooks.**

## Verdict

This plan is **not ready for implementation yet**. It has several strong ideas, but the most important ownership and abstraction decisions are still wrong for the actual VirtueStack codebase: the WHMCS adapter is modeled as if the controller talks to WHMCS, the `billing_provider` migration would misclassify existing users, the `BillingProvider` interface mixes ownership/accounting/payment concerns, and the authorization model does not match current RBAC or customer API key scopes. The plan needs one more revision focused on (1) ownership/backfill semantics, (2) abstraction boundaries, (3) idempotent ledger design, and (4) route/auth alignment with the existing controller architecture before coding begins.
