-- Migration: 000038_customer_api_key_allowed_ips.down.sql
-- Remove allowed_ips column from customer_api_keys

ALTER TABLE customer_api_keys DROP COLUMN allowed_ips;
DROP INDEX IF EXISTS idx_customer_api_keys_allowed_ips;