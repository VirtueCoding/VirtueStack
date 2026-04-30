BEGIN;

SET lock_timeout = '5s';

-- Revoke grants added in up migration
REVOKE SELECT, INSERT, DELETE ON console_tokens FROM app_customer;

-- Drop RLS policy and disable RLS
DROP POLICY IF EXISTS customer_console_tokens ON console_tokens;
ALTER TABLE console_tokens DISABLE ROW LEVEL SECURITY;

COMMIT;