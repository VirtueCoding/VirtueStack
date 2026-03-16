# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VirtueStack is a KVM/QEMU VM management platform for VPS hosting providers. It has three main components: a Go backend (Controller + Node Agent), two TypeScript/React frontends (admin + customer portals), and a PHP WHMCS billing integration module. See `AGENTS.md` for exhaustive technical reference.

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

## Architecture

### Two-Binary System
- **Controller** (`cmd/controller/main.go`): HTTP API server (Gin), task orchestrator, database owner. Talks to Node Agents via gRPC.
- **Node Agent** (`cmd/node-agent/main.go`): Runs on each hypervisor host. Manages VMs via libvirt, storage via Ceph RBD or QCOW2, networking via nftables/dnsmasq. Exposes gRPC service defined in `proto/virtuestack/node_agent.proto`.

### Controller Internal Structure
```
internal/controller/
  api/
    admin/         # Admin API handlers (/api/v1/admin/*)
    customer/      # Customer API handlers (/api/v1/customer/*)
    provisioning/  # WHMCS API handlers (/api/v1/provisioning/*)
    middleware/     # Auth (JWT/API key), rate limiting, CSRF, audit logging
  services/        # Business logic layer (auth, VM, backup, failover, RBAC, webhooks, rDNS)
  repository/      # Database access layer (pgx/v5 with PostgreSQL)
  models/          # Data structures and constants
  tasks/           # Async task handlers (vm.create, backup.create, vm.migrate, etc.)
```

### Node Agent Internal Structure
```
internal/nodeagent/
  server.go        # gRPC server implementation
  vm/              # VM lifecycle, domain XML generation, console proxying
  storage/         # Dual backend: rbd.go (Ceph) and qcow.go (QCOW2) behind StorageBackend interface
  network/         # nwfilter anti-spoofing, bandwidth shaping, DHCP, IPv6, abuse prevention
  guest/           # QEMU guest agent operations
```

### Three-Tier API Authentication
| Tier | Path Prefix | Auth Method |
|------|-------------|-------------|
| Admin | `/api/v1/admin/*` | JWT + 2FA (TOTP) |
| Customer | `/api/v1/customer/*` | JWT + Refresh Token |
| Provisioning | `/api/v1/provisioning/*` | API Key (X-API-Key header) |

### Async Task System
API handlers publish tasks to NATS JetStream (durable stream "TASKS"). A worker pool in the Controller subscribes, executes handlers that call Node Agents via gRPC, updates task status in PostgreSQL, and notifies WebSocket subscribers. Task types: `vm.create`, `vm.reinstall`, `vm.resize`, `vm.migrate`, `backup.create`, `backup.restore`, `snapshot.create`, `snapshot.revert`, `webhook.deliver`.

### Key Patterns
- **Repository pattern**: All DB access goes through `repository/` structs that take `*pgxpool.Pool`
- **Service layer**: Business logic in `services/` structs, dependency-injected with repos and gRPC clients
- **StorageBackend interface**: Both Ceph RBD and QCOW2 implement `internal/nodeagent/storage/interface.go`
- **Custom errors**: `internal/shared/errors/errors.go` defines typed errors (ValidationError, NotFoundError, etc.)
- **Standardized API responses**: `{"data": ...}` for success, `{"error": {"code": ..., "message": ..., "correlation_id": ...}}` for errors
- **Row Level Security**: PostgreSQL RLS policies isolate customer data at the database level

### Frontend Stack
Both UIs use Next.js 16, React 19, shadcn/ui (Radix primitives), Tailwind CSS, TanStack Query, React Hook Form + Zod validation. Customer portal additionally uses @novnc/novnc for VNC and @xterm/xterm for serial console.

### WHMCS Module
PHP module at `modules/servers/virtuestack/` integrates with the Provisioning API. `virtuestack.php` is the main module entry point; `hooks.php` and `webhook.php` handle WHMCS lifecycle events.

## CI Pipeline

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to main:
1. `go vet` + `go test -race` (with PostgreSQL 16 + NATS service containers)
2. Admin frontend: `npm ci` + lint + type-check + build
3. Customer frontend: `npm ci` + lint + type-check + build
4. Docker image builds (controller, admin-webui, customer-webui)
5. Security: `govulncheck` + `npm audit`

## Key References
- `AGENTS.md` - Full API endpoint listing, database schema, gRPC methods, environment variables
- `docs/ARCHITECTURE.md` - Detailed architecture specification
- `proto/virtuestack/node_agent.proto` - gRPC service definition (~786 lines)
