# VirtueStack API Reference

Complete API reference for VirtueStack Phase 6. The API provides RESTful endpoints for VM management, billing integration, and customer self-service.

---

## Table of Contents

1. [Overview](#overview)
2. [Authentication](#authentication)
3. [Rate Limiting](#rate-limiting)
4. [Error Handling](#error-handling)
5. [Admin API](#admin-api)
6. [Customer API](#customer-api)
7. [Provisioning API](#provisioning-api)
8. [Webhooks](#webhooks)

---

## Overview

### Base URL

```
https://your-domain.com/api/v1
```

### API Versions

Current version: `v1`

All endpoints are prefixed with `/api/v1`.

### Content Type

All requests and responses use JSON:

```
Content-Type: application/json
```

### HTTP Methods

| Method | Usage |
|--------|-------|
| `GET` | Retrieve resources |
| `POST` | Create resources or perform actions |
| `PUT` | Update resources |
| `DELETE` | Remove resources |

---

## Authentication

### JWT Authentication

Most API endpoints require JWT Bearer token authentication.

#### Obtaining a Token

**Admin Login**:
```http
POST /api/v1/admin/auth/login
Content-Type: application/json

{
  "email": "admin@example.com",
  "password": "your-password"
}
```

**Customer Login**:
```http
POST /api/v1/customer/auth/login
Content-Type: application/json

{
  "email": "customer@example.com",
  "password": "your-password"
}
```

**Response** (without 2FA):
```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 3600
  }
}
```

**Response** (with 2FA enabled):
```json
{
  "data": {
    "requires_2fa": true,
    "temp_token": "temp-verification-token",
    "token_type": "Bearer"
  }
}
```

#### Using the Token

Include the token in the Authorization header:

```http
GET /api/v1/customer/vms
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

#### Refreshing Tokens

```http
POST /api/v1/customer/auth/refresh
Content-Type: application/json

{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**Response**:
```json
{
  "data": {
    "access_token": "new-access-token",
    "refresh_token": "new-refresh-token",
    "token_type": "Bearer",
    "expires_in": 3600
  }
}
```

#### 2FA Verification

If the account has 2FA enabled:

```http
POST /api/v1/customer/auth/verify-2fa
Content-Type: application/json

{
  "temp_token": "temp-verification-token",
  "totp_code": "123456"
}
```

**Response**:
```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 3600
  }
}
```

### API Key Authentication

The Provisioning API uses API key authentication:

```http
POST /api/v1/provisioning/vms
X-API-Key: your-provisioning-api-key
Content-Type: application/json
```

---

## Rate Limiting

### Limits

| API Type | Requests/Minute | Burst |
|----------|----------------|-------|
| Admin API | 300 | 50 |
| Customer API | 120 | 30 |
| Provisioning API | 600 | 100 |

### Headers

Rate limit information is included in response headers:

```
X-RateLimit-Limit: 120
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1704067200
```

### Rate Limited Response

```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Rate limit exceeded. Retry after 60 seconds.",
    "correlation_id": "abc-123-def"
  }
}
```

---

## Error Handling

### Error Response Format

All errors follow a consistent format:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "correlation_id": "unique-request-id"
  }
}
```

### HTTP Status Codes

| Status | Meaning |
|--------|---------|
| `200` | Success |
| `201` | Created |
| `202` | Accepted (async operation) |
| `204` | No Content (success with no body) |
| `400` | Bad Request (validation error) |
| `401` | Unauthorized (invalid/missing token) |
| `403` | Forbidden (insufficient permissions) |
| `404` | Not Found |
| `409` | Conflict (invalid state) |
| `422` | Unprocessable Entity |
| `429` | Rate Limited |
| `500` | Internal Server Error |
| `503` | Service Unavailable |

### Common Error Codes

| Code | Description |
|------|-------------|
| `VALIDATION_ERROR` | Request validation failed |
| `INVALID_CREDENTIALS` | Invalid email or password |
| `INVALID_TOKEN` | JWT token is invalid or expired |
| `INVALID_2FA_CODE` | TOTP code is invalid |
| `VM_NOT_FOUND` | VM does not exist |
| `NODE_NOT_FOUND` | Node does not exist |
| `INVALID_VM_STATE` | VM is in an invalid state for this operation |
| `INSUFFICIENT_RESOURCES` | Not enough resources available |
| `RATE_LIMIT_EXCEEDED` | Too many requests |

---

## Admin API

Base path: `/api/v1/admin`

Requires: JWT Bearer token with `admin` role

### Authentication

#### Login

```http
POST /api/v1/admin/auth/login
```

**Request**:
```json
{
  "email": "admin@example.com",
  "password": "secure-password"
}
```

**Response**:
```json
{
  "data": {
    "access_token": "eyJhbGci...",
    "refresh_token": "eyJhbGci...",
    "token_type": "Bearer",
    "expires_in": 3600
  }
}
```

### Nodes

#### List Nodes

```http
GET /api/v1/admin/nodes
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number (default: 1) |
| `per_page` | int | Items per page (default: 20, max: 100) |
| `status` | string | Filter by status |
| `location_id` | string | Filter by location |

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "hostname": "node1.example.com",
      "grpc_address": "node1.example.com:50051",
      "management_ip": "192.168.1.10",
      "status": "online",
      "total_vcpu": 32,
      "total_memory_mb": 65536,
      "allocated_vcpu": 24,
      "allocated_memory_mb": 49152,
      "ceph_pool": "vs-vms",
      "last_heartbeat_at": "2024-01-15T10:30:00Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 5,
    "total_pages": 1
  }
}
```

#### Register Node

```http
POST /api/v1/admin/nodes
```

**Request**:
```json
{
  "hostname": "node2.example.com",
  "grpc_address": "node2.example.com:50051",
  "management_ip": "192.168.1.11",
  "location_id": "us-east-1",
  "total_vcpu": 32,
  "total_memory_mb": 65536,
  "ceph_pool": "vs-vms",
  "ipmi_address": "192.168.2.11",
  "ipmi_username": "admin",
  "ipmi_password": "ipmi-password"
}
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "hostname": "node2.example.com",
    "status": "offline",
    "created_at": "2024-01-15T10:35:00Z"
  }
}
```

#### Get Node

```http
GET /api/v1/admin/nodes/:id
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "hostname": "node1.example.com",
    "grpc_address": "node1.example.com:50051",
    "management_ip": "192.168.1.10",
    "location_id": "us-east-1",
    "status": "online",
    "total_vcpu": 32,
    "total_memory_mb": 65536,
    "allocated_vcpu": 24,
    "allocated_memory_mb": 49152,
    "ceph_pool": "vs-vms",
    "consecutive_heartbeat_misses": 0,
    "last_heartbeat_at": "2024-01-15T10:30:00Z",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

#### Update Node

```http
PUT /api/v1/admin/nodes/:id
```

**Request**:
```json
{
  "hostname": "node1-renamed.example.com",
  "total_vcpu": 64
}
```

#### Delete Node

```http
DELETE /api/v1/admin/nodes/:id
```

**Response**: `204 No Content`

#### Drain Node

```http
POST /api/v1/admin/nodes/:id/drain
```

**Request**:
```json
{
  "enabled": true
}
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "draining"
  }
}
```

#### Failover Node

```http
POST /api/v1/admin/nodes/:id/failover
```

### VMs

#### List VMs

```http
GET /api/v1/admin/vms
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number |
| `per_page` | int | Items per page |
| `customer_id` | string | Filter by customer |
| `node_id` | string | Filter by node |
| `status` | string | Filter by status |
| `search` | string | Search by hostname |

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "customer_id": "550e8400-e29b-41d4-a716-446655440003",
      "node_id": "550e8400-e29b-41d4-a716-446655440000",
      "plan_id": "550e8400-e29b-41d4-a716-446655440004",
      "hostname": "my-vps",
      "status": "running",
      "vcpu": 2,
      "memory_mb": 2048,
      "disk_gb": 40,
      "port_speed_mbps": 1000,
      "bandwidth_limit_gb": 1000,
      "bandwidth_used_bytes": 53687091200,
      "mac_address": "52:54:00:ab:cd:ef",
      "created_at": "2024-01-10T08:00:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 150,
    "total_pages": 8
  }
}
```

#### Create VM

```http
POST /api/v1/admin/vms
```

**Request**:
```json
{
  "customer_id": "550e8400-e29b-41d4-a716-446655440003",
  "plan_id": "550e8400-e29b-41d4-a716-446655440004",
  "template_id": "550e8400-e29b-41d4-a716-446655440005",
  "hostname": "customer-vps",
  "password": "SecurePassword123!",
  "ssh_keys": [
    "ssh-rsa AAAAB3NzaC1yc2E... user@example.com"
  ],
  "location_id": "us-east-1",
  "node_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Response** (202 Accepted):
```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "task_id": "task-550e8400-e29b-41d4-a716-446655440006"
  }
}
```

#### Get VM

```http
GET /api/v1/admin/vms/:id
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "customer_id": "550e8400-e29b-41d4-a716-446655440003",
    "node_id": "550e8400-e29b-41d4-a716-446655440000",
    "plan_id": "550e8400-e29b-41d4-a716-446655440004",
    "hostname": "my-vps",
    "status": "running",
    "vcpu": 2,
    "memory_mb": 2048,
    "disk_gb": 40,
    "port_speed_mbps": 1000,
    "bandwidth_limit_gb": 1000,
    "bandwidth_used_bytes": 53687091200,
    "bandwidth_reset_at": "2024-02-01T00:00:00Z",
    "mac_address": "52:54:00:ab:cd:ef",
    "template_id": "550e8400-e29b-41d4-a716-446655440005",
    "template_name": "Ubuntu 22.04 LTS",
    "node_hostname": "node1.example.com",
    "plan_name": "Starter VPS",
    "ip_addresses": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440007",
        "address": "192.168.1.100",
        "type": "ipv4",
        "primary": true
      }
    ],
    "created_at": "2024-01-10T08:00:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

