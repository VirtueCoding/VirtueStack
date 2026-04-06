package nodeagent

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapDeleteDiskSnapshotError(storageBackend string, err error) error {
	if err == nil {
		return status.Errorf(codes.InvalidArgument, "unsupported storage backend: %s", storageBackend)
	}

	return mapSnapshotStorageError(fmt.Sprintf("deleting %s snapshot", storageBackend), err)
}
