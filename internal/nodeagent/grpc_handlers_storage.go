// Package nodeagent provides gRPC handlers for storage operations.
// This file contains handlers for disk snapshot management, disk transfer,
// and migration preparation operations.
package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/transferutil"
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
			diskPath = transferutil.ResolveQCOWVMDiskPath(h.server.config.StoragePath, req.GetVmId())
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
			diskPath = transferutil.ResolveQCOWVMDiskPath(h.server.config.StoragePath, req.GetVmId())
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
// This is used for QCOW and LVM migrations where disk files need to be copied between nodes.
func (h *grpcHandler) TransferDisk(req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer) error {
	if req.GetSourceDiskPath() == "" {
		return status.Error(codes.InvalidArgument, "source_disk_path is required")
	}
	if req.GetTargetNodeAddress() == "" {
		return status.Error(codes.InvalidArgument, "target_node_address is required")
	}

	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = string(h.server.storageType)
	}

	logger := h.server.logger.With("operation", "transfer-disk", "storage_backend", storageBackend)
	logger.Info("initiating disk transfer",
		"source_disk", req.GetSourceDiskPath(),
		"target_node", req.GetTargetNodeAddress(),
		"target_disk", req.GetTargetDiskPath(),
		"disk_size_gb", req.GetDiskSizeGb())

	ctx := stream.Context()

	// Handle LVM source
	if storageBackend == "lvm" {
		return h.transferLVMDisk(ctx, req, stream, logger)
	}

	// Handle QCOW source (default)
	sourcePath, err := transferutil.ResolveQCOWSourcePath(req.GetSourceDiskPath(), h.server.config.StoragePath)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid source_disk_path: %v", err)
	}

	// Get disk info
	snapshotName := req.GetSnapshotName()
	if err := transferutil.ValidateSnapshotName(snapshotName); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid snapshot_name: %v", err)
	}

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

	// Send metadata in first chunk
	chunk := &nodeagentpb.DiskChunk{
		Data:                 nil,
		Offset:               0,
		Total:                totalSize,
		TargetDiskPath:       req.GetTargetDiskPath(),
		StorageBackend:       storageBackend,
		DiskSizeGb:           req.GetDiskSizeGb(),
		TargetLvmVolumeGroup: req.GetTargetLvmVolumeGroup(),
		TargetLvmThinPool:    req.GetTargetLvmThinPool(),
	}

	if err := stream.Send(chunk); err != nil {
		return status.Errorf(codes.Internal, "sending metadata chunk: %v", err)
	}

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

	if err := transferutil.ValidateTransferredBytes(totalSize, bytesSent); err != nil {
		return status.Errorf(codes.DataLoss, "disk transfer size mismatch: %v", err)
	}

	logger.Info("disk transfer completed", "bytes_sent", bytesSent)
	return nil
}

