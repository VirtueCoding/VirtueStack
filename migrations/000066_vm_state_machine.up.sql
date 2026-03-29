SET lock_timeout = '5s';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'vms_status_check'
      AND conrelid = 'vms'::regclass
  ) THEN
    ALTER TABLE vms ADD CONSTRAINT vms_status_check
      CHECK (status IN ('provisioning', 'running', 'stopped', 'suspended', 'migrating', 'reinstalling', 'error', 'deleted'));
  END IF;
END
$$;
