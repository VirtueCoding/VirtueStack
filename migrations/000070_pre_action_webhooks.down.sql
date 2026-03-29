SET lock_timeout = '5s';

DROP INDEX IF EXISTS idx_pre_action_webhooks_active;
DROP TABLE IF EXISTS pre_action_webhooks;
