-- Migration: 000037_admin_backup_schedules.down.sql
-- Purpose: Rollback admin backup scheduling capabilities

-- ============================================================================
-- Step 1: Restore the 'type' column to backups
-- ============================================================================

ALTER TABLE backups ADD COLUMN type VARCHAR(20) DEFAULT 'full' CHECK (type IN ('full'));

-- Update all existing backups to have type = 'full'
UPDATE backups SET type = 'full' WHERE type IS NULL;

-- ============================================================================
-- Step 2: Remove source and admin_schedule_id columns from backups
-- ============================================================================

DROP INDEX IF EXISTS idx_backups_admin_schedule;
DROP INDEX IF EXISTS idx_backups_source;

ALTER TABLE backups DROP COLUMN IF EXISTS admin_schedule_id;
ALTER TABLE backups DROP COLUMN IF EXISTS source;

-- ============================================================================
-- Step 3: Drop admin_backup_schedules table
-- ============================================================================

DROP TABLE IF EXISTS admin_backup_schedules;