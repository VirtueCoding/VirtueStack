BEGIN;

DROP INDEX IF EXISTS idx_webhook_deliveries_event_status;
DROP INDEX IF EXISTS idx_customer_webhooks_events;
DROP INDEX IF EXISTS idx_customer_webhooks_customer_active;

COMMIT;
