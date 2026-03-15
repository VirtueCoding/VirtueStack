package services

import (
	"net/netip"
	"testing"
)

func TestGenerateIPv4PTRName(t *testing.T) {
	tests := []struct {
		ip       string
		wantPTR  string
		wantZone string
	}{
		{"192.0.2.1", "1.2.0.192.in-addr.arpa", "2.0.192.in-addr.arpa"},
		{"10.0.0.1", "1.0.0.10.in-addr.arpa", "0.0.10.in-addr.arpa"},
		{"172.16.5.10", "10.5.16.172.in-addr.arpa", "5.16.172.in-addr.arpa"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.ip)
			if err != nil {
				t.Fatalf("failed to parse IP %s: %v", tt.ip, err)
			}
			ptrName, zoneName := generateIPv4PTRName(addr)
			if ptrName != tt.wantPTR {
				t.Errorf("generateIPv4PTRName(%s) PTR = %q, want %q", tt.ip, ptrName, tt.wantPTR)
			}
			if zoneName != tt.wantZone {
				t.Errorf("generateIPv4PTRName(%s) zone = %q, want %q", tt.ip, zoneName, tt.wantZone)
			}
		})
	}
}

func TestGenerateSOASerialIncrement(t *testing.T) {
	result := generateSOASerial(2023010101)
	if result <= 2023010101 {
		t.Errorf("generateSOASerial(2023010101) = %d, expected > 2023010101", result)
	}
}

func TestGenerateIPv6PTRName(t *testing.T) {
	addr, err := netip.ParseAddr("2001:db8::1")
	if err != nil {
		t.Fatalf("failed to parse IPv6: %v", err)
	}

	ptrName, zoneName := generateIPv6PTRName(addr)

	if ptrName == "" {
		t.Error("expected non-empty PTR name for IPv6")
	}
	if zoneName == "" {
		t.Error("expected non-empty zone name for IPv6")
	}
	if ptrName[len(ptrName)-9:] != ".ip6.arpa" {
		t.Errorf("PTR name should end with .ip6.arpa, got %s", ptrName)
	}
}
