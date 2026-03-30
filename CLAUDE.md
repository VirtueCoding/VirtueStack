# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VirtueStack is a KVM/QEMU VM management platform for VPS hosting providers. Stack: Go backend (Controller + Node Agent), Next.js frontends (admin/customer portals), PostgreSQL database, NATS JetStream task queue, and WHMCS billing integration.

> **Full technical reference:** See `AGENTS.md` for architecture, API endpoints, database schema, gRPC methods, authentication, storage layer, async tasks, and environment variables.

## Build & Development Commands

### Go Backend
```bash
make build                  # Build controller + node-agent binaries to bin/
make build-controller       # Controller only (always works without native libs)
make build-node-agent       # Node Agent only (requires libvirt/Ceph dev headers)
make test                   # Run Go tests that do not require libvirt/Ceph dev headers
make test-integration       # Run Docker/Testcontainers-backed integration tests
make test-native            # Run node-agent/libvirt/Ceph tests on a prepared host
make test-race              # Non-native tests with race detector
make test-all               # Run all test suites
make test-coverage          # Generate HTML coverage report for the non-native test set
make lint                   # golangci-lint run ./... (25 linters enabled)
make vet                    # go vet ./...
make vuln                   # govulncheck ./...
make proto                  # Regenerate protobuf Go code from proto/
make deps                   # go mod download + verify + tidy
make certs                  # Generate mTLS certificates for Node Agent communication
```

Run a single Go test:
```bash
go test -race -run TestFunctionName ./internal/controller/services/...
```

### Frontend (Admin & Customer WebUIs)
Both UIs use **npm** (not yarn or pnpm). Run from their respective directories:
```bash
cd webui/admin    # or webui/customer
npm ci            # Install dependencies (CI uses this, not npm install)
npm run dev       # Dev server (admin :3000, customer :3001)
npm run build     # Production build
npm run lint      # ESLint
npm run type-check # tsc --noEmit
```

### Database Migrations
```bash
make migrate-up                       # Apply all pending migrations
make migrate-down                     # Rollback last migration
make migrate-create NAME=feature_name # Create new migration pair
```
71 sequential migrations (000001–000071) in `migrations/`. Each migration has `.up.sql` and `.down.sql` files.

### Docker
```bash
docker compose up -d    # Start all services (postgres, nats, controller, admin-webui, customer-webui, nginx)
docker compose build    # Rebuild images
docker compose down     # Stop all services
```

Docker Compose variants: `docker-compose.yml` (base), `docker-compose.override.yml` (dev), `docker-compose.prod.yml` (production), `docker-compose.test.yml` (testing with mock node agent).

### E2E Testing
```bash
# Automated setup (generates secrets, certs, seed data, starts services)
./scripts/setup-e2e.sh --start

# Run E2E tests (uses pnpm, not npm)
cd tests/e2e && pnpm install && pnpm test

# Run specific test category
pnpm run test:admin      # Admin portal tests
pnpm run test:customer   # Customer portal tests
pnpm run test:auth       # Authentication tests

# Cleanup
./scripts/setup-e2e.sh --clean
```
See `tests/e2e/README.md` for detailed E2E testing guide.

## CI Pipeline

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to main:
1. Go lint + test with race detector (with PostgreSQL 18 + NATS service containers)
2. PHP module syntax check
3. Admin frontend: `npm ci` + lint + type-check + build
4. Customer frontend: `npm ci` + lint + type-check + build
5. Docker image builds + Trivy vulnerability scanning
6. Security: `govulncheck` + `npm audit`

E2E tests (`.github/workflows/e2e.yml`) run on push/PR to main:
1. Start PostgreSQL + NATS service containers
2. Build Controller + WebUIs
3. Run database migrations + seed test data
4. Run Playwright E2E tests

## Test Coverage

### Go Backend Unit Tests

| Package | Test Files | Coverage Focus |
|---------|------------|----------------|
| `internal/controller/api/admin` | `auth_test.go`, `customers_test.go`, `nodes_test.go`, `plans_test.go` | HTTP handler validation |
| `internal/controller/api/customer` | `auth_test.go` | Password change, auth validation |
| `internal/controller/api/middleware` | Multiple test files | Rate limiting, auth, CSRF, permissions |
| `internal/controller/services` | Multiple test files | Business logic, auth, 2FA, RBAC, circuit breaker |
| `internal/controller/repository` | Multiple test files | Database operations |
| `internal/controller/tasks` | `handlers_test.go`, `template_*_test.go` | Task handler logic |
| `internal/shared/*` | Multiple test files | Crypto, config, errors, SSRF, email validation |

```bash
make test-coverage   # Generate HTML coverage report
```

### Test Categories

- **Unit Tests**: Table-driven tests with testify, mocked dependencies
- **Integration Tests**: Docker/Testcontainers-backed (7 files in `tests/integration/`)
- **E2E Tests**: Playwright tests (17 spec files in `tests/e2e/`)
- **Load Tests**: k6 load tests (`tests/load/`)
- **Security Tests**: OWASP ZAP script (`tests/security/`)

## Key References
- `AGENTS.md` — LLM reference: architecture, API endpoints, database schema, gRPC methods, environment variables
- `docs/codemaps/` — Token-lean architecture summaries (~4K tokens total)
- `docs/coding-standard.md` — **MANDATORY**: 19 Quality Gates that all generated code MUST pass
- `docs/installation.md` — Installation guide for production and test environments
- `docs/api-reference.md` — Complete API reference with request/response examples
- `tests/e2e/README.md` — E2E testing guide with architecture, credentials, and troubleshooting
- `proto/virtuestack/node_agent.proto` — gRPC service definition (972 lines, 38 RPC methods)
