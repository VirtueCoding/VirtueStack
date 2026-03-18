// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/config"
)

// DefaultSnapshotQuota is the default maximum number of snapshots per VM.
const DefaultSnapshotQuota = 10

var (
	// ErrSnapshotQuotaExceeded is returned when a VM has reached its snapshot quota.
	ErrSnapshotQuotaExceeded = fmt.Errorf("snapshot quota exceeded")
)

// NodeAgentClient interface for backup operations (subset of the full interface).
// This allows backup operations without depending on the full NodeAgentClient.
type BackupNodeAgentClient interface {
	// CreateSnapshot creates a Ceph RBD snapshot for a VM's disk.
	CreateSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error
	// DeleteSnapshot deletes a Ceph RBD snapshot for a VM's disk.
	DeleteSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error
	// RestoreSnapshot restores a VM from a Ceph RBD snapshot.
	RestoreSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error
	// CloneSnapshot clones a snapshot to a backup location.
	CloneSnapshot(ctx context.Context, nodeID, vmID, snapshotName, backupPath string) error
	// GetVMInfo returns basic VM info needed for backup operations.
	GetVMNodeID(ctx context.Context, vmID string) (nodeID string, err error)
	// CreateQCOWSnapshot creates a qemu-img internal snapshot for QCOW-backed VMs.
	CreateQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error
	// DeleteQCOWSnapshot deletes a qemu-img internal snapshot for QCOW-backed VMs.
	DeleteQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error
	// CreateQCOWBackup creates a backup file from a QCOW disk using qemu-img convert.
	// If snapshotName is provided, it exports from that specific snapshot.
	CreateQCOWBackup(ctx context.Context, nodeID, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error)
	// RestoreQCOWBackup restores a VM from a QCOW backup file.
	RestoreQCOWBackup(ctx context.Context, nodeID, vmID, backupPath, targetPath string) error
	// DeleteQCOWBackupFile deletes a QCOW backup file from the backup storage.
	DeleteQCOWBackupFile(ctx context.Context, nodeID, backupPath string) error
	// GetQCOWDiskInfo returns information about a QCOW disk including size.
	GetQCOWDiskInfo(ctx context.Context, nodeID, diskPath string) (*tasks.QCOWDiskInfo, error)
}

// BackupService provides business logic for managing VM backups and snapshots.
// It coordinates between the database, storage (Ceph or QCOW), and node agents.
type BackupService struct {
	backupRepo    *repository.BackupRepository
	snapshotRepo  *repository.BackupRepository // Same repo handles both
	vmRepo        *repository.VMRepository
	nodeAgent     BackupNodeAgentClient
	taskPublisher TaskPublisher
	backupPath    string
	logger        *slog.Logger
}

type BackupNodeAgentAdapter struct {
	nodeAgent *NodeAgentGRPCClient
	vmRepo    *repository.VMRepository
}

func NewBackupNodeAgentAdapter(nodeAgent *NodeAgentGRPCClient, vmRepo *repository.VMRepository) *BackupNodeAgentAdapter {
	return &BackupNodeAgentAdapter{
		nodeAgent: nodeAgent,
		vmRepo:    vmRepo,
	}
}

func (a *BackupNodeAgentAdapter) CreateSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	_, err := a.nodeAgent.CreateSnapshot(ctx, nodeID, vmID, snapshotName)
	return err
}

func (a *BackupNodeAgentAdapter) DeleteSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	return a.nodeAgent.DeleteSnapshot(ctx, nodeID, vmID, snapshotName)
}

func (a *BackupNodeAgentAdapter) RestoreSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	return a.nodeAgent.RestoreSnapshot(ctx, nodeID, vmID, snapshotName)
}

func (a *BackupNodeAgentAdapter) CloneSnapshot(ctx context.Context, nodeID, vmID, snapshotName, backupPath string) error {
	_, err := a.nodeAgent.CloneSnapshot(ctx, nodeID, vmID, snapshotName, backupPath)
	return err
}

func (a *BackupNodeAgentAdapter) GetVMNodeID(ctx context.Context, vmID string) (string, error) {
	vm, err := a.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return "", fmt.Errorf("getting VM %s: %w", vmID, err)
	}
	if vm.NodeID == nil {
		return "", fmt.Errorf("VM %s has no node assigned", vmID)
	}
	return *vm.NodeID, nil
}

