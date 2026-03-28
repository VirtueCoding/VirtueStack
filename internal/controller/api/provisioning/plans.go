package provisioning

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetPlan handles GET /plans/:id - returns plan details for WHMCS integration.
// This endpoint is used by WHMCS to fetch plan resource specs for ChangePackage operations.
// @Tags Provisioning
// @Summary Get plan
// @Description Returns plan details by plan ID.
// @Produce json
// @Security APIKeyAuth
// @Param id path string true "Plan ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/provisioning/plans/{id} [get]
func (h *ProvisioningHandler) GetPlan(c *gin.Context) {
	planID := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(planID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "Plan ID must be a valid UUID")
		return
	}

	plan, err := h.planService.GetByID(c.Request.Context(), planID)
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "PLAN_NOT_FOUND", "Plan not found")
			return
		}
		h.logger.Error("failed to get plan",
			"plan_id", planID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "PLAN_GET_FAILED", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": plan})
}

// ListPlans handles GET /plans - returns all active plans for WHMCS integration.
// This endpoint is used by WHMCS to list available plans for product configuration.
// @Tags Provisioning
// @Summary List plans
// @Description Lists active plans available to provisioning clients.
// @Produce json
// @Security APIKeyAuth
// @Success 200 {object} models.ListResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /api/v1/provisioning/plans [get]
func (h *ProvisioningHandler) ListPlans(c *gin.Context) {
	plans, err := h.planService.ListActive(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list plans",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "PLAN_LIST_FAILED", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": plans})
}