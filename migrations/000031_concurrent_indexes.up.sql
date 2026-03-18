-- Rebuild indexes from migrations 000003-000007 using CONCURRENTLY to avoid write-blocking.
--
-- Background: migrations 000003-000008 used plain CREATE INDEX inside BEGIN/COMMIT
-- transactions. Non-concurrent CREATE INDEX takes a ShareLock that blocks INSERT,
-- UPDATE, and DELETE on the target table for the entire duration (QG-13, QG-16).
-- For large production tables this causes write downtime.
--
-- CONCURRENTLY builds the index without holding a ShareLock, allowing writes to
-- proceed. The trade-off is that it cannot run inside a transaction block, so this
-- file has no BEGIN/COMMIT.
--
-- Migration 000008 indexes (idx_customer_webhooks_* and idx_webhook_deliveries_*)
-- are intentionally omitted: migration 000010 dropped the original customer_webhooks
-- and webhook_deliveries tables and replaced them with the redesigned webhooks and
-- webhook_deliveries tables. Those legacy indexes no longer exist, and the replacement
-- tables received their own indexes in migration 000010.
--
-- Each statement is idempotent (DROP IF EXISTS / CREATE IF NOT EXISTS).

-- ============================================================================
-- 000003: bandwidth_indexes
-- ============================================================================

DROP INDEX CONCURRENTLY IF EXISTS idx_bandwidth_usage_period;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bandwidth_usage_period
    ON bandwidth_usage(year DESC, month DESC);

DROP INDEX CONCURRENTLY IF EXISTS idx_bandwidth_usage_throttled_period;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bandwidth_usage_throttled_period
    ON bandwidth_usage(throttled, year DESC, month DESC)
    WHERE throttled = TRUE;

DROP INDEX CONCURRENTLY IF EXISTS idx_bandwidth_snapshots_vm_created;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bandwidth_snapshots_vm_created
    ON bandwidth_snapshots(vm_id, created_at DESC);

DROP INDEX CONCURRENTLY IF EXISTS idx_bandwidth_snapshots_period;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bandwidth_snapshots_period
    ON bandwidth_snapshots(year DESC, month DESC, day DESC, hour DESC);

-- ============================================================================
-- 000004: ip_indexes
-- ============================================================================

DROP INDEX CONCURRENTLY IF EXISTS idx_ip_sets_location_ip_version;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ip_sets_location_ip_version
    ON ip_sets(location_id, ip_version);

DROP INDEX CONCURRENTLY IF EXISTS idx_ip_sets_network;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ip_sets_network
    ON ip_sets(network);

DROP INDEX CONCURRENTLY IF EXISTS idx_ip_addresses_set_status;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ip_addresses_set_status
    ON ip_addresses(ip_set_id, status);

DROP INDEX CONCURRENTLY IF EXISTS idx_ip_addresses_address_status;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ip_addresses_address_status
    ON ip_addresses(address, status);

DROP INDEX CONCURRENTLY IF EXISTS idx_ipv6_prefixes_node_id;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ipv6_prefixes_node_id
    ON ipv6_prefixes(node_id);

DROP INDEX CONCURRENTLY IF EXISTS idx_vm_ipv6_subnets_vm_id;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vm_ipv6_subnets_vm_id
    ON vm_ipv6_subnets(vm_id);

DROP INDEX CONCURRENTLY IF EXISTS idx_vm_ipv6_subnets_prefix_id;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_vm_ipv6_subnets_prefix_id
    ON vm_ipv6_subnets(ipv6_prefix_id);

-- ============================================================================
-- 000005: template_indexes
-- ============================================================================

DROP INDEX CONCURRENTLY IF EXISTS idx_templates_os_family_active;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_templates_os_family_active
    ON templates(os_family, is_active);

DROP INDEX CONCURRENTLY IF EXISTS idx_templates_name;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_templates_name
    ON templates(name);

DROP INDEX CONCURRENTLY IF EXISTS idx_templates_sort_order_name;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_templates_sort_order_name
    ON templates(sort_order, name);

-- ============================================================================
-- 000006: backup_schedules (indexes on pre-existing backups and snapshots tables)
-- Note: indexes on backup_schedules itself are omitted — that table was created
-- in the same migration 000006 transaction, so no rows existed and no ShareLock
-- contention could have occurred on a live table.
-- ============================================================================

DROP INDEX CONCURRENTLY IF EXISTS idx_backups_vm_created;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_backups_vm_created
    ON backups(vm_id, created_at DESC);

DROP INDEX CONCURRENTLY IF EXISTS idx_backups_expires_at;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_backups_expires_at
    ON backups(expires_at)
    WHERE expires_at IS NOT NULL;

DROP INDEX CONCURRENTLY IF EXISTS idx_snapshots_vm_created;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_snapshots_vm_created
    ON snapshots(vm_id, created_at DESC);

-- ============================================================================
-- 000007: audit_indexes
-- ============================================================================

DROP INDEX CONCURRENTLY IF EXISTS idx_audit_logs_actor_type_timestamp;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_actor_type_timestamp
    ON audit_logs(actor_type, timestamp DESC);

DROP INDEX CONCURRENTLY IF EXISTS idx_audit_logs_success_timestamp;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_success_timestamp
    ON audit_logs(success, timestamp DESC);

DROP INDEX CONCURRENTLY IF EXISTS idx_audit_logs_resource_timestamp;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_resource_timestamp
    ON audit_logs(resource_type, resource_id, timestamp DESC);
