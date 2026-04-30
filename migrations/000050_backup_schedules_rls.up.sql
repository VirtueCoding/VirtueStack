-- Migration 000050: Ensure RLS is enabled on backup_schedules
-- Addresses: F-115
--
-- Migration 000006 granted INSERT, UPDATE on backup_schedules to app_customer
-- before RLS was introduced (migration 000028 enabled RLS on this table).
-- This migration explicitly re-enables RLS with FORCE so that even the table
-- owner cannot bypass it, and confirms the policy is present.

BEGIN;

SET lock_timeout = '5s';

-- Enable (or re-confirm) RLS on backup_schedules.
-- FORCE ensures the table owner is also subject to policies.
ALTER TABLE backup_schedules ENABLE ROW LEVEL SECURITY;
ALTER TABLE backup_schedules FORCE ROW LEVEL SECURITY;

-- Ensure the customer isolation policy exists (idempotent: drop then recreate).
DROP POLICY IF EXISTS customer_backup_schedules ON backup_schedules;

CREATE POLICY customer_backup_schedules ON backup_schedules FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id', true)::UUID);

COMMENT ON TABLE backup_schedules IS
    'Customer backup schedules. RLS enforced for app_customer role. '
    'FORCE ROW LEVEL SECURITY ensures even the table owner cannot bypass policies.';

COMMIT;
