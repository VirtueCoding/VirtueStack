# VirtueStack Implementation Tasks

This document tracks pending implementation work and technical notes.

---

## Feature Implementation Required

### Admin Backup Management UI

**Status:** API exists, UI missing. Admins cannot manage backups through the web interface.

**Current State:**
- ✅ Backend API endpoints exist (`/admin/backups`, `/admin/backup-schedules`)
- ❌ No Admin WebUI components for backup management
- ❌ No mass backup scheduling (admin-level campaigns)
- ❌ No distinction between manual vs scheduled backups

#### 1. Database Schema Changes

**Decision:** Full admin schedule capabilities with separate tables.

```sql
-- Migration: 000037_admin_backup_schedules.sql

-- New table for admin mass backup schedules
CREATE TABLE admin_backup_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    frequency VARCHAR(20) NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    retention_count INTEGER NOT NULL DEFAULT 3,

    -- Targeting options (at least one required)
    target_all BOOLEAN DEFAULT FALSE,
    target_plan_ids UUID[],
    target_node_ids UUID[],
    target_customer_ids UUID[],

    -- Schedule configuration
    active BOOLEAN DEFAULT TRUE,
    next_run_at TIMESTAMPTZ NOT NULL,
    last_run_at TIMESTAMPTZ,

    -- Metadata
    created_by UUID REFERENCES admins(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Track which admin schedule created each backup
ALTER TABLE backups ADD COLUMN admin_schedule_id UUID REFERENCES admin_backup_schedules(id);
ALTER TABLE backups ADD COLUMN source VARCHAR(20) DEFAULT 'manual' CHECK (source IN ('manual', 'customer_schedule', 'admin_schedule'));

-- Remove the now-redundant 'type' column (only had 'full')
ALTER TABLE backups DROP COLUMN type;

-- Index for filtering
CREATE INDEX idx_backups_source ON backups(source);
CREATE INDEX idx_backups_admin_schedule ON backups(admin_schedule_id);
CREATE INDEX idx_admin_backup_schedules_next_run ON admin_backup_schedules(next_run_at) WHERE active = TRUE;
```

---

#### 2. Go Code Cleanup (Remove `type` column references)

The `type` column only ever had value `"full"` and is now redundant. Remove all references:

##### 2.0 Models & Repository
- [ ] `internal/controller/models/backup.go:19` - Remove `Type string` field
- [ ] `internal/controller/repository/backup_repo.go:30` - Remove `Type *string` from `BackupListFilter`
- [ ] `internal/controller/repository/backup_repo.go` - Remove `type` from all SELECT/INSERT queries (lines 55, 64, 73, 78)

##### 2.1 Task Payload Updates
- [ ] `internal/controller/tasks/handlers_types.go:260` - Change to:
  ```go
  type BackupCreatePayload struct {
      VMID            string `json:"vm_id"`
      BackupName      string `json:"backup_name"`
      Source          string `json:"source"` // "manual", "customer_schedule", "admin_schedule"
      AdminScheduleID string `json:"admin_schedule_id,omitempty"` // set when source == "admin_schedule"
  }
  ```

##### 2.2 Task Handler Updates
- [ ] `internal/controller/tasks/backup_create.go:92` - Change log key from `backup_type` to `source`
- [ ] `internal/controller/tasks/backup_create.go:96` - Change metrics label from `BackupType` to `Source`
- [ ] `internal/controller/tasks/backup_create.go:247,354` - Remove `Type: payload.BackupType`, add `Source: payload.Source`
- [ ] `internal/controller/tasks/backup_create.go` - Add `AdminScheduleID: payload.AdminScheduleID` when creating backup record

##### 2.3 Service Layer Updates
- [ ] `internal/controller/services/backup_service.go` - Update `CreateBackup()` to accept `source` parameter
- [ ] `internal/controller/services/backup_service.go` - Set `source = "manual"` for customer-initiated backups
- [ ] Schedule worker - Set `source = "customer_schedule"` when executing customer schedules
- [ ] Admin schedule worker - Set `source = "admin_schedule"` when executing admin schedules

