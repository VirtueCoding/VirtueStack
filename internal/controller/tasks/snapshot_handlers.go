// Package tasks provides async task handlers for snapshot operations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// Constants for snapshot operations.
const (
	// DefaultSnapshotQuota is the default maximum number of snapshots per VM.
	DefaultSnapshotQuota = 10
)

// SnapshotCreatePayload represents the payload for snapshot.create tasks.
type SnapshotCreatePayload struct {
	VMID       string `json:"vm_id"`
	SnapshotID string `json:"snapshot_id"`
	Name       string `json:"name"`
	CustomerID string `json:"customer_id"`
}

// SnapshotRevertPayload represents the payload for snapshot.revert tasks.
type SnapshotRevertPayload struct {
	VMID       string `json:"vm_id"`
	SnapshotID string `json:"snapshot_id"`
	CustomerID string `json:"customer_id"`
}

// SnapshotDeletePayload represents the payload for snapshot.delete tasks.
type SnapshotDeletePayload struct {
	VMID       string `json:"vm_id"`
	SnapshotID string `json:"snapshot_id"`
	CustomerID string `json:"customer_id"`
}

// handleSnapshotCreate handles the snapshot creation flow.
// Steps:
//  1. Parse payload and validate
//  2. Check idempotency (skip if already completed)
//  3. Get VM record and validate node assignment
//  4. Create RBD snapshot via Node Agent
//  5. Update snapshot record in database with RBD snapshot name
//  6. Update task progress to completed
func handleSnapshotCreate(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeSnapshotCreate)

	// Parse payload
	var payload SnapshotCreatePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse snapshot.create payload", "error", err)
		return fmt.Errorf("parsing snapshot.create payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID, "snapshot_id", payload.SnapshotID)
	logger.Info("snapshot.create task started", "name", payload.Name)

	// Idempotency check: Skip if already processed
	if task.IsTerminal() {
		logger.Info("task already in terminal state, skipping",
			"status", task.Status)
		return nil
	}

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting snapshot creation..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get VM record and validate
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	nodeID := *vm.NodeID

	// Update task progress: Creating snapshot
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Creating disk snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Generate RBD snapshot name
	rbdSnapshotName := fmt.Sprintf("snap-%s-%d", shortID(payload.SnapshotID), time.Now().Unix())

	// Create snapshot via node agent
	snapshotResp, err := deps.NodeClient.CreateSnapshot(ctx, nodeID, payload.VMID, rbdSnapshotName)
	if err != nil {
		logger.Error("failed to create snapshot", "error", err)
		return fmt.Errorf("creating snapshot for VM %s: %w", payload.VMID, err)
	}

	logger.Info("RBD snapshot created successfully",
		"snapshot_name", snapshotResp.RBDSnapshotName,
		"size_bytes", snapshotResp.SizeBytes)

	// Update task progress: Updating database
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Updating snapshot record..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get existing snapshot record and update it
	_, err = deps.BackupRepo.GetSnapshotByID(ctx, payload.SnapshotID)
	if err != nil {
		logger.Error("failed to get snapshot record", "error", err)
		// Attempt to clean up the created snapshot
		_ = deps.NodeClient.DeleteSnapshot(ctx, nodeID, payload.VMID, rbdSnapshotName)
		return fmt.Errorf("getting snapshot record %s: %w", payload.SnapshotID, err)
	}

	updatedSnapshot := &models.Snapshot{
		ID:          payload.SnapshotID,
		VMID:        payload.VMID,
		Name:        payload.Name,
		RBDSnapshot: snapshotResp.RBDSnapshotName,
		SizeBytes:   &snapshotResp.SizeBytes,
	}

	if err := deps.BackupRepo.UpdateSnapshot(ctx, updatedSnapshot); err != nil {
		logger.Error("failed to update snapshot record", "error", err)
		// Attempt to clean up the created snapshot
		_ = deps.NodeClient.DeleteSnapshot(ctx, nodeID, payload.VMID, rbdSnapshotName)
		return fmt.Errorf("updating snapshot record: %w", err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Snapshot created successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"snapshot_id":  payload.SnapshotID,
		"vm_id":        payload.VMID,
		"rbd_snapshot": snapshotResp.RBDSnapshotName,
		"size_bytes":   snapshotResp.SizeBytes,
	}
	// json.Marshal error is intentionally suppressed: the map contains only
	// primitive types (string, int, bool) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("snapshot.create task completed successfully",
		"snapshot_id", payload.SnapshotID,
		"rbd_snapshot", snapshotResp.RBDSnapshotName)

	return nil
}

