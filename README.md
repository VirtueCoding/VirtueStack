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

---

## Features

### Core Platform

| Feature | Status |
|---------|--------|
| VM Lifecycle Management | ✅ Create, start, stop, restart, migrate VMs |
| Live Migration | ✅ Zero-downtime migration between nodes |
| Storage Management | ✅ Ceph RBD + QCOW2 dual backend |
| Bandwidth Monitoring | ✅ Real-time tracking with limits |
| Backup & Snapshots | ✅ Automated backups and recovery |
| Console Access | ✅ Web-based VNC and Serial consoles |
| Node Failover | 🚧 In progress (70% complete) |
| DNS Management | 🚧 In progress (PowerDNS integration) |

### Security & Authentication

| Feature | Status |
|---------|--------|
| Multi-factor Authentication | ✅ TOTP/2FA with backup codes |
| JWT Authentication | ✅ Secure token-based sessions |
| RBAC | ✅ Role-based permissions |
| API Keys | ✅ Secure API access |
| Audit Logging | ✅ Immutable operation logs |

### Web Interfaces

| Portal | Technology |
|--------|------------|
| Admin Portal | Next.js 16 + React 19 + shadcn/ui |
| Customer Portal | Next.js 16 + React 19 + shadcn/ui |

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

### Backend

```bash
# Install dependencies
make deps

# Run database migrations
make migrate-up

# Run tests
make test

# Build
make build
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
| [AGENTS.md](AGENTS.md) | **Complete technical reference** for developers and AI agents |
| [docs/INSTALL.md](docs/INSTALL.md) | Installation guide |
| [docs/USAGE.md](docs/USAGE.md) | Usage documentation |
| [docs/API.md](docs/API.md) | API reference |

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
