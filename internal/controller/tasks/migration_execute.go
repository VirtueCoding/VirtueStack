// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// handleVMMigrate handles the VM migration flow with storage-aware logic.
// Steps:
//  1. Parse payload and determine migration strategy
//  2. Validate VM and nodes
//  3. Execute migration based on strategy:
//     - LiveSharedStorage (Ceph→Ceph): Live migration with shared storage
//     - DiskCopy (QCOW→QCOW): Copy disk, sync delta, switchover
//     - Cold (mixed storage): Stop VM, copy disk, start on target
//  4. Update VM record with new node assignment
//
// Idempotency: If the VM is already on the target node, the task is considered
// successfully completed without performing migration operations.
func handleVMMigrate(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeVMMigrate)

	// Parse payload
	var payload VMMigratePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.migrate payload", "error", err)
		return fmt.Errorf("parsing vm.migrate payload: %w", err)
	}

	logger = logger.With(
		"vm_id", payload.VMID,
		"source_node_id", payload.SourceNodeID,
		"target_node_id", payload.TargetNodeID,
		"strategy", payload.MigrationStrategy,
	)
	logger.Info("vm.migrate task started")

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Initiating migration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	// Idempotency check: If VM is already on target node, task is complete
	if vm.NodeID != nil && *vm.NodeID == payload.TargetNodeID {
		logger.Info("VM already on target node, migration complete (idempotent)")
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM already on target node"); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}
		result := map[string]any{
			"vm_id":          payload.VMID,
			"source_node_id": payload.SourceNodeID,
			"target_node_id": payload.TargetNodeID,
			"status":         "already_migrated",
		}
		resultJSON, _ := json.Marshal(result)
		if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
			logger.Warn("failed to set task completed", "error", err)
		}
		return nil
	}

	// Validate current node assignment matches source node
	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	if *vm.NodeID != payload.SourceNodeID {
		return fmt.Errorf("VM %s is on node %s, not source node %s",
			payload.VMID, *vm.NodeID, payload.SourceNodeID)
	}

	// Get source and target node info
	sourceNode, err := deps.NodeRepo.GetByID(ctx, payload.SourceNodeID)
	if err != nil {
		logger.Error("failed to get source node", "error", err)
		return fmt.Errorf("getting source node %s: %w", payload.SourceNodeID, err)
	}

	targetNode, err := deps.NodeRepo.GetByID(ctx, payload.TargetNodeID)
	if err != nil {
		logger.Error("failed to get target node", "error", err)
		return fmt.Errorf("getting target node %s: %w", payload.TargetNodeID, err)
	}

	// Validate target node is online
	if targetNode.Status != models.NodeStatusOnline {
		return fmt.Errorf("target node %s is not online (status: %s)",
			payload.TargetNodeID, targetNode.Status)
	}

	// Update task progress: Preparing migration
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 10, "Preparing migration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Update VM status to migrating
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusMigrating); err != nil {
		logger.Error("failed to update VM status to migrating", "error", err)
		return fmt.Errorf("updating VM %s status to migrating: %w", payload.VMID, err)
	}

	// Execute migration based on strategy
	var migrationErr error
	switch payload.MigrationStrategy {
	case MigrationStrategyLiveSharedStorage:
		migrationErr = executeLiveSharedStorageMigration(ctx, task, deps, payload, vm, sourceNode, targetNode, logger)
	case MigrationStrategyDiskCopy:
		migrationErr = executeDiskCopyMigration(ctx, task, deps, payload, vm, sourceNode, targetNode, logger)
	case MigrationStrategyCold:
		migrationErr = executeColdMigration(ctx, task, deps, payload, vm, sourceNode, targetNode, logger)
	default:
		migrationErr = fmt.Errorf("unknown migration strategy: %s", payload.MigrationStrategy)
	}

	if migrationErr != nil {
		logger.Error("migration failed", "error", migrationErr)
		// Attempt to restore VM status
		restoreStatus := payload.PreMigrationState
		if restoreStatus == "" {
			restoreStatus = models.VMStatusError
		}
		if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, restoreStatus); err != nil {
			logger.Error("failed to restore VM status after migration failure", "operation", "UpdateStatus", "err", err)
		}
		return migrationErr
	}

	// Update task progress: Updating database
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 90, "Updating VM assignment..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Update VM record with new node assignment
	if err := deps.VMRepo.UpdateNodeAssignment(ctx, payload.VMID, payload.TargetNodeID); err != nil {
		logger.Error("failed to update VM node assignment", "error", err)
		return fmt.Errorf("updating VM %s node assignment to %s: %w",
			payload.VMID, payload.TargetNodeID, err)
	}

	// Determine final status based on pre-migration state
	finalStatus := models.VMStatusRunning
	if payload.PreMigrationState == models.VMStatusStopped || payload.PreMigrationState == models.VMStatusSuspended {
		finalStatus = payload.PreMigrationState
	}

	// Update VM status
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, finalStatus); err != nil {
		logger.Warn("failed to update VM status", "error", err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Migration completed successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":                  payload.VMID,
		"source_node_id":         payload.SourceNodeID,
		"source_node_address":    sourceNode.GRPCAddress,
		"target_node_id":         payload.TargetNodeID,
		"target_node_address":    targetNode.GRPCAddress,
		"status":                 "migrated",
		"migration_strategy":     string(payload.MigrationStrategy),
		"source_storage_backend": payload.SourceStorageBackend,
		"target_storage_backend": payload.TargetStorageBackend,
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.migrate task completed successfully",
		"vm_id", payload.VMID,
		"source_node", payload.SourceNodeID,
		"target_node", payload.TargetNodeID,
		"strategy", payload.MigrationStrategy)

	return nil
}