func (a *BackupNodeAgentAdapter) CreateQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	return a.nodeAgent.CreateQCOWSnapshot(ctx, nodeID, vmID, diskPath, snapshotName)
}

func (a *BackupNodeAgentAdapter) DeleteQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	return a.nodeAgent.DeleteQCOWSnapshot(ctx, nodeID, vmID, diskPath, snapshotName)
}

func (a *BackupNodeAgentAdapter) CreateQCOWBackup(ctx context.Context, nodeID, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error) {
	return a.nodeAgent.CreateQCOWBackup(ctx, nodeID, vmID, diskPath, snapshotName, backupPath, compress)
}

func (a *BackupNodeAgentAdapter) RestoreQCOWBackup(ctx context.Context, nodeID, vmID, backupPath, targetPath string) error {
	return a.nodeAgent.RestoreQCOWBackup(ctx, nodeID, vmID, backupPath, targetPath)
}

func (a *BackupNodeAgentAdapter) DeleteQCOWBackupFile(ctx context.Context, nodeID, backupPath string) error {
	return a.nodeAgent.DeleteQCOWBackupFile(ctx, nodeID, backupPath)
}

func (a *BackupNodeAgentAdapter) GetQCOWDiskInfo(ctx context.Context, nodeID, diskPath string) (*tasks.QCOWDiskInfo, error) {
	return a.nodeAgent.GetQCOWDiskInfo(ctx, nodeID, diskPath)
}

const (
	storageBackendCeph = "ceph"
	storageBackendQCOW = "qcow"
)

// defaultBackupPath returns the backup storage path from the BACKUP_STORAGE_PATH
// environment variable, falling back to the standard path derived from the
// config constants (defaultStoragePath / DefaultBackupsDir).
func defaultBackupPath() string {
	if v := os.Getenv("BACKUP_STORAGE_PATH"); v != "" {
		return v
	}
	return filepath.Join("/var/lib/virtuestack", config.DefaultBackupsDir)
}

// BackupServiceConfig holds all dependencies for BackupService construction.
// Using a config struct keeps NewBackupService compliant with the ≤4-parameter
// constructor rule (QG-01) and makes future dependency additions non-breaking.
type BackupServiceConfig struct {
	BackupRepo    *repository.BackupRepository
	SnapshotRepo  *repository.BackupRepository
	VMRepo        *repository.VMRepository
	NodeAgent     BackupNodeAgentClient
	TaskPublisher TaskPublisher
	Logger        *slog.Logger
}

// NewBackupService creates a new BackupService with the given configuration.
// The backup storage path is resolved from the BACKUP_STORAGE_PATH environment
// variable; if unset it defaults to /var/lib/virtuestack/backups.
func NewBackupService(cfg BackupServiceConfig) *BackupService {
	backupPath := defaultBackupPath()
	return &BackupService{
		backupRepo:    cfg.BackupRepo,
		snapshotRepo:  cfg.SnapshotRepo,
		vmRepo:        cfg.VMRepo,
		nodeAgent:     cfg.NodeAgent,
		taskPublisher: cfg.TaskPublisher,
		backupPath:    backupPath,
		logger:        cfg.Logger.With("component", "backup-service"),
	}
}

// ============================================================================
// Backup Operations
// ============================================================================

// CreateBackup creates a full backup of a VM.
// The backup is stored in the configured backup storage location.
// For Ceph VMs: creates RBD snapshot and clones to backup pool.
// For QCOW VMs: creates qemu-img snapshot and copies to backup directory.
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

	backup := &models.Backup{
		ID:             uuid.New().String(),
		VMID:           vmID,
		Type:           "full",
		Status:         models.BackupStatusCreating,
		StorageBackend: storageBackend,
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

	if storageBackend == storageBackendQCOW {
		return s.createQCOWBackup(ctx, vm, backup, snapshotName, name)
	}
	return s.createCephBackup(ctx, vm, backup, snapshotName, name)
}

func (s *BackupService) createQCOWBackup(ctx context.Context, vm *models.VM, backup *models.Backup, snapshotName, name string) (*models.Backup, error) {
	nodeID := *vm.NodeID
	diskPath := vm.DiskPath
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

	if err := s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusCompleted); err != nil {
		s.logger.Warn("failed to update backup status", "backup_id", backup.ID, "error", err)
	}

	expiresAt := time.Now().AddDate(0, 0, 30)
	if err := s.backupRepo.SetBackupExpiration(ctx, backup.ID, expiresAt); err != nil {
		s.logger.Warn("failed to set backup expiration", "backup_id", backup.ID, "error", err)
	}

	s.logger.Info("QCOW backup created",
		"backup_id", backup.ID,
		"vm_id", vm.ID,
		"name", name,
		"file_path", backupFilePath,
		"size_bytes", sizeBytes)

	return backup, nil
}

