# VirtueStack — Remaining TODO Items

> Auto-generated from audit. Two follow-up tasks remain after the compile-fix PR.

---

## Task A: Wire gRPC Node Agent Integration

**Goal:** Replace all stubbed gRPC methods with real implementations using the generated protobuf code.

The proto files have already been compiled — generated code exists at:
- `internal/shared/proto/virtuestack/node_agent_grpc.pb.go`
- `internal/shared/proto/virtuestack/node_agent.pb.go`

### A1 — Node Agent Server: Register Generated Service

File: `internal/nodeagent/server.go`

1. **Remove ALL local placeholder message types** at the bottom of the file: `Empty`, `VMIdentifier`, `VMOperationResponse`, `PingResponse`, `NodeHealthResponse`, `VMStatusResponse`, `VMMetricsResponse`, `NodeResourcesResponse`, `CreateVMRequest`, `CreateVMResponse`, `StopVMRequest`, `PingRequest`. These are replaced by the generated protobuf types.

2. **Import the generated proto package:**
   ```go
   nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
   ```

3. **Embed the `UnimplementedNodeAgentServiceServer`** in `grpcHandler`:
   ```go
   type grpcHandler struct {
       nodeagentpb.UnimplementedNodeAgentServiceServer
       server *Server
   }
   ```

4. **Register the service** in `registerServices()`:
   ```go
   func (s *Server) registerServices() {
       handler := newGRPCHandler(s)
       nodeagentpb.RegisterNodeAgentServiceServer(s.grpcServer, handler)
   }
   ```

5. **Update all gRPC handler method signatures** to use generated protobuf types. Example:
   ```go
   func (h *grpcHandler) Ping(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.PingResponse, error) {
       return &nodeagentpb.PingResponse{
           NodeId:    h.server.config.NodeID,
           Timestamp: timestamppb.Now(),
       }, nil
   }
   ```
   Do this for ALL methods: `GetNodeHealth`, `GetVMStatus`, `GetVMMetrics`, `GetNodeResources`, `CreateVM`, `StartVM`, `StopVM`, `ForceStopVM`, `DeleteVM`. Import `"google.golang.org/protobuf/types/known/timestamppb"`.

6. **Fix the `IsAlive` call** — use the `isLibvirtAlive()` helper added in the compile-fix PR.

7. **Update `mapStatusToProto`** to return `nodeagentpb.VMStatus` enum instead of `int32`.

### A2 — Controller gRPC Client: Implement Real Calls

File: `internal/controller/services/node_agent_client.go`

1. **Import generated proto:**
   ```go
   nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
   ```

2. **Replace `fetchMetricsFromNode` stub** with real gRPC call:
   ```go
   func (c *NodeAgentGRPCClient) fetchMetricsFromNode(ctx context.Context, conn *grpc.ClientConn, nodeID string) (*models.NodeHeartbeat, error) {
       client := nodeagentpb.NewNodeAgentServiceClient(conn)
       resp, err := client.GetNodeHealth(ctx, &nodeagentpb.Empty{})
       if err != nil {
           return nil, fmt.Errorf("gRPC GetNodeHealth: %w", err)
       }
       return convertProtoHealthToHeartbeat(resp), nil
   }
   ```

3. **Replace ALL stub methods** (`StartVM`, `StopVM`, `ForceStopVM`, `DeleteVM`, `ResizeVM`, `GetVMMetrics`, `GetVMStatus`, `PingNode`, `EvacuateNode`) with real gRPC calls. Each method should:
   - Get the node address from `nodeRepo`
   - Get a connection from the pool via `connPool.GetConnection()`
   - Create a `nodeagentpb.NewNodeAgentServiceClient(conn)`
   - Call the appropriate RPC method
   - Convert the response to model types

4. **Add converter functions:**
   ```go
   func convertProtoHealthToHeartbeat(resp *nodeagentpb.NodeHealthResponse) *models.NodeHeartbeat { ... }
   func convertProtoMetrics(resp *nodeagentpb.VMMetricsResponse) *models.VMMetrics { ... }
   func convertProtoStatus(status nodeagentpb.VMStatus) string { ... }
   ```

### A3 — Add Missing NodeAgentClient Methods for Tasks

The `NodeAgentClient` interface in `internal/controller/tasks/handlers.go` requires methods used by task handlers (`MigrateVM`, `PostMigrateSetup`). Implement these in `NodeAgentGRPCClient` using the proto `MigrateVM` and `PostMigrateSetup` RPCs.

### Verification
- `go build ./...` must pass
- `go vet ./...` must pass

---

## Task B: Complete Business Logic

**Goal:** Implement all incomplete business logic identified in the audit.

### B1 — VM Migration Admin API Handler

File: `internal/controller/api/admin/vms.go` (~line 340)

The `MigrateVM` handler has a TODO and returns a fake success. Wire it to `MigrationService.MigrateVM()`:
```go
result, err := h.migrationService.MigrateVM(c.Request.Context(), &services.MigrateVMRequest{
    VMID:         vmID,
    TargetNodeID: &req.TargetNodeID,
    Live:         true,
}, claims.UserID)
if err != nil {
    // handle error
}
c.JSON(http.StatusAccepted, models.Response{Data: result})
```
Add `migrationService *services.MigrationService` to `AdminHandler` and wire it from `internal/controller/server.go` `InitializeServices`.

