// Package storage provides LVM thin-provisioned template management for VM provisioning.
// Templates are stored as thin logical volumes with copy-on-write cloning for efficient
// VM disk creation.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// lvmTemplateStepTimeout is the per-step timeout for long-running template operations.
// Large templates (multi-GB) require generous timeouts for qemu-img convert and dd operations.
const lvmTemplateStepTimeout = 30 * time.Minute

// lvmTemplateSuffix is the suffix for template base logical volumes.
const lvmTemplateSuffix = "-base"

// validateLVMLVName validates an LVM identifier (vmID, template name, etc.) for use in lvm commands.
// It reuses the validation function from LVMManager via a package-level check.
func validateLVMLVName(id string) error {
	if id == "" {
		return fmt.Errorf("LVM identifier cannot be empty")
	}
	if !validLVMLVName.MatchString(id) {
		return fmt.Errorf("invalid LVM identifier %q: must match %s", id, validLVMLVName.String())
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("invalid LVM identifier %q: must not contain path traversal", id)
	}
	return nil
}

// NormalizeLVMTemplateRef accepts either a bare template ref or the canonical
// /dev/{vg}/{lv} device path and returns the normalized LV name.
func NormalizeLVMTemplateRef(vgName, ref string) (string, error) {
	if !strings.HasPrefix(ref, "/") {
		lvName := ref
		if !strings.HasSuffix(lvName, lvmTemplateSuffix) {
			lvName += lvmTemplateSuffix
		}
		if err := validateLVMLVName(lvName); err != nil {
			return "", fmt.Errorf("validating template ref: %w", err)
		}
		return lvName, nil
	}

	expectedPrefix := fmt.Sprintf("/dev/%s/", vgName)
	if !strings.HasPrefix(ref, expectedPrefix) {
		return "", fmt.Errorf("validating template ref: expected volume group %q in %q", vgName, ref)
	}

	lvName := strings.TrimPrefix(ref, expectedPrefix)
	if lvName == "" {
		return "", fmt.Errorf("validating template ref: missing logical volume name in %q", ref)
	}
	if err := validateLVMLVName(lvName); err != nil {
		return "", fmt.Errorf("validating template ref: %w", err)
	}
	return lvName, nil
}

// CanonicalLVMTemplatePath returns the canonical device path for a template ref.
func CanonicalLVMTemplatePath(vgName, ref string) (string, error) {
	lvName, err := NormalizeLVMTemplateRef(vgName, ref)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/%s/%s", vgName, lvName), nil
}

// LVMTemplateManager handles OS template import and management for LVM thin-provisioned
// storage. Templates are stored as thin logical volumes and cloned using thin snapshots
// for instant copy-on-write VM disk creation.
//
// Template LVs follow the naming convention {name}-base and live in the configured
// thin pool. VM disks are created as thin snapshots of templates.
//
// The manager is safe for concurrent use.
type LVMTemplateManager struct {
	// vgName is the volume group name (e.g., "vgvs").
	vgName string
	// thinPool is the thin pool LV name (e.g., "thinpool").
	thinPool string
	// logger is the structured logger.
	logger *slog.Logger
}

