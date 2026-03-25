-- Add snapshot_at TIMESTAMPTZ column to bandwidth_snapshots for sargable queries.
-- The existing (year, month, day, hour) columns make the WHERE clause non-sargable
-- because make_timestamp() is computed at query time, forcing sequential scans.
-- 
-- Migration plan:
-- 1. Add snapshot_at column (nullable initially)
-- 2. Create index on (vm_id, snapshot_at)
-- 3. Backfill snapshot_at from existing (year, month, day, hour) data
-- 4. Add NOT NULL constraint once backfill is complete
-- 5. Application code will then use snapshot_at for queries

-- Step 1: Add snapshot_at column
ALTER TABLE bandwidth_snapshots ADD COLUMN IF NOT EXISTS snapshot_at TIMESTAMPTZ;

-- Step 2: Create index on (vm_id, snapshot_at) for efficient time-range queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bandwidth_snapshots_vm_snapshot_at 
    ON bandwidth_snapshots(vm_id, snapshot_at);

-- Step 3: Backfill snapshot_at from existing (year, month, day, hour) columns
-- Use timezone 'UTC' to match the AT TIME ZONE 'UTC' used in existing queries
UPDATE bandwidth_snapshots 
SET snapshot_at = make_timestamp(year, month, day, hour, 0, 0) AT TIME ZONE 'UTC'
WHERE snapshot_at IS NULL;

-- Note: We do NOT add a NOT NULL constraint here because:
-- 1. Large table updates should be done in batches in production
-- 2. The application should be updated to populate snapshot_at on INSERT
-- 3. A follow-up migration can add NOT NULL once all rows are backfilled