##### 2.4 API Handler Updates
- [ ] `internal/controller/api/customer/backups.go` - Pass `source = "manual"` when calling CreateBackup
- [ ] Remove any `type` field from API request/response structs

##### 2.5 Frontend Updates
- [ ] `webui/customer/lib/api-client.ts` - Remove `type` field from `Backup` interface
- [ ] `webui/customer/lib/api-client.ts` - Remove `type` field from `CreateBackupRequest` interface

---

#### 3. Backend API Enhancements (New Admin Features)

##### 3.1 Admin Backup Schedule Endpoints
- [ ] Add `GET /admin/backup-schedules/stats` - overview statistics
- [ ] Add `POST /admin/backup-schedules/:id/run` - trigger immediate execution
- [ ] Add `GET /admin/backup-schedules/:id/backups` - list backups from this schedule

##### 3.2 Admin Backup List Enhancements
- [ ] Add `source` filter to `GET /admin/backups`
- [ ] Add `admin_schedule_id` filter to `GET /admin/backups`
- [ ] Add bulk operations endpoint `POST /admin/backups/bulk-restore`

##### 3.3 Service Layer
- [ ] `AdminBackupScheduleService` - handles mass schedule execution
- [ ] `ExecuteAdminSchedule(ctx, scheduleID)` - creates backups for all target VMs
- [ ] `GetBackupStats(ctx)` - returns counts by status, source, node

---

#### 4. Admin WebUI Components

##### 4.1 Backup List Page (`webui/admin/app/backups/page.tsx`)
```
Features:
- Table with columns: VM, Customer, Status, Size, Source, Created, Actions
- Filters: Customer, VM, Status, Source (manual/customer_schedule/admin_schedule), Date range
- Search by VM hostname or backup ID
- Bulk select for restore/delete operations
- Export to CSV
- Pagination (50 items/page)
```

##### 4.2 Backup Detail Modal
```
Features:
- Full backup metadata display
- VM info card (hostname, IP, plan)
- Restore button with confirmation dialog
- Download button (if storage supports)
- Delete button with confirmation
- Audit log entries for this backup
```

##### 4.3 Admin Schedule List Page (`webui/admin/app/backup-schedules/page.tsx`)
```
Features:
- Two sections: Customer Schedules | Admin Schedules (tabbed)
- Customer Schedules: shows per-VM schedules created by customers
- Admin Schedules: mass backup campaigns
- Each schedule shows: Name, Targets, Frequency, Next Run, Status
```

##### 4.4 Create Admin Schedule Modal
```
Form fields:
- Name (required)
- Description
- Frequency: daily/weekly/monthly
- Retention count
- Target selection:
  - Radio: All VMs | Selected Plans | Selected Nodes | Selected Customers
  - Multi-select dropdowns for each target type
- Active toggle
- Schedule preview (shows X VMs will be backed up)
```

##### 4.5 Schedule Detail Page
```
Features:
- Schedule configuration display
- Target list (expandable: shows all affected VMs)
- Recent backups from this schedule
- Execution history timeline
- Edit/Disable/Delete actions
```

---

#### 5. File Structure

```
webui/admin/
├── app/
│   ├── backups/
│   │   ├── page.tsx                    # Backup list
│   │   └── [id]/
│   │       └── page.tsx                # Backup detail
│   └── backup-schedules/
│       ├── page.tsx                    # Schedule list (tabbed)
│       └── [id]/
│           └── page.tsx                # Schedule detail
├── components/
│   └── backups/
│       ├── BackupList.tsx              # Data table component
│       ├── BackupFilters.tsx           # Filter sidebar
│       ├── BackupDetailModal.tsx       # Detail view
│       ├── RestoreConfirmModal.tsx     # Restore confirmation
│       ├── AdminScheduleList.tsx       # Admin schedules table
│       ├── CustomerScheduleList.tsx    # Customer schedules table
│       ├── CreateScheduleModal.tsx     # Create admin schedule
│       ├── ScheduleTargetSelector.tsx  # Target VM selection
│       └── ScheduleDetailPanel.tsx     # Schedule info panel
└── lib/
    └── api-client.ts                   # Add backup API methods
```

