package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func (h *AdminHandler) ListSystemWebhooks(c *gin.Context) {
	webhooks, err := h.systemWebhookRepo.List(c.Request.Context())
	if err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError, "SYSTEM_WEBHOOK_LIST_FAILED", "Failed to list system webhooks")
		return
	}
	c.JSON(http.StatusOK, models.Response{Data: webhooks})
}

func (h *AdminHandler) CreateSystemWebhook(c *gin.Context) {
	var req models.SystemWebhookCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	webhook := &models.SystemWebhook{
		Name:     req.Name,
		URL:      req.URL,
		Secret:   req.Secret,
		Events:   req.Events,
		IsActive: active,
	}
	if err := h.systemWebhookRepo.Create(c.Request.Context(), webhook); err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError, "SYSTEM_WEBHOOK_CREATE_FAILED", "Failed to create system webhook")
		return
	}

	h.logAuditEvent(c, "system_webhook.create", "system_webhook", webhook.ID, map[string]any{
		"name": webhook.Name, "url": webhook.URL, "events": webhook.Events, "is_active": webhook.IsActive,
	}, true)
	c.JSON(http.StatusCreated, models.Response{Data: webhook})
}

func (h *AdminHandler) UpdateSystemWebhook(c *gin.Context) {
	id, ok := validateUUIDParam(c, "id", "INVALID_SYSTEM_WEBHOOK_ID", "System webhook ID must be a valid UUID")
	if !ok {
		return
	}

	var req models.SystemWebhookUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	updated, err := h.systemWebhookRepo.Update(c.Request.Context(), id, &req, req.Secret)
	if err != nil {
		if handleNotFoundError(c, err, "SYSTEM_WEBHOOK_NOT_FOUND", "System webhook not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "SYSTEM_WEBHOOK_UPDATE_FAILED", "Failed to update system webhook")
		return
	}

	h.logAuditEvent(c, "system_webhook.update", "system_webhook", id, map[string]any{
		"name": updated.Name, "url": updated.URL, "events": updated.Events, "is_active": updated.IsActive,
	}, true)
	c.JSON(http.StatusOK, models.Response{Data: updated})
}

func (h *AdminHandler) DeleteSystemWebhook(c *gin.Context) {
	id, ok := validateUUIDParam(c, "id", "INVALID_SYSTEM_WEBHOOK_ID", "System webhook ID must be a valid UUID")
	if !ok {
		return
	}

	if err := h.systemWebhookRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, sharederrors.ErrNoRowsAffected) {
			middleware.RespondWithError(c, http.StatusNotFound, "SYSTEM_WEBHOOK_NOT_FOUND", "System webhook not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "SYSTEM_WEBHOOK_DELETE_FAILED", "Failed to delete system webhook")
		return
	}

	h.logAuditEvent(c, "system_webhook.delete", "system_webhook", id, nil, true)
	c.Status(http.StatusNoContent)
}
