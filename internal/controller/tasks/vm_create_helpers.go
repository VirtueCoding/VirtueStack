// Package tasks provides helper functions for VM creation task handling.
// These functions decompose the large handleVMCreate function to comply with
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

// vmCreateInfo contains the gathered information needed for VM creation.
type vmCreateInfo struct {
	node     *models.Node
	template *models.Template
	vm       *models.VM
	macAddr  string
}

// prepareVMCreateInfo fetches all required records for VM creation.
// Returns vmCreateInfo containing node, template, VM records and MAC address.
func prepareVMCreateInfo(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMCreatePayload,
	logger *slog.Logger,
) (*vmCreateInfo, error) {
	// Get node information
	node, err := deps.NodeRepo.GetByID(ctx, payload.NodeID)
	if err != nil {
		logger.Error("failed to get node", "node_id", payload.NodeID, "error", err)
		return nil, fmt.Errorf("getting node %s: %w", payload.NodeID, err)
	}

	// Get template information
	template, err := deps.TemplateRepo.GetByID(ctx, payload.TemplateID)
	if err != nil {
		logger.Error("failed to get template", "template_id", payload.TemplateID, "error", err)
		return nil, fmt.Errorf("getting template %s: %w", payload.TemplateID, err)
	}

	// Get VM record to retrieve customer and location info
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "vm_id", payload.VMID, "error", err)
		return nil, fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	// Generate MAC address if not set
	macAddr := vm.MACAddress
	if macAddr == "" {
		macAddr = generateMACAddress(payload.VMID)
	}

	return &vmCreateInfo{
		node:     node,
		template: template,
		vm:       vm,
		macAddr:  macAddr,
	}, nil
}

// vmNetworking contains allocated IP addresses for a VM.
type vmNetworking struct {
	ipv4Addr    string
	ipv4Gateway string
	ipv6Addr    string
	ipv6Gateway string
}

// allocateVMNetworking allocates IPv4 and IPv6 addresses for a VM.
// Returns the allocated addresses or warnings if allocation fails.
func allocateVMNetworking(
	ctx context.Context,
	deps *HandlerDeps,
	payload *VMCreatePayload,
	info *vmCreateInfo,
	logger *slog.Logger,
) *vmNetworking {
	net := &vmNetworking{}
	locationID := ""
	if info.node.LocationID != nil {
		locationID = *info.node.LocationID
	}

	// Allocate IPv4 address
	ip, err := deps.IPAMService.AllocateIPv4(ctx, payload.VMID, payload.CustomerID, locationID)
	if err != nil {
		logger.Warn("failed to allocate IPv4, using DHCP", "error", err)
	} else if ip != nil {
		net.ipv4Addr = ip.Address
		// Gateway would come from the IP set configuration
	}

	// Allocate IPv6 subnet
	ipv6Subnet, err := deps.IPAMService.AllocateIPv6(ctx, payload.VMID, payload.CustomerID, payload.NodeID)
	if err != nil {
		logger.Warn("failed to allocate IPv6 subnet", "error", err)
	} else if ipv6Subnet != nil {
		net.ipv6Addr = ipv6Subnet.Subnet
		net.ipv6Gateway = ipv6Subnet.Gateway
	}

	return net
}

// buildCreateVMRequest constructs a CreateVMRequest from the given parameters.
func buildCreateVMRequest(
	payload *VMCreatePayload,
	info *vmCreateInfo,
	net *vmNetworking,
	passwordHash string,
	deps *HandlerDeps,
) *CreateVMRequest {
	return &CreateVMRequest{
		VMID:                payload.VMID,
		Hostname:            payload.Hostname,
		VCPU:                payload.VCPU,
		MemoryMB:            payload.MemoryMB,
		DiskGB:              payload.DiskGB,
		TemplateRBDImage:    info.template.RBDImage,
		TemplateRBDSnapshot: info.template.RBDSnapshot,
		RootPasswordHash:    passwordHash,
		SSHPublicKeys:       payload.SSHKeys,
		IPv4Address:         net.ipv4Addr,
		IPv4Gateway:         net.ipv4Gateway,
		IPv6Address:         net.ipv6Addr,
		IPv6Gateway:         net.ipv6Gateway,
		MACAddress:          info.macAddr,
		PortSpeedMbps:       info.vm.PortSpeedMbps,
		CephPool:            info.node.CephPool,
		CephUser:            deps.CephUser,
		CephSecretUUID:      deps.CephSecretUUID,
		CephMonitors:        append([]string(nil), deps.CephMonitors...),
		Nameservers:         append([]string(nil), deps.DNSNameservers...),
	}
}

// cleanupFailedVMCreate performs cleanup when VM creation fails.
// It attempts to delete the disk and release IP addresses.
func cleanupFailedVMCreate(
	ctx context.Context,
	deps *HandlerDeps,
	nodeID, vmID string,
	logger *slog.Logger,
) {
	if err := deps.NodeClient.DeleteDisk(ctx, nodeID, vmID); err != nil {
		logger.Error("failed to cleanup disk on VM creation failure", "operation", "DeleteDisk", "err", err)
	}
	if err := deps.IPAMService.ReleaseIPsByVM(ctx, vmID); err != nil {
		logger.Error("failed to release IPs on VM creation failure", "operation", "ReleaseIPsByVM", "err", err)
	}
}

// setVMCreateResult updates the VM status and sets the task result.
func setVMCreateResult(
	ctx context.Context,
	deps *HandlerDeps,
	task *models.Task,
	payload *VMCreatePayload,
	createResp *CreateVMResponse,
	net *vmNetworking,
	cloudInitPath string,
	logger *slog.Logger,
) error {
	// Update VM status to running
	if err := deps.VMRepo.TransitionStatus(ctx, payload.VMID, models.VMStatusProvisioning, models.VMStatusRunning); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			logger.Error("failed VM transition from provisioning to running", "error", err)
			return fmt.Errorf("transitioning VM %s to running: %w", payload.VMID, err)
		}
		logger.Warn("failed to transition VM status to running", "error", err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM provisioned successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := VMCreateResult{
		VMID:          payload.VMID,
		DomainName:    createResp.DomainName,
		VNCPort:       createResp.VNCPort,
		IPv4Address:   net.ipv4Addr,
		IPv6Subnet:    net.ipv6Addr,
		CloudInitPath: cloudInitPath,
	}
	// json.Marshal error is intentionally suppressed: the struct contains only
	// primitive types (string, int) whose marshaling cannot fail.
	resultJSON := marshalResult(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.create task completed successfully",
		"vm_id", payload.VMID,
		"domain_name", createResp.DomainName,
		"ipv4", net.ipv4Addr)

	return nil
}

// marshalResult is a helper to marshal result to JSON.
// The error is intentionally not returned as the struct contains only
// primitive types whose marshaling cannot fail.
func marshalResult(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}
