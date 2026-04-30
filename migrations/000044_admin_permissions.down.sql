BEGIN;

-- Remove permissions column from admins table.

SET lock_timeout = '5s';

ALTER TABLE admins DROP COLUMN IF EXISTS permissions;

COMMIT;
