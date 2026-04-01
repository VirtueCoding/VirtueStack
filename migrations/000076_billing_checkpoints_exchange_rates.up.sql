SET lock_timeout = '5s';

-- Hourly deduction checkpoint: prevents double billing in HA deployments.
-- The composite PK (vm_id, charge_hour) makes duplicate charges physically impossible.
CREATE TABLE IF NOT EXISTS billing_vm_checkpoints (
    vm_id          UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    charge_hour    TIMESTAMPTZ NOT NULL,
    amount         BIGINT NOT NULL,
    transaction_id UUID REFERENCES billing_transactions(id) ON DELETE RESTRICT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (vm_id, charge_hour)
);

-- Plan pricing amendments for native billing
ALTER TABLE plans ADD COLUMN IF NOT EXISTS price_hourly_stopped BIGINT;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS currency VARCHAR(3) NOT NULL DEFAULT 'USD';

COMMENT ON COLUMN plans.price_hourly_stopped IS 'Hourly price in cents when VM is stopped. NULL = same as price_hourly. 0 = free when stopped.';
COMMENT ON COLUMN plans.currency IS 'ISO 4217 currency code for plan pricing.';

-- Make price columns nullable: NULL = plan managed externally (WHMCS/Blesta)
ALTER TABLE plans ALTER COLUMN price_monthly DROP NOT NULL;
ALTER TABLE plans ALTER COLUMN price_hourly DROP NOT NULL;

-- Exchange rates for multi-currency billing
CREATE TABLE IF NOT EXISTS exchange_rates (
    currency    VARCHAR(3) PRIMARY KEY,
    rate_to_usd NUMERIC(18, 8) NOT NULL,
    source      VARCHAR(20) NOT NULL CHECK (source IN ('api', 'admin')),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed USD as the base currency
INSERT INTO exchange_rates (currency, rate_to_usd, source)
VALUES ('USD', 1.00000000, 'admin')
ON CONFLICT (currency) DO NOTHING;
