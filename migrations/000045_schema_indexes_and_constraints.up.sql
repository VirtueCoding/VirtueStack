-- Migration 000045: Schema indexes and constraints
-- Addresses: F-036, F-037, F-038, F-040, F-108, F-111, F-113, F-114,
--             F-116, F-117, F-118, F-119, F-161, F-188, F-189, F-203, F-204

BEGIN;

SET lock_timeout = '5s';

-- ============================================================================
-- F-036: Index on sessions.refresh_token_hash (hot lookup path)
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_sessions_refresh_token_hash ON sessions(refresh_token_hash);

-- ============================================================================
-- F-037: Add updated_at column to admins (required by UpdateBackupCodes)
-- ============================================================================

ALTER TABLE admins ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW();

-- ============================================================================
-- F-038: Partial unique index on vms.hostname (active VMs only)
-- ============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS uq_vms_hostname_active ON vms(hostname) WHERE deleted_at IS NULL;

-- ============================================================================
-- F-040: Foreign key constraints on console_tokens
-- ============================================================================

ALTER TABLE console_tokens
    ADD CONSTRAINT fk_console_tokens_vm_id
        FOREIGN KEY (vm_id) REFERENCES vms(id) ON DELETE CASCADE;

-- console_tokens.user_id can reference either customers or admins; since
-- PostgreSQL does not support polymorphic FKs we constrain only vm_id here.
-- user_type + user_id integrity is enforced at the application layer.

-- ============================================================================
-- F-108: Add computed snapshot_at column to bandwidth_snapshots + index
-- ============================================================================

ALTER TABLE bandwidth_snapshots
    ADD COLUMN IF NOT EXISTS snapshot_at TIMESTAMPTZ
        GENERATED ALWAYS AS (
            make_timestamp(year, month, day, COALESCE(hour, 0), 0, 0) AT TIME ZONE 'UTC'
        ) STORED;

CREATE INDEX IF NOT EXISTS idx_bandwidth_snapshots_vm_snapshot_at
    ON bandwidth_snapshots(vm_id, snapshot_at);

-- ============================================================================
-- F-111: Partial unique index on ip_addresses — at most one primary IP per VM
-- ============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS uq_ip_addresses_vm_primary
    ON ip_addresses(vm_id)
    WHERE is_primary = TRUE AND vm_id IS NOT NULL;

-- ============================================================================
-- F-113: Junction table to replace ip_sets.node_ids UUID[] with proper FKs
-- ============================================================================

CREATE TABLE IF NOT EXISTS ip_set_nodes (
    ip_set_id UUID NOT NULL REFERENCES ip_sets(id) ON DELETE CASCADE,
    node_id   UUID NOT NULL REFERENCES nodes(id)    ON DELETE CASCADE,
    PRIMARY KEY (ip_set_id, node_id)
);

COMMENT ON TABLE ip_set_nodes IS
    'Junction table replacing ip_sets.node_ids (UUID[] with no FK integrity). '
    'Migrate existing ip_sets.node_ids data here and deprecate the array column.';

-- Back-fill existing array data into the junction table (idempotent).
INSERT INTO ip_set_nodes (ip_set_id, node_id)
SELECT s.id, unnest(s.node_ids)
FROM   ip_sets s
WHERE  s.node_ids IS NOT NULL
  AND  array_length(s.node_ids, 1) > 0
ON CONFLICT DO NOTHING;

-- ============================================================================
-- F-114: Add ceph_monitors_array column (proper array type) to nodes
--         Keep ceph_monitors TEXT for backward compatibility.
-- ============================================================================

ALTER TABLE nodes ADD COLUMN IF NOT EXISTS ceph_monitors_array TEXT[];

COMMENT ON COLUMN nodes.ceph_monitors_array IS
    'Ceph monitor addresses as a proper TEXT[] array. '
    'Replaces the comma-delimited ceph_monitors TEXT column. '
    'Populated from ceph_monitors on migration; new writes should use this column.';

