-- Remove totp_backup_codes_shown column from customers table

BEGIN;

SET lock_timeout = '5s';

ALTER TABLE customers
    DROP COLUMN IF EXISTS totp_backup_codes_shown;

COMMIT;