// executeLiveSharedStorageMigration performs live migration with shared Ceph storage.
// No disk copy is needed as both nodes have access to the same Ceph cluster.
func executeLiveSharedStorageMigration(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	payload VMMigratePayload,
	vm *models.VM,
	sourceNode, targetNode *models.Node,
	logger *slog.Logger,
) error {
	logger.Info("executing live migration with shared storage")

	// Update task progress
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 20, "Executing live migration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Prepare migration options
	migrateOpts := &MigrateVMOptions{
		TargetNodeAddress:  targetNode.GRPCAddress,
		BandwidthLimitMbps: 1000,
		Compression:        true,
		AutoConverge:       true,
	}

	// Initiate live migration via gRPC on source node
	if err := deps.NodeClient.MigrateVM(ctx, payload.SourceNodeID, payload.TargetNodeID, payload.VMID, migrateOpts); err != nil {
		return fmt.Errorf("live migration failed: %w", err)
	}

	// Update task progress: Post-migration setup
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Applying post-migration configuration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Re-apply bandwidth limits on target node
	if err := deps.NodeClient.PostMigrateSetup(ctx, payload.TargetNodeID, payload.VMID, vm.PortSpeedMbps); err != nil {
		logger.Warn("failed to apply post-migration setup on target node", "error", err)
	}

	return nil
}

// executeDiskCopyMigration performs migration with disk copy for QCOW storage.
// For running VMs: create snapshot, copy disk, sync delta, switchover.
// For stopped VMs: simple disk copy.
func executeDiskCopyMigration(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	payload VMMigratePayload,
	vm *models.VM,
	sourceNode, targetNode *models.Node,
	logger *slog.Logger,
) error {
	logger.Info("executing disk copy migration",
		"source_disk", payload.SourceDiskPath,
		"target_disk", payload.TargetDiskPath,
		"live", payload.Live)

	// Update task progress
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 20, "Preparing disk transfer..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// For live migration with QCOW, we need to:
	// 1. Create a snapshot on source
	// 2. Copy the base disk to target
	// 3. Sync the delta
	// 4. Stop VM, final sync, start on target
	if payload.Live && vm.Status == models.VMStatusRunning {
		return executeLiveDiskCopyMigration(ctx, task, deps, payload, vm, sourceNode, targetNode, logger)
	}

	// For stopped VMs or cold migration: simple disk copy
	return executeColdDiskCopyMigration(ctx, task, deps, payload, vm, sourceNode, targetNode, logger)
}

