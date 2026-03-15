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
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		controllermetrics.APIRequestsTotal.WithLabelValues(method, path, status).Inc()
		controllermetrics.APIRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}
