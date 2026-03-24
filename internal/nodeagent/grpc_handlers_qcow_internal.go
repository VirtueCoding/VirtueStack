// Package nodeagent provides internal QCOW helper functions.
// This file contains internal helper methods for QCOW disk snapshot and backup operations.
package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
)

// createQCOWSnapshotInternal creates a qemu-img internal snapshot for QCOW-backed VMs.
func (h *grpcHandler) createQCOWSnapshotInternal(ctx context.Context, vmID, diskPath, snapshotName string) error {
	logger := h.server.logger.With("vm_id", vmID, "operation", "create-qcow-snapshot")
	logger.Info("creating QCOW snapshot", "disk_path", diskPath, "snapshot_name", snapshotName)

	if h.server.storageType == storage.StorageTypeQCOW {
		if err := h.server.storageBackend.CreateSnapshot(ctx, diskPath, snapshotName); err != nil {
			return fmt.Errorf("creating QCOW snapshot: %w", err)
		}
	} else {
		qcowManager, err := storage.NewQCOWManager(filepath.Dir(diskPath), h.server.logger)
		if err != nil {
			return fmt.Errorf("creating QCOW manager: %w", err)
		}
		if err := qcowManager.CreateSnapshot(ctx, diskPath, snapshotName); err != nil {
			return fmt.Errorf("creating QCOW snapshot: %w", err)
		}
	}

	logger.Info("QCOW snapshot created successfully")
	return nil
}

// deleteQCOWSnapshotInternal deletes a qemu-img internal snapshot for QCOW-backed VMs.
func (h *grpcHandler) deleteQCOWSnapshotInternal(ctx context.Context, vmID, diskPath, snapshotName string) error {
	logger := h.server.logger.With("vm_id", vmID, "operation", "delete-qcow-snapshot")
	logger.Info("deleting QCOW snapshot", "disk_path", diskPath, "snapshot_name", snapshotName)

	if h.server.storageType == storage.StorageTypeQCOW {
		if err := h.server.storageBackend.DeleteSnapshot(ctx, diskPath, snapshotName); err != nil {
			logger.Warn("failed to delete QCOW snapshot", "error", err)
		}
	} else {
		qcowManager, err := storage.NewQCOWManager(filepath.Dir(diskPath), h.server.logger)
		if err != nil {
			return fmt.Errorf("creating QCOW manager: %w", err)
		}
		if err := qcowManager.DeleteSnapshot(ctx, diskPath, snapshotName); err != nil {
			logger.Warn("failed to delete QCOW snapshot", "error", err)
		}
	}

	return nil
}

// deleteQCOWBackupFileInternal deletes a QCOW backup file from the backup storage.
func (h *grpcHandler) deleteQCOWBackupFileInternal(ctx context.Context, backupPath string) error {
	logger := h.server.logger.With("operation", "delete-qcow-backup-file")
	logger.Info("deleting QCOW backup file", "backup_path", backupPath)

	if err := os.Remove(backupPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("deleting QCOW backup file: %w", err)
		}
		logger.Warn("backup file already deleted", "error", err)
	}

	logger.Info("QCOW backup file deleted successfully")
	return nil
}

// getQCOWDiskInfoInternal returns information about a QCOW disk including size.
func (h *grpcHandler) getQCOWDiskInfoInternal(ctx context.Context, diskPath string) (map[string]interface{}, error) {
	logger := h.server.logger.With("operation", "get-qcow-disk-info")
	logger.Debug("getting QCOW disk info", "disk_path", diskPath)

	args := []string{"info", "--output=json", diskPath}

	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting QCOW disk info: %w", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("parsing QCOW disk info: %w", err)
	}

	return info, nil
}