// handleSnapshotRevert handles the snapshot revert/restore flow.
// Steps:
//  1. Parse payload and validate
//  2. Check idempotency (skip if already completed)
//  3. Get snapshot and VM records
//  4. Stop VM if running
//  5. Restore from RBD snapshot via Node Agent
//  6. Start VM
//  7. Update task progress to completed
func handleSnapshotRevert(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeSnapshotRevert)

	// Parse payload
	var payload SnapshotRevertPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse snapshot.revert payload", "error", err)
		return fmt.Errorf("parsing snapshot.revert payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID, "snapshot_id", payload.SnapshotID)
	logger.Info("snapshot.revert task started")

	// Idempotency check: Skip if already processed
	if task.IsTerminal() {
		logger.Info("task already in terminal state, skipping",
			"status", task.Status)
		return nil
	}

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting snapshot revert..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get snapshot record
	snapshot, err := deps.BackupRepo.GetSnapshotByID(ctx, payload.SnapshotID)
	if err != nil {
		logger.Error("failed to get snapshot record", "error", err)
		return fmt.Errorf("getting snapshot %s: %w", payload.SnapshotID, err)
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	nodeID := *vm.NodeID

	// Update task progress: Stopping VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Stopping virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Stop VM if running
	wasRunning := vm.Status == models.VMStatusRunning
	if wasRunning {
		if err := stopVMGracefully(ctx, deps.NodeClient, nodeID, payload.VMID, 120, logger); err != nil {
			logger.Error("failed to stop VM for snapshot revert", "error", err)
			return fmt.Errorf("stopping VM %s: %w", payload.VMID, err)
		}
		// Update VM status
		if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusStopped); err != nil {
			logger.Warn("failed to update VM status", "error", err)
		}
	}

	// Update task progress: Restoring from snapshot
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 40, "Restoring from snapshot..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Restore from snapshot via node agent
	if err := deps.NodeClient.RestoreSnapshot(ctx, nodeID, payload.VMID, snapshot.RBDSnapshot); err != nil {
		logger.Error("failed to restore snapshot", "error", err)
		return fmt.Errorf("restoring snapshot %s for VM %s: %w", payload.SnapshotID, payload.VMID, err)
	}

	// Update task progress: Starting VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Starting virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Start VM if it was running before
	if wasRunning {
		if err := deps.NodeClient.StartVM(ctx, nodeID, payload.VMID); err != nil {
			logger.Error("failed to start VM after revert", "error", err)
			return fmt.Errorf("starting VM %s after revert: %w", payload.VMID, err)
		}
		// Update VM status
		if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusRunning); err != nil {
			logger.Warn("failed to update VM status", "error", err)
		}
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Snapshot reverted successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"snapshot_id": payload.SnapshotID,
		"vm_id":       payload.VMID,
		"status":      "reverted",
		"was_running": wasRunning,
	}
	// json.Marshal error is intentionally suppressed: the map contains only
	// primitive types (string, int, bool) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("snapshot.revert task completed successfully",
		"snapshot_id", payload.SnapshotID,
		"vm_id", payload.VMID)

	return nil
}

// handleSnapshotDelete handles the snapshot deletion flow.
// Steps:
//  1. Parse payload and validate
//  2. Check idempotency (skip if already completed)
//  3. Get snapshot and VM records
//  4. Delete RBD snapshot via Node Agent
//  5. Delete snapshot record from database
//  6. Update task progress to completed
func handleSnapshotDelete(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeSnapshotDelete)

	// Parse payload
	var payload SnapshotDeletePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse snapshot.delete payload", "error", err)
		return fmt.Errorf("parsing snapshot.delete payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID, "snapshot_id", payload.SnapshotID)
	logger.Info("snapshot.delete task started")

	// Idempotency check: Skip if already processed
	if task.IsTerminal() {
		logger.Info("task already in terminal state, skipping",
			"status", task.Status)
		return nil
	}

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting snapshot deletion..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get snapshot record
	snapshot, err := deps.BackupRepo.GetSnapshotByID(ctx, payload.SnapshotID)
	if err != nil {
		// If snapshot doesn't exist, consider deletion successful (idempotent)
		logger.Info("snapshot not found, considering deletion successful", "error", err)
		if err := deps.TaskRepo.SetCompleted(ctx, task.ID, []byte(`{"status":"deleted"}`)); err != nil {
			logger.Warn("failed to set task completed", "error", err)
		}
		return nil
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	// Update task progress: Deleting from storage
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Deleting snapshot from storage..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Delete from storage if node is assigned
	if vm.NodeID != nil {
		nodeID := *vm.NodeID
		if err := deps.NodeClient.DeleteSnapshot(ctx, nodeID, payload.VMID, snapshot.RBDSnapshot); err != nil {
			logger.Warn("failed to delete snapshot from storage",
				"error", err,
				"rbd_snapshot", snapshot.RBDSnapshot)
			// Continue with database deletion
		} else {
			logger.Info("snapshot deleted from storage",
				"rbd_snapshot", snapshot.RBDSnapshot)
		}
	}

	// Update task progress: Deleting from database
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Removing snapshot record..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Delete from database
	if err := deps.BackupRepo.DeleteSnapshot(ctx, payload.SnapshotID); err != nil {
		logger.Error("failed to delete snapshot from database", "error", err)
		return fmt.Errorf("deleting snapshot %s from database: %w", payload.SnapshotID, err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Snapshot deleted successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"snapshot_id": payload.SnapshotID,
		"vm_id":       payload.VMID,
		"status":      "deleted",
	}
	// json.Marshal error is intentionally suppressed: the map contains only
	// primitive types (string, int, bool) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("snapshot.delete task completed successfully",
		"snapshot_id", payload.SnapshotID,
		"vm_id", payload.VMID)

	return nil
}
