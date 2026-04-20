-- VirtueStack Password Reset Migration (Down)
-- Removes password_resets table

BEGIN;

-- Drop table
DROP TABLE IF EXISTS password_resets;

COMMIT;