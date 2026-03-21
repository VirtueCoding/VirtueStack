package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxISOSizeBytes int64 = 10 * 1024 * 1024 * 1024
)

type ISORecord struct {
	ID        string    `json:"id"`
	VMID      string    `json:"vm_id"`
	FileName  string    `json:"file_name"`
	FileSize  int64     `json:"file_size"`
	SHA256    string    `json:"sha256"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type ISOUploadResponse struct {
	ID       string `json:"id"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	SHA256   string `json:"sha256"`
}

// isoUploadContext holds the validated context for an ISO upload operation.
type isoUploadContext struct {
	vmID       string
	customerID string
	vm         *models.VM
	file       io.ReadCloser
	header     *multipart.FileHeader
}

// UploadISO handles POST /vms/:id/iso/upload - multipart ISO upload.
func (h *CustomerHandler) UploadISO(c *gin.Context) {
	ctx, ok := h.validateISOUploadAccess(c)
	if !ok {
		return
	}
	defer ctx.file.Close()

	if !h.validateISOFile(c, ctx.header) {
		return
	}

	if !h.checkISOLimit(c, ctx.vm, ctx.customerID, ctx.vmID) {
		return
	}

	result, ok := h.writeISOFile(c, ctx.file, ctx.customerID, ctx.vmID, ctx.header.Filename)
	if !ok {
		return
	}

	h.logger.Info("ISO uploaded",
		"iso_id", result.ID,
		"vm_id", ctx.vmID,
		"customer_id", ctx.customerID,
		"file_name", ctx.header.Filename,
		"file_size", result.FileSize,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: result})
}

// validateISOUploadAccess validates the VM ID and retrieves the VM for upload.
func (h *CustomerHandler) validateISOUploadAccess(c *gin.Context) (*isoUploadContext, bool) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return nil, false
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return nil, false
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to verify VM")
		return nil, false
	}

	if vm.Status == models.VMStatusDeleted {
		middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
		return nil, false
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxISOSizeBytes+1024)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "MISSING_FILE", "No file provided in 'file' form field")
		return nil, false
	}

	return &isoUploadContext{
		vmID:       vmID,
		customerID: customerID,
		vm:         vm,
		file:       file,
		header:     header,
	}, true
}

// validateISOFile validates the uploaded file extension and size.
func (h *CustomerHandler) validateISOFile(c *gin.Context, header *multipart.FileHeader) bool {
	if !strings.EqualFold(filepath.Ext(header.Filename), ".iso") {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_FILE_TYPE", "Only .iso files are allowed")
		return false
	}

	if header.Size > maxISOSizeBytes {
		middleware.RespondWithError(c, http.StatusBadRequest, "FILE_TOO_LARGE", "ISO file exceeds maximum allowed size of 10 GB")
		return false
	}

	return true
}

// checkISOLimit checks if the VM has reached its ISO upload limit.
func (h *CustomerHandler) checkISOLimit(c *gin.Context, vm *models.VM, customerID, vmID string) bool {
	isoDir := filepath.Join(h.isoStoragePath, customerID, vmID)
	if err := os.MkdirAll(isoDir, 0750); err != nil {
		h.logger.Error("failed to create ISO directory",
			"path", isoDir, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to prepare upload directory")
		return false
	}

	existing, _ := os.ReadDir(isoDir)
	isoCount := countISOFiles(existing)

	planLimit := h.getISOPlanLimit(c.Request.Context(), vm.PlanID)
	if isoCount >= planLimit {
		middleware.RespondWithError(c, http.StatusConflict, "ISO_LIMIT_REACHED",
			fmt.Sprintf("ISO upload limit reached for this VM (%d/%d). Delete existing ISOs first.", isoCount, planLimit))
		return false
	}

	return true
}

// countISOFiles counts the number of .iso files in a directory listing.
func countISOFiles(entries []os.DirEntry) int {
	count := 0
	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".iso") {
			count++
		}
	}
	return count
}

