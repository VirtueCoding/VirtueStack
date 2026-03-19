// Package tasks provides the backup restore task handler.
// This file contains the handleBackupRestore function which handles the
// backup restoration flow including stopping the VM, replacing the disk,
// and restarting the VM.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// handleBackupRestore handles the backup restoration flow.
// Steps:
//  1. Parse payload
//  2. Get backup and VM records
//  3. Stop target VM
//  4. Delete current RBD volume
//  5. Clone from backup snapshot
//  6. Start VM
func handleBackupRestore(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeBackupRestore)

	// Parse payload
	var payload BackupRestorePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse backup.restore payload", "error", err)
		return fmt.Errorf("parsing backup.restore payload: %w", err)
	}

	logger = logger.With("backup_id", payload.BackupID, "vm_id", payload.VMID)
	logger.Info("backup.restore task started")

	// Update task progress
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting backup restoration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get backup record
	backup, err := deps.BackupRepo.GetBackupByID(ctx, payload.BackupID)
	if err != nil {
		logger.Error("failed to get backup record", "error", err)
		return fmt.Errorf("getting backup %s: %w", payload.BackupID, err)
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	nodeID := *vm.NodeID

	// Update task progress: Stopping VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Stopping virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Stop VM
	if vm.Status == models.VMStatusRunning {
		if err := stopVMGracefully(ctx, deps.NodeClient, nodeID, payload.VMID, 120, logger); err != nil {
			logger.Error("failed to stop VM for backup restore", "error", err)
			return fmt.Errorf("stopping VM %s: %w", payload.VMID, err)
		}
	}

	// Update task progress: Deleting current disk
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Removing current disk..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Delete current disk
	if err := deps.NodeClient.DeleteDisk(ctx, nodeID, payload.VMID); err != nil {
		logger.Warn("failed to delete current disk", "error", err)
		// Continue anyway
	}

	// Update task progress: Cloning from backup
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Restoring from backup..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Clone from backup snapshot
	if backup.RBDSnapshot == nil {
		return fmt.Errorf("backup %s has no RBD snapshot", payload.BackupID)
	}

	if err := deps.NodeClient.CloneFromBackup(ctx, nodeID, payload.VMID, *backup.RBDSnapshot, vm.DiskGB); err != nil {
		logger.Error("failed to clone from backup", "error", err)
		return fmt.Errorf("cloning from backup %s: %w", payload.BackupID, err)
	}

	// Update task progress: Starting VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Starting virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Start VM
	if err := deps.NodeClient.StartVM(ctx, nodeID, payload.VMID); err != nil {
		logger.Error("failed to start VM", "error", err)
		return fmt.Errorf("starting VM %s: %w", payload.VMID, err)
	}

	// Update VM status
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusRunning); err != nil {
		logger.Warn("failed to update VM status", "error", err)
	}

	// Update backup status
	if err := deps.BackupRepo.UpdateBackupStatus(ctx, payload.BackupID, models.BackupStatusCompleted); err != nil {
		logger.Warn("failed to update backup status", "error", err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Backup restored successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"backup_id": payload.BackupID,
		"vm_id":     payload.VMID,
		"status":    "restored",
	}
	// json.Marshal error is intentionally suppressed: the map contains only
	// primitive types (string, int, bool) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("backup.restore task completed successfully",
		"backup_id", payload.BackupID,
		"vm_id", payload.VMID)

	return nil
}