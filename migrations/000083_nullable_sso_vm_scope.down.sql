BEGIN;

SET lock_timeout = '5s';

DELETE FROM sso_tokens WHERE vm_id IS NULL;

ALTER TABLE sso_tokens
    ALTER COLUMN vm_id SET NOT NULL;

COMMIT;
