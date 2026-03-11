# Phase 5: Web UIs — Implementation Plan

**Created:** 2026-03-11
**Status:** Completed (19/19 tasks)
**Session:** ses_32782a8a3ffe50jpztvXn3PBxL
**Reference:** `docs/VIRTUESTACK_KICKSTART_V2.md` Section 19 (Implementation Phases)

---

## Phase 5 Overview

Build the Admin and Customer Web UIs using Next.js 16, React 19, TypeScript 5.5+, shadcn/ui, and Tailwind CSS. Includes NoVNC console integration, xterm.js serial console, resource monitoring graphs, and full responsive design.

---

## Phase 5 Tasks

### Phase 5.1: Project Scaffolding
**Goal:** Set up Next.js projects for Admin and Customer Web UIs
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 6 (Component 3: Web UIs)

- [x] **5.1.1**: `webui/admin/` — Admin WebUI Next.js project setup
  - Next.js 16 with App Router
  - TypeScript 5.5+ with strict mode
  - Tailwind CSS + shadcn/ui configuration
  - TanStack Query (React Query) setup
  - Project structure: `app/`, `components/`, `hooks/`, `lib/`, `types/`
  
- [x] **5.1.2**: `webui/customer/` — Customer WebUI Next.js project setup
  - Same configuration as Admin WebUI
  - Shared components package (optional symlink or npm workspace)
  - Different theme/styling for customer-facing UI

### Phase 5.2: Authentication UI
**Goal:** Implement login, 2FA, and session management
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 10 (Security Architecture)

- [x] **5.2.1**: `webui/admin/app/login/` — Admin login page
  - Email/password form with validation
  - TOTP 2FA verification step
  - HTTP-only cookie handling for refresh token
  - JWT access token storage (memory only, not localStorage)
  
- [x] **5.2.2**: `webui/customer/app/login/` — Customer login page
  - Same as admin but without requiring 2FA
  - Optional 2FA if enabled on account
  - Session management with TanStack Query

### Phase 5.3: VM Management UI (Customer)
**Goal:** Customer VM list, details, and control actions
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 12 (Customer API)

- [x] **5.3.1**: `webui/customer/app/vms/` — VM list page
  - Table with pagination, sorting, filtering
  - Status badges (running, stopped, error, etc.)
  - Quick actions (start, stop, restart buttons)
  
- [x] **5.3.2**: `webui/customer/app/vms/[id]/` — VM detail page
  - Resource cards (CPU, RAM, Disk, Bandwidth usage)
  - Control panel (start, stop, force-stop, restart)
  - Reinstall button with template selection modal
  - Backup/snapshot management sub-pages
  - rDNS configuration
  - ISO management

### Phase 5.4: Console Integration
**Goal:** NoVNC and serial console access
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 17 (Monitoring & Observability)

- [x] **5.4.1**: `webui/customer/components/novnc-console/` — NoVNC integration
  - `@novnc/novnc` library integration
  - WebSocket connection to Controller proxy
  - One-time token authentication
  - Full-screen mode, reconnect handling
  
- [x] **5.4.2**: `webui/customer/components/serial-console/` — xterm.js serial console
  - `xterm` + `xterm-addon-fit` integration
  - WebSocket for serial data streaming
  - Terminal resize handling

### Phase 5.5: Resource Monitoring
**Goal:** Real-time charts and metrics visualization
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 17 (Monitoring)

- [x] **5.5.1**: `webui/customer/components/charts/` — Resource charts
  - CPU usage chart (uPlot for streaming data)
  - Memory usage chart
  - Network I/O charts (bandwidth usage)
  - Disk I/O charts
  - Time range selection (1h, 24h, 7d)

### Phase 5.6: Admin Dashboard
**Goal:** Admin-specific management interfaces
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 12 (Admin API)

- [x] **5.6.1**: `webui/admin/app/dashboard/` — Admin dashboard
  - System overview cards (total VMs, nodes, customers)
  - Recent activity feed
  - Alert notifications
  
