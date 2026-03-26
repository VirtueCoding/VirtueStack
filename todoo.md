# VirtueStack audit todo

Generated: 2026-03-26

This file captures codebase findings from a repository-wide review. Each item is intentionally left unchecked and includes concrete next steps.

## Security / threat-model findings

- [ ] Replace panic-based password generation in the provisioning API request path.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/provisioning/vms.go:92-140`
  - Risk: `generateRandomPassword()` calls `panic(...)` on `crypto/rand` failure, which can crash the controller during an HTTP request instead of returning a controlled `5xx`.
  - Solution steps:
    - Change `generateRandomPassword()` to return `(string, error)` instead of panicking.
    - Propagate the error through the provisioning handler and return `middleware.RespondWithError(..., 500, ...)`.
    - Add a focused test proving entropy/read failures do not terminate the process.

- [ ] Make ISO upload limit enforcement globally atomic across controller instances.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/iso_upload.go:37-47`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/iso_upload.go:176-206`
  - Risk: `isoLimitMu` only protects a single process. In multi-controller deployments, concurrent uploads can bypass the per-VM ISO limit.
  - Solution steps:
    - Move the limit check into durable shared state instead of a process-local mutex.
    - Introduce a DB-backed ISO metadata table or another atomic counter keyed by VM.
    - Enforce the count with a transaction / `SELECT ... FOR UPDATE` or a uniqueness/limit strategy.
    - Add a concurrency test that simulates two parallel uploads against the same VM.

- [ ] Fail closed for distributed rate limiting in production.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/middleware/ratelimit.go:155-166`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/middleware/ratelimit.go:211-219`
  - Risk: the current in-memory limiter is bypassable behind a load balancer because each controller instance keeps its own counters.
  - Solution steps:
    - Add a distributed backend (for example Redis) as the production path.
    - Refuse startup in production when only the in-memory limiter is configured for horizontally scaled deployments.
    - Document the deployment requirement in install/runtime docs and CI examples.
    - Add tests that cover the backend selection logic.

- [ ] Revisit the WHMCS SSO threat model and remove JWTs from URL query strings.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/lib/VirtueStackHelper.php:438-456`, `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/virtuestack.php:615-629`
  - Risk: `?sso_token=...` can leak via browser history, referer headers, reverse-proxy logs, and support screenshots. The repo documentation already calls out this tradeoff.
  - Solution steps:
    - Replace URL-carried JWTs with opaque one-time tokens stored server-side.
    - Exchange the opaque token for an HttpOnly session on first use.
    - Shorten token lifetime and make tokens single-use if query-string delivery must remain temporarily.
    - Add audit logging for token issuance and redemption.

## Bugs / correctness gaps

- [ ] Repair the broken backup integration tests so they match the current backup model/service contract.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/tests/integration/backup_test.go:30-56`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/models/backup.go:22-43`, `/home/runner/work/VirtueStack/VirtueStack/internal/controller/services/backup_service.go:194-238`
  - Risk: `make test` currently fails because the tests still expect removed fields such as `Backup.Type` and `Backup.DiffFromSnapshot`.
  - Solution steps:
    - Decide whether the tests are stale or the model/service regressed.
    - Update the tests to assert the current `Backup` shape (`Source`, `StorageBackend`, `Status`, etc.), or restore the removed contract if it was unintentionally broken.
    - Re-run the integration suite after aligning the API contract.

- [ ] Fix the ergonomics gap where `make test` hard-fails without native libvirt/Ceph development headers.
  - Evidence: baseline `make test` failure in this environment building node-agent packages that require `libvirt.pc` and `rados/librados.h`
  - Risk: contributors and generic CI runners cannot run the default test target unless native hypervisor/storage SDKs are preinstalled.
  - Solution steps:
    - Split controller/unit tests from node-agent native-integration tests, or gate node-agent builds behind explicit tags/targets.
    - Document the required system packages for the native test path.
    - Keep `make test` usable in a default development environment.

## Unimplemented / incomplete features

- [ ] Implement the WHMCS module entry points that currently return empty values.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/modules/servers/virtuestack/virtuestack.php:1108-1120`
  - Risk: `virtuestack_UsageUpdate`, `virtuestack_SingleSignOn`, and `virtuestack_AdminServicesTabFieldsSave` are effectively placeholders, leaving advertised integration features incomplete.
  - Solution steps:
    - Define the intended behavior for each WHMCS entry point.
    - Implement the controller/API calls and error handling.
    - Add module-level tests or at least deterministic integration fixtures around the new behavior.

- [ ] Resolve the unfinished connection-lifecycle design in `NodeClient.ReleaseConnection`.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/internal/controller/grpc_client.go:122-143`
  - Risk: the API suggests caller-managed release semantics, but today it only logs mismatches and does no lifecycle management. That invites misuse and future leaks/confusion.
  - Solution steps:
    - Either implement real reference counting / pooling semantics, or simplify the API by removing `ReleaseConnection`.
    - Document the intended ownership model for pooled gRPC connections.
    - Add tests that cover concurrent get/release/remove flows.

## Dead code / stale code paths

- [ ] Remove the panic-based deprecated crypto wrappers after migrating callers.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/internal/shared/crypto/crypto.go:127-188`, current call sites in `/home/runner/work/VirtueStack/VirtueStack/tests/integration/suite_test.go:96-108`, `/home/runner/work/VirtueStack/VirtueStack/tests/integration/suite_test.go:226`, `/home/runner/work/VirtueStack/VirtueStack/tests/integration/suite_test.go:446`
  - Risk: `GenerateRandomString()` and `GenerateRandomHex()` still panic on failure. They are marked deprecated and are mostly kept alive for older test code.
  - Solution steps:
    - Migrate remaining callers to `GenerateRandomToken()` / `SafeGenerateRandomHex()`.
    - Delete the deprecated wrappers once all call sites are updated.
    - Keep only error-returning crypto helpers in the production surface area.

- [ ] Remove or consolidate the duplicate generated protobuf tree if it is truly unused.
  - Evidence: unused duplicate directory `/home/runner/work/VirtueStack/VirtueStack/internal/shared/proto/proto/virtuestack` while imports target `/home/runner/work/VirtueStack/VirtueStack/internal/shared/proto/virtuestack`
  - Risk: duplicate generated artifacts increase maintenance cost and create ambiguity about the canonical import path.
  - Solution steps:
    - Confirm no code imports the nested `internal/shared/proto/proto/virtuestack` package.
    - Fix the generation target/path if it is being produced accidentally.
    - Remove the unused duplicate files and protect against regeneration drift.

- [ ] Remove deprecated compatibility aliases once callers are fully migrated.
  - Evidence: `/home/runner/work/VirtueStack/VirtueStack/internal/controller/api/customer/backups.go:308-310`
  - Risk: compatibility shims such as deprecated ownership helpers increase surface area and can hide the real canonical code path.
  - Solution steps:
    - Check whether any remaining code still depends on the deprecated helper.
    - Switch all callers to `verifyVMOwnership`.
    - Delete the alias once no references remain.

## Validation notes from this review

- [ ] Re-run the repository validation suite after addressing the items above.
  - Observed baseline:
    - `make test` exposed a real integration-test mismatch in `tests/integration/backup_test.go`.
    - `make test` also failed to build node-agent packages in this sandbox because native `libvirt`/`ceph` headers are missing.
    - `make lint`, `webui/admin npm run lint`, and `webui/customer npm run lint` were not runnable here because `golangci-lint` / `eslint` are not installed in the sandbox.
