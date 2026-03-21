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