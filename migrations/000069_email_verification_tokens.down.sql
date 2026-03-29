SET lock_timeout = '5s';

DROP TABLE IF EXISTS email_verification_tokens;

ALTER TABLE customers
DROP CONSTRAINT IF EXISTS customers_status_check;

ALTER TABLE customers
ADD CONSTRAINT customers_status_check
CHECK (status IN ('active', 'suspended', 'deleted'));
