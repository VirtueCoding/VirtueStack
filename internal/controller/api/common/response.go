package common

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// RespondWithPaginatedList returns a standard paginated list response.
func RespondWithPaginatedList(c *gin.Context, data interface{}, total int, page, perPage int) {
	c.JSON(http.StatusOK, models.ListResponse{
		Data: data,
		Meta: models.NewPaginationMeta(page, perPage, total),
	})
}

// RespondWithCursorList returns a standard cursor-based list response.
func RespondWithCursorList(c *gin.Context, data interface{}, nextCursor string, hasMore bool) {
	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"meta": gin.H{
			"next_cursor": nextCursor,
			"has_more":    hasMore,
		},
	})
}
