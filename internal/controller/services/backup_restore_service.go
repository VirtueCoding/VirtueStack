// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

func (s *BackupService) RestoreBackup(ctx context.Context, backupID string) error {
	backup, err := s.backupRepo.GetBackupByID(ctx, backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("backup not found: %s", backupID)
		}
		return fmt.Errorf("getting backup: %w", err)
	}

	if backup.Status != models.BackupStatusCompleted {
		return fmt.Errorf("backup is not in completed state (status: %s)", backup.Status)
	}

	vm, err := s.vmRepo.GetByID(ctx, backup.VMID)
	if err != nil {
		return fmt.Errorf("getting VM: %w", err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	if err := s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusRestoring); err != nil {
		return fmt.Errorf("updating backup status: %w", err)
	}

	storageBackend := backup.StorageBackend
	if storageBackend == "" {
		storageBackend = storageBackendCeph
	}

	if s.nodeAgent == nil {
		s.logger.Warn("nodeAgent not configured, skipping backup restore",
			"backup_id", backupID,
			"vm_id", backup.VMID)
		_ = s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusFailed)
		return fmt.Errorf("node agent not configured, cannot restore backup %s", backupID)
	}

	nodeID := *vm.NodeID

	if storageBackend == storageBackendQCOW {
		if backup.FilePath == nil {
			_ = s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusFailed)
			return fmt.Errorf("backup has no file path for QCOW restore")
		}

		diskPath := ""
		if vm.DiskPath != nil {
			diskPath = *vm.DiskPath
		}
		if diskPath == "" {
			diskPath = fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vm.ID)
		}

		if err := s.nodeAgent.RestoreQCOWBackup(ctx, nodeID, vm.ID, *backup.FilePath, diskPath); err != nil {
			_ = s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusFailed)
			return fmt.Errorf("restoring QCOW backup: %w", err)
		}
	} else {
		if backup.RBDSnapshot == nil {
			_ = s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusFailed)
			return fmt.Errorf("backup has no RBD snapshot for Ceph restore")
		}

		if err := s.nodeAgent.RestoreSnapshot(ctx, nodeID, vm.ID, *backup.RBDSnapshot); err != nil {
			_ = s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusFailed)
			return fmt.Errorf("restoring backup: %w", err)
		}
	}

	if err := s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusCompleted); err != nil {
		s.logger.Warn("failed to update backup status after restore", "backup_id", backupID, "error", err)
	}

	s.logger.Info("backup restored",
		"backup_id", backupID,
		"vm_id", backup.VMID,
		"storage_backend", storageBackend)

	return nil
}
