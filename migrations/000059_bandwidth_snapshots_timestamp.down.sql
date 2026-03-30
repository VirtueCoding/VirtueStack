SET lock_timeout = '5s';

-- Compatibility no-op.
-- Migration 000045 owns snapshot_at/index lifecycle; dropping them here would break rollback order.
