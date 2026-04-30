-- Migration 000048: console_tokens — document user_id polymorphism; add FK note
-- Addresses: F-112
--
-- Migration 000039 was created without a transaction wrapper (BEGIN/COMMIT).
-- That migration has already run and cannot be changed. This migration adds a
-- table comment documenting the polymorphic user_id design and confirms the
-- schema is consistent, enclosed in a proper transaction.
--
-- The vm_id FK was added in migration 000045 (F-040).
-- The user_id column intentionally has no FK because it references either
-- customers(id) or admins(id) depending on user_type.  Integrity is enforced
-- at the application layer.

BEGIN;

SET lock_timeout = '5s';

COMMENT ON COLUMN console_tokens.user_id IS
    'UUID of the authenticated user. References customers(id) when '
    'user_type = ''customer'', or admins(id) when user_type = ''admin''. '
    'No database-level FK due to the polymorphic relationship; integrity is '
    'enforced by the application layer.';

COMMENT ON COLUMN console_tokens.vm_id IS
    'FK → vms(id) ON DELETE CASCADE (constraint added in migration 000045).';

COMMIT;