#### Update VM

```http
PUT /api/v1/admin/vms/:id
```

**Request**:
```json
{
  "hostname": "renamed-vps",
  "vcpu": 4,
  "memory_mb": 4096,
  "disk_gb": 80
}
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "hostname": "renamed-vps",
    "vcpu": 4,
    "memory_mb": 4096,
    "disk_gb": 80
  }
}
```

#### Delete VM

```http
DELETE /api/v1/admin/vms/:id
```

**Response** (202 Accepted):
```json
{
  "data": {
    "task_id": "task-550e8400-e29b-41d4-a716-446655440008"
  }
}
```

#### Migrate VM

```http
POST /api/v1/admin/vms/:id/migrate
```

**Request**:
```json
{
  "target_node_id": "550e8400-e29b-41d4-a716-446655440001"
}
```

**Response** (202 Accepted):
```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "target_node_id": "550e8400-e29b-41d4-a716-446655440001",
    "status": "migration_initiated"
  }
}
```

### Plans

#### List Plans

```http
GET /api/v1/admin/plans
```

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440004",
      "name": "Starter VPS",
      "slug": "starter-vps",
      "vcpu": 1,
      "memory_mb": 1024,
      "disk_gb": 20,
      "bandwidth_limit_gb": 1000,
      "port_speed_mbps": 1000,
      "price_monthly": 5.00,
      "price_hourly": 0.007,
      "is_active": true,
      "sort_order": 1,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Create Plan

