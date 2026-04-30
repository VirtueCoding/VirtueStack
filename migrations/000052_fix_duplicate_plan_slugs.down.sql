-- Migration 000052 (down): Remove unique slug index
-- Note: slug values that were de-duplicated cannot be automatically reverted.

BEGIN;

SET lock_timeout = '5s';

DROP INDEX IF EXISTS uq_plans_slug;

COMMIT;
