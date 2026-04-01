-- VirtueStack Password Reset Migration
-- Creates table for password reset tokens

BEGIN;

SET lock_timeout = '5s';

-- ============================================================================
-- PASSWORD_RESETS TABLE
-- Stores password reset tokens for customers
-- ============================================================================

CREATE TABLE IF NOT EXISTS password_resets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    user_type VARCHAR(20) NOT NULL DEFAULT 'customer',
    token_hash VARCHAR(64) NOT NULL,  -- SHA-256 hash (64 hex chars)
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,

    CONSTRAINT password_resets_token_hash_valid CHECK (length(token_hash) = 64),
    CONSTRAINT password_resets_user_type_valid CHECK (user_type IN ('customer', 'admin'))
);

-- Index for looking up reset by token hash (most common query)
CREATE INDEX IF NOT EXISTS idx_password_resets_token_hash ON password_resets(token_hash);

-- Index for finding resets by user
CREATE INDEX IF NOT EXISTS idx_password_resets_user ON password_resets(user_id, user_type);

-- Index for cleanup of expired tokens
CREATE INDEX IF NOT EXISTS idx_password_resets_expires_at ON password_resets(expires_at);

-- ============================================================================
-- GRANT PERMISSIONS
-- ============================================================================

-- Grant permissions to app_user
GRANT SELECT, INSERT, UPDATE ON password_resets TO app_user;

-- Grant permissions to app_customer for RLS (if needed)
GRANT SELECT, INSERT ON password_resets TO app_customer;

-- ============================================================================
-- ROW LEVEL SECURITY (QG-02)
-- Enable RLS immediately after granting permissions to prevent gap window
-- where app_customer could access all rows before RLS is enforced.
-- ============================================================================

ALTER TABLE password_resets ENABLE ROW LEVEL SECURITY;

-- Create policy for customer access: customers can only access their own resets
CREATE POLICY customer_password_resets ON password_resets FOR ALL TO app_customer
    USING (user_id = current_setting('app.current_customer_id')::UUID);

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE password_resets IS 'Password reset tokens for customers and admins';
COMMENT ON COLUMN password_resets.token_hash IS 'SHA-256 hash of the reset token (not reversible)';
COMMENT ON COLUMN password_resets.expires_at IS 'Token expiration timestamp (24 hours from creation)';
COMMENT ON COLUMN password_resets.used_at IS 'When the token was used (null if unused)';

COMMIT;