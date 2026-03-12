BEGIN;

CREATE INDEX IF NOT EXISTS idx_customer_webhooks_customer_active
    ON customer_webhooks(customer_id, is_active);

CREATE INDEX IF NOT EXISTS idx_customer_webhooks_events
    ON customer_webhooks USING GIN(events);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_event_status
    ON webhook_deliveries(event_type, status, next_retry_at);

COMMIT;
