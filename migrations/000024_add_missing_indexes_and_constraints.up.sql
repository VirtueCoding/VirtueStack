BEGIN;

SET lock_timeout = '5s';

-- Add missing indexes on frequently queried columns
CREATE INDEX IF NOT EXISTS idx_vms_plan_id ON vms(plan_id);
CREATE INDEX IF NOT EXISTS idx_vms_hostname ON vms(hostname);
CREATE INDEX IF NOT EXISTS idx_nodes_location_id ON nodes(location_id);

-- Drop redundant index (customer_id is already the primary lookup column)
DROP INDEX IF EXISTS idx_notification_preferences_customer_id;

-- Add missing FK constraints
ALTER TABLE provisioning_keys ADD CONSTRAINT fk_provisioning_keys_created_by
    FOREIGN KEY (created_by) REFERENCES admins(id);
ALTER TABLE system_settings ADD CONSTRAINT fk_system_settings_updated_by
    FOREIGN KEY (updated_by) REFERENCES admins(id);

COMMIT;
