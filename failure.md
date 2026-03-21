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
1. [ ] Create migration `000037_admin_backup_schedules.sql`
2. [ ] Remove `type` column from `backups` table
3. [ ] Add `source` column to `backups` table
4. [ ] Remove `Type` field from `models.Backup`
5. [ ] Remove `type` from all backup repository queries
6. [ ] Update `BackupCreatePayload` to use `Source` instead of `BackupType`
7. [ ] Update backup task handlers to set `source` correctly
8. [ ] Update frontend `Backup` interface to remove `type`

**Phase 2: Admin Schedule Backend** (Day 2)
1. [ ] Add `AdminBackupSchedule` model
2. [ ] Add repository methods for admin schedules
3. [ ] Add API endpoints for admin schedules
4. [ ] Update `backups` endpoint to include source filter
5. [ ] Create admin schedule execution worker

**Phase 3: Core UI Components** (Day 3-4)
1. [ ] Create `BackupList` component with filters
2. [ ] Create `BackupDetailModal` component
3. [ ] Create `RestoreConfirmModal` component
4. [ ] Add backup API methods to api-client.ts
5. [ ] Create `/backups` page

**Phase 4: Admin Schedule UI** (Day 5-6)
1. [ ] Create `AdminScheduleList` component
2. [ ] Create `CreateScheduleModal` with target selector
3. [ ] Create `ScheduleDetailPanel` component
4. [ ] Create `/backup-schedules` page
5. [ ] Add schedule API methods to api-client.ts

**Phase 5: Testing** (Day 7)
1. [ ] Add integration tests for admin schedule execution
2. [ ] Add E2E tests for admin backup list page
3. [ ] Add E2E tests for admin schedule creation
4. [ ] Manual testing of all flows

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

**Status:** Backend partially implemented, UI missing IP whitelist. Frontend missing both IP whitelist and expiration fields.

**Current State:**
- ✅ Backend API exists: `GET/POST/DELETE /customer/api-keys`, `POST /customer/api-keys/:id/rotate`
- ✅ Backend has `expires_at` support (model, DB, API)
- ✅ Frontend has basic API key management (`ApiKeysTab.tsx`)
- ❌ Backend missing `allowed_ips` column in `customer_api_keys` table
- ❌ Backend missing `AllowedIPs` field in `CustomerAPIKey` model
- ❌ Backend missing IP whitelist validation in auth middleware
- ❌ Frontend missing IP whitelist input in create dialog
- ❌ Frontend missing expiration date input in create dialog
- ❌ Frontend not displaying allowed IPs or expiration in key list

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
- [ ] Add `AllowedIPs []string` field to `CustomerAPIKey` struct (line 75-87)
  ```go
  type CustomerAPIKey struct {
      // ... existing fields ...
      AllowedIPs   []string   `json:"allowed_ips,omitempty" db:"allowed_ips"`
      // ...
  }
  ```
- [ ] Add `IsAllowedIP(ip string) bool` method to `CustomerAPIKey` (copy from `ProvisioningKey`)

##### 2.2 API Handler Updates (`internal/controller/api/customer/apikeys.go`)
- [ ] Add `AllowedIPs []string` to `CreateAPIKeyRequest` struct (line 22-26)
  ```go
  type CreateAPIKeyRequest struct {
      Name        string   `json:"name" validate:"required,max=100"`
      Permissions []string `json:"permissions" validate:"required,min=1,dive,max=100"`
      AllowedIPs  []string `json:"allowed_ips,omitempty" validate:"max=50,dive,ip|cidr"`
      ExpiresAt   *string  `json:"expires_at,omitempty"`
  }
  ```
- [ ] Add `AllowedIPs []string` to `APIKeyResponse` struct (line 34-43)
- [ ] Update `CreateAPIKey` handler to pass `AllowedIPs` to model (line 144-151)
- [ ] Update `ListAPIKeys` handler to include `AllowedIPs` in response (line 75-96)

##### 2.3 Repository Updates (`internal/controller/repository/customer_api_key_repo.go`)
- [ ] Add `allowed_ips` to INSERT query in `Create()` method
- [ ] Add `allowed_ips` to SELECT columns in `GetByIDAndCustomer()` and `GetByHash()`
- [ ] Add `allowed_ips` to `ListByCustomer()` query

##### 2.4 Auth Middleware Updates (`internal/controller/api/middleware/auth.go`)
- [ ] Update `CustomerAPIKeyValidator` to check IP whitelist
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
- [ ] Add `allowed_ips?: string[]` to `ApiKey` interface (line 460-469)
- [ ] Add `allowed_ips?: string[]` to `CreateApiKeyRequest` interface (line 483-487)

