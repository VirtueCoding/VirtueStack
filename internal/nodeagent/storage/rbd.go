// Package storage provides Ceph RBD operations, cloud-init ISO generation,
// and OS template management for the VirtueStack Node Agent.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
)

// RBDManager handles Ceph RBD operations for VM disk management.
type RBDManager struct {
	conn   *rados.Conn
	pool   string // e.g., "vs-vms"
	logger *slog.Logger
}

// NewRBDManager creates a new RBDManager connected to the Ceph cluster.
// cephConf is the path to the ceph.conf file, cephUser is the Ceph client user
// (e.g., "virtuestack"), and pool is the default RBD pool (e.g., "vs-vms").
func NewRBDManager(cephConf, cephUser, pool string, logger *slog.Logger) (*RBDManager, error) {
	conn, err := rados.NewConnWithUser(cephUser)
	if err != nil {
		return nil, fmt.Errorf("creating rados connection for user %q: %w", cephUser, err)
	}

	if err := conn.ReadConfigFile(cephConf); err != nil {
		conn.Shutdown()
		return nil, fmt.Errorf("reading ceph config %q: %w", cephConf, err)
	}

	if err := conn.Connect(); err != nil {
		conn.Shutdown()
		return nil, fmt.Errorf("connecting to ceph cluster: %w", err)
	}

	logger.Info("connected to ceph cluster", "pool", pool, "user", cephUser)

	return &RBDManager{
		conn:   conn,
		pool:   pool,
		logger: logger.With("component", "rbd-manager"),
	}, nil
}

// Close closes the Ceph connection.
func (m *RBDManager) Close() {
	m.logger.Info("closing ceph connection")
	m.conn.Shutdown()
}

// openIOContext opens an IO context for the specified pool.
func (m *RBDManager) openIOContext(pool string) (*rados.IOContext, error) {
	ioctx, err := m.conn.OpenIOContext(pool)
	if err != nil {
		return nil, fmt.Errorf("opening IO context for pool %q: %w", pool, err)
	}
	return ioctx, nil
}

// CloneFromTemplate clones a template snapshot to create a new VM disk.
// This is a copy-on-write (instant) operation.
// sourcePool/sourceImage@sourceSnap -> targetPool/targetImage
func (m *RBDManager) CloneFromTemplate(ctx context.Context, sourcePool, sourceImage, sourceSnap, targetImage string) error {
	logger := m.logger.With("source", sourcePool+"/"+sourceImage+"@"+sourceSnap, "target", m.pool+"/"+targetImage)
	logger.Info("cloning template to VM disk")

	srcIoctx, err := m.openIOContext(sourcePool)
	if err != nil {
		return fmt.Errorf("cloning template %s/%s@%s: %w", sourcePool, sourceImage, sourceSnap, err)
	}
	defer srcIoctx.Destroy()

	dstIoctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("cloning template %s/%s@%s: %w", sourcePool, sourceImage, sourceSnap, err)
	}
	defer dstIoctx.Destroy()

	opts := rbd.NewRbdImageOptions()
	defer opts.Destroy()

	if err := rbd.CloneImage(srcIoctx, sourceImage, sourceSnap, dstIoctx, targetImage, opts); err != nil {
		return fmt.Errorf("cloning template %s/%s@%s to %s: %w", sourcePool, sourceImage, sourceSnap, targetImage, err)
	}

	logger.Info("template cloned successfully")
	return nil
}

// CloneSnapshotToPool clones a snapshot from one pool to another.
// This is used for backup operations to clone a protected snapshot to the backup pool.
// sourceImage is the VM disk name (without pool prefix), sourceSnap is the snapshot name.
// targetPool is the destination pool (e.g., "vs-backups"), targetImage is the new image name.
func (m *RBDManager) CloneSnapshotToPool(ctx context.Context, sourcePool, sourceImage, sourceSnap, targetPool, targetImage string) error {
	logger := m.logger.With("source", sourcePool+"/"+sourceImage+"@"+sourceSnap, "target", targetPool+"/"+targetImage)
	logger.Info("cloning snapshot to target pool")

	srcIoctx, err := m.openIOContext(sourcePool)
	if err != nil {
		return fmt.Errorf("cloning snapshot %s/%s@%s: %w", sourcePool, sourceImage, sourceSnap, err)
	}
	defer srcIoctx.Destroy()

	dstIoctx, err := m.openIOContext(targetPool)
	if err != nil {
		return fmt.Errorf("cloning snapshot %s/%s@%s: %w", sourcePool, sourceImage, sourceSnap, err)
	}
	defer dstIoctx.Destroy()

	opts := rbd.NewRbdImageOptions()
	defer opts.Destroy()

	if err := rbd.CloneImage(srcIoctx, sourceImage, sourceSnap, dstIoctx, targetImage, opts); err != nil {
		return fmt.Errorf("cloning snapshot %s/%s@%s to %s/%s: %w", sourcePool, sourceImage, sourceSnap, targetPool, targetImage, err)
	}

	logger.Info("snapshot cloned to target pool successfully")
	return nil
}

