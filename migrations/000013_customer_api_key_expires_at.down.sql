-- Remove expires_at column from customer_api_keys

BEGIN;

DROP INDEX IF EXISTS idx_customer_api_keys_expires_at;
ALTER TABLE customer_api_keys DROP COLUMN IF EXISTS expires_at;

COMMIT;