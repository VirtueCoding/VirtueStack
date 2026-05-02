// Package tasks provides the VM creation task handler.
// This file contains the handleVMCreate function which handles the full
// VM provisioning flow including template resolution, IP allocation, and Node
// Agent materialization.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// handleVMCreate handles the full VM provisioning flow.
// Steps:
//  1. Parse payload
//  2. Resolve template and network inputs
//  3. Materialize VM through Node Agent
//  4. Update VM status
func handleVMCreate(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := taskLogger(deps.Logger, task)

	// Parse payload
	var payload VMCreatePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.create payload", "error", err)
		return fmt.Errorf("parsing vm.create payload: %w", err)
	}

	passwordHash, err := hashPasswordForCloudInit(payload.Password)
	if err != nil {
		logger.Error("failed to hash root password", "error", err)
		return fmt.Errorf("hashing password: %w", err)
	}
	payload.Password = ""

	logger.Info("vm.create task started",
		"node_id", payload.NodeID,
		"hostname", payload.Hostname,
		"template_id", payload.TemplateID)

	compensationStack := NewCompensationStack(logger)
	rollbackWithErrorStatus := func(cause error) {
		compensationStack.Rollback(ctx)
		if transitionErr := deps.VMRepo.TransitionStatus(ctx, payload.VMID, models.VMStatusProvisioning, models.VMStatusError); transitionErr != nil {
			logger.Error("failed to transition VM to error after rollback", "cause", cause, "transition_error", transitionErr)
		}
	}

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting VM provisioning..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get node information
	node, err := deps.NodeRepo.GetByID(ctx, payload.NodeID)
	if err != nil {
		logger.Error("failed to get node", "node_id", payload.NodeID, "error", err)
		return fmt.Errorf("getting node %s: %w", payload.NodeID, err)
	}

	// Get template information
	template, err := deps.TemplateRepo.GetByID(ctx, payload.TemplateID)
	if err != nil {
		logger.Error("failed to get template", "template_id", payload.TemplateID, "error", err)
		return fmt.Errorf("getting template %s: %w", payload.TemplateID, err)
	}

	// Get VM record to retrieve customer and location info
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "vm_id", payload.VMID, "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	// Generate MAC address if not set
	macAddress := vm.MACAddress
	if macAddress == "" {
		macAddress = generateMACAddress(payload.VMID)
	}

	// Update task progress: Preparing VM materialization inputs
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Preparing VM materialization inputs..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	var templateFilePath string
	if template.StorageBackend != "" && template.StorageBackend != models.StorageBackendCeph {
		templateFilePath, err = resolveTemplatePathForNode(ctx, deps, template, payload.NodeID)
		if err != nil {
			logger.Error("failed to resolve template path for node", "error", err)
			return fmt.Errorf("resolving template for VM %s: %w", payload.VMID, err)
		}
	}

	// Update task progress: Allocating network resources
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Allocating network resources..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Allocate IPv4 address if needed
	var ipv4Addr, ipv4Gateway string
	locationID := ""
	if node.LocationID != nil {
		locationID = *node.LocationID
	}

	ip, err := deps.IPAMService.AllocateIPv4(ctx, locationID, payload.VMID, payload.CustomerID)
	if err != nil {
		logger.Warn("failed to allocate IPv4, using DHCP", "error", err)
	} else if ip != nil {
		ipv4Addr = ip.Address
		// Gateway would come from the IP set configuration
	}

	// Allocate IPv6 subnet
	var ipv6Addr, ipv6Gateway string
	ipv6Subnet, err := deps.IPAMService.AllocateIPv6(ctx, payload.VMID, payload.CustomerID, payload.NodeID)
	if err != nil {
		logger.Warn("failed to allocate IPv6 subnet", "error", err)
	} else if ipv6Subnet != nil {
		ipv6Addr = ipv6Subnet.Subnet
		ipv6Gateway = ipv6Subnet.Gateway
	}
	compensationStack.Push("release-ips", func(cleanupCtx context.Context) error {
		return deps.IPAMService.ReleaseIPsByVM(cleanupCtx, payload.VMID)
	})

	// Update task progress: Creating VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Creating virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Create VM via node agent gRPC
	createReq := &CreateVMRequest{
		VMID:                payload.VMID,
		Hostname:            payload.Hostname,
		VCPU:                payload.VCPU,
		MemoryMB:            payload.MemoryMB,
		DiskGB:              payload.DiskGB,
		StorageBackend:      template.StorageBackend,
		TemplateFilePath:    templateFilePath,
		TemplateRBDImage:    template.RBDImage,
		TemplateRBDSnapshot: template.RBDSnapshot,
		RootPasswordHash:    passwordHash,
		SSHPublicKeys:       payload.SSHKeys,
		IPv4Address:         ipv4Addr,
		IPv4Gateway:         ipv4Gateway,
		IPv6Address:         ipv6Addr,
		IPv6Gateway:         ipv6Gateway,
		MACAddress:          macAddress,
		PortSpeedMbps:       vm.PortSpeedMbps,
		CephPool:            node.CephPool,
		CephUser:            deps.CephUser,
		CephSecretUUID:      deps.CephSecretUUID,
		CephMonitors:        append([]string(nil), deps.CephMonitors...),
		Nameservers:         append([]string(nil), deps.DNSNameservers...),
	}
	createResp, err := deps.NodeClient.CreateVM(ctx, payload.NodeID, createReq)
	if err != nil {
		logger.Error("failed to create VM via node agent", "error", err)
		rollbackWithErrorStatus(err)
		return fmt.Errorf("creating VM %s via node agent: %w", payload.VMID, err)
	}
	compensationStack.Push("delete-vm", func(cleanupCtx context.Context) error {
		return deps.NodeClient.DeleteVM(cleanupCtx, payload.NodeID, payload.VMID)
	})

	// Update VM status to running
	if err := deps.VMRepo.TransitionStatus(ctx, payload.VMID, models.VMStatusProvisioning, models.VMStatusRunning); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			logger.Error("failed VM transition from provisioning to running", "error", err)
			rollbackWithErrorStatus(err)
			return fmt.Errorf("transitioning VM %s to running: %w", payload.VMID, err)
		}
		logger.Error("failed to transition VM status to running; VM may be running while DB still shows provisioning", "error", err)
		rollbackWithErrorStatus(err)
		return fmt.Errorf("transitioning VM %s to running: %w", payload.VMID, err)
	}

	// Persist template_id and mac_address onto the VM record
	if payload.TemplateID != "" {
		if err := deps.VMRepo.UpdateTemplateID(ctx, payload.VMID, payload.TemplateID); err != nil {
			logger.Warn("failed to update VM template_id", "error", err)
		}
	}
	if macAddress != "" {
		if err := deps.VMRepo.UpdateMACAddress(ctx, payload.VMID, macAddress); err != nil {
			logger.Error("failed to update VM mac_address", "error", err)
			rollbackWithErrorStatus(err)
			return fmt.Errorf("updating VM %s mac address: %w", payload.VMID, err)
		}
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM provisioned successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":           payload.VMID,
		"domain_name":     createResp.DomainName,
		"vnc_port":        createResp.VNCPort,
		"ipv4_address":    ipv4Addr,
		"ipv6_subnet":     ipv6Addr,
		"cloud_init_path": createResp.CloudInitPath,
	}
	// json.Marshal error is intentionally suppressed: the map contains only
	// primitive types (string, int, bool) whose marshaling cannot fail.
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.create task completed successfully",
		"vm_id", payload.VMID,
		"domain_name", createResp.DomainName,
		"ipv4", ipv4Addr)

	return nil
}

func resolveTemplatePathForNode(ctx context.Context, deps *HandlerDeps, template *models.Template, nodeID string) (string, error) {
	if template.StorageBackend == "" || template.StorageBackend == models.StorageBackendCeph {
		return "", nil
	}
	if deps.TemplateCacheRepo == nil {
		return "", fmt.Errorf("template cache repository not configured")
	}

	entry, err := deps.TemplateCacheRepo.Get(ctx, template.ID, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return "", fmt.Errorf("template %s is not cached on node %s; distribute it first", template.ID, nodeID)
		}
		return "", fmt.Errorf("getting template cache entry: %w", err)
	}
	if entry.Status != models.TemplateCacheStatusReady {
		return "", fmt.Errorf("template %s cache on node %s is %s", template.ID, nodeID, entry.Status)
	}
	if entry.LocalPath == nil || *entry.LocalPath == "" {
		return "", fmt.Errorf("template %s cache on node %s has no local path", template.ID, nodeID)
	}

	return *entry.LocalPath, nil
}
