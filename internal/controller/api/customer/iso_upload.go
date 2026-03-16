package customer

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
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
	maxISOCount           = 5
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

// UploadISO handles POST /vms/:id/iso/upload - multipart ISO upload.
func (h *CustomerHandler) UploadISO(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to verify VM")
		return
	}

	if vm.Status == models.VMStatusDeleted {
		respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxISOSizeBytes+1024)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "MISSING_FILE", "No file provided in 'file' form field")
		return
	}
	defer file.Close()

	if !strings.EqualFold(filepath.Ext(header.Filename), ".iso") {
		respondWithError(c, http.StatusBadRequest, "INVALID_FILE_TYPE", "Only .iso files are allowed")
		return
	}

	if header.Size > maxISOSizeBytes {
		respondWithError(c, http.StatusBadRequest, "FILE_TOO_LARGE", "ISO file exceeds maximum allowed size of 10 GB")
		return
	}

	isoDir := filepath.Join(h.isoStoragePath, customerID, vmID)
	if err := os.MkdirAll(isoDir, 0750); err != nil {
		h.logger.Error("failed to create ISO directory",
			"path", isoDir, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to prepare upload directory")
		return
	}

	existing, _ := os.ReadDir(isoDir)
	if len(existing) >= maxISOCount {
		respondWithError(c, http.StatusConflict, "ISO_LIMIT_REACHED",
			"Maximum ISO limit reached (5 per VM). Delete existing ISOs first.")
		return
	}

	isoID := uuid.New().String()
	destPath := filepath.Join(isoDir, isoID+".iso")

	dst, err := os.Create(destPath)
	if err != nil {
		h.logger.Error("failed to create ISO file",
			"path", destPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to create file on disk")
		return
	}
	defer dst.Close()

	hasher := sha256.New()
	multiWriter := io.MultiWriter(dst, hasher)

	written, err := io.CopyN(multiWriter, file, maxISOSizeBytes+1)
	if err != nil && err != io.EOF {
		dst.Close()
		os.Remove(destPath)
		h.logger.Error("failed to write ISO file",
			"path", destPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "ISO_UPLOAD_FAILED", "Failed to write file")
		return
	}

	if written > maxISOSizeBytes {
		dst.Close()
		os.Remove(destPath)
		respondWithError(c, http.StatusBadRequest, "FILE_TOO_LARGE", "ISO file exceeds maximum allowed size of 10 GB")
		return
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	sumPath := destPath + ".sha256"
	if err := os.WriteFile(sumPath, []byte(checksum), 0640); err != nil {
		h.logger.Warn("failed to write checksum sidecar",
			"path", sumPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
	}

	c.JSON(http.StatusCreated, models.Response{Data: ISOUploadResponse{
		ID:       isoID,
		FileName: sanitizeFileName(header.Filename),
		FileSize: written,
		SHA256:   checksum,
	}})

	h.logger.Info("ISO uploaded",
		"iso_id", isoID,
		"vm_id", vmID,
		"customer_id", customerID,
		"file_name", header.Filename,
		"file_size", written,
		"correlation_id", middleware.GetCorrelationID(c),
	)
}

// ListISOs handles GET /vms/:id/iso - lists uploaded ISOs for a VM.
func (h *CustomerHandler) ListISOs(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "ISO_LIST_FAILED", "Failed to verify VM")
		return
	}

	isoDir := filepath.Join(h.isoStoragePath, customerID, vmID)
	records, err := listISODirectory(isoDir, vmID)
	if err != nil {
		h.logger.Error("failed to list ISOs",
			"vm_id", vmID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "ISO_LIST_FAILED", "Failed to list ISOs")
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
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(isoID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_ISO_ID", "ISO ID must be a valid UUID")
		return
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "ISO_DELETE_FAILED", "Failed to verify VM")
		return
	}

	if vm.AttachedISO != nil && *vm.AttachedISO == isoID {
		respondWithError(c, http.StatusConflict, "ISO_ATTACHED", "Cannot delete an ISO that is currently attached to the VM")
		return
	}

	isoPath := filepath.Join(h.isoStoragePath, customerID, vmID, isoID+".iso")
	if err := os.Remove(isoPath); err != nil {
		if os.IsNotExist(err) {
			respondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO not found")
			return
		}
		h.logger.Error("failed to delete ISO",
			"path", isoPath, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "ISO_DELETE_FAILED", "Failed to delete ISO file")
		return
	}
	os.Remove(isoPath + ".sha256")

	h.logger.Info("ISO deleted",
		"iso_id", isoID,
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c),
	)

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "ISO deleted successfully"}})
}

// AttachISO handles POST /vms/:id/iso/:isoId/attach - attaches an ISO to a VM.
func (h *CustomerHandler) AttachISO(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")
	isoID := c.Param("isoId")

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(isoID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_ISO_ID", "ISO ID must be a valid UUID")
		return
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "ISO_ATTACH_FAILED", "Failed to verify VM")
		return
	}

	if vm.NodeID == nil {
		respondWithError(c, http.StatusBadRequest, "VM_NOT_ASSIGNED", "VM is not assigned to a node")
		return
	}

	if vm.Status != models.VMStatusRunning && vm.Status != models.VMStatusStopped {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_STATE",
			"VM must be running or stopped to attach an ISO")
		return
	}

	isoPath := filepath.Join(h.isoStoragePath, customerID, vmID, isoID+".iso")
	if _, err := os.Stat(isoPath); os.IsNotExist(err) {
		respondWithError(c, http.StatusNotFound, "ISO_NOT_FOUND", "ISO file not found on disk")
		return
	}

	if err := h.vmRepo.UpdateAttachedISO(c.Request.Context(), vmID, &isoID); err != nil {
		h.logger.Error("failed to attach ISO",
			"vm_id", vmID, "iso_id", isoID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "ISO_ATTACH_FAILED", "Failed to attach ISO")
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
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "ISO_DETACH_FAILED", "Failed to verify VM")
		return
	}

	if vm.AttachedISO == nil || *vm.AttachedISO != isoID {
		respondWithError(c, http.StatusBadRequest, "ISO_NOT_ATTACHED", "This ISO is not attached to the VM")
		return
	}

	if err := h.vmRepo.UpdateAttachedISO(c.Request.Context(), vmID, nil); err != nil {
		h.logger.Error("failed to detach ISO",
			"vm_id", vmID, "iso_id", isoID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "ISO_DETACH_FAILED", "Failed to detach ISO")
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
	return base + ext
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
		} else {
			h := sha256.New()
			if f, openErr := os.Open(filepath.Join(dir, entry.Name())); openErr == nil {
				if _, copyErr := io.Copy(h, f); copyErr == nil {
					checksum = hex.EncodeToString(h.Sum(nil))
				}
				f.Close()
			}
		}

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
