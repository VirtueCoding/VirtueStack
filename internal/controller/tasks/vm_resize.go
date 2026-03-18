package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// VMResizePayload represents the payload for vm.resize tasks.
type VMResizePayload struct {
	VMID        string `json:"vm_id"`
	NodeID      string `json:"node_id"`
	NewVCPU     int    `json:"new_vcpu"`
	NewMemoryMB int    `json:"new_memory_mb"`
	NewDiskGB   int    `json:"new_disk_gb"`
}

// handleVMResize handles the VM resize flow.
// Steps:
//  1. Parse payload and validate
//  2. Check idempotency (skip if already completed)
//  3. Validate VM exists and node is assigned
//  4. Call node agent to resize VM resources (CPU, memory, disk)
//  5. Update VM resources in database
//  6. Update task progress to completed
func handleVMResize(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeVMResize)

	var payload VMResizePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.resize payload", "error", err)
		return fmt.Errorf("parsing vm.resize payload: %w", err)
	}

	if payload.VMID == "" || payload.NodeID == "" {
		logger.Error("invalid vm.resize payload: missing vm_id or node_id")
		return fmt.Errorf("vm.resize payload requires vm_id and node_id")
	}

	logger = logger.With("vm_id", payload.VMID, "node_id", payload.NodeID)
	logger.Info("vm.resize task started",
		"new_vcpu", payload.NewVCPU,
		"new_memory_mb", payload.NewMemoryMB,
		"new_disk_gb", payload.NewDiskGB)

	if task.IsTerminal() {
		logger.Info("task already in terminal state, skipping",
			"status", task.Status)
		return nil
	}

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting VM resize..."); err != nil {
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

	wasRunning := vm.Status == models.VMStatusRunning

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Stopping virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if wasRunning {
		if err := stopVMGracefully(ctx, deps.NodeClient, nodeID, payload.VMID, 120, logger); err != nil {
			logger.Error("failed to stop VM for resize", "error", err)
			return fmt.Errorf("stopping VM %s for resize: %w", payload.VMID, err)
		}
		if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusStopped); err != nil {
			logger.Warn("failed to update VM status", "error", err)
		}
	}

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 40, "Resizing virtual machine resources..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.NodeClient.ResizeVM(ctx, nodeID, payload.VMID, payload.NewVCPU, payload.NewMemoryMB, payload.NewDiskGB); err != nil {
		logger.Error("failed to resize VM on node agent", "error", err)
		return fmt.Errorf("resizing VM %s on node agent: %w", payload.VMID, err)
	}

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Updating VM resource records..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if err := deps.VMRepo.UpdateResources(ctx, payload.VMID, payload.NewVCPU, payload.NewMemoryMB, payload.NewDiskGB); err != nil {
		logger.Error("failed to update VM resources in database", "error", err)
		return fmt.Errorf("updating VM %s resources: %w", payload.VMID, err)
	}

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 85, "Starting virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	if wasRunning {
		if err := deps.NodeClient.StartVM(ctx, nodeID, payload.VMID); err != nil {
			logger.Warn("failed to start VM after resize", "error", err)
		} else {
			if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusRunning); err != nil {
				logger.Warn("failed to update VM status", "error", err)
			}
		}
	}

	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM resized successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	result := map[string]any{
		"vm_id":       payload.VMID,
		"vcpu":        payload.NewVCPU,
		"memory_mb":   payload.NewMemoryMB,
		"disk_gb":     payload.NewDiskGB,
		"was_running": wasRunning,
	}
	// json.Marshal error is intentionally suppressed: the map contains only
	// primitive types (string, int, bool) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.resize task completed successfully",
		"vm_id", payload.VMID,
		"vcpu", payload.NewVCPU,
		"memory_mb", payload.NewMemoryMB,
		"disk_gb", payload.NewDiskGB)

	return nil
}
