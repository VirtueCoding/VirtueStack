// Package nodeagent provides gRPC handlers for backup operations.
// This file contains handlers for LVM backup creation and related operations.
package nodeagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CreateLVMBackup creates a sparse backup file from an LVM thin snapshot.
// It uses dd with conv=sparse to efficiently copy the snapshot to a backup file,
// skipping zero blocks to minimize backup size.
// After the backup is created, the snapshot is automatically removed.
func (h *grpcHandler) CreateLVMBackup(ctx context.Context, req *nodeagentpb.CreateLVMBackupRequest) (*nodeagentpb.CreateLVMBackupResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetSnapshotName() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot_name is required")
	}
	if req.GetBackupFilePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "backup_file_path is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "create-lvm-backup")
	logger.Info("creating LVM backup from snapshot",
		"snapshot_name", req.GetSnapshotName(),
		"backup_file", req.GetBackupFilePath())

	// Validate backup file path to prevent traversal attacks
	backupDir := filepath.Dir(req.GetBackupFilePath())
	if err := validatePath(req.GetBackupFilePath(), h.server.config.StoragePath); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid backup_file_path: %v", err)
	}

	// Get the LVM manager
	lvmMgr, ok := h.server.storageBackend.(*storage.LVMManager)
	if !ok {
		return nil, status.Error(codes.Internal, "LVM storage backend not available")
	}

	// Get the snapshot device path
	snapPath := lvmMgr.DiskIdentifier(req.GetVmId())
	// The snapshot name is the LV name, need to construct the full path
	vgName := lvmMgr.VolumeGroup()
	snapDevicePath := fmt.Sprintf("/dev/%s/%s", vgName, req.GetSnapshotName())

	// Verify the snapshot exists
	snapExists, err := lvmMgr.ImageExists(ctx, req.GetSnapshotName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "checking if snapshot exists: %v", err)
	}
	if !snapExists {
		return nil, status.Errorf(codes.NotFound, "snapshot %s does not exist", req.GetSnapshotName())
	}

	// Create the backup directory if it doesn't exist
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, status.Errorf(codes.Internal, "creating backup directory: %v", err)
	}

	// Use dd with conv=sparse to create the backup
	// dd if=/dev/{vg}/{snapName} of={backupFilePath} bs=4M conv=sparse
	ddCmd := exec.CommandContext(ctx, "dd",
		"if="+snapDevicePath,
		"of="+req.GetBackupFilePath(),
		"bs=4M",
		"conv=sparse")

	logger.Info("executing dd command for backup creation",
		"source", snapDevicePath,
		"target", req.GetBackupFilePath())

	output, err := ddCmd.CombinedOutput()
	if err != nil {
		logger.Error("dd command failed", "error", err, "output", string(output))
		return nil, status.Errorf(codes.Internal, "creating backup file: %v (output: %s)", err, string(output))
	}

	// Get the backup file size
	fileInfo, err := os.Stat(req.GetBackupFilePath())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting backup file info: %v", err)
	}
	sizeBytes := fileInfo.Size()

	logger.Info("backup file created successfully",
		"backup_file", req.GetBackupFilePath(),
		"size_bytes", sizeBytes)

	// Clean up the snapshot after successful backup
	if err := lvmMgr.DeleteSnapshot(ctx, snapPath, req.GetSnapshotName()); err != nil {
		logger.Warn("failed to delete snapshot after backup, backup still valid",
			"snapshot", req.GetSnapshotName(),
			"error", err)
		// Don't fail the operation - the backup is valid
	} else {
		logger.Info("snapshot cleaned up after backup", "snapshot", req.GetSnapshotName())
	}

	return &nodeagentpb.CreateLVMBackupResponse{
		VmId:      req.GetVmId(),
		Success:   true,
		SizeBytes: sizeBytes,
	}, nil
}

// RestoreLVMBackup restores an LVM thin LV from a backup file.
// The thin LV must already exist; this uses dd to overwrite it in-place.
// The VM must be stopped before calling this method.
func (h *grpcHandler) RestoreLVMBackup(ctx context.Context, req *nodeagentpb.RestoreLVMBackupRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetBackupFilePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "backup_file_path is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "restore-lvm-backup")
	logger.Info("restoring LVM backup", "backup_file", req.GetBackupFilePath())

	// Validate backup file path to prevent traversal attacks
	if err := validatePath(req.GetBackupFilePath(), h.server.config.StoragePath); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid backup_file_path: %v", err)
	}

	// Verify the VM is stopped
	vmStatus, err := h.server.vmManager.GetStatus(ctx, req.GetVmId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting VM status: %v", err)
	}
	if vmStatus.Status == "running" {
		return nil, status.Error(codes.FailedPrecondition, "VM must be stopped before restoring backup")
	}

	// Get the LVM manager
	lvmMgr, ok := h.server.storageBackend.(*storage.LVMManager)
	if !ok {
		return nil, status.Error(codes.Internal, "LVM storage backend not available")
	}

	// Get the disk identifier for this VM
	diskID := h.server.storageBackend.DiskIdentifier(req.GetVmId())

	// Verify the thin LV exists
	exists, err := lvmMgr.ImageExists(ctx, diskID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "checking if LV exists: %v", err)
	}
	if !exists {
		return nil, status.Errorf(codes.NotFound, "LV %s does not exist", diskID)
	}

	// Verify the backup file exists
	if _, err := os.Stat(req.GetBackupFilePath()); os.IsNotExist(err) {
		return nil, status.Errorf(codes.NotFound, "backup file %s does not exist", req.GetBackupFilePath())
	}

	// Use dd to restore the backup to the LV
	// dd if={backupFilePath} of=/dev/{vg}/{diskLV} bs=4M
	ddCmd := exec.CommandContext(ctx, "dd",
		"if="+req.GetBackupFilePath(),
		"of="+diskID,
		"bs=4M")

	logger.Info("executing dd command for backup restoration",
		"source", req.GetBackupFilePath(),
		"target", diskID)

	output, err := ddCmd.CombinedOutput()
	if err != nil {
		logger.Error("dd command failed", "error", err, "output", string(output))
		return nil, status.Errorf(codes.Internal, "restoring backup: %v (output: %s)", err, string(output))
	}

	logger.Info("backup restored successfully", "vm_id", req.GetVmId())

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}
