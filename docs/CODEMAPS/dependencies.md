<!-- Generated: 2026-03-19 | Files scanned: go.mod, package.json | Token estimate: ~600 -->

# Dependencies

## Backend (Go 1.25)

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

### Security
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/alexedwards/argon2id` | v1.0.0 | Password hashing |
| `github.com/golang-jwt/jwt/v5` | v5.2.1 | JWT tokens |
| `github.com/pquerna/otp` | v1.4.0 | TOTP 2FA |

### Infrastructure
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/ceph/go-ceph` | v0.30.0 | RBD storage |
| `libvirt.org/go/libvirt` | v1.10005.0 | KVM/QEMU |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket |
| `github.com/prometheus/client_golang` | v1.20.5 | Metrics |

### Validation & Config
| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/go-playground/validator/v10` | v10.26.0 | Request validation |
| `gopkg.in/yaml.v3` | v3.0.1 | Config parsing |

## Frontend (Node.js)

### Framework
| Package | Purpose |
|---------|---------|
| `next` | React framework (App Router) |
| `react` | UI library |
| `typescript` | Type safety |

### UI
| Package | Purpose |
|---------|---------|
| `tailwindcss` | Styling |
| `@radix-ui/*` | Headless components |
| `lucide-react` | Icons |

### State & Data
| Package | Purpose |
|---------|---------|
| `@tanstack/react-query` | Server state |
| `zustand` | Client state |

### Charts & Console
| Package | Purpose |
|---------|---------|
| `uplot` | Fast charts |
| `echarts` | Complex charts |
| `@novnc/novnc` | VNC client |
| `xterm` | Terminal emulator |

## External Services

### Required
| Service | Purpose | Config |
|---------|---------|--------|
| PostgreSQL 16+ | Primary database | `DATABASE_URL` |
| NATS JetStream | Task queue | `NATS_URL` |
| KVM/QEMU | Hypervisor | Node Agent binary |
| Ceph RBD OR QCOW2 | Storage | `STORAGE_BACKEND` |

### Optional
| Service | Purpose | Config |
|---------|---------|--------|
| PowerDNS | rDNS management | `PDNS_MYSQL_DSN` |
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
| `golangci-lint` | Go linting |
| `govulncheck` | Security scanning |
| `npm` / `pnpm` | Frontend deps |
| `playwright` | E2E testing |

## Test Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/stretchr/testify` | Assertions |
| `github.com/testcontainers/*` | Integration tests |
| `@playwright/test` | E2E tests |