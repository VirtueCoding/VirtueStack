-- VirtueStack Session Re-authentication Migration (Rollback)
-- Removes last_reauth_at column from sessions table

BEGIN;

-- Drop index first
DROP INDEX IF EXISTS idx_sessions_last_reauth_at;

-- Drop column
ALTER TABLE sessions
    DROP COLUMN IF EXISTS last_reauth_at;

COMMIT;
