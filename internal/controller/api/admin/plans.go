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
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	plan, err := h.planService.Create(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("failed to create plan",
			"name", req.Name,
			"slug", req.Slug,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "PLAN_CREATE_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "plan.create", "plan", plan.ID, map[string]interface{}{
		"name":              plan.Name,
		"slug":              plan.Slug,
		"vcpu":              plan.VCPU,
		"memory_mb":         plan.MemoryMB,
		"disk_gb":           plan.DiskGB,
		"price_monthly":     plan.PriceMonthly,
	}, true)

	h.logger.Info("plan created via admin API",
		"plan_id", plan.ID,
		"name", plan.Name,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: plan})
}

// UpdatePlan handles PUT /plans/:id - updates an existing plan.
func (h *AdminHandler) UpdatePlan(c *gin.Context) {
	planID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(planID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "Plan ID must be a valid UUID")
		return
	}

	var req models.PlanUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Get existing plan
	plan, err := h.planService.GetByID(c.Request.Context(), planID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "PLAN_NOT_FOUND", "Plan not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "PLAN_GET_FAILED", "Failed to retrieve plan")
		return
	}

	// Apply updates
	if req.Name != nil {
		plan.Name = *req.Name
	}
	if req.Slug != nil {
		plan.Slug = *req.Slug
	}
	if req.VCPU != nil {
		plan.VCPU = *req.VCPU
	}
	if req.MemoryMB != nil {
		plan.MemoryMB = *req.MemoryMB
	}
	if req.DiskGB != nil {
		plan.DiskGB = *req.DiskGB
	}
	if req.BandwidthLimitGB != nil {
		plan.BandwidthLimitGB = *req.BandwidthLimitGB
	}
	if req.PortSpeedMbps != nil {
		plan.PortSpeedMbps = *req.PortSpeedMbps
	}
	if req.PriceMonthly != nil {
		plan.PriceMonthly = *req.PriceMonthly
	}
	if req.PriceHourly != nil {
		plan.PriceHourly = *req.PriceHourly
	}
	if req.IsActive != nil {
		plan.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		plan.SortOrder = *req.SortOrder
	}

	err = h.planService.Update(c.Request.Context(), plan)
	if err != nil {
		h.logger.Error("failed to update plan",
			"plan_id", planID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "PLAN_UPDATE_FAILED", err.Error())
		return
	}

	// Log audit event
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
	planID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(planID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "Plan ID must be a valid UUID")
		return
	}

	err := h.planService.Delete(c.Request.Context(), planID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "PLAN_NOT_FOUND", "Plan not found")
			return
		}
		h.logger.Error("failed to delete plan",
			"plan_id", planID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		// Check if it's a FK constraint error
		if err.Error() == "plan has existing VMs" {
			respondWithError(c, http.StatusConflict, "PLAN_IN_USE", "Cannot delete plan with existing VMs")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "PLAN_DELETE_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "plan.delete", "plan", planID, nil, true)

	h.logger.Info("plan deleted via admin API",
		"plan_id", planID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"deleted": true}})
}