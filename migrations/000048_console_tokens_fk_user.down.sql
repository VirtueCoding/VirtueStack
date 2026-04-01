-- Migration 000048 (down): Remove console_tokens column comments

BEGIN;

SET lock_timeout = '5s';

COMMENT ON COLUMN console_tokens.user_id IS NULL;
COMMENT ON COLUMN console_tokens.vm_id IS NULL;

COMMIT;
