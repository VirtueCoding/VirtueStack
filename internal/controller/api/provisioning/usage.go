package provisioning

import (
	"math"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// VMUsageResponse represents the metering values returned for a VM.
// This is consumed by WHMCS UsageUpdate() for billing.
// BandwidthUsedGB is actual monthly bandwidth usage, while disk values reflect
// the provisioned disk size currently tracked by the controller.
type VMUsageResponse struct {
	VMID             string  `json:"vm_id"`
	BandwidthUsedGB  float64 `json:"bandwidth_used_gb"`
	BandwidthLimitGB int     `json:"bandwidth_limit_gb"`
	DiskUsedGB       int     `json:"disk_used_gb"`
	DiskLimitGB      int     `json:"disk_limit_gb"`
}

// GetVMUsage handles GET /vms/:id/usage.
// It returns actual monthly bandwidth usage and the VM's provisioned disk size
// for WHMCS billing integrations.
func (h *ProvisioningHandler) GetVMUsage(c *gin.Context) {
	vmID := c.Param("id")

	vm, err := getValidVM(c.Request.Context(), h.vmRepo, vmID, h.logger)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	resp := VMUsageResponse{
		VMID:             vm.ID,
		BandwidthLimitGB: vm.BandwidthLimitGB,
		DiskUsedGB:       vm.DiskGB,
		DiskLimitGB:      vm.DiskGB,
	}

	if h.bandwidthService != nil {
		usage, err := h.bandwidthService.GetMonthlyUsage(c.Request.Context(), vm.ID, 0, 0)
		if err != nil {
			h.logger.Warn("failed to get bandwidth usage, returning zeros",
				"vm_id", vmID, "error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		} else {
			resp.BandwidthUsedGB = bytesToGB(usage.TotalBytes())
		}
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// bytesToGB converts bytes to gigabytes, rounded to 2 decimal places.
func bytesToGB(bytes uint64) float64 {
	gb := float64(bytes) / (1024 * 1024 * 1024)
	return math.Round(gb*100) / 100
}
