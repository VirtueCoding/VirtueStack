// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// Constants for backup operations.
const (
	// GuestAgentFreezeTimeout is the maximum time to wait for guest agent freeze operations.
	// We use a strict timeout because guest OS environments are untrusted and can hang.
	GuestAgentFreezeTimeout = 10 * time.Second

	// DefaultBackupExpirationDays is the default number of days before a backup expires.
	DefaultBackupExpirationDays = 30

	// BackupPoolName is the Ceph pool where backup images are stored.
	BackupPoolName = "vs-backups"
)

// handleBackupCreate handles the backup creation flow with QEMU guest agent integration.
//
// This handler implements application-consistent backup using QEMU guest agent:
// 1. Parse payload and validate
// 2. Check idempotency (skip if already completed)
// 3. Request filesystem freeze via QEMU guest agent (with strict timeout)
// 4. If freeze fails/times out, proceed with crash-consistent backup
// 5. Create RBD snapshot via Node Agent
// 6. Thaw filesystems via QEMU guest agent
// 7. Create backup record in database
// 8. Update task progress to completed
//
// Idempotency: If the task is already in a terminal state (completed/failed),
// the handler returns nil without re-processing.
func handleBackupCreate(ctx context.Context, task *Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", TaskTypeBackupCreate)

	// Parse payload
	var payload BackupCreatePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse backup.create payload", "error", err)
		return fmt.Errorf("parsing backup.create payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID, "backup_name", payload.BackupName)
	logger.Info("backup.create task started", "backup_type", payload.BackupType)

	// ========================================
	// Idempotency check: Skip if already processed
	// ========================================
	if task.IsTerminal() {
		logger.Info("task already in terminal state, skipping",
			"status", task.Status)
		return nil
	}

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting backup creation..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// ========================================
	// Get VM record and validate
	// ========================================
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	nodeID := *vm.NodeID

	// ========================================
	// Attempt filesystem freeze via QEMU guest agent
	// This provides application-consistent backups.
	// If it fails or times out, we proceed with crash-consistent backup.
	// ========================================
	var frozenCount int
	var freezeSuccessful bool

	// Create a child context with strict timeout for guest agent operations
	freezeCtx, freezeCancel := context.WithTimeout(ctx, GuestAgentFreezeTimeout)
	defer freezeCancel()

	// Update task progress: Freezing filesystems
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Freezing filesystems via guest agent..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	frozenCount, err = deps.NodeClient.GuestFreezeFilesystems(freezeCtx, nodeID, payload.VMID)
	if err != nil {
		// Log the error but continue with crash-consistent backup
		logger.Warn("guest agent freeze failed or timed out, proceeding with crash-consistent backup",
			"error", err,
			"timeout", GuestAgentFreezeTimeout)
		freezeSuccessful = false
	} else {
		logger.Info("filesystems frozen successfully",
			"frozen_count", frozenCount)
		freezeSuccessful = true
	}

	// ========================================
	// Create RBD snapshot
	// ========================================
	// Use defer to ensure filesystems are thawed even if snapshot creation fails
	defer func() {
		if freezeSuccessful && frozenCount > 0 {
			// Create a fresh context for thaw (original context may be cancelled)
			thawCtx, thawCancel := context.WithTimeout(context.Background(), GuestAgentFreezeTimeout)
			defer thawCancel()

			thawedCount, thawErr := deps.NodeClient.GuestThawFilesystems(thawCtx, nodeID, payload.VMID)
			if thawErr != nil {
				logger.Error("failed to thaw filesystems after backup",
					"error", thawErr)
				// Note: We don't return this error as the backup itself may have succeeded
			} else {
				logger.Info("filesystems thawed successfully",
					"thawed_count", thawedCount)
			}
		}
	}()

	// Update task progress: Creating snapshot
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Creating disk snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Generate snapshot name with timestamp for uniqueness
	snapshotName := fmt.Sprintf("backup-%s-%d", payload.VMID, time.Now().Unix())

	// Create snapshot via node agent
	snapshotResp, err := deps.NodeClient.CreateSnapshot(ctx, nodeID, payload.VMID, snapshotName)
	if err != nil {
		logger.Error("failed to create snapshot", "error", err)
		return fmt.Errorf("creating snapshot for VM %s: %w", payload.VMID, err)
	}

	logger.Info("RBD snapshot created successfully",
		"snapshot_name", snapshotResp.RBDSnapshotName,
		"size_bytes", snapshotResp.SizeBytes)

	// ========================================
	// Protect the snapshot (required for cloning)
	// ========================================
	// Update task progress: Protecting snapshot
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 40, "Protecting snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.ProtectSnapshot(ctx, nodeID, payload.VMID, snapshotResp.RBDSnapshotName); err != nil {
		logger.Error("failed to protect snapshot", "error", err)
		return fmt.Errorf("protecting snapshot for VM %s: %w", payload.VMID, err)
	}

	logger.Info("snapshot protected successfully", "snapshot_name", snapshotResp.RBDSnapshotName)

	// ========================================
	// Clone snapshot to backup pool
	// ========================================
	// Update task progress: Cloning to backup pool
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Cloning to backup storage..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Generate unique backup image name
	backupImageName := fmt.Sprintf("vs-%s-%d-backup", payload.VMID, time.Now().Unix())

	clonedImageName, err := deps.NodeClient.CloneSnapshot(ctx, nodeID, payload.VMID, snapshotResp.RBDSnapshotName, BackupPoolName)
	if err != nil {
		logger.Error("failed to clone snapshot to backup pool", "error", err)
		// Cleanup: unprotect the snapshot since clone failed
		if unprotectErr := deps.NodeClient.UnprotectSnapshot(ctx, nodeID, payload.VMID, snapshotResp.RBDSnapshotName); unprotectErr != nil {
			logger.Warn("failed to unprotect snapshot during cleanup", "error", unprotectErr)
		}
		return fmt.Errorf("cloning snapshot to backup pool for VM %s: %w", payload.VMID, err)
	}

	// Use the returned image name, or fallback to generated name
	if clonedImageName != "" {
		backupImageName = clonedImageName
	}

	logger.Info("snapshot cloned to backup pool successfully",
		"backup_image", backupImageName,
		"backup_pool", BackupPoolName)

	// ========================================
	// Create backup record in database
	// ========================================
	// Update task progress: Creating backup record
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Creating backup record..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Determine backup consistency type
	backupConsistency := "crash-consistent"
	if freezeSuccessful {
		backupConsistency = "application-consistent"
	}

	// Create backup record with storage path
	storagePath := fmt.Sprintf("%s/%s", BackupPoolName, backupImageName)
	backup := &models.Backup{
		VMID:        payload.VMID,
		Type:        payload.BackupType,
		RBDSnapshot: &snapshotResp.RBDSnapshotName,
		StoragePath: &storagePath,
		Status:      models.BackupStatusCompleted,
	}

	// Set expiration (default 30 days)
	expiresAt := time.Now().AddDate(0, 0, DefaultBackupExpirationDays)
	backup.ExpiresAt = &expiresAt

	if err := deps.BackupRepo.CreateBackup(ctx, backup); err != nil {
		logger.Error("failed to create backup record", "error", err)
		return fmt.Errorf("creating backup record: %w", err)
	}

	// ========================================
	// Complete the task
	// ========================================
	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Backup created successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"backup_id":          backup.ID,
		"vm_id":              payload.VMID,
		"snapshot_name":      snapshotResp.RBDSnapshotName,
		"storage_path":       storagePath,
		"size_bytes":         snapshotResp.SizeBytes,
		"consistency":        backupConsistency,
		"frozen_filesystems": frozenCount,
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("backup.create task completed successfully",
		"backup_id", backup.ID,
		"snapshot", snapshotResp.RBDSnapshotName,
		"storage_path", storagePath,
		"consistency", backupConsistency)

	return nil
}