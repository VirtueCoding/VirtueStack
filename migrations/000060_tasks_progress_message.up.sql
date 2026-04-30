SET lock_timeout = '5s';

-- Add progress_message column to tasks table for granular progress descriptions.
-- Previously, progress messages passed to UpdateProgress() were discarded.
-- This column stores the message for display in task detail UI.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS progress_message TEXT;

-- Index idx_tasks_status_created_at is already created by migration 000029.
-- Keep this migration transaction-safe by avoiding CREATE INDEX CONCURRENTLY here.
