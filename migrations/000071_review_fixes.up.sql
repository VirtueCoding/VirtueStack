SET lock_timeout = '5s';

-- 🔴 CRITICAL: Enable Row Level Security on email_verification_tokens table
-- This table has customer_id FK and stores security-sensitive tokens
ALTER TABLE email_verification_tokens ENABLE ROW LEVEL SECURITY;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE tablename = 'email_verification_tokens' AND policyname = 'email_verification_tokens_customer_isolation'
    ) THEN
        CREATE POLICY email_verification_tokens_customer_isolation
            ON email_verification_tokens FOR ALL
            USING (customer_id = current_setting('app.current_customer_id')::uuid);
    END IF;
END $$;

-- 🟡 WARNING: Add GIN indexes on TEXT[] columns for array containment queries
CREATE INDEX IF NOT EXISTS idx_system_webhooks_events_gin
    ON system_webhooks USING GIN(events);

CREATE INDEX IF NOT EXISTS idx_pre_action_webhooks_events_gin
    ON pre_action_webhooks USING GIN(events);