```http
POST /api/v1/admin/plans
```

**Request**:
```json
{
  "name": "Professional VPS",
  "slug": "pro-vps",
  "vcpu": 4,
  "memory_mb": 8192,
  "disk_gb": 100,
  "bandwidth_limit_gb": 5000,
  "port_speed_mbps": 10000,
  "price_monthly": 40.00,
  "price_hourly": 0.055,
  "is_active": true,
  "sort_order": 2
}
```

#### Update Plan

```http
PUT /api/v1/admin/plans/:id
```

#### Delete Plan

```http
DELETE /api/v1/admin/plans/:id
```

### Templates

#### List Templates

```http
GET /api/v1/admin/templates
```

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440005",
      "name": "Ubuntu 22.04 LTS",
      "slug": "ubuntu-22-04",
      "os_type": "linux",
      "image_path": "/templates/ubuntu-22.04.qcow2",
      "min_disk_gb": 10,
      "default_username": "ubuntu",
      "cloud_init_compatible": true,
      "is_active": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Create Template

```http
POST /api/v1/admin/templates
```

**Request**:
```json
{
  "name": "Debian 12",
  "slug": "debian-12",
  "os_type": "linux",
  "image_path": "/templates/debian-12.qcow2",
  "min_disk_gb": 10,
  "default_username": "debian",
  "cloud_init_compatible": true,
  "is_active": true
}
```

#### Update Template

```http
PUT /api/v1/admin/templates/:id
```

#### Delete Template

```http
DELETE /api/v1/admin/templates/:id
```

#### Import Template

```http
POST /api/v1/admin/templates/:id/import
```

### IP Sets

#### List IP Sets

```http
GET /api/v1/admin/ip-sets
```

#### Create IP Set

```http
POST /api/v1/admin/ip-sets
```

**Request**:
```json
{
  "name": "US-East IPv4 Pool",
  "location_id": "us-east-1",
  "type": "ipv4",
  "cidr": "192.168.1.0/24",
  "gateway": "192.168.1.1",
  "nameservers": ["8.8.8.8", "8.8.4.4"]
}
```

#### Get IP Set

```http
GET /api/v1/admin/ip-sets/:id
```

#### Update IP Set

```http
PUT /api/v1/admin/ip-sets/:id
```

#### Delete IP Set