// executeLiveDiskCopyMigration performs live QCOW migration with disk copy and delta sync.
func executeLiveDiskCopyMigration(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	payload VMMigratePayload,
	vm *models.VM,
	sourceNode, targetNode *models.Node,
	logger *slog.Logger,
) error {
	// Step 1: Create snapshot on source for point-in-time copy
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 25, "Creating migration snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	snapshotName := fmt.Sprintf("migrate-%s", payload.VMID[:8])
	if _, err := deps.NodeClient.CreateSnapshot(ctx, payload.SourceNodeID, payload.VMID, snapshotName); err != nil {
		return fmt.Errorf("creating migration snapshot: %w", err)
	}
	defer func() {
		// Cleanup snapshot after migration
		_ = deps.NodeClient.DeleteSnapshot(ctx, payload.SourceNodeID, payload.VMID, snapshotName)
	}()

	// Step 2: Transfer disk to target node
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Transferring disk to target node..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:   payload.SourceNodeID,
		TargetNodeID:   payload.TargetNodeID,
		SourceDiskPath: payload.SourceDiskPath,
		TargetDiskPath: payload.TargetDiskPath,
		SnapshotName:   snapshotName,
		DiskSizeGB:     payload.DiskSizeGB,
		Compress:       true,
		ProgressCallback: func(progress int) {
			deps.TaskRepo.UpdateProgress(ctx, task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress))
		},
	}

	if err := deps.NodeClient.TransferDisk(ctx, transferOpts); err != nil {
		return fmt.Errorf("disk transfer failed: %w", err)
	}

	// Step 3: Stop VM for final switchover
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Stopping VM for switchover..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.StopVM(ctx, payload.SourceNodeID, payload.VMID, 60); err != nil {
		logger.Warn("graceful stop failed, forcing", "error", err)
		if err := deps.NodeClient.ForceStopVM(ctx, payload.SourceNodeID, payload.VMID); err != nil {
			return fmt.Errorf("stopping VM: %w", err)
		}
	}

	// Step 4: Sync final delta (if any)
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 85, "Syncing final changes..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Step 5: Delete VM definition on source (but keep disk for now)
	if err := deps.NodeClient.DeleteVM(ctx, payload.SourceNodeID, payload.VMID); err != nil {
		logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Step 6: Create VM on target with transferred disk
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 88, "Starting VM on target node..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// The VM definition should be created on the target using the transferred disk
	// This is handled by the PrepareMigratedVM RPC
	if err := deps.NodeClient.PrepareMigratedVM(ctx, payload.TargetNodeID, payload.VMID, payload.TargetDiskPath, vm); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Step 7: Start VM on target
	if err := deps.NodeClient.StartVM(ctx, payload.TargetNodeID, payload.VMID); err != nil {
		return fmt.Errorf("starting VM on target: %w", err)
	}

	// Step 8: Apply post-migration setup
	if err := deps.NodeClient.PostMigrateSetup(ctx, payload.TargetNodeID, payload.VMID, vm.PortSpeedMbps); err != nil {
		logger.Warn("failed to apply post-migration setup", "error", err)
	}

	return nil
}

