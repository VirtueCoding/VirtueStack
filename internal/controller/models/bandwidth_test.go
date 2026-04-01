package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBandwidthUsage_TotalBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytesIn  uint64
		bytesOut uint64
		want     uint64
	}{
		{
			name:     "both zero",
			bytesIn:  0,
			bytesOut: 0,
			want:     0,
		},
		{
			name:     "only ingress",
			bytesIn:  1000,
			bytesOut: 0,
			want:     1000,
		},
		{
			name:     "only egress",
			bytesIn:  0,
			bytesOut: 2000,
			want:     2000,
		},
		{
			name:     "both nonzero",
			bytesIn:  5000,
			bytesOut: 3000,
			want:     8000,
		},
		{
			name:     "large values",
			bytesIn:  1_000_000_000_000,
			bytesOut: 2_000_000_000_000,
			want:     3_000_000_000_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BandwidthUsage{
				BytesIn:  tt.bytesIn,
				BytesOut: tt.bytesOut,
			}
			assert.Equal(t, tt.want, b.TotalBytes())
		})
	}
}

func TestBandwidthUsage_UsagePercent(t *testing.T) {
	tests := []struct {
		name       string
		bytesIn    uint64
		bytesOut   uint64
		limitBytes uint64
		want       float64
	}{
		{
			name:       "zero limit returns zero",
			bytesIn:    500,
			bytesOut:   500,
			limitBytes: 0,
			want:       0,
		},
		{
			name:       "no usage",
			bytesIn:    0,
			bytesOut:   0,
			limitBytes: 1000,
			want:       0,
		},
		{
			name:       "50 percent usage",
			bytesIn:    250,
			bytesOut:   250,
			limitBytes: 1000,
			want:       50.0,
		},
		{
			name:       "100 percent usage",
			bytesIn:    500,
			bytesOut:   500,
			limitBytes: 1000,
			want:       100.0,
		},
		{
			name:       "over limit",
			bytesIn:    800,
			bytesOut:   800,
			limitBytes: 1000,
			want:       160.0,
		},
		{
			name:       "small fraction",
			bytesIn:    1,
			bytesOut:   0,
			limitBytes: 1000,
			want:       0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BandwidthUsage{
				BytesIn:    tt.bytesIn,
				BytesOut:   tt.bytesOut,
				LimitBytes: tt.limitBytes,
			}
			assert.InDelta(t, tt.want, b.UsagePercent(), 0.001)
		})
	}
}

func TestBandwidthUsage_Exceeded(t *testing.T) {
	tests := []struct {
		name       string
		bytesIn    uint64
		bytesOut   uint64
		limitBytes uint64
		want       bool
	}{
		{
			name:       "no limit means not exceeded",
			bytesIn:    999999,
			bytesOut:   999999,
			limitBytes: 0,
			want:       false,
		},
		{
			name:       "under limit",
			bytesIn:    300,
			bytesOut:   200,
			limitBytes: 1000,
			want:       false,
		},
		{
			name:       "at limit (not exceeded)",
			bytesIn:    500,
			bytesOut:   500,
			limitBytes: 1000,
			want:       false,
		},
		{
			name:       "over limit",
			bytesIn:    600,
			bytesOut:   500,
			limitBytes: 1000,
			want:       true,
		},
		{
			name:       "zero usage with limit",
			bytesIn:    0,
			bytesOut:   0,
			limitBytes: 1000,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BandwidthUsage{
				BytesIn:    tt.bytesIn,
				BytesOut:   tt.bytesOut,
				LimitBytes: tt.limitBytes,
			}
			assert.Equal(t, tt.want, b.Exceeded())
		})
	}
}

func TestDefaultThrottleConfig(t *testing.T) {
	config := DefaultThrottleConfig()

	assert.Equal(t, ThrottleRateKbps, config.RateKbps)
	assert.Equal(t, 5000, config.RateKbps) // 5 Mbps
	assert.Equal(t, 32, config.BurstKB)
	assert.Equal(t, 400, config.LatencyMs)
}

func TestThrottleRateKbpsConstant(t *testing.T) {
	assert.Equal(t, 5000, ThrottleRateKbps)
}
