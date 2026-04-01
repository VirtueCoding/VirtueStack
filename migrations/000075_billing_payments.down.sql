SET lock_timeout = '5s';

DROP POLICY IF EXISTS billing_payments_customer_policy ON billing_payments;
DROP TABLE IF EXISTS billing_payments;
