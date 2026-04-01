-- Migration 000054: Storage Backend Registry
-- Creates first-class storage_backends table, node_storage junction table,
-- and adds LVM support to existing CHECK constraints.

BEGIN;

SET lock_timeout = '5s';

-- =============================================================================
-- 1. STORAGE BACKENDS TABLE
-- First-class entities representing storage pools (Ceph, QCOW, or LVM)
-- =============================================================================

CREATE TABLE storage_backends (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    type VARCHAR(10) NOT NULL,  -- 'ceph', 'qcow', 'lvm'

    -- Ceph config (nullable, required when type='ceph')
    ceph_pool VARCHAR(100),
    ceph_user VARCHAR(100),
    ceph_monitors TEXT,

    -- QCOW config (nullable, required when type='qcow')
    storage_path VARCHAR(500),

    -- LVM config (nullable, required when type='lvm')
    lvm_volume_group VARCHAR(100),
    lvm_thin_pool VARCHAR(100),

    -- Health metrics
    total_gb BIGINT,
    used_gb BIGINT,
    available_gb BIGINT,
    health_status VARCHAR(20) DEFAULT 'unknown',
    health_message TEXT,
    lvm_data_percent NUMERIC(5,2),
    lvm_metadata_percent NUMERIC(5,2),

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT chk_storage_type CHECK (type IN ('ceph', 'qcow', 'lvm')),
    CONSTRAINT chk_ceph_config CHECK (type != 'ceph' OR (ceph_pool IS NOT NULL AND ceph_pool != '')),
    CONSTRAINT chk_qcow_config CHECK (type != 'qcow' OR (storage_path IS NOT NULL AND storage_path != '')),
    CONSTRAINT chk_lvm_config CHECK (type != 'lvm' OR (lvm_volume_group IS NOT NULL AND lvm_thin_pool IS NOT NULL))
);

-- Comments for documentation
COMMENT ON TABLE storage_backends IS 'First-class storage backend entities (Ceph clusters, QCOW directories, LVM thin pools)';
COMMENT ON COLUMN storage_backends.type IS 'Storage backend type: ceph (distributed block), qcow (file-based), lvm (local thin pool)';
COMMENT ON COLUMN storage_backends.ceph_pool IS 'Ceph pool name (required for type=ceph)';
COMMENT ON COLUMN storage_backends.ceph_user IS 'Ceph authentication user (e.g., client.admin)';
COMMENT ON COLUMN storage_backends.ceph_monitors IS 'Comma-separated list of Ceph monitor addresses';
COMMENT ON COLUMN storage_backends.storage_path IS 'Base directory path for QCOW files (required for type=qcow)';
COMMENT ON COLUMN storage_backends.lvm_volume_group IS 'LVM volume group name (required for type=lvm)';
COMMENT ON COLUMN storage_backends.lvm_thin_pool IS 'LVM thin pool name within volume group (required for type=lvm)';
COMMENT ON COLUMN storage_backends.health_status IS 'Current health status: healthy, degraded, unknown, offline';
COMMENT ON COLUMN storage_backends.lvm_data_percent IS 'LVM thin pool data usage percentage (0-100)';
COMMENT ON COLUMN storage_backends.lvm_metadata_percent IS 'LVM thin pool metadata usage percentage (0-100)';

-- =============================================================================
-- 2. NODE_STORAGE JUNCTION TABLE
-- Links nodes to their assigned storage backends (many-to-many)
-- =============================================================================

CREATE TABLE node_storage (
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    storage_backend_id UUID NOT NULL REFERENCES storage_backends(id) ON DELETE RESTRICT,
    enabled BOOLEAN DEFAULT true,
    preferred BOOLEAN DEFAULT false,  -- Preferred backend for this node
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (node_id, storage_backend_id)
);

-- Indexes for efficient lookups
CREATE INDEX idx_node_storage_backend ON node_storage(storage_backend_id);
CREATE INDEX idx_node_storage_preferred ON node_storage(node_id, preferred) WHERE preferred = true;

COMMENT ON TABLE node_storage IS 'Junction table linking nodes to their available storage backends';
COMMENT ON COLUMN node_storage.enabled IS 'Whether this storage backend is enabled for the node';
COMMENT ON COLUMN node_storage.preferred IS 'Whether this is the preferred storage backend for the node';

-- =============================================================================
-- 3. ADD STORAGE_BACKEND_ID TO VMS TABLE
-- Tracks which storage backend holds each VM's disk
-- =============================================================================

ALTER TABLE vms ADD COLUMN IF NOT EXISTS storage_backend_id UUID REFERENCES storage_backends(id);

