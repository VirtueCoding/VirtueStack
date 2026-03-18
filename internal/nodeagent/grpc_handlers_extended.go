package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/guest"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"libvirt.org/go/libvirt"
)

// ReinstallVM reimages a virtual machine from a new template.
func (h *grpcHandler) ReinstallVM(ctx context.Context, req *nodeagentpb.ReinstallVMRequest) (*nodeagentpb.CreateVMResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "reinstall")
	logger.Info("reinstalling VM")

	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = h.server.config.StorageBackend
		if storageBackend == "" {
			storageBackend = vm.StorageBackendCeph
		}
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
		if err := h.server.storageBackend.Delete(ctx, diskPath); err != nil {
			logger.Warn("failed to delete old QCOW disk", "error", err)
		}

		newDiskPath, err := templateMgr.CloneForVM(ctx, templatePath, req.GetVmId(), 20)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "cloning QCOW template: %v", err)
		}
		diskPath = newDiskPath
		logger.Info("cloned QCOW template for reinstall", "template", templatePath, "disk_path", diskPath)

	case vm.StorageBackendCeph:
		if req.GetTemplateRbdImage() == "" || req.GetTemplateRbdSnapshot() == "" {
			return nil, status.Error(codes.InvalidArgument, "template_rbd_image and template_rbd_snapshot are required for ceph storage backend")
		}

		diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
		if err := h.server.storageBackend.Delete(ctx, diskName); err != nil {
			logger.Warn("failed to delete old RBD disk", "error", err)
		}

		if err := h.server.storageBackend.CloneFromTemplate(
			ctx,
			h.server.config.CephPool,
			req.GetTemplateRbdImage(),
			req.GetTemplateRbdSnapshot(),
			diskName,
		); err != nil {
			return nil, status.Errorf(codes.Internal, "cloning RBD template: %v", err)
		}
		logger.Info("cloned RBD template for reinstall", "template", req.GetTemplateRbdImage(), "snapshot", req.GetTemplateRbdSnapshot())

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
		VMID:           req.GetVmId(),
		Hostname:       req.GetHostname(),
		VCPU:           1,
		MemoryMB:       1024,
		StorageBackend: storageBackend,
		DiskPath:       diskPath,
		CephPool:       h.server.config.CephPool,
		CephUser:       h.server.config.CephUser,
		IPv4Address:    req.GetIpv4Address(),
		IPv6Address:    req.GetIpv6Address(),
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
			defer domain.Free()
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
		if storageBackend != vm.StorageBackendCeph && storageBackend != vm.StorageBackendQcow {
			return nil, status.Errorf(codes.InvalidArgument, "invalid storage_backend: %s (must be 'ceph' or 'qcow')", storageBackend)
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
		}
	}

	// Resize CPU/RAM by redefining domain XML
	if req.GetNewVcpu() > 0 || req.GetNewMemoryMb() > 0 {
		domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "looking up domain: %v", err)
		}
		defer domain.Free()

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
func (h *grpcHandler) MigrateVM(ctx context.Context, req *nodeagentpb.MigrateVMRequest) (*nodeagentpb.MigrateVMResponse, error) {
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
	defer domain.Free()

	destURI := fmt.Sprintf("qemu+tcp://%s/system", req.GetDestinationNodeAddress())

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
func (h *grpcHandler) AbortMigration(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

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

// StreamVNCConsole establishes a bidirectional VNC console stream.
func (h *grpcHandler) StreamVNCConsole(stream nodeagentpb.NodeAgentService_StreamVNCConsoleServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// Read first frame to get VM ID
	firstFrame, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "receiving initial frame: %v", err)
	}

	// Parse VM ID from first frame data
	vmID := string(firstFrame.GetData())
	if vmID == "" {
		return status.Error(codes.InvalidArgument, "first frame must contain vm_id")
	}

	logger := h.server.logger.With("vm_id", vmID, "operation", "vnc-console")

	// Get VNC port from the running domain
	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(vmID))
	if err != nil {
		return status.Errorf(codes.NotFound, "VM not found: %v", err)
	}

	vncPort, err := h.server.getVNCPort(domain)
	domain.Free()
	if err != nil {
		return status.Errorf(codes.Internal, "getting VNC port: %v", err)
	}

	// Connect to the VNC server using configured host or default
	vncHost := h.server.config.VNCHost
	if vncHost == "" {
		vncHost = "127.0.0.1"
	}
	vncAddr := fmt.Sprintf("%s:%d", vncHost, vncPort)
	conn, err := net.DialTimeout("tcp", vncAddr, 5*time.Second)
	if err != nil {
		return status.Errorf(codes.Unavailable, "connecting to VNC: %v", err)
	}
	defer conn.Close()

	logger.Info("VNC console stream established", "vnc_addr", vncAddr)

	errCh := make(chan error, 2)

	// Read from VNC, send to gRPC stream
	go func() {
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("reading from VNC: %w", err)
				} else {
					errCh <- nil
				}
				return
			}
			if err := stream.Send(&nodeagentpb.VNCFrame{Data: buf[:n]}); err != nil {
				errCh <- fmt.Errorf("sending to gRPC stream: %w", err)
				return
			}
		}
	}()

	// Read from gRPC stream, write to VNC
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			frame, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("receiving from gRPC stream: %w", err)
				} else {
					errCh <- nil
				}
				return
			}
			if _, err := conn.Write(frame.GetData()); err != nil {
				errCh <- fmt.Errorf("writing to VNC: %w", err)
				return
			}
		}
	}()

	// Wait for either direction to finish
	return <-errCh
}

