-- Migration 000045 (down): Reverse schema indexes and constraints

BEGIN;

SET lock_timeout = '5s';

-- F-204
ALTER TABLE admins ALTER COLUMN role DROP NOT NULL;

-- F-203
ALTER TABLE customers ALTER COLUMN status DROP NOT NULL;

-- F-189
ALTER TABLE backups ALTER COLUMN status DROP NOT NULL;
DROP INDEX IF EXISTS idx_backups_vm_status;

-- F-188
ALTER TABLE node_heartbeats
    DROP CONSTRAINT IF EXISTS check_cpu_percent,
    DROP CONSTRAINT IF EXISTS check_memory_percent,
    DROP CONSTRAINT IF EXISTS check_disk_percent;

-- F-161
DROP INDEX IF EXISTS idx_node_heartbeats_timestamp;

-- F-119: Revert CIDR[] back to TEXT[]
DROP INDEX IF EXISTS idx_customer_api_keys_allowed_ips;
ALTER TABLE customer_api_keys
    ALTER COLUMN allowed_ips TYPE TEXT[] USING allowed_ips::text[];
CREATE INDEX IF NOT EXISTS idx_customer_api_keys_allowed_ips
    ON customer_api_keys USING GIN(allowed_ips);

-- F-118
DROP INDEX IF EXISTS idx_failed_login_attempts_cleanup;

-- F-117
ALTER TABLE plans
    DROP CONSTRAINT IF EXISTS check_snapshot_limit_non_negative,
    DROP CONSTRAINT IF EXISTS check_backup_limit_non_negative,
    DROP CONSTRAINT IF EXISTS check_iso_upload_limit_non_negative;

-- F-116
ALTER TABLE admins DROP CONSTRAINT IF EXISTS check_permissions_is_array;

-- F-114
ALTER TABLE nodes DROP COLUMN IF EXISTS ceph_monitors_array;

-- F-113
DROP TABLE IF EXISTS ip_set_nodes;

-- F-111
DROP INDEX IF EXISTS uq_ip_addresses_vm_primary;

-- F-108
DROP INDEX IF EXISTS idx_bandwidth_snapshots_vm_snapshot_at;
ALTER TABLE bandwidth_snapshots DROP COLUMN IF EXISTS snapshot_at;

-- F-040
ALTER TABLE console_tokens DROP CONSTRAINT IF EXISTS fk_console_tokens_vm_id;

-- F-038
DROP INDEX IF EXISTS uq_vms_hostname_active;

-- F-037
ALTER TABLE admins DROP COLUMN IF EXISTS updated_at;

-- F-036
DROP INDEX IF EXISTS idx_sessions_refresh_token_hash;

COMMIT;
