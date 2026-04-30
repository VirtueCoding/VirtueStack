SET lock_timeout = '5s';

DROP TABLE IF EXISTS exchange_rates;

-- Restore NOT NULL on plan pricing columns (backfill NULLs to 0 first)
UPDATE plans SET price_monthly = 0 WHERE price_monthly IS NULL;
UPDATE plans SET price_hourly = 0 WHERE price_hourly IS NULL;
ALTER TABLE plans ALTER COLUMN price_monthly SET NOT NULL;
ALTER TABLE plans ALTER COLUMN price_hourly SET NOT NULL;

ALTER TABLE plans DROP COLUMN IF EXISTS currency;
ALTER TABLE plans DROP COLUMN IF EXISTS price_hourly_stopped;

DROP TABLE IF EXISTS billing_vm_checkpoints;
