-- Migration: 000057_add_storage_backend_id_not_null
-- Description: Rollback NOT NULL constraint on vms.storage_backend_id

BEGIN;

-- Remove the NOT NULL constraint
ALTER TABLE vms ALTER COLUMN storage_backend_id DROP NOT NULL;

-- Update the comment to reflect the change
COMMENT ON COLUMN vms.storage_backend_id IS 'Reference to the storage backend holding this VM disk.';

COMMIT;