func (s *BackupService) createCephBackup(ctx context.Context, vm *models.VM, backup *models.Backup, snapshotName, name string) (*models.Backup, error) {
	nodeID := *vm.NodeID

	if err := s.nodeAgent.CreateSnapshot(ctx, nodeID, vm.ID, snapshotName); err != nil {
		_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusFailed)
		return nil, fmt.Errorf("creating backup snapshot: %w", err)
	}

	backupPath := fmt.Sprintf("backups/%s/%s", vm.ID, backup.ID)
	if err := s.nodeAgent.CloneSnapshot(ctx, nodeID, vm.ID, snapshotName, backupPath); err != nil {
		_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusFailed)
		return nil, fmt.Errorf("cloning backup: %w", err)
	}

	backup.StoragePath = &backupPath
	backup.RBDSnapshot = &snapshotName

	if err := s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusCompleted); err != nil {
		s.logger.Warn("failed to update backup status", "backup_id", backup.ID, "error", err)
	}

	expiresAt := time.Now().AddDate(0, 0, 30)
	if err := s.backupRepo.SetBackupExpiration(ctx, backup.ID, expiresAt); err != nil {
		s.logger.Warn("failed to set backup expiration", "backup_id", backup.ID, "error", err)
	}

	s.logger.Info("Ceph backup created",
		"backup_id", backup.ID,
		"vm_id", vm.ID,
		"name", name,
		"type", "full")

	return backup, nil
}

// ListBackups returns all backups for a specific VM.
func (s *BackupService) ListBackups(ctx context.Context, vmID string) ([]models.Backup, error) {
	backups, err := s.backupRepo.ListBackupsByVM(ctx, vmID)
	if err != nil {
		return nil, fmt.Errorf("listing backups: %w", err)
	}
	return backups, nil
}

func (s *BackupService) ListBackupsWithFilter(ctx context.Context, customerID *string, filter repository.BackupListFilter) ([]models.Backup, int, error) {
	if customerID != nil && *customerID != "" {
		return s.backupRepo.ListBackupsByCustomer(ctx, *customerID, filter)
	}
	return s.backupRepo.ListBackups(ctx, filter)
}

// RestoreBackup restores a VM from a backup.
// This operation stops the VM, restores the disk, and leaves the VM stopped.
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

		diskPath := vm.DiskPath
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

// DeleteBackup removes a backup from storage and the database.
func (s *BackupService) DeleteBackup(ctx context.Context, backupID string) error {
	backup, err := s.backupRepo.GetBackupByID(ctx, backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("backup not found: %s", backupID)
		}
		return fmt.Errorf("getting backup: %w", err)
	}

	storageBackend := backup.StorageBackend
	if storageBackend == "" {
		storageBackend = storageBackendCeph
	}

	if s.nodeAgent != nil {
		vm, err := s.vmRepo.GetByID(ctx, backup.VMID)
		if err == nil && vm.NodeID != nil {
			nodeID := *vm.NodeID

			if storageBackend == storageBackendQCOW && backup.FilePath != nil {
				if err := s.nodeAgent.DeleteQCOWBackupFile(ctx, nodeID, *backup.FilePath); err != nil {
					s.logger.Warn("failed to delete QCOW backup file",
						"backup_id", backupID,
						"file_path", *backup.FilePath,
						"error", err)
				}
				if backup.SnapshotName != nil {
					diskPath := vm.DiskPath
					if diskPath == "" {
						diskPath = fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vm.ID)
					}
					if err := s.nodeAgent.DeleteQCOWSnapshot(ctx, nodeID, vm.ID, diskPath, *backup.SnapshotName); err != nil {
						s.logger.Warn("failed to delete QCOW snapshot",
							"backup_id", backupID,
							"snapshot_name", *backup.SnapshotName,
							"error", err)
					}
				}
			}
		}
	}

	if err := s.backupRepo.DeleteBackup(ctx, backupID); err != nil {
		return fmt.Errorf("deleting backup: %w", err)
	}

	s.logger.Info("backup deleted",
		"backup_id", backupID,
		"vm_id", backup.VMID,
		"storage_backend", storageBackend)

	return nil
}

