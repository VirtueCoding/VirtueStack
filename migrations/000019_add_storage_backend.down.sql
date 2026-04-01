-- +mig Down
-- Remove storage backend columns added by 000019_add_storage_backend

-- Set lock timeout
SET lock_timeout = '5s';

-- =============================================================================
-- backups table: Drop storage_backend columns
-- =============================================================================

-- Drop CHECK constraint first
ALTER TABLE backups 
DROP CONSTRAINT IF EXISTS chk_backups_storage_backend;

-- Drop columns
ALTER TABLE backups 
DROP COLUMN IF EXISTS storage_backend,
DROP COLUMN IF EXISTS file_path;

-- =============================================================================
-- templates table: Drop storage_backend columns
-- =============================================================================

ALTER TABLE templates 
DROP CONSTRAINT IF EXISTS chk_templates_storage_backend;

ALTER TABLE templates 
DROP COLUMN IF EXISTS storage_backend,
DROP COLUMN IF EXISTS file_path;

-- =============================================================================
-- nodes table: Drop storage_backend columns
-- =============================================================================

ALTER TABLE nodes 
DROP CONSTRAINT IF EXISTS chk_nodes_storage_backend;

ALTER TABLE nodes 
DROP COLUMN IF EXISTS storage_backend,
DROP COLUMN IF EXISTS storage_path;

-- =============================================================================
-- vms table: Drop storage_backend columns
-- =============================================================================

ALTER TABLE vms 
DROP CONSTRAINT IF EXISTS chk_vms_storage_backend;

ALTER TABLE vms 
DROP COLUMN IF EXISTS storage_backend,
DROP COLUMN IF EXISTS disk_path;

-- Drop indexes (must be done before dropping columns if they exist)
DROP INDEX IF EXISTS idx_vms_storage_backend;

-- =============================================================================
-- plans table: Drop storage_backend columns
-- =============================================================================

ALTER TABLE plans 
DROP CONSTRAINT IF EXISTS chk_plans_storage_backend;

ALTER TABLE plans 
DROP COLUMN IF EXISTS storage_backend;

-- Drop indexes
DROP INDEX IF EXISTS idx_plans_storage_backend;

-- Reset lock timeout
RESET lock_timeout;