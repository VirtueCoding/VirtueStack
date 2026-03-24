-- Migration 000047: Fix password_resets to support both customers and admins
-- Addresses: F-110
--
-- The original table (000011) has user_id referencing customers(id) ON DELETE CASCADE
-- but user_type = 'admin' references the admins table — there is no FK for that.
-- This migration:
--   1. Drops the monolithic user_id FK.
--   2. Adds customer_id (nullable FK → customers) and admin_id (nullable FK → admins).
--   3. Back-fills from the old user_id column using user_type.
--   4. Adds a CHECK constraint ensuring exactly one of the two FKs is non-NULL.
--
-- The old user_id column is kept (nullable) for the duration of any rollout; a
-- future migration can drop it once the application is updated.

BEGIN;

SET lock_timeout = '5s';

-- Step 1: Drop the existing FK constraint on user_id.
-- The constraint name may vary; use IF EXISTS on each possible name.
ALTER TABLE password_resets
    DROP CONSTRAINT IF EXISTS password_resets_user_id_fkey;

-- Step 2: Add typed FK columns.
ALTER TABLE password_resets
    ADD COLUMN IF NOT EXISTS customer_id UUID
        REFERENCES customers(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS admin_id UUID
        REFERENCES admins(id) ON DELETE CASCADE;

-- Step 3: Back-fill from user_id / user_type.
UPDATE password_resets
SET customer_id = user_id
WHERE user_type = 'customer' AND customer_id IS NULL;

UPDATE password_resets
SET admin_id = user_id
WHERE user_type = 'admin' AND admin_id IS NULL;

-- Step 4: Enforce that exactly one FK is populated.
ALTER TABLE password_resets
    ADD CONSTRAINT check_password_resets_one_owner
        CHECK (
            (customer_id IS NOT NULL AND admin_id IS NULL)
            OR
            (admin_id IS NOT NULL AND customer_id IS NULL)
        );

-- Step 5: Add indexes for the new FK columns.
CREATE INDEX IF NOT EXISTS idx_password_resets_customer_id
    ON password_resets(customer_id);

CREATE INDEX IF NOT EXISTS idx_password_resets_admin_id
    ON password_resets(admin_id);

COMMENT ON COLUMN password_resets.customer_id IS
    'FK to customers(id). Populated when user_type = ''customer''.';
COMMENT ON COLUMN password_resets.admin_id IS
    'FK to admins(id). Populated when user_type = ''admin''.';
COMMENT ON COLUMN password_resets.user_id IS
    'Deprecated. Use customer_id or admin_id. Kept for rollout compatibility.';

COMMIT;
