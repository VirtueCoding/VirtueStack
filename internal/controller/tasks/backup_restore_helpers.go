// Package tasks provides helper functions for backup restore task handling.
// These functions decompose the handleBackupRestore function to comply with
// docs/coding-standard.md QG-01 (functions <= 40 lines).
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// backupRestoreInfo contains gathered info for backup restoration.
type backupRestoreInfo struct {
	backup *models.Backup
	vm     *models.VM
	nodeID string
}

// parseAndValidateBackupRestore parses payload and gathers backup/VM records.
func parseAndValidateBackupRestore(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	logger *slog.Logger,
) (*backupRestoreInfo, error) {
	var payload BackupRestorePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse backup.restore payload", "error", err)
		return nil, fmt.Errorf("parsing backup.restore payload: %w", err)
	}

	backup, err := deps.BackupRepo.GetBackupByID(ctx, payload.BackupID)
	if err != nil {
		return nil, fmt.Errorf("getting backup %s: %w", payload.BackupID, err)
	}

	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		return nil, fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return nil, fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}

	return &backupRestoreInfo{
		backup: backup,
		vm:     vm,
		nodeID: *vm.NodeID,
	}, nil
}

// stopVMForRestore stops the VM if running before backup restoration.
func stopVMForRestore(
	ctx context.Context,
	deps *HandlerDeps,
	info *backupRestoreInfo,
	vmID string,
	logger *slog.Logger,
) error {
	if info.vm.Status != models.VMStatusRunning {
		return nil
	}
	if err := stopVMGracefully(ctx, deps.NodeClient, info.nodeID, vmID, 120, logger); err != nil {
		return fmt.Errorf("stopping VM %s: %w", vmID, err)
	}
	return nil
}

// restoreFromBackup clones the disk from the backup snapshot.
func restoreFromBackup(
	ctx context.Context,
	deps *HandlerDeps,
	info *backupRestoreInfo,
	vmID string,
	logger *slog.Logger,
) error {
	// Delete current disk
	if err := deps.NodeClient.DeleteDisk(ctx, info.nodeID, vmID); err != nil {
		logger.Warn("failed to delete current disk", "error", err)
	}

	// Validate backup has snapshot
	if info.backup.RBDSnapshot == nil {
		return fmt.Errorf("backup has no RBD snapshot")
	}

	// Clone from backup snapshot
	if err := deps.NodeClient.CloneFromBackup(ctx, info.nodeID, vmID,
		*info.backup.RBDSnapshot, info.vm.DiskGB); err != nil {
		return fmt.Errorf("cloning from backup: %w", err)
	}
	return nil
}

// startAndFinalizeRestore starts the VM and updates statuses.
func startAndFinalizeRestore(
	ctx context.Context,
	deps *HandlerDeps,
	info *backupRestoreInfo,
	backupID, vmID string,
	logger *slog.Logger,
) error {
	// Start VM
	if err := deps.NodeClient.StartVM(ctx, info.nodeID, vmID); err != nil {
		return fmt.Errorf("starting VM %s: %w", vmID, err)
	}

	// Update VM status
	if err := deps.VMRepo.TransitionStatus(ctx, vmID, models.VMStatusStopped, models.VMStatusRunning); err != nil {
		logger.Warn("failed to update VM status", "error", err)
	}

	// Update backup status
	if err := deps.BackupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusCompleted); err != nil {
		logger.Warn("failed to update backup status", "error", err)
	}

	return nil
}