COMMENT ON COLUMN vms.storage_backend_id IS 'Reference to the storage backend holding this VMs disk';

-- =============================================================================
-- 4. AUTO-UPDATE TRIGGER FOR UPDATED_AT
-- =============================================================================

CREATE OR REPLACE FUNCTION update_storage_backends_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_storage_backends_updated_at
    BEFORE UPDATE ON storage_backends
    FOR EACH ROW EXECUTE FUNCTION update_storage_backends_updated_at();

-- =============================================================================
-- 5. GRANT PERMISSIONS (consistent with existing migrations)
-- =============================================================================

GRANT SELECT, INSERT, UPDATE, DELETE ON storage_backends TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON node_storage TO app_user;
REVOKE ALL ON storage_backends FROM app_customer;
REVOKE ALL ON node_storage FROM app_customer;

-- =============================================================================
-- 6. MIGRATE EXISTING NODE CONFIG TO STORAGE_BACKENDS
-- Use full hostname to avoid duplicate name collisions
-- =============================================================================

-- Migrate Ceph nodes
INSERT INTO storage_backends (name, type, ceph_pool, ceph_user, ceph_monitors)
SELECT DISTINCT 'ceph-' || hostname, 'ceph', ceph_pool, ceph_user, ceph_monitors
FROM nodes
WHERE storage_backend = 'ceph' AND ceph_pool IS NOT NULL
ON CONFLICT (name) DO NOTHING;

-- Migrate QCOW nodes
INSERT INTO storage_backends (name, type, storage_path)
SELECT DISTINCT 'qcow-' || hostname, 'qcow', storage_path
FROM nodes
WHERE storage_backend = 'qcow' AND storage_path IS NOT NULL
ON CONFLICT (name) DO NOTHING;

-- =============================================================================
-- 7. CREATE JUNCTION RECORDS FROM EXISTING NODES
-- =============================================================================

INSERT INTO node_storage (node_id, storage_backend_id, preferred)
SELECT n.id, sb.id, true
FROM nodes n
JOIN storage_backends sb ON (
    (n.storage_backend = 'ceph' AND sb.type = 'ceph' AND sb.ceph_pool = n.ceph_pool)
    OR (n.storage_backend = 'qcow' AND sb.type = 'qcow' AND sb.storage_path = n.storage_path)
)
WHERE n.storage_backend IN ('ceph', 'qcow');

-- =============================================================================
-- 8. POPULATE VM STORAGE_BACKEND_ID
-- Link existing VMs to their storage backends via node assignment
-- =============================================================================

UPDATE vms v
SET storage_backend_id = (
    SELECT sb.id
    FROM storage_backends sb
    JOIN node_storage ns ON ns.storage_backend_id = sb.id
    WHERE ns.node_id = v.node_id
      AND sb.type = v.storage_backend
    ORDER BY sb.created_at ASC
    LIMIT 1
)
WHERE v.storage_backend IS NOT NULL
  AND v.storage_backend != ''
  AND v.storage_backend_id IS NULL;

-- =============================================================================
-- 9. ADD LVM TO EXISTING CHECK CONSTRAINTS
-- Update constraints to include 'lvm' as valid storage backend type
-- =============================================================================

-- vms table
ALTER TABLE vms DROP CONSTRAINT IF EXISTS chk_vms_storage_backend;
ALTER TABLE vms ADD CONSTRAINT chk_vms_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow', 'lvm'));

-- plans table
ALTER TABLE plans DROP CONSTRAINT IF EXISTS chk_plans_storage_backend;
ALTER TABLE plans ADD CONSTRAINT chk_plans_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow', 'lvm'));

-- templates table
ALTER TABLE templates DROP CONSTRAINT IF EXISTS chk_templates_storage_backend;
ALTER TABLE templates ADD CONSTRAINT chk_templates_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow', 'lvm'));

-- backups table
ALTER TABLE backups DROP CONSTRAINT IF EXISTS chk_backups_storage_backend;
ALTER TABLE backups ADD CONSTRAINT chk_backups_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow', 'lvm'));

-- nodes table
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS chk_nodes_storage_backend;
ALTER TABLE nodes ADD CONSTRAINT chk_nodes_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow', 'lvm'));

-- =============================================================================
-- 10. INDEXES FOR STORAGE_BACKEND_ID LOOKUPS
-- =============================================================================

CREATE INDEX IF NOT EXISTS idx_vms_storage_backend_id ON vms(storage_backend_id);

COMMIT;