// Package storage provides Ceph RBD operations, cloud-init ISO generation,
// and OS template management for the VirtueStack Node Agent.
package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
)

// templateStepTimeout is the per-step timeout applied to long-running template
// operations (qemu-img convert, rbd import). Large templates can be several GiB
// so a generous timeout is required, but we still want a bound to avoid hanging
// indefinitely.
const templateStepTimeout = 5 * time.Minute

const (
	// TemplatePool is the default Ceph pool for template base images.
	// Override with the CEPH_TEMPLATE_POOL environment variable.
	TemplatePool = "vs-images"
	// VMPool is the default Ceph pool for VM disk images.
	// Override with the CEPH_VM_POOL environment variable (maps to NodeAgentConfig.CephPool).
	VMPool = "vs-vms"
	// TemplateImageSuffix is the suffix for template base images.
	TemplateImageSuffix = "-base"
	// TemplateSnapshotSuffix is the suffix for template snapshots.
	TemplateSnapshotSuffix = "-snap"
)

// TemplateManager handles OS template import and management for VM provisioning.
// Templates are stored as RBD images in the vs-images pool with protected snapshots
// for efficient copy-on-write cloning.
type TemplateManager struct {
	rbdManager *RBDManager
	conn       *rados.Conn
	logger     *slog.Logger
}

// NewTemplateManager creates a new TemplateManager.
// It requires an RBDManager for cloning operations and a rados connection
// for template import operations.
func NewTemplateManager(rbdManager *RBDManager, conn *rados.Conn, logger *slog.Logger) *TemplateManager {
	return &TemplateManager{
		rbdManager: rbdManager,
		conn:       conn,
		logger:     logger.With("component", "template-manager"),
	}
}

// ImportTemplate imports a qcow2 template image into RBD storage.
// The import flow:
//  1. Convert qcow2 to raw format using qemu-img
//  2. Import raw image to RBD as vs-images/<ref>-base
//  3. Create snapshot <ref>-base@<ref>-snap
//  4. Protect snapshot for cloning
//
// Parameters:
//   - ref: Template name/identifier (e.g., "ubuntu-2204")
//   - sourcePath: Path to the qcow2 source file
//   - meta: Optional metadata (OS family, version)
//
// Returns the RBD image reference and size in bytes.
func (m *TemplateManager) ImportTemplate(ctx context.Context, ref, sourcePath string, meta TemplateMeta) (filePath string, sizeBytes int64, err error) {
	logger := m.logger.With("template", ref, "source", sourcePath)
	logger.Info("importing template")

	// Validate source file exists
	if _, err := os.Stat(sourcePath); err != nil {
		return "", 0, fmt.Errorf("importing template %s: source file not found: %w", ref, err)
	}

	// Create temporary directory for conversion
	tmpDir, err := os.MkdirTemp("", "template-import-"+ref+"-*")
	if err != nil {
		return "", 0, fmt.Errorf("importing template %s: creating temp dir: %w", ref, err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			logger.Warn("failed to remove temp directory during cleanup", "path", tmpDir, "error", err)
		}
	}()

	// Convert qcow2 to raw
	rawPath := filepath.Join(tmpDir, ref+".raw")
	if err := m.convertToRaw(ctx, sourcePath, rawPath, logger); err != nil {
		return "", 0, fmt.Errorf("importing template %s: %w", ref, err)
	}

	// Import raw to RBD
	imageName := ref + TemplateImageSuffix
	if err := m.importRawToRBD(ctx, rawPath, imageName, logger); err != nil {
		return "", 0, fmt.Errorf("importing template %s: %w", ref, err)
	}

	// Create snapshot
	snapName := ref + TemplateSnapshotSuffix
	if err := m.createTemplateSnapshot(ctx, imageName, snapName, logger); err != nil {
		// Cleanup: remove imported image on failure
		_ = m.removeImage(TemplatePool, imageName)
		return "", 0, fmt.Errorf("importing template %s: %w", ref, err)
	}

	// Protect snapshot (required for cloning)
	if err := m.protectTemplateSnapshot(ctx, imageName, snapName, logger); err != nil {
		// Cleanup: remove snapshot and image on failure
		_ = m.removeSnapshot(TemplatePool, imageName, snapName)
		_ = m.removeImage(TemplatePool, imageName)
		return "", 0, fmt.Errorf("importing template %s: %w", ref, err)
	}

	// Get the size of the imported template
	size, err := m.getImageSize(TemplatePool, imageName)
	if err != nil {
		logger.Warn("failed to get template size after import", "error", err)
		size = 0
	}

	// Return the RBD image reference (pool/image format)
	rbdRef := TemplatePool + "/" + imageName
	logger.Info("template imported successfully", "image", imageName, "snapshot", snapName, "size", size)
	return rbdRef, size, nil
}

