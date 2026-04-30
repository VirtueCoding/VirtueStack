BEGIN;

SET lock_timeout = '5s';

CREATE TABLE sso_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash BYTEA NOT NULL UNIQUE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    redirect_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_sso_tokens_expires_at ON sso_tokens (expires_at);

COMMENT ON TABLE sso_tokens IS 'Short-lived single-use opaque browser bootstrap tokens for WHMCS SSO';

COMMIT;
