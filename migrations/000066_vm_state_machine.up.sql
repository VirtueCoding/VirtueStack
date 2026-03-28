SET lock_timeout = '5s';

ALTER TABLE vms ADD CONSTRAINT vms_status_check
  CHECK (status IN ('provisioning', 'running', 'stopped', 'suspended', 'migrating', 'reinstalling', 'error', 'deleted'));
