package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminBackupListFilter represents filter options for listing all backups.
type AdminBackupListFilter struct {
	CustomerID       *string `form:"customer_id"`
	VMID             *string `form:"vm_id"`
	Status           *string `form:"status"`
	Source           *string `form:"source"`
	AdminScheduleID  *string `form:"admin_schedule_id"`
}

// ListBackups handles GET /backups - lists all backups across all customers.
func (h *AdminHandler) ListBackups(c *gin.Context) {
	pagination := models.ParsePagination(c)

	// Validate UUID query parameters before building the filter
	if customerIDStr := c.Query("customer_id"); customerIDStr != "" {
		if _, err := uuid.Parse(customerIDStr); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "customer_id must be a valid UUID")
			return
		}
	}
	if vmIDStr := c.Query("vm_id"); vmIDStr != "" {
		if _, err := uuid.Parse(vmIDStr); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "vm_id must be a valid UUID")
			return
		}
	}
	if adminScheduleIDStr := c.Query("admin_schedule_id"); adminScheduleIDStr != "" {
		if _, err := uuid.Parse(adminScheduleIDStr); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_ADMIN_SCHEDULE_ID", "admin_schedule_id must be a valid UUID")
			return
		}
	}

	// Validate status against known enum values
	validBackupStatuses := map[string]bool{
		"creating": true, "completed": true, "failed": true, "restoring": true, "deleted": true,
	}
	if statusStr := c.Query("status"); statusStr != "" && !validBackupStatuses[statusStr] {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
		return
	}

	// Validate source against known enum values
	validBackupSources := map[string]bool{
		"manual": true, "customer_schedule": true, "admin_schedule": true,
	}
	if sourceStr := c.Query("source"); sourceStr != "" && !validBackupSources[sourceStr] {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SOURCE", "Invalid source value. Must be one of: manual, customer_schedule, admin_schedule")
		return
	}

	// Build filter using single-step nil assignment (F-185)
	var filter AdminBackupListFilter
	if v := c.Query("customer_id"); v != "" {
		filter.CustomerID = &v
	}
	if v := c.Query("vm_id"); v != "" {
		filter.VMID = &v
	}
	if v := c.Query("status"); v != "" {
		filter.Status = &v
	}
	if v := c.Query("source"); v != "" {
		filter.Source = &v
	}
	if v := c.Query("admin_schedule_id"); v != "" {
		filter.AdminScheduleID = &v
	}

	repoFilter := repository.BackupListFilter{
		PaginationParams: pagination,
		VMID:             filter.VMID,
		Status:           filter.Status,
		Source:           filter.Source,
		AdminScheduleID:  filter.AdminScheduleID,
	}

	backups, total, err := h.backupService.ListBackupsWithFilter(c.Request.Context(), filter.CustomerID, repoFilter)
	if err != nil {
		h.logger.Error("failed to list backups",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_LIST_FAILED", "Failed to retrieve backups")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: backups,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// RestoreBackup handles POST /backups/:id/restore - restores a backup (admin override).
// Admins can restore any backup regardless of ownership.
func (h *AdminHandler) RestoreBackup(c *gin.Context) {
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Restore backup through service
	err := h.backupService.RestoreBackup(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		h.logger.Error("failed to restore backup",
			"backup_id", backupID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "BACKUP_RESTORE_FAILED", "Internal server error")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "backup.restore", "backup", backupID, map[string]interface{}{
		"admin_override": true,
	}, true)

	h.logger.Info("backup restored via admin API",
		"backup_id", backupID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: BackupRestoreResponse{
		BackupID: backupID,
		Status:   "restored",
	}})
}
