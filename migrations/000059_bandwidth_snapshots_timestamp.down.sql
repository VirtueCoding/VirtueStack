-- Remove snapshot_at column and index from bandwidth_snapshots
-- This reverses migration 000059

DROP INDEX CONCURRENTLY IF EXISTS idx_bandwidth_snapshots_vm_snapshot_at;

ALTER TABLE bandwidth_snapshots DROP COLUMN IF EXISTS snapshot_at;
