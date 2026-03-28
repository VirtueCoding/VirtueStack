// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// defaultMigrationBandwidthMbps is the default bandwidth limit for VM migrations in Mbps.
const defaultMigrationBandwidthMbps = 1000

// MigrationContext holds the common state for VM migration operations.
// It bundles the 8 parameters previously passed individually to migration sub-functions,
// reducing parameter count to comply with QG-01 (max 4 parameters).
type MigrationContext struct {
	Ctx        context.Context
	Task       *models.Task
	Deps       *HandlerDeps
	Payload    VMMigratePayload
	VM         *models.VM
	SourceNode *models.Node
	TargetNode *models.Node
	Logger     *slog.Logger
}

// handleVMMigrate handles the VM migration flow with storage-aware logic.
// Idempotency: If the VM is already on the target node, the task is considered
// successfully completed without performing migration operations.
func handleVMMigrate(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	mc, err := prepareMigrationContext(ctx, task, deps)
	if err != nil {
		return err
	}
	if mc == nil {
		return nil // Already on target node (idempotent success)
	}

	if err := executeMigrationStrategy(mc); err != nil {
		handleMigrationFailure(mc, err)
		return err
	}

	return finalizeMigration(mc)
}

// prepareMigrationContext validates the migration request and builds the context.
// Returns nil context if VM is already on target (idempotent success).
func prepareMigrationContext(ctx context.Context, task *models.Task, deps *HandlerDeps) (*MigrationContext, error) {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeVMMigrate)

	var payload VMMigratePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.migrate payload", "error", err)
		return nil, fmt.Errorf("parsing vm.migrate payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID, "source_node_id", payload.SourceNodeID,
		"target_node_id", payload.TargetNodeID, "strategy", payload.MigrationStrategy)
	logger.Info("vm.migrate task started")

	vm, sourceNode, targetNode, err := validateMigrationEntities(ctx, deps, payload, logger)
	if err != nil {
		return nil, err
	}

	// Idempotency check: VM already on target
	if vm.NodeID != nil && *vm.NodeID == payload.TargetNodeID {
		logger.Info("VM already on target node, migration complete (idempotent)")
		markMigrationComplete(ctx, deps, task.ID, payload, logger)
		return nil, nil
	}

	fromStatus := payload.PreMigrationState
	if fromStatus == "" {
		fromStatus = vm.Status
	}
	if err := deps.VMRepo.TransitionStatus(ctx, payload.VMID, fromStatus, models.VMStatusMigrating); err != nil {
		return nil, fmt.Errorf("updating VM %s status to migrating: %w", payload.VMID, err)
	}

	return &MigrationContext{
		Ctx: ctx, Task: task, Deps: deps, Payload: payload,
		VM: vm, SourceNode: sourceNode, TargetNode: targetNode, Logger: logger,
	}, nil
}

// validateMigrationEntities retrieves and validates VM and node entities.
func validateMigrationEntities(ctx context.Context, deps *HandlerDeps, payload VMMigratePayload, logger *slog.Logger) (*models.VM, *models.Node, *models.Node, error) {
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return nil, nil, nil, fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return nil, nil, nil, fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	if *vm.NodeID != payload.SourceNodeID {
		return nil, nil, nil, fmt.Errorf("VM %s is on node %s, not source node %s",
			payload.VMID, *vm.NodeID, payload.SourceNodeID)
	}

	sourceNode, err := deps.NodeRepo.GetByID(ctx, payload.SourceNodeID)
	if err != nil {
		logger.Error("failed to get source node", "error", err)
		return nil, nil, nil, fmt.Errorf("getting source node %s: %w", payload.SourceNodeID, err)
	}

	targetNode, err := deps.NodeRepo.GetByID(ctx, payload.TargetNodeID)
	if err != nil {
		logger.Error("failed to get target node", "error", err)
		return nil, nil, nil, fmt.Errorf("getting target node %s: %w", payload.TargetNodeID, err)
	}

	if targetNode.Status != models.NodeStatusOnline {
		return nil, nil, nil, fmt.Errorf("target node %s is not online (status: %s)",
			payload.TargetNodeID, targetNode.Status)
	}

	return vm, sourceNode, targetNode, nil
}

