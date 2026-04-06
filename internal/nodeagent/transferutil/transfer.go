package transferutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	validLVMName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.+-]*$`)
	validLVName  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

	ErrInvalidOffset = errors.New("invalid transfer offset")
	ErrTransferSize  = errors.New("invalid transfer size")
	ErrCreateImage   = errors.New("create transfer image")
	ErrOpenTarget    = errors.New("open transfer target")
	ErrReadProcess   = errors.New("read process output")
	ErrSendProcess   = errors.New("send process chunk")
	ErrWaitProcess   = errors.New("wait for process")
)

const bytesPerGiB int64 = 1024 * 1024 * 1024

type ReceiveTarget struct {
	OpenPath      string
	CreateImageID string
}

type PreparedVMDisk struct {
	DiskPath    string
	LVMDiskPath string
}

type ReceiveTracker struct {
	total         int64
	bytesReceived int64
}

func ResolveLVMSourcePath(sourceDiskPath, snapshotName, requestedVG, configuredVG string) (string, error) {
	if err := validateVG(configuredVG); err != nil {
		return "", err
	}
	if requestedVG != "" && requestedVG != configuredVG {
		return "", fmt.Errorf("requested volume group %q does not match configured volume group", requestedVG)
	}

	if snapshotName != "" {
		if err := validateLV(snapshotName); err != nil {
			return "", err
		}
		return canonicalLVMPath(configuredVG, snapshotName), nil
	}

	vg, lv, err := splitLVMPath(sourceDiskPath)
	if err != nil {
		return "", err
	}
	if vg != configuredVG {
		return "", fmt.Errorf("source disk path volume group %q does not match configured volume group", vg)
	}
	return canonicalLVMPath(vg, lv), nil
}

func ResolveQCOWSourcePath(sourceDiskPath, storagePath string) (string, error) {
	if err := ValidatePathWithin(sourceDiskPath, storagePath); err != nil {
		return "", err
	}
	return filepath.Clean(sourceDiskPath), nil
}

func ResolveQCOWVMDiskPath(storagePath, vmID string) string {
	return filepath.Join(storagePath, "vms", fmt.Sprintf("%s-disk0.qcow2", vmID))
}

func ValidateSnapshotName(snapshotName string) error {
	if snapshotName == "" {
		return nil
	}

	return validateLV(snapshotName)
}

func ResolveReceiveTarget(storageBackend, targetPath, storagePath, configuredVG, configuredThinPool, requestedVG, requestedThinPool string) (ReceiveTarget, error) {
	switch storageBackend {
	case "lvm":
		if err := validateVG(configuredVG); err != nil {
			return ReceiveTarget{}, err
		}
		if err := validateVG(configuredThinPool); err != nil {
			return ReceiveTarget{}, fmt.Errorf("invalid thin pool: %w", err)
		}
		if requestedVG != "" && requestedVG != configuredVG {
			return ReceiveTarget{}, fmt.Errorf("requested volume group %q does not match configured volume group", requestedVG)
		}
		if requestedThinPool != "" && requestedThinPool != configuredThinPool {
			return ReceiveTarget{}, fmt.Errorf("requested thin pool %q does not match configured thin pool", requestedThinPool)
		}
		vg, lv, err := splitLVMPath(targetPath)
		if err != nil {
			return ReceiveTarget{}, err
		}
		if vg != configuredVG {
			return ReceiveTarget{}, fmt.Errorf("target disk path volume group %q does not match configured volume group", vg)
		}
		return ReceiveTarget{
			OpenPath:      canonicalLVMPath(vg, lv),
			CreateImageID: lv,
		}, nil
	default:
		if err := ValidatePathWithin(targetPath, storagePath); err != nil {
			return ReceiveTarget{}, err
		}
		return ReceiveTarget{OpenPath: filepath.Clean(targetPath)}, nil
	}
}

func ResolvePreparedVMDisk(storageBackend, diskPath, storagePath, configuredVG string) (PreparedVMDisk, error) {
	switch storageBackend {
	case "lvm":
		resolvedPath, err := ResolveLVMSourcePath(diskPath, "", "", configuredVG)
		if err != nil {
			return PreparedVMDisk{}, err
		}
		return PreparedVMDisk{LVMDiskPath: resolvedPath}, nil
	case "qcow":
		resolvedPath, err := ResolveQCOWSourcePath(diskPath, storagePath)
		if err != nil {
			return PreparedVMDisk{}, err
		}
		return PreparedVMDisk{DiskPath: resolvedPath}, nil
	default:
		return PreparedVMDisk{}, nil
	}
}

func NewReceiveTracker(total int64) (*ReceiveTracker, error) {
	if total < 0 {
		return nil, fmt.Errorf("%w: total cannot be negative", ErrTransferSize)
	}
	return &ReceiveTracker{total: total}, nil
}

func (t *ReceiveTracker) Accept(offset int64, length int) error {
	if offset < 0 {
		return fmt.Errorf("%w: offset cannot be negative", ErrInvalidOffset)
	}
	if int64(length) < 0 {
		return fmt.Errorf("%w: length cannot be negative", ErrTransferSize)
	}
	if offset != t.bytesReceived {
		return fmt.Errorf("%w: expected offset %d, got %d", ErrInvalidOffset, t.bytesReceived, offset)
	}
	if offset+int64(length) > t.total {
		return fmt.Errorf("%w: chunk ending at %d exceeds total %d", ErrTransferSize, offset+int64(length), t.total)
	}
	t.bytesReceived += int64(length)
	return nil
}

func (t *ReceiveTracker) Finalize() error {
	return ValidateTransferredBytes(t.total, t.bytesReceived)
}

func (t *ReceiveTracker) BytesReceived() int64 {
	return t.bytesReceived
}

func ValidateTransferredBytes(expected, actual int64) error {
	if expected != actual {
		return fmt.Errorf("%w: expected %d bytes, received %d", ErrTransferSize, expected, actual)
	}
	return nil
}

func ValidateLVMImageCapacity(totalBytes, sizeGB int64) error {
	if totalBytes < 0 {
		return fmt.Errorf("%w: total bytes cannot be negative", ErrTransferSize)
	}
	if sizeGB <= 0 {
		return fmt.Errorf("%w: disk_size_gb must be positive", ErrTransferSize)
	}
	if totalBytes > sizeGB*bytesPerGiB {
		return fmt.Errorf("%w: disk_size_gb %d is too small for total transfer size %d bytes", ErrTransferSize, sizeGB, totalBytes)
	}
	return nil
}

func WriteFull(writer io.Writer, data []byte) error {
	n, err := writer.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func SeekAndWriteFull(writer io.WriteSeeker, offset int64, data []byte) error {
	if _, err := writer.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	return WriteFull(writer, data)
}

func OpenLVMReceiveTarget(
	ctx context.Context,
	imageID string,
	sizeGB int,
	openPath string,
	createImage func(context.Context, string, int) error,
	openFile func(string) (*os.File, error),
	deleteImage func(context.Context, string) error,
) (*os.File, func() error, error) {
	if err := createImage(ctx, imageID, sizeGB); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrCreateImage, err)
	}

	file, err := openFile(openPath)
	if err != nil {
		cleanupCtx := context.WithoutCancel(ctx)
		cleanupErr := deleteImage(cleanupCtx, imageID)
		if cleanupErr != nil {
			return nil, nil, errors.Join(fmt.Errorf("%w: %w", ErrOpenTarget, err), cleanupErr)
		}
		return nil, nil, fmt.Errorf("%w: %w", ErrOpenTarget, err)
	}

	cleanupCtx := context.WithoutCancel(ctx)
	rollback := func() error {
		return deleteImage(cleanupCtx, imageID)
	}
	return file, rollback, nil
}

func OpenFileReceiveTarget(openPath string) (*os.File, func() error, func() error, error) {
	targetDir := filepath.Dir(openPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: creating target directory: %w", ErrOpenTarget, err)
	}

	file, err := os.CreateTemp(targetDir, filepath.Base(openPath)+".receive-*")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: creating staged target file: %w", ErrOpenTarget, err)
	}
	stagedPath := file.Name()

	rollback := func() error {
		if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	commit := func() error {
		if err := os.Rename(stagedPath, openPath); err != nil {
			if cleanupErr := rollback(); cleanupErr != nil {
				return errors.Join(err, cleanupErr)
			}
			return err
		}
		return nil
	}

	return file, commit, rollback, nil
}

func DetachedTimeoutContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func StreamProcessOutput(
	reader io.Reader,
	totalSize int64,
	send func(offset, total int64, data []byte) error,
	terminate func() error,
	wait func() error,
) (int64, error) {
	buf := make([]byte, 64*1024)
	var bytesSent int64

	waitForExit := func(baseErr error) error {
		waitErr := wait()
		if waitErr != nil {
			return errors.Join(baseErr, waitErr)
		}
		return baseErr
	}

	terminateAndWait := func(baseErr error) error {
		terminateErr := terminate()
		if terminateErr != nil {
			baseErr = errors.Join(baseErr, terminateErr)
		}
		return waitForExit(baseErr)
	}

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if sendErr := send(bytesSent, totalSize, buf[:n]); sendErr != nil {
				return bytesSent, terminateAndWait(fmt.Errorf("%w: %w", ErrSendProcess, sendErr))
			}
			bytesSent += int64(n)
		}

		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			waitErr := waitForExit(nil)
			if waitErr != nil {
				return bytesSent, fmt.Errorf("%w: %w", ErrWaitProcess, waitErr)
			}
			return bytesSent, nil
		}
		return bytesSent, terminateAndWait(fmt.Errorf("%w: %w", ErrReadProcess, err))
	}
}

// ValidatePathWithin ensures path stays within allowedPrefix even when symlinks
// and ".." traversal are involved. Missing final path components are allowed so
// callers can validate new files before creating them.
func ValidatePathWithin(path, allowedPrefix string) error {
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	cleanedPrefix := filepath.Clean(allowedPrefix)
	cleaned := filepath.Clean(path)
	if !pathWithin(cleaned, cleanedPrefix) {
		return fmt.Errorf("path %q is outside the allowed directory %q", cleaned, allowedPrefix)
	}
	if !rawPathWithin(path, cleanedPrefix) {
		return fmt.Errorf("path %q is outside the allowed directory %q", cleaned, allowedPrefix)
	}
	resolvedPrefix, err := filepath.EvalSymlinks(cleanedPrefix)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("resolving allowed directory %q: %w", allowedPrefix, err)
		}
		resolvedPrefix = cleanedPrefix
	}
	if err := validateSymlinkBoundary(path, cleanedPrefix, resolvedPrefix); err != nil {
		return err
	}
	return nil
}

func validateSymlinkBoundary(path, cleanedPrefix, resolvedPrefix string) error {
	relPath := strings.TrimPrefix(path, cleanedPrefix)
	if relPath == "" {
		return nil
	}
	relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
	currentPath := resolvedPrefix
	for _, segment := range strings.Split(relPath, string(filepath.Separator)) {
		switch segment {
		case "", ".":
			continue
		case "..":
			currentPath = filepath.Dir(currentPath)
			if !pathWithin(filepath.Clean(currentPath), resolvedPrefix) {
				return fmt.Errorf("path %q resolves outside the allowed directory %q", filepath.Clean(path), cleanedPrefix)
			}
			continue
		}

		currentPath = filepath.Join(currentPath, segment)
		info, err := os.Lstat(currentPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspecting path %q: %w", currentPath, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}

		resolvedPath, err := filepath.EvalSymlinks(currentPath)
		if err != nil {
			return fmt.Errorf("resolving path %q: %w", currentPath, err)
		}
		if !pathWithin(filepath.Clean(resolvedPath), resolvedPrefix) {
			return fmt.Errorf("path %q resolves outside the allowed directory %q", path, cleanedPrefix)
		}
		currentPath = resolvedPath
	}
	return nil
}

func rawPathWithin(path, prefix string) bool {
	if path == prefix {
		return true
	}
	if prefix == string(filepath.Separator) {
		return strings.HasPrefix(path, prefix)
	}
	return strings.HasPrefix(path, prefix+string(filepath.Separator))
}

func pathWithin(path, prefix string) bool {
	relPath, err := filepath.Rel(prefix, path)
	if err != nil {
		return false
	}
	return relPath == "." || (relPath != ".." && !strings.HasPrefix(relPath, ".."+string(filepath.Separator)))
}

func validateVG(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if !validLVMName.MatchString(name) {
		return fmt.Errorf("invalid LVM name %q", name)
	}
	return nil
}

func validateLV(name string) error {
	if name == "" {
		return fmt.Errorf("logical volume name must not be empty")
	}
	if strings.Contains(name, "..") || !validLVName.MatchString(name) {
		return fmt.Errorf("invalid logical volume name %q", name)
	}
	return nil
}

func splitLVMPath(path string) (string, string, error) {
	cleaned := filepath.Clean(path)
	parts := strings.Split(cleaned, string(filepath.Separator))
	if len(parts) != 4 || parts[0] != "" || parts[1] != "dev" {
		return "", "", fmt.Errorf("invalid LVM device path %q", path)
	}
	vg := parts[2]
	lv := parts[3]
	if err := validateVG(vg); err != nil {
		return "", "", err
	}
	if err := validateLV(lv); err != nil {
		return "", "", err
	}
	return vg, lv, nil
}

func canonicalLVMPath(vg, lv string) string {
	return filepath.Join(string(filepath.Separator), "dev", vg, lv)
}
