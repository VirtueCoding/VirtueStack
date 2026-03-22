// Package nodeagent provides gRPC handlers for storage operations.
// This file contains handlers for disk snapshot management, disk transfer,
// and migration preparation operations.
package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
		defer func() {
			if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
				logger.Debug("failed to remove temporary export file", "path", tempPath, "error", err)
			}
		}()

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
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("failed to close source disk file", "error", err)
		}
	}()

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
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("failed to close target disk file", "error", err)
		}
	}()

	// Write the first chunk
	if _, err := file.Write(firstMsg.GetData()); err != nil {
		return status.Errorf(codes.Internal, "writing first chunk: %v", err)
	}

	totalSize := firstMsg.GetTotal()
	var bytesReceived int64 = int64(len(firstMsg.GetData()))

	// Receive remaining chunks
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
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
