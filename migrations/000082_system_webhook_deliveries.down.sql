BEGIN;

SET lock_timeout = '5s';

DROP TRIGGER IF EXISTS system_webhook_deliveries_updated_at ON system_webhook_deliveries;
DROP INDEX IF EXISTS idx_system_webhook_deliveries_event;
DROP INDEX IF EXISTS idx_system_webhook_deliveries_pending;
DROP INDEX IF EXISTS idx_system_webhook_deliveries_webhook;
DROP TABLE IF EXISTS system_webhook_deliveries;

COMMIT;
