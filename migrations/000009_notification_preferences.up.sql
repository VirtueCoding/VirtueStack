-- VirtueStack Notification Preferences Migration
-- Creates tables for notification preferences and events

BEGIN;

SET lock_timeout = '5s';

-- ============================================================================
-- NOTIFICATION PREFERENCES TABLE
-- ============================================================================

-- Stores customer notification preferences
CREATE TABLE IF NOT EXISTS notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL UNIQUE REFERENCES customers(id) ON DELETE CASCADE,
    email_enabled BOOLEAN DEFAULT TRUE,
    telegram_enabled BOOLEAN DEFAULT FALSE,
    events TEXT[] NOT NULL DEFAULT ARRAY['vm.created', 'vm.deleted', 'vm.suspended', 'backup.failed'],
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for fast customer lookup
CREATE INDEX IF NOT EXISTS idx_notification_preferences_customer_id ON notification_preferences(customer_id);

-- ============================================================================
-- NOTIFICATION EVENTS TABLE
-- ============================================================================

-- Logs notification events for auditing and history
CREATE TABLE IF NOT EXISTS notification_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
    event_type VARCHAR(50) NOT NULL,
    resource_type VARCHAR(50),
    resource_id UUID,
    data JSONB,
    status VARCHAR(20) DEFAULT 'sent' CHECK (status IN ('pending', 'sent', 'failed')),
    error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for notification events
CREATE INDEX IF NOT EXISTS idx_notification_events_customer_id ON notification_events(customer_id);
CREATE INDEX IF NOT EXISTS idx_notification_events_event_type ON notification_events(event_type);
CREATE INDEX IF NOT EXISTS idx_notification_events_created_at ON notification_events(created_at);

-- ============================================================================
-- TRIGGER FOR UPDATED_AT
-- ============================================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_notification_preferences_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for notification_preferences
CREATE OR REPLACE TRIGGER trigger_notification_preferences_updated_at
    BEFORE UPDATE ON notification_preferences
    FOR EACH ROW
    EXECUTE FUNCTION update_notification_preferences_updated_at();

-- ============================================================================
-- GRANT PERMISSIONS
-- ============================================================================

-- Grant permissions to app_user
GRANT SELECT, INSERT, UPDATE ON notification_preferences TO app_user;
GRANT SELECT, INSERT ON notification_events TO app_user;

-- Grant permissions to app_customer for RLS
GRANT SELECT, INSERT, UPDATE ON notification_preferences TO app_customer;
GRANT SELECT ON notification_events TO app_customer;

COMMIT;