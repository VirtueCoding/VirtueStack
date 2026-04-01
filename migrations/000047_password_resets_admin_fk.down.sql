-- Migration 000047 (down): Revert password_resets to single user_id FK

BEGIN;

SET lock_timeout = '5s';

-- Remove the mutual-exclusion constraint and new indexes.
ALTER TABLE password_resets
    DROP CONSTRAINT IF EXISTS check_password_resets_one_owner;

DROP INDEX IF EXISTS idx_password_resets_admin_id;
DROP INDEX IF EXISTS idx_password_resets_customer_id;

-- Restore the original FK on user_id → customers(id) ON DELETE CASCADE.
-- This will fail if admin rows still exist; purge them first in that case.
ALTER TABLE password_resets
    ADD CONSTRAINT password_resets_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES customers(id) ON DELETE CASCADE;

-- Drop the typed FK columns.
ALTER TABLE password_resets
    DROP COLUMN IF EXISTS admin_id,
    DROP COLUMN IF EXISTS customer_id;

COMMIT;
