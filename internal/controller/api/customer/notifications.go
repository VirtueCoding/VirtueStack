package customer

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/common"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/audit"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// NotificationsHandler handles notification-related API endpoints.
type NotificationsHandler struct {
	preferenceRepo *repository.NotificationPreferenceRepository
	eventRepo      *repository.NotificationEventRepository
	notifyService  *services.NotificationService
	auditRepo      *repository.AuditRepository
	logger         *slog.Logger
}

// NewNotificationsHandler creates a new NotificationsHandler.
func NewNotificationsHandler(
	preferenceRepo *repository.NotificationPreferenceRepository,
	eventRepo *repository.NotificationEventRepository,
	notifyService *services.NotificationService,
	auditRepo *repository.AuditRepository,
	logger *slog.Logger,
) *NotificationsHandler {
	handlerLogger := slog.Default()
	if logger != nil {
		handlerLogger = logger.With("component", "customer-notifications-handler")
	}

	return &NotificationsHandler{
		preferenceRepo: preferenceRepo,
		eventRepo:      eventRepo,
		notifyService:  notifyService,
		auditRepo:      auditRepo,
		logger:         handlerLogger,
	}
}

// UpdateNotificationPreferencesRequest represents the request body for updating preferences.
type UpdateNotificationPreferencesRequest struct {
	EmailEnabled    *bool    `json:"email_enabled,omitempty"`
	TelegramEnabled *bool    `json:"telegram_enabled,omitempty"`
	Events          []string `json:"events,omitempty"`
}

// GetNotificationPreferences handles GET /notifications/preferences.
// Returns the customer's notification preferences.
// @Tags Customer
// @Summary Get notification preferences
// @Description Returns current notification preferences for the authenticated customer.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/notifications/preferences [get]
func (h *NotificationsHandler) GetNotificationPreferences(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	prefs, err := h.preferenceRepo.GetOrCreate(c.Request.Context(), customerID)
	if err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get notification preferences")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: prefs.ToResponse()})
}

// UpdateNotificationPreferences handles PUT /notifications/preferences.
// Updates the customer's notification preferences.
// @Tags Customer
// @Summary Update notification preferences
// @Description Updates notification preferences for the authenticated customer.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Notification preference update request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/notifications/preferences [put]
func (h *NotificationsHandler) UpdateNotificationPreferences(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req UpdateNotificationPreferencesRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate event types if provided
	if req.Events != nil {
		for _, event := range req.Events {
			if !services.IsValidEventType(event) {
				middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_EVENT", "Invalid event type: "+event)
				return
			}
		}
	}

	// Get existing preferences or create new ones
	prefs, err := h.preferenceRepo.GetOrCreate(c.Request.Context(), customerID)
	if err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get notification preferences")
		return
	}

	// Update fields if provided
	if req.EmailEnabled != nil {
		prefs.EmailEnabled = *req.EmailEnabled
	}
	if req.TelegramEnabled != nil {
		prefs.TelegramEnabled = *req.TelegramEnabled
	}
	if req.Events != nil {
		prefs.Events = req.Events
	}

	// Save changes
	if err := h.preferenceRepo.Update(c.Request.Context(), prefs); err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			if upsertErr := h.preferenceRepo.Upsert(c.Request.Context(), prefs); upsertErr != nil {
				middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update notification preferences")
				return
			}
			h.logAudit(c, "notification.preferences.update", "notification_preference", prefs.ID, map[string]any{
				"email_enabled":    prefs.EmailEnabled,
				"telegram_enabled": prefs.TelegramEnabled,
				"events":           prefs.Events,
			})
			c.JSON(http.StatusOK, models.Response{Data: prefs.ToResponse()})
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update notification preferences")
		return
	}

	h.logAudit(c, "notification.preferences.update", "notification_preference", prefs.ID, map[string]any{
		"email_enabled":    prefs.EmailEnabled,
		"telegram_enabled": prefs.TelegramEnabled,
		"events":           prefs.Events,
	})

	c.JSON(http.StatusOK, models.Response{Data: prefs.ToResponse()})
}

func (h *NotificationsHandler) logAudit(c *gin.Context, action, resourceType, resourceID string, changes map[string]any) {
	if h.auditRepo == nil {
		return
	}

	customerID := middleware.GetUserID(c)
	actorIP := c.ClientIP()
	correlationID := middleware.GetCorrelationID(c)
	maskedChanges := audit.MaskSensitiveFields(changes)
	changesJSON, err := json.Marshal(maskedChanges)
	if err != nil {
		h.logger.Error("failed to marshal audit changes",
			"action", action,
			"resource_id", resourceID,
			"error", err)
		return
	}

	auditLog := &models.AuditLog{
		ActorID:       &customerID,
		ActorType:     models.AuditActorCustomer,
		ActorIP:       &actorIP,
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    &resourceID,
		Changes:       changesJSON,
		CorrelationID: &correlationID,
		Success:       true,
	}

	if err := h.auditRepo.Append(c.Request.Context(), auditLog); err != nil {
		h.logger.Error("failed to write audit log",
			"action", action,
			"resource_id", resourceID,
			"error", err)
	}
}

// ListNotificationEvents handles GET /notifications/events.
// Returns a list of notification events for the customer.
// @Tags Customer
// @Summary List notification events
// @Description Lists notification delivery events for the authenticated customer.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param page query int false "Page"
// @Param per_page query int false "Items per page"
// @Param event_type query string false "Event type"
// @Param status query string false "Delivery status"
// @Success 200 {object} models.ListResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/notifications/events [get]
func (h *NotificationsHandler) ListNotificationEvents(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	pagination := common.ParsePaginationParams(c)

	filter := repository.NotificationEventFilter{
		PaginationParams: pagination,
	}

	// Optional filters
	if eventType := c.Query("event_type"); eventType != "" {
		if !services.IsValidEventType(eventType) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_EVENT_TYPE", "Invalid event_type value")
			return
		}
		filter.EventType = &eventType
	}
	validNotificationStatuses := map[string]bool{
		"pending": true, "sent": true, "failed": true,
	}
	if status := c.Query("status"); status != "" {
		if !validNotificationStatuses[status] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		filter.Status = &status
	}

	events, hasMore, lastID, err := h.eventRepo.ListByCustomer(c.Request.Context(), customerID, filter)
	if err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list notification events")
		return
	}

	// Convert to response format
	responses := make([]models.NotificationEventResponse, len(events))
	for i, event := range events {
		responses[i] = *event.ToResponse()
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: responses,
		Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
	})
}

// GetAvailableEvents handles GET /notifications/events/types.
// Returns a list of available notification event types.
// @Tags Customer
// @Summary List available notification event types
// @Description Returns supported notification event type identifiers.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/notifications/events/types [get]
func (h *NotificationsHandler) GetAvailableEvents(c *gin.Context) {
	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{
			"events": services.AllEventTypes,
		},
	})
}

// registerNotificationReadRoutes registers notification read endpoints that support API keys.
func registerNotificationReadRoutes(router *gin.RouterGroup, handler *NotificationsHandler) {
	notifications := router.Group("/notifications")
	{
		notifications.GET("/preferences", handler.GetNotificationPreferences)
		notifications.GET("/events", handler.ListNotificationEvents)
		notifications.GET("/events/types", handler.GetAvailableEvents)
	}
}

// registerNotificationMutationRoutes registers notification mutation endpoints that require JWT.
func registerNotificationMutationRoutes(router *gin.RouterGroup, handler *NotificationsHandler) {
	notifications := router.Group("/notifications")
	notifications.PUT("/preferences", handler.UpdateNotificationPreferences)
}
