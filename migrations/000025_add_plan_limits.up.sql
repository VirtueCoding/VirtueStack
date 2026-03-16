ALTER TABLE plans ADD COLUMN IF NOT EXISTS snapshot_limit INT NOT NULL DEFAULT 2;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS backup_limit INT NOT NULL DEFAULT 2;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS iso_upload_limit INT NOT NULL DEFAULT 2;

COMMENT ON COLUMN plans.snapshot_limit IS 'Maximum number of snapshots allowed per VM on this plan. Default: 2.';
COMMENT ON COLUMN plans.backup_limit IS 'Maximum number of backups allowed per VM on this plan. Default: 2.';
COMMENT ON COLUMN plans.iso_upload_limit IS 'Maximum number of ISO uploads allowed per VM on this plan. Default: 2.';
