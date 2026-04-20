package util

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		private bool
	}{
		// RFC-1918 private ranges
		{"10.x.x.x", "10.0.0.1", true},
		{"172.16.x.x", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"192.168.x.x", "192.168.1.1", true},

		// Loopback
		{"loopback 127.0.0.1", "127.0.0.1", true},

		// 0.0.0.0/8
		{"0.0.0.0", "0.0.0.0", true},
		{"0.255.255.255", "0.255.255.255", true},

		// Link-local
		{"link-local 169.254.1.1", "169.254.1.1", true},

		// Cloud metadata IPs
		// 169.254.169.254 and fd00:ec2::254 are matched by CIDR ranges (169.254.0.0/16 and fc00::/7)
		// AND by the explicit metadataIPs guard — either mechanism alone would block them.
		{"AWS metadata 169.254.169.254", "169.254.169.254", true},
		{"AWS IPv6 metadata fd00:ec2::254", "fd00:ec2::254", true},

		// CGNAT (RFC-6598)
		{"CGNAT 100.64.0.1", "100.64.0.1", true},

		// IPv6 private
		{"IPv6 ULA fc00::1", "fc00::1", true},
		{"IPv6 link-local fe80::1", "fe80::1", true},

		// IPv6 loopback
		{"IPv6 loopback ::1", "::1", true},

		// Public IPs — must return false
		{"Google DNS 8.8.8.8", "8.8.8.8", false},
		{"Cloudflare DNS 1.1.1.1", "1.1.1.1", false},
		{"Google IPv6 DNS 2001:4860:4860::8888", "2001:4860:4860::8888", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tc.ip)
			}
			got := IsPrivateIP(ip)
			if got != tc.private {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tc.ip, got, tc.private)
			}
		})
	}
}
