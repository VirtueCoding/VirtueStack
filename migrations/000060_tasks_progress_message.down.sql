SET lock_timeout = '5s';

-- Remove progress_message column from tasks table
ALTER TABLE tasks DROP COLUMN IF EXISTS progress_message;

-- Do not drop idx_tasks_status_created_at here; it belongs to migration 000029.