---

#### 6. API Client Methods

```typescript
// webui/admin/lib/api-client.ts

export interface AdminBackup {
  id: string;
  vm_id: string;
  vm_hostname: string;
  customer_id: string;
  customer_email: string;
  source: 'manual' | 'customer_schedule' | 'admin_schedule';
  admin_schedule_id?: string;
  admin_schedule_name?: string;
  status: 'creating' | 'completed' | 'failed' | 'restoring';
  size_bytes: number;
  created_at: string;
  expires_at?: string;
}

export interface AdminBackupSchedule {
  id: string;
  name: string;
  description?: string;
  frequency: 'daily' | 'weekly' | 'monthly';
  retention_count: number;
  target_all: boolean;
  target_plan_ids?: string[];
  target_node_ids?: string[];
  target_customer_ids?: string[];
  active: boolean;
  next_run_at: string;
  last_run_at?: string;
  vm_count: number;  // Number of VMs this schedule targets
  created_at: string;
}

export const adminBackupApi = {
  listBackups: (filters: BackupListFilters) => apiClient.get<...>('/admin/backups?...'),
  restoreBackup: (id: string) => apiClient.post(`/admin/backups/${id}/restore`),
  bulkRestore: (ids: string[]) => apiClient.post('/admin/backups/bulk-restore', { ids }),

  listAdminSchedules: () => apiClient.get<AdminBackupSchedule[]>('/admin/backup-schedules'),
  createAdminSchedule: (data: CreateAdminScheduleRequest) => apiClient.post('/admin/backup-schedules', data),
  updateAdminSchedule: (id: string, data: UpdateAdminScheduleRequest) => apiClient.put(`/admin/backup-schedules/${id}`, data),
  deleteAdminSchedule: (id: string) => apiClient.delete(`/admin/backup-schedules/${id}`),
  runScheduleNow: (id: string) => apiClient.post(`/admin/backup-schedules/${id}/run`),

  getBackupStats: () => apiClient.get<BackupStats>('/admin/backups/stats'),
};
```

---

#### 7. Implementation Order

**Phase 1: Database & Cleanup** (Day 1)
1. [x] Create migration `000037_admin_backup_schedules.sql`
2. [x] Remove `type` column from `backups` table
3. [x] Add `source` column to `backups` table
4. [x] Remove `Type` field from `models.Backup`
5. [x] Remove `type` from all backup repository queries
6. [x] Update `BackupCreatePayload` to use `Source` instead of `BackupType`
7. [x] Update backup task handlers to set `source` correctly
8. [x] Update frontend `Backup` interface to remove `type`

**Phase 2: Admin Schedule Backend** (Day 2)
1. [x] Add `AdminBackupSchedule` model
2. [x] Add repository methods for admin schedules
3. [x] Add API endpoints for admin schedules
4. [x] Update `backups` endpoint to include source filter
5. [x] Create admin schedule execution worker

**Phase 3: Core UI Components** (Day 3-4)
1. [x] Create `BackupList` component with filters
2. [x] Create `BackupDetailModal` component
3. [x] Create `RestoreConfirmModal` component
4. [x] Add backup API methods to api-client.ts
5. [x] Create `/backups` page

**Phase 4: Admin Schedule UI** (Day 5-6)
1. [x] Create `AdminScheduleList` component
2. [x] Create `CreateScheduleModal` with target selector
3. [x] Create `ScheduleDetailPanel` component
4. [x] Create `/backup-schedules` page
5. [x] Add schedule API methods to api-client.ts