// NewLVMTemplateManager creates a new LVMTemplateManager.
//
// Parameters:
//   - vgName: Volume group name (e.g., "vgvs")
//   - thinPool: Thin pool logical volume name (e.g., "thinpool")
//   - logger: Structured logger for operation logging
func NewLVMTemplateManager(vgName, thinPool string, logger *slog.Logger) (*LVMTemplateManager, error) {
	if vgName == "" {
		return nil, fmt.Errorf("vgName cannot be empty")
	}
	if thinPool == "" {
		return nil, fmt.Errorf("thinPool cannot be empty")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	m := &LVMTemplateManager{
		vgName:   vgName,
		thinPool: thinPool,
		logger:   logger.With("component", "lvm-template-manager"),
	}

	m.logger.Info("LVM template manager initialized",
		"vg", vgName,
		"thin_pool", thinPool)

	return m, nil
}

// ImportTemplate imports a qcow2 template image into LVM thin storage.
// The import flow:
//  1. Get virtual size from qcow2 image using qemu-img info
//  2. Convert qcow2 to raw format using qemu-img convert
//  3. Create a thin LV with matching virtual size
//  4. Write raw data to the LV using dd
//
// Parameters:
//   - ctx: Context for cancellation
//   - ref: Template name/identifier (e.g., "ubuntu-2204")
//   - sourcePath: Path to the qcow2 source file
//   - meta: Optional metadata (OS family, version)
//
// Returns the LV path and size in bytes.
func (m *LVMTemplateManager) ImportTemplate(ctx context.Context, ref, sourcePath string, meta TemplateMeta) (filePath string, sizeBytes int64, err error) {
	logger := m.logger.With("template", ref, "source", sourcePath, "os_family", meta.OSFamily, "os_version", meta.OSVersion)
	logger.Info("importing template")

	// Validate source file exists
	if _, err := os.Stat(sourcePath); err != nil {
		return "", 0, fmt.Errorf("importing template %s: source file not found: %w", ref, err)
	}

	// Step 1: Get virtual size from qcow2 image
	virtualSize, err := m.getQemuVirtualSize(ctx, sourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("importing template %s: getting virtual size: %w", ref, err)
	}
	logger.Info("got virtual size from qcow2", "virtual_size_bytes", virtualSize)

	// Create temporary directory for conversion
	tmpDir, err := os.MkdirTemp("", "lvm-template-import-"+ref+"-*")
	if err != nil {
		return "", 0, fmt.Errorf("importing template %s: creating temp dir: %w", ref, err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			logger.Warn("failed to remove temp directory during cleanup", "path", tmpDir, "error", rmErr)
		}
	}()

	// Step 2: Convert qcow2 to raw with independent timeout
	rawPath := filepath.Join(tmpDir, ref+".raw")
	if err := m.convertToRaw(ctx, sourcePath, rawPath, logger); err != nil {
		return "", 0, fmt.Errorf("importing template %s: %w", ref, err)
	}

	// LV name follows the convention {name}-base
	lvName := ref + lvmTemplateSuffix
	lvPath := fmt.Sprintf("/dev/%s/%s", m.vgName, lvName)

	// Step 3: Create thin LV with virtual size matching the image
	if err := m.createThinLV(ctx, lvName, virtualSize, logger); err != nil {
		return "", 0, fmt.Errorf("importing template %s: %w", ref, err)
	}

	// Step 4: Write raw data to LV using dd with independent timeout
	if err := m.writeRawToLV(ctx, rawPath, lvPath, logger); err != nil {
		// Cleanup: remove the created LV on failure
		if rmErr := m.removeLV(lvName, logger); rmErr != nil {
			logger.Warn("failed to cleanup LV after dd failure", "lv", lvName, "error", rmErr)
		}
		return "", 0, fmt.Errorf("importing template %s: %w", ref, err)
	}

	logger.Info("template imported successfully", "lv_path", lvPath, "template_ref", ref, "size_bytes", virtualSize)
	return lvPath, virtualSize, nil
}

// DeleteTemplate removes a template from LVM thin storage.
// It checks for existing thin-snapshot dependents and refuses deletion if any exist,
// as deleting the origin of a thin snapshot causes data corruption.
//
// Parameters:
//   - ctx: Context for cancellation
//   - ref: Template LV name (e.g., "ubuntu-2204-base") or template name (e.g., "ubuntu-2204")
func (m *LVMTemplateManager) DeleteTemplate(ctx context.Context, ref string) error {
	// Normalize ref to include -base suffix if not present
	lvName := ref
	if !strings.HasSuffix(ref, lvmTemplateSuffix) {
		lvName = ref + lvmTemplateSuffix
	}

	logger := m.logger.With("template", lvName)
	logger.Info("deleting template")

	// Check for dependents (thin snapshots that have this LV as origin)
	hasDependents, err := m.hasDependents(lvName)
	if err != nil {
		return fmt.Errorf("deleting template %s: checking dependents: %w", lvName, err)
	}
	if hasDependents {
		return fmt.Errorf("deleting template %s: cannot delete: template has dependent VM disks (thin snapshots)", lvName)
	}

	// Remove the LV
	if err := m.removeLV(lvName, logger); err != nil {
		return fmt.Errorf("deleting template %s: %w", lvName, err)
	}

	logger.Info("template deleted successfully")
	return nil
}

// CloneForVM clones a template to create a new VM disk using thin snapshot.
// This is an instant copy-on-write operation - no data is copied.
//
// Parameters:
//   - ctx: Context for cancellation
//   - templateRef: Template LV name (e.g., "ubuntu-2204-base") or template name
//   - vmID: VM identifier for naming the disk
//   - sizeGB: Target disk size in GB (will resize if larger than template)
//
// Returns the LV path for the VM disk.
func (m *LVMTemplateManager) CloneForVM(ctx context.Context, templateRef, vmID string, sizeGB int) (string, error) {
	templateLVName, err := NormalizeLVMTemplateRef(m.vgName, templateRef)
	if err != nil {
		return "", err
	}
	if err := validateLVMLVName(vmID); err != nil {
		return "", fmt.Errorf("validating vmID: %w", err)
	}

	vmDiskName := fmt.Sprintf(VMDiskNameFmt, vmID)
	logger := m.logger.With("template", templateLVName, "vm_disk", vmDiskName, "size_gb", sizeGB)
	logger.Info("cloning template for VM")

	// Create thin snapshot
	templatePath := fmt.Sprintf("/dev/%s/%s", m.vgName, templateLVName)
	vmDiskPath := fmt.Sprintf("/dev/%s/%s", m.vgName, vmDiskName)

	if err := m.createThinSnapshot(ctx, templatePath, vmDiskName, logger); err != nil {
		return "", fmt.Errorf("cloning template %s for VM %s: %w", templateLVName, vmID, err)
	}

	// Get template size to check if resize is needed
	templateSize, err := m.getLVSize(templateLVName)
	if err != nil {
		logger.Warn("failed to get template size, skipping resize check", "error", err)
	} else {
		// Resize only if requested size exceeds template virtual size
		templateSizeGB := templateSize / gbToBytes
		if int64(sizeGB) > templateSizeGB {
			logger.Info("resizing cloned disk", "from_gb", templateSizeGB, "to_gb", sizeGB)
			if err := m.resizeLV(ctx, vmDiskName, sizeGB, logger); err != nil {
				// Cleanup on resize failure
				if rmErr := m.removeLV(vmDiskName, logger); rmErr != nil {
					logger.Warn("failed to cleanup VM disk after resize failure", "lv", vmDiskName, "error", rmErr)
				}
				return "", fmt.Errorf("resizing cloned disk for VM %s: %w", vmID, err)
			}
		}
	}

	logger.Info("template cloned successfully", "vm_disk_path", vmDiskPath)
	return vmDiskPath, nil
}

// TemplateExists checks if a template LV exists.
//
// Parameters:
//   - ctx: Context for cancellation
//   - ref: Template LV name or template name
func (m *LVMTemplateManager) TemplateExists(ctx context.Context, ref string) (bool, error) {
	lvPath, err := CanonicalLVMTemplatePath(m.vgName, ref)
	if err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, "lvs", lvPath)
	if err := cmd.Run(); err != nil {
		// lvs returns non-zero exit code if LV doesn't exist
		return false, nil
	}
	return true, nil
}