// ============================================================================
// Snapshot Operations
// ============================================================================

// CreateSnapshot creates a point-in-time snapshot of a VM's disk.
// For Ceph VMs: creates RBD snapshot.
// For QCOW VMs: creates qemu-img internal snapshot.
func (s *BackupService) CreateSnapshot(ctx context.Context, vmID, name string) (*models.Snapshot, error) {
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

	snapshotID := uuid.New().String()

	snapshot := &models.Snapshot{
		ID:             snapshotID,
		VMID:           vmID,
		Name:           name,
		StorageBackend: storageBackend,
	}

	nodeID := *vm.NodeID

	if s.nodeAgent == nil {
		s.logger.Warn("nodeAgent not configured, skipping snapshot storage operation",
			"vm_id", vmID,
			"snapshot_id", snapshotID)
	} else {
		if storageBackend == storageBackendQCOW {
			diskPath := vm.DiskPath
			if diskPath == "" {
				diskPath = fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vmID)
			}

			qcowSnapshotName := fmt.Sprintf("snap-%s", snapshotID[:8])
			if err := s.nodeAgent.CreateQCOWSnapshot(ctx, nodeID, vmID, diskPath, qcowSnapshotName); err != nil {
				return nil, fmt.Errorf("creating QCOW snapshot: %w", err)
			}
			snapshot.QCOWSnapshot = &qcowSnapshotName
		} else {
			rbdSnapshot := fmt.Sprintf("snap-%s", snapshotID[:8])
			if err := s.nodeAgent.CreateSnapshot(ctx, nodeID, vmID, rbdSnapshot); err != nil {
				return nil, fmt.Errorf("creating snapshot: %w", err)
			}
			snapshot.RBDSnapshot = rbdSnapshot
		}
	}

	if err := s.snapshotRepo.CreateSnapshot(ctx, snapshot); err != nil {
		if s.nodeAgent != nil {
			if storageBackend == storageBackendQCOW && snapshot.QCOWSnapshot != nil {
				diskPath := vm.DiskPath
				if diskPath == "" {
					diskPath = fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vmID)
				}
				_ = s.nodeAgent.DeleteQCOWSnapshot(ctx, nodeID, vmID, diskPath, *snapshot.QCOWSnapshot)
			} else if snapshot.RBDSnapshot != "" {
				_ = s.nodeAgent.DeleteSnapshot(ctx, nodeID, vmID, snapshot.RBDSnapshot)
			}
		}
		return nil, fmt.Errorf("creating snapshot record: %w", err)
	}

	s.logger.Info("snapshot created",
		"snapshot_id", snapshot.ID,
		"vm_id", vmID,
		"name", name,
		"storage_backend", storageBackend)

	return snapshot, nil
}

// ListSnapshots returns all snapshots for a specific VM.
func (s *BackupService) ListSnapshots(ctx context.Context, vmID string) ([]models.Snapshot, error) {
	snapshots, err := s.snapshotRepo.ListSnapshotsByVM(ctx, vmID)
	if err != nil {
		return nil, fmt.Errorf("listing snapshots: %w", err)
	}
	return snapshots, nil
}

// DeleteSnapshot removes a snapshot from storage and the database.
func (s *BackupService) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	snapshot, err := s.snapshotRepo.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("snapshot not found: %s", snapshotID)
		}
		return fmt.Errorf("getting snapshot: %w", err)
	}

	vm, err := s.vmRepo.GetByID(ctx, snapshot.VMID)
	if err != nil {
		return fmt.Errorf("getting VM: %w", err)
	}

	if s.nodeAgent != nil && vm.NodeID != nil {
		nodeID := *vm.NodeID
		storageBackend := snapshot.StorageBackend
		if storageBackend == "" {
			storageBackend = storageBackendCeph
		}

		if storageBackend == storageBackendQCOW && snapshot.QCOWSnapshot != nil {
			diskPath := vm.DiskPath
			if diskPath == "" {
				diskPath = fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vm.ID)
			}
			if err := s.nodeAgent.DeleteQCOWSnapshot(ctx, nodeID, vm.ID, diskPath, *snapshot.QCOWSnapshot); err != nil {
				s.logger.Warn("failed to delete snapshot from storage",
					"snapshot_id", snapshotID,
					"error", err)
			}
		} else if snapshot.RBDSnapshot != "" {
			if err := s.nodeAgent.DeleteSnapshot(ctx, nodeID, snapshot.VMID, snapshot.RBDSnapshot); err != nil {
				s.logger.Warn("failed to delete snapshot from storage",
					"snapshot_id", snapshotID,
					"error", err)
			}
		}
	} else if s.nodeAgent == nil {
		s.logger.Warn("nodeAgent not configured, skipping snapshot storage deletion",
			"snapshot_id", snapshotID,
			"vm_id", snapshot.VMID)
	}

	if err := s.snapshotRepo.DeleteSnapshot(ctx, snapshotID); err != nil {
		return fmt.Errorf("deleting snapshot: %w", err)
	}

	s.logger.Info("snapshot deleted",
		"snapshot_id", snapshotID,
		"vm_id", snapshot.VMID,
		"name", snapshot.Name)

	return nil
}

