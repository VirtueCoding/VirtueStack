-- VirtueStack Initial Schema Migration
-- Creates all tables, indexes, RLS policies, and roles

BEGIN;

SET lock_timeout = '5s';

-- ============================================================================
-- EXTENSIONS
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- ROLES
-- ============================================================================

-- Main application role (Controller connects as this)
CREATE ROLE app_user WITH NOLOGIN;

-- Role for RLS customer isolation (used with SET ROLE)
CREATE ROLE app_customer WITH NOLOGIN;

-- ============================================================================
-- BASE TABLES (no foreign key dependencies)
-- ============================================================================

-- Locations table
CREATE TABLE locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    region VARCHAR(100),
    country VARCHAR(2),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Customers table
CREATE TABLE customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name VARCHAR(255) NOT NULL,
    whmcs_client_id INTEGER UNIQUE,
    totp_secret_encrypted TEXT,
    totp_enabled BOOLEAN DEFAULT FALSE,
    totp_backup_codes_hash TEXT[],
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Admins table
CREATE TABLE admins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(254) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name VARCHAR(255) NOT NULL,
    totp_secret_encrypted TEXT NOT NULL,
    totp_enabled BOOLEAN DEFAULT FALSE,
    totp_backup_codes_hash TEXT[],
    role VARCHAR(20) DEFAULT 'admin' CHECK (role IN ('admin', 'super_admin')),
    max_sessions INTEGER DEFAULT 3,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Plans table
CREATE TABLE plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    vcpu INTEGER NOT NULL,
    memory_mb INTEGER NOT NULL,
    disk_gb INTEGER NOT NULL,
    port_speed_mbps INTEGER NOT NULL,
    bandwidth_limit_gb INTEGER DEFAULT 0,
    bandwidth_overage_speed_mbps INTEGER DEFAULT 5,
    max_ipv4 INTEGER DEFAULT 1,
    max_ipv6_slash64 INTEGER DEFAULT 1,
    max_snapshots INTEGER DEFAULT 3,
    max_backups INTEGER DEFAULT 1,
    max_iso_count INTEGER DEFAULT 1,
    max_iso_gb INTEGER DEFAULT 5,
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Templates table
CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    os_family VARCHAR(50) NOT NULL,
    os_version VARCHAR(20) NOT NULL,
    rbd_image VARCHAR(200) NOT NULL,
    rbd_snapshot VARCHAR(100) NOT NULL,
    min_disk_gb INTEGER DEFAULT 10,
    supports_cloudinit BOOLEAN DEFAULT TRUE,
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Provisioning keys table
CREATE TABLE provisioning_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    key_hash TEXT NOT NULL,
    allowed_ips INET[],
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    created_by UUID,
    description TEXT
);

-- System settings table
CREATE TABLE system_settings (
    key VARCHAR(255) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by UUID
);

-- Tasks table
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    payload JSONB NOT NULL,
    result JSONB,
    error_message TEXT,
    progress INTEGER DEFAULT 0,
    idempotency_key VARCHAR(100) UNIQUE,
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

-- Sessions table
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    user_type VARCHAR(20) NOT NULL,
    refresh_token_hash TEXT NOT NULL,
    ip_address INET,
    user_agent TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- TABLES WITH FOREIGN KEY DEPENDENCIES (LEVEL 1)
-- ============================================================================

-- IP Sets table (depends on locations)
CREATE TABLE ip_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    location_id UUID REFERENCES locations(id) ON DELETE SET NULL,
    network CIDR NOT NULL,
    gateway INET NOT NULL,
    vlan_id INTEGER,
    ip_version SMALLINT CHECK (ip_version IN (4, 6)),
    node_ids UUID[],
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Nodes table (depends on locations)
CREATE TABLE nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname VARCHAR(255) NOT NULL UNIQUE,
    grpc_address VARCHAR(255) NOT NULL,
    management_ip INET NOT NULL,
    location_id UUID REFERENCES locations(id) ON DELETE SET NULL,
    status VARCHAR(20) DEFAULT 'offline' CHECK (status IN ('online', 'degraded', 'offline', 'draining', 'failed')),
    total_vcpu INTEGER NOT NULL,
    total_memory_mb INTEGER NOT NULL,
    allocated_vcpu INTEGER DEFAULT 0,
    allocated_memory_mb INTEGER DEFAULT 0,
    ceph_pool VARCHAR(100) DEFAULT 'vs-vms',
    ipmi_address INET,
    ipmi_username_encrypted TEXT,
    ipmi_password_encrypted TEXT,
    last_heartbeat_at TIMESTAMPTZ,
    consecutive_heartbeat_misses INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Customer API keys table (depends on customers)