// transferLVMDisk handles LVM disk transfer using dd to read from block device.
func (h *grpcHandler) transferLVMDisk(ctx context.Context, req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer, logger *slog.Logger) error {
	sourcePath, err := transferutil.ResolveLVMSourcePath(
		req.GetSourceDiskPath(),
		req.GetSnapshotName(),
		req.GetSourceLvmVolumeGroup(),
		h.server.config.LVMVolumeGroup,
	)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid lvm source path: %v", err)
	}

	logger = logger.With("source_path", sourcePath)

	// Get the size of the LV
	// Use blockdev to get size in bytes
	sizeCmd := exec.CommandContext(ctx, "blockdev", "--getsize64", sourcePath)
	sizeOutput, err := sizeCmd.Output()
	if err != nil {
		return status.Errorf(codes.Internal, "getting LV size: %v", err)
	}
	totalSize, err := parseBytes(string(sizeOutput))
	if err != nil {
		return status.Errorf(codes.Internal, "parsing LV size: %v", err)
	}

	logger.Info("transferring LVM disk", "total_size", totalSize)

	// Send metadata in first chunk
	chunk := &nodeagentpb.DiskChunk{
		Data:                 nil,
		Offset:               0,
		Total:                totalSize,
		TargetDiskPath:       req.GetTargetDiskPath(),
		StorageBackend:       "lvm",
		DiskSizeGb:           req.GetDiskSizeGb(),
		TargetLvmVolumeGroup: req.GetTargetLvmVolumeGroup(),
		TargetLvmThinPool:    req.GetTargetLvmThinPool(),
	}

	if err := stream.Send(chunk); err != nil {
		return status.Errorf(codes.Internal, "sending metadata chunk: %v", err)
	}

	// Use dd to read from the LV and pipe to our stream
	// dd if=/dev/{vg}/{disk} bs=4M conv=sparse
	ddCmd := exec.CommandContext(ctx, "dd",
		"if="+sourcePath,
		"bs=4M",
		"conv=sparse")

	stdout, err := ddCmd.StdoutPipe()
	if err != nil {
		return status.Errorf(codes.Internal, "creating dd pipe: %v", err)
	}

	if err := ddCmd.Start(); err != nil {
		return status.Errorf(codes.Internal, "starting dd: %v", err)
	}

	// Stream the output in chunks
	bytesSent, err := transferutil.StreamProcessOutput(
		stdout,
		totalSize,
		func(offset, total int64, data []byte) error {
			return stream.Send(&nodeagentpb.DiskChunk{
				Data:   data,
				Offset: offset,
				Total:  total,
			})
		},
		func() error {
			if ddCmd.Process == nil {
				return nil
			}
			return ddCmd.Process.Kill()
		},
		ddCmd.Wait,
	)
	if err != nil {
		if errors.Is(err, transferutil.ErrSendProcess) {
			return status.Errorf(codes.Internal, "sending disk chunk: %v", err)
		}
		if errors.Is(err, transferutil.ErrReadProcess) {
			return status.Errorf(codes.Internal, "reading dd output: %v", err)
		}
		if errors.Is(err, transferutil.ErrWaitProcess) {
			return status.Errorf(codes.Internal, "waiting for dd to finish: %v", err)
		}
		return status.Errorf(codes.Internal, "streaming dd output: %v", err)
	}
	if err := transferutil.ValidateTransferredBytes(totalSize, bytesSent); err != nil {
		return status.Errorf(codes.DataLoss, "LVM transfer size mismatch: %v", err)
	}

	logger.Info("LVM disk transfer completed", "bytes_sent", bytesSent)
	return nil
}

// parseBytes parses a byte size string (with optional newline) into int64.
func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
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
	receiveTarget, err := transferutil.ResolveReceiveTarget(
		firstMsg.GetStorageBackend(),
		targetPath,
		h.server.config.StoragePath,
		h.server.config.LVMVolumeGroup,
		h.server.config.LVMThinPool,
		firstMsg.GetTargetLvmVolumeGroup(),
		firstMsg.GetTargetLvmThinPool(),
	)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid target_disk_path: %v", err)
	}

	logger := h.server.logger.With("operation", "receive-disk", "target_path", receiveTarget.OpenPath)
	logger.Info("receiving disk transfer")

	totalSize := firstMsg.GetTotal()
	tracker, err := transferutil.NewReceiveTracker(totalSize)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid transfer total: %v", err)
	}
	if err := tracker.Accept(firstMsg.GetOffset(), len(firstMsg.GetData())); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid initial chunk: %v", err)
	}
	file, commit, rollback, err := h.openReceiveTarget(stream.Context(), firstMsg, receiveTarget)
	if err != nil {
		return err
	}
	success := false
	closeTarget := func() error {
		if file == nil {
			return nil
		}
		if err := file.Close(); err != nil {
			return err
		}
		file = nil
		return nil
	}
	defer func() {
		if err := closeTarget(); err != nil {
			logger.Debug("failed to close target disk file", "error", err)
		}
		if success || rollback == nil {
			return
		}
		if err := rollback(); err != nil {
			logger.Warn("failed to clean up receive target", "error", err)
		}
	}()

	// Write the first chunk
	if err := transferutil.WriteFull(file, firstMsg.GetData()); err != nil {
		return status.Errorf(codes.Internal, "writing first chunk: %v", err)
	}

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
		if chunk.GetTotal() != 0 && chunk.GetTotal() != totalSize {
			return status.Errorf(codes.InvalidArgument, "chunk total %d does not match initial total %d", chunk.GetTotal(), totalSize)
		}
		if err := tracker.Accept(chunk.GetOffset(), len(chunk.GetData())); err != nil {
			if errors.Is(err, transferutil.ErrInvalidOffset) {
				return status.Errorf(codes.InvalidArgument, "invalid chunk offset: %v", err)
			}
			return status.Errorf(codes.DataLoss, "invalid chunk size: %v", err)
		}

		if err := transferutil.SeekAndWriteFull(file, chunk.GetOffset(), chunk.GetData()); err != nil {
			if errors.Is(err, io.ErrShortWrite) {
				return status.Errorf(codes.Internal, "writing chunk: %v", err)
			}
			if errors.Is(err, os.ErrInvalid) {
				return status.Errorf(codes.Internal, "seeking to offset: %v", err)
			}
			return status.Errorf(codes.Internal, "writing chunk: %v", err)
		}

		bytesReceived += int64(len(chunk.GetData()))

		// Report progress periodically
		if totalSize > 0 && bytesReceived%int64(100*1024*1024) < int64(len(chunk.GetData())) {
			progress := int((bytesReceived * 100) / totalSize)
			logger.Debug("disk receive progress", "progress", progress, "bytes", bytesReceived)
		}
	}

	if err := tracker.Finalize(); err != nil {
		return status.Errorf(codes.DataLoss, "incomplete disk transfer: %v", err)
	}
	if err := closeTarget(); err != nil {
		return status.Errorf(codes.Internal, "closing target disk file: %v", err)
	}
	if commit != nil {
		if err := commit(); err != nil {
			return status.Errorf(codes.Internal, "committing target disk file: %v", err)
		}
	}

	logger.Info("disk receive completed", "bytes_received", bytesReceived)
	success = true

	return stream.SendAndClose(&nodeagentpb.ReceiveDiskResponse{
		TargetDiskPath: receiveTarget.OpenPath,
		BytesReceived:  bytesReceived,
		Success:        true,
	})
}

