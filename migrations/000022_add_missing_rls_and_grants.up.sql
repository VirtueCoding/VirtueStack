-- Migration 000022: Add Missing RLS and Grants
-- Security Note: Migration 000011 now enables RLS for password_resets at table creation.
-- This migration remains idempotent for existing deployments where 000011 was applied before this fix.

BEGIN;

SET lock_timeout = '5s';

-- Enable RLS for notification_preferences
ALTER TABLE notification_preferences ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_notification_preferences ON notification_preferences FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Enable RLS for notification_events
ALTER TABLE notification_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_notification_events ON notification_events FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Enable RLS for password_resets (idempotent: skip if already enabled by migration 000011)
-- This handles existing deployments where 000011 was applied before the RLS fix was added.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = 'password_resets'
        AND n.nspname = 'public'
        AND c.relrowsecurity = true
    ) THEN
        ALTER TABLE password_resets ENABLE ROW LEVEL SECURITY;
        CREATE POLICY customer_password_resets ON password_resets FOR ALL TO app_customer
            USING (user_id = current_setting('app.current_customer_id')::UUID);
    END IF;
END $$;

-- Additional grants
GRANT SELECT, INSERT, UPDATE, DELETE ON backup_schedules TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON failed_login_attempts TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON failover_requests TO app_user;
GRANT SELECT, INSERT, UPDATE ON backup_schedules TO app_customer;

COMMIT;
