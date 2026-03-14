package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TemplateImportRequest represents the request body for importing a template.
type TemplateImportRequest struct {
	Name              string `json:"name" validate:"required,max=100"`
	OSFamily          string `json:"os_family" validate:"required,max=50"`
	OSVersion         string `json:"os_version" validate:"required,max=50"`
	SourcePath        string `json:"source_path" validate:"required,max=512"`
	SupportsCloudInit bool   `json:"supports_cloudinit"`
	IsActive          bool   `json:"is_active"`
}

// ListTemplates handles GET /templates - lists all OS templates.
func (h *AdminHandler) ListTemplates(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := repository.TemplateListFilter{
		PaginationParams: pagination,
	}

	// Optional active filter
	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		var isActive bool
		if isActiveStr == "true" {
			isActive = true
		} else if isActiveStr == "false" {
			isActive = false
		}
		filter.IsActive = &isActive
	}

	// Optional OS family filter
	if osFamily := c.Query("os_family"); osFamily != "" {
		filter.OSFamily = &osFamily
	}

	templates, total, err := h.templateService.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list templates",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "TEMPLATE_LIST_FAILED", "Failed to retrieve templates")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: templates,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateTemplate handles POST /templates - creates a new OS template.
func (h *AdminHandler) CreateTemplate(c *gin.Context) {
	var req models.TemplateCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	template := &models.Template{
		Name:              req.Name,
		OSFamily:          req.OSFamily,
		OSVersion:         req.OSVersion,
		RBDImage:          req.RBDImage,
		RBDSnapshot:       req.RBDSnapshot,
		MinDiskGB:         req.MinDiskGB,
		SupportsCloudInit: req.SupportsCloudInit,
		IsActive:          req.IsActive,
		SortOrder:         req.SortOrder,
		Description:       req.Description,
	}

	err := h.templateService.Create(c.Request.Context(), template)
	if err != nil {
		h.logger.Error("failed to create template",
			"name", req.Name,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "TEMPLATE_CREATE_FAILED", err.Error())
		return
	}

	h.logger.Info("template creation requested via admin API",
		"name", req.Name,
		"os_family", req.OSFamily,
		"correlation_id", middleware.GetCorrelationID(c))

	// Log audit event
	h.logAuditEvent(c, "template.create", "template", "", map[string]interface{}{
		"name":         req.Name,
		"os_family":    req.OSFamily,
		"os_version":   req.OSVersion,
		"rbd_image":    req.RBDImage,
		"rbd_snapshot": req.RBDSnapshot,
	}, true)

	c.JSON(http.StatusCreated, models.Response{Data: template})
}

// GetTemplate handles GET /templates/:id - retrieves a specific template.
func (h *AdminHandler) GetTemplate(c *gin.Context) {
	templateID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(templateID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	template, err := h.templateService.GetByID(c.Request.Context(), templateID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found")
			return
		}
		h.logger.Error("failed to get template",
			"template_id", templateID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "TEMPLATE_GET_FAILED", "Failed to retrieve template")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: template})
}

// UpdateTemplate handles PUT /templates/:id - updates an existing template.
func (h *AdminHandler) UpdateTemplate(c *gin.Context) {
	templateID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(templateID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	var req models.TemplateUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	template, err := h.templateService.Update(c.Request.Context(), templateID, &req)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found")
			return
		}
		h.logger.Error("failed to update template",
			"template_id", templateID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusBadRequest, "TEMPLATE_UPDATE_FAILED", err.Error())
		return
	}

	h.logAuditEvent(c, "template.update", "template", templateID, req, true)

	h.logger.Info("template updated via admin API",
		"template_id", templateID,
		"version", template.Version,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: template})
}

// DeleteTemplate handles DELETE /templates/:id - deletes a template.
func (h *AdminHandler) DeleteTemplate(c *gin.Context) {
	templateID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(templateID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	err := h.templateService.Delete(c.Request.Context(), templateID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found")
			return
		}
		h.logger.Error("failed to delete template",
			"template_id", templateID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "TEMPLATE_DELETE_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "template.delete", "template", templateID, nil, true)

	h.logger.Info("template deleted via admin API",
		"template_id", templateID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"deleted": true}})
}

// ImportTemplate handles POST /templates/:id/import - imports an OS image.
// This is used to upload/import a disk image into Ceph RBD.
func (h *AdminHandler) ImportTemplate(c *gin.Context) {
	templateID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(templateID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	var req TemplateImportRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Import template through service
	template, err := h.templateService.Import(c.Request.Context(), req.Name, req.OSFamily, req.OSVersion, req.SourcePath)
	if err != nil {
		h.logger.Error("failed to import template",
			"name", req.Name,
			"source_path", req.SourcePath,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "TEMPLATE_IMPORT_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "template.import", "template", template.ID, map[string]interface{}{
		"name":        req.Name,
		"os_family":   req.OSFamily,
		"os_version":  req.OSVersion,
		"source_path": req.SourcePath,
	}, true)

	h.logger.Info("template imported via admin API",
		"template_id", template.ID,
		"name", req.Name,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: template})
}
