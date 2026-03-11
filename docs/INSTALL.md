# VirtueStack Installation Guide

This guide covers the complete installation process for VirtueStack Phase 6, a KVM VPS management platform with Go Controller, Next.js Web UIs, WHMCS integration, and Docker Compose deployment.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Prerequisites](#prerequisites)
3. [Detailed Installation](#detailed-installation)
4. [Configuration Reference](#configuration-reference)
5. [Verification Steps](#verification-steps)
6. [Troubleshooting](#troubleshooting)

---

## Quick Start

For experienced users who want to get VirtueStack running quickly:

```bash
# 1. Clone the repository
git clone https://github.com/your-org/virtuestack.git
cd virtuestack

# 2. Copy and configure environment
cp .env.example .env
# Edit .env and set JWT_SECRET, ENCRYPTION_KEY, POSTGRES_PASSWORD

# 3. Generate required secrets
openssl rand -hex 32 >> .env  # For JWT_SECRET
openssl rand -hex 32 >> .env  # For ENCRYPTION_KEY

# 4. Create SSL directory and generate self-signed certificates (for testing)
mkdir -p ssl
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout ssl/key.pem -out ssl/cert.pem \
  -subj "/CN=localhost"

# 5. Deploy with Docker Compose
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# 6. Create first admin user
docker compose exec controller ./virtuestack admin create \
  --email admin@example.com \
  --password "YourSecurePassword123!"

# 7. Access the platforms
# Admin UI: https://your-domain.com/admin
# Customer UI: https://your-domain.com/
```

---

## Prerequisites

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 2 cores | 4+ cores |
| RAM | 4 GB | 8+ GB |
| Disk | 50 GB SSD | 100+ GB SSD |
| OS | Ubuntu 22.04 LTS | Ubuntu 24.04 LTS |

### Required Software

| Software | Version | Purpose |
|----------|---------|---------|
| Docker | 24.0+ | Container runtime |
| Docker Compose | 2.20+ | Container orchestration |
| OpenSSL | 1.1.1+ | Certificate generation |

### Hypervisor Node Requirements (for VM hosting)

- KVM/QEMU with libvirt
- Ceph storage cluster (recommended) or local storage
- Network bridge configured for VM networking
- gRPC access (port 50051) from controller

### Network Requirements

| Port | Service | Purpose |
|------|---------|---------|
| 80 | Nginx | HTTP (redirects to HTTPS) |
| 443 | Nginx | HTTPS (Admin UI, Customer UI, API) |
| 50051 | Node Agent | gRPC (internal, controller ↔ nodes) |

---

## Detailed Installation

### Step 1: Prepare the Host System

```bash
# Update system packages
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# Install Docker Compose (if not included)
sudo apt install docker-compose-plugin -y

# Verify installations
docker --version
docker compose version
```

Log out and log back in for Docker group membership to take effect.

### Step 2: Clone and Prepare VirtueStack

```bash
# Clone repository
git clone https://github.com/your-org/virtuestack.git
cd virtuestack

# Create required directories
mkdir -p ssl backups logs
```

### Step 3: Environment Configuration

Copy the example environment file and configure:

```bash
cp .env.example .env
```

Edit `.env` with your preferred editor:

```bash
# =============================================================================
# CRITICAL: Generate secure secrets for these values
# =============================================================================

# JWT Secret (64 hex characters)
JWT_SECRET=<generate-with: openssl rand -hex 32>

# Encryption Key for sensitive data (64 hex characters)
ENCRYPTION_KEY=<generate-with: openssl rand -hex 32>

# PostgreSQL password
POSTGRES_PASSWORD=<your-secure-password>

# =============================================================================
# Database Configuration
# =============================================================================
POSTGRES_USER=virtuestack
POSTGRES_DB=virtuestack
DATABASE_URL=postgresql://virtuestack:${POSTGRES_PASSWORD}@postgres:5432/virtuestack?sslmode=disable

# =============================================================================
# Service URLs (internal Docker network)
# =============================================================================
NATS_URL=nats://nats:4222
LISTEN_ADDR=:8080

# =============================================================================
# SSL/TLS Configuration
# =============================================================================
SSL_CERT_PATH=./ssl/cert.pem
SSL_KEY_PATH=./ssl/key.pem

# =============================================================================
# Optional: Email Notifications
# =============================================================================
# SMTP_HOST=smtp.example.com
# SMTP_PORT=587
# SMTP_USERNAME=noreply@example.com
# SMTP_PASSWORD=your-smtp-password
# SMTP_FROM=VirtueStack <noreply@example.com>

# =============================================================================
# Optional: Telegram Notifications
# =============================================================================
# TELEGRAM_BOT_TOKEN=123456:ABC-DEF
# TELEGRAM_ADMIN_CHAT_IDS=123456789,987654321

# =============================================================================
# Logging
# =============================================================================
LOG_LEVEL=info
```

### Step 4: SSL Certificate Generation

#### Option A: Let's Encrypt (Production)

```bash
# Install certbot
sudo apt install certbot -y

# Obtain certificate (replace with your domain)
sudo certbot certonly --standalone -d admin.yourdomain.com -d yourdomain.com

# Copy certificates to ssl directory
sudo cp /etc/letsencrypt/live/yourdomain.com/fullchain.pem ssl/cert.pem
sudo cp /etc/letsencrypt/live/yourdomain.com/privkey.pem ssl/key.pem
sudo chown $USER:$USER ssl/*.pem

# Update .env with certificate paths
sed -i 's|SSL_CERT_PATH=.*|SSL_CERT_PATH=/etc/letsencrypt/live/yourdomain.com/fullchain.pem|' .env
sed -i 's|SSL_KEY_PATH=.*|SSL_KEY_PATH=/etc/letsencrypt/live/yourdomain.com/privkey.pem|' .env

# Set up auto-renewal
sudo systemctl enable certbot.timer
sudo systemctl start certbot.timer
```

#### Option B: Self-Signed Certificate (Development/Testing)

```bash
# Generate self-signed certificate
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout ssl/key.pem -out ssl/cert.pem \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=localhost"

# Set appropriate permissions
chmod 600 ssl/key.pem
chmod 644 ssl/cert.pem
```

### Step 5: Deploy VirtueStack

```bash
# Build and start all services
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build

# Monitor the startup
docker compose logs -f

# Wait for all services to be healthy
docker compose ps
```

Expected output after successful deployment:

```
NAME                      STATUS    PORTS
virtuestack-postgres      healthy   5432/tcp
virtuestack-nats          healthy   4222/tcp, 8222/tcp
virtuestack-controller    healthy   8080/tcp
virtuestack-admin-webui   healthy   3000/tcp
virtuestack-customer-webui healthy  3001/tcp
virtuestack-nginx         healthy   0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp
```

### Step 6: Database Initialization

The database is automatically initialized via the migrations mounted in `docker-compose.yml`. Verify migrations ran:

```bash
# Check migration status
docker compose exec postgres psql -U virtuestack -d virtuestack -c "\dt"

# Expected tables:
# customers, vms, nodes, plans, templates, ip_addresses, backups, 
# snapshots, webhook_deliveries, audit_logs, notification_preferences, etc.
```

### Step 7: Create First Admin User

```bash
# Create admin user via CLI
docker compose exec controller ./virtuestack admin create \
  --email admin@yourdomain.com \
  --password "YourSecurePassword123!" \
  --name "System Administrator"

# Or via API
curl -X POST https://localhost/api/v1/admin/setup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@yourdomain.com",
    "password": "YourSecurePassword123!",
    "name": "System Administrator"
  }'
```

### Step 8: WHMCS Module Installation

If integrating with WHMCS:

```bash
# Copy WHMCS module to your WHMCS installation
cp -r modules/whmcs/virtuestack /path/to/whmcs/modules/servers/

# Set permissions
chown -R www-data:www-data /path/to/whmcs/modules/servers/virtuestack

# Configure in WHMCS Admin:
# 1. Go to Setup > Products/Services > Servers
# 2. Add New Server
# 3. Type: VirtueStack
# 4. Hostname: your-virtuestack-domain.com
# 5. API Key: <generate via VirtueStack admin panel>
```

Generate a provisioning API key:

```bash
# Via CLI
docker compose exec controller ./virtuestack provisioning-key create \
  --name "WHMCS Production" \
  --allowed-ips "1.2.3.4,5.6.7.8"

# Via Admin UI
# Navigate to Settings > API Keys > Create Provisioning Key
```

---

## Configuration Reference

### Environment Variables

#### Core Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JWT_SECRET` | Yes | - | Secret for signing JWT tokens (64 hex chars) |
| `ENCRYPTION_KEY` | Yes | - | Key for encrypting sensitive data (64 hex chars) |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `NATS_URL` | Yes | `nats://localhost:4222` | NATS JetStream URL |
| `LISTEN_ADDR` | No | `:8080` | Controller API listen address |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |

#### Database Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `POSTGRES_USER` | No | `virtuestack` | PostgreSQL username |
| `POSTGRES_PASSWORD` | Yes | - | PostgreSQL password |
| `POSTGRES_DB` | No | `virtuestack` | Database name |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |

#### SSL/TLS Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SSL_CERT_PATH` | No | `./ssl/cert.pem` | Path to SSL certificate |
| `SSL_KEY_PATH` | No | `./ssl/key.pem` | Path to SSL private key |

#### Email Notifications

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SMTP_HOST` | No | - | SMTP server hostname |
| `SMTP_PORT` | No | `587` | SMTP server port |
| `SMTP_USERNAME` | No | - | SMTP authentication username |
| `SMTP_PASSWORD` | No | - | SMTP authentication password |
| `SMTP_FROM` | No | - | From address for emails |

#### Telegram Notifications

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | No | - | Telegram bot token |
| `TELEGRAM_ADMIN_CHAT_IDS` | No | - | Comma-separated admin chat IDs |

#### Node Agent Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CONTROLLER_GRPC_ADDR` | Yes | - | Controller gRPC address |
| `NODE_ID` | Yes | - | Unique node identifier (UUID) |
| `CEPH_POOL` | No | `vs-vms` | Ceph pool name |
| `CEPH_USER` | No | `virtuestack` | Ceph username |
| `CEPH_CONF` | No | `/etc/ceph/ceph.conf` | Ceph configuration path |
| `CLOUDINIT_PATH` | No | `/var/lib/virtuestack/cloud-init` | Cloud-init files path |
| `ISO_STORAGE_PATH` | No | `/var/lib/virtuestack/iso` | ISO storage path |

### Docker Compose Services

| Service | Description | Port |
|---------|-------------|------|
| `postgres` | PostgreSQL 16 database | 5432 (internal) |
| `nats` | NATS JetStream message queue | 4222 (internal) |
| `controller` | Go API controller | 8080 (internal) |
| `admin-webui` | Admin dashboard (Next.js) | 3000 (internal) |
| `customer-webui` | Customer portal (Next.js) | 3001 (internal) |
| `nginx` | Reverse proxy | 80, 443 (external) |

### Nginx Configuration

The default nginx configuration proxies requests as follows:

| URL Path | Service | Purpose |
|----------|---------|---------|
| `/admin` | admin-webui:3000 | Admin dashboard |
| `/` | customer-webui:3001 | Customer portal |
| `/api/v1/admin` | controller:8080 | Admin API |
| `/api/v1/customer` | controller:8080 | Customer API |
| `/api/v1/provisioning` | controller:8080 | WHMCS provisioning API |

---

## Verification Steps

### 1. Service Health Checks

```bash
# Check all services are running and healthy
docker compose ps

# Verify controller health endpoint
curl -k https://localhost/api/v1/health
# Expected: {"status":"healthy"}

# Check database connectivity
docker compose exec controller ./virtuestack db ping
```

### 2. Admin UI Access

1. Navigate to `https://your-domain.com/admin`
2. Log in with your admin credentials
3. Verify the dashboard loads with no errors

### 3. Customer UI Access

1. Navigate to `https://your-domain.com/`
2. Verify the login page displays
3. Create a test customer via Admin UI

### 4. API Verification

```bash
# Test admin login API
curl -k -X POST https://localhost/api/v1/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@yourdomain.com","password":"YourSecurePassword123!"}'

# Expected response:
# {"data":{"access_token":"...","refresh_token":"...","token_type":"Bearer","expires_in":3600}}
```

### 5. Node Registration

If you have hypervisor nodes ready:

```bash
# Register a node via API
TOKEN="<your-admin-token>"
curl -k -X POST https://localhost/api/v1/admin/nodes \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "node1.example.com",
    "grpc_address": "node1.example.com:50051",
    "management_ip": "192.168.1.10",
    "total_vcpu": 32,
    "total_memory_mb": 65536,
    "ceph_pool": "vs-vms"
  }'
```

### 6. Create Test Plan and Template

Before provisioning VMs, create at least one plan and template:

```bash
# Create a plan
curl -k -X POST https://localhost/api/v1/admin/plans \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Starter VPS",
    "slug": "starter-vps",
    "vcpu": 1,
    "memory_mb": 1024,
    "disk_gb": 20,
    "bandwidth_limit_gb": 1000,
    "port_speed_mbps": 1000,
    "price_monthly": 5.00,
    "is_active": true
  }'

# Create a template
curl -k -X POST https://localhost/api/v1/admin/templates \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Ubuntu 22.04 LTS",
    "slug": "ubuntu-22-04",
    "os_type": "linux",
    "image_path": "ubuntu-22.04.qcow2",
    "min_disk_gb": 10,
    "is_active": true
  }'
```

---

## Troubleshooting

### Common Issues

#### Container fails to start

```bash
# Check container logs
docker compose logs controller

# Common causes:
# 1. Missing environment variables
# 2. Database not ready
# 3. Invalid SSL certificates
```

#### Database connection errors

```bash
# Verify PostgreSQL is running
docker compose exec postgres pg_isready

# Check database logs
docker compose logs postgres

# Reset database (WARNING: destroys all data)
docker compose down -v
docker compose up -d
```

#### SSL certificate errors

```bash
# Verify certificates exist
ls -la ssl/

# Check certificate validity
openssl x509 -in ssl/cert.pem -text -noout

# Regenerate self-signed certificate
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout ssl/key.pem -out ssl/cert.pem \
  -subj "/CN=localhost"
```

#### JWT authentication failures

```bash
# Verify JWT_SECRET is set
docker compose exec controller env | grep JWT_SECRET

# Ensure JWT_SECRET is 64 hex characters
echo $JWT_SECRET | wc -c
```

#### NATS connection issues

```bash
# Check NATS health
docker compose exec nats wget -qO- http://localhost:8222/healthz

# NATS monitoring endpoint
docker compose exec nats wget -qO- http://localhost:8222/varz
```

### Log Locations

| Log | Location |
|-----|----------|
| Controller | `docker compose logs controller` |
| PostgreSQL | `docker compose logs postgres` |
| NATS | `docker compose logs nats` |
| Nginx | `docker compose logs nginx` or `/var/log/nginx/` |
| Admin WebUI | `docker compose logs admin-webui` |
| Customer WebUI | `docker compose logs customer-webui` |

### Resetting the Installation

```bash
# Stop all services
docker compose down

# Remove volumes (WARNING: destroys all data)
docker compose down -v

# Rebuild from scratch
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build
```

### Getting Help

1. Check logs for error messages
2. Review the [API documentation](./API.md) for correct request formats
3. Verify environment variables are correctly set
4. Ensure network connectivity between services
5. Open an issue on GitHub with:
   - Full error message
   - Relevant log output
   - Environment configuration (redact secrets)

---

## Next Steps

After successful installation:

1. **Configure Nodes**: Register your hypervisor nodes via Admin UI
2. **Create Plans**: Define VPS plans with resource allocations
3. **Upload Templates**: Add OS templates for VM provisioning
4. **Configure IP Pools**: Set up IP address pools for VM assignment
5. **WHMCS Integration**: Configure the WHMCS module if using billing integration
6. **Set Up Notifications**: Configure email/Telegram notifications
7. **Review Security**: Enable 2FA for admin accounts, review firewall rules

For usage instructions, see the [Usage Guide](./USAGE.md).