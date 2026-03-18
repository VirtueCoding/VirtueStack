package customer

import (
	"context"
	"fmt"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateBackupRequest represents the request body for creating a backup.
type CreateBackupRequest struct {
	VMID string `json:"vm_id" validate:"required,uuid"`
	Name string `json:"name" validate:"max=100"`
}

// ListBackups handles GET /backups - lists all backups for the customer's VMs.
// Supports pagination and filtering by VM ID or status.
func (h *CustomerHandler) ListBackups(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	// Parse pagination
	pagination := models.ParsePagination(c)

	// Build filter
	filter := repository.BackupListFilter{
		PaginationParams: pagination,
	}

	// Optional VM ID filter
	if vmID := c.Query("vm_id"); vmID != "" {
		// Verify VM belongs to customer
		if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
			if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
				respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
				return
			}
			respondWithError(c, http.StatusInternalServerError, "BACKUP_LIST_FAILED", "Failed to verify VM")
			return
		}
		filter.VMID = &vmID
	}

	// Optional status filter
	validBackupStatuses := map[string]bool{
		"creating": true, "completed": true, "failed": true, "restoring": true, "deleted": true,
	}
	if status := c.Query("status"); status != "" {
		if !validBackupStatuses[status] {
			respondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		filter.Status = &status
	}

	backups, total, err := h.backupRepo.ListBackupsByCustomer(c.Request.Context(), customerID, filter)
	if err != nil {
		h.logger.Error("failed to list backups",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "BACKUP_LIST_FAILED", "Failed to retrieve backups")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: backups,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateBackup handles POST /backups - creates a backup for a VM.
// This is an async operation. Returns 202 Accepted with a task_id.
func (h *CustomerHandler) CreateBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req CreateBackupRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate UUID
	if _, err := uuid.Parse(req.VMID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Verify VM belongs to customer
	vm, err := h.vmService.GetVM(c.Request.Context(), req.VMID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "BACKUP_CREATE_FAILED", "Failed to verify VM")
		return
	}

	planLimit := defaultBackupLimit
	plan, planErr := h.planRepo.GetByID(c.Request.Context(), vm.PlanID)
	if planErr == nil && plan.BackupLimit > 0 {
		planLimit = plan.BackupLimit
	}

	backupCount, countErr := h.backupRepo.CountBackupsByVM(c.Request.Context(), vm.ID)
	if countErr == nil && backupCount >= planLimit {
		respondWithError(c, http.StatusConflict, "BACKUP_LIMIT_EXCEEDED",
			fmt.Sprintf("Backup limit reached for this VM (%d/%d). Delete existing backups first.", backupCount, planLimit))
		return
	}

	// Create backup
	backup, err := h.backupService.CreateBackup(c.Request.Context(), vm.ID, req.Name)
	if err != nil {
		h.logger.Error("failed to create backup",
			"vm_id", req.VMID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "BACKUP_CREATE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("backup created via customer API",
		"backup_id", backup.ID,
		"vm_id", req.VMID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{Data: backup})
}

// GetBackup handles GET /backups/:id - retrieves details for a specific backup.
// Enforces customer isolation.
func (h *CustomerHandler) GetBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Get backup
	backup, err := h.backupRepo.GetBackupByID(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "BACKUP_GET_FAILED", "Failed to retrieve backup")
		return
	}

	// Verify backup belongs to a VM owned by the customer
	if !h.verifyBackupOwnership(c.Request.Context(), backup.VMID, customerID) {
		respondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: backup})
}

// DeleteBackup handles DELETE /backups/:id - deletes a backup.
// Returns 200 OK on success.
func (h *CustomerHandler) DeleteBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Get backup to verify ownership
	backup, err := h.backupRepo.GetBackupByID(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "BACKUP_DELETE_FAILED", "Failed to retrieve backup")
		return
	}

	// Verify backup belongs to a VM owned by the customer
	if !h.verifyBackupOwnership(c.Request.Context(), backup.VMID, customerID) {
		respondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
		return
	}

	// Delete backup
	if err := h.backupService.DeleteBackup(c.Request.Context(), backupID); err != nil {
		h.logger.Error("failed to delete backup",
			"backup_id", backupID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "BACKUP_DELETE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("backup deleted via customer API",
		"backup_id", backupID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}

// RestoreBackup handles POST /backups/:id/restore - restores a VM from a backup.
// This is an async operation. The VM will be stopped during restore.
func (h *CustomerHandler) RestoreBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Get backup to verify ownership
	backup, err := h.backupRepo.GetBackupByID(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "BACKUP_RESTORE_FAILED", "Failed to retrieve backup")
		return
	}

	// Verify backup belongs to a VM owned by the customer
	if !h.verifyBackupOwnership(c.Request.Context(), backup.VMID, customerID) {
		respondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
		return
	}

	// Restore backup
	if err := h.backupService.RestoreBackup(c.Request.Context(), backupID); err != nil {
		h.logger.Error("failed to restore backup",
			"backup_id", backupID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "BACKUP_RESTORE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("backup restore initiated via customer API",
		"backup_id", backupID,
		"vm_id", backup.VMID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{Data: gin.H{"message": "Backup restore initiated"}})
}

// verifyBackupOwnership verifies that a VM belongs to the customer.
func (h *CustomerHandler) verifyBackupOwnership(ctx context.Context, vmID, customerID string) bool {
	_, err := h.vmService.GetVM(ctx, vmID, customerID, false)
	return err == nil
}
