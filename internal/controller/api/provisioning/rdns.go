package provisioning

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type ProvisioningRDNSRequest struct {
	Hostname string `json:"hostname" validate:"required,hostname_rfc1123,max=253"`
}

type ProvisioningRDNSResponse struct {
	IPAddress    string  `json:"ip_address"`
	RDNSHostname *string `json:"rdns_hostname,omitempty"`
}

// @Tags Provisioning
// @Summary Get VM rDNS
// @Description Lists reverse DNS records for VM IP addresses.
// @Produce json
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/provisioning/vms/{id}/rdns [get]
func (h *ProvisioningHandler) GetVMRDNS(c *gin.Context) {
	vmID := c.Param("id")
	correlationID := middleware.GetCorrelationID(c)

	if _, err := validateVMID(vmID); err != nil {
		respondWithValidationError(c, err)
		return
	}

	if _, err := h.vmRepo.GetByID(c.Request.Context(), vmID); err != nil {
		h.handleVMGetError(c, err, vmID, correlationID)
		return
	}

	filter := repository.IPAddressListFilter{VMID: &vmID}
	ips, _, _, err := h.ipRepo.ListIPAddresses(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list IPs for VM rDNS", "vm_id", vmID, "error", err, "correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "IP_LIST_FAILED", "Failed to retrieve VM IPs")
		return
	}

	result := buildRDNSResponse(ips)
	h.logger.Info("VM rDNS retrieved", "vm_id", vmID, "ip_count", len(result), "correlation_id", correlationID)
	c.JSON(http.StatusOK, models.Response{Data: result})
}

// @Tags Provisioning
// @Summary Set VM rDNS
// @Description Updates reverse DNS records for VM IP addresses.
// @Accept json
// @Produce json
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param request body object true "Set rDNS request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/provisioning/vms/{id}/rdns [put]
func (h *ProvisioningHandler) SetVMRDNS(c *gin.Context) {
	vmID := c.Param("id")
	ipID := c.Query("ip_id")
	correlationID := middleware.GetCorrelationID(c)

	if _, err := validateVMID(vmID); err != nil {
		respondWithValidationError(c, err)
		return
	}
	if _, err := validateVMID(ipID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "ip_id must be a valid UUID")
		return
	}
	var req ProvisioningRDNSRequest
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
		h.handleIPGetError(c, err, ipID, vmID, correlationID)
		return
	}
	if ip.VMID == nil || *ip.VMID != vmID {
		middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found for this VM")
		return
	}

	if err := h.ipRepo.SetRDNS(c.Request.Context(), ipID, req.Hostname); err != nil {
		h.logger.Error("failed to set rDNS", "ip_id", ipID, "vm_id", vmID, "hostname", req.Hostname, "error", err, "correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to update rDNS")
		return
	}
	h.logger.Info("rDNS updated via provisioning API", "ip_id", ipID, "vm_id", vmID, "ip_address", ip.Address, "hostname", req.Hostname, "correlation_id", correlationID)
	c.JSON(http.StatusOK, models.Response{Data: ProvisioningRDNSResponse{IPAddress: ip.Address, RDNSHostname: &req.Hostname}})
}

// handleVMGetError handles errors from VM repository GetByID calls.
func (h *ProvisioningHandler) handleVMGetError(c *gin.Context, err error, vmID, correlationID string) {
	if sharederrors.Is(err, sharederrors.ErrNotFound) {
		middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
		return
	}
	h.logger.Error("failed to get VM", "vm_id", vmID, "error", err, "correlation_id", correlationID)
	middleware.RespondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
}

// handleIPGetError handles errors from IP repository GetIPAddressByID calls.
func (h *ProvisioningHandler) handleIPGetError(c *gin.Context, err error, ipID, vmID, correlationID string) {
	if sharederrors.Is(err, sharederrors.ErrNotFound) {
		middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
		return
	}
	h.logger.Error("failed to get IP address for rDNS update", "ip_id", ipID, "vm_id", vmID, "error", err, "correlation_id", correlationID)
	middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to retrieve IP address")
}

// ipRDNS represents an IP address with rDNS information.
type ipRDNS struct {
	IPAddress    string  `json:"ip_address"`
	RDNSHostname *string `json:"rdns_hostname,omitempty"`
}

// buildRDNSResponse builds a list of ipRDNS from IP addresses.
func buildRDNSResponse(ips []models.IPAddress) []ipRDNS {
	result := make([]ipRDNS, 0, len(ips))
	for _, ip := range ips {
		result = append(result, ipRDNS{IPAddress: ip.Address, RDNSHostname: ip.RDNSHostname})
	}
	return result
}
