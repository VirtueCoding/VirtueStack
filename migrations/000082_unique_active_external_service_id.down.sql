SET lock_timeout = '5s';

DROP INDEX IF EXISTS idx_vms_external_service_id_active_unique;

CREATE INDEX IF NOT EXISTS idx_vms_external_service_id
    ON vms(external_service_id)
    WHERE external_service_id IS NOT NULL;
