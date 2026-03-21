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

	// Get query parameters
	customerID := c.Query("customer_id")
	vmID := c.Query("vm_id")
	status := c.Query("status")
	source := c.Query("source")
	adminScheduleID := c.Query("admin_schedule_id")

	// Validate UUID query parameters
	if customerID != "" {
		if _, err := uuid.Parse(customerID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "customer_id must be a valid UUID")
			return
		}
	}
	if vmID != "" {
		if _, err := uuid.Parse(vmID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "vm_id must be a valid UUID")
			return
		}
	}
	if adminScheduleID != "" {
		if _, err := uuid.Parse(adminScheduleID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_ADMIN_SCHEDULE_ID", "admin_schedule_id must be a valid UUID")
			return
		}
	}

	// Validate status against known enum values
	validBackupStatuses := map[string]bool{
		"creating": true, "completed": true, "failed": true, "restoring": true, "deleted": true,
	}
	if status != "" && !validBackupStatuses[status] {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
		return
	}

	// Validate source against known enum values
	validBackupSources := map[string]bool{
		"manual": true, "customer_schedule": true, "admin_schedule": true,
	}
	if source != "" && !validBackupSources[source] {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SOURCE", "Invalid source value. Must be one of: manual, customer_schedule, admin_schedule")
		return
	}

	// Build filter
	filter := AdminBackupListFilter{
		CustomerID:      &customerID,
		VMID:            &vmID,
		Status:          &status,
		Source:          &source,
		AdminScheduleID: &adminScheduleID,
	}

	// Clear nil pointers for empty strings
	if customerID == "" {
		filter.CustomerID = nil
	}
	if vmID == "" {
		filter.VMID = nil
	}
	if status == "" {
		filter.Status = nil
	}
	if source == "" {
		filter.Source = nil
	}
	if adminScheduleID == "" {
		filter.AdminScheduleID = nil
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
