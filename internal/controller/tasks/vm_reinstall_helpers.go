// Package tasks provides helper functions for VM reinstallation task handling.
// These functions decompose the large handleVMReinstall function to comply with
// docs/coding-standard.md QG-01 (functions <= 40 lines).
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

// vmReinstallInfo contains the gathered information needed for VM reinstallation.
type vmReinstallInfo struct {
	vm       *models.VM
	template *models.Template
	nodeID   string
}

// prepareVMReinstallInfo fetches VM and template records for reinstallation.
func prepareVMReinstallInfo(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMReinstallPayload,
	logger *slog.Logger,
) (*vmReinstallInfo, error) {
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return nil, fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}
	if vm.NodeID == nil {
		return nil, fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}

	template, err := deps.TemplateRepo.GetByID(ctx, payload.TemplateID)
	if err != nil {
		logger.Error("failed to get template", "error", err)
		return nil, fmt.Errorf("getting template %s: %w", payload.TemplateID, err)
	}

	return &vmReinstallInfo{
		vm:       vm,
		template: template,
		nodeID:   *vm.NodeID,
	}, nil
}

// checkReinstallIdempotency checks if VM is already reinstalled with the correct template.
// Returns true if the reinstall is already complete (idempotent).
func checkReinstallIdempotency(
	ctx context.Context,
	deps *HandlerDeps,
	task *models.Task,
	payload *VMReinstallPayload,
	info *vmReinstallInfo,
	logger *slog.Logger,
) (bool, error) {
	if info.vm.Status != models.VMStatusRunning {
		return false, nil
	}
	if info.vm.TemplateID == nil || *info.vm.TemplateID != payload.TemplateID {
		return false, nil
	}

	logger.Info("VM already running with correct template, reinstall complete (idempotent)")
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM already reinstalled"); err != nil {
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
	return true, nil
}

// stopVMForReinstall stops the VM gracefully before reinstallation.
// Returns an error only if the stop operation critically fails.
func stopVMForReinstall(
	ctx context.Context,
	deps *HandlerDeps,
	info *vmReinstallInfo,
	logger *slog.Logger,
) error {
	if info.vm.Status != models.VMStatusRunning && info.vm.Status != models.VMStatusSuspended {
		logger.Info("VM already stopped, skipping stop step (idempotent)")
		return nil
	}
	logger.Info("stopping VM for reinstallation")
	if err := stopVMGracefully(ctx, deps.NodeClient, info.nodeID, info.vm.ID, 30, logger); err != nil {
		logger.Warn("stop attempt returned error, continuing with reinstallation", "error", err)
	}
	return nil
}

// replaceVMDisk deletes the old disk and clones a fresh one from the template.
func replaceVMDisk(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMReinstallPayload,
	info *vmReinstallInfo,
	logger *slog.Logger,
) error {
	logger.Info("deleting existing disk volume")
	if err := deps.NodeClient.DeleteDisk(ctx, info.nodeID, payload.VMID); err != nil {
		logger.Warn("delete disk returned error, continuing with clone", "error", err)
	}

	logger.Info("cloning fresh disk from template",
		"rbd_image", info.template.RBDImage,
		"rbd_snapshot", info.template.RBDSnapshot,
		"disk_gb", info.vm.DiskGB)

	if err := deps.NodeClient.CloneFromTemplate(ctx, info.nodeID, payload.VMID,
		info.template.RBDImage, info.template.RBDSnapshot, info.vm.DiskGB); err != nil {
		logger.Error("failed to clone template", "error", err)
		return fmt.Errorf("cloning template %s for VM %s: %w", payload.TemplateID, payload.VMID, err)
	}
	return nil
}

// regenerateCloudInitForReinstall creates a new cloud-init ISO for the reinstalled VM.
func regenerateCloudInitForReinstall(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMReinstallPayload,
	info *vmReinstallInfo,
	logger *slog.Logger,
) error {
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

	passwordHash, err := hashPassword(payload.Password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	cloudInitCfg := &CloudInitConfig{
		VMID:             payload.VMID,
		Hostname:         info.vm.Hostname,
		RootPasswordHash: passwordHash,
		SSHPublicKeys:    payload.SSHKeys,
		IPv4Address:      ipv4Addr,
		IPv4Gateway:      ipv4Gateway,
		IPv6Address:      ipv6Addr,
		IPv6Gateway:      ipv6Gateway,
		Nameservers:      append([]string(nil), deps.DNSNameservers...),
	}

	if _, err := deps.NodeClient.GenerateCloudInit(ctx, info.nodeID, cloudInitCfg); err != nil {
		logger.Error("failed to generate cloud-init", "error", err)
		return fmt.Errorf("generating cloud-init for VM %s: %w", payload.VMID, err)
	}
	return nil
}

// setReinstallResult updates the VM status and sets the task result.
func setReinstallResult(
	ctx context.Context,
	deps *HandlerDeps,
	task *models.Task,
	payload *VMReinstallPayload,
	info *vmReinstallInfo,
	logger *slog.Logger,
) error {
	// Update VM status to running
	if err := deps.VMRepo.TransitionStatus(ctx, payload.VMID, models.VMStatusReinstalling, models.VMStatusRunning); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			logger.Error("failed VM transition from reinstalling to running", "error", err)
			return fmt.Errorf("transitioning VM %s to running: %w", payload.VMID, err)
		}
		logger.Warn("failed to transition VM status to running", "error", err)
	}

	// Update VM template_id
	if err := deps.VMRepo.UpdateTemplateID(ctx, payload.VMID, payload.TemplateID); err != nil {
		logger.Warn("failed to update VM template_id", "error", err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM reinstalled successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":       payload.VMID,
		"template_id": payload.TemplateID,
		"hostname":    info.vm.Hostname,
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