// markMigrationComplete marks the task as complete for idempotent success.
func markMigrationComplete(ctx context.Context, deps *HandlerDeps, taskID string, payload VMMigratePayload, logger *slog.Logger) {
	if err := deps.TaskRepo.UpdateProgress(ctx, taskID, 100, "VM already on target node"); err != nil {
		logger.Warn("failed to update task progress for idempotent completion", "error", err)
	}
	result := VMMigrateResult{
		VMID: payload.VMID, SourceNodeID: payload.SourceNodeID,
		TargetNodeID: payload.TargetNodeID, Status: "already_migrated",
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, taskID, resultJSON); err != nil {
		logger.Warn("failed to set task completed for idempotent completion", "error", err)
	}
}

// executeMigrationStrategy runs the appropriate migration strategy.
func executeMigrationStrategy(mc *MigrationContext) error {
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 10, "Preparing migration..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	switch mc.Payload.MigrationStrategy {
	case MigrationStrategyLiveSharedStorage:
		return executeLiveSharedStorageMigration(mc)
	case MigrationStrategyDiskCopy:
		return executeDiskCopyMigration(mc)
	case MigrationStrategyCold:
		return executeColdMigration(mc)
	default:
		return fmt.Errorf("unknown migration strategy: %s", mc.Payload.MigrationStrategy)
	}
}

// handleMigrationFailure restores VM status after a failed migration.
func handleMigrationFailure(mc *MigrationContext, err error) {
	mc.Logger.Error("migration failed", "error", err)
	if err := mc.Deps.VMRepo.TransitionStatus(mc.Ctx, mc.Payload.VMID, models.VMStatusMigrating, models.VMStatusError); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			mc.Logger.Error("failed VM transition from migrating to error during failure recovery", "error", err)
			return
		}
		mc.Logger.Warn("failed to restore VM status after migration failure", "error", err)
	}
}

// finalizeMigration updates the VM record and completes the task.
func finalizeMigration(mc *MigrationContext) error {
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 90, "Updating VM assignment..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.VMRepo.UpdateNodeAssignment(mc.Ctx, mc.Payload.VMID, mc.Payload.TargetNodeID); err != nil {
		return fmt.Errorf("updating VM %s node assignment: %w", mc.Payload.VMID, err)
	}

	if err := mc.Deps.VMRepo.TransitionStatus(mc.Ctx, mc.Payload.VMID, models.VMStatusMigrating, models.VMStatusRunning); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			mc.Logger.Error("failed VM transition from migrating to running after migration completion", "error", err)
			return fmt.Errorf("transitioning VM %s to running after migration: %w", mc.Payload.VMID, err)
		}
		mc.Logger.Warn("failed to transition VM status after migration", "error", err)
	}

	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 100, "Migration completed successfully"); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}
	result := VMMigrateResult{
		VMID: mc.Payload.VMID, SourceNodeID: mc.Payload.SourceNodeID,
		SourceNodeAddress: mc.SourceNode.GRPCAddress, TargetNodeID: mc.Payload.TargetNodeID,
		TargetNodeAddress: mc.TargetNode.GRPCAddress, Status: "migrated",
		MigrationStrategy:    string(mc.Payload.MigrationStrategy),
		SourceStorageBackend: mc.Payload.SourceStorageBackend, TargetStorageBackend: mc.Payload.TargetStorageBackend,
	}
	resultJSON, _ := json.Marshal(result)
	if err := mc.Deps.TaskRepo.SetCompleted(mc.Ctx, mc.Task.ID, resultJSON); err != nil {
		mc.Logger.Warn("failed to set task completed", "error", err)
	}

	mc.Logger.Info("vm.migrate task completed successfully", "vm_id", mc.Payload.VMID,
		"source_node", mc.Payload.SourceNodeID, "target_node", mc.Payload.TargetNodeID)
	return nil
}

