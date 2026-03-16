// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	// defaultCSRFCookieName is the default cookie name for CSRF token.
	defaultCSRFCookieName = "csrf_token"

	// defaultCSRFHeaderName is the default header name for CSRF token.
	defaultCSRFHeaderName = "X-CSRF-Token"

	// csrfTokenLength is the length of the CSRF token in bytes.
	csrfTokenLength = 32
)

// CSRFConfig holds configuration for CSRF protection.
type CSRFConfig struct {
	// CookieName is the name of the cookie to store the CSRF token.
	CookieName string

	// HeaderName is the name of the header to read the CSRF token from.
	HeaderName string

	// Secure indicates if the cookie should be set with the Secure flag.
	Secure bool

	// MaxAge is the maximum age of the CSRF cookie in seconds.
	MaxAge int

	// CookiePath is the path for the CSRF cookie.
	CookiePath string

	// CookieDomain is the domain for the CSRF cookie.
	CookieDomain string
}

// DefaultCSRFConfig returns a CSRFConfig with sensible defaults.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		CookieName: defaultCSRFCookieName,
		HeaderName: defaultCSRFHeaderName,
		Secure:     true,
		MaxAge:     86400, // 24 hours
		CookiePath: "/",
	}
}

// CSRF returns middleware that validates CSRF tokens for state-changing requests.
// It uses the double-submit cookie pattern to prevent CSRF attacks.
//
// For GET requests, the middleware generates a new CSRF token and sets it as both
// a cookie and in the response header.
//
// For state-changing requests (POST, PUT, DELETE, PATCH), the middleware validates
// that the token in the cookie matches the token in the X-CSRF-Token header using
// constant-time comparison to prevent timing attacks.
//
// Skip validation for GET, HEAD, and OPTIONS requests.
func CSRF(config CSRFConfig) gin.HandlerFunc {
	// Apply defaults
	if config.CookieName == "" {
		config.CookieName = defaultCSRFCookieName
	}
	if config.HeaderName == "" {
		config.HeaderName = defaultCSRFHeaderName
	}
	if config.MaxAge == 0 {
		config.MaxAge = 86400
	}
	if config.CookiePath == "" {
		config.CookiePath = "/"
	}

	return func(c *gin.Context) {
		// Skip validation for safe methods
		if c.Request.Method == http.MethodGet ||
			c.Request.Method == http.MethodHead ||
			c.Request.Method == http.MethodOptions {
			// Generate new token for GET requests
			token, err := generateToken()
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Failed to generate CSRF token",
				})
				return
			}

			// Set HttpOnly cookie
			c.SetCookie(
				config.CookieName,
				token,
				config.MaxAge,
				config.CookiePath,
				config.CookieDomain,
				config.Secure,
				true, // HttpOnly
			)

			// Set in header for SPA consumption
			c.Header(config.HeaderName, token)

			c.Next()
			return
		}

		// For state-changing methods, validate CSRF token
		cookieToken, cookieErr := c.Cookie(config.CookieName)
		headerToken := c.GetHeader(config.HeaderName)

		// Reject if cookie is missing
		if cookieErr != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF cookie missing",
			})
			return
		}

		// Reject if header is missing
		if headerToken == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF token header missing",
			})
			return
		}

		// Use constant-time comparison to prevent timing attacks
		cookieValid := subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) == 1

		if !cookieValid {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "CSRF token mismatch",
			})
			return
		}

		// Token is valid, regenerate for this request
		token, err := generateToken()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to generate CSRF token",
			})
			return
		}

		// Set new HttpOnly cookie
		c.SetCookie(
			config.CookieName,
			token,
			config.MaxAge,
			config.CookiePath,
			config.CookieDomain,
			config.Secure,
			true, // HttpOnly
		)

		// Set new token in header for subsequent requests
		c.Header(config.HeaderName, token)

		c.Next()
	}
}

// generateToken creates a cryptographically secure random token.
func generateToken() (string, error) {
	b := make([]byte, csrfTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate CSRF token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
