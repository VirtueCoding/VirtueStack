// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	controllermetrics "github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// Constants for backup operations.
const (
	GuestAgentFreezeTimeout     = 10 * time.Second
	DefaultBackupExpirationDays = 30
	BackupPoolName              = "vs-backups"
	storageBackendCeph          = "ceph"
	storageBackendQCOW          = "qcow"
	defaultBackupBasePath       = "/var/lib/virtuestack/backups"
	defaultVMDiskPathTemplate   = "/var/lib/virtuestack/vms/%s-disk0.qcow2"
)

// BackupConfig holds configuration for backup operations.
type BackupConfig struct {
	BackupPath string
}

// BackupHandlerContext holds common context for backup handler functions.
// It groups related parameters to comply with QG-01 (max 4 parameters).
type BackupHandlerContext struct {
	Task            *models.Task
	VM              *models.VM
	NodeID          string
	Payload         BackupCreatePayload
	FreezeSuccessful bool
	FrozenCount     int
	Logger          *slog.Logger
}

// completeBackupTask marks a backup task as 100% complete, persists the result JSON,
// and emits a structured log. Both QCOW and Ceph paths share this final sequence.
func completeBackupTask(ctx context.Context, task *models.Task, deps *HandlerDeps, backup *models.Backup, result BackupCreateResult, logger *slog.Logger) {
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Backup created successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// json.Marshal error is intentionally suppressed: the struct contains only
	// primitive types (string, int) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("backup.create task completed successfully",
		"backup_id", backup.ID,
		"storage_backend", result.StorageBackend)
}

// DefaultBackupConfig returns the default backup configuration.
func DefaultBackupConfig() *BackupConfig {
	return &BackupConfig{
		BackupPath: defaultBackupBasePath,
	}
}

// handleBackupCreate handles the backup creation flow with QEMU guest agent integration.
//
// This handler implements application-consistent backup using QEMU guest agent:
// 1. Parse payload and validate
// 2. Check idempotency (skip if already completed)
// 3. Determine storage backend (Ceph or QCOW)
// 4. Request filesystem freeze via QEMU guest agent (with strict timeout)
// 5. If freeze fails/times out, proceed with crash-consistent backup
// 6. For Ceph: Create RBD snapshot, protect, clone to backup pool
// 7. For QCOW: Create qemu-img snapshot, convert/copy to backup directory
// 8. Thaw filesystems via QEMU guest agent
// 9. Create backup record in database
// 10. Update task progress to completed
func handleBackupCreate(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeBackupCreate)

	var payload BackupCreatePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse backup.create payload", "error", err)
		return fmt.Errorf("parsing backup.create payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID, "backup_name", payload.BackupName)
	logger.Info("backup.create task started", "source", payload.Source)

	backupStart := time.Now()
	defer func() {
		controllermetrics.BackupDuration.WithLabelValues(payload.Source).Observe(time.Since(backupStart).Seconds())
	}()

	if task.IsTerminal() {
		logger.Info("task already in terminal state, skipping",
			"status", task.Status)
		return nil
	}

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting backup creation..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	nodeID := *vm.NodeID

	storageBackend := vm.StorageBackend
	if storageBackend == "" {
		storageBackend = storageBackendCeph
	}

	logger = logger.With("storage_backend", storageBackend)

	var frozenCount int
	var freezeSuccessful bool

	freezeCtx, freezeCancel := context.WithTimeout(ctx, GuestAgentFreezeTimeout)
	defer freezeCancel()

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Freezing filesystems via guest agent..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	frozenCount, err = deps.NodeClient.GuestFreezeFilesystems(freezeCtx, nodeID, payload.VMID)
	if err != nil {
		logger.Warn("guest agent freeze failed or timed out, proceeding with crash-consistent backup",
			"error", err,
			"timeout", GuestAgentFreezeTimeout)
		freezeSuccessful = false
	} else {
		logger.Info("filesystems frozen successfully",
			"frozen_count", frozenCount)
		freezeSuccessful = true
	}

	defer func() {
		if freezeSuccessful && frozenCount > 0 {
			// context.Background() is intentional here: the parent task context may already
			// be cancelled or near expiry by the time the defer runs, but we must always
			// attempt to thaw the filesystems to avoid leaving the guest in a frozen state.
			thawCtx, thawCancel := context.WithTimeout(context.Background(), GuestAgentFreezeTimeout)
			defer thawCancel()

			thawedCount, thawErr := deps.NodeClient.GuestThawFilesystems(thawCtx, nodeID, payload.VMID)
			if thawErr != nil {
				logger.Error("failed to thaw filesystems after backup",
					"error", thawErr)
			} else {
				logger.Info("filesystems thawed successfully",
					"thawed_count", thawedCount)
			}
		}
	}()

	backupCtx := &BackupHandlerContext{
		Task:             task,
		VM:               vm,
		NodeID:           nodeID,
		Payload:          payload,
		FreezeSuccessful: freezeSuccessful,
		FrozenCount:      frozenCount,
		Logger:           logger,
	}

	if storageBackend == storageBackendQCOW {
		return handleQCOWBackupCreate(ctx, deps, backupCtx)
	}
	return handleCephBackupCreate(ctx, deps, backupCtx)
}

