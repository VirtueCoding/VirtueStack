SET lock_timeout = '5s';

-- Neutral column for external billing system's client ID (replaces whmcs_client_id).
ALTER TABLE customers ADD COLUMN IF NOT EXISTS external_client_id INT;
UPDATE customers SET external_client_id = whmcs_client_id WHERE whmcs_client_id IS NOT NULL AND external_client_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_customers_external_client_id ON customers(external_client_id) WHERE external_client_id IS NOT NULL;

-- Neutral column for external billing system's service/order ID (replaces whmcs_service_id).
ALTER TABLE vms ADD COLUMN IF NOT EXISTS external_service_id INT;
UPDATE vms SET external_service_id = whmcs_service_id WHERE whmcs_service_id IS NOT NULL AND external_service_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_vms_external_service_id ON vms(external_service_id) WHERE external_service_id IS NOT NULL;
