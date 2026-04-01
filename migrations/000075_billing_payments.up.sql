SET lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS billing_payments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id        UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    gateway            VARCHAR(20) NOT NULL CHECK (gateway IN ('stripe', 'paypal', 'btcpay', 'nowpayments', 'admin')),
    gateway_payment_id VARCHAR(255),
    amount             BIGINT NOT NULL,
    currency           VARCHAR(3) NOT NULL DEFAULT 'USD',
    status             VARCHAR(20) NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'completed', 'failed', 'refunded')),
    reuse_key          VARCHAR(255),
    metadata           JSONB DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_payments_customer
    ON billing_payments(customer_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_billing_payments_gateway
    ON billing_payments(gateway, gateway_payment_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_payments_reuse
    ON billing_payments(reuse_key) WHERE reuse_key IS NOT NULL;

-- RLS: customers can only read their own payments
ALTER TABLE billing_payments ENABLE ROW LEVEL SECURITY;

CREATE POLICY billing_payments_customer_policy ON billing_payments
    FOR SELECT TO PUBLIC
    USING (customer_id = current_setting('app.current_customer_id')::UUID);
