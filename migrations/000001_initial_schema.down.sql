-- VirtueStack Initial Schema Migration (Rollback)
-- Drops all tables, indexes, roles, and extensions in reverse dependency order

BEGIN;

-- ============================================================================
-- REVOKE PERMISSIONS
-- ============================================================================

REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM app_user;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM app_user;
REVOKE ALL PRIVILEGES ON SCHEMA public FROM app_user;

REVOKE ALL PRIVILEGES ON vms FROM app_customer;
REVOKE ALL PRIVILEGES ON SCHEMA public FROM app_customer;

-- ============================================================================
-- DROP TABLES (reverse dependency order)
-- ============================================================================

-- Level 3 dependencies
DROP TABLE IF EXISTS snapshots;
DROP TABLE IF EXISTS backups;
DROP TABLE IF EXISTS vm_ipv6_subnets;
DROP TABLE IF EXISTS ip_addresses;

-- Level 2 dependencies
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS node_heartbeats;
DROP TABLE IF EXISTS vms;
DROP TABLE IF EXISTS ipv6_prefixes;

-- Level 1 dependencies
DROP TABLE IF EXISTS customer_webhooks;
DROP TABLE IF EXISTS customer_api_keys;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS ip_sets;

-- Base tables (no dependencies)
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS system_settings;
DROP TABLE IF EXISTS provisioning_keys;
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS plans;
DROP TABLE IF EXISTS admins;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS locations;

-- Partitioned table (CASCADE drops all partitions automatically)
DROP TABLE IF EXISTS audit_logs CASCADE;

-- ============================================================================
-- DROP ROLES
-- ============================================================================

DROP ROLE IF EXISTS app_customer;
DROP ROLE IF EXISTS app_user;

-- ============================================================================
-- DROP EXTENSIONS
-- ============================================================================

DROP EXTENSION IF EXISTS pgcrypto;

COMMIT;