```http
DELETE /api/v1/admin/ip-sets/:id
```

#### List Available IPs

```http
GET /api/v1/admin/ip-sets/:id/available
```

### Customers

#### List Customers

```http
GET /api/v1/admin/customers
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number |
| `per_page` | int | Items per page |
| `search` | string | Search by email or name |
| `status` | string | Filter by status |

#### Get Customer

```http
GET /api/v1/admin/customers/:id
```

#### Update Customer

```http
PUT /api/v1/admin/customers/:id
```

#### Delete Customer

```http
DELETE /api/v1/admin/customers/:id
```

#### Get Customer Audit Logs

```http
GET /api/v1/admin/customers/:id/audit-logs
```

### Backups

#### List Backups

```http
GET /api/v1/admin/backups
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number |
| `per_page` | int | Items per page |
| `vm_id` | string | Filter by VM |
| `status` | string | Filter by status |

#### Restore Backup

```http
POST /api/v1/admin/backups/:id/restore
```

### Audit Logs

#### List Audit Logs

```http
GET /api/v1/admin/audit-logs
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number |
| `per_page` | int | Items per page |
| `action` | string | Filter by action type |
| `resource_type` | string | Filter by resource type |
| `actor_id` | string | Filter by actor |
| `from` | string | Start date (ISO 8601) |
| `to` | string | End date (ISO 8601) |

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440009",
      "action": "vm.create",
      "resource_type": "vm",
      "resource_id": "550e8400-e29b-41d4-a716-446655440002",
      "actor_id": "550e8400-e29b-41d4-a716-446655440010",
      "actor_email": "admin@example.com",
      "changes": {
        "hostname": "my-vps",
        "plan_id": "550e8400-e29b-41d4-a716-446655440004"
      },
      "ip_address": "192.168.1.50",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 50,
    "total": 1250,
    "total_pages": 25
  }
}
```

### Settings

#### Get Settings

```http
GET /api/v1/admin/settings
```

#### Update Setting

```http
PUT /api/v1/admin/settings/:key
```

**Request**:
```json
{
  "value": "new-value"
}
```

---

## Customer API

Base path: `/api/v1/customer`

Requires: JWT Bearer token with `customer` user type

### Authentication

#### Login

```http
POST /api/v1/customer/auth/login
```

**Request**:
```json
{
  "email": "customer@example.com",
  "password": "secure-password"
}
```

**Response** (without 2FA):
```json
{
  "data": {
    "access_token": "eyJhbGci...",
    "refresh_token": "eyJhbGci...",
    "token_type": "Bearer",
    "expires_in": 3600
  }
}
```

**Response** (with 2FA):
```json
{
  "data": {
    "requires_2fa": true,
    "temp_token": "temp-token",
    "token_type": "Bearer"
  }
}
```

#### Verify 2FA

```http
POST /api/v1/customer/auth/verify-2fa
```

**Request**:
```json
{
  "temp_token": "temp-token",
  "totp_code": "123456"
}
```

#### Refresh Token

```http
POST /api/v1/customer/auth/refresh
```

**Request**:
```json
{
  "refresh_token": "eyJhbGci..."
}
```

#### Logout

```http
POST /api/v1/customer/auth/logout
Authorization: Bearer <access_token>
```

**Request**:
```json
{
  "refresh_token": "eyJhbGci..."
}
```

### VMs

#### List VMs

```http
GET /api/v1/customer/vms
Authorization: Bearer <token>
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number |
| `per_page` | int | Items per page |
| `status` | string | Filter by status |
| `search` | string | Search by hostname |

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "hostname": "my-vps",
      "status": "running",
      "vcpu": 2,
      "memory_mb": 2048,
      "disk_gb": 40,
      "plan_name": "Starter VPS",
      "ip_addresses": [
        {
          "address": "192.168.1.100",
          "type": "ipv4",
          "primary": true
        }
      ],
      "created_at": "2024-01-10T08:00:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total": 3,
    "total_pages": 1
  }
}
```

#### Create VM

```http
POST /api/v1/customer/vms
Authorization: Bearer <token>
Idempotency-Key: unique-request-id
```

**Request**:
```json
{
  "plan_id": "550e8400-e29b-41d4-a716-446655440004",
  "template_id": "550e8400-e29b-41d4-a716-446655440005",
  "hostname": "my-new-vps",
  "password": "SecurePassword123!",
  "ssh_keys": [
    "ssh-rsa AAAAB3NzaC1yc2E... user@example.com"
  ]
}
```

**Response** (202 Accepted):
```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "task_id": "task-550e8400-e29b-41d4-a716-446655440006"
  }
}
```

#### Get VM

