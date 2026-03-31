BEGIN;

-- Remove phone column from customers table
ALTER TABLE customers DROP COLUMN IF EXISTS phone;

COMMIT;