// GetTemplateSize returns the virtual size of a template LV in bytes.
//
// Parameters:
//   - ctx: Context for cancellation
//   - ref: Template LV name or template name
func (m *LVMTemplateManager) GetTemplateSize(ctx context.Context, ref string) (int64, error) {
	lvName, err := NormalizeLVMTemplateRef(m.vgName, ref)
	if err != nil {
		return 0, err
	}

	size, err := m.getLVSize(lvName)
	if err != nil {
		return 0, fmt.Errorf("getting template %s size: %w", lvName, err)
	}
	return size, nil
}

// ListTemplates lists all available templates in the LVM thin pool.
// Templates are identified by the -base suffix and filtered by pool_lv.
//
// Parameters:
//   - ctx: Context for cancellation
func (m *LVMTemplateManager) ListTemplates(ctx context.Context) ([]TemplateInfo, error) {
	// Query LVM for thin LVs in our pool with -base suffix
	// lvs --noheadings --units b -o lv_name,lv_size --select "lv_name=~-base$ && pool_lv=thinpool"
	selectExpr := fmt.Sprintf("lv_name=~-base$ && pool_lv=%s", m.thinPool)
	cmd := exec.CommandContext(ctx, "lvs",
		"--noheadings",
		"--units", "b",
		"-o", "lv_name,lv_size",
		"--select", selectExpr)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}

	templates := m.parseLVSOutput(string(output))
	return templates, nil
}

