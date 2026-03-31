SET lock_timeout = '5s';

-- Add composite index for admin payment list with status and gateway filters
CREATE INDEX IF NOT EXISTS idx_billing_payments_status_gateway
    ON billing_payments(status, gateway, created_at DESC);

-- Add index for billing_transactions customer+type for usage reports
CREATE INDEX IF NOT EXISTS idx_billing_transactions_customer_type
    ON billing_transactions(customer_id, type, created_at DESC);
