// Package network provides helper functions for nwfilter generation.
// These functions decompose the GenerateFilterXML function to comply with
// docs/coding-standard.md QG-01 (functions <= 40 lines).
package network

import (
	"fmt"
	"strings"
)

// nwFilterBuilder provides methods for building nwfilter XML rules.
type nwFilterBuilder struct {
	sb strings.Builder
}

// newNWFilterBuilder creates a new nwFilterBuilder.
func newNWFilterBuilder(filterName string) *nwFilterBuilder {
	b := &nwFilterBuilder{}
	b.sb.WriteString(`<filter name='`)
	b.sb.WriteString(escapeXML(filterName))
	b.sb.WriteString(`'>`)
	return b
}

// addCleanTrafficRef adds the clean-traffic base filter reference.
func (b *nwFilterBuilder) addCleanTrafficRef() {
	b.sb.WriteString(`<filterref filter='clean-traffic'/>`)
}

// addDHCPRules adds DHCP and DHCPv6 allow rules.
func (b *nwFilterBuilder) addDHCPRules() {
	// SECURITY: Rogue DHCP prevention
	b.sb.WriteString(`<rule action='drop' direction='out' priority='80'>`)
	b.sb.WriteString(`<udp srcportstart='67' srcportend='67'/>`)
	b.sb.WriteString(`</rule>`)

	// Allow outbound DHCP requests
	b.sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	b.sb.WriteString(`<ip srcipaddr='0.0.0.0' protocol='udp'/>`)
	b.sb.WriteString(`<udp srcportstart='68' srcportend='68' dstportstart='67' dstportend='67'/>`)
	b.sb.WriteString(`</rule>`)

	// Allow outbound DHCPv6 requests
	b.sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	b.sb.WriteString(`<ipv6 protocol='udp'/>`)
	b.sb.WriteString(`<udp srcportstart='546' srcportend='546' dstportstart='547' dstportend='547'/>`)
	b.sb.WriteString(`</rule>`)
}

// addIPv6ICMPRules adds IPv6 ICMP rules for auto-configuration.
func (b *nwFilterBuilder) addIPv6ICMPRules() {
	// Router Solicitations (ICMPv6 type 133)
	b.sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	b.sb.WriteString(`<ipv6 protocol='icmpv6'/>`)
	b.sb.WriteString(`<icmpv6 type='133'/>`)
	b.sb.WriteString(`</rule>`)

	// Neighbor Solicitations (ICMPv6 type 135)
	b.sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	b.sb.WriteString(`<ipv6 protocol='icmpv6'/>`)
	b.sb.WriteString(`<icmpv6 type='135'/>`)
	b.sb.WriteString(`</rule>`)
}

// addMACRule adds MAC spoofing protection rule.
func (b *nwFilterBuilder) addMACRule(mac string) {
	b.sb.WriteString(`<rule action='accept' direction='out' priority='500'>`)
	b.sb.WriteString(`<mac match='yes' srcmacaddr='`)
	b.sb.WriteString(escapeXML(mac))
	b.sb.WriteString(`'/>`)
	b.sb.WriteString(`</rule>`)
}

// addIPv4Rules adds IPv4 spoofing protection rules.
func (b *nwFilterBuilder) addIPv4Rules(ipv4s []string) {
	for i, ip := range ipv4s {
		if ip == "" {
			continue
		}
		priority := 100 + i
		b.sb.WriteString(`<rule action='accept' direction='out' priority='`)
		b.sb.WriteString(fmt.Sprintf("%d", priority))
		b.sb.WriteString(`'>`)
		b.sb.WriteString(`<ip match='yes' srcipaddr='`)
		b.sb.WriteString(escapeXML(ip))
		b.sb.WriteString(`'/>`)
		b.sb.WriteString(`</rule>`)
	}
}

// addIPv6Rules adds IPv6 spoofing protection rules.
func (b *nwFilterBuilder) addIPv6Rules(ipv6s []string) {
	for i, ip := range ipv6s {
		if ip == "" {
			continue
		}
		priority := 200 + i
		b.sb.WriteString(`<rule action='accept' direction='out' priority='`)
		b.sb.WriteString(fmt.Sprintf("%d", priority))
		b.sb.WriteString(`'>`)
		b.sb.WriteString(`<ipv6 match='yes' srcipaddr='`)
		b.sb.WriteString(escapeXML(ip))
		b.sb.WriteString(`'/>`)
		b.sb.WriteString(`</rule>`)
	}
}

// addARPRules adds ARP spoofing protection rules.
func (b *nwFilterBuilder) addARPRules(mac string, ipv4s []string) {
	for _, ip := range ipv4s {
		if ip == "" {
			continue
		}
		b.sb.WriteString(`<rule action='accept' direction='inout' priority='50'>`)
		b.sb.WriteString(`<arp match='yes' arpsrcmacaddr='`)
		b.sb.WriteString(escapeXML(mac))
		b.sb.WriteString(`' arpsrcipaddr='`)
		b.sb.WriteString(escapeXML(ip))
		b.sb.WriteString(`'/>`)
		b.sb.WriteString(`</rule>`)
	}
}

// addDefaultDrop adds explicit drop rule for unmatched traffic.
func (b *nwFilterBuilder) addDefaultDrop() {
	b.sb.WriteString(`<rule action='drop' direction='out' priority='1000'>`)
	b.sb.WriteString(`<all/>`)
	b.sb.WriteString(`</rule>`)
}

// build returns the final XML string.
func (b *nwFilterBuilder) build() string {
	b.sb.WriteString(`</filter>`)
	return b.sb.String()
}