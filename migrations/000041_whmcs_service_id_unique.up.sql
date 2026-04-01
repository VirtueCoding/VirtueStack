BEGIN;

SET lock_timeout = '5s';

-- Add UNIQUE constraint on whmcs_service_id to prevent duplicate VMs
-- at the database level. This enforces idempotency for WHMCS provisioning.
-- NULL values are allowed (for non-WHMCS VMs) and are not constrained.
ALTER TABLE vms ADD CONSTRAINT vms_whmcs_service_id_unique UNIQUE (whmcs_service_id);

COMMIT;