-- Unify Backup & Snapshot into a single "Backup" concept.
-- Phase 1 (expand): Add method column to backups table, copy snapshots in.
-- The snapshots table is kept for backward compatibility during the transition.

BEGIN;

SET lock_timeout = '5s';

-- Step 1: Add method column to backups table.
-- 'full' = traditional backup (exported to separate storage)
-- 'snapshot' = point-in-time reference (in-place, fast)
ALTER TABLE backups ADD COLUMN IF NOT EXISTS method VARCHAR(20) NOT NULL DEFAULT 'full';

-- Step 2: Add name column to backups table (snapshots have names).
ALTER TABLE backups ADD COLUMN IF NOT EXISTS name VARCHAR(256);

-- Step 3: Add CHECK constraint on method values.
ALTER TABLE backups ADD CONSTRAINT backups_method_check
    CHECK (method IN ('full', 'snapshot'));

-- Step 4: Migrate existing snapshots into the backups table.
-- Map snapshot fields to backup fields:
--   snapshot.name -> backup.name
--   snapshot.rbd_snapshot -> backup.rbd_snapshot
--   snapshot.name -> backup.snapshot_name (legacy snapshots have no qcow_snapshot column)
--   storage_backend defaults to 'ceph' for legacy snapshots
--   'snapshot' -> backup.method
--   'manual' -> backup.source
--   'completed' -> backup.status (snapshots don't have status; if they exist they're complete)
INSERT INTO backups (id, vm_id, source, storage_backend, rbd_snapshot, snapshot_name, size_bytes, status, method, name, created_at)
SELECT
    s.id,
    s.vm_id,
    'manual',
    'ceph',
    CASE WHEN s.rbd_snapshot != '' THEN s.rbd_snapshot ELSE NULL END,
    s.name,
    s.size_bytes,
    'completed',
    'snapshot',
    s.name,
    s.created_at
FROM snapshots s
WHERE NOT EXISTS (SELECT 1 FROM backups b WHERE b.id = s.id);

-- Step 5: Update plan limits - increase backup_limit to absorb snapshot_limit.
-- For plans where snapshot_limit > 0, add it to backup_limit.
UPDATE plans
SET backup_limit = backup_limit + snapshot_limit
WHERE snapshot_limit > 0;

-- Step 6: Add index for method-based filtering.
CREATE INDEX IF NOT EXISTS idx_backups_method ON backups(method);

-- Step 7: Add comment for method column.
COMMENT ON COLUMN backups.method IS 'Backup method: full (exported copy) or snapshot (point-in-time reference)';
COMMENT ON COLUMN backups.name IS 'User-provided name for the backup (optional for full, required for snapshots)';

COMMIT;
