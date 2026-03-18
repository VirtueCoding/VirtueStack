BEGIN;

SET lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_bandwidth_usage_period
    ON bandwidth_usage(year DESC, month DESC);

CREATE INDEX IF NOT EXISTS idx_bandwidth_usage_throttled_period
    ON bandwidth_usage(throttled, year DESC, month DESC)
    WHERE throttled = TRUE;

CREATE INDEX IF NOT EXISTS idx_bandwidth_snapshots_vm_created
    ON bandwidth_snapshots(vm_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_bandwidth_snapshots_period
    ON bandwidth_snapshots(year DESC, month DESC, day DESC, hour DESC);

COMMIT;
