package admin

import (
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// ListAuditLogs handles GET /audit-logs - lists all audit logs with filters.
// Supports filtering by actor, action, resource type, resource ID, and date range.
func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := models.AuditLogFilter{
		PaginationParams: pagination,
	}

	// Optional actor_id filter
	if actorID := c.Query("actor_id"); actorID != "" {
		filter.ActorID = &actorID
	}

	// Optional actor_type filter
	if actorType := c.Query("actor_type"); actorType != "" {
		filter.ActorType = &actorType
	}

	// Optional action filter
	if action := c.Query("action"); action != "" {
		filter.Action = &action
	}

	// Optional resource_type filter
	if resourceType := c.Query("resource_type"); resourceType != "" {
		filter.ResourceType = &resourceType
	}

	// Optional resource_id filter
	if resourceID := c.Query("resource_id"); resourceID != "" {
		filter.ResourceID = &resourceID
	}

	// Optional success filter
	if successStr := c.Query("success"); successStr != "" {
		if successStr == "true" {
			success := true
			filter.Success = &success
		} else if successStr == "false" {
			success := false
			filter.Success = &success
		}
	}

	// Optional start_date filter (ISO 8601 format)
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		startDate, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			respondWithError(c, http.StatusBadRequest, "INVALID_DATE_FORMAT", "Invalid date format, use RFC3339")
			return
		}
		filter.StartTime = &startDate
	}

	// Optional end_date filter (ISO 8601 format)
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		endDate, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			respondWithError(c, http.StatusBadRequest, "INVALID_DATE_FORMAT", "Invalid date format, use RFC3339")
			return
		}
		filter.EndTime = &endDate
	}

	logs, total, err := h.auditRepo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list audit logs",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "AUDIT_LOG_LIST_FAILED", "Failed to retrieve audit logs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: logs,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}