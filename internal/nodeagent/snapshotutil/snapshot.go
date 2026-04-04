package snapshotutil

import (
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NewSnapshotResponse returns a node-agent snapshot payload whose identifier
// round-trips through create/list/delete/revert as the backend snapshot handle.
func NewSnapshotResponse(vmID, snapshotName, backendHandle string, sizeBytes int64) *nodeagentpb.Snapshot {
	return &nodeagentpb.Snapshot{
		SnapshotId:      backendHandle,
		VmId:            vmID,
		Name:            snapshotName,
		RbdSnapshotName: backendHandle,
		SizeBytes:       sizeBytes,
		CreatedAt:       timestamppb.Now(),
	}
}
