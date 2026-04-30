package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ValidateIPAllowlistConfig validates an IPAllowlistConfig and returns an error if any
// entry is not a valid IP address or CIDR range. Call this at server startup so that
// misconfigured allowlists cause an immediate, visible failure rather than silently
// permitting or denying unexpected traffic at request time.
//
// Example startup usage:
//
//	if err := middleware.ValidateIPAllowlistConfig(cfg); err != nil {
//	    log.Fatalf("invalid IP allowlist configuration: %v", err)
//	}
func ValidateIPAllowlistConfig(config IPAllowlistConfig) error {
	for _, entry := range config.AllowedIPs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return fmt.Errorf("ip_allowlist: invalid CIDR entry %q: %w", entry, err)
			}
		} else {
			if net.ParseIP(entry) == nil {
				return fmt.Errorf("ip_allowlist: invalid IP entry %q", entry)
			}
		}
	}
	return nil
}

// IPAllowlistConfig configures the IP allowlist middleware.
type IPAllowlistConfig struct {
	// AllowedIPs is a list of allowed IP addresses or CIDR ranges.
	AllowedIPs []string
	// Enabled controls whether the middleware is active.
	Enabled bool
}

// IPAllowlist returns a middleware that restricts access to allowed IPs.
// If no allowed IPs are configured and the middleware is enabled, all requests are denied.
// Invalid CIDR or IP entries in AllowedIPs are logged as warnings and skipped so that
// a misconfiguration does not silently expand access beyond the intended allowlist.
func IPAllowlist(config IPAllowlistConfig) gin.HandlerFunc {
	if !config.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	var allowedNets []*net.IPNet
	var allowedIPs []net.IP

	for _, entry := range config.AllowedIPs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, ipNet, err := net.ParseCIDR(entry)
			if err != nil {
				slog.Warn("ip_allowlist: invalid CIDR entry skipped",
					"entry", entry,
					"error", err)
				continue
			}
			allowedNets = append(allowedNets, ipNet)
		} else {
			ip := net.ParseIP(entry)
			if ip == nil {
				slog.Warn("ip_allowlist: invalid IP entry skipped",
					"entry", entry)
				continue
			}
			allowedIPs = append(allowedIPs, ip)
		}
	}

	return func(c *gin.Context) {
		clientIP := net.ParseIP(c.ClientIP())
		if clientIP == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"code":    "IP_NOT_ALLOWED",
					"message": "Access denied",
				},
			})
			return
		}

		for _, ip := range allowedIPs {
			if ip.Equal(clientIP) {
				c.Next()
				return
			}
		}

		for _, ipNet := range allowedNets {
			if ipNet.Contains(clientIP) {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"code":    "IP_NOT_ALLOWED",
				"message": "Your IP address is not authorized to access this endpoint",
			},
		})
	}
}
