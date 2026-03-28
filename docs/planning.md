# VirtueStack Implementation Planning

> **Source:** `docs/2.md` — Comprehensive Codebase Audit (2026-03-28)
> **Format:** Unticked checklist for LLM-driven implementation
> **Execution order:** Items within each phase are ordered to avoid blocking dependencies. Phases are sequential; items within a phase may be parallelized unless noted.

---

## Phase 1: Production Safety

### Gap #1 — VM State Machine + Transition Validation

**Priority:** 🔴 Critical | **Effort:** 3 days | **Dependencies:** None (start first)

#### 1a. Database Migration — Status Constraint

- [x] Create migration `migrations/000066_vm_state_machine.up.sql`:
  ```sql
  SET lock_timeout = '5s';
  ALTER TABLE vms ADD CONSTRAINT vms_status_check
    CHECK (status IN ('provisioning','running','stopped','suspended','migrating','reinstalling','error','deleted'));
  ```
- [x] Create matching `migrations/000066_vm_state_machine.down.sql`:
  ```sql
  SET lock_timeout = '5s';
  ALTER TABLE vms DROP CONSTRAINT IF EXISTS vms_status_check;
  ```

#### 1b. Transition Map in Models

- [x] In `internal/controller/models/vm.go`, add a `ValidVMTransitions` map after the status constants (around line 16):
  ```go
  var ValidVMTransitions = map[string][]string{
      VMStatusProvisioning: {VMStatusRunning, VMStatusError},
      VMStatusRunning:      {VMStatusStopped, VMStatusSuspended, VMStatusMigrating, VMStatusReinstalling, VMStatusError},
      VMStatusStopped:      {VMStatusRunning, VMStatusDeleted, VMStatusReinstalling, VMStatusMigrating, VMStatusError},
      VMStatusSuspended:    {VMStatusRunning, VMStatusStopped, VMStatusDeleted},
      VMStatusMigrating:    {VMStatusRunning, VMStatusError},
      VMStatusReinstalling: {VMStatusRunning, VMStatusError},
      VMStatusError:        {VMStatusStopped, VMStatusDeleted},
  }
  ```
- [x] Add a `ValidateVMTransition(from, to string) error` function that checks the map and returns `sharederrors.ErrConflict` if the transition is invalid
- [x] Add unit tests in `internal/controller/models/vm_test.go` — table-driven tests covering every valid transition and a set of invalid transitions (e.g., `deleted → running`, `error → running`, `provisioning → deleted`)

#### 1c. Repository — Atomic Transition Method

- [x] In `internal/controller/repository/vm_repo.go`, add a `TransitionStatus` method:
  ```go
  func (r *VMRepository) TransitionStatus(ctx context.Context, vmID, fromStatus, toStatus string) error {
      if err := models.ValidateVMTransition(fromStatus, toStatus); err != nil {
          return err
      }
      result, err := r.db.Exec(ctx,
          "UPDATE vms SET status = $1, updated_at = NOW() WHERE id = $2 AND status = $3 AND deleted_at IS NULL",
          toStatus, vmID, fromStatus)
      if err != nil {
          return fmt.Errorf("transition vm status: %w", err)
      }
      if result.RowsAffected() == 0 {
          return fmt.Errorf("vm %s not in expected state %s for transition to %s: %w",
              vmID, fromStatus, toStatus, sharederrors.ErrConflict)
      }
      return nil
  }
  ```
- [x] Keep the existing `UpdateStatus` method for backward compatibility but add a deprecation comment pointing to `TransitionStatus`
- [x] Add unit tests for `TransitionStatus` in `internal/controller/repository/vm_repo_test.go` — test successful transition, test wrong-state rejection (0 rows affected), test invalid transition pair

#### 1d. Migrate Call Sites to TransitionStatus

- [x] Audit all callers of `UpdateStatus` or `UpdateVMStatus` across the codebase. Key files:
  - `internal/controller/tasks/handlers_vm_create.go` — provisioning → running, provisioning → error
  - `internal/controller/tasks/handlers_vm_delete.go` — * → deleted
  - `internal/controller/tasks/vm_resize.go` — stopped → stopped (after resize)
  - `internal/controller/tasks/vm_reinstall.go` — reinstalling → running, reinstalling → error
  - `internal/controller/tasks/migration_execute.go` — migrating → running, migrating → error
  - `internal/controller/services/vm_service.go` — start/stop/restart/suspend/unsuspend transitions
  - `internal/controller/api/provisioning/vms.go` — suspend/unsuspend calls
- [x] Replace each `UpdateStatus(ctx, vmID, newStatus)` call with `TransitionStatus(ctx, vmID, currentStatus, newStatus)`, passing the expected current status
- [x] For each call site, handle the `ErrConflict` error appropriately (log + return error, do not silently ignore)
- [x] Run `make test-race` to verify no regressions

---

### Gap #2 — VM Creation Cleanup (Compensation Stack)

**Priority:** 🔴 Critical | **Effort:** 2 days | **Dependencies:** Gap #1c (uses TransitionStatus)

#### 2a. Compensation Stack Helper

