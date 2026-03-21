package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// ListAuditLogs handles GET /audit-logs - lists all audit logs with filters.
// Supports filtering by actor, action, resource type, resource ID, and date range.
func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter, ok := parseAuditLogFilter(c, pagination)
	if !ok {
		return
	}

	logs, total, err := h.auditRepo.List(c.Request.Context(), *filter)
	if err != nil {
		h.logger.Error("failed to list audit logs",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "AUDIT_LOG_LIST_FAILED", "Failed to retrieve audit logs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: logs,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}