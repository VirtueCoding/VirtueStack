

# Node Metrics Implementation - Real Disk and Ceph Status

## 2026-03-11

### Summary

Implemented real disk metrics and Ceph status for node metrics endpoint. The implementation includes:

1. **Model Updates** (`internal/controller/models/node.go`):
   - Added `TotalDiskGB`, `UsedDiskGB`, `CephConnected`, `CephTotalGB`, `CephUsedGB` to `NodeHeartbeat`
   - Added `TotalDiskGB`, `UsedDiskGB`, `CephStatus`, `CephTotalGB`, `CephUsedGB` to `NodeStatus`

2. **Node Agent Client** (`internal/controller/services/node_agent_client.go`):
   - Created `NodeAgentGRPCClient` implementing the `NodeAgentClient` interface
   - Implemented 5-second caching for metrics to reduce load on node agents
   - Thread-safe cache with RWMutex for concurrent access
   - Graceful handling when node agent gRPC is not yet fully implemented (proto pending)

3. **Node Service Updates** (`internal/controller/services/node_service.go`):
   - Updated `GetNodeStatus` to populate disk metrics from node agent
   - Added Ceph status handling: "connected", "disconnected", or "unknown"
   - Handles missing Ceph gracefully by returning "unknown" status

4. **Server Wiring** (`internal/controller/server.go`):
   - Updated to create `NodeAgentGRPCClient` when `nodeClient` is available
   - Falls back to `nil` if gRPC connection pool not configured

### Ceph Status Values

- `"connected"` - Ceph is connected and reporting stats
- `"disconnected"` - Ceph is configured but not connected
- `"unknown"` - Cannot determine Ceph status (node offline, no agent, or error)

### Cache Implementation

```go
type metricsCache struct {
    data  map[string]*cachedMetrics
    mutex sync.RWMutex
    ttl   time.Duration // 5 seconds
}
```

- Read lock for cache hits (fast path)
- Write lock for cache updates
- Automatic expiration based on timestamp

### Files Modified

- `internal/controller/models/node.go` - Added disk and Ceph fields to models
- `internal/controller/services/node_agent_client.go` - New file with gRPC client + caching
- `internal/controller/services/node_service.go` - Updated GetNodeStatus to use new fields
- `internal/controller/server.go` - Wired up NodeAgentGRPCClient

### Node Agent Side (Already Implemented)

The node agent (`internal/nodeagent/server.go`) already has:
- `getDiskUsage()` - uses `syscall.Statfs` for local disk
- `getCephPoolStats()` - uses RBD manager for Ceph stats
- `isCephConnected()` - checks Ceph connection health

These are returned via gRPC in `GetNodeHealth` and `GetNodeResources` responses.

---

## 2026-03-11 (continued)

### Password Validation at Application Startup

**Summary:** Added password validation to prevent the application from starting with weak passwords.

**Files Modified:**
- `internal/shared/config/config.go`

**Implementation:**

1. **Weak password list** - Map of known weak passwords:
   - `changeme`, `password`, `123456`, `admin`, `root`

2. **Validation functions added:**
   - `isWeakPassword(password string) bool` - Checks if password is in weak list
   - `validatePasswords() error` - Validates DB_PASSWORD, ADMIN_PASSWORD, CUSTOMER_PASSWORD env vars
   - `validateDefaultPasswords()` - Logs warnings for weak default passwords

3. **Startup behavior:**
   - If any password env var is set to a weak value, app **refuses to start** with clear error
   - If weak password detected, logs warning for production awareness
   - Does not break existing config loading - only adds security check

**Key patterns:**
- Case-insensitive password matching with `strings.ToLower()`
- Clear error messages identifying which env vars have weak passwords
- Separate validation (hard fail) from warning (soft fail) functions