// Helper methods

// getQemuVirtualSize returns the virtual size of a qcow2 image using qemu-img info.
func (m *LVMTemplateManager) getQemuVirtualSize(ctx context.Context, sourcePath string) (int64, error) {
	// Use a short timeout for qemu-img info (should be fast)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", sourcePath)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return 0, fmt.Errorf("qemu-img info failed for %s: %w (stderr: %s)", sourcePath, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return 0, fmt.Errorf("qemu-img info failed for %s: %w", sourcePath, err)
	}

	var info struct {
		VirtualSize int64 `json:"virtual-size"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return 0, fmt.Errorf("parsing qemu-img info output for %s: %w", sourcePath, err)
	}

	return info.VirtualSize, nil
}

// convertToRaw converts a qcow2 image to raw format using qemu-img convert.
// Uses an independent timeout (lvmTemplateStepTimeout) so large templates don't stall indefinitely.
func (m *LVMTemplateManager) convertToRaw(ctx context.Context, sourcePath, rawPath string, logger *slog.Logger) error {
	logger.Info("converting qcow2 to raw", "source", sourcePath, "dest", rawPath)

	// Use independent timeout for this long-running operation
	stepCtx, cancel := newLVMTemplateStepContext(ctx)
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
		if stepCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("qemu-img convert timed out after %v", lvmTemplateStepTimeout)
		}
		return fmt.Errorf("qemu-img convert failed: %w (output: %s)", err, string(out))
	}

	logger.Info("qcow2 converted to raw successfully")
	return nil
}

// createThinLV creates a new thin logical volume with the specified virtual size.
func (m *LVMTemplateManager) createThinLV(ctx context.Context, lvName string, virtualSize int64, logger *slog.Logger) error {
	logger.Info("creating thin LV", "lv", lvName, "virtual_size", virtualSize)

	// lvcreate --thin -V {virtualSize}B -n {name}-base {vg}/{thinPool}
	sizeArg := fmt.Sprintf("%dB", virtualSize)
	cmd := exec.CommandContext(ctx,
		"lvcreate",
		"--thin",
		"-V", sizeArg,
		"-n", lvName,
		fmt.Sprintf("%s/%s", m.vgName, m.thinPool),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating thin LV %s: %w (output: %s)", lvName, err, string(out))
	}

	logger.Info("thin LV created successfully")
	return nil
}

// writeRawToLV writes raw image data to an LV using dd.
// Uses an independent timeout (lvmTemplateStepTimeout) so large images don't stall indefinitely.
func (m *LVMTemplateManager) writeRawToLV(ctx context.Context, rawPath, lvPath string, logger *slog.Logger) error {
	logger.Info("writing raw image to LV", "source", rawPath, "dest", lvPath)

	// Use independent timeout for this long-running operation
	stepCtx, cancel := newLVMTemplateStepContext(ctx)
	defer cancel()

	// dd if=tmp.raw of=/dev/{vg}/{lv} bs=4M
	cmd := exec.CommandContext(stepCtx,
		"dd",
		fmt.Sprintf("if=%s", rawPath),
		fmt.Sprintf("of=%s", lvPath),
		"bs=4M",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		if stepCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("dd write timed out after %v", lvmTemplateStepTimeout)
		}
		return fmt.Errorf("dd write to %s failed: %w (output: %s)", lvPath, err, string(out))
	}

	logger.Info("raw image written to LV successfully")
	return nil
}

// createThinSnapshot creates a thin snapshot of a template LV.
// This is an instant operation - no data is actually copied.
func (m *LVMTemplateManager) createThinSnapshot(ctx context.Context, templatePath, snapshotName string, logger *slog.Logger) error {
	logger.Info("creating thin snapshot", "origin", templatePath, "snapshot", snapshotName)

	// lvcreate --thin -s --name vs-{vmID}-disk0 /dev/{vg}/{template}
	cmd := exec.CommandContext(ctx,
		"lvcreate",
		"--thin",
		"-s",
		"--name", snapshotName,
		templatePath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating thin snapshot %s from %s: %w (output: %s)", snapshotName, templatePath, err, string(out))
	}

	logger.Info("thin snapshot created successfully")
	return nil
}

// resizeLV resizes a logical volume to the specified size in GB.
func (m *LVMTemplateManager) resizeLV(ctx context.Context, lvName string, sizeGB int, logger *slog.Logger) error {
	logger.Info("resizing LV", "lv", lvName, "size_gb", sizeGB)

	lvPath := fmt.Sprintf("/dev/%s/%s", m.vgName, lvName)
	sizeArg := fmt.Sprintf("%dG", sizeGB)

	cmd := exec.CommandContext(ctx,
		"lvresize",
		"-L", sizeArg,
		lvPath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("resizing LV %s to %dG: %w (output: %s)", lvName, sizeGB, err, string(out))
	}

	logger.Info("LV resized successfully")
	return nil
}

// hasDependents checks if an LV has any thin snapshots that depend on it.
// Returns true if there are dependents, false otherwise.
func (m *LVMTemplateManager) hasDependents(lvName string) (bool, error) {
	// lvs --select "origin={lvName}" -o lv_name --noheadings
	cmd := exec.Command("lvs",
		"--select", fmt.Sprintf("origin=%s", lvName),
		"-o", "lv_name",
		"--noheadings",
	)

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("checking dependents for %s: %w", lvName, err)
	}

	// If there's any output (non-empty), there are dependents
	lines := strings.TrimSpace(string(output))
	return lines != "", nil
}

// removeLV removes a logical volume.
func (m *LVMTemplateManager) removeLV(lvName string, logger *slog.Logger) error {
	lvPath := fmt.Sprintf("/dev/%s/%s", m.vgName, lvName)

	cmd := exec.Command("lvremove", "-f", lvPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing LV %s: %w (output: %s)", lvName, err, string(out))
	}

	logger.Debug("LV removed", "lv", lvName)
	return nil
}

// getLVSize returns the size of a logical volume in bytes.
func (m *LVMTemplateManager) getLVSize(lvName string) (int64, error) {
	lvPath := fmt.Sprintf("/dev/%s/%s", m.vgName, lvName)

	cmd := exec.Command("lvs",
		"--noheadings",
		"--units", "b",
		"-o", "lv_size",
		lvPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("getting size of LV %s: %w", lvName, err)
	}

	// Parse output: "  10737418240.00\n" -> 10737418240
	sizeStr := strings.TrimSpace(string(output))
	// Remove decimal point and anything after (lvs reports "10737418240.00")
	if dotIdx := strings.Index(sizeStr, "."); dotIdx != -1 {
		sizeStr = sizeStr[:dotIdx]
	}

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing LV size %q: %w", sizeStr, err)
	}

	return size, nil
}

func newLVMTemplateStepContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, lvmTemplateStepTimeout)
}

// parseLVSOutput parses the output of lvs command into TemplateInfo structs.
// Expected format: "  lv_name   size\n"
func (m *LVMTemplateManager) parseLVSOutput(output string) []TemplateInfo {
	lines := strings.Split(output, "\n")
	var templates []TemplateInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by whitespace: "lv_name   size"
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		lvName := fields[0]
		sizeStr := fields[1]

		// Parse size (remove decimal part)
		if dotIdx := strings.Index(sizeStr, "."); dotIdx != -1 {
			sizeStr = sizeStr[:dotIdx]
		}

		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			m.logger.Warn("failed to parse template size", "lv", lvName, "size_str", sizeStr, "error", err)
			continue
		}

		// Strip -base suffix for the template name
		templateName := strings.TrimSuffix(lvName, lvmTemplateSuffix)

		templates = append(templates, TemplateInfo{
			Name:      templateName,
			FilePath:  fmt.Sprintf("/dev/%s/%s", m.vgName, lvName),
			SizeBytes: size,
			CreatedAt: time.Time{}, // LVM doesn't track creation time reliably
		})
	}

	return templates
}
