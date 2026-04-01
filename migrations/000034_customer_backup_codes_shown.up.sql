-- Add totp_backup_codes_shown column to customers table
-- This column tracks whether the customer has seen their TOTP backup codes

BEGIN;

SET lock_timeout = '5s';

ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS totp_backup_codes_shown BOOLEAN DEFAULT FALSE;

COMMENT ON COLUMN customers.totp_backup_codes_shown IS 'Whether the customer has viewed their TOTP backup codes at least once';

COMMIT;