```http
GET /api/v1/customer/vms/:id
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "hostname": "my-vps",
    "status": "running",
    "vcpu": 2,
    "memory_mb": 2048,
    "disk_gb": 40,
    "port_speed_mbps": 1000,
    "bandwidth_limit_gb": 1000,
    "bandwidth_used_bytes": 53687091200,
    "bandwidth_reset_at": "2024-02-01T00:00:00Z",
    "plan_name": "Starter VPS",
    "template_name": "Ubuntu 22.04 LTS",
    "ip_addresses": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440007",
        "address": "192.168.1.100",
        "type": "ipv4",
        "primary": true
      }
    ],
    "created_at": "2024-01-10T08:00:00Z"
  }
}
```

#### Delete VM

```http
DELETE /api/v1/customer/vms/:id
Authorization: Bearer <token>
```

**Response** (202 Accepted):
```json
{
  "data": {
    "task_id": "task-550e8400-e29b-41d4-a716-446655440008"
  }
}
```

### Power Control

#### Start VM

```http
POST /api/v1/customer/vms/:id/start
Authorization: Bearer <token>
```

**Response**:
```json
{
  "message": "VM started successfully"
}
```

#### Stop VM (Graceful)

```http
POST /api/v1/customer/vms/:id/stop
Authorization: Bearer <token>
```

**Response**:
```json
{
  "message": "VM stopped successfully"
}
```

#### Restart VM

```http
POST /api/v1/customer/vms/:id/restart
Authorization: Bearer <token>
```

#### Force Stop VM

```http
POST /api/v1/customer/vms/:id/force-stop
Authorization: Bearer <token>
```

### Console Access

#### Get NoVNC Token

```http
POST /api/v1/customer/vms/:id/console-token
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": {
    "token": "console-token-here",
    "url": "wss://your-domain.com/console?token=console-token-here",
    "expires_at": "2024-01-15T11:30:00Z"
  }
}
```

#### Get Serial Console Token

```http
POST /api/v1/customer/vms/:id/serial-token
Authorization: Bearer <token>
```

### Monitoring

#### Get Metrics

```http
GET /api/v1/customer/vms/:id/metrics
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "cpu_usage_percent": 15.5,
    "memory_usage_bytes": 1073741824,
    "memory_total_bytes": 2147483648,
    "disk_read_bytes": 10737418240,
    "disk_write_bytes": 5368709120,
    "network_rx_bytes": 107374182400,
    "network_tx_bytes": 53687091200,
    "uptime_seconds": 432000,
    "timestamp": "2024-01-15T10:30:00Z"
  }
}
```

#### Get Bandwidth Usage

```http
GET /api/v1/customer/vms/:id/bandwidth
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "limit_gb": 1000,
    "used_gb": 50,
    "remaining_gb": 950,
    "reset_at": "2024-02-01T00:00:00Z"
  }
}
```

#### Get Network History

```http
GET /api/v1/customer/vms/:id/network
Authorization: Bearer <token>
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `period` | string | `hour`, `day`, `week`, `month` |

### Backups

#### List Backups

```http
GET /api/v1/customer/backups
Authorization: Bearer <token>
```

**Query Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number |
| `per_page` | int | Items per page |
| `vm_id` | string | Filter by VM |
| `status` | string | Filter by status |

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-44665544000b",
      "vm_id": "550e8400-e29b-41d4-a716-446655440002",
      "type": "full",
      "size_bytes": 21474836480,
      "status": "completed",
      "created_at": "2024-01-14T02:00:00Z",
      "expires_at": "2024-02-14T02:00:00Z"
    }
  ]
}
```

#### Create Backup

```http
POST /api/v1/customer/backups
Authorization: Bearer <token>
```

**Request**:
```json
{
  "vm_id": "550e8400-e29b-41d4-a716-446655440002",
  "name": "Pre-update backup"
}
```

**Response** (202 Accepted):
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-44665544000b",
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "status": "creating"
  }
}
```

#### Get Backup

```http
GET /api/v1/customer/backups/:id
Authorization: Bearer <token>
```

#### Delete Backup

```http
DELETE /api/v1/customer/backups/:id
Authorization: Bearer <token>
```

#### Restore Backup

```http
POST /api/v1/customer/backups/:id/restore
Authorization: Bearer <token>
```

**Response** (202 Accepted):
```json
{
  "data": {
    "message": "Backup restore initiated"
  }
}
```

### Snapshots

#### List Snapshots

```http
GET /api/v1/customer/snapshots
Authorization: Bearer <token>
```

#### Create Snapshot

```http
POST /api/v1/customer/snapshots
Authorization: Bearer <token>
```

**Request**:
```json
{
  "vm_id": "550e8400-e29b-41d4-a716-446655440002",
  "name": "Before upgrade"
}
```

#### Delete Snapshot

```http
DELETE /api/v1/customer/snapshots/:id
Authorization: Bearer <token>
```

#### Restore Snapshot

```http
POST /api/v1/customer/snapshots/:id/restore
Authorization: Bearer <token>
```

### API Keys

#### List API Keys

```http
GET /api/v1/customer/api-keys
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-44665544000c",
      "name": "CI/CD Pipeline",
      "permissions": ["vms:read", "vms:write"],
      "is_active": true,
      "last_used_at": "2024-01-14T18:00:00Z",
      "expires_at": "2024-12-31T23:59:59Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Create API Key

