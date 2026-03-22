CREATE TABLE console_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash BYTEA NOT NULL,
    user_id UUID NOT NULL,
    user_type VARCHAR(20) NOT NULL CHECK (user_type IN ('customer', 'admin')),
    vm_id UUID NOT NULL,
    console_type VARCHAR(10) NOT NULL CHECK (console_type IN ('vnc', 'serial')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- Hot path: lookup by token hash for unconsumed tokens
CREATE INDEX idx_console_tokens_hash ON console_tokens (token_hash) WHERE expires_at > NOW();

-- Cleanup: find expired tokens
CREATE INDEX idx_console_tokens_expires ON console_tokens (expires_at);

COMMENT ON TABLE console_tokens IS 'Short-lived single-use opaque tokens for WebSocket console authentication';