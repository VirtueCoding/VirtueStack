package util

import "net"

// privateRanges is the list of CIDR blocks that are blocked for outbound
// webhook/SSRF-sensitive connections. It includes RFC-1918 private ranges,
// loopback, link-local (including cloud metadata endpoints), CGNAT, and
// IPv6 equivalents. 0.0.0.0/8 is included because on some Linux
// configurations it routes to the loopback interface.
var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8",     // "This" network; routes to loopback on some Linux configs
		"10.0.0.0/8",    // RFC-1918 private
		"172.16.0.0/12", // RFC-1918 private
		"192.168.0.0/16", // RFC-1918 private
		"127.0.0.0/8",   // loopback
		"169.254.0.0/16", // link-local / cloud metadata (covers 169.254.169.254)
		"100.64.0.0/10", // CGNAT (RFC-6598)
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique-local (ULA)
		"fe80::/10",      // IPv6 link-local
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, network)
		}
	}
	return nets
}()

// metadataIPs contains explicit IP addresses that must always be blocked,
// even if they happen to fall inside one of the CIDR ranges above. This
// provides an extra layer of defense for cloud-provider metadata endpoints.
var metadataIPs = func() []net.IP {
	raw := []string{"169.254.169.254", "fd00:ec2::254"}
	ips := make([]net.IP, 0, len(raw))
	for _, s := range raw {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}()

// IsPrivateIP returns true if ip falls within any private, loopback,
// link-local, CGNAT, or otherwise reserved range that should not be
// reachable from outbound webhook or SSRF-sensitive HTTP requests.
func IsPrivateIP(ip net.IP) bool {
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	for _, mip := range metadataIPs {
		if ip.Equal(mip) {
			return true
		}
	}
	return false
}
