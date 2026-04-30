-- Add ceph_pool and rbd_image columns to vms table for VM disk management.
-- These columns store the Ceph pool and RBD image name for each VM's disk.

-- Add ceph_pool column with default matching nodes table default
ALTER TABLE vms ADD COLUMN IF NOT EXISTS ceph_pool VARCHAR(100) DEFAULT 'vs-vms';

-- Add rbd_image column for the RBD image name (e.g., "vm-{uuid}")
ALTER TABLE vms ADD COLUMN IF NOT EXISTS rbd_image VARCHAR(200);

-- Add index on ceph_pool for efficient filtering by pool
CREATE INDEX IF NOT EXISTS idx_vms_ceph_pool ON vms(ceph_pool);

-- Add index on rbd_image for lookups by image name
CREATE INDEX IF NOT EXISTS idx_vms_rbd_image ON vms(rbd_image);

COMMENT ON COLUMN vms.ceph_pool IS 'Ceph pool name for the VM disk';
COMMENT ON COLUMN vms.rbd_image IS 'RBD image name for the VM disk (e.g., "vm-{uuid}")';