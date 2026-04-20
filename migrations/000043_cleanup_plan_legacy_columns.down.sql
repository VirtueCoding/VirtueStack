BEGIN;

-- Restore legacy columns that were removed (for rollback purposes only).
-- These columns are not used by the application.

SET lock_timeout = '5s';

ALTER TABLE plans ADD COLUMN IF NOT EXISTS bandwidth_overage_speed_mbps INTEGER DEFAULT 5;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_ipv4 INTEGER DEFAULT 1;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_ipv6_slash64 INTEGER DEFAULT 1;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_snapshots INTEGER DEFAULT 3;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_backups INTEGER DEFAULT 1;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_iso_count INTEGER DEFAULT 1;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_iso_gb INTEGER DEFAULT 5;

COMMIT;