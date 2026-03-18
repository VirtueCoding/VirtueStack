package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// IPSetUpdateRequest represents the request body for updating an IP set.
type IPSetUpdateRequest struct {
	Name       *string   `json:"name,omitempty" validate:"omitempty,max=100"`
	LocationID *string   `json:"location_id,omitempty" validate:"omitempty,uuid"`
	Gateway    *string   `json:"gateway,omitempty" validate:"omitempty,ip"`
	VLANID     *int      `json:"vlan_id,omitempty" validate:"omitempty,min=1,max=4094"`
	NodeIDs    *[]string `json:"node_ids,omitempty" validate:"dive,uuid"`
}

// IPSetDetail represents an IP set with additional statistics.
type IPSetDetail struct {
	models.IPSet
	TotalIPs     int `json:"total_ips"`
	AssignedIPs  int `json:"assigned_ips"`
	AvailableIPs int `json:"available_ips"`
	ReservedIPs  int `json:"reserved_ips"`
	CooldownIPs  int `json:"cooldown_ips"`
}

// ListIPSets handles GET /ip-sets - lists all IP sets.
func (h *AdminHandler) ListIPSets(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := repository.IPSetListFilter{
		PaginationParams: pagination,
	}

	// Optional location filter
	if locationID := c.Query("location_id"); locationID != "" {
		if _, err := uuid.Parse(locationID); err != nil {
			respondWithError(c, http.StatusBadRequest, "INVALID_LOCATION_ID", "location_id must be a valid UUID")
			return
		}
		filter.LocationID = &locationID
	}

	// Optional IP version filter
	if ipVersionStr := c.Query("ip_version"); ipVersionStr != "" {
		if ipVersionStr == "4" || ipVersionStr == "6" {
			ipVersion := int16(4)
			if ipVersionStr == "6" {
				ipVersion = 6
			}
			filter.IPVersion = &ipVersion
		}
	}

	ipSets, total, err := h.ipRepo.ListIPSets(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list IP sets",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "IPSET_LIST_FAILED", "Failed to retrieve IP sets")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: ipSets,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateIPSet handles POST /ip-sets - creates a new IP set.
func (h *AdminHandler) CreateIPSet(c *gin.Context) {
	var req models.IPSetCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	ipSet := &models.IPSet{
		Name:       req.Name,
		LocationID: req.LocationID,
		Network:    req.Network,
		Gateway:    req.Gateway,
		VLANID:     req.VlanID,
		IPVersion:  int16(req.IPVersion),
		NodeIDs:    req.NodeIDs,
	}

	err := h.ipRepo.CreateIPSet(c.Request.Context(), ipSet)
	if err != nil {
		h.logger.Error("failed to create IP set",
			"name", req.Name,
			"network", req.Network,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "IPSET_CREATE_FAILED", "Internal server error")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "ipset.create", "ipset", ipSet.ID, map[string]interface{}{
		"name":       req.Name,
		"network":    req.Network,
		"gateway":    req.Gateway,
		"ip_version": req.IPVersion,
	}, true)

	h.logger.Info("IP set created via admin API",
		"ipset_id", ipSet.ID,
		"name", req.Name,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: ipSet})
}

// GetIPSet handles GET /ip-sets/:id - retrieves details for a specific IP set.
func (h *AdminHandler) GetIPSet(c *gin.Context) {
	ipSetID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(ipSetID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_IPSET_ID", "IP Set ID must be a valid UUID")
		return
	}

	ipSet, err := h.ipRepo.GetIPSetByID(c.Request.Context(), ipSetID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "IPSET_NOT_FOUND", "IP Set not found")
			return
		}
		h.logger.Error("failed to get IP set",
			"ipset_id", ipSetID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "IPSET_GET_FAILED", "Failed to retrieve IP set")
		return
	}

	// Count IPs by status using DB aggregation — efficient for large sets (e.g. /16 = 65K IPs).
	ipCounts, err := h.ipRepo.CountIPsByStatus(c.Request.Context(), ipSetID)
	if err != nil {
		h.logger.Warn("failed to get IP statistics for IP set",
			"ipset_id", ipSetID,
			"error", err)
		// ipCounts will be nil; the fields below will default to 0.
		ipCounts = map[string]int{}
	}

	totalIPs := 0
	for _, cnt := range ipCounts {
		totalIPs += cnt
	}

	detail := IPSetDetail{
		IPSet:        *ipSet,
		TotalIPs:     totalIPs,
		AssignedIPs:  ipCounts["assigned"],
		AvailableIPs: ipCounts["available"],
		ReservedIPs:  ipCounts["reserved"],
		CooldownIPs:  ipCounts["cooldown"],
	}

	c.JSON(http.StatusOK, models.Response{Data: detail})
}

