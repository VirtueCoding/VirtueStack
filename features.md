# VirtueStack — Critical Gap Implementation Plan

> **Status:** PARTIAL — Gap #4 implemented; others remain as plans  
> **Date:** 2026-03-27  
> **Scope:** Critical gaps only — features without which VirtueStack cannot operate as a production VPS hosting business.

---

## 1. Unify Backup & Snapshot into "Backup" Only

### Why Remove Snapshots as a Separate Concept?

Today VirtueStack has **two overlapping features**:

| | Backup | Snapshot |
|-|--------|----------|
| **What it does** | Full disk copy to separate storage | Point-in-time disk reference |
| **Restore** | Replace disk (VM must stop) | Rollback in-place |
| **Expiration** | 30-day default | Never (manual delete) |
| **Consistency** | Application-consistent (guest freeze) | Crash-consistent |
| **Storage** | Separate pool/directory | Same pool as VM disk |
| **Scheduling** | Admin + customer schedules | Manual only |

The distinction is a **Ceph/storage-engineer mental model**, not a customer one. Real VPS customers think:

> *"I want a copy of my server I can restore later."*

They do not care whether that copy is an RBD clone in `vs-backups` or a snapshot in `vs-vms`. Exposing both concepts:

- **Doubles UI surface** — two pages, two sets of quota limits, two sets of API endpoints
- **Confuses customers** — "should I snapshot or backup before upgrading?"
- **Doubles maintenance** — two task handlers, two repository methods, two sets of tests
- **Creates quota gaming** — 2 backups + 2 snapshots = 4 recovery points when the plan intends 2

### Proposed Unified Model: "Backup"

Merge into a **single "Backup" concept** with two modes:

