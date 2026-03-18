package services

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{"8.8.8.8", "8.8.8.8.in-addr.arpa", "8.8.8.in-addr.arpa"},
		{"0.0.0.0", "0.0.0.0.in-addr.arpa", "0.0.0.in-addr.arpa"},
		{"255.255.255.255", "255.255.255.255.in-addr.arpa", "255.255.255.in-addr.arpa"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.ip)
			require.NoError(t, err, "failed to parse IP %s", tt.ip)
			ptrName, zoneName := generateIPv4PTRName(addr)
			assert.Equal(t, tt.wantPTR, ptrName, "PTR name mismatch")
			assert.Equal(t, tt.wantZone, zoneName, "zone name mismatch")
		})
	}
}

func TestGenerateSOASerialIncrement(t *testing.T) {
	tests := []struct {
		name          string
		currentSerial int64
	}{
		{"increment existing serial", 2023010101},
		{"increment zero serial", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSOASerial(tt.currentSerial)
			assert.Greater(t, result, tt.currentSerial, "serial should increment")
		})
	}
}

func TestGenerateSOASerial_TodayFormat(t *testing.T) {
	now := time.Now().UTC()
	todayBase := int64(now.Year()*1000000 + int(now.Month())*10000 + now.Day()*100)

	result := generateSOASerial(todayBase)
	assert.Equal(t, todayBase+1, result, "should increment serial within same day")

	result2 := generateSOASerial(todayBase + 50)
	assert.Equal(t, todayBase+51, result2, "should continue incrementing within same day")
}

func TestGenerateIPv6PTRName(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"standard IPv6", "2001:db8::1"},
		{"full IPv6", "2001:0db8:0000:0000:0000:0000:0000:0001"},
		{"loopback", "::1"},
		{"all zeros", "::"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.ip)
			require.NoError(t, err, "failed to parse IPv6")

			ptrName, zoneName := generateIPv6PTRName(addr)

			assert.NotEmpty(t, ptrName, "PTR name should not be empty")
			assert.NotEmpty(t, zoneName, "zone name should not be empty")
			assert.True(t, strings.HasSuffix(ptrName, ".ip6.arpa"), "PTR name should end with .ip6.arpa")
			assert.True(t, strings.HasSuffix(zoneName, ".ip6.arpa"), "zone name should end with .ip6.arpa")
		})
	}
}

func TestGeneratePTRName_IPv4(t *testing.T) {
	addr, err := netip.ParseAddr("192.0.2.1")
	require.NoError(t, err)

	ptrName, zoneName := generatePTRName(addr)
	assert.Equal(t, "1.2.0.192.in-addr.arpa", ptrName)
	assert.Equal(t, "2.0.192.in-addr.arpa", zoneName)
}

func TestGeneratePTRName_IPv6(t *testing.T) {
	addr, err := netip.ParseAddr("2001:db8::1")
	require.NoError(t, err)

	ptrName, zoneName := generatePTRName(addr)
	assert.True(t, strings.HasSuffix(ptrName, ".ip6.arpa"))
	assert.True(t, strings.HasSuffix(zoneName, ".ip6.arpa"))
}

func TestGenerateIPv6ReverseZone(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		prefixBits int
	}{
		{"48-bit prefix", "2001:db8::1", 48},
		{"32-bit prefix", "2001:db8::1", 32},
		{"64-bit prefix", "2001:db8::1", 64},
		{"0-bit prefix", "2001:db8::1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.ip)
			require.NoError(t, err)
			bytes := addr.As16()

			zone := generateIPv6ReverseZone(bytes, tt.prefixBits)

			if tt.prefixBits > 0 {
				assert.True(t, strings.HasSuffix(zone, ".ip6.arpa"), "zone should end with .ip6.arpa")
			} else {
				assert.Equal(t, "ip6.arpa", zone)
			}
		})
	}
}
