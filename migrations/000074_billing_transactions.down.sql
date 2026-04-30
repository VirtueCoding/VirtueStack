SET lock_timeout = '5s';

DROP POLICY IF EXISTS billing_tx_customer_policy ON billing_transactions;
DROP TABLE IF EXISTS billing_transactions;
ALTER TABLE customers DROP COLUMN IF EXISTS balance;
