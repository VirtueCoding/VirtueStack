package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/common"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListFailoverRequests handles GET /failover-requests - lists all failover requests.
// Supports filtering by node_id and status.
// @Tags Admin
// @Summary List failover requests
// @Description Lists failover requests with optional pagination and filters.
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Success 200 {object} models.ListResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /api/v1/admin/failover-requests [get]
func (h *AdminHandler) ListFailoverRequests(c *gin.Context) {
	pagination := common.ParsePaginationParams(c)

	filter := models.FailoverRequestListFilter{
		PaginationParams: pagination,
	}

	if nodeID := c.Query("node_id"); nodeID != "" {
		if _, err := uuid.Parse(nodeID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "node_id must be a valid UUID")
			return
		}
		filter.NodeID = &nodeID
	}
	validFailoverStatuses := map[string]bool{
		"pending": true, "approved": true, "in_progress": true, "completed": true, "failed": true, "cancelled": true,
	}
	if status := c.Query("status"); status != "" {
		if !validFailoverStatuses[status] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		filter.Status = &status
	}

	if h.failoverRepo == nil {
		middleware.RespondWithError(c, http.StatusServiceUnavailable, "FAILOVER_UNAVAILABLE", "Failover repository not available")
		return
	}

	requests, hasMore, lastID, err := h.failoverRepo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list failover requests",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "FAILOVER_REQUEST_LIST_FAILED", "Failed to retrieve failover requests")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: requests,
		Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
	})
}

// GetFailoverRequest handles GET /failover-requests/:id - retrieves a specific failover request.
// @Tags Admin
// @Summary Get failover request
// @Description Returns details for a single failover request.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Failover request ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/failover-requests/{id} [get]
func (h *AdminHandler) GetFailoverRequest(c *gin.Context) {
	requestID := c.Param("id")

	if _, err := uuid.Parse(requestID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_REQUEST_ID", "Request ID must be a valid UUID")
		return
	}

	if h.failoverRepo == nil {
		middleware.RespondWithError(c, http.StatusServiceUnavailable, "FAILOVER_UNAVAILABLE", "Failover repository not available")
		return
	}

	req, err := h.failoverRepo.GetByID(c.Request.Context(), requestID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "FAILOVER_REQUEST_NOT_FOUND", "Failover request not found")
			return
		}
		h.logger.Error("failed to get failover request",
			"request_id", requestID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "FAILOVER_REQUEST_GET_FAILED", "Failed to retrieve failover request")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: req})
}
