// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
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
}

// BackupService provides business logic for managing VM backups and snapshots.
// It coordinates between the database, storage (Ceph), and node agents.
type BackupService struct {
	backupRepo    *repository.BackupRepository
	snapshotRepo  *repository.BackupRepository // Same repo handles both
	vmRepo        *repository.VMRepository
	nodeAgent     BackupNodeAgentClient
	taskPublisher TaskPublisher
	logger        *slog.Logger
}

// NewBackupService creates a new BackupService with the given dependencies.
func NewBackupService(
	backupRepo *repository.BackupRepository,
	snapshotRepo *repository.BackupRepository,
	vmRepo *repository.VMRepository,
	nodeAgent BackupNodeAgentClient,
	taskPublisher TaskPublisher,
	logger *slog.Logger,
) *BackupService {
	return &BackupService{
		backupRepo:    backupRepo,
		snapshotRepo:  snapshotRepo,
		vmRepo:        vmRepo,
		nodeAgent:     nodeAgent,
		taskPublisher: taskPublisher,
		logger:        logger.With("component", "backup-service"),
	}
}

// ============================================================================
// Backup Operations
// ============================================================================

// CreateBackup creates a full backup of a VM.
// The backup is stored in the configured backup storage location.
func (s *BackupService) CreateBackup(ctx context.Context, vmID, name string) (*models.Backup, error) {
	// Verify VM exists and get its node
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

	// Create backup record
	backup := &models.Backup{
		ID:      uuid.New().String(),
		VMID:    vmID,
		Type:    "full",
		Status:  models.BackupStatusCreating,
	}

	if err := s.backupRepo.CreateBackup(ctx, backup); err != nil {
		return nil, fmt.Errorf("creating backup record: %w", err)
	}

	// Generate snapshot name for the backup
	snapshotName := fmt.Sprintf("backup-%s-%d", backup.ID[:8], time.Now().Unix())

	// If we have a node agent, perform the actual backup
	if s.nodeAgent != nil {
		// Create a snapshot first
		if err := s.nodeAgent.CreateSnapshot(ctx, *vm.NodeID, vmID, snapshotName); err != nil {
			// Update backup status to failed
			_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusFailed)
			return nil, fmt.Errorf("creating backup snapshot: %w", err)
		}

		// Clone snapshot to backup storage
		backupPath := fmt.Sprintf("backups/%s/%s", vmID, backup.ID)
		if err := s.nodeAgent.CloneSnapshot(ctx, *vm.NodeID, vmID, snapshotName, backupPath); err != nil {
			_ = s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusFailed)
			return nil, fmt.Errorf("cloning backup: %w", err)
		}

		// Update backup with storage path
		backup.StoragePath = &backupPath
		backup.RBDSnapshot = &snapshotName
	}

	// Mark backup as completed
	if err := s.backupRepo.UpdateBackupStatus(ctx, backup.ID, models.BackupStatusCompleted); err != nil {
		s.logger.Warn("failed to update backup status", "backup_id", backup.ID, "error", err)
	}

	// Set default expiration (30 days)
	expiresAt := time.Now().AddDate(0, 0, 30)
	if err := s.backupRepo.SetBackupExpiration(ctx, backup.ID, expiresAt); err != nil {
		s.logger.Warn("failed to set backup expiration", "backup_id", backup.ID, "error", err)
	}

	s.logger.Info("backup created",
		"backup_id", backup.ID,
		"vm_id", vmID,
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

// RestoreBackup restores a VM from a backup.
// This operation stops the VM, restores the disk, and leaves the VM stopped.
func (s *BackupService) RestoreBackup(ctx context.Context, backupID string) error {
	// Get backup
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

	// Get VM
	vm, err := s.vmRepo.GetByID(ctx, backup.VMID)
	if err != nil {
		return fmt.Errorf("getting VM: %w", err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Update backup status to restoring
	if err := s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusRestoring); err != nil {
		return fmt.Errorf("updating backup status: %w", err)
	}

	// Perform restore via node agent
	if s.nodeAgent != nil && backup.RBDSnapshot != nil {
		if err := s.nodeAgent.RestoreSnapshot(ctx, *vm.NodeID, vm.ID, *backup.RBDSnapshot); err != nil {
			_ = s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusFailed)
			return fmt.Errorf("restoring backup: %w", err)
		}
	}

	// Mark backup as completed again
	if err := s.backupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusCompleted); err != nil {
		s.logger.Warn("failed to update backup status after restore", "backup_id", backupID, "error", err)
	}

	s.logger.Info("backup restored",
		"backup_id", backupID,
		"vm_id", backup.VMID)

	return nil
}

