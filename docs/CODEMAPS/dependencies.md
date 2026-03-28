<!-- Generated: 2026-03-28 | Files scanned: go.mod, package.json (x3) | Token estimate: ~700 -->

# Dependencies

## Backend (Go 1.26)

### Core Framework
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/gin-gonic/gin` | v1.10.1 | HTTP router |
| `google.golang.org/grpc` | v1.79.1 | gRPC framework |
| `github.com/nats-io/nats.go` | v1.38.0 | Message queue |

### Database
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/jackc/pgx/v5` | v5.7.2 | PostgreSQL driver |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | Migrations |
| `github.com/redis/go-redis/v9` | latest | Redis client (distributed rate limiting) |
| `github.com/go-sql-driver/mysql` | latest | PowerDNS MySQL driver |

### Security
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/alexedwards/argon2id` | v1.0.0 | Password hashing |
| `github.com/golang-jwt/jwt/v5` | v5.2.2 | JWT tokens |
| `github.com/pquerna/otp` | v1.4.0 | TOTP 2FA |

### Infrastructure
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/ceph/go-ceph` | v0.30.0 | RBD storage |
| `libvirt.org/go/libvirt` | v1.10005.0 | KVM/QEMU |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket |
| `github.com/prometheus/client_golang` | v1.20.5 | Metrics |
| `google.golang.org/protobuf` | latest | Protocol Buffers |

### Validation & Config
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/go-playground/validator/v10` | v10.26.0 | Request validation |
| `gopkg.in/yaml.v3` | v3.0.1 | Config parsing |
| `github.com/google/uuid` | latest | UUID generation |

## Frontend (Node.js)

### Admin WebUI (`webui/admin/package.json`)
| Package | Purpose |
|---------|---------|
| `next` (16+) | React framework (App Router) |
| `react` (19) | UI library |
| `typescript` (5.7) | Type safety |
| `tailwindcss` | Styling |
| `@radix-ui/*` | Headless components (shadcn/ui) |
| `@tanstack/react-query` (5.64) | Server state management |
| `react-hook-form` + `zod` | Form handling + validation |
| `lucide-react` | Icons |

### Customer WebUI (`webui/customer/package.json`)

Includes everything above, plus:
| Package | Purpose |
|---------|---------|
| `recharts` (3.8) | Resource charts |
| `@novnc/novnc` (1.5) | VNC console client |
| `@xterm/xterm` (6.0) | Serial console terminal |
| `@xterm/addon-fit` | Terminal auto-resize |

**Note:** Neither UI uses Zustand. TanStack Query handles all server state.

## External Services

### Required
| Service | Purpose | Config |
|---------|---------|--------|
| PostgreSQL 16+ | Primary database | `DATABASE_URL` |
| NATS JetStream | Task queue | `NATS_URL` |
| KVM/QEMU | Hypervisor | Node Agent binary |
| Ceph RBD, QCOW2, or LVM | Storage | `STORAGE_BACKEND` |

### Optional
| Service | Purpose | Config |
|---------|---------|--------|
| Redis | Distributed rate limiting (HA) | `REDIS_URL` |
| PowerDNS | rDNS management | `PDNS_MYSQL_DSN` or `PDNS_API_URL` |
| SMTP Server | Email notifications | `SMTP_*` |
| Telegram Bot | Notifications | `TELEGRAM_BOT_TOKEN` |
| WHMCS | Billing integration | `modules/servers/virtuestack/` |

## Infrastructure Dependencies

| Component | Technology | Version |
|-----------|------------|---------|
| Container Runtime | Docker | 26+ |
| Reverse Proxy | Nginx | 1.25+ |
| mTLS | Controller ↔ Node Agent | Required |

## Build Tools

| Tool | Purpose |
|------|---------|
| `make` | Build automation |
| `golangci-lint` (v2.0.2) | Go linting (25 linters) |
| `govulncheck` | Security scanning |
| `npm` | Frontend deps (WebUIs) |
| `pnpm` | E2E test deps |
| `playwright` | E2E testing |

## Test Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/stretchr/testify` | Assertions |
| `github.com/testcontainers/*` | Integration tests |
| `@playwright/test` | E2E tests |