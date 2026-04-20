-- Remove storage-agnostic columns from node_heartbeats table.

ALTER TABLE node_heartbeats DROP COLUMN IF EXISTS storage_connected;
ALTER TABLE node_heartbeats DROP COLUMN IF EXISTS storage_total_gb;
ALTER TABLE node_heartbeats DROP COLUMN IF EXISTS storage_used_gb;
ALTER TABLE node_heartbeats DROP COLUMN IF EXISTS total_disk_gb;
ALTER TABLE node_heartbeats DROP COLUMN IF EXISTS used_disk_gb;