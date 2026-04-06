package customer

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/common"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RDNSRequest represents the request body for updating rDNS.
type RDNSRequest struct {
	Hostname string `json:"hostname" validate:"required,hostname_rfc1123,max=253"`
}

// RDNSResponse represents the response for rDNS operations.
type RDNSResponse struct {
	IPAddress    string  `json:"ip_address"`
	RDNSHostname *string `json:"rdns_hostname,omitempty"`
}

// ListVMIPs handles GET /vms/:id/ips - lists all IP addresses for a VM with rDNS info.
// Customers can only view IPs for their own VMs.
// @Tags Customer
// @Summary List VM IPs
// @Description Lists IP addresses assigned to a customer VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/ips [get]
func (h *CustomerHandler) ListVMIPs(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Verify VM ownership
	if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for IP list",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_IP_LIST_FAILED", "Failed to retrieve VM IPs")
		return
	}

	// Get IPs for the VM
	filter := repository.IPAddressListFilter{
		VMID: &vmID,
	}
	ips, _, _, err := h.ipRepo.ListIPAddresses(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list IPs for VM",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "IP_LIST_FAILED", "Failed to retrieve IP addresses")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: ips,
		Meta: models.NewCursorPaginationMeta(len(ips), false, ""),
	})
}

// GetRDNS handles GET /vms/:id/ips/:ipId/rdns - gets the rDNS for a specific IP.
// Customers can only view rDNS for IPs assigned to their own VMs.
// @Tags Customer
// @Summary Get IP rDNS
// @Description Retrieves reverse DNS entry for VM IP address.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param ipId path string true "IP ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/ips/{ipId}/rdns [get]
func (h *CustomerHandler) GetRDNS(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")
	ipID := c.Param("ipId")

	// Validate UUIDs
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(ipID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "IP ID must be a valid UUID")
		return
	}

	// Verify VM ownership
	if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for rDNS",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_GET_FAILED", "Failed to retrieve rDNS")
		return
	}

	// Get the IP address and verify it belongs to the VM
	ip, err := h.ipRepo.GetIPAddressByID(c.Request.Context(), ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
			return
		}
		h.logger.Error("failed to get IP address",
			"ip_id", ipID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_GET_FAILED", "Failed to retrieve rDNS")
		return
	}

	// Verify the IP belongs to the VM
	if ip.VMID == nil || *ip.VMID != vmID {
		middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found for this VM")
		return
	}

	// Verify the IP is assigned to the customer
	if ip.CustomerID == nil || *ip.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusForbidden, "IP_NOT_OWNED", "You do not own this IP address")
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Data: RDNSResponse{
			IPAddress:    ip.Address,
			RDNSHostname: ip.RDNSHostname,
		},
	})
}

// UpdateRDNS handles PUT /vms/:id/ips/:ipId/rdns - updates the rDNS for a specific IP.
// Customers can only update rDNS for IPs assigned to their own VMs.
// Rate limited: 10 requests per hour per customer.
// @Tags Customer
// @Summary Update IP rDNS
// @Description Updates reverse DNS entry for VM IP address.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param ipId path string true "IP ID"
// @Param request body object true "rDNS update request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/ips/{ipId}/rdns [put]
func (h *CustomerHandler) UpdateRDNS(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")
	ipID := c.Param("ipId")

	// Validate UUIDs
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(ipID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "IP ID must be a valid UUID")
		return
	}

	// Parse and validate request
	var req RDNSRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Verify VM ownership
	if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for rDNS update",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to update rDNS")
		return
	}

	// Get the IP address and verify it belongs to the VM and customer
	ip, err := h.ipRepo.GetIPAddressByID(c.Request.Context(), ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
			return
		}
		h.logger.Error("failed to get IP address for rDNS update",
			"ip_id", ipID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to update rDNS")
		return
	}

	// Verify the IP belongs to the VM
	if ip.VMID == nil || *ip.VMID != vmID {
		middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found for this VM")
		return
	}

	// Verify the IP is assigned to the customer
	if ip.CustomerID == nil || *ip.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusForbidden, "IP_NOT_OWNED", "You do not own this IP address")
		return
	}

	if err := common.UpdateRDNSRecord(c.Request.Context(), h.ipRepo, h.rdnsService, *ip, req.Hostname); err != nil {
		h.logger.Error("failed to update rDNS",
			"ip_id", ipID,
			"ip_address", ip.Address,
			"customer_id", customerID,
			"hostname", req.Hostname,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		if sharederrors.Is(err, sharederrors.ErrServiceDown) {
			middleware.RespondWithError(c, http.StatusBadGateway, "RDNS_UPDATE_FAILED", "Failed to update authoritative rDNS")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_UPDATE_FAILED", "Failed to update rDNS")
		return
	}

	h.logAudit(c, "rdns.update", "ip_address", ipID, map[string]any{
		"rdns_hostname": req.Hostname,
	}, true)

	h.logger.Info("rDNS updated",
		"ip_id", ipID,
		"ip_address", ip.Address,
		"customer_id", customerID,
		"hostname", req.Hostname,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{
		Data: RDNSResponse{
			IPAddress:    ip.Address,
			RDNSHostname: &req.Hostname,
		},
	})
}

// DeleteRDNS handles DELETE /vms/:id/ips/:ipId/rdns - removes the rDNS for a specific IP.
// Customers can only delete rDNS for IPs assigned to their own VMs.
// @Tags Customer
// @Summary Delete IP rDNS
// @Description Deletes reverse DNS entry for VM IP address.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param ipId path string true "IP ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/ips/{ipId}/rdns [delete]
func (h *CustomerHandler) DeleteRDNS(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")
	ipID := c.Param("ipId")

	// Validate UUIDs
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(ipID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IP_ID", "IP ID must be a valid UUID")
		return
	}

	// Verify VM ownership
	if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for rDNS delete",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_DELETE_FAILED", "Failed to delete rDNS")
		return
	}

	// Get the IP address and verify it belongs to the VM and customer
	ip, err := h.ipRepo.GetIPAddressByID(c.Request.Context(), ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found")
			return
		}
		h.logger.Error("failed to get IP address for rDNS delete",
			"ip_id", ipID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_DELETE_FAILED", "Failed to delete rDNS")
		return
	}

	// Verify the IP belongs to the VM
	if ip.VMID == nil || *ip.VMID != vmID {
		middleware.RespondWithError(c, http.StatusNotFound, "IP_NOT_FOUND", "IP address not found for this VM")
		return
	}

	// Verify the IP is assigned to the customer
	if ip.CustomerID == nil || *ip.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusForbidden, "IP_NOT_OWNED", "You do not own this IP address")
		return
	}

	if err := common.DeleteRDNSRecord(c.Request.Context(), h.ipRepo, h.rdnsService, *ip); err != nil {
		h.logger.Error("failed to delete rDNS",
			"ip_id", ipID,
			"ip_address", ip.Address,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		if sharederrors.Is(err, sharederrors.ErrServiceDown) {
			middleware.RespondWithError(c, http.StatusBadGateway, "RDNS_DELETE_FAILED", "Failed to delete authoritative rDNS")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "RDNS_DELETE_FAILED", "Failed to delete rDNS")
		return
	}

	h.logAudit(c, "rdns.delete", "ip_address", ipID, nil, true)

	h.logger.Info("rDNS deleted",
		"ip_id", ipID,
		"ip_address", ip.Address,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}
