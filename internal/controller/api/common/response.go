package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
