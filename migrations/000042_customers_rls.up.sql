-- Add RLS policy for customers table
-- Addresses RLS-001: customers table should have RLS for customer isolation

BEGIN;

SET lock_timeout = '5s';

-- Enable RLS on customers table
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;

-- Create policy: customers can only see their own row
CREATE POLICY customer_self_isolation ON customers FOR ALL TO app_customer
    USING (id = current_setting('app.current_customer_id')::UUID);

-- Grant table access to app_customer
GRANT SELECT ON customers TO app_customer;

COMMIT;