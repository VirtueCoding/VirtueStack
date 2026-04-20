SET lock_timeout = '5s';

-- Add balance column to customers table for native billing
ALTER TABLE customers ADD COLUMN IF NOT EXISTS balance BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN customers.balance IS 'Credit balance in cents (minor currency units). Only used for native billing customers. Mutated under SELECT FOR UPDATE.';

-- Immutable credit ledger: every balance change is recorded as an append-only row
CREATE TABLE IF NOT EXISTS billing_transactions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id      UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    type             VARCHAR(30) NOT NULL CHECK (type IN ('credit', 'debit', 'adjustment', 'refund')),
    amount           BIGINT NOT NULL,
    balance_after    BIGINT NOT NULL,
    description      TEXT NOT NULL,
    reference_type   VARCHAR(30),
    reference_id     UUID,
    idempotency_key  VARCHAR(255),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_tx_customer
    ON billing_transactions(customer_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_tx_idempotency
    ON billing_transactions(customer_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- RLS: customers can only read their own transactions
ALTER TABLE billing_transactions ENABLE ROW LEVEL SECURITY;

CREATE POLICY billing_tx_customer_policy ON billing_transactions
    FOR SELECT TO PUBLIC
    USING (customer_id = current_setting('app.current_customer_id')::UUID);