func handleQCOWBackupCreate(ctx context.Context, deps *HandlerDeps, backupCtx *BackupHandlerContext) error {
	task := backupCtx.Task
	vm := backupCtx.VM
	nodeID := backupCtx.NodeID
	payload := backupCtx.Payload
	freezeSuccessful := backupCtx.FreezeSuccessful
	frozenCount := backupCtx.FrozenCount
	logger := backupCtx.Logger

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Creating QCOW snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	diskPath := ""
	if vm.DiskPath != nil {
		diskPath = *vm.DiskPath
	}
	if diskPath == "" {
		diskPath = fmt.Sprintf(defaultVMDiskPathTemplate, vm.ID)
	}

	snapshotName := fmt.Sprintf("backup-%s-%d", payload.VMID, time.Now().Unix())

	if err := deps.NodeClient.CreateQCOWSnapshot(ctx, nodeID, payload.VMID, diskPath, snapshotName); err != nil {
		logger.Error("failed to create QCOW snapshot", "error", err)
		return fmt.Errorf("creating QCOW snapshot for VM %s: %w", payload.VMID, err)
	}

	logger.Info("QCOW snapshot created successfully", "snapshot_name", snapshotName)

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Creating backup file..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	backupConfig := DefaultBackupConfig()
	backupDir := fmt.Sprintf("%s/%s", backupConfig.BackupPath, payload.VMID)
	backupFileName := fmt.Sprintf("%s-%s.qcow2", shortID(task.ID), time.Now().Format("20060102-150405"))
	backupFilePath := fmt.Sprintf("%s/%s", backupDir, backupFileName)

	sizeBytes, err := deps.NodeClient.CreateQCOWBackup(ctx, nodeID, payload.VMID, diskPath, snapshotName, backupFilePath, false)
	if err != nil {
		logger.Error("failed to create QCOW backup file", "error", err)
		if delErr := deps.NodeClient.DeleteQCOWSnapshot(ctx, nodeID, payload.VMID, diskPath, snapshotName); delErr != nil {
			logger.Error("failed to cleanup QCOW snapshot after backup failure", "operation", "DeleteQCOWSnapshot", "err", delErr)
		}
		return fmt.Errorf("creating QCOW backup file for VM %s: %w", payload.VMID, err)
	}

	logger.Info("QCOW backup file created successfully",
		"backup_file", backupFilePath,
		"size_bytes", sizeBytes)

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Creating backup record..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	backupConsistency := "crash-consistent"
	if freezeSuccessful {
		backupConsistency = "application-consistent"
	}

	backup := &models.Backup{
		VMID:           payload.VMID,
		Source:         payload.Source,
		StorageBackend: storageBackendQCOW,
		FilePath:       &backupFilePath,
		SnapshotName:   &snapshotName,
		SizeBytes:      &sizeBytes,
		Status:         models.BackupStatusCompleted,
	}

	if payload.AdminScheduleID != "" {
		backup.AdminScheduleID = &payload.AdminScheduleID
	}

	expiresAt := time.Now().AddDate(0, 0, DefaultBackupExpirationDays)
	backup.ExpiresAt = &expiresAt

	if err := deps.BackupRepo.CreateBackup(ctx, backup); err != nil {
		logger.Error("failed to create backup record", "error", err)
		if delErr := deps.NodeClient.DeleteQCOWBackupFile(ctx, nodeID, backupFilePath); delErr != nil {
			logger.Error("failed to cleanup backup file after record failure", "operation", "DeleteQCOWBackupFile", "err", delErr)
		}
		if delErr := deps.NodeClient.DeleteQCOWSnapshot(ctx, nodeID, payload.VMID, diskPath, snapshotName); delErr != nil {
			logger.Error("failed to cleanup QCOW snapshot after record failure", "operation", "DeleteQCOWSnapshot", "err", delErr)
		}
		return fmt.Errorf("creating backup record: %w", err)
	}

	result := BackupCreateResult{
		BackupID:          backup.ID,
		VMID:              payload.VMID,
		SnapshotName:      snapshotName,
		Filepath:          backupFilePath,
		SizeBytes:         sizeBytes,
		Consistency:       backupConsistency,
		FrozenFilesystems: frozenCount,
		StorageBackend:    storageBackendQCOW,
	}
	completeBackupTask(ctx, task, deps, backup, result, logger)

	return nil
}