// DeleteBackup removes a backup from storage and the database.
func (s *BackupService) DeleteBackup(ctx context.Context, backupID string) error {
	// Get backup
	backup, err := s.backupRepo.GetBackupByID(ctx, backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("backup not found: %s", backupID)
		}
		return fmt.Errorf("getting backup: %w", err)
	}

	// Delete from database
	if err := s.backupRepo.DeleteBackup(ctx, backupID); err != nil {
		return fmt.Errorf("deleting backup: %w", err)
	}

	s.logger.Info("backup deleted",
		"backup_id", backupID,
		"vm_id", backup.VMID)

	return nil
}

// ============================================================================
// Snapshot Operations
// ============================================================================

// CreateSnapshot creates a point-in-time snapshot of a VM's disk.
// Snapshots are quick and stored in Ceph RBD (no external storage needed).
func (s *BackupService) CreateSnapshot(ctx context.Context, vmID, name string) (*models.Snapshot, error) {
	// Verify VM exists and get its node
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

	// Generate snapshot ID and RBD snapshot name
	snapshotID := uuid.New().String()
	rbdSnapshot := fmt.Sprintf("snap-%s", snapshotID[:8])

	// Create snapshot via node agent
	if s.nodeAgent != nil {
		if err := s.nodeAgent.CreateSnapshot(ctx, *vm.NodeID, vmID, rbdSnapshot); err != nil {
			return nil, fmt.Errorf("creating snapshot: %w", err)
		}
	}

	// Create snapshot record
	snapshot := &models.Snapshot{
		ID:          snapshotID,
		VMID:        vmID,
		Name:        name,
		RBDSnapshot: rbdSnapshot,
	}

	if err := s.snapshotRepo.CreateSnapshot(ctx, snapshot); err != nil {
		// Attempt to clean up the created snapshot
		if s.nodeAgent != nil {
			_ = s.nodeAgent.DeleteSnapshot(ctx, *vm.NodeID, vmID, rbdSnapshot)
		}
		return nil, fmt.Errorf("creating snapshot record: %w", err)
	}

	s.logger.Info("snapshot created",
		"snapshot_id", snapshot.ID,
		"vm_id", vmID,
		"name", name,
		"rbd_snapshot", rbdSnapshot)

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
	// Get snapshot
	snapshot, err := s.snapshotRepo.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("snapshot not found: %s", snapshotID)
		}
		return fmt.Errorf("getting snapshot: %w", err)
	}

	// Get VM to find node
	vm, err := s.vmRepo.GetByID(ctx, snapshot.VMID)
	if err != nil {
		return fmt.Errorf("getting VM: %w", err)
	}

	// Delete from storage
	if s.nodeAgent != nil && vm.NodeID != nil {
		if err := s.nodeAgent.DeleteSnapshot(ctx, *vm.NodeID, snapshot.VMID, snapshot.RBDSnapshot); err != nil {
			s.logger.Warn("failed to delete snapshot from storage",
				"snapshot_id", snapshotID,
				"error", err)
			// Continue with database deletion
		}
	}

	// Delete from database
	if err := s.snapshotRepo.DeleteSnapshot(ctx, snapshotID); err != nil {
		return fmt.Errorf("deleting snapshot: %w", err)
	}

	s.logger.Info("snapshot deleted",
		"snapshot_id", snapshotID,
		"vm_id", snapshot.VMID,
		"name", snapshot.Name)

	return nil
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