SET lock_timeout = '5s';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM vms
        WHERE external_service_id IS NOT NULL
          AND deleted_at IS NULL
        GROUP BY external_service_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'duplicate active VM external_service_id values exist; resolve before applying 000082 with: SELECT external_service_id, COUNT(*) FROM vms WHERE external_service_id IS NOT NULL AND deleted_at IS NULL GROUP BY external_service_id HAVING COUNT(*) > 1';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_vms_external_service_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_vms_external_service_id_active_unique
    ON vms(external_service_id)
    WHERE external_service_id IS NOT NULL
      AND deleted_at IS NULL;
