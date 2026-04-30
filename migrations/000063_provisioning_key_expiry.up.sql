-- Add expires_at column to provisioning_keys for mandatory key rotation.
-- NULL means the key does not expire (backward-compatible default).
-- The GetByHash repository query enforces the expiry check so that expired
-- keys are automatically rejected at authentication time.
ALTER TABLE provisioning_keys ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