// Resize resizes an RBD image to the new size in GB.
// Resize can only grow an image, not shrink it.
func (m *RBDManager) Resize(ctx context.Context, imageName string, newSizeGB int) error {
	logger := m.logger.With("image", imageName, "size_gb", newSizeGB)
	logger.Info("resizing RBD image")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("resizing image %s: %w", imageName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for resize: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			logger.Warn("failed to close RBD image after resize", "image", imageName, "error", err)
		}
	}()

	newSizeBytes := uint64(newSizeGB) * gbToBytes
	if err := img.Resize(newSizeBytes); err != nil {
		return fmt.Errorf("resizing image %s to %dGB: %w", imageName, newSizeGB, err)
	}

	logger.Info("image resized successfully")
	return nil
}

// Delete removes an RBD image, removing all snapshots first.
func (m *RBDManager) Delete(ctx context.Context, imageName string) error {
	logger := m.logger.With("image", imageName)
	logger.Info("deleting RBD image")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("deleting image %s: %w", imageName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for deletion: %w", imageName, err)
	}

	if err := m.removeAllSnapshots(img, imageName); err != nil {
		if closeErr := img.Close(); closeErr != nil {
			logger.Warn("failed to close image after snapshot removal failure", "image", imageName, "error", closeErr)
		}
		return fmt.Errorf("deleting image %s: %w", imageName, err)
	}
	if err := img.Close(); err != nil {
		logger.Warn("failed to close image after snapshot removal", "image", imageName, "error", err)
	}

	if err := rbd.RemoveImage(ioctx, imageName); err != nil {
		return fmt.Errorf("removing image %s: %w", imageName, err)
	}

	logger.Info("image deleted successfully")
	return nil
}

// removeAllSnapshots removes all snapshots from an open image.
func (m *RBDManager) removeAllSnapshots(img *rbd.Image, imageName string) error {
	snaps, err := img.GetSnapshotNames()
	if err != nil {
		return fmt.Errorf("listing snapshots of %s: %w", imageName, err)
	}

	for _, snap := range snaps {
		snapObj := img.GetSnapshot(snap.Name)
		protected, err := snapObj.IsProtected()
		if err != nil {
			return fmt.Errorf("checking protection of snapshot %s@%s: %w", imageName, snap.Name, err)
		}
		if protected {
			if err := snapObj.Unprotect(); err != nil {
				return fmt.Errorf("unprotecting snapshot %s@%s: %w", imageName, snap.Name, err)
			}
		}
		if err := snapObj.Remove(); err != nil {
			return fmt.Errorf("removing snapshot %s@%s: %w", imageName, snap.Name, err)
		}
		m.logger.Info("removed snapshot", "image", imageName, "snapshot", snap.Name)
	}
	return nil
}

// CreateSnapshot creates an RBD snapshot of the named image.
func (m *RBDManager) CreateSnapshot(ctx context.Context, imageName, snapName string) error {
	logger := m.logger.With("image", imageName, "snapshot", snapName)
	logger.Info("creating snapshot")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("creating snapshot %s@%s: %w", imageName, snapName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for snapshot: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			logger.Warn("failed to close RBD image after snapshot creation", "image", imageName, "error", err)
		}
	}()

	if _, err := img.CreateSnapshot(snapName); err != nil {
		return fmt.Errorf("creating snapshot %s@%s: %w", imageName, snapName, err)
	}

	logger.Info("snapshot created successfully")
	return nil
}

// DeleteSnapshot deletes an RBD snapshot.
func (m *RBDManager) DeleteSnapshot(ctx context.Context, imageName, snapName string) error {
	logger := m.logger.With("image", imageName, "snapshot", snapName)
	logger.Info("deleting snapshot")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("deleting snapshot %s@%s: %w", imageName, snapName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for snapshot deletion: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			logger.Warn("failed to close RBD image after snapshot deletion", "image", imageName, "error", err)
		}
	}()

	snapObj := img.GetSnapshot(snapName)
	if err := snapObj.Remove(); err != nil {
		return fmt.Errorf("deleting snapshot %s@%s: %w", imageName, snapName, err)
	}

	logger.Info("snapshot deleted successfully")
	return nil
}

