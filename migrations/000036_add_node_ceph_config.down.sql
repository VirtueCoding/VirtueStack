-- Remove ceph_monitors and ceph_user columns from nodes table.

ALTER TABLE nodes DROP COLUMN IF EXISTS ceph_user;
ALTER TABLE nodes DROP COLUMN IF EXISTS ceph_monitors;