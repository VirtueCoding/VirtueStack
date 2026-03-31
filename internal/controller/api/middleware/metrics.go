package middleware

import (
	"strconv"
	"time"

	controllermetrics "github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/gin-gonic/gin"
)

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method := c.Request.Method

		c.Next()

		// Use c.FullPath() (route template, e.g. "/vms/:id") instead of
		// c.Request.URL.Path (resolved path with UUIDs) to avoid unbounded
		// Prometheus label cardinality.
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		controllermetrics.APIRequestsTotal.WithLabelValues(method, path, status).Inc()
		controllermetrics.APIRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}
