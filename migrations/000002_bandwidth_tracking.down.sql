-- VirtueStack Bandwidth Tracking Migration (Rollback)
-- Removes bandwidth tracking tables

BEGIN;

-- Drop views
DROP VIEW IF EXISTS v_bandwidth_current;
DROP VIEW IF EXISTS v_bandwidth_throttled;

-- Drop tables (cascades to indexes, triggers, policies)
DROP TABLE IF EXISTS bandwidth_snapshots;
DROP TABLE IF EXISTS bandwidth_throttle;
DROP TABLE IF EXISTS bandwidth_usage;

-- Drop trigger function if no longer needed
-- Note: This might be used by other tables, so we check
DO $$
BEGIN
    -- Only drop if not used by other tables
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger 
        WHERE tgname = 'bandwidth_usage_updated_at' 
        OR tgname = 'bandwidth_throttle_updated_at'
    ) THEN
        DROP FUNCTION IF EXISTS update_bandwidth_updated_at();
    END IF;
END $$;

COMMIT;
