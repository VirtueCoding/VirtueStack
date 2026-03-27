package admin

import (
	"errors"
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
	StorageBackend    string `json:"storage_backend" validate:"omitempty,oneof=ceph qcow"`
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
		switch isActiveStr {
		case "true":
			isActive = true
		case "false":
			isActive = false
		default:
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_IS_ACTIVE", "is_active must be 'true' or 'false'")
			return
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
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_LIST_FAILED", "Failed to retrieve templates")
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
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
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
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_CREATE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("template creation requested via admin API",
		"name", req.Name,
		"os_family", req.OSFamily,
		"correlation_id", middleware.GetCorrelationID(c))

	// Log audit event - pass template.ID as the resource_id (F-164)
	h.logAuditEvent(c, "template.create", "template", template.ID, map[string]interface{}{
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
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	template, err := h.templateService.GetByID(c.Request.Context(), templateID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found")
			return
		}
		h.logger.Error("failed to get template",
			"template_id", templateID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_GET_FAILED", "Failed to retrieve template")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: template})
}

// UpdateTemplate handles PUT /templates/:id - updates an existing template.
func (h *AdminHandler) UpdateTemplate(c *gin.Context) {
	templateID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(templateID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	var req models.TemplateUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	template, err := h.templateService.Update(c.Request.Context(), templateID, &req)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found")
			return
		}
		h.logger.Error("failed to update template",
			"template_id", templateID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_UPDATE_FAILED", "Internal server error")
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
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	err := h.templateService.Delete(c.Request.Context(), templateID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found")
			return
		}
		h.logger.Error("failed to delete template",
			"template_id", templateID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_DELETE_FAILED", "Internal server error")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "template.delete", "template", templateID, nil, true)

	h.logger.Info("template deleted via admin API",
		"template_id", templateID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}

// ImportTemplate handles POST /templates/:id/import - imports an OS image.
// This is used to upload/import a disk image into Ceph RBD.
func (h *AdminHandler) ImportTemplate(c *gin.Context) {
	templateID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(templateID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	var req TemplateImportRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Import template through service
	template, err := h.templateService.Import(c.Request.Context(), req.Name, req.OSFamily, req.OSVersion, req.SourcePath, req.StorageBackend)
	if err != nil {
		h.logger.Error("failed to import template",
			"name", req.Name,
			"source_path", req.SourcePath,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_IMPORT_FAILED", "Internal server error")
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

// BuildTemplateFromISO handles POST /templates/build-from-iso.
// Starts an async task to build a VM template from an ISO using unattended installation.
// The ISO can be specified as a local filesystem path (iso_path) or an HTTP/HTTPS URL (iso_url).
func (h *AdminHandler) BuildTemplateFromISO(c *gin.Context) {
	var req models.TemplateBuildFromISORequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	if req.ISOPath == "" && req.ISOURL == "" {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Either iso_path or iso_url is required")
		return
	}
	if req.ISOPath != "" && req.ISOURL != "" {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Only one of iso_path or iso_url may be provided")
		return
	}

	if req.DiskSizeGB == 0 {
		req.DiskSizeGB = 10
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 2048
	}
	if req.VCPUs == 0 {
		req.VCPUs = 2
	}

	taskID, err := h.templateService.BuildFromISO(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("failed to start template build from ISO",
			"name", req.Name,
			"os_family", req.OSFamily,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NODE_NOT_FOUND", "Node not found")
			return
		}
		if errors.Is(err, sharederrors.ErrValidation) {
			middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "TEMPLATE_BUILD_FAILED", "Failed to start template build")
		return
	}

	isoSource := req.ISOPath
	if isoSource == "" {
		isoSource = req.ISOURL
	}

	h.logAuditEvent(c, "template.build_from_iso", "template", taskID, map[string]interface{}{
		"name":            req.Name,
		"os_family":       req.OSFamily,
		"os_version":      req.OSVersion,
		"iso_source":      isoSource,
		"node_id":         req.NodeID,
		"storage_backend": req.StorageBackend,
	}, true)

	h.logger.Info("template build from ISO started",
		"task_id", taskID,
		"name", req.Name,
		"os_family", req.OSFamily,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{
		Data: map[string]string{"task_id": taskID},
	})
}
