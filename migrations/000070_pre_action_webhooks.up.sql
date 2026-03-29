SET lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS pre_action_webhooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    events TEXT[] NOT NULL DEFAULT '{}',
    timeout_ms INTEGER NOT NULL DEFAULT 5000,
    fail_open BOOLEAN NOT NULL DEFAULT true,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pre_action_webhooks_active
    ON pre_action_webhooks (is_active)
    WHERE is_active = true;
