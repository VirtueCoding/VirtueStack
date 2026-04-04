// Package nodeagent provides gRPC handlers for snapshot operations.
// This file contains handlers for VM disk snapshot create, delete, revert, and list operations.
package nodeagent

import (
	"context"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/snapshotutil"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateSnapshot creates a point-in-time snapshot of a VM's disk.
func (h *grpcHandler) CreateSnapshot(ctx context.Context, req *nodeagentpb.SnapshotRequest) (*nodeagentpb.Snapshot, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	diskID := h.server.storageBackend.DiskIdentifier(req.GetVmId())
	snapName := fmt.Sprintf("snap-%s", uuid.New().String()[:8])

	if err := h.server.storageBackend.CreateSnapshot(ctx, diskID, snapName); err != nil {
		return nil, status.Errorf(codes.Internal, "creating snapshot: %v", err)
	}

	// Get snapshot size
	size, _ := h.server.storageBackend.GetImageSize(ctx, diskID)

	return snapshotutil.NewSnapshotResponse(req.GetVmId(), req.GetName(), snapName, size), nil
}

// DeleteSnapshot removes a previously created disk snapshot.
func (h *grpcHandler) DeleteSnapshot(ctx context.Context, req *nodeagentpb.SnapshotIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" || req.GetSnapshotId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id and snapshot_id are required")
	}

	diskID := h.server.storageBackend.DiskIdentifier(req.GetVmId())

	// List snapshots to find by ID (use snapshot_id as the rbd snap name)
	snapshots, err := h.server.storageBackend.ListSnapshots(ctx, diskID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listing snapshots: %v", err)
	}

	for _, snap := range snapshots {
		if snap.Name == req.GetSnapshotId() {
			if err := h.server.storageBackend.DeleteSnapshot(ctx, diskID, snap.Name); err != nil {
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

	diskID := h.server.storageBackend.DiskIdentifier(req.GetVmId())

	if err := h.server.storageBackend.Rollback(ctx, diskID, req.GetSnapshotId()); err != nil {
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

	diskID := h.server.storageBackend.DiskIdentifier(req.GetVmId())

	snapshots, err := h.server.storageBackend.ListSnapshots(ctx, diskID)
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
			CreatedAt:       timestamppb.New(snap.CreatedAt),
		})
	}

	return &nodeagentpb.SnapshotListResponse{
		Snapshots: protoSnaps,
	}, nil
}
