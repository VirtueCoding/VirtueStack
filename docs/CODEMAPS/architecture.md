<!-- Generated: 2026-03-22 | Files scanned: 180+ | Token estimate: ~900 -->

# VirtueStack Architecture

## System Overview

```
                    ┌─────────────────────────────────────────────────────────────┐
                    │                      EXTERNAL CLIENTS                        │
                    │   WHMCS Module  │  Admin Portal  │  Customer Portal         │
                    └────────┬───────────────┬────────────────┬───────────────────┘
                             │               │                │
                             ▼               ▼                ▼
                    ┌─────────────────────────────────────────────────────────────┐
                    │                     NGINX (SSL Termination)                  │
                    │                    Ports: 80, 443                           │
                    └────────────────────────────┬────────────────────────────────┘
                                                 │
                    ┌────────────────────────────┼────────────────────────────────┐
                    │                            ▼                                │
                    │  ┌──────────────────────────────────────────────────────┐  │
                    │  │                    CONTROLLER                         │  │
                    │  │  Go + Gin | JWT Auth | PostgreSQL | NATS JetStream   │  │
                    │  │  Port: 8080 (internal)                               │  │
                    │  └───────────────────┬───────────────────────────────────┘  │
                    │                      │                                       │
                    │         ┌────────────┼────────────┐                         │
                    │         ▼            ▼            ▼                         │
                    │  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
                    │  │PostgreSQL│  │  NATS   │  │ PowerDNS │                  │
                    │  │   16+    │  │JetStream│  │ (optional)│                  │
                    │  └──────────┘  └──────────┘  └──────────┘                  │
                    │                                                           │
                    │  DOCKER STACK (Controller, WebUIs, PostgreSQL, NATS)      │
                    └───────────────────────────────────────────────────────────┘
                                                 │
                              gRPC over mTLS     │
                    ┌────────────────────────────┼────────────────────────────┐
                    │                            ▼                            │
                    │  ┌───────────────────────────────────────────────────┐  │
                    │  │              NODE AGENT (per hypervisor)           │  │
                    │  │  Go + gRPC | libvirt | Ceph/QCOW | QEMU Guest     │  │
                    │  │  Runs directly on KVM host (not containerized)    │  │
                    │  └───────────────────────────────────────────────────┘  │
                    │                                                           │
                    │  BARE METAL NODES (KVM/QEMU, Ceph/QCOW, libvirt)        │
                    └──────────────────────────────────────────────────────────┘
```

## Service Boundaries

| Service | Responsibility | Deployment |
|---------|---------------|------------|
| Controller | API gateway, auth, task orchestration, DB | Docker container |
| Node Agent | VM lifecycle, storage, networking, metrics | Host binary |
| Admin WebUI | Admin portal (Next.js 16) | Docker container |
| Customer WebUI | Customer self-service (Next.js 16) | Docker container |
| PostgreSQL | Persistent state, RLS policies | Docker container |
| NATS JetStream | Async task queue, durable messages | Docker container |

## Data Flow

```
API Request → Middleware (Auth, CSRF, Rate Limit, Permissions)
           → Handler → Service → Repository → PostgreSQL
           → (async) NATS → Task Worker → Node Agent gRPC
           → Response
```

## Key Files

| Component | Entry Point | Config |
|-----------|-------------|--------|
| Controller | `cmd/controller/main.go` | `internal/controller/config.go` |
| Node Agent | `cmd/node-agent/main.go` | `internal/nodeagent/config.go` |
| Admin UI | `webui/admin/app/layout.tsx` | `webui/admin/next.config.js` |
| Customer UI | `webui/customer/app/layout.tsx` | `webui/customer/next.config.js` |
| gRPC Proto | `proto/virtuestack/node_agent.proto` | - |
| Migrations | `migrations/000001_initial_schema.up.sql` | - |

## New Components (Since Last Update)

| Component | Location | Purpose |
|-----------|----------|---------|
| Audit Masking | `internal/controller/audit/` | PII masking for audit logs |
| Permissions Middleware | `internal/controller/api/middleware/permissions.go` | Admin RBAC enforcement |
| Console Tokens | `internal/controller/models/console_token.go` | Time-limited VNC/serial access |
| Admin Permissions | `internal/controller/models/permission.go` | Permission constants |