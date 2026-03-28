<!-- Generated: 2026-03-28 | Files scanned: 220+ | Token estimate: ~900 -->

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
                    │  │  Go + gRPC | libvirt | Ceph/QCOW/LVM | QEMU      │  │
                    │  │  Runs directly on KVM host (not containerized)    │  │
                    │  └───────────────────────────────────────────────────┘  │
                    │                                                           │
                    │  BARE METAL NODES (KVM/QEMU, Ceph/QCOW/LVM, libvirt)    │
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
| Redis | Distributed rate limiting (optional, HA) | External or Docker |

## Data Flow

```
API Request → Middleware (Correlation, Metrics, RateLimit, Recovery, Auth, CSRF, Permissions, Validation)
           → Handler → Service → Repository → PostgreSQL
           → (async) NATS → Task Worker (4 workers) → Node Agent gRPC
           → Response
```

## Key Files

| Component | Entry Point | Config |
|-----------|-------------|--------|
| Controller | `cmd/controller/main.go` | `internal/shared/config/config.go` |
| Node Agent | `cmd/node-agent/main.go` | `internal/nodeagent/config.go` |
| Admin UI | `webui/admin/app/layout.tsx` | `webui/admin/next.config.js` |
| Customer UI | `webui/customer/app/layout.tsx` | `webui/customer/next.config.js` |
| gRPC Proto | `proto/virtuestack/node_agent.proto` (972 lines) | - |
| Migrations | `migrations/` (65 migrations: 000001–000065) | - |
| Storage Factory | `internal/nodeagent/storage/factory.go` | 3 backends: Ceph, QCOW, LVM |

## Notable Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Audit Masking | `internal/controller/audit/` | PII masking for audit logs |
| Permissions Middleware | `internal/controller/api/middleware/permissions.go` | Admin RBAC enforcement |
| Console Tokens | `internal/controller/models/console_token.go` | Time-limited VNC/serial access |
| SSO Tokens | `internal/controller/models/sso_token.go` | WHMCS single sign-on bootstrap |
| Storage Backend Registry | `internal/controller/services/storage_backend_service.go` | Multi-backend management |
| Template Distribution | `internal/controller/tasks/template_distribute.go` | Template caching on QCOW/LVM nodes |
| Redis Client | `internal/controller/redis/client.go` | Distributed rate limiting |
| SSRF Protection | `internal/shared/util/ssrf.go` | URL validation for template ISO downloads |