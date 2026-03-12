BEGIN;

DROP INDEX IF EXISTS idx_bandwidth_snapshots_period;
DROP INDEX IF EXISTS idx_bandwidth_snapshots_vm_created;
DROP INDEX IF EXISTS idx_bandwidth_usage_throttled_period;
DROP INDEX IF EXISTS idx_bandwidth_usage_period;
DROP INDEX IF EXISTS idx_bandwidth_usage_vm_period;

COMMIT;
