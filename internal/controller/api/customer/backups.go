package customer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
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
// @Tags Customer
// @Summary List backups
// @Description Performs backup operation for customer resources.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/backups [get]
func (h *CustomerHandler) ListBackups(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	// Parse pagination
	pagination := models.ParsePagination(c)

	// Build filter
	filter := repository.BackupListFilter{
		PaginationParams: pagination,
	}

	// Get vm_ids scope from API key (if any)
	vmIDs := middleware.GetVMIDs(c)

	// Optional VM ID filter
	if vmID := c.Query("vm_id"); vmID != "" {
		// Check if API key has access to this VM
		if !middleware.CheckVMScope(c, vmID) {
			return
		}
		// Verify VM belongs to customer
		if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
			if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
				middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
				return
			}
			middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_LIST_FAILED", "Failed to verify VM")
			return
		}
		filter.VMID = &vmID
	} else if len(vmIDs) > 0 {
		// API key has vm_ids restriction, filter results
		filter.VMIDs = vmIDs
	}

	// Optional status filter
	validBackupStatuses := map[string]bool{
		"creating": true, "completed": true, "failed": true, "restoring": true, "deleted": true,
	}
	if status := c.Query("status"); status != "" {
		if !validBackupStatuses[status] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		filter.Status = &status
	}

	// Optional method filter (full or snapshot)
	validBackupMethods := map[string]bool{
		"full": true, "snapshot": true,
	}
	if method := c.Query("method"); method != "" {
		if !validBackupMethods[method] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_METHOD", "Invalid method value. Must be one of: full, snapshot")
			return
		}
		filter.Method = &method
	}

	backups, total, err := h.backupRepo.ListBackupsByCustomer(c.Request.Context(), customerID, filter)
	if err != nil {
		h.logger.Error("failed to list backups",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_LIST_FAILED", "Failed to retrieve backups")
		return
	}

	if pagination.IsCursorBased() {
		hasMore := len(backups) > pagination.PerPage
		if hasMore {
			backups = backups[:pagination.PerPage]
		}
		lastID := ""
		if hasMore && len(backups) > 0 {
			lastID = backups[len(backups)-1].ID
		}
		c.JSON(http.StatusOK, models.ListResponse{
			Data: backups,
			Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
		})
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: backups,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateBackup handles POST /backups - creates a backup for a VM.
// This is an async operation. Returns 202 Accepted with a task_id.
// Quota enforcement is handled atomically in the service layer to prevent race conditions.
// @Tags Customer
// @Summary Create backup
// @Description Performs backup operation for customer resources.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param request body object true "Create backup request"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/backups [post]
func (h *CustomerHandler) CreateBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req CreateBackupRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate UUID
	if _, err := uuid.Parse(req.VMID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Check if API key has access to this VM (vm_ids scope enforcement)
	if !middleware.CheckVMScope(c, req.VMID) {
		return
	}

	// Verify VM belongs to customer
	vm, err := h.vmService.GetVM(c.Request.Context(), req.VMID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_CREATE_FAILED", "Failed to verify VM")
		return
	}

	// Get plan limit for atomic check
	planLimit := defaultBackupLimit
	plan, planErr := h.planRepo.GetByID(c.Request.Context(), vm.PlanID)
	if planErr == nil && plan.BackupLimit > 0 {
		planLimit = plan.BackupLimit
	}

	// Create backup with atomic limit check to prevent race conditions
	backup, err := h.backupService.CreateBackupWithLimitCheck(c.Request.Context(), vm.ID, req.Name, planLimit)
	if err != nil {
		if errors.Is(err, services.ErrBackupLimitExceeded) || strings.Contains(err.Error(), "limit exceeded") {
			middleware.RespondWithError(c, http.StatusConflict, "BACKUP_LIMIT_EXCEEDED",
				fmt.Sprintf("Backup limit reached for this VM (%d max). Delete existing backups first.", planLimit))
			return
		}
		h.logger.Error("failed to create backup",
			"vm_id", req.VMID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_CREATE_FAILED", "Internal server error")
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
// @Tags Customer
// @Summary Get backup
// @Description Performs backup operation for customer resources.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "Backup ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/backups/{id} [get]
func (h *CustomerHandler) GetBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Get backup
	backup, err := h.backupRepo.GetBackupByID(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_GET_FAILED", "Failed to retrieve backup")
		return
	}

	// Check if API key has access to this VM (vm_ids scope enforcement)
	if !middleware.CheckVMScope(c, backup.VMID) {
		return
	}

	// Verify backup belongs to a VM owned by the customer
	if !h.verifyVMOwnership(c.Request.Context(), backup.VMID, customerID) {
		middleware.RespondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: backup})
}

// DeleteBackup handles DELETE /backups/:id - deletes a backup.
// Returns 200 OK on success.
// @Tags Customer
// @Summary Delete backup
// @Description Performs backup operation for customer resources.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "Backup ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/backups/{id} [delete]
func (h *CustomerHandler) DeleteBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Get backup to verify ownership
	backup, err := h.backupRepo.GetBackupByID(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_DELETE_FAILED", "Failed to retrieve backup")
		return
	}

	// Check if API key has access to this VM (vm_ids scope enforcement)
	if !middleware.CheckVMScope(c, backup.VMID) {
		return
	}

	// Verify backup belongs to a VM owned by the customer
	if !h.verifyVMOwnership(c.Request.Context(), backup.VMID, customerID) {
		middleware.RespondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
		return
	}

	// Delete backup
	if err := h.backupService.DeleteBackup(c.Request.Context(), backupID); err != nil {
		h.logger.Error("failed to delete backup",
			"backup_id", backupID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_DELETE_FAILED", "Internal server error")
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
// @Tags Customer
// @Summary Restore backup
// @Description Performs backup operation for customer resources.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "Backup ID"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/backups/{id}/restore [post]
func (h *CustomerHandler) RestoreBackup(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Get backup to verify ownership
	backup, err := h.backupRepo.GetBackupByID(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_RESTORE_FAILED", "Failed to retrieve backup")
		return
	}

	// Check if API key has access to this VM (vm_ids scope enforcement)
	if !middleware.CheckVMScope(c, backup.VMID) {
		return
	}

	// Verify backup belongs to a VM owned by the customer
	if !h.verifyVMOwnership(c.Request.Context(), backup.VMID, customerID) {
		middleware.RespondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
		return
	}

	// Restore backup
	if err := h.backupService.RestoreBackup(c.Request.Context(), backupID); err != nil {
		h.logger.Error("failed to restore backup",
			"backup_id", backupID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_RESTORE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("backup restore initiated via customer API",
		"backup_id", backupID,
		"vm_id", backup.VMID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{Data: gin.H{"message": "Backup restore initiated"}})
}

// verifyVMOwnership verifies that a VM belongs to the customer.
// It returns true if the VM exists and belongs to the customer, false otherwise.
func (h *CustomerHandler) verifyVMOwnership(ctx context.Context, vmID, customerID string) bool {
	_, err := h.vmService.GetVM(ctx, vmID, customerID, false)
	return err == nil
}
