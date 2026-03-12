# VirtueStack Usage Guide

This guide provides comprehensive instructions for using VirtueStack Phase 6, covering both the Admin Dashboard and Customer Portal interfaces.

> **Feature Status Legend:**
> - ✅ **Working** — Feature is implemented and verified end-to-end
> - ⚠️ **Planned** — Feature is designed but not yet fully implemented
> - 🔧 **Partial** — Backend exists but frontend integration is pending

---

## Table of Contents

1. [Admin Dashboard](#admin-dashboard)
   - [Overview](#admin-dashboard-overview)
   - [Node Management](#node-management)
   - [VM Management](#vm-management-admin)
   - [Plan Management](#plan-management)
   - [Template Management](#template-management)
   - [IP Management](#ip-management)
   - [Customer Management](#customer-management)
   - [Backup Management](#backup-management-admin)
   - [Audit Logs](#audit-logs)
   - [Settings](#settings)

2. [Customer Portal](#customer-portal)
   - [Overview](#customer-portal-overview)
   - [VM Management](#vm-management-customer)
   - [Console Access](#console-access)
   - [Backup Management](#backup-management-customer)
   - [Snapshot Management](#snapshot-management)
   - [Webhook Configuration](#webhook-configuration)
   - [Notification Preferences](#notification-preferences)
   - [API Keys](#api-keys)

3. [WHMCS Integration](#whmcs-integration)

---

## Admin Dashboard

### Admin Dashboard Overview ✅

The Admin Dashboard provides a comprehensive interface for managing the entire VirtueStack platform. Access it at `https://your-domain.com/admin`.

#### Dashboard Sections

| Section | Description |
|---------|-------------|
| **Dashboard** | Overview of system status, VM counts, node health |
| **Nodes** | Manage hypervisor nodes |
| **VMs** | View and manage all virtual machines |
| **Plans** | Create and manage VPS plans |
| **Templates** | Manage OS templates |
| **IP Sets** | Manage IP address pools |
| **Customers** | View and manage customer accounts |
| **Backups** | View all backups across the platform |
| **Audit Logs** | View system activity history |
| **Settings** | Configure platform settings |

#### Dashboard Metrics

When you first log in, the dashboard displays:

- **Total VMs**: Count of all active virtual machines
- **Active Nodes**: Number of online hypervisor nodes
- **Total Customers**: Registered customer count
- **Resource Usage**: Aggregate CPU/Memory/Disk utilization
- **Recent Activity**: Latest system events and VM changes
- **Node Health**: Status indicators for all registered nodes

### Node Management 🔧

Nodes are hypervisor servers that host virtual machines.

> **Status:** Backend API for node CRUD is working. Admin UI has the interface but is not yet wired to the live API. Node registration can be done via API.

#### Registering a New Node

1. Navigate to **Nodes** in the sidebar
2. Click **Register Node**
3. Fill in the required information:

| Field | Description | Example |
|-------|-------------|---------|
| Hostname | Node's DNS hostname | `node1.example.com` |
| gRPC Address | Node agent endpoint | `node1.example.com:50051` |
| Management IP | IP for management traffic | `192.168.1.10` |
| Location | Optional datacenter location | `US-East-1` |
| Total vCPU | Available CPU cores | `32` |
| Total Memory (MB) | Available RAM | `65536` |
| Ceph Pool | Storage pool name | `vs-vms` |

4. (Optional) Configure IPMI for out-of-band management:
   - IPMI Address
   - IPMI Username
   - IPMI Password

5. Click **Register Node**

#### Viewing Node Status

The node list shows:

- **Status**: Online, Offline, Degraded, Draining, Failed
- **Hostname**: Node identifier
- **Location**: Datacenter location
- **vCPU**: Allocated / Total
- **Memory**: Allocated / Total
- **VM Count**: Number of hosted VMs
- **Last Heartbeat**: Time of last health check

Click on a node to view detailed information:

- Resource utilization graphs
- List of hosted VMs
- Heartbeat history
- Network statistics

#### Node Operations

**Drain Node** (for maintenance):
1. Select the node from the list
2. Click **Drain Node**
3. Confirm the action
4. New VMs will not be provisioned on this node
5. Existing VMs continue to run normally

**Failover Node** (when node is unresponsive):
1. Select the failed node
2. Click **Initiate Failover**
3. Confirm the action
4. All VMs on the node will be marked for recovery
5. Automatic migration will be attempted if storage is shared

#### Node Status Codes

| Status | Description |
|--------|-------------|
| `online` | Node is healthy and accepting VMs |
| `degraded` | Node is operational but has warnings |
| `offline` | Node is not responding |
| `draining` | Node is under maintenance, no new VMs |
| `failed` | Node has failed, requires intervention |

### VM Management (Admin) 🔧

> **Status:** Backend API for VM CRUD is working. Admin UI has the interface but is not yet wired to the live API.

Admins can view and manage all VMs across all customers.

#### Viewing VMs

1. Navigate to **VMs** in the sidebar
2. Use filters to narrow the list:
   - Filter by Customer
   - Filter by Node
   - Filter by Status
   - Search by hostname

The VM list displays:

| Column | Description |
|--------|-------------|
| ID | Unique VM identifier (UUID) |
| Hostname | VM hostname |
| Customer | Owner's email |
| Node | Hosting node |
| Status | Current state |
| vCPU / Memory / Disk | Resource allocation |
| IP Addresses | Assigned IPs |

#### Creating a VM (Admin Override)

Admins can create VMs for any customer:

1. Click **Create VM**
2. Select the **Customer** from the dropdown
3. Choose a **Plan** for resource allocation
4. Select a **Template** for the OS
5. Enter a **Hostname** (RFC 1123 compliant)
6. Set the **Root Password**
7. (Optional) Add **SSH Keys**
8. (Optional) Select a specific **Node** or **Location**
9. Click **Create VM**

The VM creation is asynchronous. You'll receive a task ID to track progress.

#### VM Details Page

Click on any VM to view:

- **Overview**: Status, resources, IP addresses
- **Metrics**: Real-time CPU, memory, disk, network usage
- **Network**: IP addresses, bandwidth usage, traffic graphs
- **Storage**: Disk information, snapshots
- **Backups**: Backup history and restore options
- **Console**: Web-based console access
- **Activity**: VM event history

#### VM Operations

| Operation | Description |
|-----------|-------------|
| **Start** | Power on the VM |
| **Stop** | Graceful ACPI shutdown |
| **Force Stop** | Hard power off |
| **Restart** | Graceful reboot |
| **Migrate** | Move to another node |
| **Reinstall** | Rebuild with a new OS |
| **Delete** | Permanently remove the VM |

#### Migrating a VM

1. Select the VM
2. Click **Migrate**
3. Choose the **Target Node**
4. Select migration type:
   - **Live Migration**: No downtime (requires shared storage)
   - **Offline Migration**: VM will be stopped during migration
5. Click **Start Migration**

### Plan Management 🔧

> **Status:** Backend API is working. Admin UI has the interface but is not yet wired to the live API.

Plans define the resource allocations and pricing for VPS offerings.

#### Creating a Plan

1. Navigate to **Plans**
2. Click **Create Plan**
3. Configure the plan:

| Field | Description | Example |
|-------|-------------|---------|
| Name | Display name | `Starter VPS` |
| Slug | URL-friendly identifier | `starter-vps` |
| vCPU | CPU cores | `1` |
| Memory (MB) | RAM allocation | `1024` |
| Disk (GB) | Storage size | `20` |
| Bandwidth Limit (GB) | Monthly transfer limit | `1000` |
| Port Speed (Mbps) | Network speed | `1000` |
| Monthly Price | USD per month | `5.00` |
| Hourly Price | USD per hour | `0.007` |
| Active | Enable for purchase | ✓ |

4. Click **Create Plan**

#### Plan Best Practices

- Use descriptive names that indicate resources
- Set bandwidth limits to prevent abuse
- Offer multiple plans for different use cases
- Keep hourly and monthly prices proportional

### Template Management 🔧

> **Status:** Backend API is working. Admin UI has the interface but is not yet wired to the live API.

Templates are OS images used to provision VMs.

#### Creating a Template

1. Navigate to **Templates**
2. Click **Create Template**
3. Fill in template details:

| Field | Description | Example |
|-------|-------------|---------|
| Name | Display name | `Ubuntu 22.04 LTS` |
| Slug | URL identifier | `ubuntu-22-04` |
| OS Type | Operating system type | `linux` |
| Image Path | Path to QCOW2 image | `/templates/ubuntu-22.04.qcow2` |
| Minimum Disk (GB) | Minimum required disk | `10` |
| Default Username | SSH user for cloud-init | `ubuntu` |
| Cloud-Init Compatible | Supports cloud-init | ✓ |
| Active | Available for provisioning | ✓ |

4. Click **Create Template**

#### Importing Templates

To import an OS image:

1. Create or download a QCOW2 image
2. Upload to the ISO storage path on nodes
3. Create the template record in VirtueStack
4. Click **Import** on the template to process it

### IP Management 🔧

> **Status:** Backend API is working. Admin UI has the interface but is not yet wired to the live API.

IP Sets are pools of IP addresses available for VM assignment.

#### Creating an IP Set

1. Navigate to **IP Sets**
2. Click **Create IP Set**
3. Configure the IP pool:

| Field | Description | Example |
|-------|-------------|---------|
| Name | Pool identifier | `US-East IPv4` |
| Location ID | Associated location | `us-east-1` |
| Type | IPv4 or IPv6 | `ipv4` |
| CIDR | Network range | `192.168.1.0/24` |
| Gateway | Default gateway | `192.168.1.1` |
| Nameservers | DNS servers | `8.8.8.8,8.8.4.4` |

4. Click **Create IP Set**

#### Viewing Available IPs

1. Select an IP Set
2. Click **Available IPs**
3. View IPs that are unassigned and ready for use

### Customer Management 🔧

> **Status:** Backend API is working. Admin UI has the interface but is not yet wired to the live API.

#### Viewing Customers

1. Navigate to **Customers**
2. View all registered customers with:
   - Email address
   - Registration date
   - VM count
   - Status (active/suspended)

#### Customer Details

Click on a customer to view:

- Account information
- VMs owned
- Billing history
- Audit trail
- Support tickets (if WHMCS integrated)

#### Customer Operations

| Operation | Description |
|-----------|-------------|
| **View** | View customer details |
| **Edit** | Modify customer information |
| **Suspend** | Temporarily disable account |
| **Delete** | Remove customer (requires no active VMs) |

### Backup Management (Admin) ⚠️

> **Status:** Backend tables and API stubs exist. Full backup workflow (create, restore, delete) is not yet implemented end-to-end.

Admins can view and manage backups across all customers.

#### Viewing Backups

1. Navigate to **Backups**
2. View all backups with:
   - Backup ID
   - VM hostname
   - Type (full/incremental)
   - Size
   - Status
   - Creation date

#### Admin Backup Restore

Admins can restore any backup:

1. Select the backup
2. Click **Restore**
3. Confirm the action
4. The VM will be restored to the backup state

### Audit Logs 🔧

> **Status:** Backend audit logging is working. Admin UI has the interface but is not yet wired to the live API.

Audit logs track all administrative actions.

#### Viewing Audit Logs

1. Navigate to **Audit Logs**
2. Filter by:
   - Date range
   - Action type
   - Resource type
   - Actor (admin user)

#### Log Entry Details

Each log entry includes:

- Timestamp
- Action type (create, update, delete)
- Resource type (VM, Node, Plan, etc.)
- Resource ID
- Actor (who performed the action)
- Changes made (JSON diff)
- IP address

### Settings

Platform-wide configuration settings.

#### Available Settings

| Setting | Description |
|---------|-------------|
| Default backup retention | Days to keep backups |
| Max backups per VM | Backup limit |
| Webhook timeout | Seconds before timeout |
| Rate limit requests | API rate limiting |
| Maintenance mode | Disable customer portal |

---

## Customer Portal

### Customer Portal Overview ✅

> **Status:** Customer authentication (login → JWT) is fully working. Protected API endpoints are verified. The UI is built but not yet wired to the live backend API.

The Customer Portal provides self-service access for customers to manage their VPS services. Access it at `https://your-domain.com/`.

#### Portal Sections

| Section | Description |
|---------|-------------|
| **Dashboard** | Overview of VMs and resources |
| **Virtual Machines** | Manage your VPS instances |
| **Backups** | Create and restore backups |
| **Snapshots** | Point-in-time snapshots |
| **API Keys** | Manage programmatic access |
| **Webhooks** | Configure event notifications |
| **Settings** | Account and notification preferences |

### VM Management (Customer) 🔧

> **Status:** Backend API is working. Customer UI has the interface but is not yet wired to the live API.

#### Viewing Your VMs

1. Log in to the Customer Portal
2. Navigate to **Virtual Machines**
3. See all your VPS instances with:
   - Hostname
   - Status
   - IP Address
   - Plan name
   - Resource usage

#### Creating a New VM

1. Click **Create VM**
2. Select a **Plan** based on your needs
3. Choose an **OS Template**
4. Enter a **Hostname**
5. Set a **Root Password**
6. (Optional) Add **SSH Keys** for key-based authentication
7. (Optional) Select a **Location**
8. Click **Create VM**

The VM will be provisioned asynchronously. You'll be notified when it's ready.

#### VM Details

Click on any VM to access:

- **Overview**: Status, resources, connection info
- **Metrics**: Real-time performance graphs
- **Network**: Bandwidth usage, traffic history
- **Backups**: Backup management
- **Console**: Web-based terminal

#### Power Management

| Button | Action |
|--------|--------|
| **Start** | Power on the VM |
| **Stop** | Graceful shutdown via ACPI |
| **Restart** | Graceful reboot |
| **Force Stop** | Hard power off (use with caution) |

#### Reinstalling a VM

1. Navigate to the VM details
2. Click **Reinstall**
3. Select a new **OS Template**
4. Set a new **Root Password**
5. (Optional) Add **SSH Keys**
6. Confirm the reinstallation

**Warning**: This will erase all data on the VM!

#### Deleting a VM

1. Navigate to the VM details
2. Click **Delete**
3. Type the hostname to confirm
4. Click **Delete VM**

**Warning**: This action is permanent and cannot be undone!

### Console Access ⚠️

> **Status:** Not yet implemented. NoVNC/serial console integration is planned but not wired up.

#### NoVNC Console

The web-based console provides direct access to your VM:

1. Navigate to the VM details
2. Click **Console**
3. A new window opens with NoVNC
4. Log in with your root credentials

**Features**:
- Full graphical console access
- Clipboard support
- Fullscreen mode
- Send special keys (Ctrl+Alt+Del, etc.)

#### Serial Console

For headless access:

1. Navigate to the VM details
2. Click **Serial Console**
3. Terminal access opens directly

**Use Cases**:
- Quick SSH-like access
- Troubleshooting network issues
- Headless server management

### Backup Management (Customer) ⚠️

> **Status:** Backend tables exist. Customer-facing backup workflow is not yet implemented end-to-end.

#### Viewing Backups

1. Navigate to **Backups**
2. See all backups for your VMs:
   - Backup name
   - VM hostname
   - Size
   - Status
   - Creation date

#### Creating a Backup

1. Click **Create Backup**
2. Select the **VM**
3. (Optional) Enter a **Name**
4. Click **Create Backup**

**Notes**:
- Backup creation may take several minutes
- VM performance may be slightly impacted during backup
- You'll receive a notification when complete

#### Restoring a Backup

1. Select the backup
2. Click **Restore**
3. Confirm the action

**Warning**: The VM will be stopped during restore and all current data will be replaced!

#### Deleting a Backup

1. Select the backup
2. Click **Delete**
3. Confirm the deletion

### Snapshot Management ⚠️

> **Status:** Backend tables exist. Snapshot workflow is not yet implemented end-to-end.

Snapshots are lightweight point-in-time states stored in Ceph.

#### Creating a Snapshot

1. Navigate to **Snapshots**
2. Click **Create Snapshot**
3. Select the **VM**
4. Enter a **Name**
5. Click **Create**

#### Restoring a Snapshot

1. Select the snapshot
2. Click **Restore**
3. Confirm the action

**Warning**: The VM will be stopped during snapshot restore!

#### Deleting a Snapshot

1. Select the snapshot
2. Click **Delete**
3. Confirm the deletion

### Webhook Configuration 🔧

> **Status:** Backend webhook tables and delivery system exist. Customer UI has the interface but is not yet wired to the live API.

Webhooks allow you to receive event notifications at your own endpoints.

#### Creating a Webhook

1. Navigate to **Webhooks**
2. Click **Create Webhook**
3. Enter the **URL** (must be HTTPS)
4. Create a **Secret** (16-128 characters)
5. Select **Events** to subscribe to:
   - `vm.created` - VM provisioning complete
   - `vm.deleted` - VM terminated
   - `vm.started` - VM powered on
   - `vm.stopped` - VM powered off
   - `vm.reinstalled` - OS reinstallation complete
   - `vm.migrated` - VM migrated to new node
   - `backup.completed` - Backup finished
   - `backup.failed` - Backup failed

6. Click **Create Webhook**

#### Webhook Secret

The secret is used to sign payloads. Verify webhooks using HMAC-SHA256:

```python
import hmac
import hashlib

def verify_webhook(secret, payload, signature):
    expected = hmac.new(
        secret.encode(),
        payload,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)
```

#### Webhook Payload Format

```json
{
  "event": "vm.created",
  "timestamp": "2024-01-15T10:30:00Z",
  "idempotency_key": "uuid-here",
  "data": {
    "vm_id": "uuid",
    "hostname": "my-vps",
    "ip_addresses": ["192.168.1.100"],
    "status": "running"
  }
}
```

#### Viewing Delivery History

1. Select a webhook
2. Click **Deliveries**
3. View all delivery attempts with:
   - Event type
   - Response status
   - Success/failure
   - Retry schedule

### Notification Preferences ⚠️

> **Status:** Backend tables exist. Notification delivery (email/Telegram) is not yet implemented.

Configure how you receive notifications.

#### Available Notification Types

| Event | Email | Telegram |
|-------|-------|----------|
| VM Created | ✓ | ✓ |
| VM Deleted | ✓ | ✓ |
| VM Started | ✓ | ✓ |
| VM Stopped | ✓ | ✓ |
| Backup Completed | ✓ | ✓ |
| Backup Failed | ✓ | ✓ |
| Bandwidth Alert | ✓ | ✓ |
| Invoice Reminder | ✓ | ✓ |

#### Configuring Notifications

1. Navigate to **Settings** > **Notifications**
2. Toggle notification types on/off
3. Configure email and/or Telegram settings
4. Click **Save Preferences**

### API Keys ⚠️

> **Status:** Backend API key table exists. Customer-facing API key management UI is not yet wired to the live API.

Create API keys for programmatic access to your account.

#### Creating an API Key

1. Navigate to **API Keys**
2. Click **Create API Key**
3. Enter a **Name** (e.g., "CI/CD Pipeline")
4. Select **Permissions**:
   - `vms:read` - View VMs
   - `vms:write` - Create/modify VMs
   - `backups:read` - View backups
   - `backups:write` - Create/restore backups
5. (Optional) Set an **Expiration Date**
6. Click **Create**

**Important**: Copy the API key immediately! It cannot be retrieved later.

#### Using API Keys

```bash
# Example: List your VMs
curl -X GET https://your-domain.com/api/v1/customer/vms \
  -H "Authorization: Bearer YOUR_API_KEY"
```

#### Revoking an API Key

1. Find the key in the list
2. Click **Revoke**
3. Confirm the action

---

## WHMCS Integration 🔧

### Overview

> **Status:** WHMCS PHP module files exist and pass syntax checks. The provisioning API backend is working and authenticated. Full WHMCS ↔ VirtueStack integration has not been tested end-to-end with a live WHMCS instance.

VirtueStack integrates with WHMCS for automated billing and provisioning.

### Configuring the WHMCS Module

1. **Install the Module**:
   ```bash
   cp -r modules/whmcs/virtuestack /path/to/whmcs/modules/servers/
   ```

2. **Create a Server in WHMCS**:
   - Navigate to Setup > Products/Services > Servers
   - Click "Add New Server"
   - Name: `VirtueStack`
   - Type: `VirtueStack`
   - Hostname: Your VirtueStack domain
   - API Key: From Admin UI > Settings > Provisioning Keys

3. **Create a Product**:
   - Navigate to Setup > Products/Services > Products/Services
   - Create a new product
   - Module Settings: Select VirtueStack
   - Select the Plan from VirtueStack

### Automatic Provisioning

When an order is placed in WHMCS:

1. WHMCS calls the VirtueStack provisioning API
2. A VM is created with the customer's details
3. The VM ID is stored in WHMCS
4. The customer receives login credentials

### Supported Operations

| WHMCS Action | VirtueStack Operation |
|--------------|----------------------|
| Create | Provision new VM |
| Suspend | Suspend VM |
| Unsuspend | Unsuspend VM |
| Terminate | Delete VM |
| Change Password | Reset root password |
| Change Package | Resize VM resources |

### Webhook Integration

Configure WHMCS to receive VirtueStack webhooks for:

- Automatic status updates
- Bandwidth usage tracking
- Service status sync

---

## Best Practices

### Security

1. **Enable 2FA**: Require two-factor authentication for admin accounts
2. **Use Strong Passwords**: Minimum 12 characters with complexity
3. **Rotate API Keys**: Change provisioning keys periodically
4. **Limit IP Access**: Restrict provisioning API to WHMCS server IP
5. **Audit Regularly**: Review audit logs for suspicious activity

### Resource Management

1. **Monitor Bandwidth**: Set alerts for bandwidth overages
2. **Regular Backups**: Schedule automatic daily backups
3. **Right-size Plans**: Match plans to actual customer needs
4. **Clean Up**: Remove unused templates and old backups

### Operational

1. **Test Backups**: Regularly verify backup integrity
2. **Document Customizations**: Keep records of any custom configurations
3. **Monitor Nodes**: Set up alerts for node health
4. **Plan for Growth**: Add nodes before capacity is exhausted

---

## Getting Help

- **Documentation**: [API Reference](./API.md)
- **Installation**: [Installation Guide](./INSTALL.md)
- **GitHub Issues**: Report bugs and request features
- **Community**: Join discussions on GitHub