package customer

import (
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

// CreateWebhookRequest represents the request body for creating a webhook.
type CreateWebhookRequest struct {
	URL    string   `json:"url" validate:"required,url,max=2048"`
	Secret string   `json:"secret" validate:"required,min=16,max=128"`
	Events []string `json:"events" ` +
	`validate:"required,min=1,dive,max=100"`
}

// UpdateWebhookRequest represents the request body for updating a webhook.
type UpdateWebhookRequest struct {
	URL      *string  `json:"url,omitempty" validate:"omitempty,url,max=2048"`
	Secret   *string  `json:"secret,omitempty" validate:"omitempty,min=16,max=128"`
	Events   []string `json:"events,omitempty" validate:"omitempty,min=1,dive,max=100"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// WebhookResponse represents a webhook in responses.
type WebhookResponse struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	Events        []string   `json:"events"`
	IsActive      bool       `json:"is_active"`
	FailCount     int        `json:"fail_count"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

// WebhookDeliveryResponse represents a webhook delivery attempt.
type WebhookDeliveryResponse struct {
	ID             string     `json:"id"`
	Event          string     `json:"event"`
	AttemptCount   int        `json:"attempt_count"`
	ResponseStatus *int       `json:"response_status,omitempty"`
	Success        bool       `json:"success"`
	NextRetryAt    *string    `json:"next_retry_at,omitempty"`
	DeliveredAt    *string    `json:"delivered_at,omitempty"`
	CreatedAt      string     `json:"created_at"`
}

// toWebhookResponse converts a repository webhook to an API response.
func toWebhookResponse(w *repository.Webhook) WebhookResponse {
	return WebhookResponse{
		ID:            w.ID,
		URL:           w.URL,
		Events:        w.Events,
		IsActive:      w.Active,
		FailCount:     w.FailCount,
		LastSuccessAt: w.LastSuccessAt,
		LastFailureAt: w.LastFailureAt,
		CreatedAt:     w.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     w.UpdatedAt.Format(time.RFC3339),
	}
}

// toDeliveryResponse converts a repository delivery to an API response.
func toDeliveryResponse(d *repository.WebhookDelivery) WebhookDeliveryResponse {
	success := d.Status == repository.DeliveryStatusDelivered
	resp := WebhookDeliveryResponse{
		ID:           d.ID,
		Event:        d.Event,
		AttemptCount: d.AttemptCount,
		ResponseStatus: d.ResponseStatus,
		Success:      success,
		CreatedAt:    d.CreatedAt.Format(time.RFC3339),
	}
	if d.NextRetryAt != nil {
		t := d.NextRetryAt.Format(time.RFC3339)
		resp.NextRetryAt = &t
	}
	if d.DeliveredAt != nil {
		t := d.DeliveredAt.Format(time.RFC3339)
		resp.DeliveredAt = &t
	}
	return resp
}

// ListWebhooks handles GET /webhooks - lists all webhooks for the customer.
func (h *CustomerHandler) ListWebhooks(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	webhooks, err := h.webhookService.List(c.Request.Context(), customerID)
	if err != nil {
		h.logger.Error("failed to list webhooks",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list webhooks")
		return
	}

	// Convert to response format
	responses := make([]WebhookResponse, len(webhooks))
	for i, w := range webhooks {
		responses[i] = toWebhookResponse(&w)
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: responses,
		Meta: models.NewPaginationMeta(1, 20, len(responses)),
	})
}

// CreateWebhook handles POST /webhooks - creates a new webhook endpoint.
// The secret is used to sign webhook payloads for verification.
func (h *CustomerHandler) CreateWebhook(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Validate events
	for _, event := range req.Events {
		if !services.ValidWebhookEvents[event] {
			respondWithError(c, http.StatusBadRequest, "INVALID_EVENT", "Invalid webhook event: "+event)
			return
		}
	}

	// Create webhook via service
	webhook, err := h.webhookService.Create(c.Request.Context(), services.CreateWebhookRequest{
		CustomerID: customerID,
		URL:        req.URL,
		Secret:     req.Secret,
		Events:     req.Events,
	})
	if err != nil {
		switch err {
		case services.ErrInvalidURL:
			respondWithError(c, http.StatusBadRequest, "INVALID_URL", "Webhook URL must be HTTPS")
		case services.ErrInvalidEvent:
			respondWithError(c, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		case services.ErrTooManyWebhooks:
			respondWithError(c, http.StatusBadRequest, "LIMIT_EXCEEDED", "Maximum webhook limit reached (5)")
		case services.ErrSecretTooShort:
			respondWithError(c, http.StatusBadRequest, "INVALID_SECRET", "Secret must be at least 16 characters")
		case services.ErrSecretTooLong:
			respondWithError(c, http.StatusBadRequest, "INVALID_SECRET", "Secret must be at most 128 characters")
		default:
			h.logger.Error("failed to create webhook",
				"customer_id", customerID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create webhook")
		}
		return
	}

	h.logger.Info("webhook created",
		"webhook_id", webhook.ID,
		"customer_id", customerID,
		"url", req.URL,
		"events", req.Events,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: toWebhookResponse(webhook)})
}

// GetWebhook handles GET /webhooks/:id - gets a specific webhook.
func (h *CustomerHandler) GetWebhook(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	webhookID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(webhookID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_WEBHOOK_ID", "Webhook ID must be a valid UUID")
		return
	}

	webhook, err := h.webhookService.Get(c.Request.Context(), webhookID, customerID)
	if err != nil {
		if err == services.ErrWebhookNotFound {
			respondWithError(c, http.StatusNotFound, "NOT_FOUND", "Webhook not found")
			return
		}
		h.logger.Error("failed to get webhook",
			"webhook_id", webhookID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get webhook")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: toWebhookResponse(webhook)})
}

// UpdateWebhook handles PUT /webhooks/:id - updates a webhook endpoint.
func (h *CustomerHandler) UpdateWebhook(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	webhookID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(webhookID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_WEBHOOK_ID", "Webhook ID must be a valid UUID")
		return
	}

	var req UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Validate events if provided
	if req.Events != nil {
		for _, event := range req.Events {
			if !services.ValidWebhookEvents[event] {
				respondWithError(c, http.StatusBadRequest, "INVALID_EVENT", "Invalid webhook event: "+event)
				return
			}
		}
	}

	// Build update request
	updateReq := services.UpdateWebhookRequest{
		URL:    req.URL,
		Secret: req.Secret,
		Events: req.Events,
	}
	if req.IsActive != nil {
		updateReq.Active = req.IsActive
	}

	webhook, err := h.webhookService.Update(c.Request.Context(), webhookID, customerID, updateReq)
	if err != nil {
		switch err {
		case services.ErrWebhookNotFound:
			respondWithError(c, http.StatusNotFound, "NOT_FOUND", "Webhook not found")
		case services.ErrInvalidURL:
			respondWithError(c, http.StatusBadRequest, "INVALID_URL", "Webhook URL must be HTTPS")
		case services.ErrInvalidEvent:
			respondWithError(c, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		case services.ErrSecretTooShort:
			respondWithError(c, http.StatusBadRequest, "INVALID_SECRET", "Secret must be at least 16 characters")
		case services.ErrSecretTooLong:
			respondWithError(c, http.StatusBadRequest, "INVALID_SECRET", "Secret must be at most 128 characters")
		default:
			h.logger.Error("failed to update webhook",
				"webhook_id", webhookID,
				"customer_id", customerID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update webhook")
		}
		return
	}

	h.logger.Info("webhook updated",
		"webhook_id", webhookID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: toWebhookResponse(webhook)})
}

// DeleteWebhook handles DELETE /webhooks/:id - deletes a webhook endpoint.
func (h *CustomerHandler) DeleteWebhook(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	webhookID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(webhookID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_WEBHOOK_ID", "Webhook ID must be a valid UUID")
		return
	}

	err := h.webhookService.Delete(c.Request.Context(), webhookID, customerID)
	if err != nil {
		if err == services.ErrWebhookNotFound {
			respondWithError(c, http.StatusNotFound, "NOT_FOUND", "Webhook not found")
			return
		}
		h.logger.Error("failed to delete webhook",
			"webhook_id", webhookID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete webhook")
		return
	}

	h.logger.Info("webhook deleted",
		"webhook_id", webhookID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Webhook deleted successfully"}})
}

// ListWebhookDeliveries handles GET /webhooks/:id/deliveries - lists delivery attempts for a webhook.
func (h *CustomerHandler) ListWebhookDeliveries(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	webhookID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(webhookID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_WEBHOOK_ID", "Webhook ID must be a valid UUID")
		return
	}

	// Parse pagination
	pagination := models.ParsePagination(c)

	deliveries, total, err := h.webhookService.ListDeliveries(c.Request.Context(), webhookID, customerID, pagination.Page, pagination.PerPage)
	if err != nil {
		if err == services.ErrWebhookNotFound {
			respondWithError(c, http.StatusNotFound, "NOT_FOUND", "Webhook not found")
			return
		}
		h.logger.Error("failed to list webhook deliveries",
			"webhook_id", webhookID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list deliveries")
		return
	}

	// Convert to response format
	responses := make([]WebhookDeliveryResponse, len(deliveries))
	for i, d := range deliveries {
		responses[i] = toDeliveryResponse(&d)
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: responses,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}