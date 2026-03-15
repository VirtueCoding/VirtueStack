// Package storage provides storage backend abstractions for VM disk management.
// This file implements a file-based QCOW2 storage backend using qemu-img.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// QCOWManager handles file-based QCOW2 operations for VM disk management.
// It implements StorageBackend for local file-based storage using qemu-img.
// All operations are safe for concurrent use by multiple goroutines.
type QCOWManager struct {
	basePath string
	logger   *slog.Logger
}

// NewQCOWManager creates a new QCOWManager for the given base directory.
// The basePath is the root directory for VM disk images (e.g., /var/lib/virtuestack/vms).
// The directory is created if it does not exist.
func NewQCOWManager(basePath string, logger *slog.Logger) (*QCOWManager, error) {
	if basePath == "" {
		return nil, NewStorageError(ErrCodeInvalid, "basePath cannot be empty", nil)
	}
	if logger == nil {
		return nil, NewStorageError(ErrCodeInvalid, "logger cannot be nil", nil)
	}

	// Ensure base directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("creating base directory %q: %w", basePath, err)
	}

	return &QCOWManager{
		basePath: basePath,
		logger:   logger.With("component", "qcow-manager"),
	}, nil
}

// vmDiskPath returns the full path for a VM's primary disk image.
func (m *QCOWManager) vmDiskPath(vmID string) string {
	return filepath.Join(m.basePath, fmt.Sprintf("%s-disk0.qcow2", vmID))
}

// imagePath returns the full path for an image within the base directory.
func (m *QCOWManager) imagePath(imageName string) string {
	// If already an absolute path, return as-is
	if filepath.IsAbs(imageName) {
		return imageName
	}
	return filepath.Join(m.basePath, imageName)
}

// runCommand executes a command with the given arguments and timeout.
// Returns the combined stdout/stderr output and any error.
func (m *QCOWManager) runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, qemuImgTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("command %q timed out after %v", name, qemuImgTimeout)
		}
		return nil, fmt.Errorf("command %q failed: %w: %s", name, err, string(output))
	}
	return output, nil
}

// CloneFromTemplate clones a template QCOW2 image to create a new VM disk.
// For QCOW2, sourcePool is interpreted as the directory containing the template,
// sourceImage is the template filename, sourceSnap is unused (QCOW2 uses backing files),
// and targetImage becomes the new VM disk name.
// Uses copy-on-write with backing file for instant cloning.
func (m *QCOWManager) CloneFromTemplate(ctx context.Context, sourcePool, sourceImage, sourceSnap, targetImage string) error {
	// For QCOW2, construct the source template path
	// sourcePool is the template directory, sourceImage is the template file
	templatePath := filepath.Join(sourcePool, sourceImage)
	if !filepath.IsAbs(sourcePool) {
		templatePath = filepath.Join(m.basePath, sourcePool, sourceImage)
	}

	// Verify template exists
	if _, err := os.Stat(templatePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError(ErrCodeNotFound, fmt.Sprintf("template %q", templatePath), err)
		}
		return fmt.Errorf("checking template %q: %w", templatePath, err)
	}

	// Target disk path
	targetPath := m.imagePath(targetImage)

	logger := m.logger.With("template", templatePath, "target", targetPath)
	logger.Info("cloning template to VM disk")

	// Create copy-on-write image with backing file
	// qemu-img create -b template.qcow2 -F qcow2 -f qcow2 vm-disk.qcow2
	output, err := m.runCommand(ctx, "qemu-img", "create",
		"-b", templatePath,
		"-F", "qcow2",
		"-f", "qcow2",
		targetPath)
	if err != nil {
		return fmt.Errorf("cloning template to %s: %w", targetImage, err)
	}

	logger.Debug("qemu-img output", "output", string(output))
	logger.Info("template cloned successfully")
	return nil
}

// CloneSnapshotToPool clones a disk image for backup purposes.
// For QCOW2, this creates a standalone copy of the image at the target location.
func (m *QCOWManager) CloneSnapshotToPool(ctx context.Context, sourcePool, sourceImage, sourceSnap, targetPool, targetImage string) error {
	// Construct source path
	sourcePath := filepath.Join(sourcePool, sourceImage)
	if !filepath.IsAbs(sourcePool) {
		sourcePath = filepath.Join(m.basePath, sourcePool, sourceImage)
	}

	// Construct target path
	targetPath := filepath.Join(targetPool, targetImage)
	if !filepath.IsAbs(targetPool) {
		targetPath = filepath.Join(m.basePath, targetPool, targetImage)
	}

	logger := m.logger.With("source", sourcePath, "target", targetPath)
	logger.Info("cloning disk to target pool")

	// Verify source exists
	if _, err := os.Stat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError(ErrCodeNotFound, fmt.Sprintf("source image %q", sourcePath), err)
		}
		return fmt.Errorf("checking source image %q: %w", sourcePath, err)
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating target directory %q: %w", targetDir, err)
	}

	// Use qemu-img convert to create a standalone copy (flattens backing chain)
	output, err := m.runCommand(ctx, "qemu-img", "convert",
		"-f", "qcow2",
		"-O", "qcow2",
		sourcePath,
		targetPath)
	if err != nil {
		return fmt.Errorf("cloning %s to %s: %w", sourceImage, targetImage, err)
	}

	logger.Debug("qemu-img output", "output", string(output))
	logger.Info("disk cloned to target pool successfully")
	return nil
}