- [x] Create `internal/controller/tasks/cleanup.go` with a compensation stack type:
  ```go
  type CompensationStack struct {
      steps  []CompensationStep
      logger *slog.Logger
  }

  type CompensationStep struct {
      Name    string
      Cleanup func(ctx context.Context) error
  }

  func NewCompensationStack(logger *slog.Logger) *CompensationStack { ... }
  func (cs *CompensationStack) Push(name string, cleanup func(ctx context.Context) error) { ... }
  func (cs *CompensationStack) Rollback(ctx context.Context) {
      // Execute in reverse order, log each error but continue
  }
  ```
- [x] Add unit tests in `internal/controller/tasks/cleanup_test.go` — test rollback order (LIFO), test that cleanup errors are logged but don't block subsequent cleanups, test empty stack rollback is a no-op

#### 2b. Refactor VM Create Handler

- [x] In `internal/controller/tasks/handlers_vm_create.go`, refactor the handler to use the compensation stack:
  - After successful disk clone: `stack.Push("delete-disk", func(ctx) { nodeClient.DeleteDisk(...) })`
  - After successful cloud-init: `stack.Push("delete-cloudinit", func(ctx) { ... })`
  - After successful IP allocation: `stack.Push("release-ips", func(ctx) { ipRepo.ReleaseIPsByVM(...) })`
  - After successful VM creation via gRPC: `stack.Push("delete-vm", func(ctx) { nodeClient.DeleteVM(...) })`
  - After successful VM start: `stack.Push("stop-vm", func(ctx) { nodeClient.ForceStopVM(...) })`
  - On any step failure: `stack.Rollback(ctx)` then set VM status to error via `TransitionStatus`
- [x] Ensure cleanup for **StartVM failure** now includes: force-stop VM, delete VM definition, delete disk, release IPs
- [x] Ensure cleanup for **MAC address update failure** includes rollback to consistent state
- [x] Ensure cleanup for **status update to running failure** logs the inconsistency clearly (VM is actually running but DB says provisioning)
- [x] Run `make test-race` to verify no regressions

---

### Gap #4 — Stuck-Task Recovery Scanner