// StreamSerialConsole establishes a bidirectional serial console stream.
func (h *grpcHandler) StreamSerialConsole(stream nodeagentpb.NodeAgentService_StreamSerialConsoleServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	firstFrame, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "receiving initial frame: %v", err)
	}

	vmID := string(firstFrame.GetData())
	if vmID == "" {
		return status.Error(codes.InvalidArgument, "first frame must contain vm_id")
	}

	logger := h.server.logger.With("vm_id", vmID, "operation", "serial-console")

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(vmID))
	if err != nil {
		return status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	// Open a console stream via libvirt
	libvirtStream, err := h.server.libvirtConn.NewStream(0)
	if err != nil {
		return status.Errorf(codes.Internal, "creating libvirt stream: %v", err)
	}
	defer libvirtStream.Free()

	if err := domain.OpenConsole("", libvirtStream, libvirt.DOMAIN_CONSOLE_FORCE); err != nil {
		return status.Errorf(codes.Internal, "opening serial console: %v", err)
	}

	logger.Info("serial console stream established")

	errCh := make(chan error, 2)

	// Read from serial console, send to gRPC stream
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := libvirtStream.Recv(buf)
			if err != nil || n < 0 {
				errCh <- nil
				return
			}
			if err := stream.Send(&nodeagentpb.SerialData{Data: buf[:n]}); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Read from gRPC stream, write to serial console
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			data, err := stream.Recv()
			if err != nil {
				errCh <- nil
				return
			}
			if _, err := libvirtStream.Send(data.GetData()); err != nil {
				errCh <- err
				return
			}
		}
	}()

	return <-errCh
}

// CreateSnapshot creates a point-in-time snapshot of a VM's disk.
func (h *grpcHandler) CreateSnapshot(ctx context.Context, req *nodeagentpb.SnapshotRequest) (*nodeagentpb.Snapshot, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
	snapName := fmt.Sprintf("snap-%s", uuid.New().String()[:8])

	if err := h.server.storageBackend.CreateSnapshot(ctx, diskName, snapName); err != nil {
		return nil, status.Errorf(codes.Internal, "creating snapshot: %v", err)
	}

	// Get snapshot size
	size, _ := h.server.storageBackend.GetImageSize(ctx, diskName)

	return &nodeagentpb.Snapshot{
		SnapshotId:      uuid.New().String(),
		VmId:            req.GetVmId(),
		Name:            req.GetName(),
		RbdSnapshotName: snapName,
		SizeBytes:       size,
		CreatedAt:       timestamppb.Now(),
	}, nil
}