// getISOPlanLimit returns the ISO upload limit for a plan.
func (h *CustomerHandler) getISOPlanLimit(ctx context.Context, planID string) int {
	planLimit := defaultISOLimit
	plan, err := h.planRepo.GetByID(ctx, planID)
	if err == nil && plan.ISOUploadLimit > 0 {
		planLimit = plan.ISOUploadLimit
	}
	return planLimit
}

// writeISOFile writes the ISO file to disk and computes its checksum.
func (h *CustomerHandler) writeISOFile(c *gin.Context, file io.Reader, customerID, vmID, filename string) (*ISOUploadResponse, bool) {
	isoID := uuid.New().String()
	isoDir := filepath.Join(h.isoStoragePath, customerID, vmID)
	destPath := filepath.Join(isoDir, isoID+".iso")

	dst, err := os.Create(destPath)
	if err != nil {
		h.logger.Error("failed to create ISO file",
			"path", destPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to create file on disk")
		return nil, false
	}
	defer dst.Close()

	written, checksum, err := h.copyFileWithHash(dst, file, destPath)
	if err != nil {
		h.logger.Error("failed to write ISO file",
			"path", destPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to write file")
		return nil, false
	}

	if written > maxISOSizeBytes {
		os.Remove(destPath)
		middleware.RespondWithError(c, http.StatusBadRequest, "FILE_TOO_LARGE", "ISO file exceeds maximum allowed size of 10 GB")
		return nil, false
	}

	h.writeChecksumSidecar(destPath, checksum, middleware.GetCorrelationID(c))

	return &ISOUploadResponse{
		ID:       isoID,
		FileName: sanitizeFileName(filename),
		FileSize: written,
		SHA256:   checksum,
	}, true
}

// copyFileWithHash copies data from reader to file while computing SHA256 hash.
func (h *CustomerHandler) copyFileWithHash(dst *os.File, src io.Reader, destPath string) (int64, string, error) {
	hasher := sha256.New()
	multiWriter := io.MultiWriter(dst, hasher)

	written, err := io.CopyN(multiWriter, src, maxISOSizeBytes+1)
	if err != nil && err != io.EOF {
		dst.Close()
		os.Remove(destPath)
		return 0, "", err
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	return written, checksum, nil
}

// writeChecksumSidecar writes the SHA256 checksum to a sidecar file.
func (h *CustomerHandler) writeChecksumSidecar(destPath, checksum, correlationID string) {
	sumPath := destPath + ".sha256"
	if err := os.WriteFile(sumPath, []byte(checksum), 0640); err != nil {
		h.logger.Warn("failed to write checksum sidecar",
			"path", sumPath, "error", err,
			"correlation_id", correlationID)
	}
}

// ListISOs handles GET /vms/:id/iso - lists uploaded ISOs for a VM.
func (h *CustomerHandler) ListISOs(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_LIST_FAILED", "Failed to verify VM")
		return
	}

	isoDir := filepath.Join(h.isoStoragePath, customerID, vmID)
	records, err := listISODirectory(isoDir, vmID)
	if err != nil {
		h.logger.Error("failed to list ISOs",
			"vm_id", vmID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_LIST_FAILED", "Failed to list ISOs")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: records})
}

// DeleteISO handles DELETE /vms/:id/iso/:isoId - deletes an uploaded ISO.
func (h *CustomerHandler) DeleteISO(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")
	isoID := c.Param("isoId")

	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(isoID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_ISO_ID", "ISO ID must be a valid UUID")
		return
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_DELETE_FAILED", "Failed to verify VM")
		return
	}

	if vm.AttachedISO != nil && *vm.AttachedISO == isoID {
		middleware.RespondWithError(c, http.StatusConflict, "ISO_ATTACHED", "Cannot delete an ISO that is currently attached to the VM")
		return
	}

	isoPath := filepath.Join(h.isoStoragePath, customerID, vmID, isoID+".iso")
	if err := os.Remove(isoPath); err != nil {
		if os.IsNotExist(err) {
			middleware.RespondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO not found")
			return
		}
		h.logger.Error("failed to delete ISO",
			"path", isoPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_DELETE_FAILED", "Failed to delete ISO file")
		return
	}
	os.Remove(isoPath + ".sha256")

	h.logger.Info("ISO deleted",
		"iso_id", isoID,
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c),
	)

	c.Status(http.StatusNoContent)
}

// AttachISO handles POST /vms/:id/iso/:isoId/attach - attaches an ISO to a VM.
func (h *CustomerHandler) AttachISO(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")
	isoID := c.Param("isoId")

	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(isoID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_ISO_ID", "ISO ID must be a valid UUID")
		return
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_ATTACH_FAILED", "Failed to verify VM")
		return
	}

	if vm.NodeID == nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "VM_NOT_ASSIGNED", "VM is not assigned to a node")
		return
	}

	if vm.Status != models.VMStatusRunning && vm.Status != models.VMStatusStopped {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_STATE",
			"VM must be running or stopped to attach an ISO")
		return
	}

	isoPath := filepath.Join(h.isoStoragePath, customerID, vmID, isoID+".iso")
	if _, err := os.Stat(isoPath); os.IsNotExist(err) {
		middleware.RespondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO file not found on disk")
		return
	}

	if err := h.vmRepo.UpdateAttachedISO(c.Request.Context(), vmID, &isoID); err != nil {
		h.logger.Error("failed to attach ISO",
			"vm_id", vmID, "iso_id", isoID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_ATTACH_FAILED", "Failed to attach ISO")
		return
	}

	h.logger.Info("ISO attached to VM",
		"iso_id", isoID,
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c),
	)

	c.JSON(http.StatusOK, models.Response{Data: gin.H{
		"message":         "ISO attached successfully",
		"attached_iso_id": isoID,
	}})
}

// DetachISO handles POST /vms/:id/iso/:isoId/detach - detaches an ISO from a VM.
func (h *CustomerHandler) DetachISO(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")
	isoID := c.Param("isoId")

	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_DETACH_FAILED", "Failed to verify VM")
		return
	}

	if vm.AttachedISO == nil || *vm.AttachedISO != isoID {
		middleware.RespondWithError(c, http.StatusBadRequest, "ISO_NOT_ATTACHED", "This ISO is not attached to the VM")
		return
	}

	if err := h.vmRepo.UpdateAttachedISO(c.Request.Context(), vmID, nil); err != nil {
		h.logger.Error("failed to detach ISO",
			"vm_id", vmID, "iso_id", isoID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_DETACH_FAILED", "Failed to detach ISO")
		return
	}

	h.logger.Info("ISO detached from VM",
		"iso_id", isoID,
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c),
	)

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "ISO detached successfully"}})
}

func sanitizeFileName(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	base = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, base)
	if len(base) > 200 {
		base = base[:200]
	}
	// Always enforce .iso extension regardless of what was provided.
	return base + ".iso"
}

func listISODirectory(dir, vmID string) ([]ISORecord, error) {
	var records []ISORecord

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return records, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".iso") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		isoID := strings.TrimSuffix(entry.Name(), ".iso")

		checksum := ""
		sumData, sumErr := os.ReadFile(filepath.Join(dir, isoID+".sha256"))
		if sumErr == nil {
			checksum = strings.TrimSpace(string(sumData))
		}
		// If the sidecar is missing, return an empty checksum.
		// Computing SHA-256 of multi-GB ISOs on every list request is too expensive.

		records = append(records, ISORecord{
			ID:        isoID,
			VMID:      vmID,
			FileName:  entry.Name(),
			FileSize:  info.Size(),
			SHA256:    checksum,
			Status:    "available",
			CreatedAt: info.ModTime(),
		})
	}

	return records, nil
}