func handleCephBackupCreate(ctx context.Context, deps *HandlerDeps, backupCtx *BackupHandlerContext) error {
	task := backupCtx.Task
	nodeID := backupCtx.NodeID
	payload := backupCtx.Payload
	freezeSuccessful := backupCtx.FreezeSuccessful
	frozenCount := backupCtx.FrozenCount
	logger := backupCtx.Logger

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Creating disk snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	snapshotName := fmt.Sprintf("backup-%s-%d", payload.VMID, time.Now().Unix())

	snapshotResp, err := deps.NodeClient.CreateSnapshot(ctx, nodeID, payload.VMID, snapshotName)
	if err != nil {
		logger.Error("failed to create snapshot", "error", err)
		return fmt.Errorf("creating snapshot for VM %s: %w", payload.VMID, err)
	}

	logger.Info("RBD snapshot created successfully",
		"snapshot_name", snapshotResp.RBDSnapshotName,
		"size_bytes", snapshotResp.SizeBytes)

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 40, "Protecting snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.ProtectSnapshot(ctx, nodeID, payload.VMID, snapshotResp.RBDSnapshotName); err != nil {
		logger.Error("failed to protect snapshot", "error", err)
		return fmt.Errorf("protecting snapshot for VM %s: %w", payload.VMID, err)
	}

	logger.Info("snapshot protected successfully", "snapshot_name", snapshotResp.RBDSnapshotName)

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Cloning to backup storage..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	backupImageName := fmt.Sprintf("vs-%s-%d-backup", payload.VMID, time.Now().Unix())

	clonedImageName, err := deps.NodeClient.CloneSnapshot(ctx, nodeID, payload.VMID, snapshotResp.RBDSnapshotName, BackupPoolName)
	if err != nil {
		logger.Error("failed to clone snapshot to backup pool", "error", err)
		if unprotectErr := deps.NodeClient.UnprotectSnapshot(ctx, nodeID, payload.VMID, snapshotResp.RBDSnapshotName); unprotectErr != nil {
			logger.Warn("failed to unprotect snapshot during cleanup", "error", unprotectErr)
		}
		return fmt.Errorf("cloning snapshot to backup pool for VM %s: %w", payload.VMID, err)
	}

	if clonedImageName != "" {
		backupImageName = clonedImageName
	}

	logger.Info("snapshot cloned to backup pool successfully",
		"backup_image", backupImageName,
		"backup_pool", BackupPoolName)

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Creating backup record..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	backupConsistency := "crash-consistent"
	if freezeSuccessful {
		backupConsistency = "application-consistent"
	}

	storagePath := fmt.Sprintf("%s/%s", BackupPoolName, backupImageName)
	backup := &models.Backup{
		VMID:           payload.VMID,
		Source:         payload.Source,
		StorageBackend: storageBackendCeph,
		RBDSnapshot:    &snapshotResp.RBDSnapshotName,
		StoragePath:    &storagePath,
		Status:         models.BackupStatusCompleted,
	}

	if payload.AdminScheduleID != "" {
		backup.AdminScheduleID = &payload.AdminScheduleID
	}

	expiresAt := time.Now().AddDate(0, 0, DefaultBackupExpirationDays)
	backup.ExpiresAt = &expiresAt

	if err := deps.BackupRepo.CreateBackup(ctx, backup); err != nil {
		logger.Error("failed to create backup record", "error", err)
		return fmt.Errorf("creating backup record: %w", err)
	}

	result := BackupCreateResult{
		BackupID:          backup.ID,
		VMID:              payload.VMID,
		SnapshotName:      snapshotResp.RBDSnapshotName,
		StoragePath:       storagePath,
		SizeBytes:         snapshotResp.SizeBytes,
		Consistency:       backupConsistency,
		FrozenFilesystems: frozenCount,
		StorageBackend:    storageBackendCeph,
	}
	completeBackupTask(ctx, task, deps, backup, result, logger)

	return nil
}
