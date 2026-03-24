-- Migration 000051: Extend audit_log partitions through 2030
-- Addresses: F-190
--
-- The existing partitions created in migrations 000001 and 000014 cover up to
-- 2028-01-01 (2027-12). This migration adds monthly partitions for 2028, 2029,
-- and 2030 to ensure uninterrupted audit log writes.
--
-- NOTE: For long-term automation consider using pg_partman to manage partition
-- creation automatically instead of relying on periodic migrations.

BEGIN;

SET lock_timeout = '5s';

-- ============================================================================
-- 2028 partitions (January through December)
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs_2028_01 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_02 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_03 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_04 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-04-01') TO ('2028-05-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_05 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-05-01') TO ('2028-06-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_06 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-06-01') TO ('2028-07-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_07 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-07-01') TO ('2028-08-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_08 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-08-01') TO ('2028-09-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_09 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-09-01') TO ('2028-10-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_10 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-10-01') TO ('2028-11-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_11 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-11-01') TO ('2028-12-01');

CREATE TABLE IF NOT EXISTS audit_logs_2028_12 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-12-01') TO ('2029-01-01');

-- ============================================================================
-- 2029 partitions (January through December)
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs_2029_01 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-01-01') TO ('2029-02-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_02 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-02-01') TO ('2029-03-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_03 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-03-01') TO ('2029-04-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_04 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-04-01') TO ('2029-05-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_05 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-05-01') TO ('2029-06-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_06 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-06-01') TO ('2029-07-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_07 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-07-01') TO ('2029-08-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_08 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-08-01') TO ('2029-09-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_09 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-09-01') TO ('2029-10-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_10 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-10-01') TO ('2029-11-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_11 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-11-01') TO ('2029-12-01');

CREATE TABLE IF NOT EXISTS audit_logs_2029_12 PARTITION OF audit_logs
    FOR VALUES FROM ('2029-12-01') TO ('2030-01-01');

-- ============================================================================
-- 2030 partitions (January through December)
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs_2030_01 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-01-01') TO ('2030-02-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_02 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-02-01') TO ('2030-03-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_03 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-03-01') TO ('2030-04-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_04 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-04-01') TO ('2030-05-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_05 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-05-01') TO ('2030-06-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_06 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-06-01') TO ('2030-07-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_07 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-07-01') TO ('2030-08-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_08 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-08-01') TO ('2030-09-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_09 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-09-01') TO ('2030-10-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_10 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-10-01') TO ('2030-11-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_11 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-11-01') TO ('2030-12-01');

CREATE TABLE IF NOT EXISTS audit_logs_2030_12 PARTITION OF audit_logs
    FOR VALUES FROM ('2030-12-01') TO ('2031-01-01');

-- ============================================================================
-- Automation reminder
-- ============================================================================

COMMENT ON TABLE audit_logs IS
    'Append-only audit log, partitioned by month. '
    'Partitions currently extend through 2030-12. '
    'AUTOMATION NOTE (F-190): Install pg_partman and configure '
    'partman.create_parent() to eliminate the need for manual partition '
    'migrations. Run: SELECT partman.create_parent(''public.audit_logs'', '
    '''timestamp'', ''native'', ''monthly'');';

COMMIT;
