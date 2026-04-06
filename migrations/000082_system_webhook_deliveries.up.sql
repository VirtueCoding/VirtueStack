BEGIN;

SET lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS system_webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    system_webhook_id UUID NOT NULL REFERENCES system_webhooks(id) ON DELETE CASCADE,
    event VARCHAR(100) NOT NULL,
    idempotency_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    next_retry_at TIMESTAMPTZ,
    response_status INTEGER,
    response_body TEXT,
    error_message TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT system_webhook_deliveries_status_valid
        CHECK (status IN ('pending', 'delivered', 'failed', 'retrying')),
    CONSTRAINT system_webhook_deliveries_attempt_count_valid
        CHECK (attempt_count >= 0 AND attempt_count <= max_attempts),
    CONSTRAINT uq_system_webhook_deliveries_idempotency_key
        UNIQUE (idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_system_webhook_deliveries_webhook
    ON system_webhook_deliveries(system_webhook_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_system_webhook_deliveries_pending
    ON system_webhook_deliveries(status, next_retry_at)
    WHERE status IN ('pending', 'retrying');

CREATE INDEX IF NOT EXISTS idx_system_webhook_deliveries_event
    ON system_webhook_deliveries(event);

DROP TRIGGER IF EXISTS system_webhook_deliveries_updated_at ON system_webhook_deliveries;
CREATE TRIGGER system_webhook_deliveries_updated_at
    BEFORE UPDATE ON system_webhook_deliveries
    FOR EACH ROW
    EXECUTE FUNCTION update_webhook_updated_at();

COMMENT ON TABLE system_webhook_deliveries IS 'Durable delivery attempts for system webhooks with retry-safe idempotency';
COMMENT ON COLUMN system_webhook_deliveries.idempotency_key IS 'Stable idempotency key used for deduping retries and legacy task replays';

COMMIT;
