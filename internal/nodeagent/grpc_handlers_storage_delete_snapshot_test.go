package nodeagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapDeleteDiskSnapshotErrorReturnsInvalidArgumentForUnsupportedBackend(t *testing.T) {
	err := mapDeleteDiskSnapshotError("zfs", nil)

	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "unsupported storage backend: zfs", st.Message())
}

func TestMapDeleteDiskSnapshotErrorReturnsInternalForBackendDeleteFailures(t *testing.T) {
	err := mapDeleteDiskSnapshotError("qcow", fakeSnapshotStorageError{
		code:    "INTERNAL_ERROR",
		message: "INTERNAL_ERROR: qemu-img snapshot delete failed",
	})

	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "deleting qcow snapshot: INTERNAL_ERROR: qemu-img snapshot delete failed", st.Message())
}
