BEGIN;

DROP INDEX IF EXISTS idx_snapshots_vm_created;
DROP INDEX IF EXISTS idx_backups_expires_at;
DROP INDEX IF EXISTS idx_backups_vm_created;
DROP INDEX IF EXISTS idx_backup_schedules_active_next_run;
DROP INDEX IF EXISTS idx_backup_schedules_customer_id;
DROP INDEX IF EXISTS idx_backup_schedules_vm_id;
DROP TABLE IF EXISTS backup_schedules;

COMMIT;