```http
POST /api/v1/customer/api-keys
Authorization: Bearer <token>
```

**Request**:
```json
{
  "name": "Automation Key",
  "permissions": ["vms:read", "backups:read", "backups:write"],
  "expires_at": "2024-12-31T23:59:59Z"
}
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-44665544000d",
    "name": "Automation Key",
    "key": "vs_live_abc123...xyz789",
    "permissions": ["vms:read", "backups:read", "backups:write"],
    "expires_at": "2024-12-31T23:59:59Z"
  }
}
```

**Important**: The `key` field is only returned once during creation.

#### Delete API Key

```http
DELETE /api/v1/customer/api-keys/:id
Authorization: Bearer <token>
```

### Webhooks

#### List Webhooks

```http
GET /api/v1/customer/webhooks
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-44665544000e",
      "url": "https://hooks.example.com/virtuestack",
      "events": ["vm.created", "vm.deleted", "backup.completed"],
      "is_active": true,
      "fail_count": 0,
      "last_success_at": "2024-01-15T10:30:00Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Create Webhook

```http
POST /api/v1/customer/webhooks
Authorization: Bearer <token>
```

**Request**:
```json
{
  "url": "https://hooks.example.com/virtuestack",
  "secret": "my-webhook-secret-16chars",
  "events": ["vm.created", "vm.deleted", "backup.completed"]
}
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-44665544000e",
    "url": "https://hooks.example.com/virtuestack",
    "events": ["vm.created", "vm.deleted", "backup.completed"],
    "is_active": true
  }
}
```

#### Get Webhook

```http
GET /api/v1/customer/webhooks/:id
Authorization: Bearer <token>
```

#### Update Webhook

```http
PUT /api/v1/customer/webhooks/:id
Authorization: Bearer <token>
```

**Request**:
```json
{
  "url": "https://new-hooks.example.com/virtuestack",
  "events": ["vm.created", "backup.completed"],
  "is_active": true
}
```

#### Delete Webhook

```http
DELETE /api/v1/customer/webhooks/:id
Authorization: Bearer <token>
```

#### List Webhook Deliveries

```http
GET /api/v1/customer/webhooks/:id/deliveries
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-44665544000f",
      "event": "vm.created",
      "attempt_count": 1,
      "response_status": 200,
      "success": true,
      "delivered_at": "2024-01-15T10:30:05Z",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### Templates

#### List Available Templates

```http
GET /api/v1/customer/templates
Authorization: Bearer <token>
```

**Response**:
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440005",
      "name": "Ubuntu 22.04 LTS",
      "slug": "ubuntu-22-04",
      "os_type": "linux",
      "min_disk_gb": 10
    }
  ]
}
```

### Notifications

#### Get Notification Preferences

```http
GET /api/v1/customer/notifications/preferences
Authorization: Bearer <token>
```

#### Update Notification Preferences

```http
PUT /api/v1/customer/notifications/preferences
Authorization: Bearer <token>
```

**Request**:
```json
{
  "email_enabled": true,
  "telegram_enabled": false,
  "events": {
    "vm_created": true,
    "vm_deleted": true,
    "backup_completed": true,
    "bandwidth_alert": true
  }
}
```

#### List Notification Events

```http
GET /api/v1/customer/notifications/events
Authorization: Bearer <token>
```

#### Get Available Event Types

```http
GET /api/v1/customer/notifications/events/types
Authorization: Bearer <token>
```

---

## Provisioning API

Base path: `/api/v1/provisioning`

Requires: `X-API-Key` header with valid provisioning key

Used by WHMCS and other billing systems for automated provisioning.

### VM Operations

#### Create VM

```http
POST /api/v1/provisioning/vms
X-API-Key: your-api-key
Content-Type: application/json
Idempotency-Key: unique-request-id
```

**Request**:
```json
{
  "customer_id": "550e8400-e29b-41d4-a716-446655440003",
  "plan_id": "550e8400-e29b-41d4-a716-446655440004",
  "template_id": "550e8400-e29b-41d4-a716-446655440005",
  "hostname": "whmcs-service-123",
  "whmcs_service_id": 123,
  "ssh_keys": [],
  "location_id": "us-east-1"
}
```

**Response** (202 Accepted):
```json
{
  "data": {
    "task_id": "task-550e8400-e29b-41d4-a716-446655440006",
    "vm_id": "550e8400-e29b-41d4-a716-446655440002"
  }
}
```

#### Get VM by ID

```http
GET /api/v1/provisioning/vms/:id
X-API-Key: your-api-key
```

**Response**:
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "hostname": "whmcs-service-123",
    "status": "running",
    "whmcs_service_id": 123,
    "ip_addresses": ["192.168.1.100"],
    "vcpu": 2,
    "memory_mb": 2048,
    "disk_gb": 40
  }
}
```

