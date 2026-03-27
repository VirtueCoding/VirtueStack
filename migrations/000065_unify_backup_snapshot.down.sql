-- Rollback: Remove method/name columns from backups, delete migrated snapshots.
-- NOTE: Plan limit rollback is approximate. The up migration added snapshot_limit to
-- backup_limit, but we cannot reliably restore the original values without storing them.
-- Manual review of plan limits is recommended after rollback.

BEGIN;

SET lock_timeout = '5s';

-- Step 1: Remove migrated snapshot rows from backups table.
DELETE FROM backups WHERE method = 'snapshot';

-- Step 2: Drop index.
DROP INDEX IF EXISTS idx_backups_method;

-- Step 3: Drop CHECK constraint.
ALTER TABLE backups DROP CONSTRAINT IF EXISTS backups_method_check;

-- Step 4: Drop added columns.
ALTER TABLE backups DROP COLUMN IF EXISTS name;
ALTER TABLE backups DROP COLUMN IF EXISTS method;

COMMIT;
