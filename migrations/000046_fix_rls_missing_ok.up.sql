-- Migration 000046: Fix RLS policies to use current_setting missing_ok=true
-- Addresses: F-041
--
-- The original policies in 000028 use current_setting('app.current_customer_id')
-- which raises an ERROR when the GUC is not set (e.g., direct admin queries).
-- Replacing them with the two-argument form (missing_ok = true) makes the call
-- return NULL instead of throwing, so admin connections are unaffected.

BEGIN;

SET lock_timeout = '5s';

-- ============================================================================
-- backups: replace policy using missing_ok form
-- ============================================================================

DROP POLICY IF EXISTS customer_backups ON backups;

CREATE POLICY customer_backups ON backups FOR ALL TO app_customer
    USING (vm_id IN (
        SELECT id FROM vms
        WHERE customer_id = current_setting('app.current_customer_id', true)::UUID
    ));

-- ============================================================================
-- snapshots: replace policy using missing_ok form
-- ============================================================================

DROP POLICY IF EXISTS customer_snapshots ON snapshots;

CREATE POLICY customer_snapshots ON snapshots FOR ALL TO app_customer
    USING (vm_id IN (
        SELECT id FROM vms
        WHERE customer_id = current_setting('app.current_customer_id', true)::UUID
    ));

COMMIT;
