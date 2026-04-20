SET lock_timeout = '5s';

DROP INDEX IF EXISTS idx_vms_external_service_id;
ALTER TABLE vms DROP COLUMN IF EXISTS external_service_id;

DROP INDEX IF EXISTS idx_customers_external_client_id;
ALTER TABLE customers DROP COLUMN IF EXISTS external_client_id;
