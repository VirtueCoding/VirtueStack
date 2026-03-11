# VirtueStack

**A modern, scalable Virtual Machine management platform built for VPS hosting providers.**

[![Go Version](https://img.shields.io/badge/Go-1.23-blue)](https://golang.org/)
[![React](https://img.shields.io/badge/React-19-blue)](https://reactjs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-blue)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

VirtueStack provides a complete infrastructure-as-a-service platform for managing virtual machines across a distributed cluster of compute nodes. Built with Go, React/Next.js, PostgreSQL, and NATS JetStream.

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Development](#development)
- [Production Deployment](#production-deployment)
- [Documentation](#documentation)
- [Project Status](#project-status)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

VirtueStack is a cloud-native VM management platform designed for VPS hosting providers. It provides:

- **Multi-tenant Architecture** - Separate admin and customer portals with role-based access control
- **Distributed Compute** - Scale across multiple hypervisor nodes with automatic load balancing
- **Real-time Monitoring** - Track VM metrics, bandwidth usage, and node health
- **Automated Backups** - Scheduled backups with point-in-time snapshot recovery
- **API Access** - Full REST API for integration with billing systems (WHMCS, etc.)

---

## Features

### Core Platform

| Feature | Status | Notes |
|---------|--------|-------|
| VM Lifecycle Management | ✅ | Create, start, stop, restart VMs |
| Live Migration | ⚠️ | API stubbed, needs implementation |
| Node Failover | ⚠️ | Detection works, auto-recovery TODO |
| Bandwidth Monitoring | ✅ | Real-time tracking with usage graphs |
| Backup & Snapshots | ⚠️ | Backend ready, frontend placeholders |
| Console Access | ⚠️ | VNC placeholder in UI |
| API Key Management | ⚠️ | UI exists, backend stubbed |

### Authentication & Security

| Feature | Status | Notes |
|---------|--------|-------|
| JWT Authentication | ⚠️ | Frontend mocked, needs backend integration |
| 2FA/TOTP | ✅ | Backend implemented |
| RBAC | ✅ | Role-based access control |
| Password Reset | ⚠️ | Table exists, workflow TODO |
| API Authentication | ✅ | Token-based auth |

### Web Interfaces

| Portal | Framework | Status |
|--------|-----------|--------|
| Admin Portal | Next.js 15 + shadcn/ui | UI ready, API integration needed |
| Customer Portal | Next.js 15 + shadcn/ui | UI ready, API integration needed |

**See [CODEBASE_AUDIT_REPORT.md](docs/CODEBASE_AUDIT_REPORT.md) for detailed status.**

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

### Components

| Component | Technology | Purpose |
|-----------|------------|---------|
| **Controller** | Go 1.23 + Gin | API server, business logic, orchestration |
| **Node Agent** | Go 1.23 + gRPC | Runs on each hypervisor, manages VMs |
| **Admin WebUI** | Next.js 15 + React 19 | Administration interface |
| **Customer WebUI** | Next.js 15 + React 19 | Customer self-service portal |
| **Database** | PostgreSQL 16 | Primary data store |
| **Message Queue** | NATS JetStream | Async tasks, events |
| **Reverse Proxy** | Nginx 1.25 | TLS termination, routing |

---

## Quick Start

### Prerequisites

- Docker 24.0+
- Docker Compose 2.20+
- Make
- Go 1.23+ (for local development)
- Node.js 20+ (for frontend development)

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/AbuGosok/VirtueStack.git
cd VirtueStack

# Copy environment file
cp .env.example .env
# Edit .env with your configuration

# Start all services
docker compose up -d

# View logs
docker compose logs -f controller
docker compose logs -f admin-webui
docker compose logs -f customer-webui

# Stop
docker compose down
```

### Access Points

| Service | URL | Default Credentials |
|---------|-----|---------------------|
| Admin Portal | http://localhost | See .env configuration |
| Customer Portal | http://localhost | Customer signup |
| API | http://localhost/api/v1 | Token-based |
| NATS Monitoring | http://localhost:8222 | None |

---

## Development

### Backend Development

```bash
# Install dependencies
make deps

# Run database migrations
make migrate-up

# Run tests
make test

# Run with hot reload (requires Air)
make dev

# Build
make build
```

### Frontend Development

```bash
# Admin Portal
cd webui/admin
npm install
npm run dev

# Customer Portal
cd webui/customer
npm install
npm run dev
```

### Database Migrations

```bash
# Create new migration
make migrate-create name=add_feature

# Apply migrations
make migrate-up

# Rollback
make migrate-down
```

---

## Production Deployment

### Security Checklist

⚠️ **CRITICAL**: Review [CODEBASE_AUDIT_REPORT.md](docs/CODEBASE_AUDIT_REPORT.md) before production deployment.

Required fixes:
- [ ] Replace insecure password hashing (Argon2id recommended)
- [ ] Implement JWT authentication (currently mocked)
- [ ] Remove default passwords from docker-compose.yml
- [ ] Secure provisioning API endpoints
- [ ] Configure TLS certificates

### Production Deployment

```bash
# Use production compose file
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for detailed production setup.

---

## Documentation

| Document | Description |
|----------|-------------|
| [CODEBASE_AUDIT_REPORT.md](docs/CODEBASE_AUDIT_REPORT.md) | Comprehensive audit of unfinished work |
| [API.md](docs/API.md) | API reference documentation |
| [INSTALL.md](docs/INSTALL.md) | Installation guide |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | System architecture details |
| [MASTER_CODING_STANDARD_V2.md](docs/MASTER_CODING_STANDARD_V2.md) | Coding standards |

---

## Project Status

**Current State:** ~50-60% Complete

### What's Working

✅ Database schema and migrations  
✅ Basic VM CRUD operations  
✅ Node management and registration  
✅ JWT token generation/validation (backend)  
✅ 2FA/TOTP support  
✅ Bandwidth tracking  
✅ Customer/plan management  
✅ Audit logging  

### What's Not Ready

⚠️ Authentication frontend integration  
⚠️ VM live migration  
⚠️ Automatic node failover  
⚠️ Console access (VNC)  
⚠️ Backup/snapshot UI  
⚠️ API key management  
⚠️ Password reset workflow  

### Estimated Timeline

- **Minimum viable:** 2-3 weeks (security fixes only)
- **Production ready:** 8-10 weeks (full feature set)

See [CODEBASE_AUDIT_REPORT.md](docs/CODEBASE_AUDIT_REPORT.md) for detailed breakdown.

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

- Go: Follow [Effective Go](https://golang.org/doc/effective_go.html) and [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- TypeScript: Use ESLint configuration in the repo
- Tests: Maintain >80% coverage
- Documentation: Update relevant docs for API changes

---

## Technology Stack

### Backend

| Technology | Purpose |
|------------|---------|
| Go 1.23 | Primary language |
| Gin | HTTP web framework |
| pgx | PostgreSQL driver |
| NATS | Message queue |
| gRPC | Node agent communication |
| libvirt-go | VM management |
| JWT | Authentication |
| Argon2id | Password hashing |

### Frontend

| Technology | Purpose |
|------------|---------|
| Next.js 15 | React framework |
| React 19 | UI library |
| TypeScript | Type safety |
| shadcn/ui | Component library |
| Tailwind CSS | Styling |
| TanStack Query | Data fetching |

### Infrastructure

| Technology | Purpose |
|------------|---------|
| PostgreSQL 16 | Database |
| NATS JetStream | Message queue |
| Nginx 1.25 | Reverse proxy |
| Docker | Containerization |
| Ceph | Distributed storage |

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Support

- 📧 Email: support@virtuestack.com
- 💬 Discord: [Join our community](https://discord.gg/virtuestack)
- 🐛 Issues: [GitHub Issues](https://github.com/AbuGosok/VirtueStack/issues)

---

## Acknowledgments

- Built with ❤️ by the VirtueStack team
- Thanks to all contributors who have helped shape this project
- Inspired by industry-leading cloud platforms

---

**⚠️ IMPORTANT:** This codebase is under active development. See [CODEBASE_AUDIT_REPORT.md](docs/CODEBASE_AUDIT_REPORT.md) before production use.
