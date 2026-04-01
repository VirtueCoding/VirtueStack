// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"crypto/rand"
	"crypto/sha256"
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

	// defaultCSRFMaxAge is the CSRF cookie lifetime in seconds (24 hours).
	// 24 hours balances security (limits token reuse window) with usability
	// (avoids requiring users to refresh the page within a short session).
	defaultCSRFMaxAge = 86400
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
		MaxAge:     defaultCSRFMaxAge,
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
		config.MaxAge = defaultCSRFMaxAge
	}
	if config.CookiePath == "" {
		config.CookiePath = "/"
	}

	return func(c *gin.Context) {
		// Skip validation for safe methods
		if c.Request.Method == http.MethodGet ||
			c.Request.Method == http.MethodHead ||
			c.Request.Method == http.MethodOptions {
			// Only generate a new CSRF token when no valid token cookie already exists.
			// Regenerating on every GET causes race conditions in SPAs that may issue
			// multiple concurrent GET requests: the first response overwrites the cookie
			// seen by subsequent responses, leaving the SPA with an inconsistent token.
			existingToken, cookieErr := c.Cookie(config.CookieName)
			if cookieErr != nil || existingToken == "" {
				var err error
				existingToken, err = generateToken()
				if err != nil {
					c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
						Error: ErrorDetail{Code: "CSRF_TOKEN_ERROR", Message: "Failed to generate CSRF token"},
					})
					return
				}

				// Set cookie with HttpOnly=false so JavaScript can read it for the double-submit pattern.
				// The CSRF token is not secret; its security relies on Same-Origin Policy.
				http.SetCookie(c.Writer, &http.Cookie{
					Name:     config.CookieName,
					Value:    existingToken,
					Path:     config.CookiePath,
					Domain:   config.CookieDomain,
					MaxAge:   config.MaxAge,
					Secure:   config.Secure,
					HttpOnly: false,
					SameSite: http.SameSiteStrictMode,
				})
			}

			// Always echo the current token in the response header so the SPA
			// can pick it up on the initial page load regardless of whether the
			// cookie was just created or already existed.
			c.Header(config.HeaderName, existingToken)

			c.Next()
			return
		}

		// For state-changing methods, validate CSRF token
		cookieToken, cookieErr := c.Cookie(config.CookieName)
		headerToken := c.GetHeader(config.HeaderName)

		// Reject if cookie is missing
		if cookieErr != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{
				Error: ErrorDetail{Code: "CSRF_COOKIE_MISSING", Message: "CSRF cookie missing"},
			})
			return
		}

		// Reject if header is missing
		if headerToken == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{
				Error: ErrorDetail{Code: "CSRF_HEADER_MISSING", Message: "CSRF token header missing"},
			})
			return
		}

		// Use constant-time comparison to prevent timing attacks.
		// subtle.ConstantTimeCompare returns 0 immediately when lengths differ,
		// leaking the length of the expected token via a timing side-channel.
		// To prevent this, hash both tokens to a fixed-length SHA-256 digest
		// before comparing so that the comparison always operates on equal-length
		// values regardless of the input lengths.
		cookieSHA := sha256.Sum256([]byte(cookieToken))
		headerSHA := sha256.Sum256([]byte(headerToken))
		cookieValid := subtle.ConstantTimeCompare(cookieSHA[:], headerSHA[:]) == 1

		if !cookieValid {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{
				Error: ErrorDetail{Code: "CSRF_TOKEN_MISMATCH", Message: "CSRF token mismatch"},
			})
			return
		}

		// Token is valid, regenerate for this request
		token, err := generateToken()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{Code: "CSRF_TOKEN_ERROR", Message: "Failed to generate CSRF token"},
			})
			return
		}

		// Set new cookie with HttpOnly=false so JavaScript can read it for the double-submit pattern.
		// The CSRF token is not secret; its security relies on Same-Origin Policy.
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     config.CookieName,
			Value:    token,
			Path:     config.CookiePath,
			Domain:   config.CookieDomain,
			MaxAge:   config.MaxAge,
			Secure:   config.Secure,
			HttpOnly: false,
			SameSite: http.SameSiteStrictMode,
		})

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

// SkipCSRFForAPIKey returns middleware that applies CSRF protection only for non-API-key requests.
// When an API key is present in the context (set by CustomerAPIKeyAuth), CSRF validation is skipped.
// This allows programmatic API access while still protecting browser-based JWT sessions.
func SkipCSRFForAPIKey(config CSRFConfig) gin.HandlerFunc {
	csrfHandler := CSRF(config)

	return func(c *gin.Context) {
		// F-158: Use actor_type as the authoritative indicator for API key auth.
		// actor_type == "customer_api_key" is set exclusively by CustomerAPIKeyAuth
		// and JWTOrCustomerAPIKeyAuth when customer API key authentication succeeds.
		if actorType, _ := c.Get("actor_type"); actorType == "customer_api_key" {
			c.Next()
			return
		}

		// Also skip if api_key_id is present (covers provisioning API key flows).
		apiKeyID, hasAPIKey := c.Get("api_key_id")
		if hasAPIKey && apiKeyID != "" {
			// Skip CSRF for API key authenticated requests
			c.Next()
			return
		}

		// Apply CSRF for JWT authenticated requests
		csrfHandler(c)
	}
}
