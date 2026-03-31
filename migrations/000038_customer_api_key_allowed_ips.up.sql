-- Migration: 000038_customer_api_key_allowed_ips.up.sql
-- Add allowed_ips column for IP whitelist (IPv4/IPv6/CIDR support)

ALTER TABLE customer_api_keys ADD COLUMN allowed_ips TEXT[];

-- Add index for future IP-based queries if needed
CREATE INDEX idx_customer_api_keys_allowed_ips ON customer_api_keys USING GIN(allowed_ips);