**Phase 5: Testing** (Day 7)
1. [x] Add integration tests for admin schedule execution
2. [x] Add E2E tests for admin backup list page
3. [x] Add E2E tests for admin schedule creation
4. [x] Manual testing of all flows (automated via E2E tests in `admin-backups.spec.ts`)

---

#### 8. Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Backup source tracking | Separate `source` column | Enables filtering by manual/customer_schedule/admin_schedule |
| Admin schedule targets | Multiple target types (OR logic) | Flexibility to target by plan, node, customer, or all |
| `type` column | Remove entirely | Redundant now that only full backups exist |
| Execution timing | Staggered across time window | Avoid storage I/O spikes when backing up many VMs |
| Backup record creation | On-demand during execution | Don't pre-create thousands of records for large schedules |

---

### Customer API Key IP Whitelist

**Status:** ✅ COMPLETE

**Current State:**
- ✅ Backend API exists: `GET/POST/DELETE /customer/api-keys`, `POST /customer/api-keys/:id/rotate`
- ✅ Backend has `expires_at` support (model, DB, API)
- ✅ Frontend has basic API key management (`ApiKeysTab.tsx`)
- ✅ Backend has `allowed_ips` column in `customer_api_keys` table
- ✅ Backend has `AllowedIPs` field in `CustomerAPIKey` model
- ✅ Backend has IP whitelist validation in auth middleware
- ✅ Frontend has IP whitelist input in create dialog
- ✅ Frontend has expiration date input in create dialog
- ✅ Frontend displays allowed IPs and expiration in key list

**Reference:** `ProvisioningKey` model already has full IP whitelist implementation with `AllowedIPs []string` and `IsAllowedIP(ip string) bool` method.

#### 1. Database Migration

```sql
-- Migration: 000037_customer_api_key_allowed_ips.up.sql

-- Add allowed_ips column for IP whitelist (IPv4/IPv6/CIDR support)
ALTER TABLE customer_api_keys ADD COLUMN allowed_ips TEXT[];

-- Add index for future IP-based queries if needed
CREATE INDEX idx_customer_api_keys_allowed_ips ON customer_api_keys USING GIN(allowed_ips);
```

```sql
-- Migration: 000037_customer_api_key_allowed_ips.down.sql

ALTER TABLE customer_api_keys DROP COLUMN allowed_ips;
DROP INDEX IF EXISTS idx_customer_api_keys_allowed_ips;
```

---

#### 2. Backend Changes

##### 2.1 Model Updates (`internal/controller/models/provisioning_key.go`)
- [x] Add `AllowedIPs []string` field to `CustomerAPIKey` struct (line 75-87)
  ```go
  type CustomerAPIKey struct {
      // ... existing fields ...
      AllowedIPs   []string   `json:"allowed_ips,omitempty" db:"allowed_ips"`
      // ...
  }
  ```
- [x] Add `IsAllowedIP(ip string) bool` method to `CustomerAPIKey` (copy from `ProvisioningKey`)

##### 2.2 API Handler Updates (`internal/controller/api/customer/apikeys.go`)
- [x] Add `AllowedIPs []string` to `CreateAPIKeyRequest` struct (line 22-26)
  ```go
  type CreateAPIKeyRequest struct {
      Name        string   `json:"name" validate:"required,max=100"`
      Permissions []string `json:"permissions" validate:"required,min=1,dive,max=100"`
      AllowedIPs  []string `json:"allowed_ips,omitempty" validate:"max=50,dive,ip|cidr"`
      ExpiresAt   *string  `json:"expires_at,omitempty"`
  }
  ```
- [x] Add `AllowedIPs []string` to `APIKeyResponse` struct (line 34-43)
- [x] Update `CreateAPIKey` handler to pass `AllowedIPs` to model (line 144-151)
- [x] Update `ListAPIKeys` handler to include `AllowedIPs` in response (line 75-96)