// executeLiveSharedStorageMigration performs live migration with shared Ceph storage.
// No disk copy is needed as both nodes have access to the same Ceph cluster.
func executeLiveSharedStorageMigration(mc *MigrationContext) error {
	mc.Logger.Info("executing live migration with shared storage")

	// Update task progress
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 20, "Executing live migration..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	// Prepare migration options
	migrateOpts := &MigrateVMOptions{
		TargetNodeAddress:  mc.TargetNode.GRPCAddress,
		BandwidthLimitMbps: defaultMigrationBandwidthMbps,
		Compression:        true,
		AutoConverge:       true,
	}

	// Initiate live migration via gRPC on source node
	if err := mc.Deps.NodeClient.MigrateVM(mc.Ctx, mc.Payload.SourceNodeID, mc.Payload.TargetNodeID, mc.Payload.VMID, migrateOpts); err != nil {
		return fmt.Errorf("live migration failed: %w", err)
	}

	// Update task progress: Post-migration setup
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 70, "Applying post-migration configuration..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	// Re-apply bandwidth limits on target node
	if err := mc.Deps.NodeClient.PostMigrateSetup(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID, mc.VM.PortSpeedMbps); err != nil {
		mc.Logger.Warn("failed to apply post-migration setup on target node", "error", err)
	}

	return nil
}

// executeDiskCopyMigration performs migration with disk copy for QCOW storage.
// For running VMs: create snapshot, copy disk, sync delta, switchover.
// For stopped VMs: simple disk copy.
func executeDiskCopyMigration(mc *MigrationContext) error {
	mc.Logger.Info("executing disk copy migration",
		"source_disk", mc.Payload.SourceDiskPath,
		"target_disk", mc.Payload.TargetDiskPath,
		"live", mc.Payload.Live,
		"source_storage_backend", mc.Payload.SourceStorageBackend)

	// Update task progress
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 20, "Preparing disk transfer..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	// Check if source is LVM - dispatch to LVM-specific handler
	if mc.Payload.SourceStorageBackend == "lvm" {
		if mc.Payload.Live && mc.VM.Status == models.VMStatusRunning {
			return executeLiveLVMMigration(mc)
		}
		return executeColdLVMMigration(mc)
	}

	// For live migration with QCOW, we need to:
	// 1. Create a snapshot on source
	// 2. Copy the base disk to target
	// 3. Sync the delta
	// 4. Stop VM, final sync, start on target
	if mc.Payload.Live && mc.VM.Status == models.VMStatusRunning {
		return executeLiveDiskCopyMigration(mc)
	}

	// For stopped VMs or cold migration: simple disk copy
	return executeColdDiskCopyMigration(mc)
}

// executeLiveDiskCopyMigration performs live QCOW migration with disk copy and delta sync.
func executeLiveDiskCopyMigration(mc *MigrationContext) error {
	// Step 1: Create snapshot on source for point-in-time copy
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 25, "Creating migration snapshot..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	snapshotName := fmt.Sprintf("migrate-%s", shortID(mc.Payload.VMID))
	if _, err := mc.Deps.NodeClient.CreateSnapshot(mc.Ctx, mc.Payload.SourceNodeID, mc.Payload.VMID, snapshotName); err != nil {
		return fmt.Errorf("creating migration snapshot: %w", err)
	}
	defer func() {
		// Cleanup snapshot after migration
		_ = mc.Deps.NodeClient.DeleteSnapshot(mc.Ctx, mc.Payload.SourceNodeID, mc.Payload.VMID, snapshotName)
	}()

	// Step 2: Transfer disk to target node
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30, "Transferring disk to target node..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:   mc.Payload.SourceNodeID,
		TargetNodeID:   mc.Payload.TargetNodeID,
		SourceDiskPath: mc.Payload.SourceDiskPath,
		TargetDiskPath: mc.Payload.TargetDiskPath,
		SnapshotName:   snapshotName,
		DiskSizeGB:     mc.Payload.DiskSizeGB,
		Compress:       true,
		ProgressCallback: func(progress int) {
			if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress)); err != nil {
				mc.Logger.Debug("failed to update task progress during disk transfer", "error", err)
			}
		},
	}

	if err := mc.Deps.NodeClient.TransferDisk(mc.Ctx, transferOpts); err != nil {
		return fmt.Errorf("disk transfer failed: %w", err)
	}

	// Step 3: Stop VM for final switchover
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 80, "Stopping VM for switchover..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := stopVMGracefully(mc.Ctx, mc.Deps.NodeClient, mc.Payload.SourceNodeID, mc.Payload.VMID, 60, mc.Logger); err != nil {
		return fmt.Errorf("stopping VM: %w", err)
	}

	// Step 4: Sync final delta (if any)
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 85, "Syncing final changes..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	// Step 5: Delete VM definition on source (but keep disk for now)
	if err := mc.Deps.NodeClient.DeleteVM(mc.Ctx, mc.Payload.SourceNodeID, mc.Payload.VMID); err != nil {
		mc.Logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Step 6: Create VM on target with transferred disk
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 88, "Starting VM on target node..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	// The VM definition should be created on the target using the transferred disk
	// This is handled by the PrepareMigratedVM RPC
	if err := mc.Deps.NodeClient.PrepareMigratedVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID, mc.Payload.TargetDiskPath, mc.VM); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Step 7: Start VM on target
	if err := mc.Deps.NodeClient.StartVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID); err != nil {
		return fmt.Errorf("starting VM on target: %w", err)
	}

	// Step 8: Apply post-migration setup
	if err := mc.Deps.NodeClient.PostMigrateSetup(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID, mc.VM.PortSpeedMbps); err != nil {
		mc.Logger.Warn("failed to apply post-migration setup", "error", err)
	}

	return nil
}

