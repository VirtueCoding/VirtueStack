// Package vm provides helper functions for VM lifecycle metrics collection.
// These functions decompose the GetMetrics function to comply with
// docs/coding-standard.md QG-01 (functions <= 40 lines).
package vm

import (
	"context"
	"log/slog"

	"libvirt.org/go/libvirt"
)

// metricsData holds all collected metrics for a VM.
type metricsData struct {
	cpuPercent    float64
	memUsage      int64
	memTotal      int64
	diskRead      int64
	diskWrite     int64
	diskRdOps     int64
	diskWrOps     int64
	netRX         int64
	netTX         int64
	netRXPkts     int64
	netTXPkts     int64
	netRXErrs     int64
	netTXErrs     int64
	netRXDrop     int64
	netTXDrop     int64
}

// collectMetrics gathers all metrics from a running domain.
// Errors are logged but do not cause the function to fail.
func (m *Manager) collectMetrics(ctx context.Context, domain *libvirt.Domain, vmID string, logger *slog.Logger) *metricsData {
	data := &metricsData{}

	// Get CPU stats
	var err error
	data.cpuPercent, err = m.getCPUUsage(ctx, domain)
	if err != nil {
		logger.Warn("could not get CPU usage", "error", err, "vm_id", vmID)
	}

	// Get memory stats
	data.memUsage, data.memTotal, err = m.getMemoryUsage(domain)
	if err != nil {
		logger.Warn("could not get memory usage", "error", err, "vm_id", vmID)
	}

	// Get disk stats
	data.diskRead, data.diskWrite, err = m.getDiskStats(domain)
	if err != nil {
		logger.Warn("could not get disk stats", "error", err, "vm_id", vmID)
	}

	// Get disk ops
	_, _, data.diskRdOps, data.diskWrOps, err = m.getDiskStatsFull(domain)
	if err != nil {
		logger.Warn("could not get disk ops", "error", err, "vm_id", vmID)
	}

	// Get network stats
	data.netRX, data.netTX, err = m.getNetworkStats(domain)
	if err != nil {
		logger.Warn("could not get network stats", "error", err, "vm_id", vmID)
	}

	// Get full network stats
	_, _, data.netRXPkts, data.netTXPkts, data.netRXErrs, data.netTXErrs,
		data.netRXDrop, data.netTXDrop, err = m.getNetworkStatsFull(domain)
	if err != nil {
		logger.Warn("could not get full network stats", "error", err, "vm_id", vmID)
	}

	return data
}

// toVMMetrics converts collected metrics data to VMMetrics struct.
func (d *metricsData) toVMMetrics(vmID string) *VMMetrics {
	return &VMMetrics{
		VMID:             vmID,
		CPUUsagePercent:  d.cpuPercent,
		MemoryUsageBytes: d.memUsage,
		MemoryTotalBytes: d.memTotal,
		DiskReadBytes:    d.diskRead,
		DiskWriteBytes:   d.diskWrite,
		DiskReadOps:      d.diskRdOps,
		DiskWriteOps:     d.diskWrOps,
		NetworkRXBytes:   d.netRX,
		NetworkTXBytes:   d.netTX,
		NetworkRXPkts:    d.netRXPkts,
		NetworkTXPkts:    d.netTXPkts,
		NetworkRXErrs:    d.netRXErrs,
		NetworkTXErrs:    d.netTXErrs,
		NetworkRXDrop:    d.netRXDrop,
		NetworkTXDrop:    d.netTXDrop,
	}
}
