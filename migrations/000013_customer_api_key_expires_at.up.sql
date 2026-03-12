-- Add expires_at column to customer_api_keys for key expiration support

BEGIN;

ALTER TABLE customer_api_keys ADD COLUMN expires_at TIMESTAMPTZ;

-- Add index for finding expired keys
CREATE INDEX idx_customer_api_keys_expires_at ON customer_api_keys(expires_at) WHERE expires_at IS NOT NULL;

COMMIT;