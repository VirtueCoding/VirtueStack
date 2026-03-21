-- Migration: 000037_admin_backup_schedules.up.sql
-- Purpose: Add admin backup scheduling capabilities with source tracking
--
-- Changes:
-- 1. Create admin_backup_schedules table for mass backup campaigns
-- 2. Add source column to backups (manual, customer_schedule, admin_schedule)
-- 3. Add admin_schedule_id foreign key to backups
-- 4. Drop redundant 'type' column (only ever had 'full' value)

-- ============================================================================
-- Step 1: Create admin_backup_schedules table
-- ============================================================================

CREATE TABLE admin_backup_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    frequency VARCHAR(20) NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    retention_count INTEGER NOT NULL DEFAULT 3,

    -- Targeting options (at least one required)
    target_all BOOLEAN DEFAULT FALSE,
    target_plan_ids UUID[],
    target_node_ids UUID[],
    target_customer_ids UUID[],

    -- Schedule configuration
    active BOOLEAN DEFAULT TRUE,
    next_run_at TIMESTAMPTZ NOT NULL,
    last_run_at TIMESTAMPTZ,

    -- Metadata
    created_by UUID REFERENCES admins(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for active schedule queries
CREATE INDEX idx_admin_backup_schedules_next_run ON admin_backup_schedules(next_run_at) WHERE active = TRUE;

-- ============================================================================
-- Step 2: Add source column to backups table
-- ============================================================================

-- Add source column with CHECK constraint for valid values
ALTER TABLE backups ADD COLUMN source VARCHAR(20) DEFAULT 'manual' CHECK (source IN ('manual', 'customer_schedule', 'admin_schedule'));

-- Add admin_schedule_id foreign key
ALTER TABLE backups ADD COLUMN admin_schedule_id UUID REFERENCES admin_backup_schedules(id) ON DELETE SET NULL;

-- Create indexes for filtering
CREATE INDEX idx_backups_source ON backups(source);
CREATE INDEX idx_backups_admin_schedule ON backups(admin_schedule_id);

-- Update existing backups to have source = 'manual'
UPDATE backups SET source = 'manual' WHERE source IS NULL;

-- ============================================================================
-- Step 3: Add storage_backend and file_path columns if they don't exist
-- These were added in migration 000019 but we verify they exist for safety
-- ============================================================================

-- Note: These columns should already exist from migration 000019
-- The following is a defensive check that won't fail if columns exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'backups' AND column_name = 'storage_backend') THEN
        ALTER TABLE backups ADD COLUMN storage_backend VARCHAR(20) DEFAULT 'ceph';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'backups' AND column_name = 'file_path') THEN
        ALTER TABLE backups ADD COLUMN file_path TEXT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'backups' AND column_name = 'snapshot_name') THEN
        ALTER TABLE backups ADD COLUMN snapshot_name VARCHAR(100);
    END IF;
END $$;

-- ============================================================================
-- Step 4: Drop the redundant 'type' column
-- This column only ever had value 'full' and is now superseded by 'source'
-- ============================================================================

-- First drop the CHECK constraint on the type column
ALTER TABLE backups DROP CONSTRAINT IF EXISTS backups_type_check;

-- Then drop the column
ALTER TABLE backups DROP COLUMN IF EXISTS type;

-- ============================================================================
-- Step 5: Add constraints for data integrity
-- ============================================================================

-- Ensure retention_count is at least 1
ALTER TABLE admin_backup_schedules ADD CONSTRAINT chk_retention_count_positive CHECK (retention_count >= 1);

-- Ensure at least one target is specified when not targeting all
ALTER TABLE admin_backup_schedules ADD CONSTRAINT chk_has_target CHECK (
    target_all = TRUE OR
    array_length(target_plan_ids, 1) > 0 OR
    array_length(target_node_ids, 1) > 0 OR
    array_length(target_customer_ids, 1) > 0
);

-- ============================================================================
-- Step 6: Grant permissions
-- ============================================================================

GRANT SELECT, INSERT, UPDATE, DELETE ON admin_backup_schedules TO app_user;
GRANT SELECT ON admin_backup_schedules TO app_customer;

-- Grant on backups table for new columns
GRANT SELECT, INSERT, UPDATE, DELETE ON backups TO app_user;
GRANT SELECT ON backups TO app_customer;