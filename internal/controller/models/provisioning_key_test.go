package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProvisioningKeyIsAllowedIP tests the IsAllowedIP method for ProvisioningKey.
func TestProvisioningKeyIsAllowedIP(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs []string
		clientIP   string
		wantResult bool
	}{
		// Empty whitelist - all IPs allowed
		{
			name:       "empty whitelist allows all IPs",
			allowedIPs: nil,
			clientIP:   "192.168.1.100",
			wantResult: true,
		},
		{
			name:       "empty slice allows all IPs",
			allowedIPs: []string{},
			clientIP:   "10.0.0.1",
			wantResult: true,
		},

		// IPv4 exact matches
		{
			name:       "IPv4 exact match - allowed",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "192.168.1.100",
			wantResult: true,
		},
		{
			name:       "IPv4 no match - denied",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "192.168.1.101",
			wantResult: false,
		},

		// IPv4 CIDR notation
		{
			name:       "IPv4 CIDR /24 - in range",
			allowedIPs: []string{"192.168.1.0/24"},
			clientIP:   "192.168.1.50",
			wantResult: true,
		},
		{
			name:       "IPv4 CIDR /24 - out of range",
			allowedIPs: []string{"192.168.1.0/24"},
			clientIP:   "192.168.2.1",
			wantResult: false,
		},
		{
			name:       "IPv4 CIDR /16 - in range",
			allowedIPs: []string{"10.0.0.0/16"},
			clientIP:   "10.0.255.255",
			wantResult: true,
		},

		// IPv6 exact matches
		{
			name:       "IPv6 exact match - allowed",
			allowedIPs: []string{"2001:db8::1"},
			clientIP:   "2001:db8::1",
			wantResult: true,
		},
		{
			name:       "IPv6 no match - denied",
			allowedIPs: []string{"2001:db8::1"},
			clientIP:   "2001:db8::2",
			wantResult: false,
		},

		// IPv6 CIDR notation
		{
			name:       "IPv6 CIDR /64 - in range",
			allowedIPs: []string{"2001:db8:abcd::/64"},
			clientIP:   "2001:db8:abcd::1234",
			wantResult: true,
		},
		{
			name:       "IPv6 CIDR /64 - out of range",
			allowedIPs: []string{"2001:db8:abcd::/64"},
			clientIP:   "2001:db8:ef01::1",
			wantResult: false,
		},
		{
			name:       "IPv6 CIDR /48 - in range",
			allowedIPs: []string{"2001:db8::/48"},
			clientIP:   "2001:db8:0:1::1",
			wantResult: true,
		},
		{
			name:       "IPv6 CIDR /48 - out of range",
			allowedIPs: []string{"2001:db8::/48"},
			clientIP:   "2001:db8:abcd::1",
			wantResult: false,
		},

		// Mixed IPv4/IPv6/CIDR
		{
			name:       "mixed list - IPv4 match",
			allowedIPs: []string{"192.168.1.0/24", "2001:db8::1", "10.0.0.1"},
			clientIP:   "192.168.1.50",
			wantResult: true,
		},
		{
			name:       "mixed list - IPv6 match",
			allowedIPs: []string{"192.168.1.0/24", "2001:db8::/64", "10.0.0.1"},
			clientIP:   "2001:db8::abcd",
			wantResult: true,
		},
		{
			name:       "mixed list - no match",
			allowedIPs: []string{"192.168.1.0/24", "2001:db8::/64", "10.0.0.1"},
			clientIP:   "172.16.0.1",
			wantResult: false,
		},

		// Invalid inputs
		{
			name:       "invalid client IP - denied",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "not-an-ip",
			wantResult: false,
		},
		{
			name:       "empty client IP - denied",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "",
			wantResult: false,
		},

		// WHMCS typical use cases
		{
			name:       "WHMCS server IP allowed",
			allowedIPs: []string{"203.0.113.50"},
			clientIP:   "203.0.113.50",
			wantResult: true,
		},
		{
			name:       "WHMCS server IP CIDR allowed",
			allowedIPs: []string{"203.0.113.0/24"},
			clientIP:   "203.0.113.50",
			wantResult: true,
		},
		{
			name:       "non-WHMCS IP denied",
			allowedIPs: []string{"203.0.113.50"},
			clientIP:   "198.51.100.1",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &ProvisioningKey{
				ID:         "test-key",
				AllowedIPs: tt.allowedIPs,
			}

			result := key.IsAllowedIP(tt.clientIP)
			assert.Equal(t, tt.wantResult, result, "IsAllowedIP(%q) with whitelist %v", tt.clientIP, tt.allowedIPs)
		})
	}
}