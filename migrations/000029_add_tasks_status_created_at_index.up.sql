-- Add composite index on tasks(status, created_at DESC) to speed up ORDER BY queries
-- that filter by status (e.g. fetching pending or running tasks sorted by creation time).
--
-- Note: CREATE INDEX CONCURRENTLY cannot run inside a transaction block, so this file
-- has no BEGIN/COMMIT. See migration 000031 for detailed discussion.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tasks_status_created_at
    ON tasks(status, created_at DESC);
