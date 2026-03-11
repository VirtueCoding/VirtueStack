package customer

import (
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// NetworkHistoryResponse represents network traffic history data.
type NetworkHistoryResponse struct {
	VMID      string          `json:"vm_id"`
	DataPoints []NetworkPoint `json:"data_points"`
	Period    string          `json:"period"`
}

// NetworkPoint represents a single data point in network history.
type NetworkPoint struct {
	Timestamp time.Time `json:"timestamp"`
	RxBytes   int64     `json:"rx_bytes"`
	TxBytes   int64     `json:"tx_bytes"`
}

// GetMetrics handles GET /vms/:id/metrics - retrieves real-time VM metrics.
// Returns CPU usage, memory usage, disk I/O, and network I/O statistics.
func (h *CustomerHandler) GetMetrics(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM metrics with ownership verification (isAdmin=false)
	metrics, err := h.vmService.GetVMMetrics(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Warn("failed to get VM metrics",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		// Check if VM is not running
		errMsg := err.Error()
		if contains(errMsg, "not running") {
			respondWithError(c, http.StatusConflict, "VM_NOT_RUNNING", "VM must be running to get metrics")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "METRICS_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: metrics})
}

// GetBandwidth handles GET /vms/:id/bandwidth - retrieves bandwidth usage for the current billing period.
// Shows used bandwidth, limit, and when the counter resets.
func (h *CustomerHandler) GetBandwidth(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM with ownership verification (isAdmin=false)
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "BANDWIDTH_FAILED", "Failed to get VM")
		return
	}

	// Calculate bandwidth usage
	limitBytes := int64(vm.BandwidthLimitGB) * 1024 * 1024 * 1024
	percentUsed := 0
	if limitBytes > 0 {
		percentUsed = int((float64(vm.BandwidthUsedBytes) / float64(limitBytes)) * 100)
	}

	// Calculate next reset (monthly from the last reset)
	nextReset := vm.BandwidthResetAt.AddDate(0, 1, 0)

	resp := BandwidthResponse{
		UsedBytes:   vm.BandwidthUsedBytes,
		LimitBytes:  limitBytes,
		ResetAt:     nextReset.Format(time.RFC3339),
		PercentUsed: percentUsed,
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// GetNetworkHistory handles GET /vms/:id/network - retrieves network traffic history.
// Supports query parameters for period selection (hour, day, week, month).
func (h *CustomerHandler) GetNetworkHistory(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM with ownership verification to ensure access
	_, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "NETWORK_HISTORY_FAILED", "Failed to get VM")
		return
	}

	// Get period from query (default to "day")
	period := c.DefaultQuery("period", "day")
	if !isValidPeriod(period) {
		respondWithError(c, http.StatusBadRequest, "INVALID_PERIOD", "Period must be one of: hour, day, week, month")
		return
	}

	// In a production system, this would query a time-series database
	// like Prometheus, InfluxDB, or TimescaleDB for actual network history.
	// For now, return placeholder data based on the period.
	dataPoints := generatePlaceholderNetworkData(period)

	resp := NetworkHistoryResponse{
		VMID:       vmID,
		DataPoints: dataPoints,
		Period:     period,
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// isValidPeriod checks if the period parameter is valid.
func isValidPeriod(period string) bool {
	switch period {
	case "hour", "day", "week", "month":
		return true
	default:
		return false
	}
}

// generatePlaceholderNetworkData generates placeholder network history data.
// In production, this would query actual metrics from a time-series database.
func generatePlaceholderNetworkData(period string) []NetworkPoint {
	now := time.Now()
	var points []NetworkPoint
	var interval time.Duration
	var count int

	switch period {
	case "hour":
		interval = 5 * time.Minute
		count = 12
	case "day":
		interval = time.Hour
		count = 24
	case "week":
		interval = 6 * time.Hour
		count = 28
	case "month":
		interval = 24 * time.Hour
		count = 30
	default:
		interval = time.Hour
		count = 24
	}

	for i := count - 1; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * interval)
		points = append(points, NetworkPoint{
			Timestamp: t,
			RxBytes:   int64(1000000 + i*50000), // Placeholder values
			TxBytes:   int64(500000 + i*25000),
		})
	}

	return points
}