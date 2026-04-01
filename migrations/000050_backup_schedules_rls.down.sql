-- Migration 000050 (down): Revert backup_schedules RLS FORCE and policy

BEGIN;

SET lock_timeout = '5s';

-- Drop the policy re-created in the up migration.
DROP POLICY IF EXISTS customer_backup_schedules ON backup_schedules;

-- Restore the original policy (without missing_ok, matching migration 000028).
CREATE POLICY customer_backup_schedules ON backup_schedules FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Revert FORCE RLS; RLS itself stays enabled (was already on from 000028).
ALTER TABLE backup_schedules NO FORCE ROW LEVEL SECURITY;

COMMENT ON TABLE backup_schedules IS NULL;

COMMIT;
