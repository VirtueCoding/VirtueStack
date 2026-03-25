-- Remove progress_message column from tasks table
ALTER TABLE tasks DROP COLUMN IF EXISTS progress_message;

-- Remove the index on (status, created_at DESC)
DROP INDEX CONCURRENTLY IF EXISTS idx_tasks_status_created_at;
