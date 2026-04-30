BEGIN;

-- Add permissions column to admins table for fine-grained access control.
-- When NULL or empty, the admin uses the default permissions for their role.
-- This allows per-admin permission overrides when needed.

SET lock_timeout = '5s';

ALTER TABLE admins ADD COLUMN IF NOT EXISTS permissions JSONB;

COMMENT ON COLUMN admins.permissions IS 'Array of permission strings for fine-grained access control. NULL or empty array means use role-based defaults.';

COMMIT;
