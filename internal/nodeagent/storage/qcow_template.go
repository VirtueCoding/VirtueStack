// Package storage provides file-based QCOW2 template management for VM provisioning.
// Templates are stored as qcow2 files with copy-on-write cloning for efficient
// VM disk creation.
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
	"time"
)

const (
	// qemuImgTimeout is the timeout for qemu-img operations.
	qemuImgTimeout = 60 * time.Second
	// qcow2Extension is the file extension for QCOW2 images.
	qcow2Extension = ".qcow2"
)

// QCOWTemplateInfo holds metadata about a QCOW2 template image.
type QCOWTemplateInfo struct {
	// Name is the template name (e.g., "ubuntu-2204").
	Name string
	// Path is the full path to the template file.
	Path string
	// SizeBytes is the virtual size of the template in bytes.
	SizeBytes int64
	// CreatedAt is the file modification time.
	CreatedAt time.Time
}

// QCOWTemplateManager handles OS template import and management for file-based
// QCOW2 storage. Templates are stored as qcow2 files and cloned using copy-on-write
// for instant VM disk creation.
//
// The manager is safe for concurrent use.
type QCOWTemplateManager struct {
	// templatesPath is the directory for template storage.
	templatesPath string
	// vmsPath is the directory for VM disk images.
	vmsPath string
	// logger is the structured logger.
	logger *slog.Logger
}

// NewQCOWTemplateManager creates a new QCOWTemplateManager.
// It creates the templates and VMs directories if they don't exist and validates
// that the paths are writable.
//
// Parameters:
//   - templatesPath: Directory for template storage (e.g., /var/lib/virtuestack/templates)
//   - vmsPath: Directory for VM disk images (e.g., /var/lib/virtuestack/vms)
//   - logger: Structured logger for operation logging
func NewQCOWTemplateManager(templatesPath, vmsPath string, logger *slog.Logger) (*QCOWTemplateManager, error) {
	if templatesPath == "" {
		return nil, fmt.Errorf("templates path cannot be empty")
	}
	if vmsPath == "" {
		return nil, fmt.Errorf("vms path cannot be empty")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	m := &QCOWTemplateManager{
		templatesPath: templatesPath,
		vmsPath:       vmsPath,
		logger:        logger.With("component", "qcow-template-manager"),
	}

	// Create directories if they don't exist
	if err := m.ensureDirectory(templatesPath); err != nil {
		return nil, fmt.Errorf("creating templates directory %s: %w", templatesPath, err)
	}
	if err := m.ensureDirectory(vmsPath); err != nil {
		return nil, fmt.Errorf("creating vms directory %s: %w", vmsPath, err)
	}

	// Validate paths are writable
	if err := m.validateWritable(templatesPath); err != nil {
		return nil, fmt.Errorf("templates path %s not writable: %w", templatesPath, err)
	}
	if err := m.validateWritable(vmsPath); err != nil {
		return nil, fmt.Errorf("vms path %s not writable: %w", vmsPath, err)
	}

	m.logger.Info("QCOW template manager initialized",
		"templates_path", templatesPath,
		"vms_path", vmsPath)

	return m, nil
}

// ImportTemplate imports a qcow2 template image to the templates directory.
// The source file is copied to templatesPath/{name}.qcow2 and verified with qemu-img check.
//
// Parameters:
//   - ctx: Context for cancellation
//   - name: Template name (e.g., "ubuntu-2204")
//   - sourcePath: Path to the source qcow2 file
//
// Returns the file path and size in bytes of the imported template.
func (m *QCOWTemplateManager) ImportTemplate(ctx context.Context, name, sourcePath string) (filePath string, sizeBytes int64, err error) {
	logger := m.logger.With("template", name, "source", sourcePath)
	logger.Info("importing template")

	// Validate source file exists
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("importing template %s: source file not found: %w", name, err)
	}

	// Validate source is a regular file
	if info.IsDir() {
		return "", 0, fmt.Errorf("importing template %s: source path is a directory, expected file", name)
	}

	// Generate target path
	targetPath := m.templatePath(name)

	// Check if template already exists
	if _, err := os.Stat(targetPath); err == nil {
		return "", 0, fmt.Errorf("importing template %s: template already exists at %s", name, targetPath)
	}

	// Create a context with timeout for qemu-img operations
	ctx, cancel := context.WithTimeout(ctx, qemuImgTimeout)
	defer cancel()

	// Copy the source file to templates directory
	if err := m.copyFile(sourcePath, targetPath); err != nil {
		return "", 0, fmt.Errorf("importing template %s: copying file: %w", name, err)
	}

	// Verify the copied image
	if err := m.verifyImage(ctx, targetPath); err != nil {
		// Cleanup on verification failure
		os.Remove(targetPath)
		return "", 0, fmt.Errorf("importing template %s: verification failed: %w", name, err)
	}

	// Get the size of the imported template
	size, err := m.GetTemplateSize(ctx, targetPath)
	if err != nil {
		// Cleanup on size check failure
		os.Remove(targetPath)
		return "", 0, fmt.Errorf("importing template %s: getting size: %w", name, err)
	}

	logger.Info("template imported successfully", "path", targetPath, "size_bytes", size)
	return targetPath, size, nil
}