// ProtectSnapshot protects a snapshot from deletion, which is required for cloning.
func (m *RBDManager) ProtectSnapshot(ctx context.Context, imageName, snapName string) error {
	logger := m.logger.With("image", imageName, "snapshot", snapName)
	logger.Info("protecting snapshot")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("protecting snapshot %s@%s: %w", imageName, snapName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for snapshot protect: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			logger.Warn("failed to close RBD image after snapshot protect", "image", imageName, "error", err)
		}
	}()

	snapObj := img.GetSnapshot(snapName)
	if err := snapObj.Protect(); err != nil {
		return fmt.Errorf("protecting snapshot %s@%s: %w", imageName, snapName, err)
	}

	logger.Info("snapshot protected successfully")
	return nil
}

// UnprotectSnapshot unprotects a snapshot so it can be deleted.
func (m *RBDManager) UnprotectSnapshot(ctx context.Context, imageName, snapName string) error {
	logger := m.logger.With("image", imageName, "snapshot", snapName)
	logger.Info("unprotecting snapshot")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("unprotecting snapshot %s@%s: %w", imageName, snapName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for snapshot unprotect: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			logger.Warn("failed to close RBD image after snapshot unprotect", "image", imageName, "error", err)
		}
	}()

	snapObj := img.GetSnapshot(snapName)
	if err := snapObj.Unprotect(); err != nil {
		return fmt.Errorf("unprotecting snapshot %s@%s: %w", imageName, snapName, err)
	}

	logger.Info("snapshot unprotected successfully")
	return nil
}

// ListSnapshots lists all snapshots of an RBD image.
func (m *RBDManager) ListSnapshots(ctx context.Context, imageName string) ([]SnapshotInfo, error) {
	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return nil, fmt.Errorf("listing snapshots of %s: %w", imageName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return nil, fmt.Errorf("opening image %s for listing snapshots: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			m.logger.Warn("failed to close RBD image after listing snapshots", "image", imageName, "error", err)
		}
	}()

	snaps, err := img.GetSnapshotNames()
	if err != nil {
		return nil, fmt.Errorf("getting snapshot names for %s: %w", imageName, err)
	}

	result := make([]SnapshotInfo, 0, len(snaps))
	for _, s := range snaps {
		snapObj := img.GetSnapshot(s.Name)
		protected, err := snapObj.IsProtected()
		if err != nil {
			return nil, fmt.Errorf("checking protection of %s@%s: %w", imageName, s.Name, err)
		}
		result = append(result, SnapshotInfo{
			Name:      s.Name,
			Size:      int64(s.Size),
			Protected: protected,
		})
	}

	return result, nil
}

// GetImageSize returns the size of an RBD image in bytes.
func (m *RBDManager) GetImageSize(ctx context.Context, imageName string) (int64, error) {
	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return 0, fmt.Errorf("getting size of image %s: %w", imageName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return 0, fmt.Errorf("opening image %s to get size: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			m.logger.Warn("failed to close RBD image after getting size", "image", imageName, "error", err)
		}
	}()

	size, err := img.GetSize()
	if err != nil {
		return 0, fmt.Errorf("getting size of image %s: %w", imageName, err)
	}

	return int64(size), nil
}

// ImageExists checks if an RBD image exists in the pool.
// Uses rbd.OpenImage for O(1) existence check instead of the O(n) linear
// search over GetImageNames.
func (m *RBDManager) ImageExists(ctx context.Context, imageName string) (bool, error) {
	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return false, fmt.Errorf("checking existence of image %s: %w", imageName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		// rbd returns an error when the image does not exist; treat it as not-found.
		return false, nil
	}
	if closeErr := img.Close(); closeErr != nil {
		m.logger.Warn("failed to close RBD image after existence check", "image", imageName, "error", closeErr)
	}
	return true, nil
}

// FlattenImage flattens a cloned image, removing its dependency on the parent snapshot.
func (m *RBDManager) FlattenImage(ctx context.Context, imageName string) error {
	logger := m.logger.With("image", imageName)
	logger.Info("flattening RBD image")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("flattening image %s: %w", imageName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for flatten: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			logger.Warn("failed to close RBD image after flatten", "image", imageName, "error", err)
		}
	}()

	if err := img.Flatten(); err != nil {
		return fmt.Errorf("flattening image %s: %w", imageName, err)
	}

	logger.Info("image flattened successfully")
	return nil
}

