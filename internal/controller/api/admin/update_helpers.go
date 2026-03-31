package admin

import (
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// applyPlanUpdates applies the updates from PlanUpdateRequest to the plan.
// Returns an error if validation fails.
func applyPlanUpdates(plan *models.Plan, req models.PlanUpdateRequest) {
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
		plan.PriceMonthly = req.PriceMonthly
	}
	if req.PriceHourly != nil {
		plan.PriceHourly = req.PriceHourly
	}
	if req.PriceHourlyStopped != nil {
		plan.PriceHourlyStopped = req.PriceHourlyStopped
	}
	if req.Currency != nil {
		plan.Currency = *req.Currency
	}
	if req.IsActive != nil {
		plan.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		plan.SortOrder = *req.SortOrder
	}
	if req.StorageBackend != nil {
		plan.StorageBackend = *req.StorageBackend
	}
	if req.SnapshotLimit != nil {
		plan.SnapshotLimit = *req.SnapshotLimit
	}
	if req.BackupLimit != nil {
		plan.BackupLimit = *req.BackupLimit
	}
	if req.ISOUploadLimit != nil {
		plan.ISOUploadLimit = *req.ISOUploadLimit
	}
}

// applyNodeUpdates applies the updates from NodeUpdateRequest to the node.
func applyNodeUpdates(node *models.Node, req NodeUpdateRequest) {
	if req.GRPCAddress != nil {
		node.GRPCAddress = *req.GRPCAddress
	}
	if req.LocationID != nil {
		node.LocationID = req.LocationID
	}
	if req.TotalVCPU != nil {
		node.TotalVCPU = *req.TotalVCPU
	}
	if req.TotalMemory != nil {
		node.TotalMemoryMB = *req.TotalMemory
	}
	if req.IPMIAddress != nil {
		node.IPMIAddress = req.IPMIAddress
	}
	if req.StorageBackend != nil {
		node.StorageBackend = *req.StorageBackend
	}
	if req.StoragePath != nil {
		node.StoragePath = *req.StoragePath
	}
}

// parseAuditLogFilter parses query parameters into an AuditLogFilter.
// Returns an error response if any parameter is invalid.
func parseAuditLogFilter(c *gin.Context, pagination models.PaginationParams) (*models.AuditLogFilter, bool) {
	filter := &models.AuditLogFilter{
		PaginationParams: pagination,
	}

	if actorID := c.Query("actor_id"); actorID != "" {
		filter.ActorID = &actorID
	}

	if actorType := c.Query("actor_type"); actorType != "" {
		filter.ActorType = &actorType
	}

	if action := c.Query("action"); action != "" {
		filter.Action = &action
	}

	if resourceType := c.Query("resource_type"); resourceType != "" {
		filter.ResourceType = &resourceType
	}

	if resourceID := c.Query("resource_id"); resourceID != "" {
		filter.ResourceID = &resourceID
	}

	if successStr := c.Query("success"); successStr != "" {
		if successStr == "true" {
			success := true
			filter.Success = &success
		} else if successStr == "false" {
			success := false
			filter.Success = &success
		}
	}

	if startDateStr := c.Query("start_date"); startDateStr != "" {
		startDate, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_DATE_FORMAT", "Invalid date format, use RFC3339")
			return nil, false
		}
		filter.StartTime = &startDate
	}

	if endDateStr := c.Query("end_date"); endDateStr != "" {
		endDate, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_DATE_FORMAT", "Invalid date format, use RFC3339")
			return nil, false
		}
		filter.EndTime = &endDate
	}

	return filter, true
}

// validateUUIDParam validates that a path parameter is a valid UUID.
// Returns false if validation fails (error response already sent).
func validateUUIDParam(c *gin.Context, param, errorCode, errorMsg string) (string, bool) {
	id := c.Param(param)
	if _, err := uuid.Parse(id); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, errorCode, errorMsg)
		return "", false
	}
	return id, true
}

// handleNotFoundError checks if the error is ErrNotFound and sends appropriate response.
// Returns true if the error was handled (response sent), false otherwise.
func handleNotFoundError(c *gin.Context, err error, notFoundCode, notFoundMsg string) bool {
	if sharederrors.Is(err, sharederrors.ErrNotFound) {
		middleware.RespondWithError(c, http.StatusNotFound, notFoundCode, notFoundMsg)
		return true
	}
	return false
}