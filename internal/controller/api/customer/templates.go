package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// validOSFamilies is the allowlist of accepted os_family query parameter values.
var validOSFamilies = map[string]bool{
	"debian":  true,
	"ubuntu":  true,
	"centos":  true,
	"rocky":   true,
	"almalinux": true,
	"fedora":  true,
	"freebsd": true,
	"windows": true,
	"other":   true,
}

// ListTemplates handles GET /templates - lists all available OS templates.
// Templates are public and can be viewed by all authenticated customers.
// This endpoint does not require a specific customer context.
func (h *CustomerHandler) ListTemplates(c *gin.Context) {
	// Templates are public (no customer isolation needed)
	// But we still require authentication

	// Parse pagination
	pagination := models.ParsePagination(c)

	// Optional OS family filter — validated against allowlist to prevent unbounded values.
	osFamily := c.Query("os_family")
	if osFamily != "" && !validOSFamilies[osFamily] {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_OS_FAMILY", "Invalid os_family value")
		return
	}

	// Get active templates
	if h.templateService == nil {
		// Fallback if template service is not available
		templates := []models.Template{}
		c.JSON(http.StatusOK, models.ListResponse{
			Data: templates,
			Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, 0),
		})
		return
	}

	// Get templates from service
	templates, err := h.templateService.ListActive(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list templates",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_LIST_FAILED", "Failed to retrieve templates")
		return
	}

	// Filter by OS family if specified
	if osFamily != "" {
		filtered := make([]models.Template, 0)
		for _, t := range templates {
			if t.OSFamily == osFamily {
				filtered = append(filtered, t)
			}
		}
		templates = filtered
	}

	h.logger.Info("templates listed",
		"count", len(templates),
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.ListResponse{
		Data: templates,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, len(templates)),
	})
}