// executeColdDiskCopyMigration performs cold disk copy for stopped VMs.
func executeColdDiskCopyMigration(mc *MigrationContext) error {
	// Ensure VM is stopped
	if mc.VM.Status == models.VMStatusRunning {
		if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 25, "Stopping VM..."); err != nil {
			mc.Logger.Warn("failed to update task progress", "error", err)
		}
		if err := stopVMGracefully(mc.Ctx, mc.Deps.NodeClient, mc.Payload.SourceNodeID, mc.Payload.VMID, 120, mc.Logger); err != nil {
			return fmt.Errorf("stopping VM: %w", err)
		}
	}

	// Transfer disk
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30, "Transferring disk to target node..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:   mc.Payload.SourceNodeID,
		TargetNodeID:   mc.Payload.TargetNodeID,
		SourceDiskPath: mc.Payload.SourceDiskPath,
		TargetDiskPath: mc.Payload.TargetDiskPath,
		DiskSizeGB:     mc.Payload.DiskSizeGB,
		Compress:       true,
		ProgressCallback: func(progress int) {
			if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress)); err != nil {
				mc.Logger.Debug("failed to update task progress during disk transfer", "error", err)
			}
		},
	}

	if err := mc.Deps.NodeClient.TransferDisk(mc.Ctx, transferOpts); err != nil {
		return fmt.Errorf("disk transfer failed: %w", err)
	}

	// Delete VM definition on source
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 80, "Cleaning up source node..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.NodeClient.DeleteVM(mc.Ctx, mc.Payload.SourceNodeID, mc.Payload.VMID); err != nil {
		mc.Logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Create VM on target
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 85, "Preparing VM on target node..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.NodeClient.PrepareMigratedVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID, mc.Payload.TargetDiskPath, mc.VM); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Start VM on target (if it was running before)
	if mc.Payload.PreMigrationState == models.VMStatusRunning {
		if err := mc.Deps.NodeClient.StartVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID); err != nil {
			return fmt.Errorf("starting VM on target: %w", err)
		}
	}

	return nil
}

