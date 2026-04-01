-- Add LVM threshold configuration columns to storage_backends table.
-- These columns control when alerts are triggered for thin pool usage.

ALTER TABLE storage_backends
ADD COLUMN IF NOT EXISTS lvm_data_percent_threshold INT,
ADD COLUMN IF NOT EXISTS lvm_metadata_percent_threshold INT;

-- Set default thresholds (can be overridden per-backend)
-- Data threshold: 95% - critical alert
-- Metadata threshold: 70% - critical alert
ALTER TABLE storage_backends
ALTER COLUMN lvm_data_percent_threshold SET DEFAULT 95,
ALTER COLUMN lvm_metadata_percent_threshold SET DEFAULT 70;

-- Add CHECK constraints to ensure valid percentages
ALTER TABLE storage_backends
ADD CONSTRAINT check_lvm_data_percent_threshold
    CHECK (lvm_data_percent_threshold IS NULL OR (lvm_data_percent_threshold >= 1 AND lvm_data_percent_threshold <= 100));

ALTER TABLE storage_backends
ADD CONSTRAINT check_lvm_metadata_percent_threshold
    CHECK (lvm_metadata_percent_threshold IS NULL OR (lvm_metadata_percent_threshold >= 1 AND lvm_metadata_percent_threshold <= 100));

-- Backfill existing LVM storage backends with default values
UPDATE storage_backends
SET lvm_data_percent_threshold = 95,
    lvm_metadata_percent_threshold = 70
WHERE type = 'lvm'
  AND (lvm_data_percent_threshold IS NULL OR lvm_metadata_percent_threshold IS NULL);

-- Add comment for documentation
COMMENT ON COLUMN storage_backends.lvm_data_percent_threshold IS
    'Alert threshold for thin pool data usage percentage (1-100). Triggers warning/critical when exceeded.';

COMMENT ON COLUMN storage_backends.lvm_metadata_percent_threshold IS
    'Alert threshold for thin pool metadata usage percentage (1-100). Triggers warning/critical when exceeded.';
