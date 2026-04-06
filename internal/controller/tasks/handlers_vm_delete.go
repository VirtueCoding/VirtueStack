// Package tasks provides the VM deletion task handler.
// This file contains the handleVMDelete function which handles the full
// VM deletion flow including stopping the VM, deleting disk, releasing IPs,
// and soft-deleting the VM record.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// handleVMDelete handles the VM deletion flow.
// Steps:
//  1. Parse payload
//  2. Get VM record
//  3. Stop VM (if running)
//  4. Delete VM definition
//  5. Delete RBD volume
//  6. Release IP addresses
//  7. Soft delete VM record
func handleVMDelete(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := taskLogger(deps.Logger, task)

	// Parse payload
	var payload VMDeletePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.delete payload", "error", err)
		return fmt.Errorf("parsing vm.delete payload: %w", err)
	}

	logger.Info("vm.delete task started")

	// Update task progress
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting VM deletion..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		if !errors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
		}
		// If VM doesn't exist, consider deletion successful (idempotent).
		// json.Marshal error is intentionally suppressed: the map contains only
		// primitive types (string, int, bool) whose marshaling cannot fail.
		idempotentResult, _ := json.Marshal(map[string]any{
			"vm_id":  payload.VMID,
			"status": "deleted",
		})
		if err := deps.TaskRepo.SetCompleted(ctx, task.ID, idempotentResult); err != nil {
			logger.Warn("failed to set task completed", "error", err)
		}
		return nil
	}

	// Update task progress: Stopping VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Stopping virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Delete VM definition and disk if node is assigned
	if vm.NodeID != nil {
		nodeID := *vm.NodeID

		// Stop VM if running
		if vm.Status == models.VMStatusRunning {
			if err := stopVMGracefully(ctx, deps.NodeClient, nodeID, payload.VMID, 60, logger); err != nil {
				logger.Warn("failed to stop VM during deletion, continuing", "error", err)
			}
		}

		// Update task progress: Deleting VM definition
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Deleting VM definition..."); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}

		// Delete VM definition from libvirt
		if err := deps.NodeClient.DeleteVM(ctx, nodeID, payload.VMID); err != nil {
			logger.Warn("failed to delete VM definition", "error", err)
			// Continue with disk deletion
		}

		// Update task progress: Deleting disk
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Deleting disk image..."); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}

		// Delete RBD disk
		if err := deps.NodeClient.DeleteDisk(ctx, nodeID, payload.VMID); err != nil {
			logger.Warn("failed to delete disk", "error", err)
			// Continue with IP release
		}
	}

	// Update task progress: Releasing IPs
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Releasing IP addresses..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Release IP addresses
	if err := deps.IPAMService.ReleaseIPsByVM(ctx, payload.VMID); err != nil {
		logger.Warn("failed to release IPs", "error", err)
		// Continue with VM record deletion
	}

	// Update task progress: Soft deleting record
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 90, "Removing VM record..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Soft delete VM record
	if err := deps.VMRepo.SoftDelete(ctx, payload.VMID); err != nil {
		logger.Error("failed to soft delete VM record", "error", err)
		return fmt.Errorf("soft deleting VM %s: %w", payload.VMID, err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM deleted successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":  payload.VMID,
		"status": "deleted",
	}
	// json.Marshal error is intentionally suppressed: the map contains only
	// primitive types (string, int, bool) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.delete task completed successfully", "vm_id", payload.VMID)

	return nil
}
