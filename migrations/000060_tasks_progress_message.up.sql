-- Add progress_message column to tasks table for granular progress descriptions.
-- Previously, progress messages passed to UpdateProgress() were discarded.
-- This column stores the message for display in task detail UI.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS progress_message TEXT;

-- Add an index on (status, created_at DESC) for efficient pending/running task queries.
-- This improves performance for task list queries that filter by status and order by creation time.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tasks_status_created_at 
    ON tasks(status, created_at DESC);
