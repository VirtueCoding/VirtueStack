// Package storage provides LVM thin-provisioned storage operations for VM disk management.
// This file implements a file-based LVM storage backend using lvcreate, lvremove, etc.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultLVMTimeout is the default timeout for LVM commands.
	defaultLVMTimeout = 5 * time.Minute
)

// validLVMName validates LVM volume group and pool names.
// Names must start with alphanumeric or underscore, and contain only
// alphanumeric, underscore, dot, plus, or minus characters.
var validLVMName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.+-]*$`)

// validLVMLVName validates LVM logical volume names (including VM IDs and snapshot names).
// Allows alphanumeric, underscore, hyphen, and dot characters.
// Must not contain path separators or shell injection characters.
var validLVMLVName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// LVMManager handles LVM thin-provisioned operations for VM disk management.
// It implements StorageBackend for LVM block storage using lvcreate, lvremove, etc.
//
// # Concurrent Operation Safety
//
// LVM operations are serialized at the VG level via lvmlockd (the LVM locking daemon).
// When multiple processes/agents attempt concurrent LVM operations on the same VG,
// lvmlockd ensures only one operation proceeds at a time. This prevents:
//   - Concurrent LV creation/deletion conflicts
//   - Metadata corruption from simultaneous modifications
//   - Race conditions during snapshot operations
//
// The Node Agent's LVMManager runs as a single process with sequential method calls,
// so goroutine safety within the manager is not required. The underlying LVM
// subsystem (via lvmlockd) handles cross-process synchronization.
//
// For cluster scenarios with multiple Node Agents sharing a VG, ensure:
//   - lvmlockd is running and configured for shared VG locking
//   - Each Node Agent uses the same VG configuration
//
// See: https://man7.org/linux/man-pages/man8/lvmlockd.8.html
type LVMManager struct {
	vgName   string
	thinPool string
	logger   *slog.Logger
}

// NewLVMManager creates a new LVMManager for the given volume group and thin pool.
// The vgName is the LVM volume group name (e.g., "vgvs").
// The thinPool is the thin pool LV name within the VG (e.g., "thinpool").
func NewLVMManager(vgName, thinPool string, logger *slog.Logger) (*LVMManager, error) {
	if vgName == "" {
		return nil, NewStorageError(ErrCodeInvalid, "vgName cannot be empty", nil)
	}
	if thinPool == "" {
		return nil, NewStorageError(ErrCodeInvalid, "thinPool cannot be empty", nil)
	}
	if logger == nil {
		return nil, NewStorageError(ErrCodeInvalid, "logger cannot be nil", nil)
	}

	// Validate VG name format
	if !validLVMName.MatchString(vgName) {
		return nil, NewStorageError(ErrCodeInvalid,
			fmt.Sprintf("invalid volume group name %q: must match %s", vgName, validLVMName.String()), nil)
	}

	// Validate thin pool name format
	if !validLVMName.MatchString(thinPool) {
		return nil, NewStorageError(ErrCodeInvalid,
			fmt.Sprintf("invalid thin pool name %q: must match %s", thinPool, validLVMName.String()), nil)
	}

	return &LVMManager{
		vgName:   vgName,
		thinPool: thinPool,
		logger:   logger.With("component", "lvm-manager", "vg", vgName, "pool", thinPool),
	}, nil
}

// validateLVIdentifier validates that an LV name (vmID, snapshot name, etc.) is safe
// to use in LVM commands. It must match the expected pattern and not contain
// path separators or shell injection characters.
func (m *LVMManager) validateLVIdentifier(id string) error {
	if id == "" {
		return NewStorageError(ErrCodeInvalid, "LVM identifier cannot be empty", nil)
	}
	if !validLVMLVName.MatchString(id) {
		return NewStorageError(ErrCodeInvalid,
			fmt.Sprintf("invalid LVM identifier %q: must match %s (alphanumeric, underscore, hyphen, dot)", id, validLVMLVName.String()), nil)
	}
	// Additional safety: ensure no path traversal
	if strings.Contains(id, "..") {
		return NewStorageError(ErrCodeInvalid,
			fmt.Sprintf("invalid LVM identifier %q: must not contain path traversal characters", id), nil)
	}
	return nil
}

// normalizeLVName accepts either a bare LV name or the canonical device path
// returned by DiskIdentifier and resolves it to the LV name used in LVM queries.
func (m *LVMManager) normalizeLVName(identifier string) (string, error) {
	if !strings.HasPrefix(identifier, "/") {
		if err := m.validateLVIdentifier(identifier); err != nil {
			return "", err
		}
		return identifier, nil
	}

	prefix := m.lvPath("")
	if !strings.HasPrefix(identifier, prefix) {
		return "", NewStorageError(ErrCodeInvalid,
			fmt.Sprintf("invalid LVM path %q: must be within %s", identifier, strings.TrimSuffix(prefix, "/")), nil)
	}

	lvName := strings.TrimPrefix(identifier, prefix)
	if lvName == "" {
		return "", NewStorageError(ErrCodeInvalid,
			fmt.Sprintf("invalid LVM path %q: missing logical volume name", identifier), nil)
	}
	if strings.Contains(lvName, "/") {
		return "", NewStorageError(ErrCodeInvalid,
			fmt.Sprintf("invalid LVM path %q: must reference a single logical volume", identifier), nil)
	}
	if err := m.validateLVIdentifier(lvName); err != nil {
		return "", err
	}

	return lvName, nil
}

// lvPath returns the full device path for a logical volume.
func (m *LVMManager) lvPath(lvName string) string {
	return fmt.Sprintf("/dev/%s/%s", m.vgName, lvName)
}

// runLVMCommand executes an LVM command with the given arguments.
// It applies a default timeout if the context has no deadline.
// Returns the combined stdout/stderr output and any error.
func (m *LVMManager) runLVMCommand(ctx context.Context, args ...string) ([]byte, error) {
	// Use a default timeout if the context has no deadline
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultLVMTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "lvm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewStorageError(ErrCodeInternal,
				fmt.Sprintf("command timed out after %v", defaultLVMTimeout), err)
		}
		return nil, NewStorageError(ErrCodeInternal, string(output), err)
	}
	return output, nil
}

// DiskIdentifier returns the canonical block device path for a VM disk.
// The format is "/dev/{vg}/vs-{vmID}-disk0".
func (m *LVMManager) DiskIdentifier(vmID string) string {
	return m.lvPath(fmt.Sprintf(VMDiskNameFmt, vmID))
}

// CloneFromTemplate clones a template LV to create a new VM disk.
// For LVM thin pools, this uses lvcreate --thin -s for instant copy-on-write.
// sourcePool is unused (LVM uses the same VG/pool for templates).
// sourceImage is the template LV name (e.g., "ubuntu-2204-base").
// sourceSnap is unused (templates are LVs, not snapshots).
// targetImage is the new VM disk LV name (e.g., "vs-123-disk0").
func (m *LVMManager) CloneFromTemplate(ctx context.Context, sourcePool, sourceImage, sourceSnap, targetImage string) error {
	// Validate identifiers to prevent injection
	if err := m.validateLVIdentifier(sourceImage); err != nil {
		return fmt.Errorf("validating source image: %w", err)
	}
	if err := m.validateLVIdentifier(targetImage); err != nil {
		return fmt.Errorf("validating target image: %w", err)
	}

	// For LVM, we clone from the template LV directly
	// The template LV follows the naming convention {name}-base
	templatePath := m.lvPath(sourceImage)

	logger := m.logger.With("template", templatePath, "target", m.lvPath(targetImage))
	logger.Info("cloning template to VM disk")

	// Create thin snapshot of the template (instantaneous, CoW)
	// lvcreate --thin -s --name {targetImage} /dev/{vg}/{sourceImage}
	output, err := m.runLVMCommand(ctx,
		"lvcreate", "--thin", "-s",
		"--name", targetImage,
		templatePath)
	if err != nil {
		return fmt.Errorf("cloning template %s to %s: %w", sourceImage, targetImage, err)
	}

	logger.Debug("lvcreate output", "output", string(output))
	logger.Info("template cloned successfully")
	return nil
}

// CloneSnapshotToPool clones a snapshot to a target directory for backup.
// For LVM, targetDir is a filesystem path (backup directory), not a pool name.
// Creates a temporary thin snapshot, writes it out as a sparse raw file,
// then removes the temporary snapshot.
func (m *LVMManager) CloneSnapshotToPool(ctx context.Context, sourcePool, sourceImage, sourceSnap, targetDir, targetImage string) error {
	sourcePath := m.lvPath(sourceImage)
	tempSnapName := fmt.Sprintf("%s-bak", sourceSnap)
	tempSnapPath := m.lvPath(tempSnapName)
	targetPath := fmt.Sprintf("%s/%s.img", targetDir, targetImage)

	logger := m.logger.With("source", sourcePath, "temp_snap", tempSnapPath, "target", targetPath)
	logger.Info("cloning snapshot to backup file")

	// Create temporary thin snapshot
	_, err := m.runLVMCommand(ctx,
		"lvcreate", "--thin", "-s",
		"--name", tempSnapName,
		sourcePath)
	if err != nil {
		return fmt.Errorf("creating temporary snapshot %s: %w", tempSnapName, err)
	}

	// Ensure cleanup of temporary snapshot
	defer func() {
		_, cleanupErr := m.runLVMCommand(ctx, "lvremove", "-f", tempSnapPath)
		if cleanupErr != nil {
			logger.Warn("failed to remove temporary snapshot", "path", tempSnapPath, "error", cleanupErr)
		}
	}()

	// Use dd to write the snapshot to a sparse raw file
	ddCtx, ddCancel := context.WithTimeout(ctx, defaultLVMTimeout)
	defer ddCancel()

	ddCmd := exec.CommandContext(ddCtx, "dd",
		"if="+tempSnapPath,
		"of="+targetPath,
		"bs=4M",
		"conv=sparse")
	ddOutput, err := ddCmd.CombinedOutput()
	if err != nil {
		if ddCtx.Err() == context.DeadlineExceeded {
			return NewStorageError(ErrCodeInternal, "dd command timed out", err)
		}
		return fmt.Errorf("writing snapshot to file %s: %w: %s", targetPath, err, string(ddOutput))
	}

	logger.Debug("dd output", "output", string(ddOutput))
	logger.Info("snapshot cloned to backup file successfully")
	return nil
}

// Resize changes the size of an LVM thin LV to the new size in GB.
// Resize can only grow an LV, not shrink it.
// The guest OS is responsible for growing the filesystem (cloud-init / growpart).
func (m *LVMManager) Resize(ctx context.Context, imageName string, newSizeGB int) error {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return fmt.Errorf("normalizing image identifier: %w", err)
	}

	lvPath := m.lvPath(lvName)
	logger := m.logger.With("lv", lvPath, "size_gb", newSizeGB)
	logger.Info("resizing LVM thin LV")

	if newSizeGB <= 0 {
		return NewStorageError(ErrCodeInvalid, fmt.Sprintf("invalid size %d GB, must be positive", newSizeGB), nil)
	}

	// lvresize -L {sizeGB}G /dev/{vg}/{imageName}
	sizeArg := fmt.Sprintf("%dG", newSizeGB)
	output, err := m.runLVMCommand(ctx, "lvresize", "-L", sizeArg, lvPath)
	if err != nil {
		return fmt.Errorf("resizing LV %s to %dGB: %w", lvName, newSizeGB, err)
	}

	logger.Debug("lvresize output", "output", string(output))
	logger.Info("LV resized successfully")
	return nil
}

// Delete removes an LVM thin LV and all its dependent snapshots.
// Returns ErrCodeInUse if the device is open (VM running).
func (m *LVMManager) Delete(ctx context.Context, imageName string) error {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return fmt.Errorf("normalizing image identifier: %w", err)
	}

	lvPath := m.lvPath(lvName)
	logger := m.logger.With("lv", lvPath)
	logger.Info("deleting LVM thin LV")

	// Check for dependent thin snapshots
	output, err := m.runLVMCommand(ctx,
		"lvs", "--noheadings", "-o", "lv_name",
		"--select", fmt.Sprintf("origin=%s && pool_lv=%s", lvName, m.thinPool))
	if err != nil {
		// If the LV doesn't exist, treat as success (idempotent)
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			logger.Debug("LV does not exist, nothing to delete")
			return nil
		}
		return fmt.Errorf("checking dependents for %s: %w", lvName, err)
	}

	// Remove dependent snapshots first
	dependents := strings.Fields(string(output))
	for _, dep := range dependents {
		if dep == "" {
			continue
		}
		depPath := m.lvPath(dep)
		logger.Debug("removing dependent snapshot", "snapshot", depPath)
		_, err := m.runLVMCommand(ctx, "lvremove", "-f", depPath)
		if err != nil {
			return fmt.Errorf("removing dependent snapshot %s: %w", dep, err)
		}
	}

	// Remove the main LV
	output, err = m.runLVMCommand(ctx, "lvremove", "-f", lvPath)
	if err != nil {
		// Check for "device in use" error
		errMsg := err.Error()
		if strings.Contains(errMsg, "device in use") ||
			strings.Contains(errMsg, "is in use") ||
			strings.Contains(errMsg, "Logical volume") && strings.Contains(errMsg, "contains a filesystem in use") {
			return NewStorageError(ErrCodeInUse,
				fmt.Sprintf("cannot delete LV %s: device is in use (VM may be running)", imageName), err)
		}
		// If the LV doesn't exist, treat as success (idempotent)
		if strings.Contains(errMsg, "Failed to find logical volume") {
			logger.Debug("LV does not exist, nothing to delete")
			return nil
		}
		return fmt.Errorf("deleting LV %s: %w", lvName, err)
	}

	logger.Debug("lvremove output", "output", string(output))
	logger.Info("LV deleted successfully")
	return nil
}

// CreateSnapshot creates a thin snapshot of an existing LV.
// This is idempotent - if the snapshot already exists, it returns nil.
func (m *LVMManager) CreateSnapshot(ctx context.Context, imageName, snapName string) error {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return fmt.Errorf("normalizing image identifier: %w", err)
	}
	if err := m.validateLVIdentifier(snapName); err != nil {
		return fmt.Errorf("validating snapshot name: %w", err)
	}

	// Idempotent: check if snapshot already exists
	exists, err := m.ImageExists(ctx, snapName)
	if err != nil {
		return fmt.Errorf("checking if snapshot %s exists: %w", snapName, err)
	}
	if exists {
		m.logger.Debug("snapshot already exists, skipping creation", "snapshot", snapName)
		return nil
	}

	sourcePath := m.lvPath(lvName)
	snapPath := m.lvPath(snapName)

	logger := m.logger.With("source", sourcePath, "snapshot", snapPath)
	logger.Info("creating thin snapshot")

	// lvcreate --thin -s --name {snapName} /dev/{vg}/{imageName}
	// No -L size argument; thin snapshots take space from the pool on write
	output, err := m.runLVMCommand(ctx,
		"lvcreate", "--thin", "-s",
		"--name", snapName,
		sourcePath)
	if err != nil {
		return fmt.Errorf("creating snapshot %s from %s: %w", snapName, lvName, err)
	}

	logger.Debug("lvcreate output", "output", string(output))
	logger.Info("snapshot created successfully")
	return nil
}

// DeleteSnapshot removes a thin snapshot LV.
func (m *LVMManager) DeleteSnapshot(ctx context.Context, imageName, snapName string) error {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return fmt.Errorf("normalizing image identifier: %w", err)
	}
	if err := m.validateLVIdentifier(snapName); err != nil {
		return fmt.Errorf("validating snapshot name: %w", err)
	}

	exists, err := m.ImageExists(ctx, snapName)
	if err != nil {
		return fmt.Errorf("checking if snapshot %s exists: %w", snapName, err)
	}
	if !exists {
		m.logger.Debug("snapshot does not exist, nothing to delete", "snapshot", snapName)
		return nil
	}

	snapshotBelongsToImage, err := m.snapshotBelongsToImage(ctx, lvName, snapName)
	if err != nil {
		return fmt.Errorf("checking if snapshot %s belongs to %s: %w", snapName, lvName, err)
	}
	if !snapshotBelongsToImage {
		return NewStorageError(ErrCodeNotFound,
			fmt.Sprintf("snapshot %s for image %s", snapName, lvName), nil)
	}

	snapPath := m.lvPath(snapName)
	logger := m.logger.With("snapshot", snapPath)
	logger.Info("deleting thin snapshot")

	output, err := m.runLVMCommand(ctx, "lvremove", "-f", snapPath)
	if err != nil {
		// Treat "Failed to find logical volume" as success (idempotent)
		if strings.Contains(err.Error(), "Failed to find logical volume") {
			logger.Debug("snapshot does not exist, nothing to delete")
			return nil
		}
		return fmt.Errorf("deleting snapshot %s: %w", snapName, err)
	}

	logger.Debug("lvremove output", "output", string(output))
	logger.Info("snapshot deleted successfully")
	return nil
}

// ListSnapshots returns all thin snapshots of an LV in the configured pool.
func (m *LVMManager) ListSnapshots(ctx context.Context, imageName string) ([]SnapshotInfo, error) {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return nil, fmt.Errorf("normalizing image identifier: %w", err)
	}

	// lvs --noheadings --units b -o lv_name,lv_size --select "origin={imageName} && pool_lv={thinPool}"
	output, err := m.runLVMCommand(ctx,
		"lvs", "--noheadings", "--units", "b", "-o", "lv_name,lv_size",
		"--select", fmt.Sprintf("origin=%s && pool_lv=%s", lvName, m.thinPool))
	if err != nil {
		return nil, fmt.Errorf("listing snapshots of %s: %w", lvName, err)
	}

	// Parse the output
	// Format: "  snap1   1073741824B\n  snap2   2147483648B\n"
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	snapshots := make([]SnapshotInfo, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		name := fields[0]
		sizeStr := fields[1]

		// Parse size (remove trailing 'B')
		sizeStr = strings.TrimSuffix(sizeStr, "B")
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			m.logger.Warn("failed to parse snapshot size", "snapshot", name, "size_str", sizeStr, "error", err)
			continue
		}

		snapshots = append(snapshots, SnapshotInfo{
			Name: name,
			Size: size,
			// LVM thin snapshots don't have a "protected" concept like RBD
			Protected: false,
		})
	}

	return snapshots, nil
}

// GetImageSize returns the virtual size of an LVM thin LV in bytes.
// This returns the virtual size, not the amount of pool space consumed.
func (m *LVMManager) GetImageSize(ctx context.Context, imageName string) (int64, error) {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return 0, fmt.Errorf("normalizing image identifier: %w", err)
	}

	lvPath := m.lvPath(lvName)

	// lvs --noheadings --units b -o lv_size /dev/{vg}/{imageName}
	output, err := m.runLVMCommand(ctx,
		"lvs", "--noheadings", "--units", "b", "-o", "lv_size", lvPath)
	if err != nil {
		return 0, fmt.Errorf("getting size of LV %s: %w", lvName, err)
	}

	// Parse the output (strip trailing 'B')
	sizeStr := strings.TrimSpace(string(output))
	sizeStr = strings.TrimSuffix(sizeStr, "B")

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing size of LV %s: %w", lvName, err)
	}

	return size, nil
}

// ImageExists checks if an LVM thin LV exists.
func (m *LVMManager) ImageExists(ctx context.Context, imageName string) (bool, error) {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return false, fmt.Errorf("normalizing image identifier: %w", err)
	}

	lvPath := m.lvPath(lvName)

	// lvs /dev/{vg}/{imageName} - exit code 0 = exists, non-zero = does not exist
	_, err = m.runLVMCommand(ctx, "lvs", lvPath)
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find logical volume") || isLVMNotFoundExit(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking if LV %s exists: %w", lvName, err)
	}
	return true, nil
}

func isLVMNotFoundExit(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.ExitCode() == 5
}

// FlattenImage severs a snapshot's CoW relationship with its origin,
// making it a fully independent thin LV.
func (m *LVMManager) FlattenImage(ctx context.Context, imageName string) error {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return fmt.Errorf("normalizing image identifier: %w", err)
	}

	lvPath := m.lvPath(lvName)
	logger := m.logger.With("lv", lvPath)
	logger.Info("flattening thin snapshot")

	// lvconvert --splitsnapshot /dev/{vg}/{imageName}
	output, err := m.runLVMCommand(ctx, "lvconvert", "--splitsnapshot", lvPath)
	if err != nil {
		return fmt.Errorf("flattening snapshot %s: %w", lvName, err)
	}

	logger.Debug("lvconvert output", "output", string(output))
	logger.Info("snapshot flattened successfully")
	return nil
}

// GetPoolStats returns storage statistics for the thin pool LV.
// It queries the thin pool LV, not the VG, for accurate usage data.
func (m *LVMManager) GetPoolStats(ctx context.Context) (*PoolStats, error) {
	dataPercent, metadataPercent, err := m.ThinPoolStats(ctx)
	if err != nil {
		return nil, err
	}

	// Get the total size of the thin pool LV
	poolPath := m.lvPath(m.thinPool)
	output, err := m.runLVMCommand(ctx,
		"lvs", "--noheadings", "--units", "b", "-o", "lv_size", poolPath)
	if err != nil {
		return nil, fmt.Errorf("getting thin pool size: %w", err)
	}

	// Parse the total size
	sizeStr := strings.TrimSpace(string(output))
	sizeStr = strings.TrimSuffix(sizeStr, "B")
	total, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing thin pool size: %w", err)
	}

	// Calculate used and free
	used := int64(float64(total) * dataPercent / 100.0)
	free := total - used

	// Log warnings for high usage
	if dataPercent > 80 {
		m.logger.Warn("thin pool data usage is high", "data_percent", dataPercent)
	}
	if metadataPercent > 50 {
		m.logger.Warn("thin pool metadata usage is high", "metadata_percent", metadataPercent)
	}

	return &PoolStats{
		Total: total,
		Used:  used,
		Free:  free,
	}, nil
}

// ThinPoolStats returns the data and metadata usage percentages for the thin pool.
// This is a public helper used by health checks and GetPoolStats.
func (m *LVMManager) ThinPoolStats(ctx context.Context) (dataPercent, metadataPercent float64, err error) {
	poolPath := m.lvPath(m.thinPool)

	// lvs --noheadings --units b -o lv_size,data_percent,metadata_percent /dev/{vg}/{thinPool}
	output, err := m.runLVMCommand(ctx,
		"lvs", "--noheadings", "-o", "data_percent,metadata_percent", poolPath)
	if err != nil {
		return 0, 0, fmt.Errorf("getting thin pool stats: %w", err)
	}

	// Parse the output
	// Format: "  45.20  12.80\n"
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("unexpected lvs output format: %s", string(output))
	}

	dataPercent, err = strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing data_percent: %w", err)
	}

	metadataPercent, err = strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing metadata_percent: %w", err)
	}

	return dataPercent, metadataPercent, nil
}

// Rollback reverts an LV to a previous snapshot state in-place.
// Uses a three-step swap to avoid data-loss window.
// The VM must be stopped before calling this method.
func (m *LVMManager) Rollback(ctx context.Context, imageName, snapshotName string) error {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return fmt.Errorf("normalizing image identifier: %w", err)
	}
	if err := m.validateLVIdentifier(snapshotName); err != nil {
		return fmt.Errorf("validating snapshot name: %w", err)
	}

	// Verify snapshot exists first
	exists, err := m.ImageExists(ctx, snapshotName)
	if err != nil {
		return fmt.Errorf("checking if snapshot %s exists: %w", snapshotName, err)
	}
	if !exists {
		return NewStorageError(ErrCodeNotFound,
			fmt.Sprintf("snapshot %s", snapshotName), nil)
	}

	snapshotBelongsToImage, err := m.snapshotBelongsToImage(ctx, lvName, snapshotName)
	if err != nil {
		return fmt.Errorf("checking if snapshot %s belongs to %s: %w", snapshotName, lvName, err)
	}
	if !snapshotBelongsToImage {
		return NewStorageError(ErrCodeNotFound,
			fmt.Sprintf("snapshot %s for image %s", snapshotName, lvName), nil)
	}

	lvPath := m.lvPath(lvName)
	snapPath := m.lvPath(snapshotName)
	oldPath := m.lvPath(lvName + "-old")

	logger := m.logger.With("lv", lvPath, "snapshot", snapPath, "old", oldPath)
	logger.Info("rolling back LV to snapshot")

	// Step 1: Rename current disk to -old
	logger.Debug("step 1: renaming current disk to -old")
	_, err = m.runLVMCommand(ctx, "lvrename", lvPath, lvName+"-old")
	if err != nil {
		return fmt.Errorf("renaming LV %s to %s-old: %w", lvName, lvName, err)
	}

	// Step 2: Rename snapshot to main disk name
	logger.Debug("step 2: renaming snapshot to main disk name")
	_, err = m.runLVMCommand(ctx, "lvrename", snapPath, lvName)
	if err != nil {
		// Attempt to rename the old disk back
		_, rollbackErr := m.runLVMCommand(ctx, "lvrename", oldPath, lvName)
		if rollbackErr != nil {
			logger.Error("failed to rollback after rename failure",
				"original_error", err, "rollback_error", rollbackErr)
		}
		return fmt.Errorf("renaming snapshot %s to %s: %w", snapshotName, lvName, err)
	}

	// Step 3: Remove the old disk
	logger.Debug("step 3: removing old disk")
	_, err = m.runLVMCommand(ctx, "lvremove", "-f", oldPath)
	if err != nil {
		logger.Warn("failed to remove old disk after rollback", "path", oldPath, "error", err)
		// Don't fail the operation - the rollback was successful
	}

	logger.Info("LV rolled back to snapshot successfully")
	return nil
}

func (m *LVMManager) snapshotBelongsToImage(ctx context.Context, imageName, snapshotName string) (bool, error) {
	snapshots, err := m.ListSnapshots(ctx, imageName)
	if err != nil {
		return false, fmt.Errorf("listing snapshots for %s: %w", imageName, err)
	}

	for _, snapshot := range snapshots {
		if snapshot.Name == snapshotName {
			return true, nil
		}
	}

	return false, nil
}

// GetStorageType returns the storage backend type.
func (m *LVMManager) GetStorageType() StorageType {
	return StorageTypeLVM
}

// VolumeGroup returns the LVM volume group name.
func (m *LVMManager) VolumeGroup() string {
	return m.vgName
}

// ThinPoolName returns the thin pool LV name.
func (m *LVMManager) ThinPoolName() string {
	return m.thinPool
}

// CreateImage creates a new LVM thin LV of the specified size.
func (m *LVMManager) CreateImage(ctx context.Context, imageName string, sizeGB int) error {
	if sizeGB <= 0 {
		return NewStorageError(ErrCodeInvalid, fmt.Sprintf("invalid size %d GB, must be positive", sizeGB), nil)
	}

	lvPath := m.lvPath(imageName)
	logger := m.logger.With("lv", lvPath, "size_gb", sizeGB)

	// Check if LV already exists
	if exists, _ := m.ImageExists(ctx, imageName); exists {
		return NewStorageError(ErrCodeAlreadyExists, fmt.Sprintf("LV %q already exists", imageName), nil)
	}

	// Create thin LV: lvcreate -T vg/thinpool -V {size}G -n {name}
	// The -V specifies virtual size, -T specifies thin pool
	cmd := exec.CommandContext(ctx, "lvcreate",
		"-T", fmt.Sprintf("%s/%s", m.vgName, m.thinPool),
		"-V", fmt.Sprintf("%dG", sizeGB),
		"-n", imageName,
		"-y") // Auto-confirm
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating LV %s: %w (output: %s)", lvPath, err, string(output))
	}

	logger.Info("LV created successfully")
	return nil
}

// GetImageInfo returns detailed information about an LVM logical volume.
func (m *LVMManager) GetImageInfo(ctx context.Context, imageName string) (*ImageInfo, error) {
	lvName, err := m.normalizeLVName(imageName)
	if err != nil {
		return nil, fmt.Errorf("normalizing image identifier: %w", err)
	}

	// Check if LV exists
	if exists, _ := m.ImageExists(ctx, lvName); !exists {
		return nil, NewStorageError(ErrCodeNotFound, fmt.Sprintf("LV %q", lvName), nil)
	}

	lvPath := m.lvPath(lvName)

	// Get LV size using the same LVM wrapper as the rest of the backend.
	output, err := m.runLVMCommand(ctx,
		"lvs",
		"--noheadings",
		"--units", "b",
		"-o", "lv_size,lv_attr",
		"--reportformat", "json",
		fmt.Sprintf("%s/%s", m.vgName, lvName))
	if err != nil {
		return nil, fmt.Errorf("getting LV info for %s: %w", lvPath, err)
	}

	// Parse JSON output
	var lvsOutput struct {
		Report []struct {
			LV []struct {
				LvSize string `json:"lv_size"`
				LvAttr string `json:"lv_attr"`
			} `json:"lv"`
		} `json:"report"`
	}
	if err := json.Unmarshal(output, &lvsOutput); err != nil {
		return nil, fmt.Errorf("parsing lvs output for %s: %w", lvPath, err)
	}

	if len(lvsOutput.Report) == 0 || len(lvsOutput.Report[0].LV) == 0 {
		return nil, fmt.Errorf("no LV info found for %s", lvPath)
	}

	lv := lvsOutput.Report[0].LV[0]

	// Parse size (lvs returns bytes with B suffix, e.g., "10737418240B")
	sizeStr := strings.TrimSuffix(lv.LvSize, "B")
	sizeBytes, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing LV size %s: %w", lv.LvSize, err)
	}

	return &ImageInfo{
		Filename:         lvPath,
		Format:           "lvm",
		VirtualSizeBytes: sizeBytes,
		ActualSizeBytes:  0, // LVM thin doesn't track this directly
		DirtyFlag:        false,
		ClusterSize:      0,
		BackingFile:      "",
	}, nil
}
