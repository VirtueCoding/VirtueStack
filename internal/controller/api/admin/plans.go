package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// ListPlans handles GET /plans - lists all service plans.
func (h *AdminHandler) ListPlans(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := repository.PlanListFilter{
		PaginationParams: pagination,
	}

	// Optional active filter
	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		var isActive bool
		if isActiveStr == "true" {
			isActive = true
		} else if isActiveStr == "false" {
			isActive = false
		} else {
			respondWithError(c, http.StatusBadRequest, "INVALID_IS_ACTIVE", "is_active must be 'true' or 'false'")
			return
		}
		filter.IsActive = &isActive
	}

	plans, total, err := h.planService.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list plans",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "PLAN_LIST_FAILED", "Failed to retrieve plans")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: plans,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreatePlan handles POST /plans - creates a new service plan.
func (h *AdminHandler) CreatePlan(c *gin.Context) {
	var req models.PlanCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	plan, err := h.planService.Create(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("failed to create plan",
			"name", req.Name,
			"slug", req.Slug,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "PLAN_CREATE_FAILED", "Internal server error")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "plan.create", "plan", plan.ID, map[string]interface{}{
		"name":          plan.Name,
		"slug":          plan.Slug,
		"vcpu":          plan.VCPU,
		"memory_mb":     plan.MemoryMB,
		"disk_gb":       plan.DiskGB,
		"price_monthly": plan.PriceMonthly,
	}, true)

	h.logger.Info("plan created via admin API",
		"plan_id", plan.ID,
		"name", plan.Name,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: plan})
}

// UpdatePlan handles PUT /plans/:id - updates an existing plan.
func (h *AdminHandler) UpdatePlan(c *gin.Context) {
	planID, ok := validateUUIDParam(c, "id", "INVALID_PLAN_ID", "Plan ID must be a valid UUID")
	if !ok {
		return
	}

	var req models.PlanUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing plan
	plan, err := h.planService.GetByID(c.Request.Context(), planID)
	if err != nil {
		if handleNotFoundError(c, err, "PLAN_NOT_FOUND", "Plan not found") {
			return
		}
		respondWithError(c, http.StatusInternalServerError, "PLAN_GET_FAILED", "Failed to retrieve plan")
		return
	}

	applyPlanUpdates(plan, req)

	err = h.planService.Update(c.Request.Context(), plan)
	if err != nil {
		h.logger.Error("failed to update plan",
			"plan_id", planID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "PLAN_UPDATE_FAILED", "Internal server error")
		return
	}

	h.logAuditEvent(c, "plan.update", "plan", planID, req, true)

	h.logger.Info("plan updated via admin API",
		"plan_id", planID,
		"name", plan.Name,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: plan})
}

// DeletePlan handles DELETE /plans/:id - deletes a plan.
// Plans with existing VMs cannot be deleted (FK constraint).
func (h *AdminHandler) DeletePlan(c *gin.Context) {
	planID, ok := validateUUIDParam(c, "id", "INVALID_PLAN_ID", "Plan ID must be a valid UUID")
	if !ok {
		return
	}

	err := h.planService.Delete(c.Request.Context(), planID)
	if err != nil {
		if handleNotFoundError(c, err, "PLAN_NOT_FOUND", "Plan not found") {
			return
		}
		h.logger.Error("failed to delete plan",
			"plan_id", planID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		// Check if it's a FK constraint error
		if errors.Is(err, sharederrors.ErrPlanHasExistingVMs) {
			respondWithError(c, http.StatusConflict, "PLAN_IN_USE", "Cannot delete plan with existing VMs")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "PLAN_DELETE_FAILED", "Internal server error")
		return
	}

	h.logAuditEvent(c, "plan.delete", "plan", planID, nil, true)

	h.logger.Info("plan deleted via admin API",
		"plan_id", planID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}
