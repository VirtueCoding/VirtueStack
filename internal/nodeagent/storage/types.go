package storage

import "time"

const (
	// gbToBytes is the number of bytes in one gigabyte.
	gbToBytes = 1024 * 1024 * 1024
	// VMDiskNameFmt is the format for VM disk image names.
	VMDiskNameFmt = "vs-%s-disk0"
)

// SnapshotInfo holds metadata about a storage snapshot.
type SnapshotInfo struct {
	// Name is the snapshot name.
	Name string
	// Size is the snapshot size in bytes.
	Size int64
	// Protected indicates whether the snapshot is protected from deletion.
	Protected bool
	// CreatedAt is the time when the snapshot was created.
	CreatedAt time.Time
}

// PoolStats contains storage pool statistics.
type PoolStats struct {
	// Total is the total capacity of the pool in bytes.
	Total int64
	// Used is the used capacity of the pool in bytes.
	Used int64
	// Free is the available capacity of the pool in bytes.
	Free int64
}
