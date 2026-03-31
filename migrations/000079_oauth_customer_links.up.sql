SET lock_timeout = '5s';

-- Make password_hash nullable for OAuth-only users (who authenticate via
-- Google/GitHub and may never set a local password).
ALTER TABLE customers ALTER COLUMN password_hash DROP NOT NULL;

-- Track how the account was originally created. Existing rows default to
-- 'local' (email + password). OAuth-created accounts use 'google'/'github'.
ALTER TABLE customers ADD COLUMN IF NOT EXISTS auth_provider VARCHAR(20) NOT NULL DEFAULT 'local'
    CHECK (auth_provider IN ('local', 'google', 'github'));

-- OAuth provider links. One customer can link multiple providers.
-- A single provider account can only be linked to one VirtueStack customer.
CREATE TABLE customer_oauth_links (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id             UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    provider                VARCHAR(20) NOT NULL CHECK (provider IN ('google', 'github')),
    provider_user_id        VARCHAR(255) NOT NULL,
    email                   VARCHAR(255),
    display_name            VARCHAR(255),
    avatar_url              VARCHAR(500),
    access_token_encrypted  BYTEA,
    refresh_token_encrypted BYTEA,
    token_expires_at        TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_user_id)
);

CREATE INDEX idx_oauth_links_customer ON customer_oauth_links(customer_id);
