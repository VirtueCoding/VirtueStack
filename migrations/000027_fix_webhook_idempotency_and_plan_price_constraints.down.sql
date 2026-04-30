BEGIN;

-- Restore non-unique index on idempotency_key
ALTER TABLE webhook_deliveries
    DROP CONSTRAINT IF EXISTS uq_webhook_deliveries_idempotency_key;
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_idempotency
    ON webhook_deliveries(idempotency_key);

-- Remove price check constraints
ALTER TABLE plans DROP CONSTRAINT IF EXISTS chk_plans_price_monthly_non_negative;
ALTER TABLE plans DROP CONSTRAINT IF EXISTS chk_plans_price_hourly_non_negative;

COMMIT;