// GetPoolStats returns storage statistics for the configured pool.
// It queries the Ceph cluster for pool usage information.
func (m *RBDManager) GetPoolStats(ctx context.Context) (*PoolStats, error) {
	// Get pool statistics using rados command
	cmd := `{"prefix": "df", "format": "json"}`
	cmdBuf, _, err := m.conn.MonCommand([]byte(cmd))
	if err != nil {
		return nil, fmt.Errorf("getting pool stats: %w", err)
	}

	// Parse the JSON response
	var dfResp struct {
		Pools []struct {
			Name  string `json:"name"`
			Stats struct {
				BytesUsed    int64 `json:"bytes_used"`
				MaxAvailable int64 `json:"max_avail"`
				Stored       int64 `json:"stored"`
			} `json:"stats"`
		} `json:"pools"`
		Stats struct {
			TotalBytes      int64 `json:"total_bytes"`
			TotalUsedBytes  int64 `json:"total_used_bytes"`
			TotalAvailBytes int64 `json:"total_avail_bytes"`
		} `json:"stats"`
	}

	if err := json.Unmarshal(cmdBuf, &dfResp); err != nil {
		return nil, fmt.Errorf("parsing pool stats response: %w", err)
	}

	// Find our pool in the response
	for _, pool := range dfResp.Pools {
		if pool.Name == m.pool {
			return &PoolStats{
				Total: pool.Stats.BytesUsed + pool.Stats.MaxAvailable,
				Used:  pool.Stats.BytesUsed,
				Free:  pool.Stats.MaxAvailable,
			}, nil
		}
	}

	// Pool not found — return an explicit error rather than silently falling back
	// to cluster-wide statistics, which could mask misconfiguration.
	return nil, fmt.Errorf("configured pool %q not found in Ceph cluster df output", m.pool)
}

// Rollback reverts an RBD image to a previous snapshot state in-place.
// This is the Ceph-native way to restore a VM to a snapshot without
// cloning, flattening, or renaming any images.
func (m *RBDManager) Rollback(ctx context.Context, imageName, snapshotName string) error {
	logger := m.logger.With("image", imageName, "snapshot", snapshotName)
	logger.Info("rolling back RBD image to snapshot")

	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return fmt.Errorf("opening IO context for rollback: %w", err)
	}
	defer ioctx.Destroy()

	image := rbd.GetImage(ioctx, imageName)
	if err := image.Open(true); err != nil {
		return fmt.Errorf("opening image %q for rollback: %w", imageName, err)
	}
	defer func() {
		if err := image.Close(); err != nil {
			logger.Warn("failed to close RBD image after rollback", "image", imageName, "error", err)
		}
	}()

	snapshot := image.GetSnapshot(snapshotName)
	if err := snapshot.Rollback(); err != nil {
		return fmt.Errorf("rolling back image %q to snapshot %q: %w", imageName, snapshotName, err)
	}

	logger.Info("rolled back image to snapshot")
	return nil
}

// IsConnected checks if the Ceph connection is still alive.
func (m *RBDManager) IsConnected() bool {
	if m.conn == nil {
		return false
	}
	// Try a simple command to check connection
	_, err := m.conn.GetClusterStats()
	return err == nil
}

// GetStorageType returns the storage backend type.
func (m *RBDManager) GetStorageType() StorageType {
	return StorageTypeCEPH
}

// DiskIdentifier returns the canonical RBD image name for a VM disk.
// The format is "vs-{vmID}-disk0".
func (m *RBDManager) DiskIdentifier(vmID string) string {
	return fmt.Sprintf(VMDiskNameFmt, vmID)
}

// CreateImage is not supported for RBD storage.
// RBD images are created through CloneFromTemplate or other operations.
// This method returns an error indicating the operation is not supported.
func (m *RBDManager) CreateImage(ctx context.Context, imageName string, sizeGB int) error {
	return NewStorageError(ErrCodeInvalid, "RBD does not support CreateImage; use CloneFromTemplate instead", nil)
}

// GetImageInfo returns detailed information about an RBD image.
func (m *RBDManager) GetImageInfo(ctx context.Context, imageName string) (*ImageInfo, error) {
	ioctx, err := m.openIOContext(m.pool)
	if err != nil {
		return nil, fmt.Errorf("opening IO context: %w", err)
	}
	defer ioctx.Destroy()

	// Check if image exists
	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return nil, NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), nil)
	}
	defer func() { _ = img.Close() }()

	virtualSize, err := img.GetSize()
	if err != nil {
		return nil, fmt.Errorf("getting size of image %q: %w", imageName, err)
	}

	return &ImageInfo{
		Filename:         imageName,
		Format:           "rbd",
		VirtualSizeBytes: int64(virtualSize),
		ActualSizeBytes:  0,
		DirtyFlag:        false,
		ClusterSize:      0,
		BackingFile:      "",
	}, nil
}
