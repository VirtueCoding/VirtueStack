SET lock_timeout = '5s';

ALTER TABLE customers DROP COLUMN IF EXISTS billing_provider;