// GetSnapshotCount returns the number of snapshots for a VM.
func (s *BackupService) GetSnapshotCount(ctx context.Context, vmID string) (int, error) {
	snapshots, err := s.snapshotRepo.ListSnapshotsByVM(ctx, vmID)
	if err != nil {
		return 0, fmt.Errorf("counting snapshots: %w", err)
	}
	return len(snapshots), nil
}

// CheckSnapshotQuota checks if a VM has reached its snapshot quota.
func (s *BackupService) CheckSnapshotQuota(ctx context.Context, vmID string, quota int) error {
	count, err := s.GetSnapshotCount(ctx, vmID)
	if err != nil {
		return fmt.Errorf("checking snapshot quota: %w", err)
	}
	if count >= quota {
		return fmt.Errorf("%w: VM has %d snapshots, quota is %d", ErrSnapshotQuotaExceeded, count, quota)
	}
	return nil
}

// CreateSnapshotAsync creates a snapshot asynchronously via NATS task.
func (s *BackupService) CreateSnapshotAsync(ctx context.Context, vmID, name, customerID string) (*models.Snapshot, string, error) {
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, "", fmt.Errorf("VM not found: %s", vmID)
		}
		return nil, "", fmt.Errorf("getting VM: %w", err)
	}

	if vm.NodeID == nil {
		return nil, "", fmt.Errorf("VM has no node assigned")
	}

	if err := s.CheckSnapshotQuota(ctx, vmID, DefaultSnapshotQuota); err != nil {
		return nil, "", err
	}

	storageBackend := vm.StorageBackend
	if storageBackend == "" {
		storageBackend = storageBackendCeph
	}

	snapshotID := uuid.New().String()

	snapshot := &models.Snapshot{
		ID:             snapshotID,
		VMID:           vmID,
		Name:           name,
		StorageBackend: storageBackend,
	}

	if storageBackend == storageBackendCeph {
		snapshot.RBDSnapshot = fmt.Sprintf("snap-%s", snapshotID[:8])
	} else {
		qcowSnap := fmt.Sprintf("snap-%s", snapshotID[:8])
		snapshot.QCOWSnapshot = &qcowSnap
	}

	if err := s.snapshotRepo.CreateSnapshot(ctx, snapshot); err != nil {
		return nil, "", fmt.Errorf("creating snapshot record: %w", err)
	}

	if s.taskPublisher != nil {
		payload := map[string]any{
			"vm_id":       vmID,
			"snapshot_id": snapshotID,
			"name":        name,
			"customer_id": customerID,
		}

		taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeSnapshotCreate, payload)
		if err != nil {
			_ = s.snapshotRepo.DeleteSnapshot(ctx, snapshotID)
			return nil, "", fmt.Errorf("publishing snapshot task: %w", err)
		}

		s.logger.Info("snapshot create task published",
			"snapshot_id", snapshotID,
			"vm_id", vmID,
			"task_id", taskID)

		return snapshot, taskID, nil
	}

	return snapshot, "", nil
}

