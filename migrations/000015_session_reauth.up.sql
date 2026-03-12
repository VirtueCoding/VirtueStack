-- VirtueStack Session Re-authentication Migration
-- Adds last_reauth_at column to sessions table for tracking re-authentication timestamps
-- Reference: RBAC destructive action re-authentication requirement

BEGIN;

-- ============================================================================
-- SESSIONS TABLE UPDATE
-- Add last_reauth_at column to track when user last re-authenticated
-- ============================================================================

ALTER TABLE sessions
    ADD COLUMN last_reauth_at TIMESTAMPTZ NULL;

-- Index for efficient lookup of sessions needing re-auth check
CREATE INDEX idx_sessions_last_reauth_at ON sessions(last_reauth_at)
    WHERE last_reauth_at IS NOT NULL;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN sessions.last_reauth_at IS 'Timestamp of last re-authentication for destructive actions (5-minute window)';

COMMIT;
