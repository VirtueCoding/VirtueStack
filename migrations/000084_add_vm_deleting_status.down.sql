SET lock_timeout = '5s';

UPDATE vms SET status = 'error' WHERE status = 'deleting';

ALTER TABLE vms DROP CONSTRAINT IF EXISTS vms_status_check;

ALTER TABLE vms ADD CONSTRAINT vms_status_check
  CHECK (status IN ('provisioning', 'running', 'stopped', 'suspended', 'migrating', 'reinstalling', 'error', 'deleted'));
