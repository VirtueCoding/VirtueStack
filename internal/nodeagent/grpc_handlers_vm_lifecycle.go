// Package nodeagent provides gRPC handlers for VM lifecycle operations.
// This file contains handlers for VM reinstall, resize, and migration operations.
//
//nolint:gosec // gRPC numeric fields are validated for positive values before libvirt conversions.
package nodeagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/guest"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"libvirt.org/go/libvirt"
)

// ReinstallVM reimages a virtual machine from a new template.
func (h *grpcHandler) ReinstallVM(ctx context.Context, req *nodeagentpb.ReinstallVMRequest) (*nodeagentpb.CreateVMResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "reinstall")
	logger.Info("reinstalling VM")

	vcpu := int(req.GetVcpu())
	memoryMB := int(req.GetMemoryMb())
	if vcpu <= 0 {
		return nil, status.Error(codes.InvalidArgument, "vcpu must be positive")
	}
	if memoryMB <= 0 {
		return nil, status.Error(codes.InvalidArgument, "memory_mb must be positive")
	}
	if req.GetMacAddress() == "" {
		return nil, status.Error(codes.InvalidArgument, "mac_address is required")
	}

	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = h.server.config.StorageBackend
		if storageBackend == "" {
			storageBackend = vm.StorageBackendCeph
		}
	}

	cloudInitPath, err := h.server.generateCloudInitISO(ctx, storage.CloudInitConfig{
		VMID:             req.GetVmId(),
		Hostname:         req.GetHostname(),
		RootPasswordHash: req.GetRootPasswordHash(),
		SSHPublicKeys:    req.GetSshPublicKeys(),
		IPv4Address:      req.GetIpv4Address(),
		IPv4Gateway:      req.GetIpv4Gateway(),
		IPv6Address:      req.GetIpv6Address(),
		IPv6Gateway:      req.GetIpv6Gateway(),
		Nameservers:      req.GetNameservers(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generating cloud-init: %v", err)
	}

	// Stop the VM if running
	if err := h.server.vmManager.ForceStopVM(ctx, req.GetVmId()); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			logger.Warn("failed to stop VM before reinstall", "error", err)
		}
	}

	var diskPath string
	switch storageBackend {
	case vm.StorageBackendQcow:
		templatePath := req.GetTemplateFilePath()
		if templatePath == "" {
			return nil, status.Error(codes.InvalidArgument, "template_file_path is required for qcow storage backend")
		}
		if err := validatePath(templatePath, h.server.config.StoragePath); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid template_file_path: %v", err)
		}

		templateMgr, ok := h.server.templateMgr.(*storage.QCOWTemplateManager)
		if !ok {
			return nil, status.Error(codes.Internal, "QCOW template manager not available")
		}

		diskPath = fmt.Sprintf("%s/vms/%s-disk0.qcow2", h.server.config.StoragePath, req.GetVmId())

		// Get current disk size before deletion (similar to LVM approach)
		currentSizeGB := 20 // Default fallback
		if existingSize, err := h.server.storageBackend.GetImageSize(ctx, diskPath); err == nil && existingSize > 0 {
			currentSizeGB = int(existingSize / (1024 * 1024 * 1024))
			if currentSizeGB < 1 {
				currentSizeGB = 20 // Minimum 20GB
			}
		}

		// Use requested disk size if provided, otherwise use current size
		diskSizeGB := currentSizeGB
		if req.GetDiskSizeGb() > 0 {
			diskSizeGB = int(req.GetDiskSizeGb())
		}

		if err := h.server.storageBackend.Delete(ctx, diskPath); err != nil {
			logger.Warn("failed to delete old QCOW disk", "error", err)
		}

		newDiskPath, err := templateMgr.CloneForVM(ctx, templatePath, req.GetVmId(), diskSizeGB)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "cloning QCOW template: %v", err)
		}
		diskPath = newDiskPath
		logger.Info("cloned QCOW template for reinstall", "template", templatePath, "disk_path", diskPath, "size_gb", diskSizeGB)

	case vm.StorageBackendCeph:
		if req.GetTemplateRbdImage() == "" || req.GetTemplateRbdSnapshot() == "" {
			return nil, status.Error(codes.InvalidArgument, "template_rbd_image and template_rbd_snapshot are required for ceph storage backend")
		}

		cephPool := req.GetCephPool()
		if cephPool == "" {
			cephPool = h.server.config.CephPool
		}
		diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
		if err := h.server.storageBackend.Delete(ctx, diskName); err != nil {
			logger.Warn("failed to delete old RBD disk", "error", err)
		}

		if err := h.server.storageBackend.CloneFromTemplate(
			ctx,
			cephPool,
			req.GetTemplateRbdImage(),
			req.GetTemplateRbdSnapshot(),
			diskName,
		); err != nil {
			return nil, status.Errorf(codes.Internal, "cloning RBD template: %v", err)
		}
		logger.Info("cloned RBD template for reinstall", "template", req.GetTemplateRbdImage(), "snapshot", req.GetTemplateRbdSnapshot())

	case vm.StorageBackendLVM:
		templatePath := req.GetTemplateFilePath()
		if templatePath == "" {
			return nil, status.Error(codes.InvalidArgument, "template_file_path is required for lvm storage backend")
		}

		// Get current disk size before deletion
		diskIdentifier := h.server.storageBackend.DiskIdentifier(req.GetVmId())
		currentSizeGB := 20 // Default fallback
		if currentSizeBytes, err := h.server.storageBackend.GetImageSize(ctx, diskIdentifier); err == nil && currentSizeBytes > 0 {
			currentSizeGB = int(currentSizeBytes / (1024 * 1024 * 1024))
			if currentSizeGB < 1 {
				currentSizeGB = 20 // Minimum 20GB
			}
		}

		// Use requested disk size if provided, otherwise use current size
		diskSizeGB := currentSizeGB
		if req.GetDiskSizeGb() > 0 {
			diskSizeGB = int(req.GetDiskSizeGb())
		}

		// Delete old disk
		if err := h.server.storageBackend.Delete(ctx, diskIdentifier); err != nil {
			logger.Warn("failed to delete old LVM disk", "error", err)
		}

		// Clone template with determined disk size
		newDiskPath, err := h.server.templateMgr.CloneForVM(ctx, templatePath, req.GetVmId(), diskSizeGB)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "cloning LVM template: %v", err)
		}
		diskPath = newDiskPath
		logger.Info("cloned LVM template for reinstall", "template", templatePath, "disk_path", diskPath, "size_gb", diskSizeGB)

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported storage backend: %s", storageBackend)
	}

	// Delete the old domain definition
	if err := h.server.vmManager.DeleteVM(ctx, req.GetVmId()); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			logger.Warn("failed to delete old domain", "error", err)
		}
	}

	// Re-create domain from existing config with new template
	cfg := &vm.DomainConfig{
		VMID:             req.GetVmId(),
		Hostname:         req.GetHostname(),
		VCPU:             vcpu,
		MemoryMB:         memoryMB,
		StorageBackend:   storageBackend,
		DiskPath:         diskPath,
		CephPool:         req.GetCephPool(),
		CephMonitors:     req.GetCephMonitors(),
		CephUser:         req.GetCephUser(),
		CephSecretUUID:   req.GetCephSecretUuid(),
		MACAddress:       req.GetMacAddress(),
		IPv4Address:      req.GetIpv4Address(),
		IPv6Address:      req.GetIpv6Address(),
		PortSpeedKbps:    int(req.GetPortSpeedMbps()) * 1000,
		CloudInitISOPath: cloudInitPath,
	}
	if cfg.CephPool == "" {
		cfg.CephPool = h.server.config.CephPool
	}
	if cfg.CephUser == "" {
		cfg.CephUser = h.server.config.CephUser
	}
	if storageBackend == vm.StorageBackendLVM {
		cfg.LVMDiskPath = diskPath
		cfg.DiskPath = ""
	}

	result, err := h.server.vmManager.CreateVM(ctx, cfg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "recreating VM: %v", err)
	}

	// Set root password via guest agent if available
	if req.GetRootPasswordHash() != "" {
		h.server.trackBackgroundGoroutine(func() {
			time.Sleep(30 * time.Second)
			domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
			if err != nil {
				logger.Warn("failed to lookup domain for password set", "error", err)
				return
			}
			defer func() {
				if err := domain.Free(); err != nil {
					logger.Debug("failed to free domain after password set", "error", err)
				}
			}()
			agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
			if err := agent.SetUserPassword(ctx, "root", req.GetRootPasswordHash()); err != nil {
				logger.Error("failed to set root password via guest agent", "error", err)
			} else {
				logger.Info("root password set via guest agent", "vm_id", req.GetVmId())
			}
		})
	}

	return &nodeagentpb.CreateVMResponse{
		VmId:              req.GetVmId(),
		Success:           true,
		LibvirtDomainName: result.DomainName,
		VncPort:           result.VNCPort,
		CloudInitPath:     cloudInitPath,
	}, nil
}