// DeleteTemplate removes a template file from the templates directory.
//
// Parameters:
//   - ctx: Context for cancellation
//   - filePath: Full path to the template file
func (m *QCOWTemplateManager) DeleteTemplate(ctx context.Context, filePath string) error {
	logger := m.logger.With("path", filePath)
	logger.Info("deleting template")

	// Check file exists
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("deleting template: file not found: %s", filePath)
		}
		return fmt.Errorf("deleting template: checking file: %w", err)
	}

	// Remove the file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("deleting template %s: %w", filePath, err)
	}

	logger.Info("template deleted successfully")
	return nil
}

// GetTemplateSize returns the virtual size of a template in bytes using qemu-img info.
//
// Parameters:
//   - ctx: Context for cancellation
//   - filePath: Full path to the template file
func (m *QCOWTemplateManager) GetTemplateSize(ctx context.Context, filePath string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, qemuImgTimeout)
	defer cancel()

	size, err := m.getQemuImageSize(ctx, filePath)
	if err != nil {
		return 0, fmt.Errorf("getting template size for %s: %w", filePath, err)
	}
	return size, nil
}

// CloneForVM clones a template to create a new VM disk using copy-on-write.
// The clone is created as a thin overlay on top of the template.
//
// Parameters:
//   - ctx: Context for cancellation
//   - templatePath: Full path to the template file
//   - vmID: VM identifier for naming the disk
//   - sizeGB: Target disk size in GB (will resize if larger than template)
//
// Returns the path to the created VM disk.
func (m *QCOWTemplateManager) CloneForVM(ctx context.Context, templatePath, vmID string, sizeGB int) (vmDiskPath string, err error) {
	logger := m.logger.With("template", templatePath, "vm_id", vmID, "size_gb", sizeGB)
	logger.Info("cloning template for VM")

	// Validate template exists
	if _, err := os.Stat(templatePath); err != nil {
		return "", fmt.Errorf("cloning template for VM %s: template not found: %w", vmID, err)
	}

	// Create VM disk path
	vmDiskPath = filepath.Join(m.vmsPath, fmt.Sprintf("%s-disk0%s", vmID, qcow2Extension))

	// Check if VM disk already exists
	if _, err := os.Stat(vmDiskPath); err == nil {
		return "", fmt.Errorf("cloning template for VM %s: disk already exists at %s", vmID, vmDiskPath)
	}

	// Create a context with timeout for qemu-img operations
	ctx, cancel := context.WithTimeout(ctx, qemuImgTimeout)
	defer cancel()

	// Create copy-on-write overlay using qemu-img create
	if err := m.createOverlay(ctx, templatePath, vmDiskPath); err != nil {
		return "", fmt.Errorf("cloning template for VM %s: creating overlay: %w", vmID, err)
	}

	// Resize if needed (VM disk is typically larger than template)
	if err := m.resizeImage(ctx, vmDiskPath, sizeGB); err != nil {
		// Cleanup on resize failure
		os.Remove(vmDiskPath)
		return "", fmt.Errorf("cloning template for VM %s: resizing: %w", vmID, err)
	}

	logger.Info("template cloned successfully", "vm_disk_path", vmDiskPath)
	return vmDiskPath, nil
}

