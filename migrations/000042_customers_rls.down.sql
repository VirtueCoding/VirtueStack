-- Remove RLS policy for customers table

BEGIN;

SET lock_timeout = '5s';

-- Drop the policy
DROP POLICY IF EXISTS customer_self_isolation ON customers;

-- Disable RLS on customers table
ALTER TABLE customers DISABLE ROW LEVEL SECURITY;

-- Revoke access from app_customer
REVOKE SELECT ON customers FROM app_customer;

COMMIT;