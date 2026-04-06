BEGIN;

SET lock_timeout = '5s';

ALTER TABLE webhooks
    ALTER COLUMN secret_hash TYPE TEXT;

COMMENT ON COLUMN webhooks.secret_hash IS 'Encrypted customer webhook secret used for HMAC-SHA256 signature generation';

COMMIT;