// TemplateExists checks if a template file exists.
//
// Parameters:
//   - ctx: Context for cancellation
//   - filePath: Full path to the template file
func (m *QCOWTemplateManager) TemplateExists(ctx context.Context, filePath string) (bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking template existence for %s: %w", filePath, err)
	}
	return !info.IsDir(), nil
}

// ListTemplates lists all available templates in the templates directory.
// Returns template info for all .qcow2 files.
//
// Parameters:
//   - ctx: Context for cancellation
func (m *QCOWTemplateManager) ListTemplates(ctx context.Context) ([]QCOWTemplateInfo, error) {
	entries, err := os.ReadDir(m.templatesPath)
	if err != nil {
		return nil, fmt.Errorf("listing templates in %s: %w", m.templatesPath, err)
	}

	var templates []QCOWTemplateInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only include .qcow2 files
		if !strings.HasSuffix(entry.Name(), qcow2Extension) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			m.logger.Warn("failed to get file info", "file", entry.Name(), "error", err)
			continue
		}

		fullPath := filepath.Join(m.templatesPath, entry.Name())
		templateName := strings.TrimSuffix(entry.Name(), qcow2Extension)

		// Get virtual size using qemu-img
		ctx, cancel := context.WithTimeout(ctx, qemuImgTimeout)
		size, err := m.getQemuImageSize(ctx, fullPath)
		cancel()
		if err != nil {
			m.logger.Warn("failed to get template size", "template", templateName, "error", err)
			size = info.Size() // Fall back to file size
		}

		templates = append(templates, QCOWTemplateInfo{
			Name:      templateName,
			Path:      fullPath,
			SizeBytes: size,
			CreatedAt: info.ModTime(),
		})
	}

	return templates, nil
}

// Helper methods

// templatePath returns the full path for a template with the given name.
func (m *QCOWTemplateManager) templatePath(name string) string {
	return filepath.Join(m.templatesPath, name+qcow2Extension)
}

// ensureDirectory creates a directory if it doesn't exist.
func (m *QCOWTemplateManager) ensureDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// validateWritable checks if a directory is writable by attempting to create a temp file.
func (m *QCOWTemplateManager) validateWritable(path string) error {
	testFile := filepath.Join(path, ".write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// copyFile copies a file from src to dst.
func (m *QCOWTemplateManager) copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading source file: %w", err)
	}

	if err := os.WriteFile(dst, input, 0644); err != nil {
		return fmt.Errorf("writing destination file: %w", err)
	}

	return nil
}

// verifyImage verifies a qcow2 image using qemu-img check.
func (m *QCOWTemplateManager) verifyImage(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "qemu-img", "check", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-img check failed for %s: %w (output: %s)", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// createOverlay creates a copy-on-write overlay image backed by the template.
func (m *QCOWTemplateManager) createOverlay(ctx context.Context, templatePath, overlayPath string) error {
	cmd := exec.CommandContext(ctx,
		"qemu-img", "create",
		"-b", templatePath,
		"-F", "qcow2",
		"-f", "qcow2",
		overlayPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating overlay %s: %w (output: %s)", overlayPath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resizeImage resizes a qcow2 image to the specified size in GB.
func (m *QCOWTemplateManager) resizeImage(ctx context.Context, path string, sizeGB int) error {
	cmd := exec.CommandContext(ctx,
		"qemu-img", "resize",
		path,
		strconv.Itoa(sizeGB)+"G",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("resizing image %s to %dG: %w (output: %s)", path, sizeGB, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// getQemuImageSize returns the virtual size of a qcow2 image using qemu-img info.
func (m *QCOWTemplateManager) getQemuImageSize(ctx context.Context, path string) (int64, error) {
	cmd := exec.CommandContext(ctx,
		"qemu-img", "info",
		"--output=json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return 0, fmt.Errorf("qemu-img info failed for %s: %w (stderr: %s)", path, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return 0, fmt.Errorf("qemu-img info failed for %s: %w", path, err)
	}

	var info struct {
		VirtualSize int64 `json:"virtual-size"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return 0, fmt.Errorf("parsing qemu-img info output for %s: %w", path, err)
	}

	return info.VirtualSize, nil
}