##### 2.3 Repository Updates (`internal/controller/repository/customer_api_key_repo.go`)
- [x] Add `allowed_ips` to INSERT query in `Create()` method
- [x] Add `allowed_ips` to SELECT columns in `GetByIDAndCustomer()` and `GetByHash()`
- [x] Add `allowed_ips` to `ListByCustomer()` query

##### 2.4 Auth Middleware Updates (`internal/controller/api/middleware/auth.go`)
- [x] Update `CustomerAPIKeyValidator` to check IP whitelist
  ```go
  // After validating key hash and expiration, add:
  if len(key.AllowedIPs) > 0 {
      clientIP := c.ClientIP()
      if !key.IsAllowedIP(clientIP) {
          return nil, errors.New("IP address not in whitelist")
      }
  }
  ```

---

#### 3. Frontend Changes

##### 3.1 API Client Updates (`webui/customer/lib/api-client.ts`)
- [x] Add `allowed_ips?: string[]` to `ApiKey` interface (line 460-469)
- [x] Add `allowed_ips?: string[]` to `CreateApiKeyRequest` interface (line 483-487)

##### 3.2 UI Updates (`webui/customer/components/settings/ApiKeysTab.tsx`)
- [x] Add `allowed_ips` to form schema (line 26-29)
  ```typescript
  const apiKeySchema = z.object({
    name: z.string().min(1, "Name is required"),
    permissions: z.array(z.string()).min(1, "At least one permission is required"),
    allowed_ips: z.array(z.string()).optional(),
    expires_at: z.string().optional(),
  });
  ```
- [x] Add IP whitelist textarea input in Create dialog (after permissions section)
  - Placeholder: "192.168.1.1\n10.0.0.0/24\n2001:db8::/32"
  - Helper text: "One IP or CIDR per line. Leave empty to allow all IPs."
  - Validation: IPv4, IPv6, or CIDR notation
- [x] Add expiration date input in Create dialog
  - Use date picker or datetime input
  - Optional field
- [x] Display allowed IPs in key list (show count with expandable list)
- [x] Display expiration date in key list (show "Expires: X" or "Never expires")

---

#### 4. File Structure

```
Files to modify:
├── migrations/
│   ├── 000038_customer_api_key_allowed_ips.up.sql    # NEW
│   └── 000038_customer_api_key_allowed_ips.down.sql  # NEW
├── internal/controller/
│   ├── models/provisioning_key.go                    # Add AllowedIPs field + method
│   ├── api/customer/apikeys.go                       # Update request/response structs
│   ├── api/middleware/auth.go                        # Add IP validation
│   └── repository/customer_api_key_repo.go           # Update queries
└── webui/customer/
    ├── lib/api-client.ts                             # Update interfaces
    └── components/settings/ApiKeysTab.tsx            # Add IP/expiration inputs
```

---

#### 5. Implementation Order

**Phase 1: Backend** (1-2 hours)
1. [x] Create migration for `allowed_ips` column
2. [x] Update `CustomerAPIKey` model with `AllowedIPs` field and `IsAllowedIP()` method
3. [x] Update `CreateAPIKeyRequest` and `APIKeyResponse` structs
4. [x] Update repository queries
5. [x] Update auth middleware to validate IP whitelist

**Phase 2: Frontend** (1-2 hours)
1. [x] Update `ApiKey` and `CreateApiKeyRequest` interfaces
2. [x] Add IP whitelist textarea to create dialog
3. [x] Add expiration date input to create dialog
4. [x] Display allowed IPs and expiration in key list
5. [ ] Test with various IP formats (IPv4, IPv6, CIDR)

---

#### 6. Testing Checklist

- [x] Create API key with no IP restriction (empty allowed_ips)
- [x] Create API key with IPv4 address (e.g., `192.168.1.100`)
- [x] Create API key with IPv6 address (e.g., `2001:db8::1`)
- [x] Create API key with CIDR notation (e.g., `10.0.0.0/24`)
- [x] Create API key with mixed IPv4/IPv6/CIDR
- [x] Verify API key rejected from non-whitelisted IP
- [x] Verify API key accepted from whitelisted IP
- [x] Verify expiration date display and enforcement
- [x] Test rotate and delete operations still work