// CloneForVM clones a template snapshot to create a new VM disk.
// This is an instant copy-on-write operation.
//
// Parameters:
//   - templateRef: Template name (e.g., "ubuntu-2204")
//   - vmID: VM identifier for the disk
//   - sizeGB: Target disk size in GB (will resize if larger than template)
//
// Returns the RBD image name for the VM disk.
func (m *TemplateManager) CloneForVM(ctx context.Context, templateRef, vmID string, sizeGB int) (string, error) {
	logger := m.logger.With("template", templateRef, "vm", vmID, "size_gb", sizeGB)
	logger.Info("cloning template for VM")

	sourceImage := templateRef + TemplateImageSuffix
	snapName := templateRef + TemplateSnapshotSuffix
	targetImage := fmt.Sprintf(VMDiskNameFmt, vmID)

	// Clone from template snapshot
	if err := m.rbdManager.CloneFromTemplate(ctx, TemplatePool, sourceImage, snapName, targetImage); err != nil {
		return "", fmt.Errorf("cloning template %s for VM %s: %w", templateRef, vmID, err)
	}

	// Resize if needed (VM disk is typically larger than template)
	if err := m.rbdManager.Resize(ctx, targetImage, sizeGB); err != nil {
		// Cleanup: remove cloned image on failure
		_ = m.rbdManager.Delete(ctx, targetImage)
		return "", fmt.Errorf("resizing cloned disk for VM %s: %w", vmID, err)
	}

	logger.Info("template cloned successfully", "target", targetImage)
	return targetImage, nil
}

// DeleteTemplate removes a template from RBD storage.
// This unprotects and removes the snapshot, then removes the base image.
func (m *TemplateManager) DeleteTemplate(ctx context.Context, templateName string) error {
	logger := m.logger.With("template", templateName)
	logger.Info("deleting template")

	imageName := templateName + TemplateImageSuffix
	snapName := templateName + TemplateSnapshotSuffix

	// Unprotect snapshot
	if err := m.unprotectTemplateSnapshot(ctx, imageName, snapName, logger); err != nil {
		return fmt.Errorf("deleting template %s: %w", templateName, err)
	}

	// Remove snapshot
	if err := m.removeSnapshot(TemplatePool, imageName, snapName); err != nil {
		return fmt.Errorf("deleting template %s: %w", templateName, err)
	}

	// Remove base image
	if err := m.removeImage(TemplatePool, imageName); err != nil {
		return fmt.Errorf("deleting template %s: %w", templateName, err)
	}

	logger.Info("template deleted successfully")
	return nil
}

// TemplateExists checks if a template exists in RBD storage.
func (m *TemplateManager) TemplateExists(ctx context.Context, templateName string) (bool, error) {
	imageName := templateName + TemplateImageSuffix
	return m.imageExists(TemplatePool, imageName)
}

