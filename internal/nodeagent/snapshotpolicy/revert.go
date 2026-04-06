// Package snapshotpolicy contains pure helpers for snapshot handler policy checks.
package snapshotpolicy

import (
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// AllowsRevert reports whether a VM snapshot revert may proceed for the given VM status.
func AllowsRevert(status string) bool {
	return status == "stopped"
}

// RevertSnapshot enforces the revert precondition and invokes rollback only for stopped VMs.
func RevertSnapshot(vmID, vmStatus string, rollback func() error) (*nodeagentpb.VMOperationResponse, error) {
	if !AllowsRevert(vmStatus) {
		return nil, grpcstatus.Error(codes.FailedPrecondition, "VM must be stopped before reverting snapshot")
	}
	if err := rollback(); err != nil {
		return nil, err
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    vmID,
		Success: true,
	}, nil
}
