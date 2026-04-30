ALTER TABLE plans DROP COLUMN IF EXISTS snapshot_limit;
ALTER TABLE plans DROP COLUMN IF EXISTS backup_limit;
ALTER TABLE plans DROP COLUMN IF EXISTS iso_upload_limit;
