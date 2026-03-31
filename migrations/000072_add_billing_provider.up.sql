SET lock_timeout = '5s';

ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS billing_provider VARCHAR(20)
    CHECK (billing_provider IN ('whmcs', 'native', 'blesta', 'unmanaged'));

UPDATE customers
SET billing_provider = CASE
    WHEN whmcs_client_id IS NOT NULL THEN 'whmcs'
    ELSE 'unmanaged'
END
WHERE billing_provider IS NULL;

ALTER TABLE customers
    ALTER COLUMN billing_provider SET NOT NULL;

COMMENT ON COLUMN customers.billing_provider IS
    'Which billing system manages this customer: whmcs, native, blesta, or unmanaged';