---

### Admin Templates Management UI

**Status:** ✅ COMPLETE

**Current State:**
- ✅ Backend API exists: `GET/POST/PUT/DELETE /admin/templates`, `POST /admin/templates/:id/import`
- ✅ Admin api-client.ts has methods: `getTemplates()`, `createTemplate()`, `updateTemplate()`, `deleteTemplate()`, `importTemplate()`
- ✅ Admin WebUI page at `/templates`
- ✅ Navigation item in admin sidebar
- ✅ Components for template management

#### Implementation Tasks

- [x] Add "Templates" to admin navigation (`webui/admin/lib/navigation.ts`)
- [x] Create `/templates` page (`webui/admin/app/templates/page.tsx`)
- [x] Create `TemplateList` component with table showing: name, os_type, size, status
- [x] Create `TemplateCreateDialog` component
- [x] Create `TemplateEditDialog` component
- [x] Add import template functionality (calls `importTemplate` API)
- [x] Add delete confirmation dialog

---

### Customer ISO Upload UI

**Status:** ✅ COMPLETE

**Current State:**
- ✅ Backend API: `GET/POST/DELETE /customer/vms/:id/iso`, `POST /customer/vms/:id/iso/:isoId/attach|detach`
- ✅ Customer api-client.ts has `isoApi` with all methods
- ✅ Component exists: `webui/customer/components/file-upload/iso-upload.tsx`
- ✅ Component integrated into VM detail page
- ✅ ISO tab added to VM detail page

#### Implementation Tasks

- [x] Add "ISO" tab to VM detail page tabs (`webui/customer/app/vms/[id]/page.tsx`)
- [x] Create `VMISOTab` component that uses `ISOUpload` component
- [x] Add ISO list view showing uploaded ISOs
- [x] Add attach/detach ISO functionality
- [x] Add delete ISO functionality
- [x] Update tab count from 7 to 8 tabs

---

### Customer RDNS Management UI

**Status:** ✅ COMPLETE

**Current State:**
- ✅ Backend API: `GET /customer/vms/:id/ips`, `GET/PUT/DELETE /customer/vms/:id/ips/:ipId/rdns`
- ✅ api-client methods in `webui/customer/lib/api-client.ts`
- ✅ UI component: `webui/customer/components/vm/VMRDNSTab.tsx`
- ✅ Integration in VM detail page

#### Implementation Tasks

##### Backend API Client
- [x] Add `rdnsApi` to `webui/customer/lib/api-client.ts`
  ```typescript
  export interface IPAddressRecord {
    id: string;
    ip_set_id: string;
    address: string;
    ip_version: number;
    vm_id?: string;
    customer_id?: string;
    is_primary: boolean;
    rdns_hostname?: string;
    status: string;
    assigned_at?: string;
    released_at?: string;
    cooldown_until?: string;
    created_at: string;
  }

  export interface RDNSResponse {
    ip_address: string;
    rdns_hostname?: string;
  }

  export interface UpdateRDNSRequest {
    hostname: string;
  }

  export const rdnsApi = {
    listIPs: (vmId: string) => apiClient.get<IPAddressRecord[]>(`/customer/vms/${vmId}/ips`),
    getRDNS: (vmId: string, ipId: string) => apiClient.get<RDNSResponse>(`/customer/vms/${vmId}/ips/${ipId}/rdns`),
    updateRDNS: (vmId: string, ipId: string, hostname: string) => apiClient.put(`/customer/vms/${vmId}/ips/${ipId}/rdns`, { hostname }),
    deleteRDNS: (vmId: string, ipId: string) => apiClient.delete(`/customer/vms/${vmId}/ips/${ipId}/rdns`),
  };
  ```

