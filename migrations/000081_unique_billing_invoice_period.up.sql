SET lock_timeout = '5s';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM billing_invoices
        GROUP BY customer_id, period_start, period_end
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'duplicate billing invoice periods exist; resolve before applying 000081 with: SELECT customer_id, period_start, period_end, COUNT(*) FROM billing_invoices GROUP BY customer_id, period_start, period_end HAVING COUNT(*) > 1';
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_invoices_customer_period_unique
    ON billing_invoices(customer_id, period_start, period_end);