// ResizeVM modifies the resource allocation for a virtual machine.
func (h *grpcHandler) ResizeVM(ctx context.Context, req *nodeagentpb.ResizeVMRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	// Validate resource fields (QG-05: input validation)
	// Note: 0 means keep current value, so only negative values are invalid
	if req.GetNewVcpu() < 0 {
		return nil, status.Error(codes.InvalidArgument, "new_vcpu cannot be negative (0 keeps current)")
	}
	if req.GetNewMemoryMb() < 0 {
		return nil, status.Error(codes.InvalidArgument, "new_memory_mb cannot be negative (0 keeps current)")
	}
	if req.GetNewDiskGb() < 0 {
		return nil, status.Error(codes.InvalidArgument, "new_disk_gb cannot be negative (0 keeps current)")
	}

	// Validate storage_backend if provided
	if storageBackend := req.GetStorageBackend(); storageBackend != "" {
		if storageBackend != vm.StorageBackendCeph && storageBackend != vm.StorageBackendQcow && storageBackend != vm.StorageBackendLVM {
			return nil, status.Errorf(codes.InvalidArgument, "invalid storage_backend: %s (must be 'ceph', 'qcow', or 'lvm')", storageBackend)
		}
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "resize")
	logger.Info("resizing VM", "new_vcpu", req.GetNewVcpu(), "new_memory_mb", req.GetNewMemoryMb(), "new_disk_gb", req.GetNewDiskGb())

	// Stop the VM first — resize requires the VM to be stopped
	vmStatus, err := h.server.vmManager.GetStatus(ctx, req.GetVmId())
	if err != nil {
		return nil, h.mapError(err, "getting VM status")
	}
	if vmStatus.Status == "running" {
		if err := h.server.vmManager.StopVM(ctx, req.GetVmId(), 120); err != nil {
			return nil, status.Errorf(codes.Internal, "stopping VM for resize: %v", err)
		}
	}

	// Resize disk (can only grow)
	if req.GetNewDiskGb() > 0 {
		storageBackend := req.GetStorageBackend()
		if storageBackend == "" {
			storageBackend = h.server.config.StorageBackend
			if storageBackend == "" {
				storageBackend = vm.StorageBackendCeph
			}
		}

		switch storageBackend {
		case vm.StorageBackendQcow:
			diskPath := req.GetDiskPath()
			if diskPath == "" {
				diskPath = fmt.Sprintf("%s/vms/%s-disk0.qcow2", h.server.config.StoragePath, req.GetVmId())
			}
			if err := validatePath(diskPath, h.server.config.StoragePath); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid disk_path: %v", err)
			}
			if err := h.server.storageBackend.Resize(ctx, diskPath, int(req.GetNewDiskGb())); err != nil {
				return nil, status.Errorf(codes.Internal, "resizing QCOW disk: %v", err)
			}
			logger.Info("QCOW disk resized", "path", diskPath, "size_gb", req.GetNewDiskGb())

		case vm.StorageBackendCeph:
			diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
			if err := h.server.storageBackend.Resize(ctx, diskName, int(req.GetNewDiskGb())); err != nil {
				return nil, status.Errorf(codes.Internal, "resizing RBD disk: %v", err)
			}
			logger.Info("RBD disk resized", "name", diskName, "size_gb", req.GetNewDiskGb())

		case vm.StorageBackendLVM:
			diskIdentifier := h.server.storageBackend.DiskIdentifier(req.GetVmId())
			if err := h.server.storageBackend.Resize(ctx, diskIdentifier, int(req.GetNewDiskGb())); err != nil {
				return nil, status.Errorf(codes.Internal, "resizing LVM disk: %v", err)
			}
			logger.Info("LVM disk resized", "path", diskIdentifier, "size_gb", req.GetNewDiskGb())
		}
	}

	// Resize CPU/RAM by redefining domain XML
	if req.GetNewVcpu() > 0 || req.GetNewMemoryMb() > 0 {
		domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "looking up domain: %v", err)
		}
		defer func() {
			if err := domain.Free(); err != nil {
				logger.Debug("failed to free domain after resize", "error", err)
			}
		}()

		if req.GetNewVcpu() > 0 {
			if err := domain.SetVcpusFlags(uint(req.GetNewVcpu()), libvirt.DOMAIN_VCPU_CONFIG|libvirt.DOMAIN_VCPU_MAXIMUM); err != nil {
				logger.Warn("failed to set max vcpus", "error", err)
			}
			if err := domain.SetVcpusFlags(uint(req.GetNewVcpu()), libvirt.DOMAIN_VCPU_CONFIG); err != nil {
				return nil, status.Errorf(codes.Internal, "setting vcpus: %v", err)
			}
		}

		if req.GetNewMemoryMb() > 0 {
			memKB := uint64(req.GetNewMemoryMb()) * 1024
			if err := domain.SetMemoryFlags(memKB, libvirt.DOMAIN_MEM_CONFIG|libvirt.DOMAIN_MEM_MAXIMUM); err != nil {
				logger.Warn("failed to set max memory", "error", err)
			}
			if err := domain.SetMemoryFlags(memKB, libvirt.DOMAIN_MEM_CONFIG); err != nil {
				return nil, status.Errorf(codes.Internal, "setting memory: %v", err)
			}
		}
	}

	// Restart the VM
	if err := h.server.vmManager.StartVM(ctx, req.GetVmId()); err != nil {
		logger.Warn("failed to restart VM after resize", "error", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// MigrateVM initiates a live migration to a target node.
func (h *grpcHandler) MigrateVM(_ context.Context, req *nodeagentpb.MigrateVMRequest) (*nodeagentpb.MigrateVMResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetDestinationNodeAddress() == "" {
		return nil, status.Error(codes.InvalidArgument, "destination_node_address is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "destination", req.GetDestinationNodeAddress())
	logger.Info("initiating VM migration")

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			logger.Debug("failed to free domain after migration", "error", err)
		}
	}()

	destURI := fmt.Sprintf("qemu+tls://%s/system", req.GetDestinationNodeAddress())

	var flags libvirt.DomainMigrateFlags
	flags = libvirt.MIGRATE_PERSIST_DEST | libvirt.MIGRATE_UNDEFINE_SOURCE
	if req.GetLive() {
		flags |= libvirt.MIGRATE_LIVE | libvirt.MIGRATE_PEER2PEER | libvirt.MIGRATE_TUNNELLED
	}

	startTime := time.Now()

	_, err = domain.Migrate(
		h.server.libvirtConn,
		flags,
		"",
		destURI,
		0,
	)
	if err != nil {
		return &nodeagentpb.MigrateVMResponse{
			VmId:         req.GetVmId(),
			Success:      false,
			ErrorMessage: fmt.Sprintf("migration failed: %v", err),
		}, nil
	}

	downtimeMs := time.Since(startTime).Milliseconds()

	logger.Info("VM migration completed", "downtime_ms", downtimeMs)

	return &nodeagentpb.MigrateVMResponse{
		VmId:       req.GetVmId(),
		Success:    true,
		DowntimeMs: downtimeMs,
	}, nil
}

// AbortMigration cancels an in-progress VM migration.
func (h *grpcHandler) AbortMigration(_ context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "abort-migration")

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			logger.Debug("failed to free domain after abort migration", "error", err)
		}
	}()

	if err := domain.AbortJob(); err != nil {
		return nil, status.Errorf(codes.Internal, "aborting migration: %v", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// PostMigrateSetup performs post-migration setup on the destination node.
func (h *grpcHandler) PostMigrateSetup(ctx context.Context, req *nodeagentpb.PostMigrateSetupRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "post-migrate-setup")
	logger.Info("performing post-migration setup")

	// Re-apply bandwidth limits if configured
	if req.GetBandwidth() != nil && req.GetBandwidth().GetAverageKbps() > 0 {
		bwManager := h.server.newBandwidthManager()
		domainName := vm.DomainNameFromID(req.GetVmId())
		rateKbps := int(req.GetBandwidth().GetAverageKbps())
		if req.GetIsThrottled() && req.GetThrottleRateKbps() > 0 {
			rateKbps = int(req.GetThrottleRateKbps())
		}
		if err := bwManager.ApplyThrottle(ctx, domainName, rateKbps); err != nil {
			logger.Warn("failed to apply bandwidth limit after migration", "error", err)
		}
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}
