// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"context"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CorrelationIDHeader is the header name for correlation IDs.
const CorrelationIDHeader = "X-Correlation-Id"

// CorrelationIDContextKey is the context key for storing correlation IDs.
const CorrelationIDContextKey = "correlation_id"

type correlationIDRequestContextKey struct{}

// WithCorrelationID stores a correlation ID in a request context for downstream use.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, correlationIDRequestContextKey{}, correlationID)
}

// correlationIDPattern matches the standard UUID format (8-4-4-4-12 hex with hyphens)
// or bounded alphanumeric strings up to 64 characters. Anything else is rejected and
// replaced with a freshly generated UUID to prevent header injection and log pollution.
var correlationIDPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$|^[a-zA-Z0-9_\-]{1,64}$`,
)

// isValidCorrelationID returns true when id matches the accepted format.
func isValidCorrelationID(id string) bool {
	return correlationIDPattern.MatchString(id)
}

// CorrelationID is a middleware that adds a unique correlation ID to each request.
// If the request already has an X-Correlation-Id header containing a valid UUID or
// bounded alphanumeric string (up to 64 chars), it uses that value to maintain
// distributed trace continuity. Otherwise it generates a fresh UUID.
// The correlation ID is stored in the Gin context and added to response headers.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Propagate caller-supplied correlation ID so distributed traces remain
		// joinable across service boundaries. Validate the incoming value before
		// accepting it to prevent header injection attacks and unbounded string
		// values reaching log sinks. Generate a fresh UUID when the value is
		// absent or fails validation.
		correlationID := c.GetHeader(CorrelationIDHeader)
		if correlationID == "" || !isValidCorrelationID(correlationID) {
			correlationID = uuid.New().String()
		}

		// Store in context so handlers and middleware can attach it to logs
		// and outbound requests without coupling to the HTTP layer.
		c.Set(CorrelationIDContextKey, correlationID)
		c.Request = c.Request.WithContext(WithCorrelationID(c.Request.Context(), correlationID))

		// Echo back in the response so clients can correlate their request
		// with server-side log entries.
		c.Header(CorrelationIDHeader, correlationID)

		c.Next()
	}
}

// GetCorrelationID retrieves the correlation ID from the Gin context.
// Returns an empty string if not found.
func GetCorrelationID(c *gin.Context) string {
	if id, exists := c.Get(CorrelationIDContextKey); exists {
		if strID, ok := id.(string); ok {
			return strID
		}
	}
	if c.Request == nil {
		return ""
	}
	return GetCorrelationIDFromContext(c.Request.Context())
}

// GetCorrelationIDFromContext retrieves the correlation ID from a request context.
// Returns an empty string if not found.
func GetCorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(correlationIDRequestContextKey{}).(string); ok {
		return id
	}
	return ""
}
