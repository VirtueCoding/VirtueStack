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

## CI Pipeline

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to main:
1. `go vet` + `go test -race` (with PostgreSQL 16 + NATS service containers)
2. Admin frontend: `npm ci` + lint + type-check + build
3. Customer frontend: `npm ci` + lint + type-check + build
4. Docker image builds (controller, admin-webui, customer-webui)
5. Security: `govulncheck` + `npm audit`

## Key References
- `AGENTS.md` - LLM reference: architecture, API endpoints, database schema, gRPC methods, environment variables
- `CODING_STANDARD.md` - **MANDATORY**: 19 Quality Gates that all generated code MUST pass
- `proto/virtuestack/node_agent.proto` - gRPC service definition (785 lines)