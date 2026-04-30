package common

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// RespondWithCursorList returns a standard cursor-based list response.
func RespondWithCursorList(c *gin.Context, data any, perPage int, nextCursor string, hasMore bool) {
	c.JSON(http.StatusOK, models.ListResponse{
		Data: data,
		Meta: models.PaginationMeta{
			PerPage:    perPage,
			HasMore:    hasMore,
			NextCursor: nextCursor,
		},
	})
}
