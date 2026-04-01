-- Add storage-agnostic columns to node_heartbeats table.
-- These columns were missing from the initial schema but are defined in the models.

-- Add storage connection status
ALTER TABLE node_heartbeats ADD COLUMN IF NOT EXISTS storage_connected BOOLEAN DEFAULT false;

-- Add storage capacity metrics
ALTER TABLE node_heartbeats ADD COLUMN IF NOT EXISTS storage_total_gb BIGINT DEFAULT 0;
ALTER TABLE node_heartbeats ADD COLUMN IF NOT EXISTS storage_used_gb BIGINT DEFAULT 0;

-- Add local disk metrics (for non-shared storage)
ALTER TABLE node_heartbeats ADD COLUMN IF NOT EXISTS total_disk_gb BIGINT DEFAULT 0;
ALTER TABLE node_heartbeats ADD COLUMN IF NOT EXISTS used_disk_gb BIGINT DEFAULT 0;