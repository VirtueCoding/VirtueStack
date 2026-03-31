-- VirtueStack Notification Preferences Migration (Down)
-- Removes notification tables

BEGIN;

-- Drop trigger and function
DROP TRIGGER IF EXISTS trigger_notification_preferences_updated_at ON notification_preferences;
DROP FUNCTION IF EXISTS update_notification_preferences_updated_at();

-- Drop tables
DROP TABLE IF EXISTS notification_events;
DROP TABLE IF EXISTS notification_preferences;

COMMIT;