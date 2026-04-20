SET lock_timeout = '5s';

DROP INDEX IF EXISTS idx_system_webhooks_active;
DROP TABLE IF EXISTS system_webhooks;
