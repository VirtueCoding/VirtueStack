BEGIN;

ALTER TABLE notification_preferences ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_notification_preferences ON notification_preferences FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

ALTER TABLE notification_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_notification_events ON notification_events FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

ALTER TABLE password_resets ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_password_resets ON password_resets FOR ALL TO app_customer
    USING (user_id = current_setting('app.current_customer_id')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON backup_schedules TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON failed_login_attempts TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON failover_requests TO app_user;
GRANT SELECT, INSERT, UPDATE ON backup_schedules TO app_customer;

COMMIT;
