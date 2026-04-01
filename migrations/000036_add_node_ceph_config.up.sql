-- Add ceph_monitors and ceph_user columns to nodes table for per-node Ceph configuration.
-- These columns are nullable - if NULL, the global config values are used.

-- Add ceph_monitors column (comma-separated list of monitor addresses)
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS ceph_monitors TEXT;

-- Add ceph_user column (Ceph authentication user)
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS ceph_user VARCHAR(100);

COMMENT ON COLUMN nodes.ceph_monitors IS 'Comma-separated list of Ceph monitor addresses (e.g., "10.0.0.1:6789,10.0.0.2:6789"). NULL means use global config.';
COMMENT ON COLUMN nodes.ceph_user IS 'Ceph authentication user (e.g., "client.admin"). NULL means use global config.';