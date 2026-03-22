<!-- Generated: 2026-03-22 | Files scanned: 82 TSX files | Token estimate: ~950 -->

# Frontend Architecture

## Admin Portal

**Directory:** `webui/admin/` | **Port:** 3000

### Page Tree

```
app/
├── layout.tsx          # Root layout, providers
├── page.tsx            # Redirect to /dashboard
├── providers.tsx       # TanStack Query, theme
├── login/
│   └── page.tsx        # JWT auth + 2FA
└── dashboard/
    ├── layout.tsx      # Sidebar, auth guard
    └── page.tsx        # Node overview, alerts
        ├── audit-logs/page.tsx
        ├── backups/page.tsx        # All backups management
        ├── backup-schedules/page.tsx # Admin backup campaigns
        ├── customers/page.tsx
        ├── ip-sets/page.tsx
        ├── nodes/page.tsx
        ├── plans/page.tsx
        ├── settings/
        │   ├── page.tsx            # System settings
        │   └── permissions/page.tsx # Permission management (super_admin) (NEW)
        ├── templates/page.tsx
        └── vms/page.tsx
```

### Key Components

```
components/
├── sidebar.tsx         # Navigation
├── mobile-nav.tsx      # Responsive nav
├── theme-toggle.tsx    # Dark/light mode
├── ui/                 # shadcn/ui primitives
│   ├── button.tsx, input.tsx, table.tsx, textarea.tsx
│   ├── dialog.tsx, sheet.tsx, dropdown-menu.tsx
│   └── toast.tsx, toaster.tsx, badge.tsx, checkbox.tsx
├── backups/
│   ├── BackupList.tsx           # Backup table with filters
│   ├── BackupDetailModal.tsx    # Backup detail view
│   ├── RestoreConfirmModal.tsx  # Restore confirmation
│   ├── AdminScheduleList.tsx    # Admin schedules table
│   └── CreateScheduleModal.tsx  # Create/edit schedule
├── customers/
│   ├── CustomerCreateDialog.tsx # Create customer (NEW)
│   └── CustomerEditDialog.tsx   # Edit name/status (NEW)
├── ip-sets/
│   ├── IPSetList.tsx            # IP set table
│   ├── IPSetCreateDialog.tsx    # Create IP set
│   ├── IPSetEditDialog.tsx      # Edit IP set
│   ├── IPSetDetailDialog.tsx    # View IP set details
│   └── IPSetImportDialog.tsx    # Import IPs
├── nodes/
│   ├── NodeCreateDialog.tsx     # Register new node
│   └── NodeEditDialog.tsx       # Edit node config
├── plans/
│   ├── PlanList.tsx             # Plans table
│   ├── PlanCreateDialog.tsx     # Create plan (NEW)
│   └── PlanEditDialog.tsx       # Edit plan
├── templates/
│   └── TemplateEditDialog.tsx   # Edit template
└── vms/
    ├── VMCreateDialog.tsx       # Create VM manually
    └── VMEditDialog.tsx         # Edit VM properties
```

## Customer Portal

**Directory:** `webui/customer/` | **Port:** 3001

### Page Tree

```
app/
├── layout.tsx          # Root layout
├── page.tsx            # Redirect to /vms
├── providers.tsx       # TanStack Query, theme
├── login/
│   └── page.tsx        # JWT auth
├── settings/
│   └── page.tsx        # Profile, 2FA, API keys, webhooks
└── vms/
    ├── layout.tsx      # VM list layout
    ├── page.tsx        # VM list
    └── [id]/
        └── page.tsx    # VM detail, controls, console, metrics
```

### Key Components

```
components/
├── sidebar.tsx
├── mobile-nav.tsx
├── theme-toggle.tsx
├── ui/                 # shadcn/ui primitives
├── charts/
│   └── resource-charts.tsx  # uPlot + ECharts
├── novnc-console/
│   └── vnc-console.tsx      # noVNC WebSocket
├── serial-console/
│   └── serial-console.tsx   # xterm.js WebSocket
├── file-upload/
│   └── iso-upload.tsx       # tus protocol
├── settings/
│   ├── ProfileTab.tsx
│   ├── SecurityTab.tsx
│   ├── ApiKeysTab.tsx
│   └── WebhooksTab.tsx
└── vm/
    ├── VMControls.tsx
    ├── VMConsoleTab.tsx
    ├── VMBackupsTab.tsx
    ├── VMSnapshotsTab.tsx
    └── VMSettingsTab.tsx
```

## State Management

```
TanStack Query (React Query)
├── Server state: VMs, nodes, plans, customers
├── Cache invalidation on mutations
└── Optimistic updates

Zustand (if used)
├── UI state: sidebar collapse, theme
└── Local-only state
```

## API Client Pattern

**Files:** `webui/*/lib/api-client.ts`

```typescript
// Authenticated fetch wrapper
const api = {
  get: (path) => fetch(`/api/v1/${path}`, { headers: authHeaders }),
  post: (path, body) => fetch(`/api/v1/${path}`, { method: 'POST', body, headers }),
  // ... put, delete
}

// TanStack Query hooks
useQuery({ queryKey: ['vms'], queryFn: () => api.get('customer/vms') })
useMutation({ mutationFn: (id) => api.post(`customer/vms/${id}/start`) })
```

## WebSocket Connections

| Endpoint | Purpose | Component |
|----------|---------|-----------|
| `/ws/vnc/:vmId` | VNC console | `vnc-console.tsx` |
| `/ws/serial/:vmId` | Serial console | `serial-console.tsx` |

## Tech Stack

| Layer | Technology |
|-------|------------|
| Framework | Next.js 16+ (App Router) |
| UI Library | React 19+ |
| Language | TypeScript 5.5+ |
| Styling | Tailwind CSS |
| Components | shadcn/ui |
| State | TanStack Query + Zustand |
| Charts | uPlot + Apache ECharts |
| Console | noVNC + xterm.js |

## New Components (Since Last Update)

| Component | Location | Purpose |
|-----------|----------|---------|
| Permissions Page | `app/settings/permissions/` | Super-admin permission management |
| CustomerCreateDialog | `components/customers/` | Create customer modal |
| CustomerEditDialog | `components/customers/` | Edit customer modal |
| PlanCreateDialog | `components/plans/` | Create plan modal |