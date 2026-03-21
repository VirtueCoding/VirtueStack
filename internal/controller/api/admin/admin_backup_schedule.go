// Package admin provides HTTP handlers for the Admin API.
package admin

import (
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminBackupScheduleCreateRequest represents the request body for creating an admin backup schedule.
type AdminBackupScheduleCreateRequest struct {
	Name              string   `json:"name" validate:"required,max=100"`
	Description       string   `json:"description,omitempty" validate:"max=500"`
	Frequency         string   `json:"frequency" validate:"required,oneof=daily weekly monthly"`
	RetentionCount    int      `json:"retention_count" validate:"required,min=1,max=52"`
	TargetAll         bool     `json:"target_all"`
	TargetPlanIDs     []string `json:"target_plan_ids,omitempty" validate:"dive,uuid"`
	TargetNodeIDs     []string `json:"target_node_ids,omitempty" validate:"dive,uuid"`
	TargetCustomerIDs []string `json:"target_customer_ids,omitempty" validate:"dive,uuid"`
	Active            bool     `json:"active"`
}

// AdminBackupScheduleUpdateRequest represents the request body for updating an admin backup schedule.
type AdminBackupScheduleUpdateRequest struct {
	Name              *string  `json:"name,omitempty" validate:"omitempty,max=100"`
	Description       *string  `json:"description,omitempty" validate:"omitempty,max=500"`
	Frequency         *string  `json:"frequency,omitempty" validate:"omitempty,oneof=daily weekly monthly"`
	RetentionCount    *int     `json:"retention_count,omitempty" validate:"omitempty,min=1,max=52"`
	TargetAll         *bool    `json:"target_all,omitempty"`
	TargetPlanIDs     []string `json:"target_plan_ids,omitempty" validate:"dive,uuid"`
	TargetNodeIDs     []string `json:"target_node_ids,omitempty" validate:"dive,uuid"`
	TargetCustomerIDs []string `json:"target_customer_ids,omitempty" validate:"dive,uuid"`
	Active            *bool    `json:"active,omitempty"`
}

// AdminBackupScheduleResponse represents an admin backup schedule in API responses.
type AdminBackupScheduleResponse struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Frequency         string   `json:"frequency"`
	RetentionCount    int      `json:"retention_count"`
	TargetAll         bool     `json:"target_all"`
	TargetPlanIDs     []string `json:"target_plan_ids,omitempty"`
	TargetNodeIDs     []string `json:"target_node_ids,omitempty"`
	TargetCustomerIDs []string `json:"target_customer_ids,omitempty"`
	Active            bool     `json:"active"`
	NextRunAt         string   `json:"next_run_at"`
	LastRunAt         string   `json:"last_run_at,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

// CreateAdminBackupSchedule handles POST /admin-backup-schedules - creates a new admin backup schedule.
func (h *AdminHandler) CreateAdminBackupSchedule(c *gin.Context) {
	var req AdminBackupScheduleCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate that at least one target is specified
	if !req.TargetAll && len(req.TargetPlanIDs) == 0 && len(req.TargetNodeIDs) == 0 && len(req.TargetCustomerIDs) == 0 {
		middleware.RespondWithError(c, http.StatusBadRequest, "NO_TARGET", "At least one target must be specified (target_all, target_plan_ids, target_node_ids, or target_customer_ids)")
		return
	}

	// Calculate next run time based on frequency
	nextRunAt := calculateNextRunTime(req.Frequency, time.Now())

	// Get admin ID from context
	adminID, exists := c.Get("admin_id")
	var createdBy *string
	if exists {
		if id, ok := adminID.(string); ok {
			createdBy = &id
		}
	}

	schedule := &models.AdminBackupSchedule{
		Name:              req.Name,
		Frequency:         req.Frequency,
		RetentionCount:    req.RetentionCount,
		TargetAll:         req.TargetAll,
		TargetPlanIDs:     req.TargetPlanIDs,
		TargetNodeIDs:     req.TargetNodeIDs,
		TargetCustomerIDs: req.TargetCustomerIDs,
		Active:            req.Active,
		NextRunAt:         nextRunAt,
		CreatedBy:         createdBy,
	}

	if req.Description != "" {
		schedule.Description = &req.Description
	}

	if err := h.adminBackupScheduleRepo.Create(c.Request.Context(), schedule); err != nil {
		h.logger.Error("failed to create admin backup schedule",
			"error", err,
			"name", req.Name,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_CREATE_FAILED", "Failed to create admin backup schedule")
		return
	}

	h.logAuditEvent(c, "admin_backup_schedule.create", "admin_backup_schedule", schedule.ID, req, true)

	h.logger.Info("admin backup schedule created",
		"schedule_id", schedule.ID,
		"name", req.Name,
		"frequency", schedule.Frequency,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: toAdminBackupScheduleResponse(*schedule)})
}

// ListAdminBackupSchedules handles GET /admin-backup-schedules - lists all admin backup schedules.
func (h *AdminHandler) ListAdminBackupSchedules(c *gin.Context) {
	pagination := models.ParsePagination(c)

	var activeFilter *bool
	if activeStr := c.Query("active"); activeStr != "" {
		active := activeStr == "true"
		activeFilter = &active
	}

	filter := repository.AdminBackupScheduleListFilter{
		PaginationParams: pagination,
		Active:           activeFilter,
	}

	schedules, total, err := h.adminBackupScheduleRepo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list admin backup schedules",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_LIST_FAILED", "Failed to retrieve admin backup schedules")
		return
	}

	responses := make([]AdminBackupScheduleResponse, len(schedules))
	for i, s := range schedules {
		responses[i] = toAdminBackupScheduleResponse(s)
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: responses,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// GetAdminBackupSchedule handles GET /admin-backup-schedules/:id - gets an admin backup schedule by ID.
func (h *AdminHandler) GetAdminBackupSchedule(c *gin.Context) {
	scheduleID := c.Param("id")

	if _, err := uuid.Parse(scheduleID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SCHEDULE_ID", "Schedule ID must be a valid UUID")
		return
	}

	schedule, err := h.adminBackupScheduleRepo.GetByID(c.Request.Context(), scheduleID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Admin backup schedule not found")
			return
		}
		h.logger.Error("failed to get admin backup schedule",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_GET_FAILED", "Failed to retrieve admin backup schedule")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: toAdminBackupScheduleResponse(*schedule)})
}

// UpdateAdminBackupSchedule handles PUT /admin-backup-schedules/:id - updates an admin backup schedule.
func (h *AdminHandler) UpdateAdminBackupSchedule(c *gin.Context) {
	scheduleID := c.Param("id")

	if _, err := uuid.Parse(scheduleID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SCHEDULE_ID", "Schedule ID must be a valid UUID")
		return
	}

	var req AdminBackupScheduleUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing schedule
	schedule, err := h.adminBackupScheduleRepo.GetByID(c.Request.Context(), scheduleID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Admin backup schedule not found")
			return
		}
		h.logger.Error("failed to get admin backup schedule for update",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_GET_FAILED", "Failed to retrieve admin backup schedule")
		return
	}

	// Apply updates
	if req.Name != nil {
		schedule.Name = *req.Name
	}
	if req.Description != nil {
		schedule.Description = req.Description
	}
	if req.Frequency != nil {
		schedule.Frequency = *req.Frequency
		schedule.NextRunAt = calculateNextRunTime(*req.Frequency, time.Now())
	}
	if req.RetentionCount != nil {
		schedule.RetentionCount = *req.RetentionCount
	}
	if req.TargetAll != nil {
		schedule.TargetAll = *req.TargetAll
	}
	if req.TargetPlanIDs != nil {
		schedule.TargetPlanIDs = req.TargetPlanIDs
	}
	if req.TargetNodeIDs != nil {
		schedule.TargetNodeIDs = req.TargetNodeIDs
	}
	if req.TargetCustomerIDs != nil {
		schedule.TargetCustomerIDs = req.TargetCustomerIDs
	}
	if req.Active != nil {
		schedule.Active = *req.Active
	}

	// Validate that at least one target is specified
	if !schedule.TargetAll && len(schedule.TargetPlanIDs) == 0 && len(schedule.TargetNodeIDs) == 0 && len(schedule.TargetCustomerIDs) == 0 {
		middleware.RespondWithError(c, http.StatusBadRequest, "NO_TARGET", "At least one target must be specified")
		return
	}

	if err := h.adminBackupScheduleRepo.Update(c.Request.Context(), schedule); err != nil {
		h.logger.Error("failed to update admin backup schedule",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_UPDATE_FAILED", "Failed to update admin backup schedule")
		return
	}

	h.logAuditEvent(c, "admin_backup_schedule.update", "admin_backup_schedule", scheduleID, req, true)

	h.logger.Info("admin backup schedule updated",
		"schedule_id", scheduleID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: toAdminBackupScheduleResponse(*schedule)})
}

// DeleteAdminBackupSchedule handles DELETE /admin-backup-schedules/:id - deletes an admin backup schedule.
func (h *AdminHandler) DeleteAdminBackupSchedule(c *gin.Context) {
	scheduleID := c.Param("id")

	if _, err := uuid.Parse(scheduleID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SCHEDULE_ID", "Schedule ID must be a valid UUID")
		return
	}

	if err := h.adminBackupScheduleRepo.Delete(c.Request.Context(), scheduleID); err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Admin backup schedule not found")
			return
		}
		h.logger.Error("failed to delete admin backup schedule",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_DELETE_FAILED", "Failed to delete admin backup schedule")
		return
	}

	h.logAuditEvent(c, "admin_backup_schedule.delete", "admin_backup_schedule", scheduleID, nil, true)

	h.logger.Info("admin backup schedule deleted",
		"schedule_id", scheduleID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}

// RunAdminBackupSchedule handles POST /admin-backup-schedules/:id/run - triggers immediate execution of a schedule.
func (h *AdminHandler) RunAdminBackupSchedule(c *gin.Context) {
	scheduleID := c.Param("id")

	if _, err := uuid.Parse(scheduleID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SCHEDULE_ID", "Schedule ID must be a valid UUID")
		return
	}

	schedule, err := h.adminBackupScheduleRepo.GetByID(c.Request.Context(), scheduleID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Admin backup schedule not found")
			return
		}
		h.logger.Error("failed to get admin backup schedule for run",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_GET_FAILED", "Failed to retrieve admin backup schedule")
		return
	}

	// Update the next run time to now to trigger execution
	now := time.Now()
	nextRun := calculateNextRunTime(schedule.Frequency, now)
	if err := h.adminBackupScheduleRepo.UpdateNextRunAt(c.Request.Context(), scheduleID, now, now); err != nil {
		h.logger.Error("failed to trigger admin backup schedule run",
			"schedule_id", scheduleID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SCHEDULE_RUN_FAILED", "Failed to trigger schedule run")
		return
	}

	// Update the schedule's next_run_at for the response
	schedule.NextRunAt = nextRun

	h.logAuditEvent(c, "admin_backup_schedule.run", "admin_backup_schedule", scheduleID, nil, true)

	h.logger.Info("admin backup schedule triggered",
		"schedule_id", scheduleID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{
		"message":     "Schedule execution triggered",
		"next_run_at": nextRun,
	}})
}

// toAdminBackupScheduleResponse converts an AdminBackupSchedule to a response struct.
func toAdminBackupScheduleResponse(s models.AdminBackupSchedule) AdminBackupScheduleResponse {
	resp := AdminBackupScheduleResponse{
		ID:             s.ID,
		Name:           s.Name,
		Frequency:      s.Frequency,
		RetentionCount: s.RetentionCount,
		TargetAll:      s.TargetAll,
		TargetPlanIDs:  s.TargetPlanIDs,
		TargetNodeIDs:  s.TargetNodeIDs,
		Active:         s.Active,
		NextRunAt:      s.NextRunAt.Format(time.RFC3339),
		CreatedAt:      s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      s.UpdatedAt.Format(time.RFC3339),
	}

	if s.Description != nil {
		resp.Description = *s.Description
	}
	if s.TargetCustomerIDs != nil {
		resp.TargetCustomerIDs = s.TargetCustomerIDs
	}
	if s.LastRunAt != nil {
		resp.LastRunAt = s.LastRunAt.Format(time.RFC3339)
	}

	return resp
}

// calculateNextRunTime calculates the next run time based on frequency.
func calculateNextRunTime(frequency string, from time.Time) time.Time {
	switch frequency {
	case "daily":
		return from.Add(24 * time.Hour)
	case "weekly":
		return from.Add(7 * 24 * time.Hour)
	case "monthly":
		return from.AddDate(0, 1, 0)
	default:
		return from.Add(24 * time.Hour)
	}
}