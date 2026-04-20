BEGIN;

SET lock_timeout = '5s';

-- Enable RLS on console_tokens and restrict to owning user
-- Customer users can only see their own tokens
-- Admin users bypass RLS via app_admin role (typically SUPERUSER)
ALTER TABLE console_tokens ENABLE ROW LEVEL SECURITY;

-- Customer policy: users can only access their own tokens
CREATE POLICY customer_console_tokens ON console_tokens FOR ALL TO app_customer
    USING (user_id = current_setting('app.current_customer_id')::UUID AND user_type = 'customer');

-- Grant table access to app_customer for RLS-protected operations
GRANT SELECT, INSERT, DELETE ON console_tokens TO app_customer;

COMMIT;