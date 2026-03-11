// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// Recovery is a middleware that recovers from panics in handlers.
// It logs the panic with stack trace and returns a 500 error with
// the standard VirtueStack error response format.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				// Get correlation ID for logging and response
				correlationID := GetCorrelationID(c)

				// Get stack trace
				stack := string(debug.Stack())

				// Log the panic with details
				logger.Error("panic recovered in handler",
					"error", r,
					"stack", stack,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"correlation_id", correlationID,
				)

				// Create standard error response
				apiErr := &errors.APIError{
					Code:       "INTERNAL_ERROR",
					Message:    "An internal error occurred. Please try again later.",
					HTTPStatus: http.StatusInternalServerError,
				}

				// Build response
				resp := ErrorResponse{
					Error: ErrorDetail{
						Code:          apiErr.Code,
						Message:       apiErr.Message,
						CorrelationID: correlationID,
					},
				}

				// Abort and respond with error
				c.AbortWithStatusJSON(http.StatusInternalServerError, resp)
			}
		}()

		c.Next()
	}
}

// ErrorDetail represents an error detail in API responses.
type ErrorDetail struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// ErrorResponse is the standard API error response format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}