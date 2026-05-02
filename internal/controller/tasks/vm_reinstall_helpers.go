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
) bool {
	if info.vm.Status != models.VMStatusRunning {
		return false
	}
	if info.vm.TemplateID == nil || *info.vm.TemplateID != payload.TemplateID {
		return false
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
	resultJSON, err := json.Marshal(result)
	if err != nil {
		logger.Warn("failed to marshal idempotent reinstall result", "error", err)
		return true
	}
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}
	return true
}

type reinstallNetworkConfig struct {
	ipv4Address string
	ipv4Gateway string
	ipv6Address string
	ipv6Gateway string
}

func buildReinstallRequest(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMReinstallPayload,
	info *vmReinstallInfo,
	passwordHash string,
	logger *slog.Logger,
) (*ReinstallVMRequest, error) {
	templateFilePath, err := resolveReinstallTemplatePath(ctx, deps, payload, info)
	if err != nil {
		return nil, err
	}
	network := loadReinstallNetworkConfig(ctx, deps, payload, logger)
	cephPool := ""
	if info.vm.CephPool != nil {
		cephPool = *info.vm.CephPool
	}
	return &ReinstallVMRequest{
		VMID:                payload.VMID,
		Hostname:            info.vm.Hostname,
		VCPU:                info.vm.VCPU,
		MemoryMB:            info.vm.MemoryMB,
		DiskGB:              info.vm.DiskGB,
		StorageBackend:      info.template.StorageBackend,
		TemplateFilePath:    templateFilePath,
		TemplateRBDImage:    info.template.RBDImage,
		TemplateRBDSnapshot: info.template.RBDSnapshot,
		RootPasswordHash:    passwordHash,
		SSHPublicKeys:       payload.SSHKeys,
		IPv4Address:         network.ipv4Address,
		IPv4Gateway:         network.ipv4Gateway,
		IPv6Address:         network.ipv6Address,
		IPv6Gateway:         network.ipv6Gateway,
		MACAddress:          info.vm.MACAddress,
		PortSpeedMbps:       info.vm.PortSpeedMbps,
		CephMonitors:        append([]string(nil), deps.CephMonitors...),
		CephUser:            deps.CephUser,
		CephSecretUUID:      deps.CephSecretUUID,
		CephPool:            cephPool,
		Nameservers:         append([]string(nil), deps.DNSNameservers...),
	}, nil
}

func resolveReinstallTemplatePath(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMReinstallPayload,
	info *vmReinstallInfo,
) (string, error) {
	if info.template.StorageBackend == "" || info.template.StorageBackend == models.StorageBackendCeph {
		return "", nil
	}
	path, err := resolveTemplatePathForNode(ctx, deps, info.template, info.nodeID)
	if err != nil {
		return "", fmt.Errorf("resolving template for VM %s: %w", payload.VMID, err)
	}
	return path, nil
}

func loadReinstallNetworkConfig(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMReinstallPayload,
	logger *slog.Logger,
) reinstallNetworkConfig {
	network := reinstallNetworkConfig{}
	ipv4, err := deps.IPAMService.GetPrimaryIPv4(ctx, payload.VMID)
	if err != nil {
		logger.Warn("failed to get IPv4 for reinstall cloud-init", "error", err)
	} else if ipv4 != nil {
		network.ipv4Address = ipv4.Address
	}
	ipv6Subnets, err := deps.IPAMService.GetIPv6SubnetsByVM(ctx, payload.VMID)
	if err != nil {
		logger.Warn("failed to get IPv6 subnet for reinstall cloud-init", "error", err)
	} else if len(ipv6Subnets) > 0 {
		network.ipv6Address = ipv6Subnets[0].Subnet
		network.ipv6Gateway = ipv6Subnets[0].Gateway
	}
	return network
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
