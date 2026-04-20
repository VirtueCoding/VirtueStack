BEGIN;

SET lock_timeout = '5s';

-- Remove the UNIQUE constraint on whmcs_service_id
ALTER TABLE vms DROP CONSTRAINT IF EXISTS vms_whmcs_service_id_unique;

COMMIT;