// DeleteSnapshot removes a previously created disk snapshot.
func (h *grpcHandler) DeleteSnapshot(ctx context.Context, req *nodeagentpb.SnapshotIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" || req.GetSnapshotId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id and snapshot_id are required")
	}

	diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())

	// List snapshots to find by ID (use snapshot_id as the rbd snap name)
	snapshots, err := h.server.storageBackend.ListSnapshots(ctx, diskName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listing snapshots: %v", err)
	}

	for _, snap := range snapshots {
		if snap.Name == req.GetSnapshotId() || strings.HasPrefix(snap.Name, "snap-") {
			if err := h.server.storageBackend.DeleteSnapshot(ctx, diskName, snap.Name); err != nil {
				return nil, status.Errorf(codes.Internal, "deleting snapshot: %v", err)
			}
			return &nodeagentpb.VMOperationResponse{
				VmId:    req.GetVmId(),
				Success: true,
			}, nil
		}
	}

	return nil, status.Error(codes.NotFound, "snapshot not found")
}

// RevertSnapshot restores a VM to a previous snapshot state.
func (h *grpcHandler) RevertSnapshot(ctx context.Context, req *nodeagentpb.SnapshotIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" || req.GetSnapshotId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id and snapshot_id are required")
	}

	vmStatus, err := h.server.vmManager.GetStatus(ctx, req.GetVmId())
	if err != nil {
		return nil, h.mapError(err, "getting VM status")
	}
	if vmStatus.Status == "running" {
		if err := h.server.vmManager.ForceStopVM(ctx, req.GetVmId()); err != nil {
			return nil, status.Errorf(codes.Internal, "stopping VM: %v", err)
		}
	}

	diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())

	if err := h.server.storageBackend.Rollback(ctx, diskName, req.GetSnapshotId()); err != nil {
		return nil, status.Errorf(codes.Internal, "rolling back disk to snapshot: %v", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// ListSnapshots retrieves all snapshots for a given virtual machine.
func (h *grpcHandler) ListSnapshots(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.SnapshotListResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())

	snapshots, err := h.server.storageBackend.ListSnapshots(ctx, diskName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listing snapshots: %v", err)
	}

	var protoSnaps []*nodeagentpb.Snapshot
	for _, snap := range snapshots {
		protoSnaps = append(protoSnaps, &nodeagentpb.Snapshot{
			SnapshotId:      snap.Name,
			VmId:            req.GetVmId(),
			Name:            snap.Name,
			RbdSnapshotName: snap.Name,
			SizeBytes:       snap.Size,
			CreatedAt:       timestamppb.Now(),
		})
	}

	return &nodeagentpb.SnapshotListResponse{
		Snapshots: protoSnaps,
	}, nil
}

// GuestExecCommand executes a command inside the VM via QEMU guest agent.
func (h *grpcHandler) GuestExecCommand(ctx context.Context, req *nodeagentpb.GuestExecRequest) (*nodeagentpb.GuestExecResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetCommand() == "" {
		return nil, status.Error(codes.InvalidArgument, "command is required")
	}

	// Whitelist allowed commands: read-only diagnostic commands only.
	// Removed cat, ls, ip, hostname, whoami to prevent info leakage
	// (file contents, directory listings, network config, usernames).
	allowedCommands := map[string]bool{
		"df": true, "free": true, "uname": true, "date": true, "uptime": true,
	}
	fullCmd := req.GetCommand()
	cmdBase := strings.Split(fullCmd, " ")[0]

	resolvedPath, err := filepath.EvalSymlinks(cmdBase)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "command path could not be resolved: %v", err)
	}

	allowedPaths := []string{"/usr/bin/", "/bin/"}
	validPath := false
	for _, prefix := range allowedPaths {
		if strings.HasPrefix(resolvedPath, prefix) {
			validPath = true
			cmdBase = strings.TrimPrefix(resolvedPath, prefix)
			break
		}
	}
	if !validPath {
		return nil, status.Errorf(codes.PermissionDenied, "command path must be in /usr/bin or /bin")
	}

	if !allowedCommands[cmdBase] {
		return nil, status.Errorf(codes.PermissionDenied, "command %q is not in the allowed whitelist", cmdBase)
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	// Build the guest-exec command
	args := req.GetArgs()

	execCmd := map[string]interface{}{
		"execute": "guest-exec",
		"arguments": map[string]interface{}{
			"path":           fullCmd,
			"arg":            args,
			"capture-output": true,
		},
	}
	cmdJSON, _ := json.Marshal(execCmd)

	timeout := int(req.GetTimeoutSeconds())
	if timeout <= 0 {
		timeout = 10
	}

	output, err := domain.QemuAgentCommand(string(cmdJSON), libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, uint32(timeout))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "guest exec: %v", err)
	}

	// Parse pid from response
	var execResp struct {
		Return struct {
			PID int `json:"pid"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(output), &execResp); err != nil {
		return nil, status.Errorf(codes.Internal, "parsing exec response: %v", err)
	}

	// Get the execution status
	time.Sleep(500 * time.Millisecond)
	statusCmd := fmt.Sprintf(`{"execute":"guest-exec-status","arguments":{"pid":%d}}`, execResp.Return.PID)
	statusOutput, err := domain.QemuAgentCommand(statusCmd, libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, uint32(timeout))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting exec status: %v", err)
	}

	var statusResp struct {
		Return struct {
			Exited   bool   `json:"exited"`
			ExitCode int    `json:"exitcode"`
			OutData  string `json:"out-data"`
			ErrData  string `json:"err-data"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(statusOutput), &statusResp); err != nil {
		return nil, status.Errorf(codes.Internal, "parsing status response: %v", err)
	}

	return &nodeagentpb.GuestExecResponse{
		ExitCode: int32(statusResp.Return.ExitCode),
		Stdout:   []byte(statusResp.Return.OutData),
		Stderr:   []byte(statusResp.Return.ErrData),
	}, nil
}