**Priority:** 🟡 High | **Effort:** 1 day | **Dependencies:** None (parallel with #1)

> **Note:** JetStream handles message-level redelivery (AckWait=5min, MaxDeliver=3), but tasks that were ack'd by the worker and then the Controller crashed remain in `running` state in PostgreSQL forever. This scanner addresses that gap.

#### 4a. Task Recovery Scanner

- [x] In `internal/controller/tasks/worker.go`, add a `StartStuckTaskScanner` method on the `Worker` struct:
  ```go
  func (w *Worker) StartStuckTaskScanner(ctx context.Context, interval time.Duration, stuckThreshold time.Duration) {
      ticker := time.NewTicker(interval)
      defer ticker.Stop()
      for {
          select {
          case <-ctx.Done():
              return
          case <-ticker.C:
              w.recoverStuckTasks(ctx, stuckThreshold)
          }
      }
  }
  ```
- [x] Implement `recoverStuckTasks` — query for tasks where `status = 'running' AND started_at < NOW() - $1` (parameterized threshold), check retry count, either reset to `pending` (if retries < max) or mark `failed` with error message `"stuck task recovered after timeout"`
- [x] Add a `task_repo` method `FindStuckTasks(ctx, threshold time.Duration) ([]*models.Task, error)` and `ResetTask(ctx, taskID string) error` in `internal/controller/repository/task_repo.go`
- [x] Add a `retry_count` column if not present (check current schema) or use the existing `attempts` field

#### 4b. Wire Scanner into Controller Startup

- [x] In `internal/controller/server.go` (in `StartSchedulers` method), start the stuck-task scanner as a background goroutine:
  ```go
  go w.StartStuckTaskScanner(ctx, 5*time.Minute, 30*time.Minute)
  ```
- [x] Ensure the scanner respects the server's context for graceful shutdown

#### 4c. Tests

- [x] Add unit tests in `internal/controller/tasks/worker_test.go` (or new `stuck_task_scanner_test.go`):
  - Test: task stuck for 30+ minutes is reset to pending
  - Test: task stuck but under threshold is left alone
  - Test: task at max retries is marked failed instead of reset
  - Test: scanner handles empty result set gracefully
- [x] Run `make test-race`

---

### Gap #5 — Password Reset Rate Limiting

**Priority:** 🟡 High | **Effort:** 0.5 day | **Dependencies:** None (parallel)

> **Current state:** `middleware.PasswordResetRateLimit()` is already applied to both `/auth/forgot-password` and `/auth/reset-password` routes in `internal/controller/api/customer/routes.go`.

- [x] Verify the rate limits in `internal/controller/api/middleware/rate_limit.go` — confirm `PasswordResetRateLimit()` enforces per-email AND per-IP limits (audit recommends: 3 requests/hour per email, 10 requests/hour per IP)
- [x] If the current rate limiter only uses IP-based limiting, add email-based limiting:
  - Extract the email from the request body in the forgot-password handler
  - Apply a separate rate limit key using `email:<normalized_email>` in addition to IP
- [x] If limits are already adequate, add a comment documenting the rationale and mark this gap as addressed
- [x] Add or verify unit tests for the rate limiter covering: rate exceeded returns 429, rate not exceeded proceeds, different emails have independent limits
- [x] Run `make test-race`

---

## Phase 2: Developer Experience

### Gap #3 — OpenAPI/Swagger Specification

**Priority:** 🟡 High | **Effort:** 3–5 days | **Dependencies:** None (start of Phase 2)

#### 3a. Install swag

- [x] Add `github.com/swaggo/swag` and `github.com/swaggo/gin-swagger` to `go.mod`:
  ```bash
  go get github.com/swaggo/swag/v2/cmd/swag@latest
  go get github.com/swaggo/gin-swagger
  go get github.com/swaggo/files
  ```
- [x] Add `swag init` command to `Makefile` (e.g., `make swagger`)
- [x] Add general API info annotation in `cmd/controller/main.go`:
  ```go
  // @title VirtueStack API
  // @version 1.0
  // @description KVM/QEMU VM management platform API
  // @securityDefinitions.apikey BearerAuth
  // @in header
  // @name Authorization
  // @securityDefinitions.apikey APIKeyAuth
  // @in header
  // @name X-API-Key
  ```

#### 3b. Annotate Admin API Handlers

- [x] Add swag annotations to all handlers in `internal/controller/api/admin/`:
  - `auth.go` — Login, Verify2FA, Refresh, Logout
  - `nodes.go` — CRUD + Drain/Undrain/Failover
  - `vms.go` — CRUD + Migrate
  - `plans.go` — CRUD
  - `templates.go` — CRUD + Import + BuildFromISO + Distribute
  - `ip_sets.go` — CRUD + Available
  - `customers.go` — List, Get, Update, Delete + AuditLogs
  - `settings.go` — Get, Update
  - `backups.go` — List, Restore
  - `backup_schedules.go` — CRUD + Run
  - `provisioning_keys.go` — CRUD
  - `failover.go` — List
  - `audit_logs.go` — List
  - `storage_backends.go` — CRUD + health
  - `rdns.go` — if exists

#### 3c. Annotate Customer API Handlers

- [x] Add swag annotations to all handlers in `internal/controller/api/customer/`:
  - `auth.go`, `auth_password_reset.go` — Auth flows
  - `vms.go` — List, Get, Power operations
  - `backups.go`, `snapshots.go` — CRUD
  - `iso_upload.go` — Upload, List, Delete, Attach, Detach
  - `rdns.go` — Get, Set, Delete
  - `apikeys.go` — CRUD + Rotate
  - `webhooks.go` — CRUD + Deliveries
  - `two_factor.go` — 2FA flows
  - `profile.go` — Get, Update
  - `notifications.go` — Preferences

#### 3d. Annotate Provisioning API Handlers

- [x] Add swag annotations to all handlers in `internal/controller/api/provisioning/`:
  - `vms.go` — Create, Get, Delete, Suspend, Unsuspend, Resize, Password, Power, Status
  - `usage.go` — GetUsage
  - `tasks.go` — GetTask
  - `customers.go` — CreateOrGet

#### 3e. Generate and Serve

- [x] Run `swag init` to generate `docs/swagger.json` and `docs/swagger.yaml`
- [x] Add Swagger UI route in `internal/controller/server.go` (admin-only, behind auth):
  ```go
  router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
  ```
- [x] Add generated swagger files to `.gitignore` or commit them (team decision — document choice)
- [x] Verify generated spec covers all 130+ endpoints by running `swag init` and inspecting output
- [x] Run `make build` to ensure annotations don't break compilation

---

### Gap #6 — Split server.go (852 lines)

**Priority:** 🟡 Medium | **Effort:** 1 day | **Dependencies:** None (parallel with #3)

- [x] Create `internal/controller/dependencies.go` — move `InitializeServices()` and all repository/service construction logic from `server.go`
- [x] Create `internal/controller/schedulers.go` — move `StartSchedulers()`, `startMetricsCollector()`, `startBandwidthCollector()`, `startSessionCleanup()`, `collectControllerMetrics()`, `collectBandwidth()` from `server.go`
- [x] Create `internal/controller/response.go` — move `healthHandler()`, `readinessHandler()`, and `requestLogger()` from `server.go`
- [x] Keep in `server.go`: `Server` struct definition, `NewServer()`, `Start()`, `Stop()`, `setupRoutes()`, `RegisterAPIRoutes()`, and setter methods
- [x] Verify all files are in `package controller` and all methods still reference the `Server` struct correctly
- [x] Run `make build-controller && make test-race` to confirm no regressions

---

### Gap #7 — Split node_agent_client.go (1,556 lines)

**Priority:** 🟡 Medium | **Effort:** 1 day | **Dependencies:** None (parallel with #6)

- [x] Create `internal/controller/services/node_agent_vm.go` — move VM power operations (`StartVM`, `StopVM`, `ForceStopVM`, `DeleteVM`), VM creation (`CreateVM`), VM metrics (`GetVMMetrics`, `GetVMStatus`), and cloud-init (`GenerateCloudInit`)
- [x] Create `internal/controller/services/node_agent_storage.go` — move disk operations (`DeleteDisk`, `DeleteDiskSnapshot`, `CloneFromBackup`, `CloneFromTemplate`), snapshot operations (`CreateSnapshot`, `DeleteSnapshot`, `RestoreSnapshot`)
- [x] Create `internal/controller/services/node_agent_network.go` — move bandwidth operations if any exist in this file, and network-related gRPC calls
- [x] Create `internal/controller/services/node_agent_migration.go` — move `MigrateVM`, `AbortMigration`, `PostMigrateSetup`, `EvacuateNode`
- [x] Keep in `node_agent_client.go`: struct definition, constructor, connection pool management, metrics cache, `GetNodeMetrics`, `PingNode`, `GetNodeResources`
- [x] Verify all files are in `package services` and share the `NodeAgentGRPCClient` receiver
- [x] Run `make build-controller && make test-race`

---

### Gap #13 — Split backup_service.go (1,259 lines) and vm_service.go (1,009 lines)

**Priority:** 🟡 Medium | **Effort:** 1 day | **Dependencies:** None (parallel with #6/#7)

#### backup_service.go Split

- [x] Create `internal/controller/services/backup_create_service.go` — move `CreateBackup`, `CreateBackupWithLimitCheck`, `createQCOWBackup`, `createCephBackup`
- [x] Create `internal/controller/services/backup_restore_service.go` — move `RestoreBackup`
- [x] Create `internal/controller/services/backup_scheduler_service.go` — move `StartScheduler`, `runSchedulerTick`, `processVMsForBackup`, `shouldBackupVM`, `scheduleBackupForVM`, `CreateSchedule`, `ListSchedules`, `ListSchedulesPaginated`, `UpdateSchedule`, `DeleteSchedule`, `ApplyRetentionPolicy`, `ProcessExpiredBackups`
- [x] Keep in `backup_service.go`: struct definition, constructor, simple CRUD (`ListBackups`, `ListBackupsWithFilter`, `DeleteBackup`), snapshot methods (`CreateSnapshot`, `ListSnapshots`, `DeleteSnapshot`, `GetSnapshotCount`, `CheckSnapshotQuota`, async snapshot methods)

#### vm_service.go Split

- [x] Create `internal/controller/services/vm_power_service.go` — move `StartVM`, `StopVM`, `RestartVM`, `ForceStopVM`, and any suspend/unsuspend methods
- [x] Keep in `vm_service.go`: struct definition, constructor, `CreateVM`, `DeleteVM`, `ReinstallVM`, `ResizeVM`, `ResizeVMWithPlan`, `GetVM`, `ListVMs`, `GetVMMetrics`, `GetVMStatus`, `GetVMDetail`, `UpdateVMHostname`, `UpdateVMNetworkLimits`, `GetTaskStatus`, `ListTasks`, and internal helpers

- [x] Run `make build-controller && make test-race`

---

### Gap #9 — Proto Breaking-Change Detection (buf)

**Priority:** 🟡 Medium | **Effort:** 0.5 day | **Dependencies:** None (CI-only)

- [x] Create `buf.yaml` in the repository root:
  ```yaml
  version: v2
  modules:
    - path: proto
  ```
- [x] Create `buf.gen.yaml`:
  ```yaml
  version: v2
  plugins:
    - remote: buf.build/protocolbuffers/go
      out: internal/shared/proto
      opt: paths=source_relative
    - remote: buf.build/grpc/go
      out: internal/shared/proto
      opt: paths=source_relative
  ```
- [x] Add `buf-breaking` step to `.github/workflows/ci.yml`:
  ```yaml
  - name: Install buf
    uses: bufbuild/buf-setup-action@v1
  - name: Check proto breaking changes
    uses: bufbuild/buf-breaking-action@v1
    with:
      against: 'https://github.com/AbuGosok/VirtueStack.git#branch=main'
  ```
- [x] Test locally: `buf breaking --against .git#branch=main`
- [x] Document in `AGENTS.md` that `buf breaking` is now part of CI

---

### Gap #10 — Golden-File Tests for Domain XML

**Priority:** 🟡 Medium | **Effort:** 1 day | **Dependencies:** None (parallel)

> **Note:** `internal/nodeagent/vm/domain_xml_test.go` already exists. This gap adds golden-file (snapshot) tests.

- [x] Create `internal/nodeagent/vm/testdata/` directory for golden XML files
- [x] Create golden-file test helper in `internal/nodeagent/vm/domain_xml_test.go`:
  ```go
  func goldenTest(t *testing.T, name string, got string) {
      t.Helper()
      golden := filepath.Join("testdata", name+".golden.xml")
      if os.Getenv("UPDATE_GOLDEN") != "" {
          os.WriteFile(golden, []byte(got), 0644)
          return
      }
      expected, err := os.ReadFile(golden)
      require.NoError(t, err, "golden file missing, run with UPDATE_GOLDEN=1")
      assert.Equal(t, string(expected), got)
  }
  ```
- [x] Add golden-file test cases covering:
  - Ceph RBD disk configuration
  - QCOW2 disk configuration
  - LVM disk configuration
  - VM with attached ISO
  - VM without ISO
  - Multiple NIC configurations
  - Various CPU/memory configurations
- [x] Generate initial golden files: `UPDATE_GOLDEN=1 go test ./internal/nodeagent/vm/...`
- [x] Commit the `testdata/*.golden.xml` files
- [x] Run `make test-native` (or the node-agent tests) to verify tests pass

---

### Gap #15 — Squirrel Query Builder Adoption

**Priority:** 🟡 Medium | **Effort:** 1–2 days | **Dependencies:** None (parallel)

#### 15a. Add Dependency

- [x] Add squirrel to `go.mod`:
  ```bash
  go get github.com/Masterminds/squirrel
  ```

#### 15b. Migrate Repository Methods

- [x] Identify repository methods with manual SQL string building (conditional WHERE clauses):
  - `internal/controller/repository/vm_repo.go` — `List` method with `VMListFilter`
  - `internal/controller/repository/backup_repo.go` — `List` methods with filters
  - `internal/controller/repository/task_repo.go` — `List` method with filters
  - `internal/controller/repository/audit_log_repo.go` — `List` method with filters
  - `internal/controller/repository/ip_address_repo.go` — filtered queries
- [x] For each method, replace manual SQL concatenation with squirrel:
  ```go
  q := sq.Select(vmColumns...).From("vms").PlaceholderFormat(sq.Dollar)
  if filter.CustomerID != nil {
      q = q.Where(sq.Eq{"customer_id": *filter.CustomerID})
  }
  if filter.Status != nil {
      q = q.Where(sq.Eq{"status": *filter.Status})
  }
  sql, args, err := q.ToSql()
  ```
- [x] Keep simple single-table queries (GetByID, Create, Update) as raw SQL — no need to convert those
- [x] Run `make build-controller && make test-race` after each repository migration

---

## Phase 3: Operational Maturity

### Gap #11 — System-Level Webhook Events

**Priority:** 🟡 Medium | **Effort:** 3 days | **Dependencies:** None (start of Phase 3)

#### 11a. System Event Types

- [x] In `internal/controller/models/webhook.go` (or new file `system_events.go`), define system event types:
  ```go
  const (
      SystemEventNodeOffline       = "system.node.offline"
      SystemEventNodeOnline        = "system.node.online"
      SystemEventNodeDegraded      = "system.node.degraded"
      SystemEventFailoverTriggered = "system.failover.triggered"
      SystemEventFailoverCompleted = "system.failover.completed"
      SystemEventStorageWarning    = "system.storage.warning"
      SystemEventStorageCritical   = "system.storage.critical"
  )
  ```

#### 11b. System Webhook Configuration

- [x] Create migration `migrations/000067_system_webhooks.up.sql`:
  ```sql
  SET lock_timeout = '5s';
  CREATE TABLE system_webhooks (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      name VARCHAR(255) NOT NULL,
      url TEXT NOT NULL,
      secret TEXT NOT NULL,
      events TEXT[] NOT NULL DEFAULT '{}',
      is_active BOOLEAN NOT NULL DEFAULT true,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  CREATE INDEX idx_system_webhooks_active ON system_webhooks (is_active) WHERE is_active = true;
  ```
- [x] Create matching down migration

#### 11c. System Event Publisher

- [x] Create `internal/controller/services/system_event_service.go`:
  - `PublishSystemEvent(ctx, eventType string, payload map[string]any)` — publishes to NATS subject `virtuestack.events.system.*` and triggers system webhook delivery
  - Query active system webhooks matching the event type
  - Queue webhook delivery tasks via NATS

#### 11d. Wire Events into Existing Services

- [x] In `internal/controller/services/heartbeat_checker.go` — publish `system.node.offline` when a node misses heartbeat threshold
- [x] In `internal/controller/services/failover_service.go` — publish `system.failover.triggered` and `system.failover.completed`
- [x] In storage health monitoring — publish `system.storage.warning` / `system.storage.critical` when thresholds exceeded

#### 11e. Admin API for System Webhooks

- [x] Add CRUD endpoints in `internal/controller/api/admin/system_webhooks.go`:
  - `GET /admin/system-webhooks`
  - `POST /admin/system-webhooks`
  - `PUT /admin/system-webhooks/:id`
  - `DELETE /admin/system-webhooks/:id`
- [x] Register routes in `internal/controller/api/admin/routes.go`
- [x] Add repository methods in `internal/controller/repository/system_webhook_repo.go`
- [x] Run `make build-controller && make test-race`

---

### Gap #12 — Pre-Action Hooks + NATS Event Subjects

**Priority:** 🟡 Medium | **Effort:** 2 days | **Dependencies:** Gap #11 (uses event infrastructure)

#### 12a. NATS Event Publishing

- [x] Create `internal/controller/services/event_bus.go`:
  ```go
  type EventBus struct {
      js     nats.JetStreamContext
      logger *slog.Logger
  }

  func (eb *EventBus) Publish(ctx context.Context, subject string, data any) error {
      // Publish to NATS subject like "virtuestack.events.vm.created"
  }
  ```
- [x] Create NATS stream for events in worker initialization:
  ```go
  js.AddStream(&nats.StreamConfig{
      Name:     "EVENTS",
      Subjects: []string{"virtuestack.events.>"},
      MaxAge:   7 * 24 * time.Hour,
  })
  ```

#### 12b. Publish Events from Services

- [x] Add event publishing to key service methods:
  - `vm_service.go` — `virtuestack.events.vm.created`, `vm.started`, `vm.stopped`, `vm.deleted`, `vm.migrated`
  - `backup_service.go` — `virtuestack.events.backup.created`, `backup.restored`, `backup.deleted`
  - `services/auth_service.go` — `virtuestack.events.customer.login`

#### 12c. Pre-Action Webhook

- [x] Add `pre_action_webhooks` table via migration (URL, events, timeout, fail_open flag)
- [x] Implement synchronous webhook call before VM creation:
  - HTTP POST to webhook URL with action payload
  - 5-second timeout, fail-open by default (if webhook unreachable, proceed)
  - If webhook returns `{"approved": false}`, reject the request with 403
- [x] Wire into `vm_service.go` `CreateVM` before task publishing
- [x] Add admin API endpoints for managing pre-action webhooks
- [x] Run `make build-controller && make test-race`

---

### Gap #19 — Database Connection Pool Metrics

**Priority:** 🟡 Required | **Effort:** 0.5 day | **Dependencies:** None (parallel)

- [x] In `internal/controller/metrics/prometheus.go`, add pgx pool metric collectors:
  ```go
  var (
      DBPoolTotalConns = prometheus.NewGaugeFunc(...)
      DBPoolIdleConns  = prometheus.NewGaugeFunc(...)
      DBPoolMaxConns   = prometheus.NewGaugeFunc(...)
      DBPoolAcquiredConns = prometheus.NewGaugeFunc(...)
      DBPoolAcquireWaitTime = prometheus.NewGaugeFunc(...)
  )
  ```
- [x] Create a `RegisterDBPoolMetrics(pool *pgxpool.Pool)` function that registers the gauge funcs reading from `pool.Stat()`
- [x] Call `RegisterDBPoolMetrics` in `internal/controller/server.go` after pool creation
- [x] Add Grafana dashboard panel in `configs/grafana/` for DB pool metrics
- [x] Run `make build-controller && make test-race`

---

### Gap #18 — NATS Health Check

**Priority:** 🟡 Required | **Dependencies:** None

> **Current state:** Already implemented in `readinessHandler()` in `server.go` — checks `s.natsConn.Status() != nats.CONNECTED` and includes `"nats": "connected"/"disconnected"` in response.

- [x] Verify `readinessHandler()` returns HTTP 503 (not 200) when NATS is disconnected — if it currently returns 200 with `"nats": "disconnected"`, change to return 503 for proper load balancer integration
- [x] Add unit test for readiness endpoint with mocked NATS connection in both connected and disconnected states
- [x] Document the health check behavior in `docs/API.md`

---

### Gap #8 — Frontend Monorepo Extraction

**Priority:** 🟡 Medium | **Effort:** 3–5 days | **Dependencies:** None (parallel, large)

#### 8a. Setup Workspace

- [x] Create `webui/packages/ui/` directory
- [x] Create `webui/packages/api-client/` directory
- [x] Create `webui/packages/config/` directory (shared Tailwind config, tsconfig)
- [x] Add `webui/package.json` with npm workspaces configuration:
  ```json
  {
    "private": true,
    "workspaces": ["admin", "customer", "packages/*"]
  }
  ```

#### 8b. Extract Shared UI Components

- [x] Move the 15 duplicated shadcn/ui components from both `webui/admin/components/ui/` and `webui/customer/components/ui/` to `webui/packages/ui/`:
  - `avatar.tsx`, `badge.tsx`, `button.tsx`, `card.tsx`, `dialog.tsx`, `dropdown-menu.tsx`, `input.tsx`, `label.tsx`, `scroll-area.tsx`, `select.tsx`, `sheet.tsx`, `switch.tsx`, `table.tsx`, `toast.tsx`, `toaster.tsx`
- [x] Update imports in both `admin/` and `customer/` to reference `@virtuestack/ui` package
- [x] Keep app-specific components (`admin/components/ui/checkbox.tsx`, `admin/components/ui/textarea.tsx`, `customer/components/ui/progress.tsx`, `customer/components/ui/tabs.tsx`) in their respective apps

#### 8c. Extract Shared API Client

- [x] Identify common API client base logic (fetch wrapper, auth token handling, error parsing)
- [x] Move to `webui/packages/api-client/`
- [x] Update imports in both apps

#### 8d. Validate

- [x] Run `cd webui/admin && npm ci && npm run lint && npm run type-check && npm run build`
- [x] Run `cd webui/customer && npm ci && npm run lint && npm run type-check && npm run build`
- [x] Update CI workflow to use workspace install

---

### Gap #17 — Structured Logging in Task Handlers

**Priority:** 🟡 Required | **Effort:** 1 day | **Dependencies:** None (parallel)

- [x] Create a `taskLogger` helper in `internal/controller/tasks/logger.go`:
  ```go
  func taskLogger(base *slog.Logger, task *models.Task) *slog.Logger {
      l := base.With(
          "task_id", task.ID,
          "task_type", task.Type,
      )
      // Extract common payload fields
      if vmID, ok := task.Payload["vm_id"].(string); ok {
          l = l.With("vm_id", vmID)
      }
      if nodeID, ok := task.Payload["node_id"].(string); ok {
          l = l.With("node_id", nodeID)
      }
      return l
  }
  ```
- [x] Update all task handlers in `internal/controller/tasks/` to use `taskLogger` at entry:
  - `handlers_vm_create.go`
  - `handlers_vm_delete.go`
  - `vm_resize.go`
  - `vm_reinstall.go`
  - `migration_execute.go`
  - `backup_create.go`
  - `snapshot_handlers.go`
  - `webhook_deliver.go`
  - `template_build.go`
  - `template_distribute.go`
- [x] Replace ad-hoc `logger.With(...)` calls in each handler with the standardized `taskLogger`
- [x] Run `make build-controller && make test-race`

---

### Gap #14 — Handler Duplication Extraction

**Priority:** 🟡 Medium | **Effort:** 1–2 days | **Dependencies:** None (parallel)

- [ ] Create `internal/controller/api/common/` package (or `internal/controller/api/shared/`)
- [ ] Extract shared pagination parsing into `common/pagination.go`:
  ```go
  func ParsePaginationParams(c *gin.Context) (page, perPage int, err error) { ... }
  func ParseCursorParams(c *gin.Context) (cursor string, limit int, err error) { ... }
  ```
- [ ] Extract shared response formatting into `common/response.go`:
  ```go
  func RespondWithPaginatedList(c *gin.Context, data any, total int64, page, perPage int) { ... }
  func RespondWithCursorList(c *gin.Context, data any, nextCursor string, hasMore bool) { ... }
  ```
- [ ] Extract shared VM response enrichment (adding IPs, plan, template info) into `common/vm_response.go` if applicable
- [ ] Update admin, customer, and provisioning handlers to use common helpers
- [ ] Run `make build-controller && make test-race`

---

### Gap #16 — Cursor-Based Pagination Standardization

**Priority:** 🟡 Medium | **Effort:** 1–2 days | **Dependencies:** Gap #14 (uses common pagination helpers)

- [ ] Audit all list endpoints and identify which use offset-based vs cursor-based pagination:
  - Check admin: `/admin/vms`, `/admin/nodes`, `/admin/customers`, `/admin/plans`, `/admin/audit-logs`, `/admin/backups`, `/admin/backup-schedules`
  - Check customer: `/customer/vms`, `/customer/backups`, `/customer/snapshots`
  - Check provisioning: any list endpoints
- [ ] For each offset-based endpoint, migrate to cursor-based:
  - Update repository method to accept cursor parameter and use keyset pagination
  - Update handler to parse `cursor` query param instead of `page`
  - Use existing `internal/controller/repository/cursor/pagination.go` utilities
  - Return `next_cursor` and `has_more` in response meta
- [ ] Maintain backward compatibility: accept both `page` and `cursor` params during transition, prefer cursor if both provided
- [ ] Update frontend API clients in `webui/admin/` and `webui/customer/` to use cursor-based pagination
- [ ] Run `make build-controller && make test-race`
- [ ] Run `cd webui/admin && npm run type-check && cd ../customer && npm run type-check`

---

### Gap #21 — .env Validation Script

**Priority:** 🟡 Required | **Effort:** 0.5 day | **Dependencies:** None (parallel)

- [x] Create `scripts/validate-env.sh`:
  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  ERRORS=0

  check_required() { ... }  # Check var exists and is non-empty
  check_hex() { ... }       # Check var is valid hex string of expected length
  check_int() { ... }       # Check var is a positive integer

  check_required "DATABASE_URL"
  check_required "NATS_URL"
  check_required "NATS_AUTH_TOKEN"
  check_required "JWT_SECRET"
  check_hex "ENCRYPTION_KEY" 64

  # Validated when present
  if [ -n "${REDIS_URL:-}" ]; then check_format "REDIS_URL" "redis://"; fi
  if [ -n "${SMTP_PORT:-}" ]; then check_int "SMTP_PORT"; fi

  if [ "$ERRORS" -gt 0 ]; then
      echo "❌ $ERRORS configuration errors found"
      exit 1
  fi
  echo "✅ All configuration validated"
  ```
- [x] Make script executable: `chmod +x scripts/validate-env.sh`
- [x] Add validation step to `docker-compose.yml` or document in `docs/INSTALL.md` as a pre-start check
- [x] Add unit test: run script with missing vars and verify exit code 1; run with all vars and verify exit code 0

---

## Phase 4: Expansion & Scale

### Gap #20 — Customer Self-Registration

**Priority:** 🟡 Required | **Effort:** 3 days | **Dependencies:** None (parallel)

#### 20a. Configuration

- [ ] Add `ALLOW_SELF_REGISTRATION` environment variable (default: `false`) to `internal/shared/config/config.go`
- [ ] Add `REGISTRATION_EMAIL_VERIFICATION` environment variable (default: `true`)

#### 20b. Registration Endpoint

- [ ] Create `internal/controller/api/customer/registration.go`:
  - `POST /auth/register` — accepts email, password, name, phone
  - Validate input (email format, password strength)
  - Check if email already exists → return appropriate error
  - Create customer with `status = "pending_verification"` if email verification enabled
  - Send verification email with token
  - Return 201 Created
- [ ] Add email verification endpoint:
  - `POST /auth/verify-email` — accepts token, activates customer account
- [ ] Add rate limiting: 3 registrations/hour per IP

#### 20c. Migration

- [ ] Create migration for `email_verification_tokens` table:
  ```sql
  CREATE TABLE email_verification_tokens (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
      token_hash VARCHAR(128) NOT NULL,
      expires_at TIMESTAMPTZ NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  ```

#### 20d. Conditional Route Registration

- [ ] In `internal/controller/api/customer/routes.go`, conditionally register the route:
  ```go
  if cfg.AllowSelfRegistration {
      auth.POST("/register", handler.Register)
      auth.POST("/verify-email", handler.VerifyEmail)
  }
  ```
- [ ] Add tests for both enabled and disabled states
- [ ] Run `make build-controller && make test-race`

---

### Gap #22 — Service-Layer Test Coverage for Critical Paths

**Priority:** 🟡 Required | **Effort:** 3–5 days | **Dependencies:** None (parallel)

> **Current state:** Only `vm_service_health_test.go` exists. No tests for `backup_service` or `node_agent_client`.

#### 22a. VM Service Tests

- [ ] Create `internal/controller/services/vm_service_test.go` with table-driven tests:
  - Node selection with no available nodes → returns appropriate error
  - Node selection with all nodes at capacity → returns appropriate error
  - CreateVM with invalid plan ID → returns validation error
  - CreateVM with invalid template ID → returns validation error
  - Concurrent VM operations on the same VM → tests state machine enforcement
  - StartVM on already-running VM → returns conflict error
  - StopVM on already-stopped VM → returns conflict error
  - DeleteVM on already-deleted VM → idempotent success

#### 22b. Backup Service Tests

- [ ] Create `internal/controller/services/backup_service_test.go`:
  - CreateBackup with quota exceeded → returns limit error
  - CreateBackup with VM not found → returns not found error
  - RestoreBackup with backup not found → returns not found error
  - CreateSnapshot quota enforcement → returns limit error at quota
  - DeleteBackup with storage error → returns appropriate error
  - Scheduler tick with no VMs eligible → no-op

#### 22c. Node Agent Client Tests

- [ ] Create `internal/controller/services/node_agent_client_test.go`:
  - gRPC connection failure → returns appropriate error
  - gRPC timeout → returns appropriate error with context deadline exceeded
  - Metrics cache hit → returns cached data without gRPC call
  - Metrics cache miss → makes gRPC call and caches result
  - Node not found → returns not found error

- [ ] Run `make test-race` after each test file

---

### Gap #23 — Load Testing Expansion

**Priority:** 🟡 Required | **Effort:** 2–3 days | **Dependencies:** None (parallel)

> **Current state:** `tests/load/k6-vm-operations.js` exists with basic VM operation tests.

- [ ] Expand `tests/load/k6-vm-operations.js` or create additional k6 scripts for:
  1. `tests/load/k6-provisioning-create.js` — VM creation under concurrent load (10, 50, 100 simultaneous provisions)
  2. `tests/load/k6-customer-list.js` — Customer VM listing with many customers and VMs (pagination performance)
  3. `tests/load/k6-power-operations.js` — Power operations (start/stop/restart) under concurrent load
  4. `tests/load/k6-admin-listing.js` — Admin VM/customer listing with filters and pagination
  5. `tests/load/k6-task-throughput.js` — Task creation and processing throughput measurement
- [ ] Add k6 thresholds for each test:
  ```javascript
  export const options = {
      thresholds: {
          http_req_duration: ['p(95)<500'],  // 95th percentile under 500ms
          http_req_failed: ['rate<0.01'],     // Less than 1% errors
      },
  };
  ```
- [ ] Add `make load-test` target in `Makefile` to run all k6 scripts
- [ ] Document load testing setup in `tests/load/README.md`

---

## Cross-Cutting Concerns

### Documentation Updates

- [ ] After completing Phase 1, update `AGENTS.md` §8 (VM Lifecycle) to document the state machine and valid transitions
- [ ] After completing Gap #3 (OpenAPI), update `AGENTS.md` §5 to reference the generated spec
- [ ] After completing Gap #11 (system webhooks), update `AGENTS.md` §5 to document system webhook endpoints
- [ ] After each god-file split (#6, #7, #13), update `AGENTS.md` §2 (Repository Structure) with new file listings
- [ ] Keep `docs/CODEMAPS/backend.md` in sync with new files and routes

### CI Pipeline Updates

- [ ] After Gap #9: `buf breaking` check in CI
- [ ] After Gap #3: `swag init` validation step in CI (ensure generated spec is up-to-date)
- [ ] After Gap #8: Update frontend CI jobs to use workspace-level install
- [ ] After Gap #15: No CI change needed (squirrel is a build dependency, tested via existing tests)

### Regression Testing

- [ ] After each phase, run the full test suite: `make test-race`
- [ ] After frontend changes: `cd webui/admin && npm run lint && npm run type-check && npm run build`
- [ ] After frontend changes: `cd webui/customer && npm run lint && npm run type-check && npm run build`
- [ ] After migration changes: verify `make migrate-up && make migrate-down` cycle works
