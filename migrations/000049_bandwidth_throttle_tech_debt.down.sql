-- Migration 000049 (down): Remove bandwidth_throttle tech-debt comment

BEGIN;

SET lock_timeout = '5s';

COMMENT ON TABLE bandwidth_throttle IS NULL;

COMMIT;
