-- VirtueStack Webhook Delivery System Migration
-- Creates webhooks and webhook_deliveries tables
-- Reference: Webhook Delivery System specification

BEGIN;

-- Drop legacy webhook tables from initial schema (migration 1)
-- Migration 1 created customer_webhooks + webhook_deliveries with a different schema.
-- This migration replaces them with the redesigned webhook system.
DROP TABLE IF EXISTS webhook_deliveries CASCADE;
DROP TABLE IF EXISTS customer_webhooks CASCADE;

CREATE TABLE webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    url VARCHAR(2048) NOT NULL,
    secret_hash VARCHAR(128) NOT NULL,  -- Hashed secret for signature verification
    events TEXT[] NOT NULL DEFAULT '{}',  -- Array of event types to subscribe to
    active BOOLEAN DEFAULT TRUE NOT NULL,
    fail_count INTEGER DEFAULT 0 NOT NULL,  -- Consecutive failure count for auto-disable
    last_success_at TIMESTAMPTZ,
    last_failure_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    
    CONSTRAINT webhooks_url_https CHECK (url ~ '^https://'),
    CONSTRAINT webhooks_events_not_empty CHECK (array_length(events, 1) > 0),
    CONSTRAINT webhooks_fail_count_non_negative CHECK (fail_count >= 0)
);

-- Index for looking up webhooks by customer
CREATE INDEX idx_webhooks_customer ON webhooks(customer_id);

-- Index for finding active webhooks for event dispatch
CREATE INDEX idx_webhooks_active ON webhooks(active) WHERE active = TRUE;

-- Index for finding webhooks subscribed to specific events
CREATE INDEX idx_webhooks_events ON webhooks USING GIN(events);

-- ============================================================================
-- WEBHOOK DELIVERIES TABLE
-- Tracks individual webhook delivery attempts
-- ============================================================================

CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event VARCHAR(100) NOT NULL,
    idempotency_key UUID NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER DEFAULT 0 NOT NULL,
    max_attempts INTEGER DEFAULT 5 NOT NULL,
    next_retry_at TIMESTAMPTZ,
    response_status INTEGER,
    response_body TEXT,
    error_message TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    
    CONSTRAINT webhook_deliveries_status_valid CHECK (status IN ('pending', 'delivered', 'failed', 'retrying')),
    CONSTRAINT webhook_deliveries_attempt_count_valid CHECK (attempt_count >= 0 AND attempt_count <= max_attempts)
);

-- Index for looking up deliveries by webhook
CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id, created_at DESC);

-- Index for finding pending deliveries to process
CREATE INDEX idx_webhook_deliveries_pending ON webhook_deliveries(status, next_retry_at) 
    WHERE status IN ('pending', 'retrying');

-- Index for idempotency key lookups
CREATE INDEX idx_webhook_deliveries_idempotency ON webhook_deliveries(idempotency_key);

-- Index for event type filtering
CREATE INDEX idx_webhook_deliveries_event ON webhook_deliveries(event);

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION update_webhook_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER webhooks_updated_at
    BEFORE UPDATE ON webhooks
    FOR EACH ROW
    EXECUTE FUNCTION update_webhook_updated_at();

CREATE TRIGGER webhook_deliveries_updated_at
    BEFORE UPDATE ON webhook_deliveries
    FOR EACH ROW
    EXECUTE FUNCTION update_webhook_updated_at();

-- ============================================================================
-- VIEWS
-- ============================================================================

-- Active webhooks with customer info
CREATE VIEW v_active_webhooks AS
SELECT 
    w.id,
    w.customer_id,
    c.email as customer_email,
    w.url,
    w.events,
    w.fail_count,
    w.last_success_at,
    w.last_failure_at,
    w.created_at
FROM webhooks w
JOIN customers c ON w.customer_id = c.id
WHERE w.active = TRUE;

-- Recent delivery statistics per webhook
CREATE VIEW v_webhook_delivery_stats AS
SELECT 
    w.id as webhook_id,
    w.url,
    COUNT(wd.id) as total_deliveries,
    COUNT(wd.id) FILTER (WHERE wd.status = 'delivered') as successful_deliveries,
    COUNT(wd.id) FILTER (WHERE wd.status = 'failed') as failed_deliveries,
    COUNT(wd.id) FILTER (WHERE wd.status IN ('pending', 'retrying')) as pending_deliveries,
    MAX(wd.created_at) as last_delivery_attempt,
    AVG(wd.attempt_count) FILTER (WHERE wd.status = 'delivered') as avg_attempts_to_success
FROM webhooks w
LEFT JOIN webhook_deliveries wd ON w.id = wd.webhook_id
GROUP BY w.id, w.url;

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- Enable RLS on webhook tables
ALTER TABLE webhooks ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries ENABLE ROW LEVEL SECURITY;

-- App user can see all (for internal operations)
CREATE POLICY webhooks_app_all ON webhooks
    FOR ALL TO app_user USING (true);

CREATE POLICY webhook_deliveries_app_all ON webhook_deliveries
    FOR ALL TO app_user USING (true);

-- Customer isolation: customers can only see their own webhooks
CREATE POLICY webhooks_customer_isolation ON webhooks
    FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

CREATE POLICY webhook_deliveries_customer_isolation ON webhook_deliveries
    FOR SELECT TO app_customer
    USING (webhook_id IN (
        SELECT id FROM webhooks WHERE customer_id = current_setting('app.current_customer_id')::UUID
    ));

-- ============================================================================
-- FUNCTIONS
-- ============================================================================

-- Function to check if customer has reached webhook limit
CREATE OR REPLACE FUNCTION check_webhook_limit()
RETURNS TRIGGER AS $$
DECLARE
    webhook_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO webhook_count
    FROM webhooks
    WHERE customer_id = NEW.customer_id;
    
    IF webhook_count >= 5 THEN
        RAISE EXCEPTION 'Customer has reached maximum webhook limit (5)';
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to enforce webhook limit
CREATE TRIGGER enforce_webhook_limit
    BEFORE INSERT ON webhooks
    FOR EACH ROW
    EXECUTE FUNCTION check_webhook_limit();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE webhooks IS 'Webhook endpoint configurations for event notifications';
COMMENT ON TABLE webhook_deliveries IS 'Individual webhook delivery attempts with retry tracking';

COMMENT ON COLUMN webhooks.secret_hash IS 'Hashed secret used for HMAC-SHA256 signature generation';
COMMENT ON COLUMN webhooks.events IS 'Array of event types this webhook subscribes to (e.g., {vm.created,vm.deleted})';
COMMENT ON COLUMN webhooks.fail_count IS 'Consecutive delivery failures; auto-disables after 50';

COMMENT ON COLUMN webhook_deliveries.idempotency_key IS 'Unique key for idempotent delivery';
COMMENT ON COLUMN webhook_deliveries.status IS 'Current delivery status: pending, delivered, failed, retrying';
COMMENT ON COLUMN webhook_deliveries.attempt_count IS 'Number of delivery attempts made';
COMMENT ON COLUMN webhook_deliveries.next_retry_at IS 'Scheduled time for next retry attempt';

COMMIT;