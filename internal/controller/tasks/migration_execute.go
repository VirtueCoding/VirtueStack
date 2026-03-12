// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// handleVMMigrate handles the live migration flow.
// Steps:
//  1. Parse payload
//  2. Validate VM and nodes
//  3. Update VM status to "migrating"
//  4. Initiate live migration via gRPC on source node
//  5. Re-apply tc throttling and nwfilter on target node
//  6. Update VM record with new node assignment
//  7. Update VM status back to "running"
//
// Idempotency: If the VM is already on the target node, the task is considered
// successfully completed without performing migration operations.
func handleVMMigrate(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeVMMigrate)

	// Parse payload
	var payload VMMigratePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.migrate payload", "error", err)
		return fmt.Errorf("parsing vm.migrate payload: %w", err)
	}

	logger = logger.With(
		"vm_id", payload.VMID,
		"source_node_id", payload.SourceNodeID,
		"target_node_id", payload.TargetNodeID,
	)
	logger.Info("vm.migrate task started")

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Initiating live migration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	// Idempotency check: If VM is already on target node, task is complete
	if vm.NodeID != nil && *vm.NodeID == payload.TargetNodeID {
		logger.Info("VM already on target node, migration complete (idempotent)")
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM already on target node"); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}
		result := map[string]any{
			"vm_id":          payload.VMID,
			"source_node_id": payload.SourceNodeID,
			"target_node_id": payload.TargetNodeID,
			"status":         "already_migrated",
		}
		resultJSON, _ := json.Marshal(result)
		if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
			logger.Warn("failed to set task completed", "error", err)
		}
		return nil
	}

	// Validate current node assignment matches source node
	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	if *vm.NodeID != payload.SourceNodeID {
		return fmt.Errorf("VM %s is on node %s, not source node %s",
			payload.VMID, *vm.NodeID, payload.SourceNodeID)
	}

	// Get source and target node info
	sourceNode, err := deps.NodeRepo.GetByID(ctx, payload.SourceNodeID)
	if err != nil {
		logger.Error("failed to get source node", "error", err)
		return fmt.Errorf("getting source node %s: %w", payload.SourceNodeID, err)
	}

	targetNode, err := deps.NodeRepo.GetByID(ctx, payload.TargetNodeID)
	if err != nil {
		logger.Error("failed to get target node", "error", err)
		return fmt.Errorf("getting target node %s: %w", payload.TargetNodeID, err)
	}

	// Validate target node is online
	if targetNode.Status != models.NodeStatusOnline {
		return fmt.Errorf("target node %s is not online (status: %s)",
			payload.TargetNodeID, targetNode.Status)
	}

	// Update task progress: Preparing migration
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 10, "Preparing live migration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Update VM status to migrating
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusMigrating); err != nil {
		logger.Error("failed to update VM status to migrating", "error", err)
		return fmt.Errorf("updating VM %s status to migrating: %w", payload.VMID, err)
	}

	// Update task progress: Executing migration
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 20, "Executing live migration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Prepare migration options
	migrateOpts := &MigrateVMOptions{
		TargetNodeAddress:  targetNode.GRPCAddress,
		BandwidthLimitMbps: 1000, // Default 1Gbps migration bandwidth
		Compression:        true, // Enable compression for faster migration
		AutoConverge:       true, // Force convergence if migration stalls
	}

	// Initiate live migration via gRPC on source node
	if err := deps.NodeClient.MigrateVM(ctx, payload.SourceNodeID, payload.TargetNodeID, payload.VMID, migrateOpts); err != nil {
		logger.Error("failed to migrate VM", "error", err)
		// Attempt to restore VM status
		_ = deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusError)
		return fmt.Errorf("migrating VM %s from %s to %s: %w",
			payload.VMID, payload.SourceNodeID, payload.TargetNodeID, err)
	}

	// Update task progress: Post-migration setup
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Applying post-migration configuration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Re-apply tc throttling and nwfilter on target node
	if err := deps.NodeClient.PostMigrateSetup(ctx, payload.TargetNodeID, payload.VMID, vm.PortSpeedMbps); err != nil {
		logger.Warn("failed to apply post-migration setup on target node", "error", err)
		// Continue with DB update - the VM has migrated successfully
	}

	// Update task progress: Updating database
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 85, "Updating VM assignment..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Update VM record with new node assignment
	if err := deps.VMRepo.UpdateNodeAssignment(ctx, payload.VMID, payload.TargetNodeID); err != nil {
		logger.Error("failed to update VM node assignment", "error", err)
		return fmt.Errorf("updating VM %s node assignment to %s: %w",
			payload.VMID, payload.TargetNodeID, err)
	}

	// Update VM status back to running
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusRunning); err != nil {
		logger.Warn("failed to update VM status to running", "error", err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Live migration completed successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":               payload.VMID,
		"source_node_id":      payload.SourceNodeID,
		"source_node_address": sourceNode.GRPCAddress,
		"target_node_id":      payload.TargetNodeID,
		"target_node_address": targetNode.GRPCAddress,
		"status":              "migrated",
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.migrate task completed successfully",
		"vm_id", payload.VMID,
		"source_node", payload.SourceNodeID,
		"target_node", payload.TargetNodeID)

	return nil
}