CREATE TABLE customer_api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    key_hash TEXT NOT NULL,
    vm_ids UUID[],
    permissions TEXT[],
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

-- Customer webhooks table (depends on customers)
CREATE TABLE customer_webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    url VARCHAR(2048) NOT NULL,
    secret TEXT NOT NULL,
    events TEXT[] NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    last_triggered_at TIMESTAMPTZ,
    consecutive_failures INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- TABLES WITH FOREIGN KEY DEPENDENCIES (LEVEL 2)
-- ============================================================================

-- IPv6 prefixes table (depends on nodes)
CREATE TABLE ipv6_prefixes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    prefix CIDR NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- VMs table (depends on customers, nodes, plans, templates)
CREATE TABLE vms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    plan_id UUID NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    hostname VARCHAR(63) NOT NULL,
    status VARCHAR(20) DEFAULT 'provisioning' CHECK (status IN ('provisioning', 'running', 'stopped', 'suspended', 'migrating', 'reinstalling', 'error', 'deleted')),
    vcpu INTEGER NOT NULL,
    memory_mb INTEGER NOT NULL,
    disk_gb INTEGER NOT NULL,
    port_speed_mbps INTEGER NOT NULL,
    bandwidth_limit_gb INTEGER DEFAULT 0,
    bandwidth_used_bytes BIGINT DEFAULT 0,
    bandwidth_reset_at TIMESTAMPTZ DEFAULT NOW(),
    mac_address MACADDR NOT NULL UNIQUE,
    template_id UUID REFERENCES templates(id) ON DELETE SET NULL,
    libvirt_domain_name VARCHAR(100),
    root_password_encrypted TEXT,
    whmcs_service_id INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Node heartbeats table (depends on nodes)
CREATE TABLE node_heartbeats (
    id BIGSERIAL PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cpu_percent REAL,
    memory_percent REAL,
    disk_percent REAL,
    vm_count INTEGER,
    load_average REAL[]
);

-- Webhook deliveries table (depends on customer_webhooks)
CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES customer_webhooks(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    response_code INTEGER,
    response_body TEXT,
    attempt INTEGER DEFAULT 1,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'retrying')),
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- TABLES WITH FOREIGN KEY DEPENDENCIES (LEVEL 3)
-- ============================================================================

-- IP Addresses table (depends on ip_sets, vms, customers)
CREATE TABLE ip_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_set_id UUID NOT NULL REFERENCES ip_sets(id) ON DELETE CASCADE,
    address INET NOT NULL UNIQUE,
    ip_version SMALLINT CHECK (ip_version IN (4, 6)),
    vm_id UUID REFERENCES vms(id) ON DELETE SET NULL,
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    is_primary BOOLEAN DEFAULT FALSE,
    rdns_hostname VARCHAR(255),
    status VARCHAR(20) DEFAULT 'available' CHECK (status IN ('available', 'assigned', 'reserved', 'cooldown')),
    assigned_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    cooldown_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- VM IPv6 subnets table (depends on vms, ipv6_prefixes)
CREATE TABLE vm_ipv6_subnets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    ipv6_prefix_id UUID NOT NULL REFERENCES ipv6_prefixes(id) ON DELETE CASCADE,
    subnet CIDR NOT NULL,
    subnet_index INTEGER NOT NULL,
    gateway INET NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Backups table (depends on vms)