// RevertSnapshotAsync reverts a VM to a snapshot asynchronously via NATS task.
func (s *BackupService) RevertSnapshotAsync(ctx context.Context, snapshotID, customerID string) (string, error) {
	// Get snapshot
	snapshot, err := s.snapshotRepo.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return "", fmt.Errorf("snapshot not found: %s", snapshotID)
		}
		return "", fmt.Errorf("getting snapshot: %w", err)
	}

	// Get VM
	vm, err := s.vmRepo.GetByID(ctx, snapshot.VMID)
	if err != nil {
		return "", fmt.Errorf("getting VM: %w", err)
	}

	if vm.NodeID == nil {
		return "", fmt.Errorf("VM has no node assigned")
	}

	// Publish task if task publisher is available
	if s.taskPublisher != nil {
		payload := map[string]any{
			"vm_id":       snapshot.VMID,
			"snapshot_id": snapshotID,
			"customer_id": customerID,
		}

		taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeSnapshotRevert, payload)
		if err != nil {
			return "", fmt.Errorf("publishing snapshot revert task: %w", err)
		}

		s.logger.Info("snapshot revert task published",
			"snapshot_id", snapshotID,
			"vm_id", snapshot.VMID,
			"task_id", taskID)

		return taskID, nil
	}

	return "", nil
}

// DeleteSnapshotAsync deletes a snapshot asynchronously via NATS task.
func (s *BackupService) DeleteSnapshotAsync(ctx context.Context, snapshotID, customerID string) (string, error) {
	// Get snapshot
	snapshot, err := s.snapshotRepo.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return "", fmt.Errorf("snapshot not found: %s", snapshotID)
		}
		return "", fmt.Errorf("getting snapshot: %w", err)
	}

	// Publish task if task publisher is available
	if s.taskPublisher != nil {
		payload := map[string]any{
			"vm_id":       snapshot.VMID,
			"snapshot_id": snapshotID,
			"customer_id": customerID,
		}

		taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeSnapshotDelete, payload)
		if err != nil {
			return "", fmt.Errorf("publishing snapshot delete task: %w", err)
		}

		s.logger.Info("snapshot delete task published",
			"snapshot_id", snapshotID,
			"vm_id", snapshot.VMID,
			"task_id", taskID)

		return taskID, nil
	}

	return "", nil
}

// ============================================================================
// Backup Scheduler
// ============================================================================

// Scheduler constants.
const (
	// schedulerInterval is how often the scheduler wakes up to check for backups.
	schedulerInterval = 1 * time.Hour
	// staggerDays is the number of days in a month used for staggering backups.
	staggerDays = 28
)

// StartScheduler starts the backup scheduler that runs periodically to create
// monthly backups for VMs. It uses a staggered approach where each VM is assigned
// a specific day of the month based on its VM ID hash, spreading the backup load.
// The scheduler runs until the context is cancelled.
func (s *BackupService) StartScheduler(ctx context.Context) {
	s.logger.Info("backup scheduler started")

	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.runSchedulerTick(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("backup scheduler stopped", "reason", ctx.Err())
			return
		case <-ticker.C:
			s.runSchedulerTick(ctx)
		}
	}
}

// runSchedulerTick performs a single iteration of the backup scheduler.
// It queries all active VMs and creates backup tasks for those that need
// a monthly backup and haven't been backed up this month.
func (s *BackupService) runSchedulerTick(ctx context.Context) {
	now := time.Now().UTC()
	currentDay := now.Day()
	currentYear, currentMonth, _ := now.Date()

	s.logger.Debug("running backup scheduler tick",
		"day", currentDay,
		"month", currentMonth,
		"year", currentYear)

	// Get all active VMs
	vms, err := s.vmRepo.ListAllActive(ctx)
	if err != nil {
		s.logger.Error("failed to list active VMs for backup scheduling", "error", err)
		return
	}

	backupCount := 0
	skippedCount := 0
	errorCount := 0

	for _, vm := range vms {
		// Check if this VM should be backed up today using staggered schedule
		assignedDay := s.getVMBackupDay(vm.ID)
		if assignedDay != currentDay {
			continue
		}

		// Check if VM already has a backup this month
		hasBackup, err := s.backupRepo.HasBackupInMonth(ctx, vm.ID, currentYear, int(currentMonth))
		if err != nil {
			s.logger.Error("failed to check backup status for VM",
				"vm_id", vm.ID,
				"error", err)
			errorCount++
			continue
		}

		if hasBackup {
			skippedCount++
			s.logger.Debug("VM already has backup this month, skipping",
				"vm_id", vm.ID,
				"assigned_day", assignedDay)
			continue
		}

		// Publish backup task
		if s.taskPublisher == nil {
			s.logger.Warn("task publisher not configured, cannot schedule backup",
				"vm_id", vm.ID)
			continue
		}

		taskID, err := s.taskPublisher.PublishTask(ctx, "backup.create", map[string]any{
			"vm_id":       vm.ID,
			"backup_name": fmt.Sprintf("monthly-%d-%02d", currentYear, currentMonth),
			"backup_type": "full",
		})
		if err != nil {
			s.logger.Error("failed to publish backup task for VM",
				"vm_id", vm.ID,
				"error", err)
			errorCount++
			continue
		}

		backupCount++
		s.logger.Info("scheduled monthly backup for VM",
			"vm_id", vm.ID,
			"task_id", taskID,
			"assigned_day", assignedDay)
	}

	s.logger.Info("backup scheduler tick completed",
		"total_vms", len(vms),
		"backups_scheduled", backupCount,
		"skipped_has_backup", skippedCount,
		"errors", errorCount)
}

