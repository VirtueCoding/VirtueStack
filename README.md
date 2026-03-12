# VirtueStack

**A modern, scalable Virtual Machine management platform built for VPS hosting providers.**

[![Go Version](https://img.shields.io/badge/Go-1.24-blue)](https://golang.org/)
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
| JWT Authentication | ✅ | Backend fully implemented, E2E verified (admin, customer, provisioning) |
| 2FA/TOTP | ✅ | Fully implemented and E2E verified (admin login flow) |
| RBAC | ✅ | Role-based access control |
| Password Reset | ⚠️ | Table exists, workflow TODO |
| API Authentication | ✅ | Token-based auth |

### Web Interfaces

| Portal | Framework | Status |
|--------|-----------|--------|
| Admin Portal | Next.js 15 + shadcn/ui | UI ready, backend API verified |
| Customer Portal | Next.js 15 + shadcn/ui | UI ready, backend API verified |

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
| **Controller** | Go 1.24 + Gin | API server, business logic, orchestration |
| **Node Agent** | Go 1.24 + gRPC | Runs on each hypervisor, manages VMs |
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
- Go 1.24+ (for local development)
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
| Admin Portal | https://localhost/admin | admin@virtuestack.local / admin123 (2FA enabled) |
| Customer Portal | https://localhost | customer@virtuestack.local / customer123 |
| API | https://localhost/api/v1 | JWT or API key |
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

Required fixes before production:
- [x] Password hashing with Argon2id — implemented and verified
- [x] JWT authentication — fully implemented for admin, customer, and provisioning APIs
- [x] TLS certificates — configured and working (self-signed for dev, replace for production)
- [ ] Remove default passwords from docker-compose.yml
- [ ] Secure provisioning API endpoints with IP allowlisting
- [ ] Replace self-signed certificates with proper CA-signed certs

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

**Current State:** ~65-70% Complete (backend core verified, frontend integration pending)

### What's Working (E2E Verified)

✅ Database schema and migrations (15 migration files, 52 tables)  
✅ All three API groups: Admin, Customer, Provisioning  
✅ JWT authentication (admin + customer login flows)  
✅ 2FA/TOTP (admin login with TOTP verification)  
✅ Provisioning API key authentication  
✅ Role-based access control (admin, super_admin, customer)  
✅ Password hashing with Argon2id  
✅ Basic VM CRUD operations  
✅ Node management and registration  
✅ Bandwidth tracking  
✅ Customer/plan management  
✅ Audit logging  
✅ Docker Compose deployment (6 containers, all healthy)  
✅ TLS/HTTPS via Nginx reverse proxy  

### What's Not Ready

⚠️ Frontend ↔ Backend API wiring (UIs built, not connected to live API)  
⚠️ VM live migration  
⚠️ Automatic node failover  
⚠️ Console access (VNC/noVNC)  
⚠️ Backup/snapshot UI integration  
⚠️ API key management UI  
⚠️ Password reset workflow  

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
| Go 1.24 | Primary language |
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
