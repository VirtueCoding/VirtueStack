BEGIN;

-- Remove unused legacy columns from plans table.
-- These were superseded by snapshot_limit, backup_limit, iso_upload_limit
-- and were never exposed in the Go model.

SET lock_timeout = '5s';

ALTER TABLE plans DROP COLUMN IF EXISTS bandwidth_overage_speed_mbps;
ALTER TABLE plans DROP COLUMN IF EXISTS max_ipv4;
ALTER TABLE plans DROP COLUMN IF EXISTS max_ipv6_slash64;
ALTER TABLE plans DROP COLUMN IF EXISTS max_snapshots;
ALTER TABLE plans DROP COLUMN IF EXISTS max_backups;
ALTER TABLE plans DROP COLUMN IF EXISTS max_iso_count;
ALTER TABLE plans DROP COLUMN IF EXISTS max_iso_gb;

COMMIT;