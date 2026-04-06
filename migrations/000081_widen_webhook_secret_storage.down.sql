BEGIN;

SET lock_timeout = '5s';

ALTER TABLE webhooks
    ALTER COLUMN secret_hash TYPE VARCHAR(128);

COMMENT ON COLUMN webhooks.secret_hash IS 'Hashed secret used for HMAC-SHA256 signature generation';

COMMIT;
