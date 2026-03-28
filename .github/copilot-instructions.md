# Copilot Coding Agent Instructions for VirtueStack

## Project Overview

VirtueStack is a KVM/QEMU Virtual Machine management platform for VPS hosting providers. It consists of a **Go backend** (Controller + Node Agent), **TypeScript/React frontends** (Next.js admin and customer portals), **PostgreSQL 16** database with Row-Level Security, and **NATS JetStream** for async task processing. The Controller communicates with Node Agents over **gRPC with mTLS**.

## Quick Context Loading

For a token-efficient architecture overview, read the codemaps first (~4K tokens total):

- `docs/CODEMAPS/architecture.md` — System overview, service boundaries, data flow
- `docs/CODEMAPS/backend.md` — API routes, services, repositories, middleware
- `docs/CODEMAPS/frontend.md` — Page tree, components, state management
- `docs/CODEMAPS/data.md` — Database schema, entity relationships, RLS policies
- `docs/CODEMAPS/dependencies.md` — External deps, build tools

For full specifications, refer to `AGENTS.md` (comprehensive LLM reference) and `docs/CODING_STANDARD.md` (mandatory quality gates).

## Repository Layout

```
cmd/controller/          — Controller entry point
cmd/node-agent/          — Node Agent entry point
internal/controller/     — Controller: API handlers, services, repos, models, tasks
internal/nodeagent/      — Node Agent: VM lifecycle, storage, network, metrics
internal/shared/         — Shared packages: config, crypto, errors, logging, proto
proto/                   — gRPC .proto definitions
migrations/              — Sequential SQL migrations (000001–000044)
webui/admin/             — Next.js admin portal
webui/customer/          — Next.js customer portal
modules/servers/virtuestack/ — WHMCS billing integration (PHP)
tests/integration/       — Go integration tests (Docker/Testcontainers)
tests/e2e/               — Playwright end-to-end tests
tests/load/              — k6 load tests
configs/                 — Grafana dashboards, Prometheus alert rules
scripts/                 — Utility scripts (backup, E2E setup, certs)
docs/                    — Architecture docs, install guide, API reference
```

## Build & Test Commands

### Go Backend

```bash
make build                # Build controller + node-agent (output: bin/)
make build-controller     # Build controller only (always works)
make build-node-agent     # Build node-agent only (requires native libs, see below)
make test                 # Run Go unit tests (controller packages only)
make test-race            # Unit tests with race detector
make test-integration     # Docker/Testcontainers integration tests
make test-native          # Node Agent tests (requires libvirt/Ceph dev headers)
make test-coverage        # Generate HTML coverage report
make lint                 # golangci-lint (25 linters, see .golangci.yml)
make vet                  # go vet (requires native libs for full scan)
make vuln                 # govulncheck
make proto                # Regenerate protobuf Go code
make deps                 # go mod download + verify + tidy
```

Run a single Go test:
```bash
go test -race -run TestFunctionName ./internal/controller/services/...
```

### Frontend (webui/admin and webui/customer)

Both WebUIs use **npm** with `package-lock.json`:
```bash
cd webui/admin    # or webui/customer
npm ci            # Install dependencies (use npm ci, not npm install)
npm run build     # Production build
npm run lint      # ESLint
npm run type-check # tsc --noEmit
npm run dev       # Dev server (admin: 3000, customer: 3001)
```

### E2E Tests

E2E tests use **pnpm** (not npm):
```bash
cd tests/e2e
pnpm install              # Install E2E dependencies
pnpm test                 # Run all Playwright tests
pnpm run test:admin       # Admin portal tests only
pnpm run test:customer    # Customer portal tests only
```

### Database Migrations

```bash
make migrate-up                         # Apply all pending migrations
make migrate-down                       # Rollback last migration
make migrate-create NAME=feature_name   # Create new migration pair (.up.sql + .down.sql)
```

Migrations use `golang-migrate/migrate` with sequential numbering (000001, 000002, ...).

### Docker

```bash
docker compose up -d      # Start full stack (postgres, nats, controller, UIs, nginx)
docker compose build      # Rebuild images
docker compose down       # Stop all services
```

## Critical Build Gotchas

### 1. Node Agent Requires Native Libraries

The Node Agent uses CGO with dynamic linking to libvirt and Ceph libraries. Building or testing it requires native development headers:

```bash
# Required for build/test of node-agent packages:
sudo apt install -y pkg-config libvirt-dev librados-dev librbd-dev
```

**Without these headers:**
- `make build-node-agent` fails with "Package 'libvirt' not found"
- `make vet` fails (scans all packages including node-agent)
- `make test-native` fails

**What always works without native headers:**
- `make build-controller` — builds the Controller binary
- `make test` — runs controller/shared unit tests only (node-agent packages are excluded)
- `make test-race` — same as `make test` with race detector

### 2. Package Manager Split