##### 3.2 UI Updates (`webui/customer/components/settings/ApiKeysTab.tsx`)
- [ ] Add `allowed_ips` to form schema (line 26-29)
  ```typescript
  const apiKeySchema = z.object({
    name: z.string().min(1, "Name is required"),
    permissions: z.array(z.string()).min(1, "At least one permission is required"),
    allowed_ips: z.array(z.string()).optional(),
    expires_at: z.string().optional(),
  });
  ```
- [ ] Add IP whitelist textarea input in Create dialog (after permissions section)
  - Placeholder: "192.168.1.1\n10.0.0.0/24\n2001:db8::/32"
  - Helper text: "One IP or CIDR per line. Leave empty to allow all IPs."
  - Validation: IPv4, IPv6, or CIDR notation
- [ ] Add expiration date input in Create dialog
  - Use date picker or datetime input
  - Optional field
- [ ] Display allowed IPs in key list (show count with expandable list)
- [ ] Display expiration date in key list (show "Expires: X" or "Never expires")

---

#### 4. File Structure

```
Files to modify:
├── migrations/
│   ├── 000037_customer_api_key_allowed_ips.up.sql    # NEW
│   └── 000037_customer_api_key_allowed_ips.down.sql  # NEW
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
1. [ ] Create migration for `allowed_ips` column
2. [ ] Update `CustomerAPIKey` model with `AllowedIPs` field and `IsAllowedIP()` method
3. [ ] Update `CreateAPIKeyRequest` and `APIKeyResponse` structs
4. [ ] Update repository queries
5. [ ] Update auth middleware to validate IP whitelist

**Phase 2: Frontend** (1-2 hours)
1. [ ] Update `ApiKey` and `CreateApiKeyRequest` interfaces
2. [ ] Add IP whitelist textarea to create dialog
3. [ ] Add expiration date input to create dialog
4. [ ] Display allowed IPs and expiration in key list
5. [ ] Test with various IP formats (IPv4, IPv6, CIDR)

---

#### 6. Testing Checklist

- [ ] Create API key with no IP restriction (empty allowed_ips)
- [ ] Create API key with IPv4 address (e.g., `192.168.1.100`)
- [ ] Create API key with IPv6 address (e.g., `2001:db8::1`)
- [ ] Create API key with CIDR notation (e.g., `10.0.0.0/24`)
- [ ] Create API key with mixed IPv4/IPv6/CIDR
- [ ] Verify API key rejected from non-whitelisted IP
- [ ] Verify API key accepted from whitelisted IP
- [ ] Verify expiration date display and enforcement
- [ ] Test rotate and delete operations still work

---

### Admin Templates Management UI

**Status:** Backend API complete, frontend API client has methods, but NO page exists.

**Current State:**
- ✅ Backend API exists: `GET/POST/PUT/DELETE /admin/templates`, `POST /admin/templates/:id/import`
- ✅ Admin api-client.ts has methods: `getTemplates()`, `createTemplate()`, `updateTemplate()`, `deleteTemplate()`, `importTemplate()`
- ❌ No Admin WebUI page at `/templates`
- ❌ No navigation item in admin sidebar
- ❌ No components for template management

#### Implementation Tasks

- [ ] Add "Templates" to admin navigation (`webui/admin/lib/navigation.ts`)
- [ ] Create `/templates` page (`webui/admin/app/templates/page.tsx`)
- [ ] Create `TemplateList` component with table showing: name, os_type, size, status
- [ ] Create `TemplateCreateDialog` component
- [ ] Create `TemplateEditDialog` component
- [ ] Add import template functionality (calls `importTemplate` API)
- [ ] Add delete confirmation dialog

---

### Customer ISO Upload UI

**Status:** Backend API exists, component exists, but NOT integrated into VM detail page.

**Current State:**
- ✅ Backend API: `GET/POST/DELETE /customer/vms/:id/iso`, `POST /customer/vms/:id/iso/:isoId/attach|detach`
- ✅ Customer api-client.ts has `isoApi` with all methods
- ✅ Component exists: `webui/customer/components/file-upload/iso-upload.tsx`
- ❌ Component NOT imported or used anywhere
- ❌ No ISO tab in VM detail page

#### Implementation Tasks

- [ ] Add "ISO" tab to VM detail page tabs (`webui/customer/app/vms/[id]/page.tsx`)
- [ ] Create `VMISOTab` component that uses `ISOUpload` component
- [ ] Add ISO list view showing uploaded ISOs
- [ ] Add attach/detach ISO functionality
- [ ] Add delete ISO functionality
- [ ] Update tab count from 7 to 8 tabs

---

### Customer RDNS Management UI

**Status:** Backend API exists, but NO frontend implementation at all.

**Current State:**
- ✅ Backend API: `GET /customer/vms/:id/ips`, `GET/PUT/DELETE /customer/vms/:id/ips/:ipId/rdns`
- ❌ No api-client methods in `webui/customer/lib/api-client.ts`
- ❌ No UI components
- ❌ No integration in VM detail page

#### Implementation Tasks

##### Backend API Client
- [ ] Add `rdnsApi` to `webui/customer/lib/api-client.ts`
  ```typescript
  export interface RDNSRecord {
    id: string;
    ip_address: string;
    ip_version: number;
    ptr_record: string | null;
  }

  export interface UpdateRDNSRequest {
    ptr_record: string;
  }

  export const rdnsApi = {
    listIPs: (vmId: string) => apiClient.get<RDNSRecord[]>(`/customer/vms/${vmId}/ips`),
    getRDNS: (vmId: string, ipId: string) => apiClient.get<RDNSRecord>(`/customer/vms/${vmId}/ips/${ipId}/rdns`),
    updateRDNS: (vmId: string, ipId: string, ptr: string) => apiClient.put(`/customer/vms/${vmId}/ips/${ipId}/rdns`, { ptr_record: ptr }),
    deleteRDNS: (vmId: string, ipId: string) => apiClient.delete(`/customer/vms/${vmId}/ips/${ipId}/rdns`),
  };
  ```

##### UI Components
- [ ] Create `VMRDNSTab` component (`webui/customer/components/vm/VMRDNSTab.tsx`)
- [ ] Display IP addresses with PTR records
- [ ] Add edit PTR record functionality with validation
- [ ] Add delete PTR record functionality
- [ ] Integrate into VM detail page as "RDNS" tab

---

### Customer Notification Preferences UI

**Status:** Backend API exists, but NO frontend implementation at all.

**Current State:**
- ✅ Backend API: `GET/PUT /customer/notifications/preferences`, `GET /customer/notifications/events`
- ✅ Backend handler: `internal/controller/api/customer/notifications.go`
- ❌ No api-client methods
- ❌ No UI components
- ❌ No integration in Settings page

#### Implementation Tasks

##### Backend API Client
- [ ] Add notification API to `webui/customer/lib/api-client.ts`
  ```typescript
  export interface NotificationPreferences {
    email_on_backup_complete: boolean;
    email_on_backup_fail: boolean;
    email_on_vm_created: boolean;
    email_on_vm_deleted: boolean;
    email_on_bandwidth_threshold: boolean;
    // ... other preferences
  }

  export interface NotificationEvent {
    event_type: string;
    description: string;
    enabled: boolean;
  }

  export const notificationApi = {
    getPreferences: () => apiClient.get<NotificationPreferences>('/customer/notifications/preferences'),
    updatePreferences: (prefs: Partial<NotificationPreferences>) => apiClient.put('/customer/notifications/preferences', prefs),
    getEvents: () => apiClient.get<NotificationEvent[]>('/customer/notifications/events'),
  };
  ```

##### UI Components
- [ ] Add "Notifications" tab to Settings page (`webui/customer/app/settings/page.tsx`)
- [ ] Create `NotificationsTab` component (`webui/customer/components/settings/NotificationsTab.tsx`)
- [ ] Add toggle switches for each notification type
- [ ] Add email address display/edit
- [ ] Add bandwidth threshold configuration (if applicable)

---

## Summary: Backend→Frontend Gaps

| Feature | Backend API | API Client | UI Page | Priority |
|---------|-------------|------------|---------|----------|
| Admin Templates | ✅ | ✅ | ❌ | HIGH |
| Admin Backups | ✅ | ❌ | ❌ | HIGH (documented above) |
| Admin Backup Schedules | ✅ | ❌ | ❌ | HIGH (documented above) |
| Customer ISO Upload | ✅ | ✅ | ❌ | MEDIUM |
| Customer RDNS | ✅ | ❌ | ❌ | MEDIUM |
| Customer Notifications | ✅ | ❌ | ❌ | LOW |
| Customer API Key IP Whitelist | ✅ | ❌ | ❌ | MEDIUM (documented above) |

---

## Remaining Recommendations

- [ ] **SBOM verification** for supply chain security
- [ ] **Cursor-based pagination** for `ListAllActiveBatch` (see Pagination Implementation Note below)
- [ ] **More integration test coverage** for error paths

---

## Pagination Implementation Note

- [ ] Replace offset-based pagination with cursor-based pagination in `ListAllActiveBatch`

### Current Implementation (Offset-based)

```go
const q = `SELECT ... FROM vms WHERE ... ORDER BY id LIMIT $1 OFFSET $2`
```

**Issues:**
- [ ] Performance degrades as offset grows (database scans all previous rows)
- [ ] Consistency: rows may be missed or duplicated if data changes during iteration

### Recommended Implementation (Cursor-based)

```go
// Cursor-based - use last seen ID
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