// getVMBackupDay calculates the assigned backup day for a VM using a deterministic
// hash of the VM ID. This ensures VMs are evenly distributed across the first
// 28 days of each month (to handle February reliably).
func (s *BackupService) getVMBackupDay(vmID string) int {
	// Create SHA-256 hash of VM ID
	hash := sha256.Sum256([]byte(vmID))

	// Take first 8 bytes and convert to uint64
	hashValue := binary.BigEndian.Uint64(hash[:8])

	// Map to day 1-28 (inclusive)
	day := int(hashValue%staggerDays) + 1

	return day
}

func (s *BackupService) CreateSchedule(ctx context.Context, schedule *models.BackupSchedule) (string, error) {
	if schedule == nil {
		return "", fmt.Errorf("schedule is required")
	}
	if schedule.VMID == "" || schedule.CustomerID == "" {
		return "", fmt.Errorf("vm_id and customer_id are required")
	}

	frequency := strings.ToLower(strings.TrimSpace(schedule.Frequency))
	if frequency != "daily" && frequency != "weekly" && frequency != "monthly" {
		return "", fmt.Errorf("invalid frequency: %s", schedule.Frequency)
	}

	if schedule.RetentionCount <= 0 {
		schedule.RetentionCount = 30
	}

	schedule.Frequency = frequency
	schedule.NextRunAt = computeNextRun(time.Now().UTC(), frequency)
	schedule.Active = true

	if err := s.backupRepo.CreateBackupSchedule(ctx, schedule); err != nil {
		return "", fmt.Errorf("creating schedule: %w", err)
	}

	s.logger.Info("backup schedule created",
		"schedule_id", schedule.ID,
		"vm_id", schedule.VMID,
		"customer_id", schedule.CustomerID,
		"frequency", schedule.Frequency)

	return schedule.ID, nil
}

func (s *BackupService) ListSchedules(ctx context.Context, vmID string) ([]*models.BackupSchedule, error) {
	filter := repository.BackupScheduleListFilter{}
	if vmID != "" {
		filter.VMID = &vmID
	}

	schedules, _, err := s.backupRepo.ListBackupSchedules(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing schedules: %w", err)
	}

	result := make([]*models.BackupSchedule, 0, len(schedules))
	for i := range schedules {
		sched := schedules[i]
		result = append(result, &sched)
	}

	return result, nil
}

// ListSchedulesPaginated returns backup schedules with pagination support,
// returning the slice, total count, and any error.
func (s *BackupService) ListSchedulesPaginated(ctx context.Context, vmID string, pagination models.PaginationParams) ([]*models.BackupSchedule, int, error) {
	filter := repository.BackupScheduleListFilter{
		PaginationParams: pagination,
	}
	if vmID != "" {
		filter.VMID = &vmID
	}

	schedules, total, err := s.backupRepo.ListBackupSchedules(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("listing schedules: %w", err)
	}

	result := make([]*models.BackupSchedule, 0, len(schedules))
	for i := range schedules {
		sched := schedules[i]
		result = append(result, &sched)
	}

	return result, total, nil
}

func (s *BackupService) UpdateSchedule(ctx context.Context, scheduleID string, enabled bool) error {
	if err := s.backupRepo.UpdateBackupScheduleActive(ctx, scheduleID, enabled); err != nil {
		return fmt.Errorf("updating schedule: %w", err)
	}
	return nil
}

