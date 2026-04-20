-- Remove LVM threshold configuration columns from storage_backends table.

ALTER TABLE storage_backends
DROP COLUMN IF EXISTS lvm_data_percent_threshold,
DROP COLUMN IF EXISTS lvm_metadata_percent_threshold;