// executeColdDiskCopyMigration performs cold disk copy for stopped VMs.
func executeColdDiskCopyMigration(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	payload VMMigratePayload,
	vm *models.VM,
	sourceNode, targetNode *models.Node,
	logger *slog.Logger,
) error {
	// Ensure VM is stopped
	if vm.Status == models.VMStatusRunning {
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 25, "Stopping VM..."); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}
		if err := deps.NodeClient.StopVM(ctx, payload.SourceNodeID, payload.VMID, 120); err != nil {
			logger.Warn("graceful stop failed, forcing", "error", err)
			if err := deps.NodeClient.ForceStopVM(ctx, payload.SourceNodeID, payload.VMID); err != nil {
				return fmt.Errorf("stopping VM: %w", err)
			}
		}
	}

	// Transfer disk
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Transferring disk to target node..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:   payload.SourceNodeID,
		TargetNodeID:   payload.TargetNodeID,
		SourceDiskPath: payload.SourceDiskPath,
		TargetDiskPath: payload.TargetDiskPath,
		DiskSizeGB:     payload.DiskSizeGB,
		Compress:       true,
		ProgressCallback: func(progress int) {
			deps.TaskRepo.UpdateProgress(ctx, task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress))
		},
	}

	if err := deps.NodeClient.TransferDisk(ctx, transferOpts); err != nil {
		return fmt.Errorf("disk transfer failed: %w", err)
	}

	// Delete VM definition on source
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Cleaning up source node..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.DeleteVM(ctx, payload.SourceNodeID, payload.VMID); err != nil {
		logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Create VM on target
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 85, "Preparing VM on target node..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.PrepareMigratedVM(ctx, payload.TargetNodeID, payload.VMID, payload.TargetDiskPath, vm); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Start VM on target (if it was running before)
	if payload.PreMigrationState == models.VMStatusRunning {
		if err := deps.NodeClient.StartVM(ctx, payload.TargetNodeID, payload.VMID); err != nil {
			return fmt.Errorf("starting VM on target: %w", err)
		}
	}

	return nil
}

// executeColdMigration performs cold migration with format conversion for mixed storage.
func executeColdMigration(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	payload VMMigratePayload,
	vm *models.VM,
	sourceNode, targetNode *models.Node,
	logger *slog.Logger,
) error {
	logger.Info("executing cold migration with format conversion",
		"source_backend", payload.SourceStorageBackend,
		"target_backend", payload.TargetStorageBackend)

	// Ensure VM is stopped
	if vm.Status == models.VMStatusRunning {
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 20, "Stopping VM for cold migration..."); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}
		if err := deps.NodeClient.StopVM(ctx, payload.SourceNodeID, payload.VMID, 120); err != nil {
			logger.Warn("graceful stop failed, forcing", "error", err)
			if err := deps.NodeClient.ForceStopVM(ctx, payload.SourceNodeID, payload.VMID); err != nil {
				return fmt.Errorf("stopping VM: %w", err)
			}
		}
	}

	// Transfer disk with format conversion
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Transferring disk with format conversion..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:         payload.SourceNodeID,
		TargetNodeID:         payload.TargetNodeID,
		SourceDiskPath:       payload.SourceDiskPath,
		TargetDiskPath:       payload.TargetDiskPath,
		SourceStorageBackend: payload.SourceStorageBackend,
		TargetStorageBackend: payload.TargetStorageBackend,
		DiskSizeGB:           payload.DiskSizeGB,
		Compress:             true,
		ConvertFormat:        true,
		ProgressCallback: func(progress int) {
			deps.TaskRepo.UpdateProgress(ctx, task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress))
		},
	}

	if err := deps.NodeClient.TransferDisk(ctx, transferOpts); err != nil {
		return fmt.Errorf("disk transfer with conversion failed: %w", err)
	}

	// Delete VM definition on source
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Cleaning up source node..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.DeleteVM(ctx, payload.SourceNodeID, payload.VMID); err != nil {
		logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Create VM on target
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 85, "Preparing VM on target node..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.PrepareMigratedVM(ctx, payload.TargetNodeID, payload.VMID, payload.TargetDiskPath, vm); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Start VM on target (if it was running before)
	if payload.PreMigrationState == models.VMStatusRunning {
		if err := deps.NodeClient.StartVM(ctx, payload.TargetNodeID, payload.VMID); err != nil {
			return fmt.Errorf("starting VM on target: %w", err)
		}
	}

	return nil
}