- `webui/admin/` and `webui/customer/` use **npm** (`package-lock.json`)
- `tests/e2e/` uses **pnpm** (`pnpm-lock.yaml`)
- Do not mix them — use `npm ci` for WebUIs and `pnpm install` for E2E tests.

### 3. Migration 000031 Is a No-Op

`migrations/000031_concurrent_indexes.up.sql` is intentionally a compatibility no-op. The original migration used `CREATE INDEX CONCURRENTLY` which cannot run inside `golang-migrate`'s transaction wrapper. The indexes already exist from earlier migrations. For production upgrades on large tables, rebuild indexes manually outside the migration chain.

### 4. golangci-lint Not Pre-installed

`golangci-lint` is not installed by default. CI installs it via the GitHub Action. To run locally:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.0.2
```

### 5. Environment Variables Required for Docker Stack

The Docker stack requires these variables (set via `.env` file, see `.env.example`):
- `POSTGRES_PASSWORD` — no default, containers fail without it
- `NATS_AUTH_TOKEN` — no default, required for NATS authentication
- `JWT_SECRET` — 32+ characters for JWT signing
- `ENCRYPTION_KEY` — 64 hex characters for AES-256-GCM encryption

### 6. Redis Required in Multi-Instance Production

When `APP_ENV=production` with multiple Controller instances, `REDIS_URL` must be set for shared rate limiting. Single-instance deployments use in-memory rate limiting by default.

## Code Architecture Patterns

### Three-Tier API System

| Tier | Base Path | Auth | Purpose |
|------|-----------|------|---------|
| Admin | `/api/v1/admin/*` | JWT + 2FA + RBAC | Full management |
| Customer | `/api/v1/customer/*` | JWT or API Key | Self-service portal |
| Provisioning | `/api/v1/provisioning/*` | API Key | WHMCS integration |

### Layered Architecture

```
Handler (api/{tier}/) → Service (services/) → Repository (repository/) → PostgreSQL
                                            → NATS JetStream (async tasks)
                                            → Node Agent (gRPC)
```

### Handler Pattern

All HTTP handlers follow this structure:

```go
func (h *Handler) CreateResource(c *gin.Context) {
    // 1. Parse & validate request
    var req models.CreateResourceRequest
    if err := middleware.BindAndValidate(c, &req); err != nil {
        // Returns typed API error
        middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
        return
    }

    // 2. Call service layer with context
    resource, err := h.service.Create(c.Request.Context(), &req)
    if err != nil {
        h.logger.Error("failed to create resource", "error", err,
            "correlation_id", middleware.GetCorrelationID(c))
        middleware.RespondWithError(c, http.StatusInternalServerError,
            "CREATE_FAILED", "Internal server error")
        return
    }

    // 3. Audit log (for mutations)
    h.logAuditEvent(c, "resource.create", "resource", resource.ID, changes, true)

    // 4. Return standardized response
    c.JSON(http.StatusCreated, models.Response{Data: resource})
}
```

### Error Response Format

Always use `middleware.RespondWithError()` — never `c.JSON()` for errors:

```go
middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid hostname")
```

This produces:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid hostname format",
    "correlation_id": "req_abc123"
  }
}
```

### Service Constructor Pattern

Services use a config struct for dependency injection (avoids parameter explosion):

```go
type VMServiceConfig struct {
    VMRepo        *repository.VMRepository
    NodeRepo      *repository.NodeRepository
    TaskPublisher TaskPublisher    // Interface
    NodeAgent     NodeAgentClient  // Interface
    Logger        *slog.Logger
}

func NewVMService(cfg VMServiceConfig) *VMService {
    return &VMService{
        vmRepo: cfg.VMRepo,
        logger: cfg.Logger.With("component", "vm-service"),
    }
}
```

### Repository Pattern

Repositories use pgx/v5 with centralized column lists:

```go
type VMRepository struct {
    db *pgxpool.Pool
}

func (r *VMRepository) GetByID(ctx context.Context, id string) (*models.VM, error) {
    // Uses parameterized queries, never string interpolation
    row := r.db.QueryRow(ctx, "SELECT "+vmColumns+" FROM vms WHERE id = $1", id)
    return scanVM(row)
}
```

### Custom Error Types

Located in `internal/shared/errors/errors.go`:

```go
// Sentinel errors — use with errors.Is()
errors.Is(err, sharederrors.ErrNotFound)
errors.Is(err, sharederrors.ErrUnauthorized)
errors.Is(err, sharederrors.ErrValidation)
errors.Is(err, sharederrors.ErrConflict)
errors.Is(err, sharederrors.ErrLimitExceeded)

// Typed errors
sharederrors.ValidationError{Field: "hostname", Message: "invalid format"}
sharederrors.OperationError{Operation: "vm.create", Step: "allocate_ip", Err: err}
```

### Async Tasks via NATS

```go
// Publish a task from a handler/service
taskID, err := h.taskPublisher.PublishTask(ctx, "vm.create", map[string]any{
    "vm_id": vm.ID,
    "node_id": node.ID,
})

// Task handlers in internal/controller/tasks/
// Registered in worker.go, executed by worker pool (4 workers)
```

### Testing Patterns

Go tests use **table-driven tests** with **testify**:

```go
tests := []struct {
    name    string
    input   string
    wantErr bool
}{
    {"valid input", "test-vm", false},
    {"empty input", "", true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        err := service.Validate(tt.input)
        if tt.wantErr {
            require.Error(t, err)
        } else {
            require.NoError(t, err)
        }
    })
}
```

Mock interfaces with function fields:

```go
type mockDB struct {
    queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}
```

### Frontend Patterns

- **State management:** TanStack Query (server state)
- **Forms:** react-hook-form + Zod validation
- **UI components:** shadcn/ui (Radix primitives + Tailwind)
- **API calls:** Centralized in `lib/api-client.ts`
- **Charts:** Recharts

## Key Conventions

1. **Context threading:** Always pass `ctx context.Context` to service and repository methods.
2. **Structured logging:** Use `slog.Logger` with component context; include `correlation_id` in error logs.
3. **Request validation:** Use `validate` struct tags with `middleware.BindAndValidate()`.
4. **Secrets in models:** Mark sensitive fields with `json:"-"` to prevent serialization.
5. **RLS-aware queries:** Customer-scoped repo methods set `app.current_customer_id` via PostgreSQL session variables.
6. **Error wrapping:** Use `fmt.Errorf("context: %w", err)` for proper error chain propagation.
7. **Permission constants:** Defined in `internal/controller/models/`, enforced via middleware.
8. **Audit logging:** Log all resource mutations with actor ID, resource type, and changes.
9. **No TODOs/FIXMEs:** The coding standard (docs/CODING_STANDARD.md §1) prohibits leaving these in code.
10. **Max function length:** 40 lines (QG-01), max nesting: 3 levels.

## Adding New Features

### New API Endpoint

1. Add handler method in `internal/controller/api/{admin|customer|provisioning}/`
2. Register route in the tier's `routes.go` with appropriate middleware
3. Add service method in `internal/controller/services/`
4. Add repository methods in `internal/controller/repository/`
5. Add/update models in `internal/controller/models/`
6. Add database migration if schema changes: `make migrate-create NAME=feature_name`
7. Write tests (table-driven, mocked dependencies)

### New Storage Operation

1. Add method to `StorageBackend` interface (`internal/nodeagent/storage/interface.go`)
2. Implement in `rbd.go`, `qcow.go`, and `lvm.go`
3. Add gRPC method to `proto/virtuestack/node_agent.proto`
4. Regenerate proto: `make proto`
5. Implement gRPC handler in `internal/nodeagent/server.go`

### New Async Task

1. Define task type constant in `internal/controller/models/task.go`
2. Create handler in `internal/controller/tasks/{task_name}.go`
3. Register handler in `internal/controller/tasks/worker.go`
4. Publish from API handler via `taskPublisher.PublishTask()`

### New Database Migration

```bash
make migrate-create NAME=add_feature_column
# Edit: migrations/000045_add_feature_column.up.sql
# Edit: migrations/000045_add_feature_column.down.sql
```

Follow expand-contract pattern for zero-downtime migrations. Never use `CREATE INDEX CONCURRENTLY` inside migrations (golang-migrate wraps them in transactions).

## CI Pipeline

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to main:

1. **go-lint-test:** `go vet` + `golangci-lint` + `make test-race` (with PostgreSQL + NATS service containers)
2. **php-module-check:** PHP syntax validation of WHMCS module
3. **frontend-admin:** `npm ci` + lint + type-check + build
4. **frontend-customer:** `npm ci` + lint + type-check + build
5. **docker-build:** Build Docker images + Trivy security scan
6. **security:** `govulncheck` + `npm audit`

E2E tests (`.github/workflows/e2e.yml`) run on changes to WebUI, API, or test files.

## Key File References

| Purpose | File(s) |
|---------|---------|
| Full LLM reference | `AGENTS.md` |
| Quality gates & coding rules | `docs/CODING_STANDARD.md` |
| Architecture quick reference | `docs/CODEMAPS/*.md` |
| Admin API routes | `internal/controller/api/admin/routes.go` |
| Customer API routes | `internal/controller/api/customer/routes.go` |
| Provisioning API routes | `internal/controller/api/provisioning/routes.go` |
| Auth middleware | `internal/controller/api/middleware/auth.go` |
| Error response helper | `internal/controller/api/middleware/recovery.go` |
| Custom error types | `internal/shared/errors/errors.go` |
| VM model & states | `internal/controller/models/vm.go` |
| Storage interface | `internal/nodeagent/storage/interface.go` |
| gRPC proto definition | `proto/virtuestack/node_agent.proto` |
| Go linter config | `.golangci.yml` |
| Docker Compose | `docker-compose.yml` |
| Environment template | `.env.example` |
| E2E test guide | `tests/e2e/README.md` |
