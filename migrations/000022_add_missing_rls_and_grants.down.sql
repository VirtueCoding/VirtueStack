BEGIN;

DROP POLICY IF EXISTS customer_notification_preferences ON notification_preferences;
ALTER TABLE notification_preferences DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_notification_events ON notification_events;
ALTER TABLE notification_events DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS customer_password_resets ON password_resets;
ALTER TABLE password_resets DISABLE ROW LEVEL SECURITY;

REVOKE SELECT, INSERT, UPDATE ON backup_schedules FROM app_customer;
REVOKE SELECT, INSERT, UPDATE, DELETE ON failover_requests FROM app_user;
REVOKE SELECT, INSERT, UPDATE, DELETE ON failed_login_attempts FROM app_user;
REVOKE SELECT, INSERT, UPDATE, DELETE ON backup_schedules FROM app_user;

COMMIT;
