package customer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxISOSizeBytes int64 = 10 * 1024 * 1024 * 1024

	// isoMagicReadBytes is the number of bytes read at the start of the file
	// to detect ISO 9660 / UDF magic identifiers (F-074). The ISO 9660
	// primary volume descriptor identifier begins at offset 0x8001.
	isoMagicReadBytes = 0x8001 + len("CD001")

	// defaultISOLimit is the default maximum number of ISO files per VM
	// when the plan does not specify a limit.
	defaultISOLimit = 2
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
// @Tags Customer
// @Summary Upload ISO
// @Description Uploads an ISO image file for a customer VM.
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param file formData file true "ISO file"
// @Success 201 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/iso/upload [post]
func (h *CustomerHandler) UploadISO(c *gin.Context) {
	ctx, ok := h.validateISOUploadAccess(c)
	if !ok {
		return
	}
	defer func() {
		if err := ctx.file.Close(); err != nil {
			h.logger.Warn("failed to close uploaded file", "error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		}
	}()

	if !h.validateISOFile(c, ctx.header) {
		return
	}

	if !h.prepareISODirectory(c, ctx.customerID, ctx.vmID) {
		return
	}

	result, ok := h.writeISOFile(c, ctx.file, ctx.vm, ctx.customerID, ctx.vmID, ctx.header.Filename)
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

	// F-012: Reject oversized uploads before reading the body.
	// Content-Length is advisory (clients may lie), but it catches honest clients
	// and reduces unnecessary disk I/O for clearly oversized uploads.
	if cl := c.Request.ContentLength; cl > 0 && cl > maxISOSizeBytes+1024 {
		middleware.RespondWithError(c, http.StatusBadRequest, "FILE_TOO_LARGE", "ISO file exceeds maximum allowed size of 10 GB")
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

// prepareISODirectory ensures the VM ISO directory exists before the upload starts.
func (h *CustomerHandler) prepareISODirectory(c *gin.Context, customerID, vmID string) bool {
	isoDir := filepath.Join(h.isoStoragePath, customerID, vmID)
	if err := os.MkdirAll(isoDir, 0750); err != nil {
		h.logger.Error("failed to create ISO directory",
			"path", isoDir, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to prepare upload directory")
		return false
	}

	return true
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

// resolvedISOBase returns the real (symlink-resolved) path of the ISO storage root.
// F-073: Resolving symlinks at point-of-use prevents path traversal via symlinks.
func (h *CustomerHandler) resolvedISOBase() (string, error) {
	resolved, err := filepath.EvalSymlinks(h.isoStoragePath)
	if err != nil {
		return "", fmt.Errorf("resolving ISO storage path: %w", err)
	}
	return resolved, nil
}

// validateISOWritePath ensures the destination path is under the resolved ISO base
// to prevent directory traversal and symlink attacks (F-073).
func validateISOWritePath(resolvedBase, destPath string) error {
	resolvedDest, err := filepath.EvalSymlinks(filepath.Dir(destPath))
	if err != nil {
		// The file doesn't exist yet; resolve only the directory.
		resolvedDest, err = filepath.EvalSymlinks(filepath.Dir(destPath))
		if err != nil {
			return fmt.Errorf("resolving destination directory: %w", err)
		}
	}
	rel, err := filepath.Rel(resolvedBase, resolvedDest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("destination %q is outside ISO storage root", destPath)
	}
	return nil
}

// writeISOFile writes the ISO file to disk and computes its checksum.
// F-012: Wraps the reader with io.LimitReader so the stream is aborted mid-copy if
// the file exceeds maxISOSizeBytes, avoiding a full-file write before the size check.
// F-073: Validates the write path against the resolved storage root.
// F-074: Checks ISO 9660 / UDF magic bytes after writing.
func (h *CustomerHandler) writeISOFile(c *gin.Context, file io.Reader, vm *models.VM, customerID, vmID, filename string) (*ISOUploadResponse, bool) {
	resolvedBase, err := h.resolvedISOBase()
	if err != nil {
		h.logger.Error("failed to resolve ISO storage path",
			"path", h.isoStoragePath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Internal server error")
		return nil, false
	}

	isoID := uuid.New().String()
	isoDir := filepath.Join(h.isoStoragePath, customerID, vmID)
	tempPath := filepath.Join(isoDir, isoID+".uploading")
	finalPath := filepath.Join(isoDir, isoID+".iso")

	// F-073: Ensure the destination is inside the resolved storage root.
	if err := validateISOWritePath(resolvedBase, tempPath); err != nil {
		h.logger.Error("ISO path traversal detected",
			"path", tempPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_PATH", "Invalid upload path")
		return nil, false
	}

	dst, err := os.Create(tempPath)
	if err != nil {
		h.logger.Error("failed to create ISO file",
			"path", tempPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to create file on disk")
		return nil, false
	}
	defer func() {
		if err := dst.Close(); err != nil {
			h.logger.Warn("failed to close ISO file", "path", tempPath, "error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		}
	}()

	// F-012: Limit the reader to maxISOSizeBytes+1 so that copyFileWithHash will
	// detect an overrun without writing the entire oversized stream to disk first.
	limitedReader := io.LimitReader(file, maxISOSizeBytes+1)

	written, checksum, magicBuf, err := h.copyFileWithHash(dst, limitedReader, tempPath)
	if err != nil {
		h.logger.Error("failed to write ISO file",
			"path", tempPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to write file")
		return nil, false
	}

	// F-012: If exactly maxISOSizeBytes+1 bytes were written the file is oversized.
	if written > maxISOSizeBytes {
		if err := os.Remove(tempPath); err != nil {
			h.logger.Warn("failed to remove oversized ISO file", "path", tempPath, "error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "FILE_TOO_LARGE", "ISO file exceeds maximum allowed size of 10 GB")
		return nil, false
	}

	// F-074: Validate ISO 9660 / UDF magic bytes.
	if !isValidISOMagic(magicBuf) {
		if err := os.Remove(tempPath); err != nil {
			h.logger.Warn("failed to remove invalid ISO file", "path", tempPath, "error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_FILE_TYPE", "File does not appear to be a valid ISO 9660 or UDF image")
		return nil, false
	}

	planLimit := h.getISOPlanLimit(c.Request.Context(), vm.PlanID)
	upload := &repository.ISOUpload{
		ID:          isoID,
		VMID:        vmID,
		CustomerID:  customerID,
		FileName:    sanitizeFileName(filename),
		FileSize:    written,
		SHA256:      checksum,
		StoragePath: finalPath,
	}
	if err := h.isoUploadRepo.CreateIfUnderLimit(c.Request.Context(), upload, planLimit); err != nil {
		if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
			h.logger.Warn("failed to remove rejected ISO upload", "path", tempPath, "error", removeErr,
				"correlation_id", middleware.GetCorrelationID(c))
		}
		var limitErr *repository.LimitExceededError
		if errors.As(err, &limitErr) {
			middleware.RespondWithError(c, http.StatusConflict, "ISO_LIMIT_REACHED",
				fmt.Sprintf("ISO upload limit reached for this VM (%d/%d). Delete existing ISOs first.", limitErr.Current, limitErr.Limit))
			return nil, false
		}
		h.logger.Error("failed to persist ISO upload metadata",
			"vm_id", vmID, "customer_id", customerID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to register ISO upload")
		return nil, false
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = h.isoUploadRepo.Delete(c.Request.Context(), upload.ID)
		if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
			h.logger.Warn("failed to remove ISO temp file after rename failure", "path", tempPath, "error", removeErr,
				"correlation_id", middleware.GetCorrelationID(c))
		}
		h.logger.Error("failed to finalize ISO upload",
			"path", finalPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to finalize ISO upload")
		return nil, false
	}

	h.writeChecksumSidecar(finalPath, checksum, middleware.GetCorrelationID(c))

	return &ISOUploadResponse{
		ID:       upload.ID,
		FileName: upload.FileName,
		FileSize: upload.FileSize,
		SHA256:   upload.SHA256,
	}, true
}

// isValidISOMagic checks ISO 9660 and UDF magic signatures in the captured bytes.
// F-074: ISO 9660 Primary Volume Descriptor begins at byte 32768 (sector 16,
// 2048 bytes/sector) and the identifier starts at offset 0x8001.
func isValidISOMagic(buf []byte) bool {
	// ISO 9660: the Primary Volume Descriptor starts at offset 0x8000 (32768).
	// The 5-byte identifier "CD001" starts at offset 0x8001.
	const iso9660Offset = 0x8001
	iso9660Magic := []byte("CD001")

	if len(buf) >= iso9660Offset+len(iso9660Magic) {
		if bytes.Equal(buf[iso9660Offset:iso9660Offset+len(iso9660Magic)], iso9660Magic) {
			return true
		}
	}

	// UDF volume recognition sequence descriptors are recorded in the same
	// descriptor area, one 2048-byte sector apart.
	udfMarkers := []struct {
		offset int
		magic  []byte
	}{
		{offset: 0x8001, magic: []byte("BEA01")},
		{offset: 0x8801, magic: []byte("NSR02")},
		{offset: 0x9001, magic: []byte("TEA01")},
	}

	for _, marker := range udfMarkers {
		if len(buf) < marker.offset+len(marker.magic) {
			return false
		}
		if !bytes.Equal(buf[marker.offset:marker.offset+len(marker.magic)], marker.magic) {
			return false
		}
	}

	return true
}

// copyFileWithHash copies data from reader to file while computing SHA256 hash.
// Returns (bytesWritten, sha256hex, firstNBytes, error).
// F-074: The first isoMagicReadBytes are captured for magic-byte validation.
// F-012: The caller is expected to pass an io.LimitReader; this function no longer
// enforces the limit internally — it copies until EOF or error.
func (h *CustomerHandler) copyFileWithHash(dst *os.File, src io.Reader, destPath string) (int64, string, []byte, error) {
	hasher := sha256.New()

	// Tee the first isoMagicReadBytes into a buffer for magic-byte validation.
	// Pre-allocate magicBuf with fixed capacity to avoid confusion about buffer growth.
	// The buffer only ever needs isoMagicReadBytes worth of data.
	magicBuf := bytes.NewBuffer(make([]byte, 0, isoMagicReadBytes))
	teeReader := io.TeeReader(src, magicBuf)
	limitedMagic := io.LimitReader(teeReader, int64(isoMagicReadBytes))

	// Write the first isoMagicReadBytes through hasher and dst.
	multiWriter := io.MultiWriter(dst, hasher)
	_, err := io.Copy(multiWriter, limitedMagic)
	if err != nil {
		_ = dst.Close()
		_ = os.Remove(destPath)
		return 0, "", nil, err
	}

	// Now copy the remainder of the stream (the LimitReader stopped at isoMagicReadBytes).
	remaining, err := io.Copy(multiWriter, src)
	if err != nil {
		_ = dst.Close()
		_ = os.Remove(destPath)
		return 0, "", nil, err
	}

	written := int64(magicBuf.Len()) + remaining
	checksum := hex.EncodeToString(hasher.Sum(nil))
	return written, checksum, magicBuf.Bytes(), nil
}

// writeChecksumSidecar writes the SHA256 checksum to a sidecar file.
func (h *CustomerHandler) writeChecksumSidecar(destPath, checksum, correlationID string) {
	sumPath := destPath + ".sha256"
	if err := os.WriteFile(sumPath, []byte(checksum), 0640); err != nil { //nolint:gosec // G306: 0640 is acceptable for checksum files (non-sensitive)
		h.logger.Warn("failed to write checksum sidecar",
			"path", sumPath, "error", err,
			"correlation_id", correlationID)
	}
}

// ListISOs handles GET /vms/:id/iso - lists uploaded ISOs for a VM.
// @Tags Customer
// @Summary List ISOs
// @Description Lists ISO uploads for a customer VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/iso [get]
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

	uploads, err := h.isoUploadRepo.ListByVM(c.Request.Context(), vmID)
	if err != nil {
		h.logger.Error("failed to list ISOs",
			"vm_id", vmID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_LIST_FAILED", "Failed to list ISOs")
		return
	}

	records := make([]ISORecord, 0, len(uploads))
	for _, upload := range uploads {
		records = append(records, ISORecord{
			ID:        upload.ID,
			VMID:      upload.VMID,
			FileName:  upload.FileName,
			FileSize:  upload.FileSize,
			SHA256:    upload.SHA256,
			Status:    "available",
			CreatedAt: upload.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, models.Response{Data: records})
}

// DeleteISO handles DELETE /vms/:id/iso/:isoId - deletes an uploaded ISO.
// @Tags Customer
// @Summary Delete ISO
// @Description Deletes an ISO upload from a customer VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param isoId path string true "ISO ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/iso/{isoId} [delete]
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

	upload, err := h.isoUploadRepo.GetByID(c.Request.Context(), isoID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_DELETE_FAILED", "Failed to retrieve ISO upload")
		return
	}
	if upload.VMID != vmID || upload.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO not found")
		return
	}

	if err := os.Remove(upload.StoragePath); err != nil && !os.IsNotExist(err) {
		h.logger.Error("failed to delete ISO",
			"path", upload.StoragePath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_DELETE_FAILED", "Failed to delete ISO file")
		return
	}
	if err := h.isoUploadRepo.DeleteByVMAndID(c.Request.Context(), vmID, isoID); err != nil && !sharederrors.Is(err, sharederrors.ErrNoRowsAffected) {
		h.logger.Error("failed to delete ISO metadata",
			"iso_id", isoID, "vm_id", vmID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_DELETE_FAILED", "Failed to delete ISO metadata")
		return
	}
	if err := os.Remove(upload.StoragePath + ".sha256"); err != nil && !os.IsNotExist(err) {
		h.logger.Warn("failed to delete ISO checksum sidecar",
			"path", upload.StoragePath+".sha256", "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
	}

	h.logger.Info("ISO deleted",
		"iso_id", isoID,
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c),
	)

	c.Status(http.StatusNoContent)
}

// AttachISO handles POST /vms/:id/iso/:isoId/attach - attaches an ISO to a VM.
// @Tags Customer
// @Summary Attach ISO
// @Description Attaches an uploaded ISO to a customer VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param isoId path string true "ISO ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/iso/{isoId}/attach [post]
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

	upload, err := h.isoUploadRepo.GetByID(c.Request.Context(), isoID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "ISO_ATTACH_FAILED", "Failed to retrieve ISO metadata")
		return
	}
	if upload.VMID != vmID || upload.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO not found")
		return
	}

	if _, err := os.Stat(upload.StoragePath); os.IsNotExist(err) {
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
// @Tags Customer
// @Summary Detach ISO
// @Description Detaches an ISO from a customer VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param isoId path string true "ISO ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/iso/{isoId}/detach [post]
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