// UpdateIPSet handles PUT /ip-sets/:id - updates an existing IP set.
func (h *AdminHandler) UpdateIPSet(c *gin.Context) {
	ipSetID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(ipSetID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_IPSET_ID", "IP Set ID must be a valid UUID")
		return
	}

	var req IPSetUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing IP set
	ipSet, err := h.ipRepo.GetIPSetByID(c.Request.Context(), ipSetID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "IPSET_NOT_FOUND", "IP Set not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "IPSET_GET_FAILED", "Failed to retrieve IP set")
		return
	}

	// Apply updates
	if req.Name != nil {
		ipSet.Name = *req.Name
	}
	if req.LocationID != nil {
		ipSet.LocationID = req.LocationID
	}
	if req.Gateway != nil {
		ipSet.Gateway = *req.Gateway
	}
	if req.VLANID != nil {
		ipSet.VLANID = req.VLANID
	}
	if req.NodeIDs != nil {
		ipSet.NodeIDs = *req.NodeIDs
	}

	err = h.ipRepo.UpdateIPSet(c.Request.Context(), ipSet)
	if err != nil {
		h.logger.Error("failed to update IP set",
			"ipset_id", ipSetID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "IPSET_UPDATE_FAILED", "Internal server error")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "ipset.update", "ipset", ipSetID, req, true)

	h.logger.Info("IP set updated via admin API",
		"ipset_id", ipSetID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: ipSet})
}

// DeleteIPSet handles DELETE /ip-sets/:id - deletes an IP set.
// IP sets with assigned IPs cannot be deleted.
func (h *AdminHandler) DeleteIPSet(c *gin.Context) {
	ipSetID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(ipSetID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_IPSET_ID", "IP Set ID must be a valid UUID")
		return
	}

	// Check for assigned IPs
	assignedFilter := repository.IPAddressListFilter{IPSetID: &ipSetID, Status: util.StringPtr("assigned")}
	assignedIPs, _, err := h.ipRepo.ListIPAddresses(c.Request.Context(), assignedFilter)
	if err == nil && len(assignedIPs) > 0 {
		respondWithError(c, http.StatusConflict, "IPSET_IN_USE", "Cannot delete IP set with assigned IPs")
		return
	}

	err = h.ipRepo.DeleteIPSet(c.Request.Context(), ipSetID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "IPSET_NOT_FOUND", "IP Set not found")
			return
		}
		h.logger.Error("failed to delete IP set",
			"ipset_id", ipSetID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "IPSET_DELETE_FAILED", "Internal server error")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "ipset.delete", "ipset", ipSetID, nil, true)

	h.logger.Info("IP set deleted via admin API",
		"ipset_id", ipSetID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}

// ListAvailableIPs handles GET /ip-sets/:id/available - lists available IPs in an IP set.
func (h *AdminHandler) ListAvailableIPs(c *gin.Context) {
	ipSetID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(ipSetID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_IPSET_ID", "IP Set ID must be a valid UUID")
		return
	}

	pagination := models.ParsePagination(c)

	// Verify IP set exists
	_, err := h.ipRepo.GetIPSetByID(c.Request.Context(), ipSetID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "IPSET_NOT_FOUND", "IP Set not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "IPSET_GET_FAILED", "Failed to retrieve IP set")
		return
	}

	// List available IPs
	filter := repository.IPAddressListFilter{
		IPSetID:          &ipSetID,
		Status:           util.StringPtr("available"),
		PaginationParams: pagination,
	}

	ips, total, err := h.ipRepo.ListIPAddresses(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list available IPs",
			"ipset_id", ipSetID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "IP_LIST_FAILED", "Failed to retrieve available IPs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: ips,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}
