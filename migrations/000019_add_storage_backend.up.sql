-- +mig Up
-- Add storage backend support to enable file-based storage (qcow) alongside Ceph
-- Storage backend can be 'ceph' (default, block storage) or 'qcow' (file-based)

-- Set lock timeout to prevent indefinite locks on production tables
SET lock_timeout = '5s';

-- =============================================================================
-- plans table: Add storage_backend for specifying default storage type
-- =============================================================================

-- Add storage_backend column with default 'ceph'
ALTER TABLE plans 
ADD COLUMN IF NOT EXISTS storage_backend VARCHAR(20) NOT NULL DEFAULT 'ceph';

-- Add CHECK constraint to enforce valid storage backend values
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'chk_plans_storage_backend'
    ) THEN
        ALTER TABLE plans 
        ADD CONSTRAINT chk_plans_storage_backend 
        CHECK (storage_backend IN ('ceph', 'qcow'));
    END IF;
END $$;

-- Add index on storage_backend for efficient filtering
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_plans_storage_backend 
ON plans(storage_backend);

-- Add comment for documentation
COMMENT ON COLUMN plans.storage_backend IS 'Storage backend type: ceph (block) or qcow (file-based)';

-- =============================================================================
-- vms table: Add storage_backend and disk_path for VM disk management
-- =============================================================================

-- Add storage_backend column
ALTER TABLE vms 
ADD COLUMN IF NOT EXISTS storage_backend VARCHAR(20) NOT NULL DEFAULT 'ceph';

-- Add disk_path for file-based storage (qcow disk file location)
ALTER TABLE vms 
ADD COLUMN IF NOT EXISTS disk_path VARCHAR(500);

-- Add CHECK constraint
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'chk_vms_storage_backend'
    ) THEN
        ALTER TABLE vms 
        ADD CONSTRAINT chk_vms_storage_backend 
        CHECK (storage_backend IN ('ceph', 'qcow'));
    END IF;
END $$;

-- Add index on storage_backend
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vms_storage_backend 
ON vms(storage_backend);

-- Add comments
COMMENT ON COLUMN vms.storage_backend IS 'Storage backend type: ceph (block) or qcow (file-based)';
COMMENT ON COLUMN vms.disk_path IS 'Path to disk file for qcow storage backend';

-- =============================================================================
-- nodes table: Add storage_backend and storage_path for node storage config
-- =============================================================================

-- Add storage_backend column
ALTER TABLE nodes 
ADD COLUMN IF NOT EXISTS storage_backend VARCHAR(20) NOT NULL DEFAULT 'ceph';

-- Add storage_path for base path for file storage on this node
ALTER TABLE nodes 
ADD COLUMN IF NOT EXISTS storage_path VARCHAR(500);

-- Add CHECK constraint
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'chk_nodes_storage_backend'
    ) THEN
        ALTER TABLE nodes 
        ADD CONSTRAINT chk_nodes_storage_backend 
        CHECK (storage_backend IN ('ceph', 'qcow'));
    END IF;
END $$;

-- Add comments
COMMENT ON COLUMN nodes.storage_backend IS 'Storage backend type: ceph (block) or qcow (file-based)';
COMMENT ON COLUMN nodes.storage_path IS 'Base path for file-based storage on this node';

-- =============================================================================
-- templates table: Add storage_backend and file_path for template storage
-- =============================================================================

-- Add storage_backend column
ALTER TABLE templates 
ADD COLUMN IF NOT EXISTS storage_backend VARCHAR(20) NOT NULL DEFAULT 'ceph';

-- Add file_path for file-based template storage
ALTER TABLE templates 
ADD COLUMN IF NOT EXISTS file_path VARCHAR(500);

-- Add CHECK constraint
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'chk_templates_storage_backend'
    ) THEN
        ALTER TABLE templates 
        ADD CONSTRAINT chk_templates_storage_backend 
        CHECK (storage_backend IN ('ceph', 'qcow'));
    END IF;
END $$;

-- Add comments
COMMENT ON COLUMN templates.storage_backend IS 'Storage backend type: ceph (block) or qcow (file-based)';
COMMENT ON COLUMN templates.file_path IS 'Path to template file for qcow storage backend';

-- =============================================================================
-- backups table: Add storage_backend and file_path for backup storage
-- =============================================================================

-- Add storage_backend column
ALTER TABLE backups 
ADD COLUMN IF NOT EXISTS storage_backend VARCHAR(20) NOT NULL DEFAULT 'ceph';

-- Add file_path for file-based backup storage
ALTER TABLE backups 
ADD COLUMN IF NOT EXISTS file_path VARCHAR(500);

-- Add CHECK constraint
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'chk_backups_storage_backend'
    ) THEN
        ALTER TABLE backups 
        ADD CONSTRAINT chk_backups_storage_backend 
        CHECK (storage_backend IN ('ceph', 'qcow'));
    END IF;
END $$;

-- Add comments
COMMENT ON COLUMN backups.storage_backend IS 'Storage backend type: ceph (block) or qcow (file-based)';
COMMENT ON COLUMN backups.file_path IS 'Path to backup file for qcow storage backend';

-- Reset lock timeout
RESET lock_timeout;