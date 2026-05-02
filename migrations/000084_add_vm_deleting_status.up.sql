SET lock_timeout = '5s';

ALTER TABLE vms DROP CONSTRAINT IF EXISTS vms_status_check;

ALTER TABLE vms ADD CONSTRAINT vms_status_check
  CHECK (status IN ('provisioning', 'running', 'stopped', 'suspended', 'deleting', 'migrating', 'reinstalling', 'error', 'deleted'));
