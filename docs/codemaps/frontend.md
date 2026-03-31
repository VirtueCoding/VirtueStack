<!-- Generated: 2026-03-28 | Files scanned: 90+ TSX files | Token estimate: ~1000 -->

# Frontend Architecture

## Admin Portal

**Directory:** `webui/admin/` | **Dev Port:** 3000 | **Prod Port:** 3001

### Page Tree

```
app/
├── layout.tsx          # Root layout, providers
├── page.tsx            # Redirect to /dashboard
├── providers.tsx       # TanStack Query, theme
├── login/
│   └── page.tsx        # JWT auth + 2FA
├── dashboard/
│   └── page.tsx        # Node overview, alerts
├── audit-logs/
│   └── page.tsx        # Audit trail viewer
├── backup-schedules/
│   └── page.tsx        # Customer backup schedules
├── customers/
│   └── page.tsx        # Customer management
├── failover-requests/
│   └── page.tsx        # Failover request viewer
├── ip-sets/
│   └── page.tsx        # IP pool management
├── nodes/
│   └── page.tsx        # Node management, drain, failover
├── plans/
│   └── page.tsx        # Plan management with resource limits
├── provisioning-keys/
│   └── page.tsx        # WHMCS API key lifecycle
├── settings/
│   ├── page.tsx        # System settings
│   └── permissions/
│       └── page.tsx    # Permission management (super_admin)
├── storage-backends/
│   └── page.tsx        # Storage backend registry + health
├── templates/
│   └── page.tsx        # Template management, build from ISO, distribute
└── vms/
    └── page.tsx        # VM management
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
│   ├── CustomerCreateDialog.tsx # Create customer modal
│   └── CustomerEditDialog.tsx   # Edit name/status modal
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
│   ├── PlanCreateDialog.tsx     # Create plan
│   └── PlanEditDialog.tsx       # Edit plan
├── storage-backends/            # Storage backend management components
├── templates/
│   └── TemplateEditDialog.tsx   # Edit template
└── vms/
    ├── VMCreateDialog.tsx       # Create VM manually
    └── VMEditDialog.tsx         # Edit VM properties
```

### Auth & Permissions

```
contexts/
└── PermissionContext.tsx    # Admin permission context provider

hooks/
└── usePermissions.ts       # Permission check hook

lib/
├── api-client.ts           # Centralized API client
├── auth-context.tsx        # Auth state
├── navigation.ts           # Route definitions
├── require-auth.tsx        # Auth guard HOC
├── status-badge.ts         # Status badge utilities
└── utils.ts                # General utilities
```

## Customer Portal

**Directory:** `webui/customer/` | **Dev Port:** 3001 | **Prod Port:** 3002

### Page Tree

```
app/
├── layout.tsx          # Root layout
├── page.tsx            # Redirect to /vms
├── providers.tsx       # TanStack Query, theme
├── login/
│   └── page.tsx        # JWT auth
├── forgot-password/
│   └── page.tsx        # Password reset request
├── reset-password/
│   └── page.tsx        # Password reset form
├── settings/
│   └── page.tsx        # Profile, 2FA, API keys, webhooks, notifications
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
│   └── resource-charts.tsx  # Recharts-based resource charts
├── novnc-console/
│   └── vnc-console.tsx      # noVNC WebSocket client
├── serial-console/
│   └── serial-console.tsx   # xterm.js WebSocket terminal
├── file-upload/
│   └── iso-upload.tsx       # ISO upload component
├── settings/
│   ├── ProfileTab.tsx       # Profile management
│   ├── SecurityTab.tsx      # 2FA management
│   ├── ApiKeysTab.tsx       # API key management
│   ├── WebhooksTab.tsx      # Webhook configuration
│   └── NotificationsTab.tsx # Notification preferences
└── vm/
    ├── VMControls.tsx       # Power control buttons
    ├── VMConsoleTab.tsx      # VNC/serial console
    ├── VMBackupsTab.tsx      # Backup management
    ├── VMSnapshotsTab.tsx    # Snapshot management
    ├── VMISOTab.tsx          # ISO upload/attach/detach
    ├── VMRDNSTab.tsx         # Reverse DNS management
    └── VMSettingsTab.tsx     # VM settings
```

## State Management

```
TanStack Query (React Query) — sole state management
├── Server state: VMs, nodes, plans, customers
├── Cache invalidation on mutations
└── Optimistic updates
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
| UI Library | React 19 |
| Language | TypeScript 5.7 |
| Styling | Tailwind CSS |
| Components | shadcn/ui (Radix primitives) |
| State | TanStack Query 5.64 |
| Forms | react-hook-form 7 + Zod 3.24 |
| Charts | Recharts 3.8 (customer portal) |
| Console | noVNC 1.5 + xterm.js 6.0 |