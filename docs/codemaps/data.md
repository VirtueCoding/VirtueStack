<!-- Generated: 2026-03-30 | Files scanned: 71 migrations | Token estimate: ~1100 -->

# Data Architecture

## Core Tables (30+ tables)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              ENTITY RELATIONSHIPS                            │
└─────────────────────────────────────────────────────────────────────────────┘

locations ──┬── nodes ────────┬── ipv6_prefixes ── vm_ipv6_subnets
            │                 │
            │                 ├── node_heartbeats
            │                 │
            │                 └── vms ──────┬── snapshots
            │                               ├── backups
            │                               ├── ip_addresses
            │                               └── iso_uploads
            │
            └── ip_sets ──────── ip_addresses

customers ──┬── vms
            ├── sessions
            ├── customer_api_keys
            ├── customer_webhooks ── webhook_deliveries
            ├── notification_preferences
            └── sso_tokens          # WHMCS SSO bootstrap

admins ───┬── sessions
          ├── console_tokens     # Time-limited console access
          └── admin_permissions  # RBAC permissions

plans ─── vms

templates ──┬── vms
            └── template_node_cache  # QCOW/LVM node template caching

storage_backends ── node_storage (junction) ── nodes

provisioning_keys (WHMCS API keys)

tasks (async job queue)

audit_logs (partitioned by timestamp)

system_settings (key-value)

failover_requests (HA tracking)

admin_backup_schedules (mass backup campaigns)

system_webhooks (system-level webhook configs)

email_verification_tokens (customer email verification)

pre_action_webhooks (pre-action approval webhooks)
```

## Table Schemas

### Identity & Auth

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `customers` | id, email, password_hash, totp_*, status | Customer accounts |
| `admins` | id, email, password_hash, totp_*, role | Admin users |
| `sessions` | id, user_id, user_type, refresh_token_hash, expires_at | JWT refresh |
| `customer_api_keys` | id, customer_id, key_hash, vm_ids, permissions, allowed_ips, expires_at | API auth |
| `provisioning_keys` | id, key_hash, allowed_ips, expires_at | WHMCS auth |
| `console_tokens` | id, token_hash, user_id, user_type, vm_id, console_type, expires_at | Console access |
| `admin_permissions` | id, admin_id, permissions (jsonb) | RBAC |
| `sso_tokens` | id, token_hash, customer_id, vm_id, redirect_path, expires_at | SSO bootstrap |
| `password_resets` | id, user_id, token_hash, expires_at | Password reset flow |

### Infrastructure

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `locations` | id, name, region, country | Data centers |
| `nodes` | id, hostname, grpc_address, status, storage_backend | Hypervisors |
| `node_heartbeats` | id, node_id, timestamp, cpu_percent, memory_percent | Health metrics |
| `ip_sets` | id, location_id, network, gateway | IP pools |
| `ip_addresses` | id, ip_set_id, address, vm_id, rdns_hostname | IP allocations |
| `ipv6_prefixes` | id, node_id, prefix | /48 allocations |
| `vm_ipv6_subnets` | id, vm_id, ipv6_prefix_id, subnet | /64 subnets |
| `storage_backends` | id, name, type, ceph_*, storage_path, lvm_*, health_* | Storage backend registry |
| `node_storage` | node_id, storage_backend_id, enabled | Node-to-backend junction |

### VM Resources

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `plans` | id, name, slug, vcpu, memory_mb, disk_gb, snapshot_limit, backup_limit, iso_upload_limit | VPS tiers |
| `templates` | id, name, os_family, rbd_image, storage_backend, file_path | OS images |
| `template_node_cache` | template_id, node_id, status, local_path, size_bytes | Template distribution cache |
| `vms` | id, customer_id, node_id, plan_id, hostname, status, storage_backend, disk_path, storage_backend_id, attached_iso | Virtual machines |
| `snapshots` | id, vm_id, name, rbd_snapshot | Point-in-time |
| `backups` | id, vm_id, source, status, storage_path, size_bytes | Backups |
| `backup_schedules` | id, vm_id, interval, retention | Scheduled backups |
| `admin_backup_schedules` | id, name, frequency, target_*, retention_count | Mass backup campaigns |
| `iso_uploads` | id, vm_id, filename, size_bytes, storage_path | Uploaded ISOs |

### Async & Audit

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `tasks` | id, type, status, payload, result, progress, progress_message, retry_count | Job queue |
| `audit_logs` | id, timestamp, actor_id, action, resource_type, changes | Immutable trail |
| `failover_requests` | id, source_node_id, target_node_id, status | HA tracking |

### Integrations

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `customer_webhooks` | id, customer_id, url, secret, events | Webhooks |
| `webhook_deliveries` | id, webhook_id, event_type, status, idempotency_key | Delivery log |
| `notification_preferences` | id, customer_id, channels | Notification settings |
| `system_settings` | key, value (jsonb) | Config |
| `system_webhooks` | id, name, url, secret, events[], is_active | System-level webhooks |
| `email_verification_tokens` | id, customer_id, token_hash, expires_at | Email verification |
| `pre_action_webhooks` | id, name, url, secret, events[], timeout_ms, fail_open, is_active | Pre-action approval webhooks |

## Row Level Security

```sql
-- Customer isolation enforced at DB level
ALTER TABLE vms ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_vms ON vms FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Also protected: customer_api_keys, ip_addresses, backups, snapshots,
-- backup_schedules, sessions, notification_preferences, notification_events,
-- console_tokens, customers, password_resets, email_verification_tokens,
-- system_webhooks, pre_action_webhooks
```

## Index Strategy

| Table | Index | Purpose |
|-------|-------|---------|
| vms | idx_vms_customer_id, idx_vms_node_id, idx_vms_status | Lookups |
| ip_addresses | idx_ip_addresses_vm_id, idx_ip_addresses_status | Allocation |
| tasks | idx_tasks_status, idx_tasks_status_created | Queue queries |
| audit_logs | idx_audit_logs_actor_id, idx_audit_logs_timestamp | Search |
| backups | idx_backups_vm_id, idx_backups_status_created | List/restore |
| console_tokens | idx_console_tokens_expires_at | Token lookup |
| vm_ipv6_subnets | unique(vm_id, subnet) | Subnet uniqueness |

## Migration History

| Range | Purpose |
|-------|---------|
| 000001 | Initial schema |
| 000002-000008 | Indexes, bandwidth, notifications, webhooks |
| 000009-000018 | Features (API keys, password reset, failed logins) |
| 000019-000021 | Storage backends, failover, ISO |
| 000022-000028 | RLS policies, constraints |
| 000029-000034 | Performance indexes, plan limits |
| 000035-000038 | Ceph config, admin backup schedules, API key IP whitelist |
| 000039-000044 | Console tokens, RLS policies, plan cleanup, admin permissions |
| 000045-000053 | Schema indexes, RLS fixes, FK constraints, audit log partitions |
| 000054-000057 | Rename ceph to storage stats, storage backend registry, LVM thresholds |
| 000058-000060 | IPv6 subnet uniqueness, bandwidth snapshots, task progress messages |
| 000061-000065 | ISO uploads, SSO tokens, provisioning key expiry, template cache, unify backup/snapshot |
| 000066-000071 | VM state machine constraint, task retry count, system webhooks, email verification, pre-action webhooks, review fixes (RLS + GIN indexes) |

## Database Roles

```sql
app_user      -- Controller connection (read/write)
app_customer  -- RLS isolation (SET ROLE for customer context)
```