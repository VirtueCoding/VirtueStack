-- VirtueStack Webhook Delivery System - Down Migration
-- Rolls back webhooks and webhook_deliveries tables
-- Restores the original customer_webhooks and webhook_deliveries tables from migration 001

BEGIN;

-- ============================================================================
-- DROP NEW WEBHOOK SYSTEM (migration 010)
-- ============================================================================

DROP POLICY IF EXISTS webhooks_customer_isolation ON webhooks;
DROP POLICY IF EXISTS webhook_deliveries_customer_isolation ON webhook_deliveries;
DROP POLICY IF EXISTS webhooks_app_all ON webhooks;
DROP POLICY IF EXISTS webhook_deliveries_app_all ON webhook_deliveries;

DROP TRIGGER IF EXISTS enforce_webhook_limit ON webhooks;
DROP TRIGGER IF EXISTS webhooks_updated_at ON webhooks;
DROP TRIGGER IF EXISTS webhook_deliveries_updated_at ON webhook_deliveries;

DROP VIEW IF EXISTS v_webhook_delivery_stats;
DROP VIEW IF EXISTS v_active_webhooks;

DROP FUNCTION IF EXISTS check_webhook_limit();
DROP FUNCTION IF EXISTS update_webhook_updated_at();

DROP TABLE IF EXISTS webhook_deliveries CASCADE;
DROP TABLE IF EXISTS webhooks CASCADE;

-- ============================================================================
-- RESTORE ORIGINAL WEBHOOK TABLES (from migration 001)
-- ============================================================================

CREATE TABLE customer_webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    url VARCHAR(2048) NOT NULL,
    secret TEXT NOT NULL,
    events TEXT[] NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    last_triggered_at TIMESTAMPTZ,
    consecutive_failures INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES customer_webhooks(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    response_code INTEGER,
    response_body TEXT,
    attempt INTEGER DEFAULT 1,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'retrying')),
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_status_retry ON webhook_deliveries(status, next_retry_at);

COMMIT;