// GetTemplateSize returns the size of a template in bytes.
// ref is the template name (without suffix).
func (m *TemplateManager) GetTemplateSize(ctx context.Context, ref string) (int64, error) {
	imageName := ref + TemplateImageSuffix
	size, err := m.getImageSize(TemplatePool, imageName)
	if err != nil {
		return 0, fmt.Errorf("getting template %s size: %w", ref, err)
	}
	return size, nil
}

// ListTemplates lists all available templates in RBD storage.
// Returns template info for all images matching the template naming convention.
func (m *TemplateManager) ListTemplates(ctx context.Context) ([]TemplateInfo, error) {
	ioctx, err := m.openIOContext(TemplatePool)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer ioctx.Destroy()

	names, err := rbd.GetImageNames(ioctx)
	if err != nil {
		return nil, fmt.Errorf("listing images in pool %s: %w", TemplatePool, err)
	}

	var templates []TemplateInfo
	for _, name := range names {
		// Only include images with the template suffix
		if !m.isTemplateImage(name) {
			continue
		}

		templateName := m.stripTemplateSuffix(name)
		size, err := m.getImageSize(TemplatePool, name)
		if err != nil {
			m.logger.Warn("failed to get template size", "template", templateName, "error", err)
			continue
		}

		templates = append(templates, TemplateInfo{
			Name:      templateName,
			FilePath:  TemplatePool + "/" + name,
			SizeBytes: size,
			CreatedAt: time.Time{}, // No reliable way to get creation time from RBD
		})
	}

	return templates, nil
}

// Helper methods

// convertToRaw converts a qcow2 image to raw format using qemu-img.
// A per-step timeout (templateStepTimeout) is applied independently of the parent
// context so that large templates do not stall indefinitely.
func (m *TemplateManager) convertToRaw(ctx context.Context, sourcePath, rawPath string, logger *slog.Logger) error {
	logger.Info("converting qcow2 to raw", "source", sourcePath, "dest", rawPath)

	stepCtx, cancel := context.WithTimeout(ctx, templateStepTimeout)
	defer cancel()

	cmd := exec.CommandContext(stepCtx,
		"qemu-img",
		"convert",
		"-f", "qcow2",
		"-O", "raw",
		sourcePath,
		rawPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img convert failed: %w (output: %s)", err, string(out))
	}

	logger.Info("qcow2 converted to raw successfully")
	return nil
}

// importRawToRBD imports a raw image file into RBD using the rbd CLI.
// A per-step timeout (templateStepTimeout) is applied independently of the parent
// context so that large images do not stall indefinitely.
func (m *TemplateManager) importRawToRBD(ctx context.Context, rawPath, imageName string, logger *slog.Logger) error {
	logger.Info("importing raw image to RBD", "path", rawPath, "image", imageName)

	stepCtx, cancel := context.WithTimeout(ctx, templateStepTimeout)
	defer cancel()

	cmd := exec.CommandContext(stepCtx, "rbd", "import", rawPath, TemplatePool+"/"+imageName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("importing image %s: %w (output: %s)", imageName, err, string(out))
	}

	logger.Info("raw image imported to RBD successfully")
	return nil
}

// createTemplateSnapshot creates a snapshot of the template base image.
func (m *TemplateManager) createTemplateSnapshot(ctx context.Context, imageName, snapName string, logger *slog.Logger) error {
	logger.Info("creating template snapshot", "image", imageName, "snapshot", snapName)

	ioctx, err := m.openIOContext(TemplatePool)
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

	logger.Info("template snapshot created successfully")
	return nil
}

// protectTemplateSnapshot protects a snapshot to enable cloning.
func (m *TemplateManager) protectTemplateSnapshot(ctx context.Context, imageName, snapName string, logger *slog.Logger) error {
	logger.Info("protecting template snapshot", "image", imageName, "snapshot", snapName)

	ioctx, err := m.openIOContext(TemplatePool)
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

	logger.Info("template snapshot protected successfully")
	return nil
}

