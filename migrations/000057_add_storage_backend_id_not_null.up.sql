-- Migration: 000057_add_storage_backend_id_not_null
-- Description: Enforce NOT NULL constraint on vms.storage_backend_id after data migration
--              guarantees that all VMs have a storage backend assigned via migration 000055

BEGIN;

-- Ensure all existing VMs have a storage_backend_id assigned
-- This should have been populated by migration 000055, but we verify here
DO $$
DECLARE
    vm_count INTEGER;
    unassigned_count INTEGER;
BEGIN
    -- Count VMs with no storage_backend_id
    SELECT COUNT(*) INTO unassigned_count
    FROM vms
    WHERE storage_backend_id IS NULL;

    IF unassigned_count > 0 THEN
        RAISE NOTICE 'Found % VMs without storage_backend_id - attempting to backfill', unassigned_count;

        -- Attempt to backfill from node_storage junction
        UPDATE vms v
        SET storage_backend_id = (
            SELECT ns.storage_backend_id
            FROM node_storage ns
            JOIN storage_backends sb ON sb.id = ns.storage_backend_id
            WHERE ns.node_id = v.node_id
              AND sb.type = v.storage_backend
              AND ns.enabled = true
            LIMIT 1
        )
        WHERE v.storage_backend_id IS NULL
          AND v.node_id IS NOT NULL
          AND v.storage_backend IS NOT NULL
          AND v.storage_backend != '';
    END IF;

    -- Check again after backfill
    SELECT COUNT(*) INTO unassigned_count
    FROM vms
    WHERE storage_backend_id IS NULL;

    IF unassigned_count > 0 THEN
        RAISE EXCEPTION 'Cannot enforce NOT NULL: % VMs still have NULL storage_backend_id', unassigned_count;
    END IF;
END $$;

-- Now add the NOT NULL constraint
-- Using ALTER COLUMN since the column already exists
ALTER TABLE vms ALTER COLUMN storage_backend_id SET NOT NULL;

-- Add a helpful comment
COMMENT ON COLUMN vms.storage_backend_id IS 'Reference to the storage backend holding this VM disk. NOT NULL enforced after migration 000057.';

COMMIT;