#### Get VM by WHMCS Service ID

```http
GET /api/v1/provisioning/vms/by-service/:service_id
X-API-Key: your-api-key
```

#### Delete VM

```http
DELETE /api/v1/provisioning/vms/:id
X-API-Key: your-api-key
```

**Response** (202 Accepted):
```json
{
  "data": {
    "task_id": "task-550e8400-e29b-41d4-a716-446655440008"
  }
}
```

#### Suspend VM

```http
POST /api/v1/provisioning/vms/:id/suspend
X-API-Key: your-api-key
```

**Response**:
```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "status": "suspended"
  }
}
```

#### Unsuspend VM

```http
POST /api/v1/provisioning/vms/:id/unsuspend
X-API-Key: your-api-key
```

#### Resize VM

```http
POST /api/v1/provisioning/vms/:id/resize
X-API-Key: your-api-key
```

**Request**:
```json
{
  "plan_id": "550e8400-e29b-41d4-a716-446655440004"
}
```

Or specify resources directly:

```json
{
  "vcpu": 4,
  "memory_mb": 4096,
  "disk_gb": 80
}
```

#### Set Password

```http
POST /api/v1/provisioning/vms/:id/password
X-API-Key: your-api-key
```

**Request**:
```json
{
  "password": "NewSecurePassword123!"
}
```

#### Reset Password

```http
POST /api/v1/provisioning/vms/:id/password/reset
X-API-Key: your-api-key
```

**Response**:
```json
{
  "data": {
    "password": "GeneratedPassword123!"
  }
}
```

#### Power Operations

```http
POST /api/v1/provisioning/vms/:id/power
X-API-Key: your-api-key
```

**Request**:
```json
{
  "action": "start"
}
```

Valid actions: `start`, `stop`, `restart`, `force_stop`

#### Get VM Status

```http
GET /api/v1/provisioning/vms/:id/status
X-API-Key: your-api-key
```

**Response**:
```json
{
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "status": "running",
    "power_state": "on",
    "uptime_seconds": 432000
  }
}
```

### Tasks

#### Get Task Status

```http
GET /api/v1/provisioning/tasks/:id
X-API-Key: your-api-key
```

**Response**:
```json
{
  "data": {
    "id": "task-550e8400-e29b-41d4-a716-446655440006",
    "type": "vm.create",
    "status": "completed",
    "progress": 100,
    "result": {
      "vm_id": "550e8400-e29b-41d4-a716-446655440002",
      "ip_addresses": ["192.168.1.100"]
    },
    "created_at": "2024-01-15T10:30:00Z",
    "completed_at": "2024-01-15T10:35:00Z"
  }
}
```

**Task Statuses**:

| Status | Description |
|--------|-------------|
| `pending` | Task is queued |
| `running` | Task is in progress |
| `completed` | Task finished successfully |
| `failed` | Task failed |

---

## Webhooks

### Available Events

| Event | Description | Trigger |
|-------|-------------|---------|
| `vm.created` | VM provisioning complete | After VM is fully provisioned |
| `vm.deleted` | VM terminated | After VM is deleted |
| `vm.started` | VM powered on | After successful start |
| `vm.stopped` | VM powered off | After successful stop |
| `vm.reinstalled` | OS reinstallation complete | After reinstall finishes |
| `vm.migrated` | VM moved to new node | After migration completes |
| `backup.completed` | Backup finished | After backup is ready |
| `backup.failed` | Backup failed | When backup errors |

### Webhook Payload Format

All webhooks use the following payload structure:

```json
{
  "event": "vm.created",
  "timestamp": "2024-01-15T10:30:00Z",
  "idempotency_key": "550e8400-e29b-41d4-a716-446655440010",
  "data": {
    // Event-specific data
  }
}
```

