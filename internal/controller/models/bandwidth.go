// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// BandwidthUsage represents monthly bandwidth usage for a VM.
// It tracks ingress/egress bytes against the plan's bandwidth limit
// and records throttling state.
type BandwidthUsage struct {
	// VMID is the unique identifier of the virtual machine.
	VMID string `json:"vm_id" db:"vm_id"`
	// Year is the calendar year for this usage record.
	Year int `json:"year" db:"year"`
	// Month is the calendar month (1-12) for this usage record.
	Month int `json:"month" db:"month"`
	// BytesIn is the total ingress bytes for the month.
	BytesIn uint64 `json:"bytes_in" db:"bytes_in"`
	// BytesOut is the total egress bytes for the month.
	BytesOut uint64 `json:"bytes_out" db:"bytes_out"`
	// LimitBytes is the bandwidth limit in bytes for this billing period.
	LimitBytes uint64 `json:"limit_bytes" db:"limit_bytes"`
	// Throttled indicates whether the VM is currently bandwidth-throttled.
	Throttled bool `json:"throttled" db:"throttled"`
	// ThrottledAt is the timestamp when throttling was applied.
	ThrottledAt *time.Time `json:"throttled_at,omitempty" db:"throttled_at"`
	// ResetAt is the timestamp when counters were last reset.
	ResetAt *time.Time `json:"reset_at,omitempty" db:"reset_at"`
}

// TotalBytes returns the total bytes (in + out) for the month.
func (b *BandwidthUsage) TotalBytes() uint64 {
	return b.BytesIn + b.BytesOut
}

// UsagePercent returns the usage as a percentage of the limit.
func (b *BandwidthUsage) UsagePercent() float64 {
	if b.LimitBytes == 0 {
		return 0
	}
	return float64(b.TotalBytes()) / float64(b.LimitBytes) * 100
}

// Exceeded returns true if the VM has exceeded its bandwidth limit.
func (b *BandwidthUsage) Exceeded() bool {
	if b.LimitBytes == 0 {
		return false // No limit set
	}
	return b.TotalBytes() > b.LimitBytes
}

// BandwidthUsageFilter holds query parameters for filtering bandwidth usage.
type BandwidthUsageFilter struct {
	// VMID filters by specific VM.
	VMID *string
	// Year filters by specific year.
	Year *int
	// Month filters by specific month.
	Month *int
	// Throttled filters for throttled VMs only.
	Throttled *bool
}

// ThrottleRateKbps is the default throttle rate (5 Mbps = 5000 Kbps).
const ThrottleRateKbps = 5000

// ThrottleConfig contains configuration for bandwidth throttling.
type ThrottleConfig struct {
	// RateKbps is the throttle rate in kilobits per second.
	RateKbps int
	// BurstKB is the burst allowance in kilobytes.
	BurstKB int
	// LatencyMs is the maximum latency for queued packets.
	LatencyMs int
}

// DefaultThrottleConfig returns the default throttle configuration.
// Rate: 5 Mbps, Burst: 32 KB, Latency: 400ms
func DefaultThrottleConfig() ThrottleConfig {
	return ThrottleConfig{
		RateKbps:  ThrottleRateKbps,
		BurstKB:   32,
		LatencyMs: 400,
	}
}