// Package tasks provides helper functions for VM deletion task handling.
// These functions decompose the handleVMDelete function to comply with
// CODING_STANDARD.md QG-01 (functions <= 40 lines).
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// vmDeleteCheckResult contains the result of checking VM existence for deletion.
type vmDeleteCheckResult struct {
	vm       *models.VM
	notFound bool
}

// parseAndCheckVMDelete parses the delete payload and checks VM existence.
// If VM doesn't exist, returns notFound=true for idempotent handling.
func parseAndCheckVMDelete(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	logger *slog.Logger,
) (*vmDeleteCheckResult, error) {
	var payload VMDeletePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.delete payload", "error", err)
		return nil, fmt.Errorf("parsing vm.delete payload: %w", err)
	}

	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		// If VM doesn't exist, consider deletion successful (idempotent).
		if errors.Is(err, sharederrors.ErrNotFound) {
			return &vmDeleteCheckResult{notFound: true}, nil
		}
		return nil, fmt.Errorf("checking VM existence: %w", err)
	}
	return &vmDeleteCheckResult{vm: vm}, nil
}

// stopAndDeleteVMFromNode stops and deletes the VM from the libvirt node.
func stopAndDeleteVMFromNode(
	ctx context.Context,
	deps *HandlerDeps,
	vm *models.VM,
	vmID string,
	logger *slog.Logger,
) error {
	if vm.NodeID == nil {
		return nil
	}

	nodeID := *vm.NodeID

	// Stop VM if running
	if vm.Status == models.VMStatusRunning {
		if err := stopVMGracefully(ctx, deps.NodeClient, nodeID, vmID, 60, logger); err != nil {
			logger.Warn("failed to stop VM during deletion, continuing", "error", err)
		}
	}

	// Delete VM definition from libvirt
	if err := deps.NodeClient.DeleteVM(ctx, nodeID, vmID); err != nil {
		logger.Warn("failed to delete VM definition", "error", err)
	}

	// Delete RBD disk
	if err := deps.NodeClient.DeleteDisk(ctx, nodeID, vmID); err != nil {
		logger.Warn("failed to delete disk", "error", err)
	}

	return nil
}

// releaseVMResources releases IP addresses and soft deletes the VM record.
func releaseVMResources(
	ctx context.Context,
	deps *HandlerDeps,
	vmID string,
	logger *slog.Logger,
) error {
	// Release IP addresses
	if err := deps.IPAMService.ReleaseIPsByVM(ctx, vmID); err != nil {
		logger.Warn("failed to release IPs", "error", err)
	}

	// Soft delete VM record
	if err := deps.VMRepo.SoftDelete(ctx, vmID); err != nil {
		return fmt.Errorf("soft deleting VM %s: %w", vmID, err)
	}

	return nil
}

// setVMDeleteResult sets the task result for VM deletion.
func setVMDeleteResult(
	ctx context.Context,
	deps *HandlerDeps,
	task *models.Task,
	vmID string,
	logger *slog.Logger,
) {
	result := VMDeleteResult{
		VMID:   vmID,
		Status: "deleted",
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}
	logger.Info("vm.delete task completed successfully", "vm_id", vmID)
}

// setIdempotentDeleteResult sets the result for idempotent VM deletion.
func setIdempotentDeleteResult(
	ctx context.Context,
	deps *HandlerDeps,
	task *models.Task,
	vmID string,
) error {
	idempotentResult, _ := json.Marshal(map[string]any{
		"vm_id":  vmID,
		"status": "deleted",
	})
	return deps.TaskRepo.SetCompleted(ctx, task.ID, idempotentResult)
}