// unprotectTemplateSnapshot unprotects a snapshot so it can be deleted.
func (m *TemplateManager) unprotectTemplateSnapshot(ctx context.Context, imageName, snapName string, logger *slog.Logger) error {
	logger.Info("unprotecting template snapshot", "image", imageName, "snapshot", snapName)

	ioctx, err := m.openIOContext(TemplatePool)
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

	logger.Info("template snapshot unprotected successfully")
	return nil
}

// openIOContext opens an IO context for the specified pool.
func (m *TemplateManager) openIOContext(pool string) (*rados.IOContext, error) {
	ioctx, err := m.conn.OpenIOContext(pool)
	if err != nil {
		return nil, fmt.Errorf("opening IO context for pool %s: %w", pool, err)
	}
	return ioctx, nil
}

// removeImage removes an RBD image from a pool.
func (m *TemplateManager) removeImage(pool, imageName string) error {
	ioctx, err := m.openIOContext(pool)
	if err != nil {
		return fmt.Errorf("removing image %s/%s: %w", pool, imageName, err)
	}
	defer ioctx.Destroy()

	if err := rbd.RemoveImage(ioctx, imageName); err != nil {
		return fmt.Errorf("removing image %s/%s: %w", pool, imageName, err)
	}
	return nil
}

// removeSnapshot removes a snapshot from an RBD image.
func (m *TemplateManager) removeSnapshot(pool, imageName, snapName string) error {
	ioctx, err := m.openIOContext(pool)
	if err != nil {
		return fmt.Errorf("removing snapshot %s/%s@%s: %w", pool, imageName, snapName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("opening image %s for snapshot removal: %w", imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			m.logger.Warn("failed to close RBD image after snapshot removal", "image", imageName, "error", err)
		}
	}()

	snapObj := img.GetSnapshot(snapName)
	if err := snapObj.Remove(); err != nil {
		return fmt.Errorf("removing snapshot %s/%s@%s: %w", pool, imageName, snapName, err)
	}
	return nil
}

// imageExists checks if an RBD image exists in a pool.
func (m *TemplateManager) imageExists(pool, imageName string) (bool, error) {
	ioctx, err := m.openIOContext(pool)
	if err != nil {
		return false, fmt.Errorf("checking existence of image %s/%s: %w", pool, imageName, err)
	}
	defer ioctx.Destroy()

	names, err := rbd.GetImageNames(ioctx)
	if err != nil {
		return false, fmt.Errorf("listing images in pool %s: %w", pool, err)
	}

	for _, name := range names {
		if name == imageName {
			return true, nil
		}
	}

	return false, nil
}

// getImageSize returns the size of an RBD image in bytes.
func (m *TemplateManager) getImageSize(pool, imageName string) (int64, error) {
	ioctx, err := m.openIOContext(pool)
	if err != nil {
		return 0, fmt.Errorf("getting size of image %s/%s: %w", pool, imageName, err)
	}
	defer ioctx.Destroy()

	img, err := rbd.OpenImage(ioctx, imageName, rbd.NoSnapshot)
	if err != nil {
		return 0, fmt.Errorf("opening image %s/%s to get size: %w", pool, imageName, err)
	}
	defer func() {
		if err := img.Close(); err != nil {
			m.logger.Warn("failed to close RBD image after getting size", "image", imageName, "error", err)
		}
	}()

	size, err := img.GetSize()
	if err != nil {
		return 0, fmt.Errorf("getting size of image %s/%s: %w", pool, imageName, err)
	}

	return int64(size), nil
}

// isTemplateImage checks if an image name follows the template naming convention.
func (m *TemplateManager) isTemplateImage(name string) bool {
	return len(name) > len(TemplateImageSuffix) &&
		name[len(name)-len(TemplateImageSuffix):] == TemplateImageSuffix
}

// stripTemplateSuffix removes the template suffix from an image name.
func (m *TemplateManager) stripTemplateSuffix(name string) string {
	if m.isTemplateImage(name) {
		return name[:len(name)-len(TemplateImageSuffix)]
	}
	return name
}
