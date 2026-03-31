BEGIN;
SET lock_timeout = '5s';
-- Drop old plan limit columns superseded by migration 025
ALTER TABLE plans DROP COLUMN IF EXISTS max_snapshots;
ALTER TABLE plans DROP COLUMN IF EXISTS max_backups;
ALTER TABLE plans DROP COLUMN IF EXISTS max_iso_count;
COMMIT;