func (h *grpcHandler) openReceiveTarget(ctx context.Context, firstMsg *nodeagentpb.DiskChunk, target transferutil.ReceiveTarget) (*os.File, func() error, func() error, error) {
	if firstMsg.GetStorageBackend() == "lvm" {
		if target.CreateImageID == "" {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "target LVM identifier is required")
		}
		if firstMsg.GetDiskSizeGb() <= 0 {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "disk_size_gb is required for lvm transfers")
		}
		if err := transferutil.ValidateLVMImageCapacity(firstMsg.GetTotal(), int64(firstMsg.GetDiskSizeGb())); err != nil {
			return nil, nil, nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		file, rollback, err := transferutil.OpenLVMReceiveTarget(
			ctx,
			target.CreateImageID,
			int(firstMsg.GetDiskSizeGb()),
			target.OpenPath,
			h.server.storageBackend.CreateImage,
			func(path string) (*os.File, error) {
				return os.OpenFile(path, os.O_WRONLY, 0)
			},
			func(ctx context.Context, imageID string) error {
				return h.server.storageBackend.Delete(ctx, imageID)
			},
		)
		if err != nil {
			if errors.Is(err, transferutil.ErrCreateImage) {
				return nil, nil, nil, status.Errorf(codes.Internal, "creating target LVM image: %v", err)
			}
			return nil, nil, nil, status.Errorf(codes.Internal, "opening target LVM device: %v", err)
		}
		return file, func() error { return nil }, rollback, nil
	}

	file, commit, rollback, err := transferutil.OpenFileReceiveTarget(target.OpenPath)
	if err != nil {
		return nil, nil, nil, status.Errorf(codes.Internal, "opening target file: %v", err)
	}
	return file, commit, rollback, nil
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

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "prepare-migrated-vm")
	logger.Info("preparing migrated VM", "disk_path", req.GetDiskPath())

	// Determine storage backend
	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = string(h.server.storageType)
	}

	resolvedDisk, err := transferutil.ResolvePreparedVMDisk(
		storageBackend,
		req.GetDiskPath(),
		h.server.config.StoragePath,
		h.server.config.LVMVolumeGroup,
	)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid disk_path: %v", err)
	}

	// Build domain config from the request
	cfg := &vm.DomainConfig{
		VMID:           req.GetVmId(),
		Hostname:       req.GetHostname(),
		VCPU:           int(req.GetVcpu()),
		MemoryMB:       int(req.GetMemoryMb()),
		StorageBackend: storageBackend,
		DiskPath:       resolvedDisk.DiskPath,
		LVMDiskPath:    resolvedDisk.LVMDiskPath,
		MACAddress:     req.GetMacAddress(),
		IPv4Address:    req.GetIpv4Address(),
		IPv6Address:    req.GetIpv6Address(),
		PortSpeedKbps:  int(req.GetPortSpeedMbps()) * 1000,
	}

	// For QCOW, the disk is already transferred
	if storageBackend == "qcow" {
		cfg.DiskPath = resolvedDisk.DiskPath
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
