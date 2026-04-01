SET lock_timeout = '5s';

ALTER TABLE vms DROP CONSTRAINT IF EXISTS vms_status_check;
