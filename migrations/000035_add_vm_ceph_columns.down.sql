-- Remove ceph_pool and rbd_image columns from vms table.

DROP INDEX IF EXISTS idx_vms_rbd_image;
DROP INDEX IF EXISTS idx_vms_ceph_pool;
ALTER TABLE vms DROP COLUMN IF EXISTS rbd_image;
ALTER TABLE vms DROP COLUMN IF EXISTS ceph_pool;