// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

func (s *BackupService) CreateBackup(ctx context.Context, vmID, name string) (*models.Backup, error) {
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("VM not found: %s", vmID)
		}
		return nil, fmt.Errorf("getting VM: %w", err)
	}

	if vm.NodeID == nil {
		return nil, fmt.Errorf("VM has no node assigned")
	}

	storageBackend := vm.StorageBackend
	if storageBackend == "" {
		storageBackend = storageBackendCeph
	}
	if storageBackend != storageBackendQCOW {
		return nil, unsupportedSyncFullBackupError(storageBackend)
	}

	backup := &models.Backup{
		ID:             uuid.New().String(),
		VMID:           vmID,
		Method:         models.BackupMethodFull,
		Source:         models.BackupSourceManual,
		Status:         models.BackupStatusCreating,
		StorageBackend: storageBackend,
	}
	if name != "" {
		backup.Name = util.StringPtr(name)
	}

	if err := s.backupRepo.CreateBackup(ctx, backup); err != nil {
		return nil, fmt.Errorf("creating backup record: %w", err)
	}

	snapshotName := fmt.Sprintf("backup-%s-%d", backup.ID[:8], time.Now().Unix())

	if s.nodeAgent == nil {
		s.logger.Warn("nodeAgent not configured, skipping backup storage operations",
			"vm_id", vmID,
			"backup_id", backup.ID)
		_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusCompleted)
		return backup, nil
	}

	return s.createQCOWBackup(ctx, vm, backup, snapshotName, name)
}

func (s *BackupService) CreateBackupWithLimitCheck(ctx context.Context, vmID, name string, limit int) (*models.Backup, error) {
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("VM not found: %s", vmID)
		}
		return nil, fmt.Errorf("getting VM: %w", err)
	}

	if vm.NodeID == nil {
		return nil, fmt.Errorf("VM has no node assigned")
	}

	storageBackend := vm.StorageBackend
	if storageBackend == "" {
		storageBackend = storageBackendCeph
	}
	if storageBackend != storageBackendQCOW {
		return nil, unsupportedSyncFullBackupError(storageBackend)
	}

	backup := &models.Backup{
		ID:             uuid.New().String(),
		VMID:           vmID,
		Method:         models.BackupMethodFull,
		Source:         models.BackupSourceManual,
		Status:         models.BackupStatusCreating,
		StorageBackend: storageBackend,
	}
	if name != "" {
		backup.Name = util.StringPtr(name)
	}

	// Use atomic create with limit check to prevent TOCTOU race condition
	if err := s.backupRepo.CreateBackupWithLimitCheck(ctx, backup, limit); err != nil {
		if strings.Contains(err.Error(), "limit exceeded") {
			return nil, fmt.Errorf("%w: %s", ErrBackupLimitExceeded, err.Error())
		}
		return nil, fmt.Errorf("creating backup record: %w", err)
	}

	snapshotName := fmt.Sprintf("backup-%s-%d", backup.ID[:8], time.Now().Unix())

	if s.nodeAgent == nil {
		s.logger.Warn("nodeAgent not configured, skipping backup storage operations",
			"vm_id", vmID,
			"backup_id", backup.ID)
		_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusCompleted)
		return backup, nil
	}

	return s.createQCOWBackup(ctx, vm, backup, snapshotName, name)
}

func (s *BackupService) createQCOWBackup(ctx context.Context, vm *models.VM, backup *models.Backup, snapshotName, name string) (*models.Backup, error) {
	nodeID := *vm.NodeID
	diskPath := ""
	if vm.DiskPath != nil {
		diskPath = *vm.DiskPath
	}
	if diskPath == "" {
		diskPath = fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vm.ID)
	}

	if err := s.nodeAgent.CreateQCOWSnapshot(ctx, nodeID, vm.ID, diskPath, snapshotName); err != nil {
		_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusFailed)
		return nil, fmt.Errorf("creating QCOW snapshot: %w", err)
	}

	backupDir := fmt.Sprintf("%s/%s", s.backupPath, vm.ID)
	backupFileName := fmt.Sprintf("%s-%s.qcow2", backup.ID[:8], time.Now().Format("20060102-150405"))
	backupFilePath := fmt.Sprintf("%s/%s", backupDir, backupFileName)

	sizeBytes, err := s.nodeAgent.CreateQCOWBackup(ctx, nodeID, vm.ID, diskPath, snapshotName, backupFilePath, false)
	if err != nil {
		_ = s.nodeAgent.DeleteQCOWSnapshot(ctx, nodeID, vm.ID, diskPath, snapshotName)
		_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusFailed)
		return nil, fmt.Errorf("creating QCOW backup file: %w", err)
	}

	backup.FilePath = &backupFilePath
	backup.SnapshotName = &snapshotName
	backup.SizeBytes = &sizeBytes
	if err := s.completeBackupRecord(ctx, backup); err != nil {
		return nil, err
	}

	s.logger.Info("QCOW backup created",
		"backup_id", backup.ID,
		"vm_id", vm.ID,
		"name", name,
		"file_path", backupFilePath,
		"size_bytes", sizeBytes)

	return backup, nil
}

func unsupportedSyncFullBackupError(storageBackend string) error {
	return fmt.Errorf(
		"synchronous full backups are not supported for storage backend %q: %w",
		storageBackend,
		sharederrors.ErrNotSupported,
	)
}

func (s *BackupService) completeBackupRecord(ctx context.Context, backup *models.Backup) error {
	expiresAt := time.Now().AddDate(0, 0, DefaultBackupRetentionDays)
	backup.Status = models.BackupStatusCompleted
	backup.ExpiresAt = &expiresAt
	if err := s.backupRepo.CompleteBackup(ctx, backup); err != nil {
		return fmt.Errorf("persisting backup completion: %w", err)
	}
	return nil
}