| Field | Value | Meaning |
|-------|-------|---------|
| `type` | `automatic` | Created by schedule (admin or customer) |
| `type` | `manual` | Created by customer on-demand |
| `method` | `full` | Exported to backup pool/directory (today's "backup") |
| `method` | `snapshot` | Point-in-time reference in-place (today's "snapshot") |

**Customer-facing simplification:**
- One "Backups" page in the portal
- One quota limit per plan: `backup_limit` (e.g., 5)
- Customer creates a backup → system picks optimal method based on storage backend
- Scheduled backups always use `full` method (safer for disaster recovery)
- Manual backups default to `snapshot` method (faster) with option to request `full`

**What changes:**

| Layer | Change |
|-------|--------|
| **Database** | Add `method` column to `backups`; migrate existing snapshots into `backups` table; drop `snapshots` table |
| **Models** | Remove `Snapshot` struct; add `Method` field to `Backup` |
| **Repository** | Remove `SnapshotRepository`; consolidate into `BackupRepository` |
| **Service** | Remove snapshot methods from `BackupService`; route by `method` field |
| **Tasks** | Merge `snapshot.create`/`snapshot.revert`/`snapshot.delete` into `backup.*` tasks |
| **Customer API** | Remove `/snapshots` endpoints; add `method` filter to `/backups` |
| **Admin API** | Update backup schedule to support method selection |
| **Customer WebUI** | Remove Snapshots page; add method toggle to Backups page |
| **Plan model** | Remove `snapshot_limit`; `backup_limit` covers both |
| **WHMCS** | No change (WHMCS doesn't manage snapshots today) |

**Migration strategy (expand-contract):**
1. Add `method` column to `backups` (default `full`)
2. Copy `snapshots` rows into `backups` with `method = 'snapshot'`
3. Dual-write period: both tables updated
4. Switch APIs to read from `backups` only
5. Drop `snapshots` table in later migration

---

## 2. Customer Password Reset

### Current State

- `RequestPasswordReset()` and `ResetPassword()` exist in the service layer
- `password_resets` database table exists (migration 000011)
- **No REST API endpoints exposed** — customers cannot reset their own passwords
- Email infrastructure exists but is never called for password resets

### Implementation Plan

| Step | Work |
|------|------|
| **API routes** | Add to `customer/routes.go`: `POST /auth/forgot-password`, `POST /auth/reset-password` |
| **Handler** | New `auth_password_reset.go` in `api/customer/` |
| **Email** | Wire `EmailProvider.SendPasswordReset()` in the handler |
| **Rate limit** | 3 requests/hour per email (prevent enumeration) |
| **Security** | Constant-time token comparison; token expires in 1 hour; single-use |
| **Frontend** | Add forgot-password page to customer WebUI login flow |
| **Tests** | Table-driven tests for happy path, expired token, invalid token, rate limit |

**Estimated scope:** ~200 lines Go + ~150 lines TSX

---

## 3. Customer Self-Registration

### Current State

Not needed for WHMCS-centric deployments. Customers register and pay through WHMCS. When a VPS order is provisioned, the WHMCS module calls `POST /provisioning/customers` which automatically creates (or retrieves) the matching VirtueStack customer account by email. There is no need for a separate public signup page.

**If self-registration is needed in the future** (e.g. for a standalone deployment without WHMCS):

| Step | Work |
|------|------|
| **API route** | Add `POST /auth/register` to `customer/routes.go` (no auth required) |
| **Handler** | New `auth_register.go` — validate email, hash password, create customer |
| **Email verification** | Send verification email; customer must confirm before creating VMs |
| **Model** | Add `email_verified` boolean + `verification_token` to `customers` table |
| **Migration** | `000065_customer_email_verification.up.sql` |
| **Rate limit** | 5 registrations/hour per IP |
| **Config guard** | `REGISTRATION_ENABLED=true/false` env var (default: false for WHMCS-only deployments) |
| **Frontend** | Add registration page with email + password form |
| **Tests** | Duplicate email, weak password, rate limit, email verification flow |

**Config guard is critical:** Many operators use WHMCS exclusively for customer management. Self-registration must be opt-in.

**Estimated scope:** ~300 lines Go + ~200 lines TSX + 1 migration

---

## 4. WHMCS Customer Auto-Provisioning ✅ IMPLEMENTED

### Flow

When a customer pays for a VPS in WHMCS and it triggers the module's `CreateAccount()`:

1. **WHMCS module** calls `virtuestack_ensureCustomer()` which calls `POST /provisioning/customers` with the WHMCS customer's email and name
2. **Controller** checks if a customer with that email already exists in VirtueStack
   - **If email exists:** returns the existing customer (no duplicate created)
   - **If email does not exist:** creates a new customer with the same email + a cryptographically random password, returns the new customer
3. **WHMCS module** then calls `POST /provisioning/vms` with the `customer_id` to create the VM under that customer's ownership

The random password is generated because WHMCS manages customer authentication via SSO — customers access the VirtueStack portal through WHMCS Single Sign-On and never need to know their VirtueStack password.

### Implementation

| Component | File | What |
|-----------|------|------|
| **Handler** | `internal/controller/api/provisioning/customers.go` | `CreateOrGetCustomer` — idempotent create-or-get by email |
| **Route** | `internal/controller/api/provisioning/routes.go` | `POST /provisioning/customers` |
| **WHMCS module** | `modules/servers/virtuestack/virtuestack.php` | `virtuestack_ensureCustomer()` calls `createCustomer()` before `createVM()` |
| **API client** | `modules/servers/virtuestack/lib/ApiClient.php` | `createCustomer()` → `POST /provisioning/customers` |
| **Tests** | `internal/controller/api/provisioning/customers_test.go` | Validation, response format |

---

## 5. WHMCS Usage Metering

### What `virtuestack_UsageUpdate()` Does Today

The `virtuestack_UsageUpdate()` function in `modules/servers/virtuestack/virtuestack.php` is called periodically by WHMCS's cron job to collect resource usage data for each service. WHMCS uses this data to:

- Display usage statistics in the admin and client areas
- Enforce overage charges (if configured with metered billing)
- Suspend services that exceed their limits

**Current implementation** (lines 1073–1094 of `virtuestack.php`):
1. Gets the API client and syncs the VM's current state from VirtueStack
2. Returns the VM's **allocated** resources: `vcpu`, `memory_mb`, `disk_gb`, `bandwidth_limit_gb`

**Problem:** It returns what the plan *allocates*, not what the VM actually *uses*. WHMCS expects fields like `diskusage` (actual GB used) and `bwusage` (actual bandwidth consumed this month) to enable metered billing and overage charges. Without actual usage data, only fixed-price plans work.

### Implementation Plan

| Step | Work |
|------|------|
| **Provisioning API** | Add `GET /provisioning/vms/:id/usage` returning actual bandwidth, disk, CPU usage |
| **WHMCS module** | Update `UsageUpdate()` to fetch real metrics and report to WHMCS |
| **Bandwidth** | `BandwidthService.GetMonthlyUsage()` already tracks this — expose via provisioning API |
| **Disk** | Query node agent for actual disk usage (not just allocated size) |
| **Response format** | Match WHMCS `UsageUpdate` expected fields: `diskusage`, `disklimit`, `bwusage`, `bwlimit` |

**Estimated scope:** ~100 lines Go + ~40 lines PHP

---

## Summary — Priority Order

| # | Gap | Severity | Scope | Dependencies |
|---|-----|----------|-------|--------------|
| 1 | Unify Backup/Snapshot | HIGH | Large (1–2 weeks) | Migration, all layers |
| 2 | Customer Password Reset | CRITICAL | Small (1–2 days) | Email infra (exists) |
| 3 | Customer Self-Registration | LOW | Medium (3–5 days) | Email verification, migration (not needed for WHMCS-only) |
| 4 | ~~WHMCS Customer Auto-Provisioning~~ | ✅ DONE | — | `POST /provisioning/customers` implemented |
| 5 | WHMCS Usage Metering | HIGH | Small (1–2 days) | Bandwidth service (exists) |

**Recommended execution order:** 2 → 5 → 1 (→ 3 only if self-registration is needed)

Password reset is a quick win that unblocks production. WHMCS customer auto-provisioning is already done (Gap #4). Self-registration is needed only if operating without WHMCS. Backup/snapshot unification is the largest refactor and should be planned as a dedicated sprint.

---

## Non-Goals (Already Working)

These features were audited and are **not** critical gaps:

- ✅ VM Console (VNC + Serial)
- ✅ Bandwidth monitoring/throttling
- ✅ Node health + heartbeat
- ✅ Automatic failover
- ✅ DNS/rDNS management
- ✅ IPv6 support
- ✅ Rate limiting
- ✅ Prometheus metrics
- ✅ Backup scheduler execution
- ✅ Disk resize
- ✅ ISO management
- ✅ Template management (build from ISO, distribute)
- ✅ Audit logging
- ✅ OS reinstall (customer)
- ✅ Multi-admin (2 roles: admin, super_admin)
