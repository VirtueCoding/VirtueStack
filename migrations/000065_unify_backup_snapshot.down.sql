-- Rollback: Remove method/name columns from backups, delete migrated snapshots.

BEGIN;

SET lock_timeout = '5s';

-- Step 1: Remove migrated snapshot rows from backups table.
DELETE FROM backups WHERE method = 'snapshot';

-- Step 2: Restore plan limits by subtracting what was added.
-- This is approximate; exact rollback is not possible without tracking original values.

-- Step 3: Drop index.
DROP INDEX IF EXISTS idx_backups_method;

-- Step 4: Drop CHECK constraint.
ALTER TABLE backups DROP CONSTRAINT IF EXISTS backups_method_check;

-- Step 5: Drop added columns.
ALTER TABLE backups DROP COLUMN IF EXISTS name;
ALTER TABLE backups DROP COLUMN IF EXISTS method;

COMMIT;
