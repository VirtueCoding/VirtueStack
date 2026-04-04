package snapshotutil

import "testing"

func TestNewSnapshotResponseUsesBackendSnapshotHandleAsSnapshotID(t *testing.T) {
	t.Parallel()

	const (
		vmID          = "vm-123"
		snapshotName  = "Nightly Snapshot"
		backendHandle = "snap-abc12345"
		sizeBytes     = int64(4096)
	)

	snapshot := NewSnapshotResponse(vmID, snapshotName, backendHandle, sizeBytes)

	if snapshot.GetSnapshotId() != backendHandle {
		t.Fatalf("SnapshotId = %q, want %q", snapshot.GetSnapshotId(), backendHandle)
	}
	if snapshot.GetRbdSnapshotName() != backendHandle {
		t.Fatalf("RbdSnapshotName = %q, want %q", snapshot.GetRbdSnapshotName(), backendHandle)
	}
	if snapshot.GetVmId() != vmID {
		t.Fatalf("VmId = %q, want %q", snapshot.GetVmId(), vmID)
	}
	if snapshot.GetName() != snapshotName {
		t.Fatalf("Name = %q, want %q", snapshot.GetName(), snapshotName)
	}
	if snapshot.GetSizeBytes() != sizeBytes {
		t.Fatalf("SizeBytes = %d, want %d", snapshot.GetSizeBytes(), sizeBytes)
	}
	if snapshot.GetCreatedAt() == nil {
		t.Fatal("CreatedAt must be set")
	}
}
