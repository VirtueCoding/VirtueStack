package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// ListAuditLogs handles GET /audit-logs - lists all audit logs with filters.
// Supports filtering by actor, action, resource type, resource ID, and date range.
// @Tags Admin
// @Summary List audit logs
// @Description Lists audit logs with optional filters and pagination.
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Param actor_id query string false "Actor ID"
// @Param action query string false "Action"
// @Param resource_type query string false "Resource type"
// @Success 200 {object} models.ListResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /api/v1/admin/audit-logs [get]
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