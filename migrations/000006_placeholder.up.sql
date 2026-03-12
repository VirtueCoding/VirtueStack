BEGIN;

CREATE TABLE IF NOT EXISTS backup_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    frequency VARCHAR(20) NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    retention_count INTEGER NOT NULL DEFAULT 30,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    next_run_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_backup_schedules_vm_id
    ON backup_schedules(vm_id);

CREATE INDEX IF NOT EXISTS idx_backup_schedules_customer_id
    ON backup_schedules(customer_id);

CREATE INDEX IF NOT EXISTS idx_backup_schedules_active_next_run
    ON backup_schedules(active, next_run_at)
    WHERE active = TRUE;

CREATE INDEX IF NOT EXISTS idx_backups_vm_created
    ON backups(vm_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_backups_expires_at
    ON backups(expires_at)
    WHERE expires_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_snapshots_vm_created
    ON snapshots(vm_id, created_at DESC);

COMMIT;
