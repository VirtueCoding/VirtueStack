-- Revert migration 000024
ALTER TABLE system_settings DROP CONSTRAINT IF EXISTS fk_system_settings_updated_by;
ALTER TABLE provisioning_keys DROP CONSTRAINT IF EXISTS fk_provisioning_keys_created_by;

CREATE INDEX IF NOT EXISTS idx_notification_preferences_customer_id ON notification_preferences(customer_id);

DROP INDEX IF EXISTS idx_nodes_location_id;
DROP INDEX IF EXISTS idx_vms_hostname;
DROP INDEX IF EXISTS idx_vms_plan_id;
