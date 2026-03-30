SET lock_timeout = '5s';

-- Compatibility no-op.
-- Migration 000045 already added generated column bandwidth_snapshots.snapshot_at
-- and index idx_bandwidth_snapshots_vm_snapshot_at. Re-running ALTER/UPDATE logic here
-- causes failures (generated column write attempts) on fresh installs.