-- Back-fill from the comma-delimited column (idempotent).
UPDATE nodes
SET    ceph_monitors_array = string_to_array(ceph_monitors, ',')
WHERE  ceph_monitors IS NOT NULL
  AND  ceph_monitors_array IS NULL;

-- ============================================================================
-- F-116: CHECK constraint ensuring admins.permissions is always a JSON array
-- ============================================================================

ALTER TABLE admins
    ADD CONSTRAINT check_permissions_is_array
        CHECK (permissions IS NULL OR jsonb_typeof(permissions) = 'array');

-- ============================================================================
-- F-117: Non-negative CHECK constraints on plan limit columns
-- ============================================================================

ALTER TABLE plans
    ADD CONSTRAINT check_snapshot_limit_non_negative
        CHECK (snapshot_limit >= 0),
    ADD CONSTRAINT check_backup_limit_non_negative
        CHECK (backup_limit >= 0),
    ADD CONSTRAINT check_iso_upload_limit_non_negative
        CHECK (iso_upload_limit >= 0);

-- ============================================================================
-- F-118: Index on failed_login_attempts.attempted_at for efficient cleanup
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_failed_login_attempts_cleanup
    ON failed_login_attempts(attempted_at);

COMMENT ON INDEX idx_failed_login_attempts_cleanup IS
    'Supports periodic purge of old failed login records, e.g.: '
    'DELETE FROM failed_login_attempts WHERE attempted_at < NOW() - INTERVAL ''30 days'';';

-- ============================================================================
-- F-119: Change customer_api_keys.allowed_ips from TEXT[] to CIDR[]
-- ============================================================================

-- Drop the existing GIN index first (it is on the TEXT[] column).
DROP INDEX IF EXISTS idx_customer_api_keys_allowed_ips;

ALTER TABLE customer_api_keys
    ALTER COLUMN allowed_ips TYPE CIDR[] USING allowed_ips::cidr[];

-- Re-create GIN index on the new CIDR[] column.
CREATE INDEX IF NOT EXISTS idx_customer_api_keys_allowed_ips
    ON customer_api_keys USING GIN(allowed_ips);

-- ============================================================================
-- F-161: Index on node_heartbeats.timestamp for efficient retention queries
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_node_heartbeats_timestamp
    ON node_heartbeats(timestamp);

COMMENT ON INDEX idx_node_heartbeats_timestamp IS
    'Supports efficient cleanup of old heartbeat rows, e.g.: '
    'DELETE FROM node_heartbeats WHERE timestamp < NOW() - INTERVAL ''7 days'';';

-- ============================================================================
-- F-188: CHECK constraints on node_heartbeats percent columns (0-100)
-- ============================================================================

ALTER TABLE node_heartbeats
    ADD CONSTRAINT check_cpu_percent
        CHECK (cpu_percent IS NULL OR cpu_percent BETWEEN 0 AND 100),
    ADD CONSTRAINT check_memory_percent
        CHECK (memory_percent IS NULL OR memory_percent BETWEEN 0 AND 100),
    ADD CONSTRAINT check_disk_percent
        CHECK (disk_percent IS NULL OR disk_percent BETWEEN 0 AND 100);

-- ============================================================================
-- F-189: NOT NULL on backups.status + compound index on (vm_id, status)
-- ============================================================================

-- Ensure no NULLs exist before enforcing NOT NULL.
UPDATE backups SET status = 'creating' WHERE status IS NULL;

ALTER TABLE backups ALTER COLUMN status SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_backups_vm_status ON backups(vm_id, status);

-- ============================================================================
-- F-203: NOT NULL on customers.status
-- ============================================================================

-- Ensure no NULLs before enforcing.
UPDATE customers SET status = 'active' WHERE status IS NULL;

ALTER TABLE customers ALTER COLUMN status SET NOT NULL;

-- ============================================================================
-- F-204: NOT NULL on admins.role
-- ============================================================================

-- Ensure no NULLs before enforcing.
UPDATE admins SET role = 'admin' WHERE role IS NULL;

ALTER TABLE admins ALTER COLUMN role SET NOT NULL;

COMMIT;
