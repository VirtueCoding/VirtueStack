package common

import (
	"fmt"
	"strconv"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

const (
	defaultCursorLimit = 20
	maxCursorLimit     = 100
)

// ParsePaginationParams parses cursor pagination params using shared model defaults.
func ParsePaginationParams(c *gin.Context) models.PaginationParams {
	return models.ParsePagination(c)
}

// ParseCursorParams parses cursor pagination params.
func ParseCursorParams(c *gin.Context) (string, int, error) {
	cursor := c.Query("cursor")
	limit := defaultCursorLimit

	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return "", 0, fmt.Errorf("invalid limit: %w", err)
		}
		if parsed < 1 || parsed > maxCursorLimit {
			return "", 0, fmt.Errorf("limit must be between 1 and %d", maxCursorLimit)
		}
		limit = parsed
	}

	return cursor, limit, nil
}