// Resize changes the size of a QCOW2 image to the new size in GB.
// Resize can only grow an image, not shrink it.
func (m *QCOWManager) Resize(ctx context.Context, imageName string, newSizeGB int) error {
	imagePath := m.imagePath(imageName)
	logger := m.logger.With("image", imagePath, "size_gb", newSizeGB)
	logger.Info("resizing QCOW2 image")

	if newSizeGB <= 0 {
		return NewStorageError(ErrCodeInvalid, fmt.Sprintf("invalid size %d GB, must be positive", newSizeGB), nil)
	}

	// Verify image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// Resize with qemu-img
	sizeArg := fmt.Sprintf("%dG", newSizeGB)
	output, err := m.runCommand(ctx, "qemu-img", "resize", imagePath, sizeArg)
	if err != nil {
		return fmt.Errorf("resizing image %s to %dGB: %w", imageName, newSizeGB, err)
	}

	logger.Debug("qemu-img output", "output", string(output))
	logger.Info("image resized successfully")
	return nil
}

// Delete removes a QCOW2 image file.
func (m *QCOWManager) Delete(ctx context.Context, imageName string) error {
	imagePath := m.imagePath(imageName)
	logger := m.logger.With("image", imagePath)
	logger.Info("deleting QCOW2 image")

	// Check if image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			logger.Debug("image does not exist, nothing to delete")
			return nil
		}
		return fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// Remove the file
	if err := os.Remove(imagePath); err != nil {
		return fmt.Errorf("deleting image %s: %w", imageName, err)
	}

	logger.Info("image deleted successfully")
	return nil
}

// CreateSnapshot creates a new internal snapshot of a QCOW2 image.
func (m *QCOWManager) CreateSnapshot(ctx context.Context, imageName, snapshotName string) error {
	imagePath := m.imagePath(imageName)
	logger := m.logger.With("image", imagePath, "snapshot", snapshotName)
	logger.Info("creating snapshot")

	// Verify image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// Create snapshot with qemu-img
	output, err := m.runCommand(ctx, "qemu-img", "snapshot", "-c", snapshotName, imagePath)
	if err != nil {
		return fmt.Errorf("creating snapshot %s@%s: %w", imageName, snapshotName, err)
	}

	logger.Debug("qemu-img output", "output", string(output))
	logger.Info("snapshot created successfully")
	return nil
}

// DeleteSnapshot removes an internal snapshot from a QCOW2 image.
func (m *QCOWManager) DeleteSnapshot(ctx context.Context, imageName, snapshotName string) error {
	imagePath := m.imagePath(imageName)
	logger := m.logger.With("image", imagePath, "snapshot", snapshotName)
	logger.Info("deleting snapshot")

	// Verify image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// Delete snapshot with qemu-img
	output, err := m.runCommand(ctx, "qemu-img", "snapshot", "-d", snapshotName, imagePath)
	if err != nil {
		return fmt.Errorf("deleting snapshot %s@%s: %w", imageName, snapshotName, err)
	}

	logger.Debug("qemu-img output", "output", string(output))
	logger.Info("snapshot deleted successfully")
	return nil
}

// ProtectSnapshot marks a snapshot as protected.
// Note: QCOW2 does not support snapshot protection natively.
// This method returns an error indicating the limitation.
func (m *QCOWManager) ProtectSnapshot(ctx context.Context, imageName, snapshotName string) error {
	return NewStorageError(ErrCodeInternal,
		"QCOW2 does not support snapshot protection natively; consider using file permissions or external tracking",
		nil)
}

// UnprotectSnapshot removes protection from a snapshot.
// Note: QCOW2 does not support snapshot protection natively.
// This method returns an error indicating the limitation.
func (m *QCOWManager) UnprotectSnapshot(ctx context.Context, imageName, snapshotName string) error {
	return NewStorageError(ErrCodeInternal,
		"QCOW2 does not support snapshot protection natively",
		nil)
}

