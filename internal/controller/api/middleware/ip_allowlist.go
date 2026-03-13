package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// IPAllowlistConfig configures the IP allowlist middleware.
type IPAllowlistConfig struct {
	// AllowedIPs is a list of allowed IP addresses or CIDR ranges.
	AllowedIPs []string
	// Enabled controls whether the middleware is active.
	Enabled bool
}

// IPAllowlist returns a middleware that restricts access to allowed IPs.
// If no allowed IPs are configured and the middleware is enabled, all requests are denied.
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
			if err == nil {
				allowedNets = append(allowedNets, ipNet)
			}
		} else {
			ip := net.ParseIP(entry)
			if ip != nil {
				allowedIPs = append(allowedIPs, ip)
			}
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
