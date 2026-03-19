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

const (
	qcowBackupBasePath = "/var/lib/virtuestack/backups"
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

// createQCOWBackupInternal creates a backup file from a QCOW disk using qemu-img convert.
func (h *grpcHandler) createQCOWBackupInternal(ctx context.Context, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error) {
	logger := h.server.logger.With("vm_id", vmID, "operation", "create-qcow-backup")
	logger.Info("creating QCOW backup",
		"disk_path", diskPath,
		"backup_path", backupPath,
		"snapshot_name", snapshotName,
		"compress", compress)

	backupDir := filepath.Dir(backupPath)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return 0, fmt.Errorf("creating backup directory: %w", err)
	}

	args := []string{"convert", "-f", "qcow2", "-O", "qcow2"}

	if snapshotName != "" {
		args = append(args, "-l", snapshotName)
	}

	if compress {
		args = append(args, "-c")
	}

	args = append(args, diskPath, backupPath)

	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("creating QCOW backup: %w, output: %s", err, string(output))
	}

	fileInfo, err := os.Stat(backupPath)
	if err != nil {
		return 0, fmt.Errorf("getting backup file info: %w", err)
	}

	sizeBytes := fileInfo.Size()

	logger.Info("QCOW backup created successfully", "size_bytes", sizeBytes)
	return sizeBytes, nil
}

// restoreQCOWBackupInternal restores a VM from a QCOW backup file.
func (h *grpcHandler) restoreQCOWBackupInternal(ctx context.Context, vmID, backupPath, targetPath string) error {
	logger := h.server.logger.With("vm_id", vmID, "operation", "restore-qcow-backup")
	logger.Info("restoring QCOW backup",
		"backup_path", backupPath,
		"target_path", targetPath)

	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}

	args := []string{"convert", "-f", "qcow2", "-O", "qcow2", backupPath, targetPath}

	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restoring QCOW backup: %w, output: %s", err, string(output))
	}

	logger.Info("QCOW backup restored successfully")
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