func (s *BackupService) DeleteSchedule(ctx context.Context, scheduleID string) error {
	if err := s.backupRepo.DeleteBackupSchedule(ctx, scheduleID); err != nil {
		return fmt.Errorf("deleting schedule: %w", err)
	}
	return nil
}

func (s *BackupService) ApplyRetentionPolicy(ctx context.Context, vmID string, retention int) error {
	if retention < 0 {
		return fmt.Errorf("retention must be >= 0")
	}

	backups, err := s.backupRepo.ListBackupsByVM(ctx, vmID)
	if err != nil {
		return fmt.Errorf("listing backups: %w", err)
	}

	if retention >= len(backups) {
		return nil
	}

	for i := retention; i < len(backups); i++ {
		if err := s.backupRepo.DeleteBackup(ctx, backups[i].ID); err != nil {
			return fmt.Errorf("deleting backup %s: %w", backups[i].ID, err)
		}
	}

	return nil
}

func (s *BackupService) ProcessExpiredBackups(ctx context.Context) (int, error) {
	expired, err := s.backupRepo.ListExpiredBackups(ctx, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("listing expired backups: %w", err)
	}

	deleted := 0
	for _, b := range expired {
		if err := s.backupRepo.DeleteBackup(ctx, b.ID); err != nil {
			s.logger.Warn("failed to delete expired backup", "backup_id", b.ID, "error", err)
			continue
		}
		deleted++
	}

	return deleted, nil
}

func (s *BackupService) BackupRepo() *repository.BackupRepository {
	return s.backupRepo
}

func (s *BackupService) RestoreSnapshot(ctx context.Context, snapshotID string) error {
	snapshot, err := s.snapshotRepo.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		return fmt.Errorf("getting snapshot: %w", err)
	}

	vm, err := s.vmRepo.GetByID(ctx, snapshot.VMID)
	if err != nil {
		return fmt.Errorf("getting VM: %w", err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	if s.nodeAgent == nil {
		s.logger.Warn("nodeAgent not configured, skipping snapshot restore",
			"snapshot_id", snapshotID,
			"vm_id", snapshot.VMID)
		return nil
	}

	nodeID := *vm.NodeID
	storageBackend := snapshot.StorageBackend
	if storageBackend == "" {
		storageBackend = storageBackendCeph
	}

	if storageBackend == storageBackendQCOW && snapshot.QCOWSnapshot != nil {
		return fmt.Errorf("QCOW snapshot revert not supported via this method - use backup restore")
	}

	if snapshot.RBDSnapshot != "" {
		if err := s.nodeAgent.RestoreSnapshot(ctx, nodeID, vm.ID, snapshot.RBDSnapshot); err != nil {
			return fmt.Errorf("restoring snapshot: %w", err)
		}
	}

	return nil
}

func (s *BackupService) ScheduleBackup(ctx context.Context, schedule *models.BackupSchedule) (string, error) {
	return s.CreateSchedule(ctx, schedule)
}

func (s *BackupService) GetSchedule(ctx context.Context, scheduleID string) (*models.BackupSchedule, error) {
	schedule, err := s.backupRepo.GetBackupScheduleByID(ctx, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("getting schedule: %w", err)
	}
	return schedule, nil
}

func (s *BackupService) PauseSchedule(ctx context.Context, scheduleID string) error {
	return s.UpdateSchedule(ctx, scheduleID, false)
}

func (s *BackupService) ResumeSchedule(ctx context.Context, scheduleID string) error {
	return s.UpdateSchedule(ctx, scheduleID, true)
}

func (s *BackupService) UpdateScheduleFrequency(ctx context.Context, scheduleID, frequency string) error {
	f := strings.ToLower(strings.TrimSpace(frequency))
	if f != "daily" && f != "weekly" && f != "monthly" {
		return fmt.Errorf("invalid frequency: %s", frequency)
	}
	nextRun := computeNextRun(time.Now().UTC(), f)
	if err := s.backupRepo.UpdateBackupScheduleFrequency(ctx, scheduleID, f, nextRun); err != nil {
		return fmt.Errorf("updating schedule frequency: %w", err)
	}
	return nil
}

func computeNextRun(now time.Time, frequency string) time.Time {
	switch frequency {
	case "daily":
		return now.Add(24 * time.Hour)
	case "weekly":
		return now.Add(7 * 24 * time.Hour)
	case "monthly":
		return now.AddDate(0, 1, 0)
	default:
		return now.Add(24 * time.Hour)
	}
}