- [x] **5.6.2**: `webui/admin/app/nodes/` — Node management
  - Node list with status, capacity
  - Node detail with VM list
  - Drain/failover actions (with TOTP re-auth)
  
- [x] **5.6.3**: `webui/admin/app/plans/` — Plan management
  - CRUD for VM plans (CPU, RAM, Disk, Bandwidth)
  
- [x] **5.6.4**: `webui/admin/app/ip-sets/` — IP set management
  - IP pool overview
  - Bulk import interface
  
- [x] **5.6.5**: `webui/admin/app/customers/` — Customer management
  - Customer list with VM count
  - Customer detail with all their VMs
  - Suspend/unsuspend actions
  
- [x] **5.6.6**: `webui/admin/app/audit-logs/` — Audit log viewer
  - Filterable log table
  - Export to CSV

### Phase 5.7: Customer Self-Service
**Goal:** Additional customer features
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 12 (Customer API)

- [x] **5.7.1**: `webui/customer/app/settings/` — Customer settings
  - Profile management
  - 2FA enable/disable with QR code
  - API key management
  - Webhook management
  
- [x] **5.7.2**: `webui/customer/components/file-upload/` — ISO upload (tus)
  - `tus-js-client` for resumable uploads
  - Progress tracking
  - Drag-and-drop interface

### Phase 5.8: Theming & Polish
**Goal:** Responsive design and theming
**Reference:** `VIRTUESTACK_KICKSTART_V2.md` Section 20 (Quality Gates)

- [x] **5.8.1**: `webui/*/app/layout.tsx` — Responsive layout
  - Mobile navigation drawer
  - Tablet-optimized tables
  - Touch-friendly controls
  
- [x] **5.8.2**: `webui/*/styles/theme.ts` — Dark/light theme
  - CSS variable-based theming ✅
  - Theme toggle component ✅
  - System preference detection ✅
  - shadcn/ui theme configuration ✅

---

## Technical Stack

| Component | Technology | Version |
|-----------|-----------|---------|
| Framework | Next.js | 16+ |
| Language | TypeScript | 5.5+ |
| UI Library | React | 19+ |
| Styling | Tailwind CSS | 3.x |
| Components | shadcn/ui | Latest |
| Data Fetching | TanStack Query | 5.x |
| Forms | React Hook Form + Zod | Latest |
| Charts | uPlot + Apache ECharts | Latest |
| Console | @novnc/novnc + xterm.js | Latest |
| Upload | tus-js-client | Latest |

---

## Shared Components Strategy

Both Admin and Customer WebUIs should share:
- UI primitives (Button, Input, Modal, etc.) via shadcn/ui
- API client utilities (`lib/api.ts`)
- Type definitions (`types/api.ts`)
- React hooks for data fetching (`hooks/useVMs.ts`, etc.)

Consider using:
- npm workspaces
- Shared package in `webui/shared/`
- Or symlink common code

---

## API Integration Pattern

```typescript
// hooks/useVMs.ts - TanStack Query example
export function useVMs() {
  return useQuery({
    queryKey: ['vms'],
    queryFn: async () => {
      const res = await fetch('/api/v1/customer/vms', {
        headers: { 'Authorization': `Bearer ${getAccessToken()}` }
      });
      if (!res.ok) throw new Error('Failed to fetch VMs');
      return res.json();
    }
  });
}
```

---

## Session History

| Session ID | Date | Work Completed |
|------------|------|----------------|
| ses_32782a8a3ffe50jpztvXn3PBxL | 2026-03-11 | Completed Phase 4. Marked all Phase 4 tasks complete. Created Phase 5 plan. |
| Current | 2026-03-11 | Completed Phase 5. Created theme.ts configuration files for both Admin and Customer WebUIs. All 19 tasks now complete. |

---

## Next Steps After Phase 5

Phase 6: Integration & Polish
- WHMCS provisioning module
- Notification system (email, Telegram)
- Webhook delivery
- Docker Compose production config
- End-to-end testing
- Documentation
