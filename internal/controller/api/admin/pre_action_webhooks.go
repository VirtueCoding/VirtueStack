package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// ListPreActionWebhooks handles GET /pre-action-webhooks.
func (h *AdminHandler) ListPreActionWebhooks(c *gin.Context) {
	webhooks, err := h.preActionWebhookRepo.List(c.Request.Context())
	if err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PRE_ACTION_WEBHOOK_LIST_FAILED", "Failed to list pre-action webhooks")
		return
	}
	c.JSON(http.StatusOK, models.Response{Data: webhooks})
}

// CreatePreActionWebhook handles POST /pre-action-webhooks.
func (h *AdminHandler) CreatePreActionWebhook(c *gin.Context) {
	var req models.PreActionWebhookCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	timeoutMs := 5000
	if req.TimeoutMs != nil {
		timeoutMs = *req.TimeoutMs
	}
	failOpen := true
	if req.FailOpen != nil {
		failOpen = *req.FailOpen
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	webhook := &models.PreActionWebhook{
		Name:      req.Name,
		URL:       req.URL,
		Secret:    req.Secret,
		Events:    req.Events,
		TimeoutMs: timeoutMs,
		FailOpen:  failOpen,
		IsActive:  active,
	}
	if err := h.preActionWebhookRepo.Create(c.Request.Context(), webhook); err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PRE_ACTION_WEBHOOK_CREATE_FAILED", "Failed to create pre-action webhook")
		return
	}

	h.logAuditEvent(c, "pre_action_webhook.create", "pre_action_webhook", webhook.ID, map[string]any{
		"name": webhook.Name, "url": webhook.URL, "events": webhook.Events,
		"timeout_ms": webhook.TimeoutMs, "fail_open": webhook.FailOpen, "is_active": webhook.IsActive,
	}, true)
	c.JSON(http.StatusCreated, models.Response{Data: webhook})
}

// UpdatePreActionWebhook handles PUT /pre-action-webhooks/:id.
func (h *AdminHandler) UpdatePreActionWebhook(c *gin.Context) {
	id, ok := validateUUIDParam(c, "id", "INVALID_ID", "Pre-action webhook ID must be a valid UUID")
	if !ok {
		return
	}

	var req models.PreActionWebhookUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	updated, err := h.preActionWebhookRepo.Update(c.Request.Context(), id, &req)
	if err != nil {
		if handleNotFoundError(c, err, "PRE_ACTION_WEBHOOK_NOT_FOUND", "Pre-action webhook not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PRE_ACTION_WEBHOOK_UPDATE_FAILED", "Failed to update pre-action webhook")
		return
	}

	h.logAuditEvent(c, "pre_action_webhook.update", "pre_action_webhook", id, map[string]any{
		"name": updated.Name, "url": updated.URL, "events": updated.Events,
	}, true)
	c.JSON(http.StatusOK, models.Response{Data: updated})
}

// DeletePreActionWebhook handles DELETE /pre-action-webhooks/:id.
func (h *AdminHandler) DeletePreActionWebhook(c *gin.Context) {
	id, ok := validateUUIDParam(c, "id", "INVALID_ID", "Pre-action webhook ID must be a valid UUID")
	if !ok {
		return
	}

	if err := h.preActionWebhookRepo.Delete(c.Request.Context(), id); err != nil {
		if sharederrors.Is(err, sharederrors.ErrNoRowsAffected) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"PRE_ACTION_WEBHOOK_NOT_FOUND", "Pre-action webhook not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PRE_ACTION_WEBHOOK_DELETE_FAILED", "Failed to delete pre-action webhook")
		return
	}

	h.logAuditEvent(c, "pre_action_webhook.delete", "pre_action_webhook", id, nil, true)
	c.JSON(http.StatusOK, models.Response{Data: map[string]string{"status": "deleted"}})
}