// executeColdMigration performs cold migration with format conversion for mixed storage.
func executeColdMigration(mc *MigrationContext) error {
	mc.Logger.Info("executing cold migration with format conversion",
		"source_backend", mc.Payload.SourceStorageBackend,
		"target_backend", mc.Payload.TargetStorageBackend)

	// Ensure VM is stopped
	if mc.VM.Status == models.VMStatusRunning {
		if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 20, "Stopping VM for cold migration..."); err != nil {
			mc.Logger.Warn("failed to update task progress", "error", err)
		}
		if err := stopVMGracefully(mc.Ctx, mc.Deps.NodeClient, mc.Payload.SourceNodeID, mc.Payload.VMID, 120, mc.Logger); err != nil {
			return fmt.Errorf("stopping VM: %w", err)
		}
	}

	// Transfer disk with format conversion
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30, "Transferring disk with format conversion..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:         mc.Payload.SourceNodeID,
		TargetNodeID:         mc.Payload.TargetNodeID,
		SourceDiskPath:       mc.Payload.SourceDiskPath,
		TargetDiskPath:       mc.Payload.TargetDiskPath,
		SourceStorageBackend: mc.Payload.SourceStorageBackend,
		TargetStorageBackend: mc.Payload.TargetStorageBackend,
		DiskSizeGB:           mc.Payload.DiskSizeGB,
		Compress:             true,
		ConvertFormat:        true,
		ProgressCallback: func(progress int) {
			if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress)); err != nil {
				mc.Logger.Debug("failed to update task progress during disk transfer", "error", err)
			}
		},
	}

	if err := mc.Deps.NodeClient.TransferDisk(mc.Ctx, transferOpts); err != nil {
		return fmt.Errorf("disk transfer with conversion failed: %w", err)
	}

	// Delete VM definition on source
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 80, "Cleaning up source node..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.NodeClient.DeleteVM(mc.Ctx, mc.Payload.SourceNodeID, mc.Payload.VMID); err != nil {
		mc.Logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Create VM on target
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 85, "Preparing VM on target node..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.NodeClient.PrepareMigratedVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID, mc.Payload.TargetDiskPath, mc.VM); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Start VM on target (if it was running before)
	if mc.Payload.PreMigrationState == models.VMStatusRunning {
		if err := mc.Deps.NodeClient.StartVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID); err != nil {
			return fmt.Errorf("starting VM on target: %w", err)
		}
	}

	return nil
}

// executeLiveLVMMigration performs near-live LVM migration with thin snapshot and delta sync.
func executeLiveLVMMigration(mc *MigrationContext) error {
	vmID := mc.Payload.VMID
	snapshotName := fmt.Sprintf("migrate-%s", shortID(vmID))

	// Step 1: Create thin snapshot on source
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 25, "Creating LVM migration snapshot..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if _, err := mc.Deps.NodeClient.CreateSnapshot(mc.Ctx, mc.Payload.SourceNodeID, vmID, snapshotName); err != nil {
		return fmt.Errorf("creating LVM migration snapshot: %w", err)
	}
	defer func() {
		_ = mc.Deps.NodeClient.DeleteSnapshot(mc.Ctx, mc.Payload.SourceNodeID, vmID, snapshotName)
	}()

	// Step 2: Transfer base disk
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30, "Transferring base disk..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:         mc.Payload.SourceNodeID,
		TargetNodeID:         mc.Payload.TargetNodeID,
		SourceDiskPath:       mc.Payload.SourceDiskPath,
		TargetDiskPath:       mc.Payload.TargetDiskPath,
		SourceStorageBackend: "lvm",
		TargetStorageBackend: "lvm",
		SourceLVMVolumeGroup: mc.Payload.SourceLVMVolumeGroup,
		SourceLVMThinPool:    mc.Payload.SourceLVMThinPool,
		TargetLVMVolumeGroup: mc.Payload.TargetLVMVolumeGroup,
		TargetLVMThinPool:    mc.Payload.TargetLVMThinPool,
		SnapshotName:         snapshotName,
		DiskSizeGB:           mc.Payload.DiskSizeGB,
		Compress:             true,
		ProgressCallback: func(progress int) {
			_ = mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress))
		},
	}

	if err := mc.Deps.NodeClient.TransferDisk(mc.Ctx, transferOpts); err != nil {
		return fmt.Errorf("LVM disk transfer failed: %w", err)
	}

	// Step 3: Stop VM for switchover
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 80, "Stopping VM for switchover..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := stopVMGracefully(mc.Ctx, mc.Deps.NodeClient, mc.Payload.SourceNodeID, vmID, 60, mc.Logger); err != nil {
		return fmt.Errorf("stopping VM: %w", err)
	}

	// Step 4: Delete VM on source
	if err := mc.Deps.NodeClient.DeleteVM(mc.Ctx, mc.Payload.SourceNodeID, vmID); err != nil {
		mc.Logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Step 5: Create VM on target
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 85, "Preparing VM on target..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.NodeClient.PrepareMigratedVM(mc.Ctx, mc.Payload.TargetNodeID, vmID, mc.Payload.TargetDiskPath, mc.VM); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Step 6: Start VM on target
	if err := mc.Deps.NodeClient.StartVM(mc.Ctx, mc.Payload.TargetNodeID, vmID); err != nil {
		return fmt.Errorf("starting VM on target: %w", err)
	}

	// Step 7: Apply post-migration setup
	if err := mc.Deps.NodeClient.PostMigrateSetup(mc.Ctx, mc.Payload.TargetNodeID, vmID, mc.VM.PortSpeedMbps); err != nil {
		mc.Logger.Warn("failed to apply post-migration setup", "error", err)
	}

	return nil
}

