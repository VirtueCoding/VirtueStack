# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VirtueStack is a KVM/QEMU VM management platform for VPS hosting providers. Stack: Go backend (Controller + Node Agent), Next.js frontends (admin/customer portals), PostgreSQL database, NATS JetStream task queue, and WHMCS billing integration.

> **Full technical reference:** See `AGENTS.md` for architecture, API endpoints, database schema, gRPC methods, authentication, storage layer, async tasks, and environment variables.

## Build & Development Commands

### Go Backend
```bash
make build                  # Build controller + node-agent binaries to bin/
make build-controller       # Controller only
make build-node-agent       # Node Agent only
make test                   # Run all Go tests
make test-race              # Tests with race detector (used in CI)
make test-coverage          # Generate HTML coverage report
make lint                   # golangci-lint run ./...
make vet                    # go vet ./...
make vuln                   # govulncheck ./...
make proto                  # Regenerate protobuf Go code from proto/
make deps                   # go mod download + verify + tidy
```

Run a single Go test:
```bash
go test -race -run TestFunctionName ./internal/controller/services/...
```

### Frontend (Admin & Customer WebUIs)
Both UIs use the same scripts. Run from their respective directories:
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
Migrations use `golang-migrate/migrate` with sequential numbering (000001, 000002, ...). Each migration has `.up.sql` and `.down.sql` files in `migrations/`.

### Docker
```bash
docker compose up -d    # Start all services (postgres, nats, controller, admin-webui, customer-webui, nginx)
docker compose build    # Rebuild images
docker compose down     # Stop all services
```

### E2E Testing
```bash
# Automated setup (generates secrets, certs, seed data, starts services)
./scripts/setup-e2e.sh --start

# Run E2E tests
cd tests/e2e && npm test

# Run specific test category
npm run test:admin      # Admin portal tests
npm run test:customer   # Customer portal tests
npm run test:auth       # Authentication tests

# Cleanup
./scripts/setup-e2e.sh --clean
```
See `tests/e2e/README.md` for detailed E2E testing guide.

## CI Pipeline

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to main:
1. `go vet` + `go test -race` (with PostgreSQL 16 + NATS service containers)
2. Admin frontend: `npm ci` + lint + type-check + build
3. Customer frontend: `npm ci` + lint + type-check + build
4. Docker image builds (controller, admin-webui, customer-webui)
5. Security: `govulncheck` + `npm audit`

E2E tests (`.github/workflows/e2e.yml`) run on push/PR to main affecting WebUI or API code:
1. Start PostgreSQL + NATS service containers
2. Build Controller + WebUIs
3. Run database migrations
4. Seed test data
5. Run Playwright E2E tests across browsers

## Test Coverage

### Go Backend Unit Tests

| Package | Test Files | Coverage Focus |
|---------|------------|----------------|
| `internal/controller/api/admin` | `auth_test.go`, `customers_test.go`, `nodes_test.go`, `plans_test.go` | HTTP handler validation (40+ tests) |
| `internal/controller/api/customer` | `auth_test.go` | Password change, auth validation |
| `internal/controller/api/middleware` | Multiple | Rate limiting, auth, CSRF |
| `internal/controller/services` | Multiple | Business logic, auth flows, 2FA |
| `internal/controller/repository` | Multiple | Database operations |
| `internal/shared/*` | Multiple | Crypto, config, utilities |

Run coverage report:
```bash
make test-coverage   # Opens HTML report in browser
```

### Test Categories

- **Validation Tests**: Input sanitization, boundary testing, error format consistency
- **Service Tests**: Business logic with mocked repositories
- **Integration Tests**: Database operations with test containers
- **E2E Tests**: Full user flows via Playwright

## Key References
- `AGENTS.md` - LLM reference: architecture, API endpoints, database schema, gRPC methods, environment variables
- `docs/CODEMAPS/` - Token-lean architecture summaries (~4K tokens total): `architecture.md`, `backend.md`, `frontend.md`, `data.md`, `dependencies.md`
- `docs/CODING_STANDARD.md` - **MANDATORY**: 19 Quality Gates that all generated code MUST pass
- `docs/INSTALL.md` - Installation guide for production and test environments
- `tests/e2e/README.md` - E2E testing guide with architecture, credentials, and troubleshooting
- `proto/virtuestack/node_agent.proto` - gRPC service definition (785 lines)