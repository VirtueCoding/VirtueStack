SET lock_timeout = '5s';

DROP POLICY IF EXISTS billing_invoices_customer_policy ON billing_invoices;
DROP TABLE IF EXISTS billing_invoices;
DROP TABLE IF EXISTS billing_invoice_counters;
