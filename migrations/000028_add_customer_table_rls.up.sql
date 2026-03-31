BEGIN;

SET lock_timeout = '5s';

-- Enable RLS on customer_api_keys and restrict to owning customer
ALTER TABLE customer_api_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_api_keys_isolation ON customer_api_keys FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Enable RLS on ip_addresses and restrict to owning customer
ALTER TABLE ip_addresses ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_ip_addresses ON ip_addresses FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Enable RLS on backups via VM ownership
ALTER TABLE backups ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_backups ON backups FOR ALL TO app_customer
    USING (vm_id IN (
        SELECT id FROM vms
        WHERE customer_id = current_setting('app.current_customer_id')::UUID
    ));

-- Enable RLS on snapshots via VM ownership
ALTER TABLE snapshots ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_snapshots ON snapshots FOR ALL TO app_customer
    USING (vm_id IN (
        SELECT id FROM vms
        WHERE customer_id = current_setting('app.current_customer_id')::UUID
    ));

-- Enable RLS on backup_schedules and restrict to owning customer
ALTER TABLE backup_schedules ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_backup_schedules ON backup_schedules FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Enable RLS on sessions and restrict to owning customer
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_sessions ON sessions FOR ALL TO app_customer
    USING (user_id = current_setting('app.current_customer_id')::UUID AND user_type = 'customer');

-- Grant table access to app_customer for newly RLS-protected tables
GRANT SELECT, INSERT, UPDATE, DELETE ON customer_api_keys TO app_customer;
GRANT SELECT ON ip_addresses TO app_customer;
GRANT SELECT ON backups TO app_customer;
GRANT SELECT ON snapshots TO app_customer;
GRANT SELECT ON sessions TO app_customer;

COMMIT;
