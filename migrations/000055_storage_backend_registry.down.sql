-- Migration 000054: Storage Backend Registry (Rollback)
-- Removes storage_backends table, node_storage junction table,
-- and reverts LVM support in CHECK constraints.

BEGIN;

SET lock_timeout = '5s';

-- =============================================================================
-- 1. RESTORE NODE STORAGE CONFIG FROM STORAGE_BACKENDS
-- Copy config back to nodes table before dropping storage_backends
-- =============================================================================

UPDATE nodes n
SET
    ceph_pool = sb.ceph_pool,
    ceph_user = sb.ceph_user,
    ceph_monitors = sb.ceph_monitors,
    storage_path = sb.storage_path
FROM storage_backends sb
JOIN node_storage ns ON ns.storage_backend_id = sb.id
WHERE ns.node_id = n.id;

-- =============================================================================
-- 2. REVERT CHECK CONSTRAINTS (remove 'lvm')
-- Restore original constraint values
-- =============================================================================

-- vms table
ALTER TABLE vms DROP CONSTRAINT IF EXISTS chk_vms_storage_backend;
ALTER TABLE vms ADD CONSTRAINT chk_vms_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow'));

-- plans table
ALTER TABLE plans DROP CONSTRAINT IF EXISTS chk_plans_storage_backend;
ALTER TABLE plans ADD CONSTRAINT chk_plans_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow'));

-- templates table
ALTER TABLE templates DROP CONSTRAINT IF EXISTS chk_templates_storage_backend;
ALTER TABLE templates ADD CONSTRAINT chk_templates_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow'));

-- backups table
ALTER TABLE backups DROP CONSTRAINT IF EXISTS chk_backups_storage_backend;
ALTER TABLE backups ADD CONSTRAINT chk_backups_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow'));

-- nodes table
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS chk_nodes_storage_backend;
ALTER TABLE nodes ADD CONSTRAINT chk_nodes_storage_backend
    CHECK (storage_backend IN ('ceph', 'qcow'));

-- =============================================================================
-- 3. DROP INDEXES
-- =============================================================================

DROP INDEX IF EXISTS idx_vms_storage_backend_id;
DROP INDEX IF EXISTS idx_node_storage_preferred;
DROP INDEX IF EXISTS idx_node_storage_backend;

-- =============================================================================
-- 4. DROP TRIGGER AND FUNCTION
-- =============================================================================

DROP TRIGGER IF EXISTS trg_storage_backends_updated_at ON storage_backends;
DROP FUNCTION IF EXISTS update_storage_backends_updated_at();

-- =============================================================================
-- 5. DROP COLUMNS AND TABLES
-- =============================================================================

ALTER TABLE vms DROP COLUMN IF EXISTS storage_backend_id;

DROP TABLE IF EXISTS node_storage;
DROP TABLE IF EXISTS storage_backends;

COMMIT;