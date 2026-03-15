package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *AdminHandler) ListFailoverRequests(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := models.FailoverRequestListFilter{
		PaginationParams: pagination,
	}

	if nodeID := c.Query("node_id"); nodeID != "" {
		filter.NodeID = &nodeID
	}
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	if h.failoverRepo == nil {
		respondWithError(c, http.StatusServiceUnavailable, "FAILOVER_UNAVAILABLE", "Failover repository not available")
		return
	}

	requests, total, err := h.failoverRepo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list failover requests",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "FAILOVER_REQUEST_LIST_FAILED", "Failed to retrieve failover requests")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: requests,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

func (h *AdminHandler) GetFailoverRequest(c *gin.Context) {
	requestID := c.Param("id")

	if _, err := uuid.Parse(requestID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_REQUEST_ID", "Request ID must be a valid UUID")
		return
	}

	if h.failoverRepo == nil {
		respondWithError(c, http.StatusServiceUnavailable, "FAILOVER_UNAVAILABLE", "Failover repository not available")
		return
	}

	req, err := h.failoverRepo.GetByID(c.Request.Context(), requestID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "FAILOVER_REQUEST_NOT_FOUND", "Failover request not found")
			return
		}
		h.logger.Error("failed to get failover request",
			"request_id", requestID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "FAILOVER_REQUEST_GET_FAILED", "Failed to retrieve failover request")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: req})
}
