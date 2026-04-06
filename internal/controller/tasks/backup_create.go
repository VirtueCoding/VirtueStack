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
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// Constants for backup operations.
const (
	GuestAgentFreezeTimeout     = 10 * time.Second
	DefaultBackupExpirationDays = 30
	BackupPoolName              = "vs-backups"
	storageBackendCeph          = "ceph"
	storageBackendQCOW          = "qcow"
	storageBackendLVM           = "lvm"
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
	Task             *models.Task
	VM               *models.VM
	NodeID           string
	Payload          BackupCreatePayload
	FreezeSuccessful bool
	FrozenCount      int
	Logger           *slog.Logger
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
	logger := taskLogger(deps.Logger, task)

	var payload BackupCreatePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse backup.create payload", "error", err)
		return fmt.Errorf("parsing backup.create payload: %w", err)
	}

	logger = logger.With("backup_name", payload.BackupName)
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
			// Create a detached context for the thaw operation. The parent task context
			// may already be cancelled or near expiry by the time the defer runs, but we
			// must always attempt to thaw the filesystems to avoid leaving the guest in
			// a frozen state. We derive from ctx to maintain trace correlation while
			// using a fresh timeout that is independent of parent cancellation.
			thawCtx, thawCancel := context.WithTimeout(context.WithoutCancel(ctx), GuestAgentFreezeTimeout)
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

	switch storageBackend {
	case storageBackendQCOW:
		return handleQCOWBackupCreate(ctx, deps, backupCtx)
	case storageBackendLVM:
		return handleLVMBackupCreate(ctx, deps, backupCtx)
	default:
		return handleCephBackupCreate(ctx, deps, backupCtx)
	}
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
		Method:         models.BackupMethodFull,
		VMID:           payload.VMID,
		Source:         payload.Source,
		StorageBackend: storageBackendQCOW,
		FilePath:       &backupFilePath,
		SnapshotName:   &snapshotName,
		SizeBytes:      &sizeBytes,
		Status:         models.BackupStatusCompleted,
	}
	if payload.BackupName != "" {
		backup.Name = util.StringPtr(payload.BackupName)
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
	return fmt.Errorf(
		"ceph full backups are not supported until clone transport is implemented: %w",
		sharederrors.ErrNotSupported,
	)
}

// handleLVMBackupCreate handles LVM backup creation with thin snapshot support.
// The backup flow:
// 1. Create thin snapshot via CreateDiskSnapshot
// 2. Thaw filesystems immediately (minimize freeze window)
// 3. Use dd with conv=sparse to create backup file
// 4. Cleanup snapshot after backup completes
func handleLVMBackupCreate(ctx context.Context, deps *HandlerDeps, backupCtx *BackupHandlerContext) error {
	task := backupCtx.Task
	nodeID := backupCtx.NodeID
	payload := backupCtx.Payload
	freezeSuccessful := backupCtx.FreezeSuccessful
	frozenCount := backupCtx.FrozenCount
	logger := backupCtx.Logger

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Creating LVM thin snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	snapshotName := fmt.Sprintf("backup-%s-%d", payload.VMID, time.Now().Unix())

	// Create LVM thin snapshot (diskPath is empty - node agent derives from vmID)
	if err := deps.NodeClient.CreateDiskSnapshot(ctx, nodeID, payload.VMID, "", snapshotName, "lvm"); err != nil {
		logger.Error("failed to create LVM snapshot", "error", err)
		return fmt.Errorf("creating LVM snapshot for VM %s: %w", payload.VMID, err)
	}

	logger.Info("LVM thin snapshot created successfully", "snapshot_name", snapshotName)

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Creating backup file..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	backupConfig := DefaultBackupConfig()
	backupDir := fmt.Sprintf("%s/%s", backupConfig.BackupPath, payload.VMID)
	backupFileName := fmt.Sprintf("%s-%s.raw", shortID(task.ID), time.Now().Format("20060102-150405"))
	backupFilePath := fmt.Sprintf("%s/%s", backupDir, backupFileName)

	sizeBytes, err := deps.NodeClient.CreateLVMBackup(ctx, nodeID, payload.VMID, snapshotName, backupFilePath)
	if err != nil {
		logger.Error("failed to create LVM backup file", "error", err)
		if delErr := deps.NodeClient.DeleteDiskSnapshot(ctx, nodeID, payload.VMID, "", snapshotName, "lvm"); delErr != nil {
			logger.Error("failed to cleanup LVM snapshot after backup failure", "operation", "DeleteDiskSnapshot", "err", delErr)
		}
		return fmt.Errorf("creating LVM backup file for VM %s: %w", payload.VMID, err)
	}

	logger.Info("LVM backup file created successfully",
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
		Method:         models.BackupMethodFull,
		VMID:           payload.VMID,
		Source:         payload.Source,
		StorageBackend: storageBackendLVM,
		FilePath:       &backupFilePath,
		SnapshotName:   &snapshotName,
		SizeBytes:      &sizeBytes,
		Status:         models.BackupStatusCompleted,
	}
	if payload.BackupName != "" {
		backup.Name = util.StringPtr(payload.BackupName)
	}

	if payload.AdminScheduleID != "" {
		backup.AdminScheduleID = &payload.AdminScheduleID
	}

	expiresAt := time.Now().AddDate(0, 0, DefaultBackupExpirationDays)
	backup.ExpiresAt = &expiresAt

	if err := deps.BackupRepo.CreateBackup(ctx, backup); err != nil {
		logger.Error("failed to create backup record", "error", err)
		if delErr := deps.NodeClient.DeleteLVMBackupFile(ctx, nodeID, backupFilePath); delErr != nil {
			logger.Error("failed to cleanup backup file after record failure", "operation", "DeleteLVMBackupFile", "err", delErr)
		}
		if delErr := deps.NodeClient.DeleteDiskSnapshot(ctx, nodeID, payload.VMID, "", snapshotName, "lvm"); delErr != nil {
			logger.Error("failed to cleanup LVM snapshot after record failure", "operation", "DeleteDiskSnapshot", "err", delErr)
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
		StorageBackend:    storageBackendLVM,
	}
	completeBackupTask(ctx, task, deps, backup, result, logger)

	return nil
}
