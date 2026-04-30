-- ===========================================================================
-- Audit Log Default Partition and Extended Partitions
-- ===========================================================================
-- QG-16: This migration creates:
-- 1. A DEFAULT partition to catch any records outside existing partition ranges
-- 2. Partitions through 2028-03
--
-- IMPORTANT: Partitions are manually created through 2028-03. For production:
-- - Consider pg_partman extension for automated partition management
-- - Or implement a scheduled task to create future partitions quarterly
-- - Without automation, the DEFAULT partition will absorb all records after 2028-03,
--   which degrades query performance as it grows unbounded
-- ===========================================================================

BEGIN;

SET lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS audit_logs_default PARTITION OF audit_logs DEFAULT;

CREATE TABLE audit_logs_2028_01 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');

CREATE TABLE audit_logs_2028_02 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');

CREATE TABLE audit_logs_2028_03 PARTITION OF audit_logs
    FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');

COMMIT;
