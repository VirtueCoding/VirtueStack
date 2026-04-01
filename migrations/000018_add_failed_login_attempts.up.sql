BEGIN;

SET lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS failed_login_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip_address INET
);

CREATE INDEX IF NOT EXISTS idx_failed_login_attempts_email_time 
ON failed_login_attempts(email, attempted_at);

-- Add comment for documentation
COMMENT ON TABLE failed_login_attempts IS 'Tracks failed login attempts for account lockout protection';

-- ============================================================================
-- PERMISSIONS
-- ============================================================================

GRANT SELECT, INSERT, UPDATE, DELETE ON failed_login_attempts TO app_user;

COMMIT;