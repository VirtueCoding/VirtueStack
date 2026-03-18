// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CorrelationIDHeader is the header name for correlation IDs.
const CorrelationIDHeader = "X-Correlation-Id"

// CorrelationIDContextKey is the context key for storing correlation IDs.
const CorrelationIDContextKey = "correlation_id"

// CorrelationID is a middleware that adds a unique correlation ID to each request.
// If the request already has an X-Correlation-Id header, it uses that value.
// Otherwise, it generates a new UUID.
// The correlation ID is stored in the Gin context and added to response headers.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Propagate caller-supplied correlation ID so distributed traces remain
		// joinable across service boundaries; generate one when absent so every
		// request carries a traceable identity regardless of the caller.
		correlationID := c.GetHeader(CorrelationIDHeader)
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Store in context so handlers and middleware can attach it to logs
		// and outbound requests without coupling to the HTTP layer.
		c.Set(CorrelationIDContextKey, correlationID)

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
	return ""
}