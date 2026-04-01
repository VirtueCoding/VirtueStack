# Blesta Billing Integration

## Overview

The Blesta module provides server provisioning integration between Blesta (billing/client management) and VirtueStack (VM infrastructure). Blesta handles invoicing, payments, and customer management while VirtueStack handles VM provisioning via the neutral Provisioning API.

```
┌──────────┐    Provisioning API     ┌──────────────┐    gRPC/mTLS     ┌────────────┐
│  Blesta  │ ──────────────────────► │  Controller  │ ───────────────► │ Node Agent │
│ (Billing)│ ◄────── Webhooks ────── │  (VirtueStack)│                  │  (KVM)     │
└──────────┘                         └──────────────┘                  └────────────┘

Communication is one-way: Blesta → VirtueStack Provisioning API.
VirtueStack sends async task completion webhooks back to Blesta.
```

## Module Files

| File | Purpose |
|------|---------|
| `modules/blesta/virtuestack/virtuestack.php` | Main module — server provisioning (create, suspend, unsuspend, terminate, resize) |
| `modules/blesta/virtuestack/config.json` | Module metadata (name, version, authors, description) |
| `modules/blesta/virtuestack/language/en_us/virtuestack.php` | English language strings |
| `modules/blesta/virtuestack/lib/ApiClient.php` | HTTP client for VirtueStack Provisioning API |
| `modules/blesta/virtuestack/lib/VirtueStackHelper.php` | URL builder, SSO token helpers |
| `modules/blesta/virtuestack/views/default/tab_client_service.pdt` | Client area service tab |
| `modules/blesta/virtuestack/views/default/tab_client_console.pdt` | Client area console tab (iframe) |
| `modules/blesta/virtuestack/views/default/tab_admin_manage.pdt` | Admin service management tab |
| `modules/blesta/virtuestack/views/default/tab_admin_add.pdt` | Admin add-service form |
| `modules/blesta/virtuestack/webhook.php` | Webhook receiver for async task completion |

## Installation

### 1. Copy Module Files

```bash
cp -r modules/blesta/virtuestack/ /path/to/blesta/components/modules/servers/virtuestack/
```

### 2. Install Module in Blesta Admin

1. Navigate to **Settings → Modules → Available**
2. Find **VirtueStack** and click **Install**
3. The module will appear under **Settings → Modules → Installed**

### 3. Add a Server

1. Go to **Settings → Modules → Installed → VirtueStack → Add Server**
2. Configure:
   - **Server Name:** Display name (e.g., "VirtueStack Production")
   - **Hostname:** VirtueStack Controller URL (e.g., `https://panel.example.com`)
   - **API Key:** A Provisioning API key from VirtueStack admin panel

### 4. Create a Package

1. Go to **Packages → Create Package**
2. Select **VirtueStack** as the module
3. Configure:
   - **Plan ID:** The VirtueStack plan UUID (from admin panel → Plans)
   - **Template ID:** The OS template UUID (from admin panel → Templates)
   - **Location ID:** The location UUID (optional, for node selection)
4. Set pricing (Blesta handles billing; VirtueStack just provisions)

### 5. Configure Webhook (Optional)

For async task completion notifications:

1. In VirtueStack admin, set the webhook URL:
   ```
   https://blesta.example.com/components/modules/servers/virtuestack/webhook.php
   ```
2. The webhook receives `task.completed` and `task.failed` events
3. On `task.completed`, the module updates the Blesta service status to active

## Configuration Reference

| Setting | Location | Description |
|---------|----------|-------------|
| API URL | Blesta server config | VirtueStack Controller URL |
| API Key | Blesta server config | Provisioning API key (X-API-Key header) |
| Plan ID | Blesta package config | VirtueStack plan UUID for VM sizing |
| Template ID | Blesta package config | OS template UUID |
| Location ID | Blesta package config | Data center location (optional) |

