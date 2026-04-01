BEGIN;

SET lock_timeout = '5s';

-- Revoke grants added in up migration
REVOKE SELECT ON sessions FROM app_customer;
REVOKE SELECT ON snapshots FROM app_customer;
REVOKE SELECT ON backups FROM app_customer;
REVOKE SELECT ON ip_addresses FROM app_customer;
REVOKE SELECT, INSERT, UPDATE, DELETE ON customer_api_keys FROM app_customer;

-- Drop RLS policies and disable RLS
DROP POLICY IF EXISTS customer_sessions ON sessions;
ALTER TABLE sessions DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_backup_schedules ON backup_schedules;
ALTER TABLE backup_schedules DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_snapshots ON snapshots;
ALTER TABLE snapshots DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_backups ON backups;
ALTER TABLE backups DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_ip_addresses ON ip_addresses;
ALTER TABLE ip_addresses DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_api_keys_isolation ON customer_api_keys;
ALTER TABLE customer_api_keys DISABLE ROW LEVEL SECURITY;

COMMIT;