// ListSnapshots returns all internal snapshots of a QCOW2 image.
func (m *QCOWManager) ListSnapshots(ctx context.Context, imageName string) ([]SnapshotInfo, error) {
	imagePath := m.imagePath(imageName)

	// Verify image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return nil, NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return nil, fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// List snapshots with qemu-img
	output, err := m.runCommand(ctx, "qemu-img", "snapshot", "-l", imagePath)
	if err != nil {
		return nil, fmt.Errorf("listing snapshots of %s: %w", imageName, err)
	}

	// Parse the output
	snapshots := m.parseSnapshotList(string(output))
	return snapshots, nil
}

// parseSnapshotList parses the output of qemu-img snapshot -l.
// Output format:
// Snapshot list:
// ID        TAG                 VM SIZE                DATE       VM CLOCK
// 1         snap1               1.2G      2024-01-15 10:30:00   00:00:05.123
func (m *QCOWManager) parseSnapshotList(output string) []SnapshotInfo {
	lines := strings.Split(output, "\n")
	var snapshots []SnapshotInfo

	inSnapshotSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Find the start of snapshot list
		if strings.HasPrefix(line, "Snapshot list:") {
			inSnapshotSection = true
			continue
		}

		if !inSnapshotSection {
			continue
		}

		// Skip header line
		if strings.HasPrefix(line, "ID") || strings.Contains(line, "TAG") {
			continue
		}

		// Parse snapshot line
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			// fields[0] = ID, fields[1] = TAG (name), fields[2..n-3] = SIZE, fields[n-3..] = DATE/TIME
			name := fields[1]
			snapshots = append(snapshots, SnapshotInfo{
				Name:      name,
				Size:      0, // Size parsing is complex, skip for now
				Protected: false,
			})
		}
	}

	return snapshots
}

