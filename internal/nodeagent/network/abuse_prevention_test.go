package network

import (
	"strings"
	"testing"
)

// TestAbusePreventionSMTPRule and TestAbusePreventionMetadataRule verify that
// abuse-prevention rules emitted for the forward chain match VM-outbound
// traffic via iifname (packets entering the host on the VM tap), not oifname
// (which only matches host->VM traffic and would no-op the SMTP/metadata
// blocks). Regression for NA-1.
func TestAbusePreventionSMTPRule(t *testing.T) {
	tests := []struct {
		name string
		tap  string
		want string
	}{
		{name: "tap123", tap: "tap123", want: "iifname tap123 tcp dport 25 drop"},
		{name: "vnet0", tap: "vnet0", want: "iifname vnet0 tcp dport 25 drop"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := smtpBlockRule(tc.tap)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("smtpBlockRule(%q) = %q, want substring %q", tc.tap, got, tc.want)
			}
			if strings.Contains(got, "oifname") {
				t.Fatalf("smtpBlockRule(%q) = %q, must not contain oifname", tc.tap, got)
			}
		})
	}
}

func TestAbusePreventionMetadataRule(t *testing.T) {
	tests := []struct {
		name string
		tap  string
		want string
	}{
		{name: "tap123", tap: "tap123", want: "iifname tap123 ip daddr 169.254.169.254 drop"},
		{name: "vnet0", tap: "vnet0", want: "iifname vnet0 ip daddr 169.254.169.254 drop"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := metadataBlockRule(tc.tap)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("metadataBlockRule(%q) = %q, want substring %q", tc.tap, got, tc.want)
			}
			if strings.Contains(got, "oifname") {
				t.Fatalf("metadataBlockRule(%q) = %q, must not contain oifname", tc.tap, got)
			}
		})
	}
}