##### UI Components
- [x] Create `VMRDNSTab` component (`webui/customer/components/vm/VMRDNSTab.tsx`)
- [x] Display IP addresses with PTR records
- [x] Add edit PTR record functionality with validation
- [x] Add delete PTR record functionality
- [x] Integrate into VM detail page as "RDNS" tab

---

### Customer Notification Preferences UI

**Status:** ✅ COMPLETE

**Current State:**
- ✅ Backend API: `GET/PUT /customer/notifications/preferences`, `GET /customer/notifications/events`
- ✅ Backend handler: `internal/controller/api/customer/notifications.go`
- ✅ api-client methods in `webui/customer/lib/api-client.ts`
- ✅ UI component: `webui/customer/components/settings/NotificationsTab.tsx`
- ✅ Integration in Settings page

#### Implementation Tasks

##### Backend API Client
- [x] Add notification API to `webui/customer/lib/api-client.ts`
  ```typescript
  export interface NotificationPreferences {
    id: string;
    email_enabled: boolean;
    telegram_enabled: boolean;
    events: string[];
    created_at: string;
    updated_at: string;
  }

  export interface UpdateNotificationPreferencesRequest {
    email_enabled?: boolean;
    telegram_enabled?: boolean;
    events?: string[];
  }

  export const notificationApi = {
    getPreferences: () => apiClient.get<NotificationPreferences>('/customer/notifications/preferences'),
    updatePreferences: (prefs: UpdateNotificationPreferencesRequest) => apiClient.put('/customer/notifications/preferences', prefs),
    getEventTypes: () => apiClient.get<{ events: string[] }>('/customer/notifications/events/types'),
  };
  ```

##### UI Components
- [x] Add "Notifications" tab to Settings page (`webui/customer/app/settings/page.tsx`)
- [x] Create `NotificationsTab` component (`webui/customer/components/settings/NotificationsTab.tsx`)
- [x] Add toggle switches for each notification type
- [x] Display channel options (email, telegram)
- [x] Add event type toggles

---

## Summary: Backend→Frontend Gaps

| Feature | Backend API | API Client | UI Page | Status |
|---------|-------------|------------|---------|--------|
| Admin Templates | ✅ | ✅ | ✅ | COMPLETE |
| Admin Backups | ✅ | ✅ | ✅ | COMPLETE |
| Admin Backup Schedules | ✅ | ✅ | ✅ | COMPLETE |
| Customer ISO Upload | ✅ | ✅ | ✅ | COMPLETE |
| Customer RDNS | ✅ | ✅ | ✅ | COMPLETE |
| Customer Notifications | ✅ | ✅ | ✅ | COMPLETE |
| Customer API Key IP Whitelist | ✅ | ✅ | ✅ | COMPLETE |

---

## Remaining Recommendations

- [ ] **SBOM verification** for supply chain security
- [x] **Cursor-based pagination** for `ListAllActiveBatch` - Already implemented as `ListAllActiveCursor`
- [ ] **More integration test coverage** for error paths

---

## Pagination Implementation Note

- [x] Replace offset-based pagination with cursor-based pagination in `ListAllActiveBatch`

### Current Implementation

The cursor-based pagination method `ListAllActiveCursor` is already implemented:

```go
// ListAllActiveCursor returns a batch of active VMs using cursor-based pagination.
// This is more efficient than offset-based pagination for large datasets.
// Pass afterID="" for the first batch, then pass the last VM's ID from the previous batch.
// Returns an empty slice when no more results are available.
func (r *VMRepository) ListAllActiveCursor(ctx context.Context, afterID string, limit int) ([]models.VM, error) {
    var q string
    var args []any

    if afterID == "" {
        q = `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND node_id IS NOT NULL ORDER BY id LIMIT $1`
        args = []any{limit}
    } else {
        q = `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND node_id IS NOT NULL AND id > $1 ORDER BY id LIMIT $2`
        args = []any{afterID, limit}
    }
    // ...
}
```

**Benefits:**
- O(1) performance regardless of position
- Consistent results even if rows inserted/deleted during iteration