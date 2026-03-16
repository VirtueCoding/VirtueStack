# VirtueStack API Reference

> **Version:** 2.0  
> **Base URL:** `https://<controller-host>/api/v1`  
> **Protocol:** HTTPS (enforced in production)  
> **Content-Type:** `application/json`  
> **Character Encoding:** UTF-8

---

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
  - [Admin Authentication (JWT + 2FA)](#admin-authentication-jwt--2fa)
  - [Customer Authentication (JWT)](#customer-authentication-jwt)
  - [Provisioning Authentication (API Key)](#provisioning-authentication-api-key)
  - [CSRF Protection](#csrf-protection)
  - [Rate Limiting](#rate-limiting)
  - [Correlation IDs](#correlation-ids)
- [Response Formats](#response-formats)
  - [Success Response](#success-response)
  - [Paginated List Response](#paginated-list-response)
  - [Error Response](#error-response)
  - [Async Task Response](#async-task-response)
- [Common Error Codes](#common-error-codes)
- [Pagination](#pagination)
- [Admin API](#admin-api)
  - [Authentication](#admin-api-authentication)
  - [Nodes](#admin-api-nodes)
  - [Failover Requests](#admin-api-failover-requests)
  - [Virtual Machines](#admin-api-virtual-machines)
  - [Plans](#admin-api-plans)
  - [Templates](#admin-api-templates)
  - [IP Sets](#admin-api-ip-sets)
  - [Customers](#admin-api-customers)
  - [Audit Logs](#admin-api-audit-logs)
  - [Settings](#admin-api-settings)
  - [Backups](#admin-api-backups)
  - [Backup Schedules](#admin-api-backup-schedules)
- [Customer API](#customer-api)
  - [Authentication](#customer-api-authentication)
  - [Profile](#customer-api-profile)
  - [Password Management](#customer-api-password-management)
  - [Two-Factor Authentication](#customer-api-two-factor-authentication)
  - [Virtual Machines](#customer-api-virtual-machines)
  - [VM Power Control](#customer-api-vm-power-control)
  - [VM Console Access](#customer-api-vm-console-access)
  - [VM Monitoring & Metrics](#customer-api-vm-monitoring--metrics)
  - [VM Network](#customer-api-vm-network)
  - [VM ISO Management](#customer-api-vm-iso-management)
  - [Backups](#customer-api-backups)
  - [Snapshots](#customer-api-snapshots)
  - [API Keys](#customer-api-api-keys)
  - [Webhooks](#customer-api-webhooks)
  - [Templates](#customer-api-templates)
  - [Notifications](#customer-api-notifications)
  - [Reverse DNS (rDNS)](#customer-api-reverse-dns-rdns)
  - [WebSocket Connections](#customer-api-websocket-connections)
- [Provisioning API](#provisioning-api)
  - [Virtual Machines](#provisioning-api-virtual-machines)
  - [VM Status](#provisioning-api-vm-status)
  - [Task Polling](#provisioning-api-task-polling)
- [Data Models Reference](#data-models-reference)
- [WebSocket Protocol](#websocket-protocol)

---

## Overview

VirtueStack exposes a three-tier RESTful API designed for different consumers:

| Tier | Base Path | Audience | Authentication | Primary Use Case |
|------|-----------|----------|----------------|------------------|
| **Admin** | `/api/v1/admin` | System administrators | JWT + mandatory 2FA | Full platform management |
| **Customer** | `/api/v1/customer` | VPS customers | JWT + optional 2FA | Self-service VM management |
| **Provisioning** | `/api/v1/provisioning` | Billing systems (WHMCS) | API Key (X-API-Key) | Automated VM provisioning |

All requests and responses use JSON. The API is stateless — all necessary state is carried in JWT tokens or API keys.

---

## Authentication

### Admin Authentication (JWT + 2FA)

Admin authentication uses a two-step flow: credentials verification followed by mandatory TOTP 2FA.

**Step 1: Login with email and password**

```http
POST /api/v1/admin/auth/login
Content-Type: application/json

{
  "email": "admin@example.com",
  "password": "securepassword123"
}
```

**Response (2FA required):**

```json
{
  "data": {
    "token_type": "Bearer",
    "expires_in": 300,
    "requires_2fa": true,
    "temp_token": "eyJhbGciOiJIUzI1NiIs..."
  }
}
```

The `temp_token` is a short-lived token (5 minutes) used exclusively for the 2FA verification step. It cannot be used for authenticated endpoints.

**Step 2: Verify TOTP code**

```http
POST /api/v1/admin/auth/verify-2fa
Content-Type: application/json

{
  "temp_token": "eyJhbGciOiJIUzI1NiIs...",
  "totp_code": "123456"
}
```

**Response (success):**

```json
{
  "data": {
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

On success, the server sets two HTTP-only cookies:
- `access_token` — JWT access token (max age: 15 minutes)
- `refresh_token` — Refresh token (max age: 4 hours for admin)

**Using tokens:**

Include the access token in subsequent requests via the `Authorization` header:

```http
Authorization: Bearer <access_token>
```

Or rely on the HTTP-only cookies (automatic). If cookies are present, the `Authorization` header takes precedence.

**Token refresh:**

```http
POST /api/v1/admin/auth/refresh
Content-Type: application/json

{
  "refresh_token": "<refresh_token>"
}
```

The refresh token can be provided in the request body or via the `refresh_token` cookie. On success, both cookies are updated with new tokens.

**Logout:**

```http
POST /api/v1/admin/auth/logout
```

Clears the authentication cookies.

#### JWT Claims (Admin)

```json
{
  "user_id": "uuid-of-admin",
  "email": "admin@example.com",
  "role": "admin",
  "user_type": "admin",
  "iss": "virtuestack",
  "exp": 1700000000
}
```

Valid roles: `admin`, `super_admin`.

#### Login Request Fields

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `email` | string | Yes | Valid email, max 254 characters |
| `password` | string | Yes | Min 12 characters, max 128 characters |

#### Verify2FA Request Fields

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `temp_token` | string | Yes | JWT from login response |
| `totp_code` | string | Yes | Exactly 6 numeric digits |

---

### Customer Authentication (JWT)

Customer authentication supports both direct login (JWT) and optional 2FA (if the customer has enabled TOTP).

**Login:**

```http
POST /api/v1/customer/auth/login
Content-Type: application/json

{
  "email": "customer@example.com",
  "password": "securepassword123"
}
```

**Response (no 2FA):**

```json
{
  "data": {
    "token_type": "Bearer",
    "expires_in": 900,
    "requires_2fa": false
  }
}
```

**Response (2FA enabled):**

```json
{
  "data": {
    "token_type": "Bearer",
    "expires_in": 300,
    "requires_2fa": true,
    "temp_token": "eyJhbGciOiJIUzI1NiIs..."
  }
}
```

On success (without 2FA), the server sets two HTTP-only cookies:
- `access_token` — JWT access token (max age: 15 minutes)
- `refresh_token` — Refresh token (max age: 7 days for customer)

**Verify 2FA (if enabled):**

```http
POST /api/v1/customer/auth/verify-2fa
Content-Type: application/json

{
  "temp_token": "eyJhbGciOiJIUzI1NiIs...",
  "totp_code": "123456"
}
```

**Refresh token:**

```http
POST /api/v1/customer/auth/refresh
Content-Type: application/json

{
  "refresh_token": "<refresh_token>"
}
```

**Logout:**

```http
POST /api/v1/customer/auth/logout
```

#### Login Request Fields

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `email` | string | Yes | Valid email, max 254 characters |
| `password` | string | Yes | Min 12 characters, max 128 characters |

#### JWT Claims (Customer)

```json
{
  "user_id": "uuid-of-customer",
  "email": "customer@example.com",
  "role": "customer",
  "user_type": "customer",
  "iss": "virtuestack",
  "exp": 1700000000
}
```

---

### Provisioning Authentication (API Key)

The Provisioning API uses API key authentication. Keys are created in the admin panel and validated against the `provisioning_keys` table.

```http
GET /api/v1/provisioning/vms/:id
X-API-Key: vs_live_a1b2c3d4e5f6...
```

**API key format:** `vs_<uuid>` (total length: 39 characters)

**Validation process:**
1. The raw key is SHA-256 hashed
2. The hash is looked up in the `provisioning_keys` table
3. If the key has `allowed_ips` configured, the request IP is checked against the list
4. Successful validation sets `user_id` context to the key's ID

**Key properties:**
- Keys can be restricted to specific IP addresses (CIDR notation supported)
- Keys track last-used timestamps
- Keys can be revoked (soft delete)

---

### CSRF Protection

Protected endpoints (all write operations after login) enforce CSRF protection using the double-submit cookie pattern.

The server sets a `csrf_token` cookie on login. Include the same token in the `X-CSRF-Token` header for all POST/PUT/DELETE requests:

```http
X-CSRF-Token: <value-from-csrf_token-cookie>
```

CSRF protection applies to all authenticated (JWT-protected) routes. The Provisioning API (API key auth) is exempt.

---

### Rate Limiting

Rate limiting uses a sliding-window algorithm with per-endpoint limits.

| Endpoint Category | Limit | Window |
|-------------------|-------|--------|
| **Admin — General** | 500 requests | per minute |
| **Customer — Read** | 100 requests | per minute |
| **Customer — Write** | 30 requests | per minute |
| **Provisioning — All** | 1000 requests | per minute |

Specific per-action limits:

| Action | Limit | Window |
|--------|-------|--------|
| VM Create | 5 requests | per minute |
| VM Delete | 10 requests | per minute |
| VM Start | 20 requests | per minute |
| VM Stop | 20 requests | per minute |
| VM List | 100 requests | per minute |
| Console Token | 10 requests | per minute |
| Backup Create | 5 requests | per minute |
| rDNS Update | 30 requests | per minute |
| Login | 10 requests | per minute |
| Password Change | 3 requests | per minute |

**Rate limit headers (included in all responses):**

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1700000060
```

**Rate limit exceeded response:**

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded. Try again in 30 seconds.",
    "correlation_id": "req_abc123"
  }
}
```

HTTP status: `429 Too Many Requests`

---

### Correlation IDs

Every request receives a unique correlation ID for tracing. The server checks for an incoming `X-Correlation-ID` header and uses it if present; otherwise, a new UUID is generated.

```http
X-Correlation-ID: req_abc123
```

All responses include the correlation ID in error responses and structured log entries.

---

## Response Formats

### Success Response

Single-item responses use the `data` wrapper:

```json
{
  "data": { ... }
}
```

HTTP status codes: `200 OK`, `201 Created`, `202 Accepted` (async operations)

### Paginated List Response

List endpoints return paginated results:

```json
{
  "data": [ ... ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 150,
    "total_pages": 8
  }
}
```

### Error Response

All errors follow a consistent format:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Hostname format is invalid",
    "correlation_id": "req_abc123"
  }
}
```

For validation errors with field-level details:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": [
      {
        "field": "hostname",
        "issue": "Must be RFC 1123 compliant"
      }
    ],
    "correlation_id": "req_abc123"
  }
}
```

### Async Task Response

Long-running operations (VM creation, deletion, migration, backup, restore, snapshot) return a `task_id` for status polling:

```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440000",
    "task_id": "660e8400-e29b-41d4-a716-446655440001"
  }
}
```

HTTP status: `202 Accepted`

---

## Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `VALIDATION_ERROR` | 400 | Request body or query parameters failed validation |
| `INVALID_CREDENTIALS` | 401 | Email/password combination is incorrect |
| `INVALID_2FA_CODE` | 401 | TOTP code is invalid or expired |
| `INVALID_REFRESH_TOKEN` | 401 | Refresh token is invalid or expired |
| `UNAUTHORIZED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Authenticated but not authorized for the resource |
| `NOT_FOUND` / `*_NOT_FOUND` | 404 | Requested resource does not exist |
| `CONFLICT` | 409 | Resource state conflict (e.g., plan in use, VM not running) |
| `RATE_LIMITED` | 429 | Rate limit exceeded |
| `INTERNAL_ERROR` | 500 | Unexpected server error |

---

## Pagination

All list endpoints support pagination via query parameters:

| Parameter | Type | Default | Max | Description |
|-----------|------|---------|-----|-------------|
| `page` | integer | 1 | - | Page number (1-indexed) |
| `per_page` | integer | 20 | 100 | Items per page |

**Example:**

```http
GET /api/v1/admin/nodes?page=2&per_page=50
```

**Response metadata:**

```json
{
  "data": [ ... ],
  "meta": {
    "page": 2,
    "per_page": 50,
    "total": 157,
    "total_pages": 4
  }
}
```

---

## Admin API

**Base Path:** `/api/v1/admin`  
**Authentication:** JWT (Bearer) with role `admin` or `super_admin` + mandatory 2FA  
**CSRF:** Required for all write operations  
**Rate Limit:** 500 requests/minute (general), plus per-action limits

---

### Admin API Authentication

#### `POST /admin/auth/login`

Authenticate an admin user. Returns a `temp_token` if 2FA is enabled (always for admins).

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `email` | string | Yes | Valid email, max 254 chars |
| `password` | string | Yes | Min 12, max 128 chars |

**Response (`200 OK`):**

| Field | Type | Description |
|-------|------|-------------|
| `token_type` | string | Always `"Bearer"` |
| `expires_in` | integer | Token lifetime in seconds |
| `requires_2fa` | boolean | Whether 2FA verification is needed |
| `temp_token` | string | Short-lived token for 2FA step (if `requires_2fa` is true) |

**Error Responses:**
- `401` — `INVALID_CREDENTIALS`: Invalid email or password
- `500` — `LOGIN_FAILED`: Server error during login

---

#### `POST /admin/auth/verify-2fa`

Verify TOTP code to complete authentication.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `temp_token` | string | Yes | Token from login response |
| `totp_code` | string | Yes | Exactly 6 numeric digits |

**Response (`200 OK`):**

| Field | Type | Description |
|-------|------|-------------|
| `token_type` | string | `"Bearer"` |
| `expires_in` | integer | Access token lifetime (900 seconds / 15 min) |

Sets `access_token` and `refresh_token` HTTP-only cookies.

**Error Responses:**
- `401` — `INVALID_2FA_CODE`: Invalid or expired 2FA code

---

#### `POST /admin/auth/refresh`

Refresh the access token using a refresh token.

**Request Body (optional):**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `refresh_token` | string | No | If omitted, cookie is used |

**Response (`200 OK`):**

| Field | Type | Description |
|-------|------|-------------|
| `token_type` | string | `"Bearer"` |
| `expires_in` | integer | New access token lifetime |

Updates both cookies.

**Error Responses:**
- `400` — `VALIDATION_ERROR`: No refresh token provided
- `401` — `INVALID_REFRESH_TOKEN`: Token is invalid or expired

---

#### `POST /admin/auth/logout`

Invalidate the current session and clear cookies.

**Response (`200 OK`):**

```json
{
  "data": { "message": "Logged out successfully" }
}
```

---

### Admin API Nodes

#### `GET /admin/nodes`

List all hypervisor nodes with optional filtering.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number (default: 1) |
| `per_page` | integer | No | Items per page (default: 20, max: 100) |
| `status` | string | No | Filter by status: `online`, `degraded`, `offline`, `draining`, `failed` |
| `location_id` | string | No | Filter by location UUID |

**Response (`200 OK`):**

Returns an array of `NodeStatus` objects:

```json
{
  "data": [
    {
      "node_id": "uuid",
      "hostname": "node-01",
      "status": "online",
      "last_heartbeat_at": "2026-03-15T10:30:00Z",
      "consecutive_heartbeat_misses": 0,
      "total_vcpu": 64,
      "allocated_vcpu": 24,
      "available_vcpu": 40,
      "total_memory_mb": 262144,
      "allocated_memory_mb": 98304,
      "available_memory_mb": 163840,
      "vm_count": 8,
      "cpu_percent": 35.2,
      "memory_percent": 42.1,
      "disk_percent": 28.5,
      "total_disk_gb": 2000,
      "used_disk_gb": 570,
      "ceph_status": "connected",
      "ceph_total_gb": 10000,
      "ceph_used_gb": 3200,
      "load_average": [1.2, 1.5, 1.8],
      "is_healthy": true
    }
  ],
  "meta": { "page": 1, "per_page": 20, "total": 3, "total_pages": 1 }
}
```

---

#### `POST /admin/nodes`

Register a new hypervisor node.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `hostname` | string | Yes | Max 255 chars |
| `grpc_address` | string | Yes | Max 255 chars |
| `management_ip` | string | Yes | Valid IP address |
| `location_id` | string | No | Valid UUID |
| `total_vcpu` | integer | Yes | Min 1 |
| `total_memory_mb` | integer | Yes | Min 1024 |
| `storage_backend` | string | Yes | `ceph` or `qcow` |
| `storage_path` | string | No | Max 500 chars (required when `storage_backend` is `qcow`) |
| `ceph_pool` | string | No | Max 100 chars |
| `ipmi_address` | string | No | Valid IP address |
| `ipmi_username` | string | No | IPMI username |
| `ipmi_password` | string | No | IPMI password |

**Response (`201 Created`):** Returns the created `Node` object.

**Error Responses:**
- `400` — `VALIDATION_ERROR`: Invalid request body
- `500` — `NODE_REGISTER_FAILED`: Registration failed

---

#### `GET /admin/nodes/:id`

Get detailed node status including live health metrics.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string (UUID) | Node ID |

**Response (`200 OK`):** Returns `NodeStatus` object with aggregated health data.

If live metrics are unavailable (e.g., node offline), returns the basic `Node` record.

**Error Responses:**
- `400` — `INVALID_NODE_ID`: Not a valid UUID
- `404` — `NODE_NOT_FOUND`: Node does not exist

---

#### `PUT /admin/nodes/:id`

Update node configuration. All fields are optional (partial update).

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string (UUID) | Node ID |

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `grpc_address` | string | No | Max 255 chars |
| `location_id` | string | No | Valid UUID |
| `total_vcpu` | integer | No | Min 1 |
| `total_memory_mb` | integer | No | Min 1024 |
| `ipmi_address` | string | No | Valid IP address |
| `storage_backend` | string | No | `ceph` or `qcow` |
| `storage_path` | string | No | Max 500 chars |

**Response (`200 OK`):** Returns the updated `Node` object.

---

#### `DELETE /admin/nodes/:id`

Permanently remove a node. Nodes with running VMs cannot be deleted.

**Response (`200 OK`):**

```json
{
  "data": { "deleted": true }
}
```

---

#### `POST /admin/nodes/:id/drain`

Set a node to draining mode. New VMs will not be placed on this node, but existing VMs continue running.

**Response (`200 OK`):**

```json
{
  "data": { "status": "draining" }
}
```

---

#### `POST /admin/nodes/:id/failover`

Mark a node as failed. This triggers IPMI fencing, Ceph blocklist, and VM redistribution to other healthy nodes.

**Response (`200 OK`):**

```json
{
  "data": { "status": "failed" }
}
```

---

#### `POST /admin/nodes/:id/undrain`

Restore a draining node to online mode. New VM placements will resume.

**Response (`200 OK`):**

```json
{
  "data": { "status": "online" }
}
```

---

### Admin API Failover Requests

#### `GET /admin/failover-requests`

List all failover requests with optional filtering.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `node_id` | string | No | Filter by node UUID |
| `status` | string | No | Filter by status: `pending`, `approved`, `in_progress`, `completed`, `failed`, `cancelled` |

**Response (`200 OK`):** Returns paginated array of `FailoverRequest` objects.

---

#### `GET /admin/failover-requests/:id`

Get details of a specific failover request.

**Response (`200 OK`):** Returns `FailoverRequest` object.

---

### Admin API Virtual Machines

#### `GET /admin/vms`

List all VMs across all customers.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `customer_id` | string | No | Filter by customer UUID |
| `node_id` | string | No | Filter by node UUID |
| `status` | string | No | Filter by VM status |
| `search` | string | No | Search by hostname (substring match) |

**Response (`200 OK`):** Returns paginated array of `VM` objects.

---

#### `POST /admin/vms`

Create a new VM for any customer (admin override). This is an async operation.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `customer_id` | string | Yes | Valid UUID |
| `plan_id` | string | Yes | Valid UUID |
| `template_id` | string | Yes | Valid UUID |
| `hostname` | string | Yes | RFC 1123 compliant, max 63 chars |
| `password` | string | Yes | Min 8, max 128 chars |
| `ssh_keys` | string[] | No | Max 10 keys, max 4096 chars each |
| `location_id` | string | No | Valid UUID (auto-select if omitted) |
| `node_id` | string | No | Valid UUID (force specific node) |

**Response (`202 Accepted`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "task_id": "uuid"
  }
}
```

---

#### `GET /admin/vms/:id`

Get detailed VM information including IP addresses and plan details.

**Response (`200 OK`):** Returns `VMDetail` object (enriched with `ip_addresses`, `ipv6_subnets`, `node_hostname`, `plan_name`, `template_name`).

---

#### `PUT /admin/vms/:id`

Update VM configuration. If `vcpu`, `memory_mb`, or `disk_gb` are changed, a resize task is created.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `hostname` | string | No | RFC 1123, max 63 chars |
| `vcpu` | integer | No | Min 1 (triggers async resize) |
| `memory_mb` | integer | No | Min 512 (triggers async resize) |
| `disk_gb` | integer | No | Min 10 (triggers async resize) |
| `port_speed_mbps` | integer | No | Min 1 |
| `bandwidth_limit_gb` | integer | No | Min 0 |

**Response:**
- `200 OK` — Non-resize fields updated, returns updated VM
- `202 Accepted` — Resize initiated, returns task_id

---

#### `DELETE /admin/vms/:id`

Delete a VM. This is an async operation.

**Response (`202 Accepted`):**

```json
{
  "data": {
    "task_id": "uuid"
  }
}
```

---

#### `POST /admin/vms/:id/migrate`

Migrate a VM to another node. Validates that the target node supports the VM's storage backend.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `target_node_id` | string | Yes | Valid UUID |

**Response (`202 Accepted`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "target_node_id": "uuid",
    "task_id": "uuid",
    "status": "migration_initiated"
  }
}
```

**Error Responses:**
- `400` — `NODE_NOT_FOUND`: Target node does not exist
- `500` — `MIGRATION_FAILED`: Migration could not be initiated

---

#### `GET /admin/vms/:id/ips`

Get IP addresses assigned to a VM.

**Response (`200 OK`):** Returns array of `IPAddress` objects.

---

#### `GET /admin/vms/:id/ips/:ipId/rdns`

Get reverse DNS configuration for a specific IP address.

**Response (`200 OK`):** Returns `RDNS` object.

---

#### `PUT /admin/vms/:id/ips/:ipId/rdns`

Set reverse DNS for an IP address. Updates both the database and PowerDNS (if configured).

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `hostname` | string | Yes | RFC 1123 compliant, max 253 chars |

**Response (`200 OK`):** Returns updated `RDNS` object.

---

#### `DELETE /admin/vms/:id/ips/:ipId/rdns`

Remove reverse DNS record for an IP address.

**Response (`200 OK`):** Returns success message.

---

### Admin API Plans

#### `GET /admin/plans`

List all service plans.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `is_active` | boolean | No | Filter by active status (`true`/`false`) |

**Response (`200 OK`):** Returns paginated array of `Plan` objects.

---

#### `POST /admin/plans`

Create a new service plan.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `name` | string | Yes | Max 100 chars |
| `slug` | string | Yes | Max 100 chars, alphanumeric |
| `vcpu` | integer | Yes | Min 1 |
| `memory_mb` | integer | Yes | Min 512 |
| `disk_gb` | integer | Yes | Min 10 |
| `bandwidth_limit_gb` | integer | No | Min 0 |
| `port_speed_mbps` | integer | Yes | Min 1 |
| `price_monthly` | integer | No | Min 0 (cents) |
| `price_hourly` | integer | No | Min 0 (cents) |
| `storage_backend` | string | No | `ceph` or `qcow` (default: `ceph`) |
| `is_active` | boolean | No | Default: false |
| `sort_order` | integer | No | Min 0 |

**Response (`201 Created`):** Returns the created `Plan` object.

---

#### `PUT /admin/plans/:id`

Update an existing plan (partial update).

**Request Body:** Same fields as create, all optional.

**Response (`200 OK`):** Returns the updated `Plan` object.

---

#### `DELETE /admin/plans/:id`

Delete a plan. Plans with existing VMs cannot be deleted.

**Response (`200 OK`):**

```json
{
  "data": { "deleted": true }
}
```

**Error Responses:**
- `404` — `PLAN_NOT_FOUND`: Plan does not exist
- `409` — `PLAN_IN_USE`: Cannot delete plan with existing VMs

---

### Admin API Templates

#### `GET /admin/templates`

List all OS templates.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `is_active` | boolean | No | Filter by active status |
| `os_family` | string | No | Filter by OS family (e.g., `ubuntu`, `debian`, `centos`) |

**Response (`200 OK`):** Returns paginated array of `Template` objects.

---

#### `POST /admin/templates`

Register a new OS template.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `name` | string | Yes | Max 100 chars |
| `os_family` | string | Yes | Max 50 chars (e.g., `ubuntu`, `debian`, `centos`, `almalinux`) |
| `os_version` | string | Yes | Max 50 chars (e.g., `24.04`, `12`, `9`) |
| `rbd_image` | string | Yes | Max 255 chars |
| `rbd_snapshot` | string | Yes | Max 255 chars |
| `min_disk_gb` | integer | Yes | Min 1 |
| `supports_cloudinit` | boolean | No | Default: false |
| `is_active` | boolean | No | Default: false |
| `sort_order` | integer | No | Min 0 |
| `description` | string | No | Max 500 chars |
| `storage_backend` | string | No | `ceph` or `qcow` (default: `ceph`) |
| `file_path` | string | No | Max 500 chars (QCOW template path) |

**Response (`201 Created`):** Returns the created `Template` object.

---

#### `PUT /admin/templates/:id`

Update an existing template (partial update).

**Request Body:** Same fields as create, all optional.

**Response (`200 OK`):** Returns the updated `Template` object.

---

#### `DELETE /admin/templates/:id`

Delete a template.

**Response (`200 OK`):**

```json
{
  "data": { "deleted": true }
}
```

---

#### `POST /admin/templates/:id/import`

Import an OS image from a source path. This creates/updates the template's disk image in the storage backend.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `name` | string | Yes | Max 100 chars |
| `os_family` | string | Yes | Max 50 chars |
| `os_version` | string | Yes | Max 50 chars |
| `source_path` | string | Yes | Max 512 chars |
| `storage_backend` | string | No | `ceph` or `qcow` |
| `supports_cloudinit` | boolean | No | Default: false |
| `is_active` | boolean | No | Default: false |

**Response (`202 Accepted`):** Import is initiated as an async operation.

---

### Admin API IP Sets

#### `GET /admin/ip-sets`

List all IP address pools.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |

**Response (`200 OK`):** Returns paginated array of `IPSet` objects.

---

#### `POST /admin/ip-sets`

Create a new IP address pool.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `name` | string | Yes | Max 100 chars |
| `location_id` | string | No | Valid UUID |
| `network` | string | Yes | Valid CIDR notation |
| `gateway` | string | Yes | Valid IP address |
| `vlan_id` | integer | No | Min 1, max 4094 |
| `ip_version` | integer | Yes | `4` or `6` |
| `node_ids` | string[] | No | Array of valid UUIDs |

**Response (`201 Created`):** Returns the created `IPSet` object.

---

#### `GET /admin/ip-sets/:id`

Get IP set details.

**Response (`200 OK`):** Returns `IPSet` object.

---

#### `PUT /admin/ip-sets/:id`

Update an existing IP set (partial update).

**Request Body:** Same fields as create, all optional.

**Response (`200 OK`):** Returns the updated `IPSet` object.

---

#### `DELETE /admin/ip-sets/:id`

Delete an IP set. IP sets with assigned addresses cannot be deleted.

**Response (`200 OK`):**

```json
{
  "data": { "deleted": true }
}
```

---

#### `GET /admin/ip-sets/:id/available`

List available (unassigned) IP addresses in the set.

**Response (`200 OK`):** Returns array of `IPAddress` objects with `status: "available"`.

---

### Admin API Customers

#### `GET /admin/customers`

List all customer accounts.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |

**Response (`200 OK`):** Returns paginated array of `Customer` objects.

---

#### `GET /admin/customers/:id`

Get customer details.

**Response (`200 OK`):** Returns `Customer` object.

---

#### `PUT /admin/customers/:id`

Update customer account (partial update).

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `name` | string | No | Max 255 chars |
| `email` | string | No | Valid email, max 254 chars |
| `phone` | string | No | Phone number |
| `status` | string | No | `active`, `suspended`, `deleted` |

**Response (`200 OK`):** Returns the updated `Customer` object.

---

#### `DELETE /admin/customers/:id`

Delete a customer account. Customers with active VMs may be blocked.

**Response (`200 OK`):** Returns success confirmation.

---

#### `GET /admin/customers/:id/audit-logs`

Get audit log entries for a specific customer.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |

**Response (`200 OK`):** Returns paginated array of `AuditLog` entries.

---

### Admin API Audit Logs

#### `GET /admin/audit-logs`

List all audit log entries with filtering.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `actor_id` | string | No | Filter by actor UUID |
| `action` | string | No | Filter by action (e.g., `vm.create`, `node.update`) |
| `resource_type` | string | No | Filter by resource type (e.g., `vm`, `node`, `plan`) |

**Response (`200 OK`):** Returns paginated array of `AuditLog` entries.

Each audit log entry:

```json
{
  "id": "uuid",
  "timestamp": "2026-03-15T10:30:00Z",
  "actor_id": "uuid",
  "actor_type": "admin",
  "actor_ip": "192.168.1.100",
  "action": "vm.create",
  "resource_type": "vm",
  "resource_id": "uuid",
  "changes": { "hostname": "web-01", "plan_id": "uuid" },
  "correlation_id": "req_abc123",
  "success": true
}
```

---

### Admin API Settings

#### `GET /admin/settings`

Get all system settings as key-value pairs.

**Response (`200 OK`):**

```json
{
  "data": [
    { "key": "default_location", "value": "us-east-1", "description": "Default data center location" },
    { "key": "backup_retention_days", "value": "30", "description": "Default backup retention" }
  ]
}
```

---

#### `PUT /admin/settings/:key`

Update a system setting.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `key` | string | Setting key name |

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `value` | string | Yes | New setting value |

**Response (`200 OK`):** Returns the updated setting.

---

### Admin API Backups

#### `GET /admin/backups`

List all backups across all customers.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `vm_id` | string | No | Filter by VM UUID |
| `status` | string | No | Filter by status |

**Response (`200 OK`):** Returns paginated array of `Backup` objects.

---

#### `POST /admin/backups/:id/restore`

Restore a VM from a backup (admin override, can restore any customer's backup).

**Response (`202 Accepted`):** Returns task ID for the restore operation.

---

### Admin API Backup Schedules

#### `POST /admin/backup-schedules`

Create a backup schedule.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `vm_id` | string | Yes | Valid UUID |
| `frequency` | string | Yes | Cron expression (e.g., `0 2 * * *`) |
| `retention_count` | integer | Yes | Number of backups to retain |
| `active` | boolean | No | Whether schedule is active (default: true) |

**Response (`201 Created`):** Returns the created `BackupSchedule` object.

---

#### `GET /admin/backup-schedules`

List backup schedules.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `vm_id` | string | No | Filter by VM UUID |

**Response (`200 OK`):** Returns paginated array of `BackupSchedule` objects.

---

#### `GET /admin/backup-schedules/:id`

Get backup schedule details.

**Response (`200 OK`):** Returns `BackupSchedule` object.

---

#### `PUT /admin/backup-schedules/:id`

Update a backup schedule (partial update).

**Request Body:** Same fields as create, all optional.

**Response (`200 OK`):** Returns the updated `BackupSchedule` object.

---

#### `DELETE /admin/backup-schedules/:id`

Delete a backup schedule.

**Response (`200 OK`):** Returns success confirmation.

---

## Customer API

**Base Path:** `/api/v1/customer`  
**Authentication:** JWT (Bearer) with role `customer` (2FA optional)  
**CSRF:** Required for all write operations  
**Rate Limit:** 100 read/min, 30 write/min (plus per-action limits)

All endpoints enforce customer isolation — customers can only access their own resources.

---

### Customer API Authentication

#### `POST /customer/auth/login`

Authenticate a customer. If 2FA is enabled, a `temp_token` is returned.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `email` | string | Yes | Valid email, max 254 chars |
| `password` | string | Yes | Min 12, max 128 chars |

**Response (`200 OK`):** Same format as admin login (see Admin Authentication above).

---

#### `POST /customer/auth/verify-2fa`

Verify TOTP code (if 2FA is enabled).

**Request Body:** Same as admin 2FA verification.

**Response (`200 OK`):** Sets cookies and returns token info.

---

#### `POST /customer/auth/refresh`

Refresh the access token.

**Response (`200 OK`):** Updates cookies and returns token info.

---

#### `POST /customer/auth/logout`

Invalidate session and clear cookies.

**Response (`200 OK`):**

```json
{
  "data": { "message": "Logged out successfully" }
}
```

---

### Customer API Profile

#### `GET /customer/profile`

Get the authenticated customer's profile.

**Response (`200 OK`):** Returns `Customer` object.

---

#### `PUT /customer/profile`

Update the authenticated customer's profile.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `name` | string | No | Max 255 chars |
| `phone` | string | No | Phone number |

**Response (`200 OK`):** Returns updated `Customer` object.

---

### Customer API Password Management

#### `PUT /customer/password`

Change the authenticated customer's password.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `current_password` | string | Yes | Min 12, max 128 chars |
| `new_password` | string | Yes | Min 12, max 128 chars |

**Response (`200 OK`):**

```json
{
  "data": { "message": "Password updated successfully" }
}
```

**Error Responses:**
- `400` — `VALIDATION_ERROR`: Password too short
- `401` — `INVALID_CURRENT_PASSWORD`: Current password is incorrect
- `401` — `UNAUTHORIZED`: Not authenticated

---

### Customer API Two-Factor Authentication

#### `POST /customer/2fa/initiate`

Start the 2FA enrollment process. Returns a TOTP secret and QR code data.

**Response (`200 OK`):**

```json
{
  "data": {
    "secret": "JBSWY3DPEHPK3PXP",
    "qr_code_data": "otpauth://totp/VirtueStack:customer@example.com?secret=JBSWY3DPEHPK3PXP&issuer=VirtueStack"
  }
}
```

---

#### `POST /customer/2fa/enable`

Enable 2FA by verifying a TOTP code against the enrolled secret.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `totp_code` | string | Yes | 6 numeric digits |
| `password` | string | Yes | Current password for verification |

**Response (`200 OK`):** Returns backup recovery codes (shown only once):

```json
{
  "data": {
    "backup_codes": [
      "ABC123",
      "DEF456",
      "GHI789",
      "JKL012",
      "MNO345",
      "PQR678",
      "STU901",
      "VWX234"
    ]
  }
}
```

---

#### `POST /customer/2fa/disable`

Disable 2FA. Requires password verification.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `password` | string | Yes | Current password |

**Response (`200 OK`):** Returns confirmation message.

---

#### `GET /customer/2fa/status`

Check if 2FA is enabled for the account.

**Response (`200 OK`):**

```json
{
  "data": {
    "enabled": true,
    "backup_codes_configured": true
  }
}
```

---

#### `GET /customer/2fa/backup-codes`

Get backup recovery codes (only available if 2FA is enabled).

**Response (`200 OK`):** Returns array of backup code strings.

---

#### `POST /customer/2fa/backup-codes/regenerate`

Generate new backup recovery codes. Invalidates all previous codes.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `password` | string | Yes | Current password |

**Response (`200 OK`):** Returns new array of backup codes.

---

### Customer API Virtual Machines

#### `GET /customer/vms`

List VMs owned by the authenticated customer.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `status` | string | No | Filter by VM status |

**Response (`200 OK`):** Returns paginated array of `VM` objects.

---

#### `POST /customer/vms`

Create a new VM. Uses the authenticated customer's ID automatically.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `plan_id` | string | Yes | Valid UUID |
| `template_id` | string | Yes | Valid UUID |
| `hostname` | string | Yes | RFC 1123, max 63 chars |
| `password` | string | Yes | Min 8, max 128 chars |
| `ssh_keys` | string[] | No | Max 10 keys, max 4096 chars each |
| `location_id` | string | No | Valid UUID |

**Response (`202 Accepted`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "task_id": "uuid"
  }
}
```

---

#### `GET /customer/vms/:id`

Get detailed VM information. Enforces ownership check.

**Response (`200 OK`):** Returns `VMDetail` object.

**Error Responses:**
- `404` — `VM_NOT_FOUND`: VM does not exist or does not belong to customer

---

#### `DELETE /customer/vms/:id`

Delete a VM (async operation). Enforces ownership check.

**Response (`202 Accepted`):**

```json
{
  "data": {
    "task_id": "uuid"
  }
}
```

---

### Customer API VM Power Control

#### `POST /customer/vms/:id/start`

Start a stopped VM.

**Response (`200 OK`):** Returns VM status confirmation.

**Error Responses:**
- `409` — VM is already running or in an incompatible state

---

#### `POST /customer/vms/:id/stop`

Gracefully stop a running VM (ACPI shutdown).

**Response (`200 OK`):** Returns VM status confirmation.

---

#### `POST /customer/vms/:id/force-stop`

Force stop a VM (power cut). Use when graceful shutdown fails.

**Response (`200 OK`):** Returns VM status confirmation.

---

#### `POST /customer/vms/:id/restart`

Restart a running VM (graceful shutdown + start).

**Response (`200 OK`):** Returns VM status confirmation.

---

### Customer API VM Console Access

#### `POST /customer/vms/:id/console-token`

Generate a NoVNC access token for graphical (VNC) console access.

**Prerequisites:** VM must be running and have a node assigned.

**Response (`200 OK`):**

```json
{
  "data": {
    "token": "hex-encoded-32-byte-token",
    "url": "https://controller/vnc?token=hex-encoded-32-byte-token",
    "expires_at": "2026-03-15T11:30:00Z"
  }
}
```

Token is valid for 1 hour and is single-use.

**Error Responses:**
- `409` — `VM_NOT_RUNNING`: VM must be running to access console
- `409` — `VM_NO_NODE`: VM has no node assigned

---

#### `POST /customer/vms/:id/serial-token`

Generate a serial console access token (text-based, xterm.js).

**Prerequisites:** VM must be running and have a node assigned.

**Response (`200 OK`):**

```json
{
  "data": {
    "token": "hex-encoded-32-byte-token",
    "url": "https://controller/serial?token=hex-encoded-32-byte-token",
    "expires_at": "2026-03-15T11:30:00Z"
  }
}
```

Token is valid for 1 hour.

---

### Customer API VM Monitoring & Metrics

#### `GET /customer/vms/:id/metrics`

Get real-time resource metrics for a VM. Proxies to the node agent via gRPC.

**Response (`200 OK`):** Returns `VMMetrics` object:

```json
{
  "data": {
    "vm_id": "uuid",
    "cpu_usage_percent": 25.3,
    "memory_usage_bytes": 536870912,
    "memory_total_bytes": 1073741824,
    "disk_read_bytes": 104857600,
    "disk_write_bytes": 52428800,
    "network_rx_bytes": 10485760,
    "network_tx_bytes": 5242880,
    "uptime_seconds": 86400,
    "timestamp": "2026-03-15T10:30:00Z"
  }
}
```

---

#### `GET /customer/vms/:id/bandwidth`

Get bandwidth usage for the current billing period.

**Response (`200 OK`):** Returns `BandwidthResponse`:

```json
{
  "data": {
    "used_bytes": 10737418240,
    "limit_bytes": 53687091200,
    "reset_at": "2026-04-01T00:00:00Z",
    "percent_used": 20
  }
}
```

---

### Customer API VM Network

#### `GET /customer/vms/:id/network`

Get network interface information for a VM. Proxies to the node agent's guest agent.

**Response (`200 OK`):** Returns array of network interfaces with IP addresses, MAC addresses, and link state.

---

### Customer API VM ISO Management

#### `POST /customer/vms/:id/iso/upload`

Upload an ISO image to attach to a VM.

**Request:** `multipart/form-data`

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `file` | file | Yes | `.iso` file, max 10 GB |

**Additional constraints:**
- Max 5 ISO files per VM
- Only `.iso` files accepted
- File size limited to 10 GB

**Response (`200 OK`):**

```json
{
  "data": {
    "id": "uuid",
    "file_name": "custom-os.iso",
    "file_size": 536870912,
    "sha256": "abc123def456..."
  }
}
```

**Error Responses:**
- `400` — `MISSING_FILE`: No file in `file` form field
- `400` — `INVALID_FILE_TYPE`: Only `.iso` files allowed
- `400` — `FILE_TOO_LARGE`: ISO exceeds 10 GB limit
- `400` — `ISO_LIMIT_REACHED`: Max 5 ISOs per VM

---

#### `GET /customer/vms/:id/iso`

List uploaded ISO files for a VM.

**Response (`200 OK`):** Returns array of `ISORecord` objects.

---

#### `POST /customer/vms/:id/iso/:isoId/mount`

Mount an ISO to the VM's virtual CD-ROM drive. VM must be running.

**Response (`200 OK`):** Returns confirmation.

**Error Responses:**
- `409` — `VM_NOT_RUNNING`: VM must be running
- `400` — `ISO_ALREADY_MOUNTED`: An ISO is already attached

---

#### `POST /customer/vms/:id/iso/unmount`

Unmount the currently attached ISO.

**Response (`200 OK`):** Returns confirmation.

---

#### `DELETE /customer/vms/:id/iso/:isoId`

Delete an uploaded ISO file. Cannot delete a mounted ISO.

**Response (`200 OK`):** Returns confirmation.

---

### Customer API Backups

#### `GET /customer/backups`

List backups for the customer's VMs.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `vm_id` | string | No | Filter by VM UUID |
| `status` | string | No | Filter by status |

**Response (`200 OK`):** Returns paginated array of `Backup` objects.

---

#### `POST /customer/backups`

Create a backup for a VM.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `vm_id` | string | Yes | Valid UUID (must belong to customer) |
| `name` | string | No | Backup name, max 100 chars |

**Response (`202 Accepted`):** Returns `Backup` object.

---

#### `GET /customer/backups/:id`

Get backup details. Enforces ownership check.

**Response (`200 OK`):** Returns `Backup` object.

---

#### `DELETE /customer/backups/:id`

Delete a backup. Enforces ownership check.

**Response (`200 OK`):** Returns success confirmation.

---

#### `POST /customer/backups/:id/restore`

Restore a VM from a backup. The VM will be stopped during restore. Async operation.

**Response (`202 Accepted`):** Returns confirmation message.

---

### Customer API Snapshots

#### `GET /customer/snapshots`

List snapshots for the customer's VMs.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |
| `vm_id` | string | No | Filter by VM UUID |

**Response (`200 OK`):** Returns paginated array of `Snapshot` objects.

---

#### `POST /customer/snapshots`

Create a snapshot of a VM.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `vm_id` | string | Yes | Valid UUID (must belong to customer) |
| `name` | string | Yes | Max 100 chars |

**Response (`202 Accepted`):** Returns `Snapshot` object.

---

#### `DELETE /customer/snapshots/:id`

Delete a snapshot.

**Response (`202 Accepted`):** Returns task confirmation.

---

#### `POST /customer/snapshots/:id/restore`

Restore a VM to a snapshot. The VM will be stopped during restore.

**Response (`202 Accepted`):** Returns task confirmation.

---

### Customer API API Keys

#### `GET /customer/api-keys`

List all API keys for the authenticated customer.

**Response (`200 OK`):** Returns array of `APIKeyResponse` objects. Note: the `key` field is only included when creating or rotating a key.

---

#### `POST /customer/api-keys`

Create a new API key. **The raw key is only returned once.**

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `name` | string | Yes | Max 100 chars |
| `permissions` | string[] | Yes | Min 1 permission |
| `expires_at` | string | No | RFC 3339 timestamp (must be in the future) |

**Valid permissions:**

| Permission | Description |
|------------|-------------|
| `vm:read` | Read VM information |
| `vm:write` | Create/update/delete VMs |
| `vm:power` | Start/stop/restart VMs |
| `backup:read` | List and view backups |
| `backup:write` | Create/delete/restore backups |
| `snapshot:read` | List and view snapshots |
| `snapshot:write` | Create/delete/restore snapshots |

**Response (`201 Created`):**

```json
{
  "data": {
    "id": "uuid",
    "name": "My API Key",
    "permissions": ["vm:read", "vm:power"],
    "key": "vs_a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "is_active": true,
    "expires_at": "2027-03-15T00:00:00Z",
    "created_at": "2026-03-15T10:00:00Z"
  }
}
```

---

#### `POST /customer/api-keys/:id/rotate`

Rotate an API key. The old key is invalidated and a new key is generated.

**Response (`200 OK`):** Returns `APIKeyResponse` with the new `key`.

**Error Responses:**
- `400` — `KEY_REVOKED`: Cannot rotate a revoked key
- `400` — `KEY_EXPIRED`: Cannot rotate an expired key

---

#### `DELETE /customer/api-keys/:id`

Revoke an API key.

**Response (`200 OK`):**

```json
{
  "data": { "message": "API key revoked successfully" }
}
```

---

### Customer API Webhooks

#### `GET /customer/webhooks`

List all webhook endpoints for the customer.

**Response (`200 OK`):** Returns array of `CustomerWebhook` objects.

---

#### `POST /customer/webhooks`

Register a new webhook endpoint.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `url` | string | Yes | Valid URL, max 2048 chars |
| `secret` | string | Yes | Min 16, max 128 chars (HMAC signing secret) |
| `events` | string[] | Yes | Min 1 event type |

**Valid webhook events:**

| Event | Trigger |
|-------|---------|
| `vm.created` | VM provisioned successfully |
| `vm.deleted` | VM deleted |
| `vm.started` | VM started |
| `vm.stopped` | VM stopped |
| `vm.reinstalled` | VM reinstalled |
| `backup.completed` | Backup creation completed |

**Response (`201 Created`):** Returns `CustomerWebhook` object.

---

#### `GET /customer/webhooks/:id`

Get webhook details.

**Response (`200 OK`):** Returns `CustomerWebhook` object.

---

#### `PUT /customer/webhooks/:id`

Update a webhook (partial update).

**Request Body:** Same fields as create, all optional.

**Response (`200 OK`):** Returns updated `CustomerWebhook` object.

---

#### `DELETE /customer/webhooks/:id`

Delete a webhook endpoint.

**Response (`200 OK`):** Returns success confirmation.

---

#### `GET /customer/webhooks/:id/deliveries`

List delivery attempts for a webhook.

**Response (`200 OK`):** Returns array of `WebhookDelivery` objects:

```json
{
  "data": [
    {
      "id": "uuid",
      "webhook_id": "uuid",
      "event": "vm.created",
      "payload": "{ ... }",
      "attempt_count": 3,
      "response_status": 200,
      "response_body": "OK",
      "success": true,
      "delivered_at": "2026-03-15T10:30:00Z",
      "created_at": "2026-03-15T10:29:55Z"
    }
  ]
}
```

Failed deliveries are retried with exponential backoff.

---

### Customer API Templates

#### `GET /customer/templates`

List available OS templates (only active templates).

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | integer | No | Page number |
| `per_page` | integer | No | Items per page |

**Response (`200 OK`):** Returns paginated array of `Template` objects.

---

### Customer API Notifications

#### `GET /customer/notifications/preferences`

Get notification preferences for the customer.

**Response (`200 OK`):** Returns `NotificationPreferences` object:

```json
{
  "data": {
    "email_enabled": true,
    "webhook_enabled": false,
    "events": {
      "vm.created": { "email": true, "webhook": false },
      "vm.started": { "email": false, "webhook": true }
    }
  }
}
```

---

#### `PUT /customer/notifications/preferences`

Update notification preferences.

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email_enabled` | boolean | No | Global email notification toggle |
| `webhook_enabled` | boolean | No | Global webhook notification toggle |
| `events` | object | No | Per-event preferences |

**Response (`200 OK`):** Returns updated preferences.

---

### Customer API Reverse DNS (rDNS)

#### `GET /customer/vms/:id/rdns`

Get rDNS records for all IP addresses assigned to a VM.

**Response (`200 OK`):** Returns array of rDNS records with IP address and hostname.

---

#### `PUT /customer/vms/:id/rdns`

Set reverse DNS for an IP address on the VM. Updates database and PowerDNS (if configured).

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `ip_id` | string | Yes | Valid UUID of the IP address |
| `hostname` | string | Yes | RFC 1123, max 253 chars |

**Response (`200 OK`):** Returns updated rDNS record.

---

#### `DELETE /customer/vms/:id/rdns`

Remove reverse DNS for an IP address.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ip_id` | string | Yes | IP address UUID |

**Response (`200 OK`):** Returns success confirmation.

---

### Customer API WebSocket Connections

#### `GET /customer/ws/vnc/:vmId`

WebSocket endpoint for VNC console access. Requires a valid console token.

**Protocol:** `wss://`  
**Authentication:** Console token (from `POST /vms/:id/console-token`)  
**Message format:** Binary frames (VNC protocol proxied through WebSocket)

**Connection lifecycle:**
1. Obtain a console token via REST API
2. Connect to WebSocket with `?token=<token>`
3. VNC frames are bidirectionally proxied
4. Connection auto-closes after 1 hour (token expiry)

---

#### `GET /customer/ws/serial/:vmId`

WebSocket endpoint for serial console access. Requires a valid serial token.

**Protocol:** `wss://`  
**Authentication:** Serial token (from `POST /vms/:id/serial-token`)  
**Message format:** Text frames (terminal data)

**Connection lifecycle:**
1. Obtain a serial token via REST API
2. Connect to WebSocket with `?token=<token>`
3. Terminal data is bidirectionally proxied
4. Connection auto-closes after 1 hour (token expiry)

---

## Provisioning API

**Base Path:** `/api/v1/provisioning`  
**Authentication:** API Key via `X-API-Key` header  
**CSRF:** Not required (API key auth)  
**Rate Limit:** 1000 requests/minute

Designed for integration with billing systems like WHMCS. All VM operations use admin-level privileges.

---

### Provisioning API Virtual Machines

#### `POST /provisioning/vms`

Create a new VM asynchronously.

**Request Headers:**

| Header | Type | Required | Description |
|--------|------|----------|-------------|
| `X-API-Key` | string | Yes | Valid provisioning API key |
| `Idempotency-Key` | string | No | UUID for idempotent request deduplication |

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `customer_id` | string | Yes | Valid UUID |
| `plan_id` | string | Yes | Valid UUID |
| `template_id` | string | Yes | Valid UUID |
| `hostname` | string | Yes | RFC 1123, max 63 chars |
| `ssh_keys` | string[] | No | Max 10 keys, max 4096 chars each |
| `whmcs_service_id` | integer | Yes | WHMCS service identifier |
| `location_id` | string | No | Valid UUID |

A random password is auto-generated if SSH keys are provided. If neither password nor SSH keys are provided, a random password is generated and returned via the task result.

**Response (`202 Accepted`):**

```json
{
  "data": {
    "task_id": "uuid",
    "vm_id": "uuid",
    "storage_backend": "ceph"
  }
}
```

---

#### `GET /provisioning/vms/:id`

Get detailed VM information.

**Response (`200 OK`):** Returns `VMDetail` object.

---

#### `GET /provisioning/vms/by-service/:service_id`

Look up a VM by its WHMCS service ID.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `service_id` | integer | WHMCS service identifier |

**Response (`200 OK`):** Returns `VM` object.

**Error Responses:**
- `400` — `INVALID_SERVICE_ID`: Not a valid integer
- `404` — `VM_NOT_FOUND`: No VM with that service ID

---

#### `DELETE /provisioning/vms/:id`

Terminate (delete) a VM asynchronously.

**Response (`202 Accepted`):**

```json
{
  "data": {
    "task_id": "uuid"
  }
}
```

**Error Responses:**
- `410` — `VM_ALREADY_DELETED`: VM has already been terminated

---

#### `POST /provisioning/vms/:id/suspend`

Suspend a VM for billing purposes (e.g., non-payment). The VM is stopped and console access is blocked.

**Response (`200 OK`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "status": "suspended"
  }
}
```

If the VM is already suspended, returns the same response (idempotent).

---

#### `POST /provisioning/vms/:id/unsuspend`

Unsuspend a VM. Restores status to `stopped` — the customer must manually start the VM.

**Response (`200 OK`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "status": "stopped"
  }
}
```

---

#### `POST /provisioning/vms/:id/resize`

Resize VM resources (upgrade only for disk). Supports async disk resize.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `vcpu` | integer | No | Min 1 |
| `memory_mb` | integer | No | Min 512 |
| `disk_gb` | integer | No | Min current size (no shrinking) |

At least one field must be provided.

**Response:**
- `200 OK` — CPU/memory only change (instant):
  ```json
  {
    "data": {
      "vm_id": "uuid",
      "vcpu": 4,
      "memory_mb": 4096,
      "disk_gb": 50
    }
  }
  ```
- `202 Accepted` — Disk resize (async, returns task_id):
  ```json
  {
    "data": {
      "task_id": "uuid",
      "vm_id": "uuid",
      "vcpu": 2,
      "memory_mb": 2048,
      "disk_gb": 100
    }
  }
  ```

**Error Responses:**
- `400` — `VM_SUSPENDED`: Cannot resize a suspended VM
- `400` — `VALIDATION_ERROR`: Disk shrinking not supported

---

#### `POST /provisioning/vms/:id/password`

Set the root password for a VM.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `password` | string | Yes | Min 8, max 128 chars. Must contain uppercase, lowercase, digit, and special character |

**Response (`200 OK`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "message": "Password updated successfully"
  }
}
```

---

#### `POST /provisioning/vms/:id/password/reset`

Generate and set a new random password. Returns the plaintext password.

**Response (`200 OK`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "password": "VsAbCdEfGhIjKlMn",
    "message": "Password reset successfully"
  }
}
```

**Note:** The generated password starts with `Vs` prefix, is 16 characters, and includes alphanumeric characters. This is the only API that returns a plaintext password.

---

#### `POST /provisioning/vms/:id/power`

Perform power operations on a VM.

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `operation` | string | Yes | One of: `start`, `stop`, `restart` |

**Response (`200 OK`):**

```json
{
  "data": {
    "vm_id": "uuid",
    "operation": "start",
    "message": "Power operation completed successfully"
  }
}
```

**Error Responses:**
- `400` — `VM_SUSPENDED`: Cannot operate on suspended VM (except `start`)

---

#### `GET /provisioning/vms/:id/rdns`

Get reverse DNS records for all IPs assigned to a VM.

**Response (`200 OK`):** Returns array of `{ ip_address, rdns_hostname }` objects.

---

#### `PUT /provisioning/vms/:id/rdns`

Set reverse DNS for a specific IP on a VM.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ip_id` | string | Yes | IP address UUID |

**Request Body:**

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `hostname` | string | Yes | RFC 1123, max 253 chars |

**Response (`200 OK`):** Returns `{ ip_address, rdns_hostname }`.

---

### Provisioning API VM Status

#### `GET /provisioning/vms/:id/status`

Get the current status of a VM. For running VMs, attempts to fetch live status from the node agent.

**Response (`200 OK`):**

```json
{
  "data": {
    "status": "running",
    "node_id": "uuid"
  }
}
```

**Possible status values:** `provisioning`, `running`, `stopped`, `suspended`, `migrating`, `reinstalling`, `error`, `deleted`

---

### Provisioning API Task Polling

#### `GET /provisioning/tasks/:id`

Get the status of an async task. Used by WHMCS to poll long-running operations.

**Response (`200 OK`):**

```json
{
  "data": {
    "id": "uuid",
    "type": "vm.create",
    "status": "running",
    "progress": 60,
    "message": "Uploading cloud-init ISO...",
    "created_at": "2026-03-15T10:00:00Z"
  }
}
```

**Task states:**

| Status | Description |
|--------|-------------|
| `pending` | Task queued, not yet started |
| `running` | Task in progress |
| `completed` | Task finished successfully |
| `failed` | Task failed (check `message` for details) |
| `cancelled` | Task was cancelled |

**Task types and their progress messages:**

| Type | Description | Progress Steps |
|------|-------------|----------------|
| `vm.create` | VM provisioning | Validating → Allocating IPs → Cloning disk → Resizing → Cloud-init → Creating VM → Starting → Verifying |
| `vm.delete` | VM termination | Stopping → Deleting disk → Releasing IPs → Removing VM |
| `vm.reinstall` | OS reinstallation | Stopping → Deleting disk → Cloning fresh disk → Cloud-init → Starting |
| `vm.resize` | Resource resize | Processing... |
| `backup.create` | Backup creation | Creating backup... |
| `backup.restore` | Backup restoration | Restoring from backup... |
| `snapshot.create` | Snapshot creation | Validating → Creating snapshot → Updating record |
| `snapshot.revert` | Snapshot restore | Stopping → Restoring → Starting |
| `snapshot.delete` | Snapshot deletion | Deleting from storage → Removing record |

When a task is `completed`, the `result` field contains the operation result (e.g., VM details, backup ID).

---

## Data Models Reference

### VM (Virtual Machine)

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `customer_id` | string (UUID) | Owner customer |
| `node_id` | string (UUID) | Assigned compute node (null during provisioning) |
| `plan_id` | string (UUID) | Service plan |
| `hostname` | string | RFC 1123 hostname |
| `status` | string | Current state |
| `vcpu` | integer | Virtual CPUs |
| `memory_mb` | integer | RAM in megabytes |
| `disk_gb` | integer | Disk size in GB |
| `port_speed_mbps` | integer | Network port speed |
| `bandwidth_limit_gb` | integer | Monthly bandwidth cap |
| `bandwidth_used_bytes` | integer | Current period usage |
| `bandwidth_reset_at` | timestamp | Next bandwidth reset |
| `mac_address` | string | Primary NIC MAC address |
| `template_id` | string (UUID) | OS template used |
| `storage_backend` | string | `ceph` or `qcow` (immutable after creation) |
| `disk_path` | string | QCOW file path (QCOW backend only) |
| `ceph_pool` | string | Ceph pool name (Ceph backend) |
| `rbd_image` | string | RBD image name (Ceph backend) |
| `attached_iso` | string (UUID) | Currently mounted ISO |
| `whmcs_service_id` | integer | WHMCS service reference |
| `created_at` | timestamp | Creation time |
| `updated_at` | timestamp | Last update time |
| `deleted_at` | timestamp | Soft delete time (null if active) |

**VM Status Values:** `provisioning`, `running`, `stopped`, `suspended`, `migrating`, `reinstalling`, `error`, `deleted`

### VMDetail (Extended VM)

Extends `VM` with:

| Field | Type | Description |
|-------|------|-------------|
| `ip_addresses` | IPAddress[] | Assigned IP addresses |
| `ipv6_subnets` | VMIPv6Subnet[] | IPv6 /64 subnets |
| `node_hostname` | string | Node hostname |
| `plan_name` | string | Plan display name |
| `template_name` | string | Template display name |

### Node (Hypervisor)

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `hostname` | string | Node hostname |
| `grpc_address` | string | gRPC listener address |
| `management_ip` | string | Management network IP |
| `location_id` | string (UUID) | Data center location |
| `status` | string | Operational state |
| `total_vcpu` | integer | Total CPU cores |
| `total_memory_mb` | integer | Total RAM |
| `allocated_vcpu` | integer | Allocated to VMs |
| `allocated_memory_mb` | integer | Allocated to VMs |
| `storage_backend` | string | `ceph` or `qcow` |
| `storage_path` | string | QCOW base path |
| `last_heartbeat_at` | timestamp | Last health report |
| `consecutive_heartbeat_misses` | integer | Missed heartbeat count |
| `created_at` | timestamp | Registration time |

**Node Status Values:** `online`, `degraded`, `offline`, `draining`, `failed`

### Plan (Service Plan)

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `name` | string | Display name |
| `slug` | string | URL-friendly identifier |
| `vcpu` | integer | Virtual CPUs |
| `memory_mb` | integer | RAM in MB |
| `disk_gb` | integer | Disk in GB |
| `bandwidth_limit_gb` | integer | Monthly bandwidth |
| `port_speed_mbps` | integer | Port speed |
| `price_monthly` | integer | Monthly price (cents) |
| `price_hourly` | integer | Hourly price (cents) |
| `storage_backend` | string | Default storage backend |
| `is_active` | boolean | Available for new VMs |
| `sort_order` | integer | Display ordering |

### Template (OS Image)

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `name` | string | Display name |
| `os_family` | string | OS family (e.g., `ubuntu`) |
| `os_version` | string | OS version (e.g., `24.04`) |
| `rbd_image` | string | Ceph RBD image name |
| `rbd_snapshot` | string | Ceph RBD snapshot name |
| `min_disk_gb` | integer | Minimum disk requirement |
| `supports_cloudinit` | boolean | Cloud-init support |
| `is_active` | boolean | Available for provisioning |
| `version` | integer | Template version (incremented on update) |
| `storage_backend` | string | `ceph` or `qcow` |
| `file_path` | string | QCOW template file path |
| `description` | string | Description |

### IPSet (IP Pool)

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `name` | string | Pool name |
| `location_id` | string (UUID) | Data center location |
| `network` | string | CIDR notation |
| `gateway` | string | Gateway IP |
| `vlan_id` | integer | VLAN ID |
| `ip_version` | integer | `4` or `6` |
| `node_ids` | string[] | Associated node UUIDs |

### IPAddress

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `ip_set_id` | string (UUID) | Parent IP set |
| `address` | string | IP address |
| `ip_version` | integer | `4` or `6` |
| `vm_id` | string (UUID) | Assigned VM |
| `customer_id` | string (UUID) | Owner customer |
| `is_primary` | boolean | Primary address of VM |
| `rdns_hostname` | string | Reverse DNS hostname |
| `status` | string | Allocation status |
| `assigned_at` | timestamp | Assignment time |
| `cooldown_until` | timestamp | IP cooldown expiry |

**IP Status Values:** `available`, `assigned`, `reserved`, `cooldown`

### Customer

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `email` | string | Email address |
| `name` | string | Display name |
| `phone` | string | Phone number |
| `whmcs_client_id` | integer | WHMCS client reference |
| `totp_enabled` | boolean | 2FA enabled |
| `status` | string | Account status |
| `created_at` | timestamp | Registration time |
| `updated_at` | timestamp | Last update time |

**Customer Status Values:** `active`, `suspended`, `deleted`

### Backup

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `vm_id` | string (UUID) | Source VM |
| `type` | string | `full` or `incremental` |
| `storage_backend` | string | `ceph` or `qcow` |
| `rbd_snapshot` | string | Ceph snapshot name |
| `file_path` | string | QCOW backup file path |
| `size_bytes` | integer | Backup size |
| `status` | string | Backup status |
| `created_at` | timestamp | Creation time |
| `expires_at` | timestamp | Expiration time |

**Backup Status Values:** `creating`, `completed`, `failed`, `restoring`, `deleted`

### Snapshot

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `vm_id` | string (UUID) | Source VM |
| `name` | string | Snapshot name |
| `storage_backend` | string | `ceph` or `qcow` |
| `rbd_snapshot` | string | Ceph snapshot name |
| `qcow_snapshot` | string | QCOW snapshot name |
| `size_bytes` | integer | Snapshot size |
| `created_at` | timestamp | Creation time |

### Task

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `type` | string | Task type |
| `status` | string | Task status |
| `progress` | integer | Progress percentage (0-100) |
| `error_message` | string | Error details (if failed) |
| `result` | object | Result data (if completed) |
| `idempotency_key` | string | Deduplication key |
| `created_by` | string | Creator user ID |
| `created_at` | timestamp | Creation time |
| `started_at` | timestamp | Start time |
| `completed_at` | timestamp | Completion time |

### FailoverRequest

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `node_id` | string (UUID) | Affected node |
| `requested_by` | string | Admin who initiated |
| `status` | string | Failover status |
| `reason` | string | Reason for failover |
| `result` | object | Failover result details |
| `approved_at` | timestamp | Approval time |
| `completed_at` | timestamp | Completion time |

**Failover Status Values:** `pending`, `approved`, `in_progress`, `completed`, `failed`, `cancelled`

### BackupSchedule

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `vm_id` | string (UUID) | Target VM |
| `customer_id` | string (UUID) | Owner customer |
| `frequency` | string | Cron expression |
| `retention_count` | integer | Max backups to keep |
| `active` | boolean | Schedule enabled |
| `next_run_at` | timestamp | Next execution time |

### AuditLog

| Field | Type | Description |
|-------|------|-------------|
| `id` | string (UUID) | Unique identifier |
| `timestamp` | timestamp | Event time |
| `actor_id` | string (UUID) | User who performed the action |
| `actor_type` | string | `admin`, `customer`, `provisioning`, `system` |
| `actor_ip` | string | Source IP address |
| `action` | string | Action performed (e.g., `vm.create`) |
| `resource_type` | string | Resource type (e.g., `vm`, `node`) |
| `resource_id` | string (UUID) | Resource identifier |
| `changes` | object | Changes made (JSON) |
| `correlation_id` | string | Request correlation ID |
| `success` | boolean | Whether the action succeeded |

---

## WebSocket Protocol

### VNC Console (`wss://<host>/api/v1/customer/ws/vnc/:vmId`)

| Aspect | Detail |
|--------|--------|
| Protocol | WebSocket over TLS (`wss://`) |
| Authentication | Console token via `?token=` query parameter |
| Frame Type | Binary |
| Direction | Bidirectional (client ↔ server) |
| Timeout | 1 hour (token expiry) |
| Proxy | Controller proxies to Node Agent gRPC `StreamVNCConsole` |

### Serial Console (`wss://<host>/api/v1/customer/ws/serial/:vmId`)

| Aspect | Detail |
|--------|--------|
| Protocol | WebSocket over TLS (`wss://`) |
| Authentication | Serial token via `?token=` query parameter |
| Frame Type | Text |
| Direction | Bidirectional (client ↔ server) |
| Timeout | 1 hour (token expiry) |
| Proxy | Controller proxies to Node Agent gRPC `StreamSerialConsole` |

---

*This document is auto-generated from the VirtueStack codebase. For architecture details, see [ARCHITECTURE.md](ARCHITECTURE.md). For coding standards, see [CODING_STANDARD.md](CODING_STANDARD.md).*
