package controller

import (
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	apierrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
)

// healthHandler returns a simple liveness check.
func (s *Server) healthHandler(c *gin.Context) {
	respondJSON(c, http.StatusOK, gin.H{"status": "ok"})
}

// readinessHandler checks if the server is ready to serve requests.
func (s *Server) readinessHandler(c *gin.Context) {
	ctx := c.Request.Context()

	// Check database connection
	if err := s.dbPool.Ping(ctx); err != nil {
		respondError(c, &apierrors.APIError{
			Code:       "DATABASE_UNAVAILABLE",
			Message:    "Database connection failed",
			HTTPStatus: http.StatusServiceUnavailable,
		})
		return
	}

	// Check NATS connection status
	natsStatus := "connected"
	nodeCount := 0

	if s.natsConn == nil || s.natsConn.Status() != nats.CONNECTED {
		natsStatus = "disconnected"
	}

	// Get node count from gRPC client
	if s.nodeClient != nil {
		nodeCount = s.nodeClient.ConnectionCount()
	}

	respondJSON(c, http.StatusOK, gin.H{
		"status":     "ready",
		"database":   "connected",
		"nats":       natsStatus,
		"node_count": nodeCount,
	})
}

// requestLogger returns a middleware that logs HTTP requests.
func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logger := s.logger.With(
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"correlation_id", middleware.GetCorrelationID(c),
		)

		switch {
		case status >= 500:
			logger.Error("request completed with error")
		case status >= 400:
			logger.Warn("request completed with client error")
		default:
			logger.Debug("request completed")
		}
	}
}