## Go-Side Adapter

The Go-side Blesta adapter (`internal/controller/billing/blesta/adapter.go`) is a **no-op stub**. It satisfies the `BillingProvider` interface so the billing registry can reference `blesta` as a valid provider name, but all operational methods (except `Name()`, `ValidateConfig()`, and `GetUserBillingStatus()`) return `nil` or no-op results.

This is by design — Blesta manages its own invoicing, payments, and customer accounts. VirtueStack only needs to provision/deprovision VMs when Blesta calls the Provisioning API.

| Method | Behavior |
|--------|----------|
| `Name()` | Returns `"blesta"` |
| `ValidateConfig()` | Checks `BLESTA_API_URL` + `BLESTA_API_KEY` are set |
| `OnVMCreated()` | No-op (returns nil) |
| `OnVMDeleted()` | No-op (returns nil) |
| `OnVMResized()` | No-op (returns nil) |
| `GetUserBillingStatus()` | Returns `"active"` (Blesta manages externally) |

Environment variables for the adapter:

| Variable | Required | Description |
|----------|----------|-------------|
| `BILLING_BLESTA_ENABLED` | No | Enable Blesta provider (default: `false`) |
| `BILLING_BLESTA_PRIMARY` | No | Set as primary for new registrations (default: `false`) |
| `BLESTA_API_URL` | When enabled | Blesta API base URL |
| `BLESTA_API_KEY` | When enabled | Blesta API authentication key |

## Key Module Methods

| Blesta Method | VirtueStack API Call | Purpose |
|---------------|---------------------|---------|
| `addService()` | `POST /api/v1/provisioning/vms` | Create VM (async, returns task_id) |
| `suspendService()` | `POST /api/v1/provisioning/vms/{id}/suspend` | Suspend VM |
| `unsuspendService()` | `POST /api/v1/provisioning/vms/{id}/unsuspend` | Unsuspend VM |
| `cancelService()` | `DELETE /api/v1/provisioning/vms/{id}` | Delete VM |
| `changeServicePackage()` | `POST /api/v1/provisioning/vms/{id}/resize` | Resize VM |

## Webhook Events

The webhook receiver (`webhook.php`) handles these events from VirtueStack:

| Event | Action |
|-------|--------|
| `task.completed` (type: `vm.create`) | Updates Blesta service status to active, stores VM ID |
| `task.failed` (type: `vm.create`) | Logs error, service remains pending |
| `task.completed` (type: `vm.delete`) | Confirms deletion |
| `vm.suspended` | Syncs suspension status |

## Troubleshooting

### Module Not Appearing in Blesta

- Verify files are in `components/modules/servers/virtuestack/`
- Check `config.json` is valid JSON
- Clear Blesta cache: **Settings → System → Clear Cache**

### API Connection Failures

- Verify the Controller URL is accessible from the Blesta server
- Check the Provisioning API key is valid and not expired
- Ensure the Blesta server IP is in the `PROVISIONING_ALLOWED_IPS` list (if configured)
- Check Controller logs for `401 Unauthorized` or `403 Forbidden` errors

### VM Creation Stuck in Pending

- Check VirtueStack admin → Tasks for the task status
- Verify the webhook URL is configured and accessible
- If no webhook, the Blesta cron job will poll `GET /api/v1/provisioning/tasks/{task_id}`

### SSO Not Working

- Verify SSO tokens are enabled in VirtueStack settings
- Check the customer portal URL is correctly configured
- SSO flow: `POST /provisioning/sso-tokens` → redirect to `/customer/auth/sso-exchange?token={opaque}`

## Customer Assignment

Customers provisioned through Blesta have `billing_provider = 'blesta'` set automatically:

```sql
SELECT id, email, billing_provider FROM customers WHERE billing_provider = 'blesta';
```

When `BILLING_BLESTA_PRIMARY=true`, new self-registered customers are also assigned to Blesta.
