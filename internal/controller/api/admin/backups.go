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
	CustomerID *string `form:"customer_id"`
	VMID       *string `form:"vm_id"`
	Status     *string `form:"status"`
}

// ListBackups handles GET /backups - lists all backups across all customers.
func (h *AdminHandler) ListBackups(c *gin.Context) {
	pagination := models.ParsePagination(c)

	// Get query parameters
	customerID := c.Query("customer_id")
	vmID := c.Query("vm_id")
	status := c.Query("status")

	// Validate UUID query parameters
	if customerID != "" {
		if _, err := uuid.Parse(customerID); err != nil {
			respondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "customer_id must be a valid UUID")
			return
		}
	}
	if vmID != "" {
		if _, err := uuid.Parse(vmID); err != nil {
			respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "vm_id must be a valid UUID")
			return
		}
	}

	// Validate status against known enum values
	validBackupStatuses := map[string]bool{
		"creating": true, "completed": true, "failed": true, "restoring": true, "deleted": true,
	}
	if status != "" && !validBackupStatuses[status] {
		respondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
		return
	}

	// Build filter
	filter := AdminBackupListFilter{
		CustomerID: &customerID,
		VMID:       &vmID,
		Status:     &status,
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

	repoFilter := repository.BackupListFilter{
		PaginationParams: pagination,
		VMID:             filter.VMID,
		Status:           filter.Status,
	}
	repoFilter.Type = nil

	backups, total, err := h.backupService.ListBackupsWithFilter(c.Request.Context(), filter.CustomerID, repoFilter)
	if err != nil {
		h.logger.Error("failed to list backups",
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

// RestoreBackup handles POST /backups/:id/restore - restores a backup (admin override).
// Admins can restore any backup regardless of ownership.
func (h *AdminHandler) RestoreBackup(c *gin.Context) {
	backupID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(backupID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_BACKUP_ID", "Backup ID must be a valid UUID")
		return
	}

	// Restore backup through service
	err := h.backupService.RestoreBackup(c.Request.Context(), backupID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "BACKUP_NOT_FOUND", "Backup not found")
			return
		}
		h.logger.Error("failed to restore backup",
			"backup_id", backupID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "BACKUP_RESTORE_FAILED", "Internal server error")
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
