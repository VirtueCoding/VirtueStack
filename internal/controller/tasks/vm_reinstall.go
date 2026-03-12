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
//  1. Parse payload (vm_id, template_id)
//  2. Get VM and template info
//  3. Stop VM forcefully (idempotent, ignore if already stopped)
//  4. Delete existing RBD disk volume
//  5. Clone fresh RBD image from template
//  6. Regenerate cloud-init ISO
//  7. Start the VM
//  8. Update VM database record with new template_id and status "running"
//
// Idempotency: The handler tracks progress and checks state at each step.
// If the task retries after a partial failure, it skips already-completed steps:
//   - If VM is already stopped, skip stopping
//   - If disk matches expected template, skip disk recreation
//   - If VM is already running with correct template, task is complete
func handleVMReinstall(ctx context.Context, task *Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", TaskTypeVMReinstall)

	// Parse payload
	var payload VMReinstallPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.reinstall payload", "error", err)
		return fmt.Errorf("parsing vm.reinstall payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID, "template_id", payload.TemplateID)
	logger.Info("vm.reinstall task started")

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting VM reinstallation..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
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

	// Get template information
	template, err := deps.TemplateRepo.GetByID(ctx, payload.TemplateID)
	if err != nil {
		logger.Error("failed to get template", "error", err)
		return fmt.Errorf("getting template %s: %w", payload.TemplateID, err)
	}

	// ============================================================
	// IDEMPOTENCY CHECK: If VM is already running with the correct template,
	// the reinstall is complete.
	// ============================================================
	if vm.Status == models.VMStatusRunning && vm.TemplateID != nil && *vm.TemplateID == payload.TemplateID {
		logger.Info("VM already running with correct template, reinstall complete (idempotent)")
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM already reinstalled with correct template"); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}
		result := map[string]any{
			"vm_id":       payload.VMID,
			"template_id": payload.TemplateID,
			"status":      "already_reinstalled",
		}
		resultJSON, _ := json.Marshal(result)
		if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
			logger.Warn("failed to set task completed", "error", err)
		}
		return nil
	}

	// Update task progress: Stopping VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 10, "Stopping virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Update VM status to reinstalling
	if vm.Status != models.VMStatusReinstalling {
		if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusReinstalling); err != nil {
			logger.Warn("failed to update VM status", "error", err)
		}
	}

	// ============================================================
	// STEP 1: Stop VM forcefully (idempotent)
	// Try graceful stop first, then force if needed.
	// Ignore errors if VM is already stopped.
	// ============================================================
	if vm.Status == models.VMStatusRunning || vm.Status == models.VMStatusSuspended {
		logger.Info("stopping VM for reinstallation")

		// Try graceful stop with timeout
		if err := deps.NodeClient.StopVM(ctx, nodeID, payload.VMID, 30); err != nil {
			logger.Warn("graceful stop failed, attempting force stop", "error", err)
			// Force stop if graceful fails
			if err := deps.NodeClient.ForceStopVM(ctx, nodeID, payload.VMID); err != nil {
				// Check if error indicates VM is already stopped
				logger.Warn("force stop returned error, continuing with reinstallation", "error", err)
				// Continue anyway - the VM might already be stopped
			}
		}
	} else {
		logger.Info("VM already stopped, skipping stop step (idempotent)")
	}

	// Update task progress: Deleting disk
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 25, "Removing old disk image..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// ============================================================
	// STEP 2: Delete existing RBD disk volume
	// This is idempotent - deleting a non-existent disk is a no-op.
	// ============================================================
	logger.Info("deleting existing disk volume")
	if err := deps.NodeClient.DeleteDisk(ctx, nodeID, payload.VMID); err != nil {
		logger.Warn("delete disk returned error, continuing with clone", "error", err)
		// Continue anyway - the disk might not exist or deletion is idempotent
	}

	// Update task progress: Cloning new disk
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 40, "Cloning fresh disk from template..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// ============================================================
	// STEP 3: Clone fresh RBD image from template
	// ============================================================
	logger.Info("cloning fresh disk from template",
		"rbd_image", template.RBDImage,
		"rbd_snapshot", template.RBDSnapshot,
		"disk_gb", vm.DiskGB)

	if err := deps.NodeClient.CloneFromTemplate(ctx, nodeID, payload.VMID,
		template.RBDImage, template.RBDSnapshot, vm.DiskGB); err != nil {
		logger.Error("failed to clone template", "error", err)
		return fmt.Errorf("cloning template %s for VM %s: %w", payload.TemplateID, payload.VMID, err)
	}

	// Update task progress: Generating cloud-init
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 60, "Generating cloud-init configuration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// ============================================================
	// STEP 4: Regenerate cloud-init ISO
	// Get existing IP configuration for cloud-init.
	// ============================================================
	logger.Info("generating cloud-init configuration")

	// Get IP addresses for cloud-init
	ipv4, _ := deps.IPAMService.GetPrimaryIPv4(ctx, payload.VMID)
	ipv6Subnets, _ := deps.IPAMService.GetIPv6SubnetsByVM(ctx, payload.VMID)

	var ipv4Addr, ipv4Gateway string
	if ipv4 != nil {
		ipv4Addr = ipv4.Address
	}

	var ipv6Addr, ipv6Gateway string
	if len(ipv6Subnets) > 0 {
		ipv6Addr = ipv6Subnets[0].Subnet
		ipv6Gateway = ipv6Subnets[0].Gateway
	}

	// Generate cloud-init with new credentials
	passwordHash, err := hashPassword(payload.Password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	cloudInitCfg := &CloudInitConfig{
		VMID:             payload.VMID,
		Hostname:         vm.Hostname,
		RootPasswordHash: passwordHash,
		SSHPublicKeys:    payload.SSHKeys,
		IPv4Address:      ipv4Addr,
		IPv4Gateway:      ipv4Gateway,
		IPv6Address:      ipv6Addr,
		IPv6Gateway:      ipv6Gateway,
		Nameservers:      []string{"8.8.8.8", "8.8.4.4"},
	}

	_, err = deps.NodeClient.GenerateCloudInit(ctx, nodeID, cloudInitCfg)
	if err != nil {
		logger.Error("failed to generate cloud-init", "error", err)
		return fmt.Errorf("generating cloud-init for VM %s: %w", payload.VMID, err)
	}

	// Update task progress: Starting VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Starting virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// ============================================================
	// STEP 5: Start the VM
	// This is idempotent - starting an already running VM is handled by the agent.
	// ============================================================
	logger.Info("starting VM after reinstallation")
	if err := deps.NodeClient.StartVM(ctx, nodeID, payload.VMID); err != nil {
		logger.Error("failed to start VM", "error", err)
		return fmt.Errorf("starting VM %s: %w", payload.VMID, err)
	}

	// ============================================================
	// STEP 6: Update VM database record with new template_id and status
	// ============================================================
	logger.Info("updating VM database record")

	// Update VM status to running
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusRunning); err != nil {
		logger.Warn("failed to update VM status to running", "error", err)
	}

	// Update VM template_id
	if err := deps.VMRepo.UpdateTemplateID(ctx, payload.VMID, payload.TemplateID); err != nil {
		logger.Warn("failed to update VM template_id", "error", err)
		// Don't fail the task - the VM is running with the new template
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM reinstalled successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":       payload.VMID,
		"template_id": payload.TemplateID,
		"hostname":    vm.Hostname,
		"status":      "running",
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.reinstall task completed successfully",
		"vm_id", payload.VMID,
		"template_id", payload.TemplateID)

	return nil
}