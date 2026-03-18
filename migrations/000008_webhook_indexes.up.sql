-- ===========================================================================
-- NOTE: This migration creates indexes on customer_webhooks and webhook_deliveries
-- tables. Migration 010 (000010_webhooks.up.sql) drops these tables CASCADE and
-- recreates them with a new schema. These indexes are therefore superseded by
-- migration 010's indexes on the new webhooks table.
-- ===========================================================================

BEGIN;

SET lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_customer_webhooks_customer_active
    ON customer_webhooks(customer_id, is_active);

CREATE INDEX IF NOT EXISTS idx_customer_webhooks_events
    ON customer_webhooks USING GIN(events);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_event_status
    ON webhook_deliveries(event_type, status, next_retry_at);

COMMIT;
