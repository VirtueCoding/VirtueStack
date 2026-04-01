-- Migration 000046 (down): Restore original RLS policies without missing_ok

BEGIN;

SET lock_timeout = '5s';

DROP POLICY IF EXISTS customer_backups ON backups;

CREATE POLICY customer_backups ON backups FOR ALL TO app_customer
    USING (vm_id IN (
        SELECT id FROM vms
        WHERE customer_id = current_setting('app.current_customer_id')::UUID
    ));

DROP POLICY IF EXISTS customer_snapshots ON snapshots;

CREATE POLICY customer_snapshots ON snapshots FOR ALL TO app_customer
    USING (vm_id IN (
        SELECT id FROM vms
        WHERE customer_id = current_setting('app.current_customer_id')::UUID
    ));

COMMIT;