CREATE TABLE backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    type VARCHAR(20) CHECK (type IN ('full', 'incremental')),
    rbd_snapshot VARCHAR(100),
    diff_from_snapshot VARCHAR(100),
    storage_path TEXT,
    size_bytes BIGINT,
    status VARCHAR(20) DEFAULT 'creating' CHECK (status IN ('creating', 'completed', 'failed', 'restoring', 'deleted')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

-- Snapshots table (depends on vms)
CREATE TABLE snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    rbd_snapshot VARCHAR(100) NOT NULL,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- PARTITIONED TABLES
-- ============================================================================

-- Audit logs table (partitioned by timestamp)
CREATE TABLE audit_logs (
    id UUID DEFAULT gen_random_uuid(),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_id UUID,
    actor_type VARCHAR(20) NOT NULL,
    actor_ip INET,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    changes JSONB,
    correlation_id UUID,
    success BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

-- Create initial partitions for audit_logs
CREATE TABLE audit_logs_2026_03 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE audit_logs_2026_04 PARTITION OF audit_logs
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

-- ============================================================================
-- INDEXES
-- ============================================================================

-- VMs indexes
CREATE INDEX idx_vms_customer_id ON vms(customer_id);
CREATE INDEX idx_vms_node_id ON vms(node_id);
CREATE INDEX idx_vms_status ON vms(status);
CREATE INDEX idx_vms_whmcs_service_id ON vms(whmcs_service_id);
CREATE INDEX idx_vms_deleted_at ON vms(deleted_at);

-- IP Addresses indexes
CREATE INDEX idx_ip_addresses_ip_set_id ON ip_addresses(ip_set_id);
CREATE INDEX idx_ip_addresses_vm_id ON ip_addresses(vm_id);
CREATE INDEX idx_ip_addresses_customer_id ON ip_addresses(customer_id);
CREATE INDEX idx_ip_addresses_status ON ip_addresses(status);

-- Tasks indexes
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_type ON tasks(type);
CREATE INDEX idx_tasks_created_by ON tasks(created_by);
CREATE INDEX idx_tasks_idempotency_key ON tasks(idempotency_key);

-- Audit logs indexes
CREATE INDEX idx_audit_logs_actor_id ON audit_logs(actor_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_logs_correlation_id ON audit_logs(correlation_id);

-- Node heartbeats index
CREATE INDEX idx_node_heartbeats_node_timestamp ON node_heartbeats(node_id, timestamp);

-- Sessions indexes
CREATE INDEX idx_sessions_user ON sessions(user_id, user_type);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- Backups indexes
CREATE INDEX idx_backups_vm_id ON backups(vm_id);
CREATE INDEX idx_backups_status ON backups(status);

-- Snapshots index
CREATE INDEX idx_snapshots_vm_id ON snapshots(vm_id);

-- Webhook deliveries indexes
CREATE INDEX idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_status_retry ON webhook_deliveries(status, next_retry_at);

-- Indexes for customer_api_keys
CREATE INDEX IF NOT EXISTS idx_customer_api_keys_customer_id ON customer_api_keys(customer_id);
CREATE INDEX IF NOT EXISTS idx_customer_api_keys_key_hash_active ON customer_api_keys(key_hash) WHERE revoked_at IS NULL;

-- Indexes for provisioning_keys
CREATE INDEX IF NOT EXISTS idx_provisioning_keys_key_hash ON provisioning_keys(key_hash);

-- Composite index for backups
CREATE INDEX IF NOT EXISTS idx_backups_status_created ON backups(status, created_at DESC);

-- Composite index for tasks
CREATE INDEX IF NOT EXISTS idx_tasks_status_created ON tasks(status, created_at DESC);

-- ============================================================================
-- ROW LEVEL SECURITY
-- ============================================================================

-- Enable RLS on vms table
ALTER TABLE vms ENABLE ROW LEVEL SECURITY;

-- Create policy for customer isolation
CREATE POLICY customer_vms ON vms FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- ============================================================================
-- PERMISSIONS
-- ============================================================================

-- Grant appropriate permissions to app_user
GRANT USAGE ON SCHEMA public TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;

-- Revoke UPDATE, DELETE on audit_logs from app_user (append-only)
REVOKE UPDATE, DELETE ON audit_logs FROM app_user;

-- Grant permissions to app_customer for RLS
GRANT USAGE ON SCHEMA public TO app_customer;
GRANT SELECT, INSERT, UPDATE ON vms TO app_customer;

COMMIT;