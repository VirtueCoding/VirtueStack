BEGIN;

SET lock_timeout = '5s';

-- Issue 12: Make webhook_deliveries.idempotency_key UNIQUE to prevent duplicate deliveries.
-- Drop the non-unique index created in migration 000010 and replace with a unique constraint.
DROP INDEX IF EXISTS idx_webhook_deliveries_idempotency;
ALTER TABLE webhook_deliveries
    ADD CONSTRAINT uq_webhook_deliveries_idempotency_key UNIQUE (idempotency_key);

-- Issue 13: Add non-negative CHECK constraints on plan pricing columns added in migration 000016.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_plans_price_monthly_non_negative'
    ) THEN
        ALTER TABLE plans
            ADD CONSTRAINT chk_plans_price_monthly_non_negative CHECK (price_monthly >= 0);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_plans_price_hourly_non_negative'
    ) THEN
        ALTER TABLE plans
            ADD CONSTRAINT chk_plans_price_hourly_non_negative CHECK (price_hourly >= 0);
    END IF;
END $$;

COMMIT;
