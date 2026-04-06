package nodeagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeSnapshotStorageError struct {
	code    string
	message string
}

func (e fakeSnapshotStorageError) Error() string {
	return e.message
}

func (e fakeSnapshotStorageError) ErrorCode() string {
	return e.code
}

func TestMapSnapshotStorageErrorReturnsNotFoundForMissingSnapshots(t *testing.T) {
	err := mapSnapshotStorageError("rolling back disk to snapshot", fakeSnapshotStorageError{
		code:    "NOT_FOUND",
		message: "snapshot snap-missing for image vs-test-vm-123-disk0",
	})

	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Equal(t, "snapshot not found", st.Message())
}

func TestMapSnapshotStorageErrorReturnsInvalidArgumentForInvalidSnapshotIdentifier(t *testing.T) {
	err := mapSnapshotStorageError("rolling back disk to snapshot", fakeSnapshotStorageError{
		code:    "INVALID_ARGUMENT",
		message: "invalid LVM identifier",
	})

	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "snapshot_id is invalid", st.Message())
}

func TestMapSnapshotStorageErrorReturnsInternalForUnexpectedStorageErrors(t *testing.T) {
	err := mapSnapshotStorageError("rolling back disk to snapshot", fakeSnapshotStorageError{
		code:    "INTERNAL_ERROR",
		message: "INTERNAL_ERROR: lvmlockd unavailable",
	})

	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "rolling back disk to snapshot: INTERNAL_ERROR: lvmlockd unavailable", st.Message())
}
