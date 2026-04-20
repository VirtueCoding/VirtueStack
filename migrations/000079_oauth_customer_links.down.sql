SET lock_timeout = '5s';

DROP TABLE IF EXISTS customer_oauth_links;

ALTER TABLE customers DROP COLUMN IF EXISTS auth_provider;

-- Restore NOT NULL. Existing OAuth-only rows will need a dummy hash first.
-- This is a destructive rollback — OAuth-only accounts will be broken.
UPDATE customers SET password_hash = '' WHERE password_hash IS NULL;
ALTER TABLE customers ALTER COLUMN password_hash SET NOT NULL;
