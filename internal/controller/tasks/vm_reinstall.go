// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// handleVMReinstall handles the VM reinstallation flow.
// Steps:
//  1. Parse payload and gather info
//  2. Check idempotency (already reinstalled?)
//  3. Stop VM forcefully
//  4. Replace disk with fresh clone from template
//  5. Regenerate cloud-init ISO
//  6. Start VM and update database
func handleVMReinstall(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeVMReinstall)

	// Step 1: Parse payload
	var payload VMReinstallPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.reinstall payload", "error", err)
		return fmt.Errorf("parsing vm.reinstall payload: %w", err)
	}
	logger = logger.With("vm_id", payload.VMID, "template_id", payload.TemplateID)
	logger.Info("vm.reinstall task started")

	// Step 2: Gather VM and template info
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting VM reinstallation..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}
	info, err := prepareVMReinstallInfo(ctx, deps, &payload, logger)
	if err != nil {
		return err
	}

	// Step 3: Check idempotency - if already running with correct template, done
	if done, _ := checkReinstallIdempotency(ctx, deps, task, &payload, info, logger); done {
		return nil
	}

	// Step 4: Update status and stop VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 10, "Stopping VM..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}
	if info.vm.Status != models.VMStatusReinstalling {
		if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusReinstalling); err != nil {
			logger.Warn("failed to update VM status", "error", err)
		}
	}
	if err := stopVMForReinstall(ctx, deps, info, logger); err != nil {
		logger.Warn("stop VM returned error", "error", err)
	}

	// Step 5: Replace disk (delete old, clone new from template)
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 25, "Replacing disk..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}
	if err := replaceVMDisk(ctx, deps, &payload, info, logger); err != nil {
		return err
	}

	// Step 6: Regenerate cloud-init
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 60, "Generating cloud-init..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}
	if err := regenerateCloudInitForReinstall(ctx, deps, &payload, info, logger); err != nil {
		return err
	}

	// Step 7: Start VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Starting VM..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}
	logger.Info("starting VM after reinstallation")
	if err := deps.NodeClient.StartVM(ctx, info.nodeID, payload.VMID); err != nil {
		logger.Error("failed to start VM", "error", err)
		return fmt.Errorf("starting VM %s: %w", payload.VMID, err)
	}

	// Step 8: Set final result
	return setReinstallResult(ctx, deps, task, &payload, info, logger)
}