// GuestSetPassword changes a user password inside the VM.
func (h *grpcHandler) GuestSetPassword(ctx context.Context, req *nodeagentpb.GuestPasswordRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetUsername() == "" || req.GetPasswordHash() == "" {
		return nil, status.Error(codes.InvalidArgument, "username and password_hash are required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
	if err := agent.SetUserPassword(ctx, req.GetUsername(), req.GetPasswordHash()); err != nil {
		return nil, status.Errorf(codes.Internal, "setting password: %v", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// GuestFreezeFilesystems freezes all filesystems in the VM.
func (h *grpcHandler) GuestFreezeFilesystems(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
	count, err := agent.FreezeFilesystems(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "freezing filesystems: %v", err)
	}

	h.server.logger.Info("filesystems frozen", "vm_id", req.GetVmId(), "count", count)

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// GuestThawFilesystems unfreezes all filesystems in the VM.
func (h *grpcHandler) GuestThawFilesystems(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
	count, err := agent.ThawFilesystems(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "thawing filesystems: %v", err)
	}

	h.server.logger.Info("filesystems thawed", "vm_id", req.GetVmId(), "count", count)

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// GuestGetNetworkInterfaces retrieves network interface information from the VM.
func (h *grpcHandler) GuestGetNetworkInterfaces(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.GuestNetworkResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	cmd := `{"execute":"guest-network-get-interfaces"}`
	output, err := domain.QemuAgentCommand(cmd, libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, 10)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "querying network interfaces: %v", err)
	}

	var resp struct {
		Return []struct {
			Name       string `json:"name"`
			HWAddr     string `json:"hardware-address"`
			IPAddrList []struct {
				IPAddr string `json:"ip-address"`
				Prefix int    `json:"ip-address-prefix"`
				Type   string `json:"ip-address-type"`
			} `json:"ip-addresses"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return nil, status.Errorf(codes.Internal, "parsing network response: %v", err)
	}

	var ifaces []*nodeagentpb.GuestNetworkInterface
	for _, iface := range resp.Return {
		protoIface := &nodeagentpb.GuestNetworkInterface{
			Name:       iface.Name,
			MacAddress: iface.HWAddr,
		}
		for _, addr := range iface.IPAddrList {
			ipType := nodeagentpb.IPType_IP_TYPE_IPV4
			if addr.Type == "ipv6" {
				ipType = nodeagentpb.IPType_IP_TYPE_IPV6
			}
			protoIface.IpAddresses = append(protoIface.IpAddresses, &nodeagentpb.IPAddress{
				Ip:     addr.IPAddr,
				Prefix: int32(addr.Prefix),
				Type:   ipType,
			})
		}
		ifaces = append(ifaces, protoIface)
	}

	return &nodeagentpb.GuestNetworkResponse{
		VmId:       req.GetVmId(),
		Interfaces: ifaces,
	}, nil
}

// GetBandwidthUsage retrieves current network bandwidth usage for a VM.
func (h *grpcHandler) GetBandwidthUsage(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.BandwidthUsageResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	bwManager := h.server.newBandwidthManager()
	domainName := vm.DomainNameFromID(req.GetVmId())

	bytesIn, bytesOut, err := bwManager.GetVMNetworkStats(ctx, domainName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting bandwidth usage: %v", err)
	}

	return &nodeagentpb.BandwidthUsageResponse{
		VmId:    req.GetVmId(),
		RxBytes: int64(bytesIn),
		TxBytes: int64(bytesOut),
	}, nil
}

// SetBandwidthLimit applies a bandwidth cap to a VM's network interface.
func (h *grpcHandler) SetBandwidthLimit(ctx context.Context, req *nodeagentpb.BandwidthLimitRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetLimitMbps() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "limit_mbps must be positive")
	}

	bwManager := h.server.newBandwidthManager()
	domainName := vm.DomainNameFromID(req.GetVmId())
	rateKbps := int(req.GetLimitMbps()) * 1000

	if err := bwManager.ApplyThrottle(ctx, domainName, rateKbps); err != nil {
		return nil, status.Errorf(codes.Internal, "setting bandwidth limit: %v", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// ResetBandwidthCounters resets the bandwidth usage counters for a VM.
func (h *grpcHandler) ResetBandwidthCounters(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	// Remove existing throttle and re-apply to reset counters
	bwManager := h.server.newBandwidthManager()
	domainName := vm.DomainNameFromID(req.GetVmId())

	if err := bwManager.RemoveThrottle(ctx, domainName); err != nil {
		h.server.logger.Warn("failed to remove throttle for counter reset", "error", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// CreateDiskSnapshot creates a point-in-time snapshot of a VM disk for migration.
// This is used during QCOW migration to create a consistent disk copy.
func (h *grpcHandler) CreateDiskSnapshot(ctx context.Context, req *nodeagentpb.CreateDiskSnapshotRequest) (*nodeagentpb.CreateDiskSnapshotResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetSnapshotName() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot_name is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "create-disk-snapshot")
	logger.Info("creating disk snapshot for migration", "snapshot_name", req.GetSnapshotName())

	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = string(h.server.storageType)
	}

	switch storageBackend {
	case "qcow":
		// For QCOW, use internal snapshot
		diskPath := req.GetDiskPath()
		if diskPath == "" {
			diskPath = fmt.Sprintf("%s/%s-disk0.qcow2", h.server.config.StoragePath, req.GetVmId())
		}
		if err := validatePath(diskPath, h.server.config.StoragePath); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid disk_path: %v", err)
		}

		if err := h.server.storageBackend.CreateSnapshot(ctx, diskPath, req.GetSnapshotName()); err != nil {
			return nil, status.Errorf(codes.Internal, "creating QCOW snapshot: %v", err)
		}

		return &nodeagentpb.CreateDiskSnapshotResponse{
			VmId:         req.GetVmId(),
			SnapshotName: req.GetSnapshotName(),
			Success:      true,
		}, nil

	case "ceph":
		// For Ceph, use RBD snapshot
		diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
		if err := h.server.storageBackend.CreateSnapshot(ctx, diskName, req.GetSnapshotName()); err != nil {
			return nil, status.Errorf(codes.Internal, "creating RBD snapshot: %v", err)
		}

		return &nodeagentpb.CreateDiskSnapshotResponse{
			VmId:         req.GetVmId(),
			SnapshotName: req.GetSnapshotName(),
			Success:      true,
		}, nil

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported storage backend: %s", storageBackend)
	}
}

// DeleteDiskSnapshot removes a disk snapshot created during migration.
func (h *grpcHandler) DeleteDiskSnapshot(ctx context.Context, req *nodeagentpb.DeleteDiskSnapshotRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetSnapshotName() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot_name is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "delete-disk-snapshot")
	logger.Info("deleting disk snapshot", "snapshot_name", req.GetSnapshotName())

	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = string(h.server.storageType)
	}

	switch storageBackend {
	case "qcow":
		diskPath := req.GetDiskPath()
		if diskPath == "" {
			diskPath = fmt.Sprintf("%s/%s-disk0.qcow2", h.server.config.StoragePath, req.GetVmId())
		}
		if err := validatePath(diskPath, h.server.config.StoragePath); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid disk_path: %v", err)
		}

		if err := h.server.storageBackend.DeleteSnapshot(ctx, diskPath, req.GetSnapshotName()); err != nil {
			logger.Warn("failed to delete QCOW snapshot", "error", err)
		}

	case "ceph":
		diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
		if err := h.server.storageBackend.DeleteSnapshot(ctx, diskName, req.GetSnapshotName()); err != nil {
			logger.Warn("failed to delete RBD snapshot", "error", err)
		}
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// TransferDisk initiates a disk transfer from this node to a target node.
// This is used for QCOW migrations where disk files need to be copied between nodes.
func (h *grpcHandler) TransferDisk(req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer) error {
	if req.GetSourceDiskPath() == "" {
		return status.Error(codes.InvalidArgument, "source_disk_path is required")
	}
	if req.GetTargetNodeAddress() == "" {
		return status.Error(codes.InvalidArgument, "target_node_address is required")
	}

	logger := h.server.logger.With("operation", "transfer-disk")
	logger.Info("initiating disk transfer",
		"source_disk", req.GetSourceDiskPath(),
		"target_node", req.GetTargetNodeAddress(),
		"target_disk", req.GetTargetDiskPath())

	ctx := stream.Context()

	// Get disk info
	sourcePath := req.GetSourceDiskPath()
	snapshotName := req.GetSnapshotName()

	// For QCOW with snapshot, export the snapshot
	if h.server.storageType == storage.StorageTypeQCOW && snapshotName != "" {
		// Create a temporary exported image from the snapshot
		tempPath := sourcePath + ".export"
		defer os.Remove(tempPath)

		// Use qemu-img convert to export snapshot
		args := []string{"convert", "-l", snapshotName, "-O", "qcow2", sourcePath, tempPath}
		if req.GetCompress() {
			args = append(args[:len(args)-2], "-c", args[len(args)-2], args[len(args)-1])
		}

		cmd := exec.CommandContext(ctx, "qemu-img", args...)
		if err := cmd.Run(); err != nil {
			return status.Errorf(codes.Internal, "exporting snapshot: %v", err)
		}
		sourcePath = tempPath
	}

	// Open the source file
	file, err := os.Open(sourcePath)
	if err != nil {
		return status.Errorf(codes.Internal, "opening source disk: %v", err)
	}
	defer file.Close()

	// Get file info for progress tracking
	fileInfo, err := file.Stat()
	if err != nil {
		return status.Errorf(codes.Internal, "getting file info: %v", err)
	}
	totalSize := fileInfo.Size()

	// Stream the file in chunks
	buf := make([]byte, 64*1024) // 64KB chunks
	var bytesSent int64

	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return status.Errorf(codes.Internal, "reading disk file: %v", err)
		}

		if n > 0 {
			chunk := &nodeagentpb.DiskChunk{
				Data:   buf[:n],
				Offset: bytesSent,
				Total:  totalSize,
			}

			if err := stream.Send(chunk); err != nil {
				return status.Errorf(codes.Internal, "sending disk chunk: %v", err)
			}

			bytesSent += int64(n)
		}

		if err == io.EOF {
			break
		}
	}

	logger.Info("disk transfer completed", "bytes_sent", bytesSent)
	return nil
}

// ReceiveDisk receives a disk transfer from a source node and writes it to local storage.
func (h *grpcHandler) ReceiveDisk(stream nodeagentpb.NodeAgentService_ReceiveDiskServer) error {
	// Receive the first message to get metadata
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "receiving initial message: %v", err)
	}

	targetPath := firstMsg.GetTargetDiskPath()
	if targetPath == "" {
		return status.Error(codes.InvalidArgument, "target_disk_path is required")
	}
	if err := validatePath(targetPath, h.server.config.StoragePath); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid target_disk_path: %v", err)
	}

	logger := h.server.logger.With("operation", "receive-disk", "target_path", targetPath)
	logger.Info("receiving disk transfer")

	// Create the target directory if needed
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return status.Errorf(codes.Internal, "creating target directory: %v", err)
	}

	// Create/truncate the target file
	file, err := os.Create(targetPath)
	if err != nil {
		return status.Errorf(codes.Internal, "creating target file: %v", err)
	}
	defer file.Close()

	// Write the first chunk
	if _, err := file.Write(firstMsg.GetData()); err != nil {
		return status.Errorf(codes.Internal, "writing first chunk: %v", err)
	}

	totalSize := firstMsg.GetTotal()
	var bytesReceived int64 = int64(len(firstMsg.GetData()))

	// Receive remaining chunks
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receiving chunk: %v", err)
		}

		// Seek to the correct offset and write
		if _, err := file.Seek(chunk.GetOffset(), 0); err != nil {
			return status.Errorf(codes.Internal, "seeking to offset: %v", err)
		}

		if _, err := file.Write(chunk.GetData()); err != nil {
			return status.Errorf(codes.Internal, "writing chunk: %v", err)
		}

		bytesReceived += int64(len(chunk.GetData()))

		// Report progress periodically
		if totalSize > 0 && bytesReceived%int64(100*1024*1024) < int64(len(chunk.GetData())) {
			progress := int((bytesReceived * 100) / totalSize)
			logger.Debug("disk receive progress", "progress", progress, "bytes", bytesReceived)
		}
	}

	logger.Info("disk receive completed", "bytes_received", bytesReceived)

	return stream.SendAndClose(&nodeagentpb.ReceiveDiskResponse{
		TargetDiskPath: targetPath,
		BytesReceived:  bytesReceived,
		Success:        true,
	})
}

// PrepareMigratedVM creates a VM definition on this node using a transferred disk.
// This is called on the target node after disk transfer is complete.
func (h *grpcHandler) PrepareMigratedVM(ctx context.Context, req *nodeagentpb.PrepareMigratedVMRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetDiskPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "disk_path is required")
	}
	if err := validatePath(req.GetDiskPath(), h.server.config.StoragePath); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid disk_path: %v", err)
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "prepare-migrated-vm")
	logger.Info("preparing migrated VM", "disk_path", req.GetDiskPath())

	// Determine storage backend
	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = string(h.server.storageType)
	}

	// Build domain config from the request
	cfg := &vm.DomainConfig{
		VMID:           req.GetVmId(),
		Hostname:       req.GetHostname(),
		VCPU:           int(req.GetVcpu()),
		MemoryMB:       int(req.GetMemoryMb()),
		StorageBackend: storageBackend,
		DiskPath:       req.GetDiskPath(),
		MACAddress:     req.GetMacAddress(),
		IPv4Address:    req.GetIpv4Address(),
		IPv6Address:    req.GetIpv6Address(),
		PortSpeedKbps:  int(req.GetPortSpeedMbps()) * 1000,
	}

	// For QCOW, the disk is already transferred
	if storageBackend == "qcow" {
		cfg.DiskPath = req.GetDiskPath()
	} else {
		// For Ceph, set the pool info
		cfg.CephPool = req.GetCephPool()
		cfg.CephMonitors = req.GetCephMonitors()
		cfg.CephUser = req.GetCephUser()
		cfg.CephSecretUUID = req.GetCephSecretUuid()
	}

	// Create the VM definition (without cloning template since disk exists)
	result, err := h.server.vmManager.CreateVM(ctx, cfg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "creating VM definition: %v", err)
	}

	logger.Info("migrated VM prepared successfully", "domain_name", result.DomainName)

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

const (
	qcowBackupBasePath = "/var/lib/virtuestack/backups"
)

// createQCOWSnapshotInternal creates a qemu-img internal snapshot for QCOW-backed VMs.
func (h *grpcHandler) createQCOWSnapshotInternal(ctx context.Context, vmID, diskPath, snapshotName string) error {
	logger := h.server.logger.With("vm_id", vmID, "operation", "create-qcow-snapshot")
	logger.Info("creating QCOW snapshot", "disk_path", diskPath, "snapshot_name", snapshotName)

	if h.server.storageType == storage.StorageTypeQCOW {
		if err := h.server.storageBackend.CreateSnapshot(ctx, diskPath, snapshotName); err != nil {
			return fmt.Errorf("creating QCOW snapshot: %w", err)
		}
	} else {
		qcowManager, err := storage.NewQCOWManager(filepath.Dir(diskPath), h.server.logger)
		if err != nil {
			return fmt.Errorf("creating QCOW manager: %w", err)
		}
		if err := qcowManager.CreateSnapshot(ctx, diskPath, snapshotName); err != nil {
			return fmt.Errorf("creating QCOW snapshot: %w", err)
		}
	}

	logger.Info("QCOW snapshot created successfully")
	return nil
}

// deleteQCOWSnapshotInternal deletes a qemu-img internal snapshot for QCOW-backed VMs.
func (h *grpcHandler) deleteQCOWSnapshotInternal(ctx context.Context, vmID, diskPath, snapshotName string) error {
	logger := h.server.logger.With("vm_id", vmID, "operation", "delete-qcow-snapshot")
	logger.Info("deleting QCOW snapshot", "disk_path", diskPath, "snapshot_name", snapshotName)

	if h.server.storageType == storage.StorageTypeQCOW {
		if err := h.server.storageBackend.DeleteSnapshot(ctx, diskPath, snapshotName); err != nil {
			logger.Warn("failed to delete QCOW snapshot", "error", err)
		}
	} else {
		qcowManager, err := storage.NewQCOWManager(filepath.Dir(diskPath), h.server.logger)
		if err != nil {
			return fmt.Errorf("creating QCOW manager: %w", err)
		}
		if err := qcowManager.DeleteSnapshot(ctx, diskPath, snapshotName); err != nil {
			logger.Warn("failed to delete QCOW snapshot", "error", err)
		}
	}

	return nil
}

// createQCOWBackupInternal creates a backup file from a QCOW disk using qemu-img convert.
func (h *grpcHandler) createQCOWBackupInternal(ctx context.Context, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error) {
	logger := h.server.logger.With("vm_id", vmID, "operation", "create-qcow-backup")
	logger.Info("creating QCOW backup",
		"disk_path", diskPath,
		"backup_path", backupPath,
		"snapshot_name", snapshotName,
		"compress", compress)

	backupDir := filepath.Dir(backupPath)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return 0, fmt.Errorf("creating backup directory: %w", err)
	}

	args := []string{"convert", "-f", "qcow2", "-O", "qcow2"}

	if snapshotName != "" {
		args = append(args, "-l", snapshotName)
	}

	if compress {
		args = append(args, "-c")
	}

	args = append(args, diskPath, backupPath)

	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("creating QCOW backup: %w, output: %s", err, string(output))
	}

	fileInfo, err := os.Stat(backupPath)
	if err != nil {
		return 0, fmt.Errorf("getting backup file info: %w", err)
	}

	sizeBytes := fileInfo.Size()

	logger.Info("QCOW backup created successfully", "size_bytes", sizeBytes)
	return sizeBytes, nil
}

// restoreQCOWBackupInternal restores a VM from a QCOW backup file.
func (h *grpcHandler) restoreQCOWBackupInternal(ctx context.Context, vmID, backupPath, targetPath string) error {
	logger := h.server.logger.With("vm_id", vmID, "operation", "restore-qcow-backup")
	logger.Info("restoring QCOW backup",
		"backup_path", backupPath,
		"target_path", targetPath)

	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}

	args := []string{"convert", "-f", "qcow2", "-O", "qcow2", backupPath, targetPath}

	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restoring QCOW backup: %w, output: %s", err, string(output))
	}

	logger.Info("QCOW backup restored successfully")
	return nil
}

// deleteQCOWBackupFileInternal deletes a QCOW backup file from the backup storage.
func (h *grpcHandler) deleteQCOWBackupFileInternal(ctx context.Context, backupPath string) error {
	logger := h.server.logger.With("operation", "delete-qcow-backup-file")
	logger.Info("deleting QCOW backup file", "backup_path", backupPath)

	if err := os.Remove(backupPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("deleting QCOW backup file: %w", err)
		}
		logger.Warn("backup file already deleted", "error", err)
	}

	logger.Info("QCOW backup file deleted successfully")
	return nil
}

// getQCOWDiskInfoInternal returns information about a QCOW disk including size.
func (h *grpcHandler) getQCOWDiskInfoInternal(ctx context.Context, diskPath string) (map[string]interface{}, error) {
	logger := h.server.logger.With("operation", "get-qcow-disk-info")
	logger.Debug("getting QCOW disk info", "disk_path", diskPath)

	args := []string{"info", "--output=json", diskPath}

	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting QCOW disk info: %w", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("parsing QCOW disk info: %w", err)
	}

	return info, nil
}
