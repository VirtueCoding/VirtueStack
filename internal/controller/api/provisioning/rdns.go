package provisioning

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ProvisioningRDNSRequest struct {
	Hostname string `json:"hostname" validate:"required,hostname_rfc1123,max=253"`
}

type ProvisioningRDNSResponse struct {
	IPAddress    string  `json:"ip_address"`
	RDNSHostname *string `json:"rdns_hostname,omitempty"`
}

func (h *ProvisioningHandler) GetVMRDNS(c *gin.Context) {
	vmID := c.Param("id")
	correlationID := middleware.GetCorrelationID(c)

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	_, err := h.vmRepo.GetByID(c.Request.Context(), vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for rDNS lookup",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		respondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	filter := repository.IPAddressListFilter{
		VMID: &vmID,
	}
	ips, _, err := h.ipRepo.ListIPAddresses(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list IPs for VM rDNS",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		respondWithError(c, http.StatusInternalServerError, "IP_LIST_FAILED", "Failed to retrieve VM IPs")
		return
	}

	type ipRDNS struct {
		IPAddress    string  `json:"ip_address"`
		RDNSHostname *string `json:"rdns_hostname,omitempty"`
	}
	result := make([]ipRDNS, 0, len(ips))
	for _, ip := range ips {
		result = append(result, ipRDNS{
			IPAddress:    ip.Address,
			RDNSHostname: ip.RDNSHostname,
		})
	}

	h.logger.Info("VM rDNS retrieved",
		"vm_id", vmID,
		"ip_count", len(result),
		"correlation_id", correlationID)

	c.JSON(http.StatusOK, models.Response{Data: result})
}

func (h *ProvisioningHandler) SetVMRDNS(c *gin.Context) {
	vmID := c.Param("id")
	ipID := c.Query("ip_id")
	correlationID := middleware.GetCorrelationID(c)

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	if _, err := uuid.Parse(ipID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "ip_id must be a valid UUID")
		return
	}

	var req ProvisioningRDNSRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	ip, err := h.ipRepo.GetIPAddressByID(c.Request.Context(), ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
			return
		}
		h.logger.Error("failed to get IP address for rDNS update",
			"ip_id", ipID,
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		respondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to retrieve IP address")
		return
	}

	if ip.VMID == nil || *ip.VMID != vmID {
		respondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found for this VM")
		return
	}

	if err := h.ipRepo.SetRDNS(c.Request.Context(), ipID, req.Hostname); err != nil {
		h.logger.Error("failed to set rDNS",
			"ip_id", ipID,
			"vm_id", vmID,
			"hostname", req.Hostname,
			"error", err,
			"correlation_id", correlationID)
		respondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to update rDNS")
		return
	}

	h.logger.Info("rDNS updated via provisioning API",
		"ip_id", ipID,
		"vm_id", vmID,
		"ip_address", ip.Address,
		"hostname", req.Hostname,
		"correlation_id", correlationID)

	c.JSON(http.StatusOK, models.Response{
		Data: ProvisioningRDNSResponse{
			IPAddress:    ip.Address,
			RDNSHostname: &req.Hostname,
		},
	})
}
