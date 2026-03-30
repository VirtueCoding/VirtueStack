SET lock_timeout = '5s';

DROP INDEX IF EXISTS idx_pre_action_webhooks_events_gin;
DROP INDEX IF EXISTS idx_system_webhooks_events_gin;

DROP POLICY IF EXISTS email_verification_tokens_customer_isolation ON email_verification_tokens;
ALTER TABLE email_verification_tokens DISABLE ROW LEVEL SECURITY;
