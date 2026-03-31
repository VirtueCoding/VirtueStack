SET lock_timeout = '5s';

ALTER TABLE customers
DROP CONSTRAINT IF EXISTS customers_status_check;

ALTER TABLE customers
ADD CONSTRAINT customers_status_check
CHECK (status IN ('active', 'pending_verification', 'suspended', 'deleted'));

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL,
    token_hash VARCHAR(128) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT email_verification_tokens_customer_id_fk
        FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_token_hash
    ON email_verification_tokens(token_hash);

CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_customer_id
    ON email_verification_tokens(customer_id);
