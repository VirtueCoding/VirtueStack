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
		// Check if correlation ID already exists in request headers
		correlationID := c.GetHeader(CorrelationIDHeader)

		// Generate a new correlation ID if not present
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Set in Gin context for use in handlers
		c.Set(CorrelationIDContextKey, correlationID)

		// Add to response headers
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