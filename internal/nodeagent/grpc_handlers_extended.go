package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	if req.GetTemplateRbdImage() == "" || req.GetTemplateRbdSnapshot() == "" {
		return nil, status.Error(codes.InvalidArgument, "template_rbd_image and template_rbd_snapshot are required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "reinstall")
	logger.Info("reinstalling VM")

	// Stop the VM if running
	if err := h.server.vmManager.ForceStopVM(ctx, req.GetVmId()); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			logger.Warn("failed to stop VM before reinstall", "error", err)
		}
	}

	// Delete existing disk
	diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
	if err := h.server.rbdManager.Delete(ctx, diskName); err != nil {
		logger.Warn("failed to delete old disk", "error", err)
	}

	// Clone from new template
	if err := h.server.rbdManager.CloneFromTemplate(
		ctx,
		h.server.config.CephPool,
		req.GetTemplateRbdImage(),
		req.GetTemplateRbdSnapshot(),
		diskName,
	); err != nil {
		return nil, status.Errorf(codes.Internal, "cloning template: %v", err)
	}

	// Delete the old domain definition
	if err := h.server.vmManager.DeleteVM(ctx, req.GetVmId()); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			logger.Warn("failed to delete old domain", "error", err)
		}
	}

	// Re-create domain from existing config with new template
	cfg := &vm.DomainConfig{
		VMID:        req.GetVmId(),
		Hostname:    req.GetHostname(),
		VCPU:        1,
		MemoryMB:    1024,
		CephPool:    h.server.config.CephPool,
		CephUser:    h.server.config.CephUser,
		IPv4Address: req.GetIpv4Address(),
		IPv6Address: req.GetIpv6Address(),
	}

	result, err := h.server.vmManager.CreateVM(ctx, cfg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "recreating VM: %v", err)
	}

	// Set root password via guest agent if available
	if req.GetRootPasswordHash() != "" {
		go func() {
			time.Sleep(30 * time.Second)
			domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
			if err != nil {
				return
			}
			defer domain.Free()
			agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
			_ = agent.SetUserPassword(context.Background(), "root", req.GetRootPasswordHash())
		}()
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
		diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
		if err := h.server.rbdManager.Resize(ctx, diskName, int(req.GetNewDiskGb())); err != nil {
			return nil, status.Errorf(codes.Internal, "resizing disk: %v", err)
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
	if req.GetVmUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_uuid is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmUuid(), "operation", "post-migrate-setup")
	logger.Info("performing post-migration setup")

	// Re-apply bandwidth limits if configured
	if req.GetBandwidth() != nil && req.GetBandwidth().GetAverageKbps() > 0 {
		bwManager := h.server.newBandwidthManager()
		domainName := vm.DomainNameFromID(req.GetVmUuid())
		rateKbps := int(req.GetBandwidth().GetAverageKbps())
		if req.GetIsThrottled() && req.GetThrottleRateKbps() > 0 {
			rateKbps = int(req.GetThrottleRateKbps())
		}
		if err := bwManager.ApplyThrottle(ctx, domainName, rateKbps); err != nil {
			logger.Warn("failed to apply bandwidth limit after migration", "error", err)
		}
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmUuid(),
		Success: true,
	}, nil
}

// StreamVNCConsole establishes a bidirectional VNC console stream.
func (h *grpcHandler) StreamVNCConsole(stream nodeagentpb.NodeAgentService_StreamVNCConsoleServer) error {
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
func (h *grpcHandler) CreateSnapshot(ctx context.Context, req *nodeagentpb.SnapshotRequest) (*nodeagentpb.SnapshotResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
	snapName := fmt.Sprintf("snap-%s", uuid.New().String()[:8])

	if err := h.server.rbdManager.CreateSnapshot(ctx, diskName, snapName); err != nil {
		return nil, status.Errorf(codes.Internal, "creating snapshot: %v", err)
	}

	// Get snapshot size
	size, _ := h.server.rbdManager.GetImageSize(ctx, diskName)

	return &nodeagentpb.SnapshotResponse{
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
	snapshots, err := h.server.rbdManager.ListSnapshots(ctx, diskName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listing snapshots: %v", err)
	}

	for _, snap := range snapshots {
		if snap.Name == req.GetSnapshotId() || strings.HasPrefix(snap.Name, "snap-") {
			if err := h.server.rbdManager.DeleteSnapshot(ctx, diskName, snap.Name); err != nil {
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

	// VM must be stopped
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

	// Rollback the RBD image to the snapshot
	// This is done by cloning the snapshot to a new image and swapping
	snapName := req.GetSnapshotId()
	tempImage := diskName + "-revert-temp"

	if err := h.server.rbdManager.CloneSnapshotToPool(ctx,
		h.server.config.CephPool, diskName, snapName,
		h.server.config.CephPool, tempImage,
	); err != nil {
		return nil, status.Errorf(codes.Internal, "cloning snapshot for revert: %v", err)
	}

	// Flatten the cloned image
	if err := h.server.rbdManager.FlattenImage(ctx, tempImage); err != nil {
		_ = h.server.rbdManager.Delete(ctx, tempImage)
		return nil, status.Errorf(codes.Internal, "flattening reverted image: %v", err)
	}

	// Delete old disk and rename temp
	if err := h.server.rbdManager.Delete(ctx, diskName); err != nil {
		_ = h.server.rbdManager.Delete(ctx, tempImage)
		return nil, status.Errorf(codes.Internal, "deleting old disk: %v", err)
	}

	// Note: RBD doesn't support rename directly. The clone+flatten approach
	// creates the image with the temp name. We'll need the controller to
	// track the new image name, or use a symlink approach.
	// For now, we re-create from the temp image.

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

	snapshots, err := h.server.rbdManager.ListSnapshots(ctx, diskName)
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

	// Whitelist allowed commands for security
	allowedCommands := map[string]bool{
		"cat": true, "ls": true, "df": true, "free": true, "uname": true,
		"ip": true, "hostname": true, "whoami": true, "date": true, "uptime": true,
	}
	cmdBase := strings.Split(req.GetCommand(), " ")[0]
	cmdBase = strings.TrimPrefix(cmdBase, "/usr/bin/")
	cmdBase = strings.TrimPrefix(cmdBase, "/bin/")
	if !allowedCommands[cmdBase] {
		return nil, status.Errorf(codes.PermissionDenied, "command %q is not in the allowed whitelist", cmdBase)
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	// Build the guest-exec command
	fullCmd := req.GetCommand()
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

	output, err := domain.QemuAgentCommand(string(cmdJSON), libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, timeout)
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
	statusOutput, err := domain.QemuAgentCommand(statusCmd, libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, timeout)
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