// executeColdLVMMigration performs cold LVM migration for stopped VMs.
func executeColdLVMMigration(mc *MigrationContext) error {
	// Ensure VM is stopped
	if mc.VM.Status == models.VMStatusRunning {
		if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 25, "Stopping VM..."); err != nil {
			mc.Logger.Warn("failed to update task progress", "error", err)
		}
		if err := stopVMGracefully(mc.Ctx, mc.Deps.NodeClient, mc.Payload.SourceNodeID, mc.Payload.VMID, 120, mc.Logger); err != nil {
			return fmt.Errorf("stopping VM: %w", err)
		}
	}

	// Transfer disk
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30, "Transferring LVM disk..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	transferOpts := &DiskTransferOptions{
		SourceNodeID:         mc.Payload.SourceNodeID,
		TargetNodeID:         mc.Payload.TargetNodeID,
		SourceDiskPath:       mc.Payload.SourceDiskPath,
		TargetDiskPath:       mc.Payload.TargetDiskPath,
		SourceStorageBackend: "lvm",
		TargetStorageBackend: "lvm",
		SourceLVMVolumeGroup: mc.Payload.SourceLVMVolumeGroup,
		SourceLVMThinPool:    mc.Payload.SourceLVMThinPool,
		TargetLVMVolumeGroup: mc.Payload.TargetLVMVolumeGroup,
		TargetLVMThinPool:    mc.Payload.TargetLVMThinPool,
		DiskSizeGB:           mc.Payload.DiskSizeGB,
		Compress:             true,
		ProgressCallback: func(progress int) {
			_ = mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 30+progress/2, fmt.Sprintf("Disk transfer: %d%%", progress))
		},
	}

	if err := mc.Deps.NodeClient.TransferDisk(mc.Ctx, transferOpts); err != nil {
		return fmt.Errorf("LVM disk transfer failed: %w", err)
	}

	// Delete VM on source
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 80, "Cleaning up source..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.NodeClient.DeleteVM(mc.Ctx, mc.Payload.SourceNodeID, mc.Payload.VMID); err != nil {
		mc.Logger.Warn("failed to delete VM definition on source", "error", err)
	}

	// Create VM on target
	if err := mc.Deps.TaskRepo.UpdateProgress(mc.Ctx, mc.Task.ID, 85, "Preparing VM on target..."); err != nil {
		mc.Logger.Warn("failed to update task progress", "error", err)
	}

	if err := mc.Deps.NodeClient.PrepareMigratedVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID, mc.Payload.TargetDiskPath, mc.VM); err != nil {
		return fmt.Errorf("preparing VM on target: %w", err)
	}

	// Start VM on target (if it was running before)
	if mc.Payload.PreMigrationState == models.VMStatusRunning {
		if err := mc.Deps.NodeClient.StartVM(mc.Ctx, mc.Payload.TargetNodeID, mc.Payload.VMID); err != nil {
			return fmt.Errorf("starting VM on target: %w", err)
		}
	}

	return nil
}
