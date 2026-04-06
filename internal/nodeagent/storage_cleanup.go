package nodeagent

import (
	"context"
	"fmt"
	"path/filepath"
)

type createVMFunc func(ctx context.Context) error
type diskCleanupFunc func(ctx context.Context, diskID string) error

func createVMWithRollbackCleanup(
	ctx context.Context,
	createVM createVMFunc,
	cleanupOwnedStorage func(context.Context) error,
) error {
	if err := createVM(ctx); err != nil {
		if cleanupOwnedStorage == nil {
			return err
		}
		if cleanupErr := cleanupOwnedStorage(ctx); cleanupErr != nil {
			return fmt.Errorf("%w: cleanup owned storage: %v", err, cleanupErr)
		}
		return err
	}

	return nil
}

func ownedPrepareCleanupForBackend(
	storageBackend string,
	requestedDiskID string,
	canonicalDiskID string,
	ownedDiskID string,
	deleteDisk diskCleanupFunc,
) func(context.Context, string) error {
	if requestedDiskID == "" || ownedDiskID == "" || canonicalDiskID == "" || deleteDisk == nil {
		return nil
	}

	switch storageBackend {
	case "qcow", "lvm":
		if normalizeOwnedDiskID(storageBackend, requestedDiskID) != normalizeOwnedDiskID(storageBackend, canonicalDiskID) {
			return nil
		}
		return func(ctx context.Context, _ string) error {
			return deleteDisk(ctx, ownedDiskID)
		}
	default:
		return nil
	}
}

func normalizeOwnedDiskID(storageBackend, diskID string) string {
	if diskID == "" {
		return ""
	}

	switch storageBackend {
	case "qcow", "lvm":
		return filepath.Clean(diskID)
	default:
		return diskID
	}
}