### B2 — MigrationService.CancelMigration

File: `internal/controller/services/migration_service.go` (~line 347)

Replace the stub with real implementation:
- Cancel the NATS task if still pending
- If migration is in progress, call `NodeClient.AbortMigration` via gRPC
- Revert VM status to the previous state (running or stopped)

### B3 — MigrationService.GetMigrationStatus

File: `internal/controller/services/migration_service.go` (~line 317)

Add a `TaskRepository` dependency to `MigrationService` and implement status lookup:
```go
type MigrationService struct {
    vmRepo        *repository.VMRepository
    nodeRepo      *repository.NodeRepository
    taskRepo      *repository.TaskRepository  // ADD THIS
    taskPublisher TaskPublisher
    logger        *slog.Logger
}
```

### B4 — UpdateVMHostname Repository Method

File: `internal/controller/services/vm_service.go` (~line 725) and `internal/controller/repository/vm_repo.go`

Remove the "not implemented" error from `VMService.UpdateVMHostname` and add the repository method:
```go
func (r *VMRepository) UpdateHostname(ctx context.Context, vmID, hostname string) error {
    const q = `UPDATE vms SET hostname = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
    tag, err := r.db.Exec(ctx, q, hostname, vmID)
    if err != nil {
        return fmt.Errorf("updating hostname for VM %s: %w", vmID, err)
    }
    if tag.RowsAffected() == 0 {
        return fmt.Errorf("updating hostname for VM %s: %w", vmID, ErrNoRowsAffected)
    }
    return nil
}
```
Then update `VMService.UpdateVMHostname` to call `s.vmRepo.UpdateHostname(ctx, vm.ID, newHostname)`.

### B5 — Admin Session Limit Enforcement

File: `internal/controller/services/auth_service.go` (~line 587)

Replace the placeholder comment with:
```go
activeSessions, err := s.customerRepo.CountSessionsByUser(ctx, admin.ID, "admin")
if err != nil {
    s.logger.Warn("failed to count admin sessions", "admin_id", admin.ID, "error", err)
} else if activeSessions >= MaxAdminSessions {
    if err := s.customerRepo.DeleteOldestSession(ctx, admin.ID, "admin"); err != nil {
        s.logger.Warn("failed to delete oldest admin session", "admin_id", admin.ID, "error", err)
    }
}
```
Add `CountSessionsByUser` and `DeleteOldestSession` to `CustomerRepository` in `internal/controller/repository/customer_repo.go`:
```go
func (r *CustomerRepository) CountSessionsByUser(ctx context.Context, userID, userType string) (int, error) {
    const q = `SELECT COUNT(*) FROM sessions WHERE user_id = $1 AND user_type = $2 AND expires_at > NOW()`
    var count int
    err := r.db.QueryRow(ctx, q, userID, userType).Scan(&count)
    return count, err
}

func (r *CustomerRepository) DeleteOldestSession(ctx context.Context, userID, userType string) error {
    const q = `DELETE FROM sessions WHERE id = (
        SELECT id FROM sessions WHERE user_id = $1 AND user_type = $2 ORDER BY created_at ASC LIMIT 1
    )`
    _, err := r.db.Exec(ctx, q, userID, userType)
    return err
}
```

### B6 — TOTP Backup Code Verification

File: `internal/controller/services/auth_service.go` (~line 206)

After TOTP validation fails, check backup codes:
```go
if !valid {
    if customer.BackupCodesHash != nil && len(customer.BackupCodesHash) > 0 {
        for i, codeHash := range customer.BackupCodesHash {
            if crypto.HashSHA256(totpCode) == codeHash {
                remaining := append(customer.BackupCodesHash[:i], customer.BackupCodesHash[i+1:]...)
                _ = s.customerRepo.UpdateBackupCodes(ctx, customer.ID, remaining)
                valid = true
                s.logger.Info("backup code used", "customer_id", customer.ID)
                break
            }
        }
    }
    if !valid {
        return nil, "", sharederrors.ErrUnauthorized
    }
}
```
Add `UpdateBackupCodes` to `CustomerRepository` if it doesn't exist.

### B7 — Template Storage Backend

File: `internal/controller/server.go` (~line 175)

`TemplateService` currently gets `nil` for its storage backend. Make `TemplateService` handle `nil` storage gracefully for operations that don't need it (metadata-only operations use PostgreSQL). Add nil checks before any storage calls.

### B8 — Backup Node Agent

File: `internal/controller/server.go` (~line 185)

`BackupService` gets `nil` for `nodeAgent`. This is already handled gracefully in `BackupService.CreateBackup` (it checks `if s.nodeAgent != nil`). Verify all code paths in BackupService are nil-safe for `nodeAgent`.

### Verification
- `go build ./...` must pass
- `go vet ./...` must pass
