BEGIN;

SET lock_timeout = '5s';

ALTER TABLE sso_tokens
    ALTER COLUMN vm_id DROP NOT NULL;

COMMIT;
