package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminRDNSRequest represents the request body for updating rDNS.
type AdminRDNSRequest struct {
	Hostname string `json:"hostname" validate:"required,hostname_rfc1123,max=253"`
}

// AdminRDNSResponse represents the response for rDNS operations.
type AdminRDNSResponse struct {
	IPAddress    string  `json:"ip_address"`
	RDNSHostname *string `json:"rdns_hostname,omitempty"`
}

// GetIPRDNS handles GET /vms/:id/ips/:ipId/rdns - retrieves rDNS for an IP address.
// @Tags Admin
// @Summary Get IP rDNS
// @Description Retrieves reverse DNS entry for a VM IP address.
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Param ipId path string true "IP ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id}/ips/{ipId}/rdns [get]
func (h *AdminHandler) GetIPRDNS(c *gin.Context) {
	ipID := c.Param("ipId")

	if _, err := uuid.Parse(ipID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "IP ID must be a valid UUID")
		return
	}

	ip, err := h.ipRepo.GetIPAddressByID(c.Request.Context(), ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
			return
		}
		h.logger.Error("failed to get IP address",
			"ip_id", ipID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_GET_FAILED", "Failed to retrieve rDNS")
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Data: AdminRDNSResponse{
			IPAddress:    ip.Address,
			RDNSHostname: ip.RDNSHostname,
		},
	})
}

// UpdateIPRDNS handles PUT /vms/:id/ips/:ipId/rdns - updates rDNS for an IP address.
// @Tags Admin
// @Summary Update IP rDNS
// @Description Updates reverse DNS entry for a VM IP address.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Param ipId path string true "IP ID"
// @Param request body object true "rDNS update request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id}/ips/{ipId}/rdns [put]
func (h *AdminHandler) UpdateIPRDNS(c *gin.Context) {
	ipID := c.Param("ipId")

	if _, err := uuid.Parse(ipID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "IP ID must be a valid UUID")
		return
	}

	var req AdminRDNSRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	ip, err := h.ipRepo.GetIPAddressByID(c.Request.Context(), ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to retrieve IP address")
		return
	}

	if err := h.ipRepo.SetRDNS(c.Request.Context(), ipID, req.Hostname); err != nil {
		h.logger.Error("failed to update rDNS in database",
			"ip_id", ipID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to update rDNS")
		return
	}

	if h.rdnsService != nil {
		if err := h.rdnsService.SetReverseDNS(c.Request.Context(), ip.Address, req.Hostname); err != nil {
			h.logger.Error("failed to update rDNS in PowerDNS",
				"ip_id", ipID,
				"ip_address", ip.Address,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		}
	}

	h.logAuditEvent(c, "rdns.update", "ip_address", ipID, map[string]interface{}{
		"ip_address":    ip.Address,
		"rdns_hostname": req.Hostname,
	}, true)

	c.JSON(http.StatusOK, models.Response{
		Data: AdminRDNSResponse{
			IPAddress:    ip.Address,
			RDNSHostname: &req.Hostname,
		},
	})
}

// DeleteIPRDNS handles DELETE /vms/:id/ips/:ipId/rdns - clears rDNS for an IP address.
// @Tags Admin
// @Summary Delete IP rDNS
// @Description Removes reverse DNS entry for a VM IP address.
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Param ipId path string true "IP ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id}/ips/{ipId}/rdns [delete]
func (h *AdminHandler) DeleteIPRDNS(c *gin.Context) {
	ipID := c.Param("ipId")

	if _, err := uuid.Parse(ipID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "IP ID must be a valid UUID")
		return
	}

	ip, err := h.ipRepo.GetIPAddressByID(c.Request.Context(), ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_DELETE_FAILED", "Failed to retrieve IP address")
		return
	}

	if err := h.ipRepo.SetRDNS(c.Request.Context(), ipID, ""); err != nil {
		h.logger.Error("failed to clear rDNS in database",
			"ip_id", ipID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_DELETE_FAILED", "Failed to delete rDNS")
		return
	}

	if h.rdnsService != nil {
		if err := h.rdnsService.DeleteReverseDNS(c.Request.Context(), ip.Address); err != nil {
			h.logger.Error("failed to delete rDNS from PowerDNS",
				"ip_id", ipID,
				"ip_address", ip.Address,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		}
	}

	h.logAuditEvent(c, "rdns.delete", "ip_address", ipID, map[string]interface{}{
		"ip_address": ip.Address,
	}, true)

	c.Status(http.StatusNoContent)
}

// GetVMIPs handles GET /vms/:id/ips - lists all IP addresses for a VM.
// @Tags Admin
// @Summary List VM IPs
// @Description Lists IPv4/IPv6 addresses assigned to a VM.
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id}/ips [get]
func (h *AdminHandler) GetVMIPs(c *gin.Context) {
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	pagination := models.ParsePagination(c)
	filter := repository.IPAddressListFilter{
		VMID:             &vmID,
		PaginationParams: pagination,
	}
	ips, total, err := h.ipRepo.ListIPAddresses(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list IPs for VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "IP_LIST_FAILED", "Failed to retrieve IP addresses")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: ips,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}