### Event Data Examples

#### vm.created

```json
{
  "event": "vm.created",
  "timestamp": "2024-01-15T10:35:00Z",
  "idempotency_key": "uuid-here",
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "customer_id": "550e8400-e29b-41d4-a716-446655440003",
    "hostname": "my-vps",
    "ip_addresses": ["192.168.1.100"],
    "status": "running",
    "plan_name": "Starter VPS",
    "template_name": "Ubuntu 22.04 LTS"
  }
}
```

#### vm.deleted

```json
{
  "event": "vm.deleted",
  "timestamp": "2024-01-15T11:00:00Z",
  "idempotency_key": "uuid-here",
  "data": {
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "customer_id": "550e8400-e29b-41d4-a716-446655440003",
    "hostname": "my-vps"
  }
}
```

#### backup.completed

```json
{
  "event": "backup.completed",
  "timestamp": "2024-01-15T12:00:00Z",
  "idempotency_key": "uuid-here",
  "data": {
    "backup_id": "550e8400-e29b-41d4-a716-44665544000b",
    "vm_id": "550e8400-e29b-41d4-a716-446655440002",
    "size_bytes": 21474836480,
    "status": "completed"
  }
}
```

### Signature Verification

Webhook payloads are signed using HMAC-SHA256. Verify the signature:

```python
import hmac
import hashlib

def verify_webhook_signature(secret: str, payload: bytes, signature: str) -> bool:
    """
    Verify webhook signature.
    
    Args:
        secret: Webhook secret configured during creation
        payload: Raw request body bytes
        signature: X-Signature header value
    
    Returns:
        True if signature is valid
    """
    expected = hmac.new(
        secret.encode('utf-8'),
        payload,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)
```

### Delivery and Retry

- **Timeout**: 30 seconds
- **Max Attempts**: 5
- **Retry Delays**: 1min, 5min, 15min, 1hr, 6hr

Webhooks are disabled after 5 consecutive failures. Re-enable via the API or UI.

### Request Headers

```
Content-Type: application/json
X-Signature: <hmac-sha256-signature>
X-Event: vm.created
X-Delivery-ID: <unique-delivery-id>
```

---

## SDK Examples

### JavaScript/TypeScript

```typescript
// Customer API client
const API_BASE = 'https://your-domain.com/api/v1';

async function login(email: string, password: string) {
  const response = await fetch(`${API_BASE}/customer/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password })
  });
  return response.json();
}

async function listVMs(token: string) {
  const response = await fetch(`${API_BASE}/customer/vms`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  return response.json();
}

async function startVM(token: string, vmId: string) {
  const response = await fetch(`${API_BASE}/customer/vms/${vmId}/start`, {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  return response.json();
}
```

### Python

```python
import requests

class VirtueStackClient:
    def __init__(self, base_url: str):
        self.base_url = base_url
        self.token = None
    
    def login(self, email: str, password: str) -> dict:
        response = requests.post(
            f"{self.base_url}/api/v1/customer/auth/login",
            json={"email": email, "password": password}
        )
        data = response.json()["data"]
        self.token = data.get("access_token")
        return data
    
    def list_vms(self) -> list:
        response = requests.get(
            f"{self.base_url}/api/v1/customer/vms",
            headers={"Authorization": f"Bearer {self.token}"}
        )
        return response.json()["data"]
    
    def create_vm(self, plan_id: str, template_id: str, hostname: str, password: str) -> dict:
        response = requests.post(
            f"{self.base_url}/api/v1/customer/vms",
            headers={"Authorization": f"Bearer {self.token}"},
            json={
                "plan_id": plan_id,
                "template_id": template_id,
                "hostname": hostname,
                "password": password
            }
        )
        return response.json()

# Usage
client = VirtueStackClient("https://your-domain.com")
client.login("customer@example.com", "password")
vms = client.list_vms()
```

### cURL

```bash
# Login
TOKEN=$(curl -s -X POST https://your-domain.com/api/v1/customer/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"customer@example.com","password":"password"}' \
  | jq -r '.data.access_token')

# List VMs
curl -s https://your-domain.com/api/v1/customer/vms \
  -H "Authorization: Bearer $TOKEN" | jq

# Create VM
curl -s -X POST https://your-domain.com/api/v1/customer/vms \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "plan_id": "plan-uuid",
    "template_id": "template-uuid",
    "hostname": "my-vps",
    "password": "SecurePassword123!"
  }' | jq
```

---

## Changelog

### v1 (Current)

- Initial API release
- Admin, Customer, and Provisioning endpoints
- JWT and API key authentication
- Webhook support
- Rate limiting