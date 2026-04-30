package network

import (
	"strings"
	"testing"
)

// TestGenerateDNSMasqConfigFullRejectsInvalidMAC verifies that
// GenerateDNSMasqConfigFull validates cfg.MACAddress with net.ParseMAC and
// rejects values containing newlines or other malformed input that could
// inject arbitrary dnsmasq directives via the dhcp-host= line. Regression
// for NA-11.
func TestGenerateDNSMasqConfigFullRejectsInvalidMAC(t *testing.T) {
	m := &DHCPManager{}
	base := DHCPConfig{
		VMID:            "vm-1",
		VMName:          "test",
		BridgeInterface: "vs-br0",
		IPAddress:       "10.0.0.2",
		Gateway:         "10.0.0.1",
		DNS:             "8.8.8.8",
	}

	cases := []struct {
		name string
		mac  string
	}{
		{"newline injection", "52:54:00:11:22:33\nserver=evil.example.com"},
		{"trailing newline", "52:54:00:11:22:33\n"},
		{"empty", ""},
		{"garbage", "not-a-mac"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.MACAddress = tc.mac
			out, err := m.GenerateDNSMasqConfigFull(cfg)
			if err == nil {
				t.Fatalf("expected error for invalid MAC %q, got nil; output=%q", tc.mac, out)
			}
			if out != "" {
				t.Fatalf("expected empty output on error, got %q", out)
			}
		})
	}
}

func TestGenerateDNSMasqConfigFullAcceptsValidMAC(t *testing.T) {
	m := &DHCPManager{}
	cfg := DHCPConfig{
		VMID:            "vm-1",
		VMName:          "test",
		BridgeInterface: "vs-br0",
		IPAddress:       "10.0.0.2",
		Gateway:         "10.0.0.1",
		DNS:             "8.8.8.8",
		MACAddress:      "52:54:00:11:22:33",
	}
	out, err := m.GenerateDNSMasqConfigFull(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "dhcp-host=52:54:00:11:22:33,10.0.0.2,") {
		t.Fatalf("expected dhcp-host line with MAC, got: %s", out)
	}
}
