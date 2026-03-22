<!-- Generated: 2026-03-22 | Files scanned: 46 migrations | Token estimate: ~950 -->

# Data Architecture

## Core Tables (27 tables)

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
            │                               └── ip_addresses
            │
            └── ip_sets ──────── ip_addresses

customers ──┬── vms
            ├── sessions
            ├── customer_api_keys
            ├── customer_webhooks ── webhook_deliveries
            └── notification_preferences

admins ───┬── sessions
          ├── console_tokens     # Time-limited console access
          └── admin_permissions  # RBAC permissions (NEW)

plans ─── vms

templates ─── vms

provisioning_keys (WHMCS API keys)

tasks (async job queue)

audit_logs (partitioned by timestamp)

system_settings (key-value)

failover_requests (HA tracking)

admin_backup_schedules (mass backup campaigns)
```

## Table Schemas

### Identity & Auth

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `customers` | id, email, password_hash, totp_*, status | Customer accounts |
| `admins` | id, email, password_hash, totp_*, role | Admin users |
| `sessions` | id, user_id, user_type, refresh_token_hash, expires_at | JWT refresh |
| `customer_api_keys` | id, customer_id, key_hash, vm_ids, permissions, allowed_ips | API auth |
| `provisioning_keys` | id, key_hash, allowed_ips | WHMCS auth |
| `console_tokens` | id, admin_id, vm_id, type, expires_at | Console access (NEW) |
| `admin_permissions` | id, admin_id, permissions (jsonb) | RBAC (NEW) |

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

### VM Resources

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `plans` | id, name, vcpu, memory_mb, disk_gb, snapshot_limit, backup_limit | VPS tiers |
| `templates` | id, name, os_family, rbd_image, rbd_snapshot | OS images |
| `vms` | id, customer_id, node_id, plan_id, hostname, status, storage_backend, attached_iso | Virtual machines |
| `snapshots` | id, vm_id, name, rbd_snapshot | Point-in-time |
| `backups` | id, vm_id, source, status, storage_path | Backups |
| `backup_schedules` | id, vm_id, interval, retention | Scheduled backups |
| `admin_backup_schedules` | id, name, frequency, target_*, retention_count | Mass backup campaigns |

### Async & Audit

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `tasks` | id, type, status, payload, result, progress | Job queue |
| `audit_logs` | id, timestamp, actor_id, action, resource_type, changes | Immutable trail |
| `failover_requests` | id, source_node_id, target_node_id, status | HA tracking |

### Integrations

| Table | Key Columns | Purpose |
|-------|-------------|---------|
| `customer_webhooks` | id, customer_id, url, secret, events | Webhooks |
| `webhook_deliveries` | id, webhook_id, event_type, status, attempt | Delivery log |
| `system_settings` | key, value (jsonb) | Config |

## Row Level Security

```sql
-- Customer isolation enforced at DB level
ALTER TABLE vms ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_vms ON vms FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Also protected: customer_api_keys, ip_addresses, backups, snapshots,
-- backup_schedules, sessions, notification_preferences, notification_events,
-- console_tokens
```

## Index Strategy

| Table | Index | Purpose |
|-------|-------|---------|
| vms | idx_vms_customer_id, idx_vms_node_id, idx_vms_status | Lookups |
| ip_addresses | idx_ip_addresses_vm_id, idx_ip_addresses_status | Allocation |
| tasks | idx_tasks_status, idx_tasks_status_created | Queue queries |
| audit_logs | idx_audit_logs_actor_id, idx_audit_logs_timestamp | Search |
| backups | idx_backups_vm_id, idx_backups_status_created | List/restore |
| console_tokens | idx_console_tokens_admin_id, idx_console_tokens_expires_at | Token lookup (NEW) |

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
| 000039-000044 | Console tokens, RLS policies, plan cleanup, admin permissions (NEW) |

## Database Roles

```sql
app_user      -- Controller connection (read/write)
app_customer  -- RLS isolation (SET ROLE for customer context)
```

## New Tables (Since Last Update)

| Table | Migration | Purpose |
|-------|-----------|---------|
| `console_tokens` | 000039 | Time-limited VNC/serial console access |
| `admin_permissions` | 000044 | Admin role-based access control |