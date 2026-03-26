# VirtueStack Installation Guide

**Version:** 2.3 — March 2026

This guide covers installation for both production deployments and test/development environments.

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Production Deployment](#2-production-deployment)
   - [Controller Stack (Docker)](#21-controller-stack-docker)
   - [Node Agent (KVM Host)](#22-node-agent-kvm-host)
   - [mTLS Certificate Setup](#23-mtls-certificate-setup)
   - [Database Migrations](#24-database-migrations)
   - [SSL/TLS for Web Access](#25-ssltls-for-web-access)
3. [Development/Test Environment](#3-developmenttest-environment)
   - [Quick Start](#31-quick-start)
   - [E2E Testing Setup](#32-e2e-testing-setup)
   - [Test Data and Credentials](#33-test-data-and-credentials)
4. [Configuration Reference](#4-configuration-reference)
5. [Verification](#5-verification)
6. [Troubleshooting](#6-troubleshooting)

---

## 1. Prerequisites

### Controller Stack (Docker Host)

| Requirement | Version | Notes |
|-------------|---------|-------|
| Docker Engine | 26+ | Container runtime |
| Docker Compose | 2.x | Container orchestration |
| Go | 1.26+ | For building from source |
| Make | Any | Build automation |
| OpenSSL | 1.1.1+ | Certificate generation |

### Node Agent (KVM Host)

| Requirement | Version | Notes |
|-------------|---------|-------|
| Ubuntu Server | 24.04 LTS | Recommended OS |
| KVM/QEMU | 8.x | Hardware virtualization |
| libvirt | 10.x | VM management |
| Go | 1.26+ | For building from source |
| Ceph client | Reef (18.x) or Squid (19.x) | Only for Ceph backend |
| qemu-utils | 8.x | For QCOW2 operations |
| cloud-image-utils | Any | Cloud-init ISO generation |
| dnsmasq | 2.x | DHCP/DNS for VMs |

**Node Agent Binary Dependencies:**

The `node-agent` binary is dynamically linked and requires these shared libraries at runtime:

| Library | Package | Purpose |
|---------|---------|---------|
| `libvirt.so.0` | `libvirt0` | KVM/QEMU VM management |
| `libvirt-qemu.so.0` | `libvirt0` | QEMU-specific APIs |
| `librbd.so.1` | `librbd1` | Ceph RBD block storage |
| `librados.so.2` | `librados2` | Ceph RADOS client |

**Minimum installation for binary-only deployment:**
```bash
sudo apt install -y libvirt0 librbd1 librados2
```

> **Note:** The node-agent uses CGO to interface with libvirt and Ceph. These dependencies cannot be statically linked. Full hypervisor nodes typically already have these libraries installed as part of KVM/libvirt setup.

### Hardware Requirements

**Controller Stack:**
- CPU: 2 cores minimum
- RAM: 4GB minimum (8GB recommended)
- Storage: 20GB minimum

**KVM Node:**
- CPU: Hardware virtualization support (VT-x/AMD-V)
- RAM: 32GB+ recommended (depends on VM density)
- Storage: Depends on VM disk requirements
- Network: 1Gbps+ recommended

---

## 2. Production Deployment

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Docker Stack (Controller Side)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │PostgreSQL│  │   NATS   │  │Controller│  │ WebUIs + Nginx   │ │
│  │   :5432  │  │  :4222   │  │  :8080   │  │  :3000/:3001     │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                     │ gRPC (mTLS)
                                     ▼
┌─────────────────────────────────────────────────────────────────┐
│              KVM Hypervisor Node (Node Agent Side)               │
│  ┌──────────────┐  ┌──────────┐  ┌──────────────────────────┐   │
│  │  Node Agent  │  │  libvirt │  │  VMs (QEMU/KVM)          │   │
│  │   :50051     │  │  daemon  │  │  + Ceph/QCOW storage     │   │
│  └──────────────┘  └──────────┘  └──────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### 2.1 Controller Stack (Docker)

#### Step 1: Clone the Repository

```bash
git clone https://github.com/your-org/virtuestack.git
cd virtuestack
```

#### Step 2: Create Environment File

Create `.env` file (use strong secrets in production):

```bash
# Generate strong secrets
JWT_SECRET=$(openssl rand -base64 48)
ENCRYPTION_KEY=$(openssl rand -base64 32)
NATS_AUTH_TOKEN=$(openssl rand -base64 32)
POSTGRES_PASSWORD=$(openssl rand -base64 24)

cat > .env << EOF
# PostgreSQL
POSTGRES_USER=virtuestack
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=virtuestack

# NATS JetStream
NATS_AUTH_TOKEN=${NATS_AUTH_TOKEN}

# JWT Configuration (32+ characters required)
JWT_SECRET=${JWT_SECRET}

# Encryption Key (32 bytes for AES-256-GCM)
ENCRYPTION_KEY=${ENCRYPTION_KEY}

# Logging
LOG_LEVEL=info

# WebUI Configuration
NEXT_PUBLIC_API_URL=/api/v1

# SSL Certificates (update paths for production)
SSL_CERT_PATH=/etc/ssl/virtuestack/cert.pem
SSL_KEY_PATH=/etc/ssl/virtuestack/key.pem

# Docker network (optional, for isolation)
DOCKER_NETWORK_SUBNET=172.20.0.0/24
EOF

chmod 600 .env
```

#### Step 3: Generate SSL Certificates

**Option A: Let's Encrypt (Recommended for Production)**

```bash
# Install certbot
sudo apt install certbot

# Obtain certificate
sudo certbot certonly --standalone -d admin.yourdomain.com -d customer.yourdomain.com

# Copy certificates
sudo cp /etc/letsencrypt/live/yourdomain.com/fullchain.pem ./ssl/cert.pem
sudo cp /etc/letsencrypt/live/yourdomain.com/privkey.pem ./ssl/key.pem
sudo chown $USER:$USER ./ssl/*.pem
```

**Option B: Self-Signed (Development Only)**

```bash
mkdir -p ssl
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout ssl/key.pem \
    -out ssl/cert.pem \
    -subj "/C=US/ST=State/L=City/O=VirtueStack/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,DNS:*.localhost,IP:127.0.0.1"
```

#### Step 4: Build Docker Images

```bash
make docker-build
# Or manually:
docker compose build
```

#### Step 5: Start Services

```bash
make docker-up
# Or:
docker compose up -d
```

#### Step 6: Run Database Migrations

```bash
# Install migrate tool (if not installed)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Run migrations
make migrate-up
# Or with explicit URL:
migrate -path migrations -database "postgresql://virtuestack:${POSTGRES_PASSWORD}@localhost:5432/virtuestack?sslmode=disable" up
```

#### Step 7: Create Initial Admin User

```bash
# Connect to the database
docker exec -it virtuestack-postgres psql -U virtuestack -d virtuestack

# Insert admin user (password: change this in production!)
INSERT INTO admins (id, email, password_hash, role, status, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    'admin@yourdomain.com',
    '$argon2id$v=19$m=65536,t=3,p=4$your-salt-here$your-hash-here',
    'super_admin',
    'active',
    NOW(),
    NOW()
);
```

For production, generate a proper Argon2id hash using the application's password hashing utility.

### 2.2 Node Agent (KVM Host)

#### Step 1: Prepare KVM Host

```bash
# Install KVM and libvirt
sudo apt update
sudo apt install -y qemu-kvm libvirt-daemon-system libvirt-clients bridge-utils virtinst

# Enable and start libvirt
sudo systemctl enable --now libvirtd

# Add user to libvirt group
sudo usermod -aG libvirt $(whoami)

# Install additional dependencies
sudo apt install -y qemu-utils cloud-image-utils dnsmasq

# For Ceph backend, install ceph-common
sudo apt install -y ceph-common

# For LVM backend, install lvm2 (usually pre-installed)
sudo apt install -y lvm2

# Verify node-agent binary dependencies (run after copying binary)
# This will show all required shared libraries
ldd /usr/local/bin/virtuestack-node-agent
# If any libraries show "not found", install the missing packages above
```

#### Step 2: Configure Network Bridge

```bash
# Create bridge configuration
sudo cat > /etc/netplan/99-virtuestack-bridge.yaml << 'EOF'
network:
  version: 2
  renderer: networkd
  ethernets:
    eth0:
      dhcp4: no
  bridges:
    br0:
      interfaces: [eth0]
      addresses: [192.168.1.100/24]  # Update for your network
      routes:
        - to: default
          via: 192.168.1.1
      nameservers:
        addresses: [8.8.8.8, 8.8.4.4]
      parameters:
        stp: true
        forward-delay: 4
EOF

sudo netplan apply
```

#### Step 3: Configure libvirt nwfilter

```bash
# Create anti-spoofing nwfilter
sudo virsh nwfilter-define - << 'EOF'
<filter name='virtuestack-clean-traffic' chain='root'>
  <uuid>aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee</uuid>
  <filterref filter='no-mac-spoofing'/>
  <filterref filter='no-ip-spoofing'/>
  <filterref filter='no-arp-spoofing'/>
  <filterref filter='allow-dhcp'/>
  <rule action='drop' direction='out' priority='500'>
    <ip match='no' srcipaddr='$IP'/>
  </rule>
  <rule action='drop' direction='out' priority='500'>
    <ipv6 match='no' srcipaddr='$IPV6'/>
  </rule>
</filter>
EOF
```

#### Step 3.5: Configure LVM Thin Pool (LVM Backend Only)

If using the LVM storage backend, create a volume group and thin pool before deploying VMs.

**Prerequisites:**
- A dedicated block device or partition (e.g., `/dev/sdb`, `/dev/nvme0n1`)
- The `lvm2` package installed (`sudo apt install -y lvm2`)

**Create Volume Group and Thin Pool:**

```bash
# 1. Create physical volume (replace /dev/sdb with your device)
sudo pvcreate /dev/sdb

# 2. Create volume group (replace vgvs with your preferred name)
sudo vgcreate vgvs /dev/sdb

# 3. Create thin pool LV
# Recommended size: total planned VM virtual disk GiB × 0.3 as starting point
# Example: For 1TB total VM storage, create ~300GB thin pool
# The pool will expand with overprovisioning; monitor usage carefully.
sudo lvcreate -L 300G -T vgvs/thinpool

# 4. Verify thin pool
sudo lvs -a vgvs
# Output should show:
#   LV        VG   Attr       LSize   Pool Origin Data%  Meta%
#   thinpool  vgvs twi-a-tz-- 300.00g              0.00   0.00
```

**Configuration:**

Add to `/etc/virtuestack/node-agent.env`:

```bash
STORAGE_BACKEND=lvm
LVM_VOLUME_GROUP=vgvs
LVM_THIN_POOL=thinpool
```

**Monitoring Thresholds:**

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| `data_percent` | >= 90% | >= 95% | VM creation blocked at 95% |
| `metadata_percent` | >= 60% | >= 70% | VM creation blocked at 70% |

Check current usage:

```bash
sudo lvs -o lv_name,vg_name,lv_attr,lv_size,data_percent,metadata_percent vgvs
```

**Overprovisioning Risk:**

Thin pools allow overprovisioning (total virtual size > physical size). If the pool fills:
- VMs may freeze or corrupt on write
- Recovery requires adding physical space or deleting LVs

**Best Practices:**
- Monitor pool usage daily via `lvs` or Prometheus metrics
- Set up alerts for `data_percent >= 90%`
- Reserve 10-20% headroom for metadata and snapshots
- Consider thick provisioning for critical workloads

**Guest TRIM Requirement:**

VirtueStack configures `discard='unmap'` on LVM disks, allowing guests to release unused blocks. Guests must be configured to issue TRIM commands:

| OS | Configuration |
|----|---------------|
| **Linux** | Enable `fstrim.timer`: `sudo systemctl enable --now fstrim.timer` |
| **Windows** | Run as Administrator: `fsutil behavior set DisableDeleteNotify 0` |

For Linux VMs, cloud-init can configure TRIM automatically during first boot by including this in user-data:

```yaml
#cloud-config
runcmd:
  - systemctl enable fstrim.timer
  - systemctl start fstrim.timer
```

#### Step 4: Build Node Agent Binary

On a build machine (or the KVM host):

```bash
# Clone repository
git clone https://github.com/your-org/virtuestack.git
cd virtuestack

# Build
make build-node-agent

# The binary is at: bin/node-agent
```

#### Step 5: Install Node Agent

```bash
# Copy binary to KVM host
scp bin/node-agent user@kvm-host:/usr/local/bin/virtuestack-node-agent

# Create configuration directory
sudo mkdir -p /etc/virtuestack
sudo mkdir -p /var/lib/virtuestack/{vms,templates,cloud-init,iso}

# Create systemd service
sudo cat > /etc/systemd/system/virtuestack-node-agent.service << 'EOF'
[Unit]
Description=VirtueStack Node Agent
After=network.target libvirtd.service
Wants=libvirtd.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/virtuestack-node-agent
Restart=always
RestartSec=5

# Environment variables
EnvironmentFile=/etc/virtuestack/node-agent.env

[Install]
WantedBy=multi-user.target
EOF
```

#### Step 6: Configure Node Agent

Create `/etc/virtuestack/node-agent.env`:

```bash
# Controller gRPC address
CONTROLLER_GRPC_ADDR=controller.yourdomain.com:50051

# Node identifier (unique per node)
NODE_ID=your-node-uuid-here

# Storage backend: "ceph" or "qcow"
STORAGE_BACKEND=ceph

# Ceph configuration (if using Ceph)
CEPH_POOL=vs-vms
CEPH_USER=virtuestack
CEPH_CONF=/etc/ceph/ceph.conf
CEPH_KEYRING=/etc/ceph/ceph.client.virtuestack.keyring

# QCOW2 paths (if using QCOW2)
STORAGE_PATH=/var/lib/virtuestack/vms
TEMPLATE_PATH=/var/lib/virtuestack/templates

# mTLS certificates
TLS_CERT_FILE=/etc/virtuestack/certs/node-agent.crt
TLS_KEY_FILE=/etc/virtuestack/certs/node-agent.key
TLS_CA_FILE=/etc/virtuestack/certs/ca.crt

# Logging
LOG_LEVEL=info
```

#### Step 7: Register Node in Database

```bash
# On the controller, register the node
psql -h localhost -U virtuestack -d virtuestack << EOF
INSERT INTO nodes (id, hostname, grpc_address, management_ip, status, storage_backend, location_id, created_at, updated_at)
VALUES (
    'generated-uuid',
    'node-1.yourdomain.com',
    'node-1.yourdomain.com:50051',
    '192.168.1.100',
    'online',
    'ceph',
    'location-uuid',
    NOW(),
    NOW()
);
EOF
```

#### Step 8: Start Node Agent

```bash
sudo systemctl daemon-reload
sudo systemctl enable virtuestack-node-agent
sudo systemctl start virtuestack-node-agent

# Check logs
sudo journalctl -u virtuestack-node-agent -f
```

### 2.3 mTLS Certificate Setup

The Controller and Node Agents communicate via gRPC with mutual TLS authentication.

#### Generate CA and Certificates

```bash
# Create certs directory
mkdir -p certs
cd certs

# Generate CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
    -subj "/C=US/ST=State/L=City/O=VirtueStack/OU=CA/CN=VirtueStack CA"

# Generate Controller certificate
openssl genrsa -out controller.key 4096
openssl req -new -key controller.key -out controller.csr \
    -subj "/C=US/ST=State/L=City/O=VirtueStack/OU=Controller/CN=controller"
openssl x509 -req -days 365 -in controller.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out controller.crt \
    -extfile <(echo "subjectAltName=DNS:controller,DNS:localhost,IP:127.0.0.1")

# Generate Node Agent certificate (repeat for each node)
openssl genrsa -out node-agent.key 4096
openssl req -new -key node-agent.key -out node-agent.csr \
    -subj "/C=US/ST=State/L=City/O=VirtueStack/OU=Node Agent/CN=node-1"
openssl x509 -req -days 365 -in node-agent.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out node-agent.crt \
    -extfile <(echo "subjectAltName=DNS:node-1,DNS:localhost,IP:192.168.1.100")

# Set permissions
chmod 600 *.key
chmod 644 *.crt
```

#### Distribute Certificates

**Controller:**
```bash
# On the Docker host
cp certs/controller.key certs/controller.crt certs/ca.crt /etc/virtuestack/certs/
```

**Node Agent:**
```bash
# On each KVM host
scp certs/node-agent.key certs/node-agent.crt certs/ca.crt user@kvm-host:/etc/virtuestack/certs/
```

### 2.4 Database Migrations

Migrations are versioned and must be applied in order.

```bash
# Check current migration status
migrate -path migrations -database "postgresql://virtuestack:${POSTGRES_PASSWORD}@localhost:5432/virtuestack?sslmode=disable" version

# Apply all pending migrations
make migrate-up

# Rollback last migration (use with caution)
make migrate-down

# Create new migration
make migrate-create NAME=add_new_feature
```

### 2.5 SSL/TLS for Web Access

The Nginx reverse proxy handles HTTPS termination. Configure it in `nginx/conf.d/default.conf`:

```nginx
server {
    listen 443 ssl http2;
    server_name admin.yourdomain.com;

    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;
    ssl_protocols TLSv1.3 TLSv1.2;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;

    location / {
        proxy_pass http://admin-webui:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }

    location /api/ {
        proxy_pass http://controller:8080/api/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## 3. Development/Test Environment

### 3.1 Quick Start

For local development without a full KVM setup:

```bash
# Clone and setup
git clone https://github.com/your-org/virtuestack.git
cd virtuestack

# Install dependencies
make deps

# Create development .env
cat > .env << EOF
POSTGRES_USER=virtuestack
POSTGRES_PASSWORD=virtuestack_dev
POSTGRES_DB=virtuestack
NATS_AUTH_TOKEN=dev_token
JWT_SECRET=development_jwt_secret_minimum_32_chars
ENCRYPTION_KEY=development_encryption_key_32b
LOG_LEVEL=debug
NEXT_PUBLIC_API_URL=/api/v1
SSL_CERT_PATH=./ssl/cert.pem
SSL_KEY_PATH=./ssl/key.pem
EOF

# Generate dev certificates
make certs
# Or for SSL:
mkdir -p ssl
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout ssl/key.pem -out ssl/cert.pem \
    -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

# Build and start
make docker-build
make docker-up

# Run migrations
make migrate-up

# Build and run Controller locally (for debugging)
make build-controller
DATABASE_URL="postgresql://virtuestack:virtuestack_dev@localhost:5432/virtuestack?sslmode=disable" \
NATS_URL="nats://dev_token@localhost:4222" \
JWT_SECRET="development_jwt_secret_minimum_32_chars" \
ENCRYPTION_KEY="development_encryption_key_32b" \
./bin/controller
```

### 3.2 E2E Testing Setup

The E2E test infrastructure provides a complete test environment with mock services.

#### Automated Setup

```bash
# Full setup (generates secrets, certs, seed data, starts services)
./scripts/setup-e2e.sh --start

# Setup only (without starting services)
./scripts/setup-e2e.sh

# Cleanup when done
./scripts/setup-e2e.sh --clean
```

#### Manual Setup

```bash
# 1. Install dependencies
cd tests/e2e
npm ci
npx playwright install --with-deps chromium

# 2. Generate test environment
./scripts/setup-e2e.sh

# 3. Start services
docker compose -f docker-compose.yml -f docker-compose.test.yml up -d

# 4. Run migrations
make migrate-up

# 5. Seed test data
psql postgresql://virtuestack:virtuestack_test_password@localhost:5432/virtuestack < migrations/test_seed.sql

# 6. Run tests
npm test
```

#### Test Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Docker Stack (Controller Side)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │PostgreSQL│  │   NATS   │  │Controller│  │ WebUIs + Nginx   │ │
│  │   :5432  │  │  :4222   │  │  :8080   │  │  :3000/:3001     │ │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                     │ gRPC (mocked)
                                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                  Mock Node Agent (Wiremock)                     │
│                     Port 50051 (HTTP)                           │
└─────────────────────────────────────────────────────────────────┘
```

For tests requiring real KVM operations, deploy a Node Agent on a KVM host and configure it to connect to the test Controller.

### 3.3 Test Data and Credentials

The E2E seed script creates predictable test data:

#### Test Users

| User | Email | Password | 2FA Secret |
|------|-------|----------|------------|
| Admin | admin@test.virtuestack.local | AdminTest123! | - |
| Admin (2FA) | 2fa-admin@test.virtuestack.local | AdminTest123! | JBSWY3DPEHPK3PXP |
| Customer | customer@test.virtuestack.local | CustomerTest123! | - |
| Customer (2FA) | 2fa-customer@test.virtuestack.local | CustomerTest123! | KRSXG5DSN5XW4ZLP |

#### Test Data IDs

| Resource | ID Pattern |
|----------|------------|
| Plans | `11111111-1111-1111-1111-111111111001` - `004` |
| Locations | `22222222-2222-2222-2222-222222222001` - `002` |
| Nodes | `33333333-3333-3333-3333-333333333001` - `005` |
| IP Sets | `44444444-4444-4444-4444-444444444001` - `002` |
| Templates | `66666666-6666-6666-6666-666666666001` - `005` |
| Admins | `77777777-7777-7777-7777-777777777001` - `003` |
| Customers | `88888888-8888-8888-8888-888888888001` - `003` |
| VMs | `99999999-9999-9999-9999-999999999001` - `003` |
| Backups | `aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01` - `03` |
| Snapshots | `bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01` - `03` |
| API Keys | `cccccccc-cccc-cccc-cccc-cccccccccc01` - `02` |
| Webhooks | `dddddddd-dddd-dddd-dddd-dddddddddd01` |

#### Test URLs

| Service | URL |
|---------|-----|
| Admin WebUI | http://localhost:3000 |
| Customer WebUI | http://localhost:3001 |
| Controller API | http://localhost:8080 |

---

## 4. Configuration Reference

### Controller Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| DATABASE_URL | Yes | - | PostgreSQL connection string |
| NATS_URL | Yes | - | NATS server URL with auth token |
| JWT_SECRET | Yes | - | HMAC secret for JWT signing (32+ chars) |
| ENCRYPTION_KEY | Yes | - | AES-256 key for secret encryption |
| LISTEN_ADDR | No | :8080 | HTTP listen address |
| LOG_LEVEL | No | info | Logging level (debug/info/warn/error) |
| SMTP_HOST | No | - | SMTP server hostname |
| SMTP_PORT | No | 587 | SMTP server port |
| SMTP_USER | No | - | SMTP auth username |
| SMTP_PASSWORD | No | - | SMTP auth password |
| SMTP_FROM | No | - | Email sender address |
| SMTP_ENABLED | No | false | Enable email notifications |
| SMTP_REQUIRE_TLS | No | true | Enforce STARTTLS |
| TELEGRAM_BOT_TOKEN | No | - | Telegram bot token |
| PDNS_MYSQL_DSN | No | - | PowerDNS MySQL connection |

### Node Agent Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| CONTROLLER_GRPC_ADDR | Yes | - | Controller gRPC address |
| NODE_ID | Yes | - | Unique node identifier (UUID) |
| STORAGE_BACKEND | No | ceph | Storage backend: "ceph", "qcow", or "lvm" |
| STORAGE_PATH | No | /var/lib/virtuestack/vms | QCOW2 VM storage path |
| TEMPLATE_PATH | No | /var/lib/virtuestack/templates | QCOW2 template path |
| CEPH_POOL | No | vs-vms | Ceph pool for VMs |
| CEPH_USER | No | virtuestack | Ceph auth user |
| CEPH_CONF | No | /etc/ceph/ceph.conf | Ceph configuration file path |
| LVM_VOLUME_GROUP | Conditional* | - | LVM volume group name (required if STORAGE_BACKEND=lvm) |
| LVM_THIN_POOL | Conditional* | - | LVM thin pool LV name (required if STORAGE_BACKEND=lvm) |
| TLS_CERT_FILE | Yes | - | mTLS client certificate |
| TLS_KEY_FILE | Yes | - | mTLS client key |
| TLS_CA_FILE | Yes | - | CA certificate |
| LOG_LEVEL | No | info | Logging level |

*Both `LVM_VOLUME_GROUP` and `LVM_THIN_POOL` are required when `STORAGE_BACKEND=lvm`. The thin pool must be pre-existing within the volume group.

### Security Considerations

#### Rate Limiting for Distributed Deployments

**IMPORTANT:** The default in-memory rate limiting does NOT protect multi-instance deployments.

| Deployment | Recommended Rate Limiter | Why |
|------------|-------------------------|-----|
| Single controller instance | In-memory (default) | Simplicity, no external dependencies |
| Multiple controller instances (load balanced) | **Redis-backed** | Shared state across all instances |

**Why Redis is required for multi-instance:**
- Each controller instance maintains its own in-memory rate limit counters
- Attackers can bypass limits by distributing requests across instances
- Redis provides shared state so all instances see the same request counts

**Configuration:**
To use Redis-backed rate limiting, configure a Redis instance and use the `RedisRateLimit` middleware instead of `RateLimit`. See `internal/controller/api/middleware/ratelimit.go` for implementation details.

#### NATS Authentication

`NATS_AUTH_TOKEN` is **required** and must be:
- At least 32 characters in production
- Different from the default development token
- Kept secret and rotated periodically

---

## 5. Verification

### Check Controller Stack

```bash
# Check all services are running
docker compose ps

# Check health endpoints
curl -sf http://localhost:8080/health
curl -sf http://localhost:3000
curl -sf http://localhost:3001

# Check PostgreSQL
docker exec virtuestack-postgres pg_isready -U virtuestack

# Check NATS
docker exec virtuestack-nats wget -qO- http://localhost:8222/healthz
```

### Check Node Agent

```bash
# Check service status
sudo systemctl status virtuestack-node-agent

# Check logs
sudo journalctl -u virtuestack-node-agent -n 50

# Check libvirt connectivity
virsh list --all

# Check Ceph connectivity (if applicable)
ceph -s
rbd ls -p vs-vms
```

### Run Integration Tests

```bash
# Go tests that do not require native libvirt/Ceph development headers
make test

# Docker/Testcontainers-backed integration tests
make test-integration

# Go tests with race detector (same non-native package set)
make test-race

# Native node-agent tests (requires libvirt/Ceph development headers on the host)
make test-native

# E2E tests
cd tests/e2e
npm test
```

---

## 6. Troubleshooting

### Common Issues

#### Controller won't start

```bash
# Check logs
docker logs virtuestack-controller

# Common causes:
# - Database not ready: wait longer or check postgres health
# - Missing secrets: ensure all required env vars are set
# - Migration pending: run make migrate-up
```

#### Node Agent connection refused

```bash
# Check if Node Agent is running
sudo systemctl status virtuestack-node-agent

# Check mTLS certificates
openssl verify -CAfile /etc/virtuestack/certs/ca.crt /etc/virtuestack/certs/node-agent.crt

# Check network connectivity
telnet controller.yourdomain.com 50051
```

#### Database migration failed

```bash
# Check migration version
migrate -path migrations -database "postgresql://..." version

# Force version (use with caution)
migrate -path migrations -database "postgresql://..." force <version>

# Check for dirty state
migrate -path migrations -database "postgresql://..." version
# If dirty, manually fix in schema_migrations table
```

#### VM creation fails

```bash
# Check Node Agent logs
sudo journalctl -u virtuestack-node-agent -n 100

# Check libvirt
virsh list --all
virsh dominfo vs-{vm-uuid}

# Check storage (Ceph)
rbd info vs-vms/vs-{vm-uuid}-disk0

# Check storage (QCOW2)
ls -la /var/lib/virtuestack/vms/
qemu-img info /var/lib/virtuestack/vms/vs-{vm-uuid}-disk0.qcow2
```

#### Node agent fails to start with "library not found"

```bash
# Check for missing shared libraries
ldd /usr/local/bin/virtuestack-node-agent | grep "not found"

# Common missing libraries and their packages:
# libvirt.so.0 not found  → sudo apt install libvirt0
# librbd.so.1 not found   → sudo apt install librbd1
# librados.so.2 not found → sudo apt install librados2

# Install all required libraries
sudo apt install -y libvirt0 librbd1 librados2
```

### Useful Commands

```bash
# View all container logs
docker compose logs -f

# Restart a specific service
docker compose restart controller

# Access database
docker exec -it virtuestack-postgres psql -U virtuestack -d virtuestack

# Check NATS stream info
docker exec virtuestack-nats nats stream info TASKS

# Rebuild a specific image
docker compose build --no-cache controller

# Full cleanup and rebuild
docker compose down -v
docker compose build --no-cache
docker compose up -d
```

---

## Additional Resources

- [Architecture Documentation](ARCHITECTURE.md)
- [API Reference](API.md)
- [E2E Testing Guide](../tests/e2e/README.md)
- [Coding Standards](CODING_STANDARD.md)
- [Agent Reference (AGENTS.md)](../AGENTS.md)
