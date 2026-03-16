# VirtueStack

**A modern, scalable Virtual Machine management platform built for VPS hosting providers.**

[![Go Version](https://img.shields.io/badge/Go-1.25-blue)](https://golang.org/)
[![React](https://img.shields.io/badge/React-19-blue)](https://reactjs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-blue)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

VirtueStack provides a complete infrastructure-as-a-service platform for managing virtual machines across distributed compute nodes.

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Development](#development)
- [Production Deployment](#production-deployment)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

VirtueStack is a cloud-native VM management platform for VPS hosting providers:

- **Multi-tenant** - Separate admin and customer portals with RBAC
- **Distributed** - Scale across multiple hypervisor nodes
- **Flexible Storage** - Ceph RBD or QCOW2 backends
- **Live Migration** - Zero-downtime VM movement
- **API Integration** - REST API for WHMCS and custom billing
- **High Availability** - Automatic node failover with IPMI fencing
- **Monitoring** - Prometheus metrics with Grafana dashboards

---

## Features

### Core Platform

| Feature | Status |
|---------|--------|
| VM Lifecycle Management | ✅ Create, start, stop, restart, reinstall VMs |
| Live Migration | ✅ Zero-downtime migration between nodes |
| Storage Management | ✅ Ceph RBD + QCOW2 dual backend |
| Bandwidth Monitoring | ✅ Real-time tracking with limits and overage throttling |
| Backup & Snapshots | ✅ Automated backups, snapshots, and recovery |
| Console Access | ✅ Web-based VNC (noVNC) and Serial (xterm.js) consoles |
| HA Node Failover | ✅ IPMI fencing, STONITH, Ceph blocklist, VM redistribution |
| PowerDNS rDNS | ✅ Reverse DNS with MySQL direct access, IPv4+IPv6 PTR |
| IPv6 Support | ✅ /48 prefix allocation, /64 per-VM subnets |
| WHMCS Integration | ✅ Full provisioning, suspend, resize, terminate module |
| ISO Mounting | ✅ Upload and attach ISO images to VMs |
| Webhooks | ✅ Customer webhook delivery with retry and logging |

### Security & Authentication

| Feature | Status |
|---------|--------|
| Multi-factor Authentication | ✅ TOTP/2FA with backup codes |
| JWT Authentication | ✅ Secure token-based sessions with refresh tokens |
| RBAC | ✅ Role-based permissions (customer and admin) |
| API Keys | ✅ Secure API access with expiration |
| Audit Logging | ✅ Immutable operation logs with partitioning |
| Anti-Spoofing | ✅ nwfilter MAC, IP, ARP, DHCP, RA spoofing prevention |
| Abuse Prevention | ✅ nftables rules (SMTP block, metadata endpoint block) |
| Row Level Security | ✅ PostgreSQL RLS for customer data isolation |

### Monitoring & Observability

| Feature | Status |
|---------|--------|
| Prometheus Metrics | ✅ Controller (10) and Node Agent (20) metric endpoints |
| Grafana Dashboards | ✅ Pre-built dashboard templates |
| Alerting Rules | ✅ Prometheus alerting configuration |
| Background Collector | ✅ Periodic resource and health data collection |

### Web Interfaces

| Portal | Technology | Pages |
|--------|------------|-------|
| Admin Portal | Next.js 16 + React 19 + shadcn/ui | Dashboard, VMs, Nodes, Customers, Plans, IP Sets, Audit Logs, Settings |
| Customer Portal | Next.js 16 + React 19 + shadcn/ui | VM List, VM Detail (console, metrics), Settings (profile, 2FA, API keys) |

### API System

| Tier | Base Path | Auth | Rate Limit |
|------|-----------|------|------------|
| Admin | `/api/v1/admin/*` | JWT + 2FA | 500/min |
| Customer | `/api/v1/customer/*` | JWT + Refresh | 100 read / 30 write per min |
| Provisioning | `/api/v1/provisioning/*` | API Key | 1000/min |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Load Balancer                        │
│                         (Nginx)                             │
└────────────────────┬────────────────────────────────────────┘
                     │
        ┌────────────┼────────────┐
        │            │            │
   ┌────▼────┐  ┌────▼────┐  ┌────▼────┐
   │  Admin  │  │Customer │  │   API   │
   │  WebUI  │  │ WebUI   │  │         │
   │(Next.js)│  │(Next.js)│  │ (Go)    │
   └────┬────┘  └────┬────┘  └────┬────┘
        │            │            │
        └────────────┼────────────┘
                     │
            ┌────────▼────────┐
            │   Controller    │
            │   (Go/Gin)      │
            └────────┬────────┘
                     │
        ┌────────────┼────────────┐
        │            │            │
   ┌────▼────┐  ┌────▼────┐  ┌────▼────┐
   │PostgreSQL│  │  NATS   │  │ Node    │
   │   16    │  │ JetStream│  │ Agents  │
   └─────────┘  └─────────┘  └────┬────┘
                                  │
                         ┌────────▼────────┐
                         │  Hypervisor     │
                         │   Nodes         │
                         │ (KVM/libvirt)   │
                         └─────────────────┘
```

### Technology Stack

| Layer | Technologies |
|-------|-------------|
| **Backend** | Go 1.25, Gin, gRPC, NATS JetStream, PostgreSQL |
| **Frontend** | Next.js 16, React 19, TypeScript, Tailwind CSS, shadcn/ui |
| **Infrastructure** | KVM/QEMU, Ceph RBD/QCOW2, Docker, Nginx |

---

## Quick Start

### Prerequisites

- Docker 26.0+
- Docker Compose 2.20+
- Make

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/AbuGosok/VirtueStack.git
cd VirtueStack

# Copy and edit environment file
cp .env.example .env

# Start all services
docker compose up -d

# View logs
docker compose logs -f

# Stop
docker compose down
```

### Access Points

| Service | URL | Credentials |
|---------|-----|-------------|
| Admin Portal | https://localhost/admin | admin@virtuestack.local / admin123 |
| Customer Portal | https://localhost | customer@virtuestack.local / customer123 |
| API | https://localhost/api/v1 | JWT or API key |

---

## Development

### Testing Methodology

VirtueStack uses a hybrid testing approach:

- **Docker stack** (Controller, NATS, PostgreSQL, Admin UI, Customer UI, Nginx) — run via `docker compose up -d`. This replicates the production runtime environment.
- **Node Agent** — build and run directly on the host via `make build-node-agent`. The Node Agent requires direct access to the host's KVM/libvirt daemon and is not containerized during testing.

For integration testing, start the Docker stack for the Controller side and run the Node Agent binary separately on a real KVM node.

### Backend

```bash
# Install dependencies
make deps

# Run database migrations
make migrate-up

# Run tests (unit tests)
make test

# Build Node Agent (runs directly on host, not in Docker)
make build-node-agent
```

### Docker Stack

```bash
# Build and start Controller, PostgreSQL, NATS, UIs, Nginx
make docker-build && make docker-up

# View logs
docker compose logs -f

# Stop
docker compose down
```

### Frontend

```bash
# Admin Portal
cd webui/admin && npm install && npm run dev

# Customer Portal
cd webui/customer && npm install && npm run dev
```

---

## Production Deployment

### Prerequisites

Before deploying to production:
- Replace default passwords in docker-compose.yml
- Configure IP allowlisting for provisioning API
- Use CA-signed TLS certificates
- Review firewall rules

### Deployment

```bash
# Production deployment
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

See [docs/INSTALL.md](docs/INSTALL.md) for detailed setup instructions.

---

## Documentation

| Document | Description |
|----------|-------------|
| [AGENTS.md](AGENTS.md) | Technical reference for AI agents and LLM-assisted development |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Detailed architecture specification |
| [docs/INSTALL.md](docs/INSTALL.md) | Installation guide |
| [docs/USAGE.md](docs/USAGE.md) | Usage documentation |
| [docs/API.md](docs/API.md) | API reference |
| [CODING_STANDARD.md](CODING_STANDARD.md) | Quality gates and coding rules |

---

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'Add amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Standards

- Go: Follow [Effective Go](https://golang.org/doc/effective_go.html)
- TypeScript: Use ESLint configuration in the repo
- Tests: Maintain >80% coverage

See [CODING_STANDARD.md](CODING_STANDARD.md) for complete quality gates.

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Support

- 📧 Email: support@virtuestack.com
- 🐛 Issues: [GitHub Issues](https://github.com/AbuGosok/VirtueStack/issues)

---

**For complete technical details, API specifications, and development patterns, see [AGENTS.md](AGENTS.md).**

---

## Project Status

**Overall: 100% Complete**

| Component | Status |
|-----------|--------|
| Controller APIs | 100% |
| Node Agent | 100% |
| Database Schema (24 migrations) | 100% |
| Authentication (JWT, 2FA, API Keys) | 100% |
| VM Lifecycle | 100% |
| Storage (Ceph RBD + QCOW) | 100% |
| Live Migration | 100% |
| Backup & Snapshots | 100% |
| WebSocket Console | 100% |
| Web UIs | 100% |
| WHMCS Module | 100% |
| Networking | 100% |
| HA Failover | 100% |
| PowerDNS rDNS | 100% |
| Monitoring | 100% |
