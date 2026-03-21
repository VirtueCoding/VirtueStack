// Package admin provides HTTP handlers for the Admin API.
package admin

import (
	"net/http"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// BackupScheduleCreateRequest represents the request body for creating a backup schedule.
type BackupScheduleCreateRequest struct {
	VMID           string `json:"vm_id" validate:"required,uuid"`
	CustomerID     string `json:"customer_id" validate:"required,uuid"`
	Frequency      string `json:"frequency" validate:"required,oneof=daily weekly monthly"`
	RetentionCount int    `json:"retention_count" validate:"required,min=1"`
	Active         bool   `json:"active"`
}

// BackupScheduleUpdateRequest represents the request body for updating a backup schedule.
type BackupScheduleUpdateRequest struct {
	Active    *bool   `json:"active,omitempty"`
	Frequency *string `json:"frequency,omitempty" validate:"omitempty,oneof=daily weekly monthly"`
}

// CreateBackupSchedule handles POST /backup-schedules - creates a new backup schedule.
func (h *AdminHandler) CreateBackupSchedule(c *gin.Context) {
	var req BackupScheduleCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	schedule := &models.BackupSchedule{
		VMID:           req.VMID,
		CustomerID:     req.CustomerID,
		Frequency:      strings.ToLower(strings.TrimSpace(req.Frequency)),
		RetentionCount: req.RetentionCount,
		Active:         req.Active,
	}

	scheduleID, err := h.backupService.CreateSchedule(c.Request.Context(), schedule)
	if err != nil {
		h.logger.Error("failed to create backup schedule",
			"error", err,
			"vm_id", req.VMID,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_CREATE_FAILED", "Failed to create backup schedule")
		return
	}

	h.logAuditEvent(c, "backup_schedule.create", "backup_schedule", scheduleID, req, true)

	h.logger.Info("backup schedule created",
		"schedule_id", scheduleID,
		"vm_id", req.VMID,
		"frequency", schedule.Frequency,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: BackupScheduleCreateResponse{
		ID:        scheduleID,
		NextRunAt: schedule.NextRunAt,
	}})
}

// ListBackupSchedules handles GET /backup-schedules - lists all backup schedules.
func (h *AdminHandler) ListBackupSchedules(c *gin.Context) {
	vmID := c.Query("vm_id")

	if vmID != "" {
		if _, err := uuid.Parse(vmID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "vm_id must be a valid UUID")
			return
		}
	}

	pagination := models.ParsePagination(c)

	schedules, total, err := h.backupService.ListSchedulesPaginated(c.Request.Context(), vmID, pagination)
	if err != nil {
		h.logger.Error("failed to list backup schedules",
			"error", err,
			"vm_id", vmID,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_LIST_FAILED", "Failed to retrieve backup schedules")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: schedules,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// GetBackupSchedule handles GET /backup-schedules/:id - gets a backup schedule by ID.
func (h *AdminHandler) GetBackupSchedule(c *gin.Context) {
	scheduleID := c.Param("id")

	if _, err := uuid.Parse(scheduleID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SCHEDULE_ID", "Schedule ID must be a valid UUID")
		return
	}

	schedule, err := h.backupService.GetSchedule(c.Request.Context(), scheduleID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Backup schedule not found")
			return
		}
		h.logger.Error("failed to get backup schedule",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_GET_FAILED", "Failed to retrieve backup schedule")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: schedule})
}

// UpdateBackupSchedule handles PUT /backup-schedules/:id - updates a backup schedule.
func (h *AdminHandler) UpdateBackupSchedule(c *gin.Context) {
	scheduleID := c.Param("id")

	if _, err := uuid.Parse(scheduleID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SCHEDULE_ID", "Schedule ID must be a valid UUID")
		return
	}

	var req BackupScheduleUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	if req.Active == nil && req.Frequency == nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "NO_UPDATE_FIELDS", "Must provide active or frequency to update")
		return
	}

	if req.Active != nil {
		if err := h.backupService.UpdateSchedule(c.Request.Context(), scheduleID, *req.Active); err != nil {
			if sharederrors.Is(err, sharederrors.ErrNotFound) {
				middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Backup schedule not found")
				return
			}
			h.logger.Error("failed to update backup schedule active",
				"schedule_id", scheduleID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_UPDATE_FAILED", "Failed to update backup schedule")
			return
		}
	}

	if req.Frequency != nil {
		freq := strings.ToLower(strings.TrimSpace(*req.Frequency))
		if freq != "daily" && freq != "weekly" && freq != "monthly" {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_FREQUENCY", "frequency must be daily, weekly, or monthly")
			return
		}

		if err := h.backupService.UpdateScheduleFrequency(c.Request.Context(), scheduleID, freq); err != nil {
			if sharederrors.Is(err, sharederrors.ErrNotFound) {
				middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Backup schedule not found")
				return
			}
			h.logger.Error("failed to update backup schedule frequency",
				"schedule_id", scheduleID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_UPDATE_FAILED", "Failed to update backup schedule frequency")
			return
		}
	}

	h.logAuditEvent(c, "backup_schedule.update", "backup_schedule", scheduleID, req, true)

	h.logger.Info("backup schedule updated",
		"schedule_id", scheduleID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: MessageResponse{Message: "Schedule updated"}})
}

// DeleteBackupSchedule handles DELETE /backup-schedules/:id - deletes a backup schedule.
func (h *AdminHandler) DeleteBackupSchedule(c *gin.Context) {
	scheduleID := c.Param("id")

	if _, err := uuid.Parse(scheduleID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SCHEDULE_ID", "Schedule ID must be a valid UUID")
		return
	}

	if err := h.backupService.DeleteSchedule(c.Request.Context(), scheduleID); err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Backup schedule not found")
			return
		}
		h.logger.Error("failed to delete backup schedule",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_DELETE_FAILED", "Failed to delete backup schedule")
		return
	}

	h.logAuditEvent(c, "backup_schedule.delete", "backup_schedule", scheduleID, nil, true)

	h.logger.Info("backup schedule deleted",
		"schedule_id", scheduleID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}
