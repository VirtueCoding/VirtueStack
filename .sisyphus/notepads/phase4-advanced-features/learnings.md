# Phase 4 Learnings

## 2026-03-11: Backup Task Handler - Snapshot Protection and Cloning

### Pattern: RBD Snapshot Cloning for Backups

When creating backups, the workflow is:
1. Create snapshot on VM disk (in vs-vms pool)
2. **Protect** the snapshot (required before cloning)
3. **Clone** the protected snapshot to vs-backups pool
4. Set `backup.StoragePath` to track the backup location

### Key Code Patterns

#### NodeAgentClient Interface (handlers.go)
The interface abstracts gRPC communication with node agents. When adding new storage operations:
- Add methods that take `nodeID`, `vmID` as parameters
- The node agent translates these to RBD operations

#### RBDManager Methods (rbd.go)
- `CloneFromTemplate` (lines 84-109): Pattern for cloning between pools
- `ProtectSnapshot` (lines 246-269): Must be called before cloning
- `CloneSnapshotToPool`: New method following CloneFromTemplate pattern
  - Opens IO contexts for both source and target pools
  - Uses `rbd.CloneImage` with source snapshot

### Backup Image Naming Convention
- Snapshot name: `backup-{vmID}-{timestamp}`
- Backup image name: `vs-{vmID}-{timestamp}-backup`
- Storage path: `{pool}/{image}` (e.g., `vs-backups/vs-vm123-1700000000-backup`)

### Error Handling Pattern
When clone fails after protecting snapshot:
```go
if err := deps.NodeClient.CloneSnapshot(...); err != nil {
    // Cleanup: unprotect the snapshot since clone failed
    if unprotectErr := deps.NodeClient.UnprotectSnapshot(...); unprotectErr != nil {
        logger.Warn("failed to unprotect snapshot during cleanup", "error", unprotectErr)
    }
    return fmt.Errorf("cloning snapshot: %w", err)
}
```

### Constants Added
- `BackupPoolName = "vs-backups"` - The Ceph pool for backup images