// GetImageSize returns the virtual size of a QCOW2 image in bytes.
func (m *QCOWManager) GetImageSize(ctx context.Context, imageName string) (int64, error) {
	imagePath := m.imagePath(imageName)

	// Verify image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return 0, NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return 0, fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// Get image info as JSON
	output, err := m.runCommand(ctx, "qemu-img", "info", "--output=json", imagePath)
	if err != nil {
		return 0, fmt.Errorf("getting image info for %s: %w", imageName, err)
	}

	// Parse JSON response
	var info struct {
		VirtualSize int64 `json:"virtual-size"`
		ActualSize  int64 `json:"actual-size"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return 0, fmt.Errorf("parsing image info for %s: %w", imageName, err)
	}

	return info.VirtualSize, nil
}

// ImageExists checks if a QCOW2 image file exists.
func (m *QCOWManager) ImageExists(ctx context.Context, imageName string) (bool, error) {
	imagePath := m.imagePath(imageName)

	_, err := os.Stat(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of image %q: %w", imagePath, err)
	}
	return true, nil
}

// FlattenImage removes the backing file dependency from a QCOW2 image.
// This makes the image independent and self-contained.
func (m *QCOWManager) FlattenImage(ctx context.Context, imageName string) error {
	imagePath := m.imagePath(imageName)
	logger := m.logger.With("image", imagePath)
	logger.Info("flattening QCOW2 image")

	// Verify image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// Create a temporary file for the flattened image
	tempPath := imagePath + ".flatten-temp"

	// Convert to a new standalone image (this removes backing file)
	output, err := m.runCommand(ctx, "qemu-img", "convert",
		"-f", "qcow2",
		"-O", "qcow2",
		imagePath,
		tempPath)
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("flattening image %s: %w", imageName, err)
	}

	logger.Debug("qemu-img output", "output", string(output))

	// Replace original with flattened version
	if err := os.Rename(tempPath, imagePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("replacing image with flattened version %s: %w", imageName, err)
	}

	logger.Info("image flattened successfully")
	return nil
}

// Rollback reverts a QCOW2 image to a previous internal snapshot state in-place.
// This uses qemu-img snapshot -a to apply the snapshot, restoring the disk
// contents to the state at the time the snapshot was taken.
func (m *QCOWManager) Rollback(ctx context.Context, imageName, snapshotName string) error {
	imagePath := m.imagePath(imageName)
	logger := m.logger.With("image", imagePath, "snapshot", snapshotName)
	logger.Info("rolling back QCOW2 image to snapshot")

	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	output, err := m.runCommand(ctx, "qemu-img", "snapshot", "-a", snapshotName, imagePath)
	if err != nil {
		return fmt.Errorf("rolling back image %s to snapshot %s: %w", imageName, snapshotName, err)
	}

	logger.Debug("qemu-img output", "output", string(output))
	logger.Info("image rolled back to snapshot successfully")
	return nil
}

// GetPoolStats returns storage statistics for the base directory.
// Uses syscall.Statfs to get filesystem capacity and usage.
func (m *QCOWManager) GetPoolStats(ctx context.Context) (*PoolStats, error) {
	var stat syscall.Statfs_t

	if err := syscall.Statfs(m.basePath, &stat); err != nil {
		return nil, fmt.Errorf("getting filesystem stats for %q: %w", m.basePath, err)
	}

	// Calculate sizes in bytes
	// stat.Blocks is in units of stat.Bsize (optimal transfer block size)
	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize) // Available to non-root
	used := total - free

	return &PoolStats{
		Total: total,
		Used:  used,
		Free:  free,
	}, nil
}

// GetStorageType returns the storage backend type.
func (m *QCOWManager) GetStorageType() StorageType {
	return StorageTypeQCOW
}

// CreateImage creates a new empty QCOW2 image of the specified size.
// This is an additional helper method not part of StorageBackend interface.
func (m *QCOWManager) CreateImage(ctx context.Context, imageName string, sizeGB int) error {
	imagePath := m.imagePath(imageName)
	logger := m.logger.With("image", imagePath, "size_gb", sizeGB)
	logger.Info("creating QCOW2 image")

	if sizeGB <= 0 {
		return NewStorageError(ErrCodeInvalid, fmt.Sprintf("invalid size %d GB, must be positive", sizeGB), nil)
	}

	// Check if image already exists
	if _, err := os.Stat(imagePath); err == nil {
		return NewStorageError(ErrCodeAlreadyExists, fmt.Sprintf("image %q", imageName), nil)
	}

	// Create image with qemu-img
	sizeArg := fmt.Sprintf("%dG", sizeGB)
	output, err := m.runCommand(ctx, "qemu-img", "create",
		"-f", "qcow2",
		imagePath,
		sizeArg)
	if err != nil {
		return fmt.Errorf("creating image %s: %w", imageName, err)
	}

	logger.Debug("qemu-img output", "output", string(output))
	logger.Info("image created successfully")
	return nil
}

// GetImageInfo returns detailed information about a QCOW2 image.
// This is an additional helper method not part of StorageBackend interface.
func (m *QCOWManager) GetImageInfo(ctx context.Context, imageName string) (map[string]interface{}, error) {
	imagePath := m.imagePath(imageName)

	// Verify image exists
	if _, err := os.Stat(imagePath); err != nil {
		if os.IsNotExist(err) {
			return nil, NewStorageError(ErrCodeNotFound, fmt.Sprintf("image %q", imageName), err)
		}
		return nil, fmt.Errorf("checking image %q: %w", imagePath, err)
	}

	// Get image info as JSON
	output, err := m.runCommand(ctx, "qemu-img", "info", "--output=json", imagePath)
	if err != nil {
		return nil, fmt.Errorf("getting image info for %s: %w", imageName, err)
	}

	// Parse JSON into generic map
	var info map[string]interface{}
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("parsing image info for %s: %w", imageName, err)
	}

	return info, nil
}

// CheckQemuImgAvailable checks if qemu-img is available on the system.
// Returns an error if qemu-img is not found or not executable.
func CheckQemuImgAvailable() error {
	cmd := exec.Command("qemu-img", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qemu-img not available: %w", err)
	}
	return nil
}

// parseSize parses a size string like "1.2G" or "500M" into bytes.
func parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	if len(sizeStr) == 0 {
		return 0, fmt.Errorf("empty size string")
	}

	// Extract numeric part and suffix
	var numStr string
	var multiplier int64 = 1

	lastChar := strings.ToUpper(string(sizeStr[len(sizeStr)-1]))
	switch lastChar {
	case "K":
		multiplier = 1024
		numStr = sizeStr[:len(sizeStr)-1]
	case "M":
		multiplier = 1024 * 1024
		numStr = sizeStr[:len(sizeStr)-1]
	case "G":
		multiplier = 1024 * 1024 * 1024
		numStr = sizeStr[:len(sizeStr)-1]
	case "T":
		multiplier = 1024 * 1024 * 1024 * 1024
		numStr = sizeStr[:len(sizeStr)-1]
	default:
		// No suffix, assume bytes
		if lastChar >= "0" && lastChar <= "9" {
			numStr = sizeStr
		} else {
			return 0, fmt.Errorf("invalid size suffix: %s", lastChar)
		}
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing size number %q: %w", numStr, err)
	}

	return int64(num * float64(multiplier)), nil
}
