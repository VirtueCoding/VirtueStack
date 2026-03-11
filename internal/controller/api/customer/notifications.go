package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/gin-gonic/gin"
)

// NotificationsHandler handles notification-related API endpoints.
type NotificationsHandler struct {
	preferenceRepo *repository.NotificationPreferenceRepository
	eventRepo      *repository.NotificationEventRepository
	notifyService  *services.NotificationService
}

// NewNotificationsHandler creates a new NotificationsHandler.
func NewNotificationsHandler(
	preferenceRepo *repository.NotificationPreferenceRepository,
	eventRepo *repository.NotificationEventRepository,
	notifyService *services.NotificationService,
) *NotificationsHandler {
	return &NotificationsHandler{
		preferenceRepo: preferenceRepo,
		eventRepo:      eventRepo,
		notifyService:  notifyService,
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
func (h *NotificationsHandler) GetNotificationPreferences(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	prefs, err := h.preferenceRepo.GetOrCreate(c.Request.Context(), customerID)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get notification preferences")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: prefs.ToResponse()})
}

// UpdateNotificationPreferences handles PUT /notifications/preferences.
// Updates the customer's notification preferences.
func (h *NotificationsHandler) UpdateNotificationPreferences(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req UpdateNotificationPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Validate event types if provided
	if req.Events != nil {
		for _, event := range req.Events {
			if !services.IsValidEventType(event) {
				respondWithError(c, http.StatusBadRequest, "INVALID_EVENT", "Invalid event type: "+event)
				return
			}
		}
	}

	// Get existing preferences or create new ones
	prefs, err := h.preferenceRepo.GetOrCreate(c.Request.Context(), customerID)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get notification preferences")
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
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update notification preferences")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: prefs.ToResponse()})
}

// ListNotificationEvents handles GET /notifications/events.
// Returns a list of notification events for the customer.
func (h *NotificationsHandler) ListNotificationEvents(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	pagination := models.ParsePagination(c)

	filter := repository.NotificationEventFilter{
		PaginationParams: pagination,
	}

	// Optional filters
	if eventType := c.Query("event_type"); eventType != "" {
		filter.EventType = &eventType
	}
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	events, total, err := h.eventRepo.ListByCustomer(c.Request.Context(), customerID, filter)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list notification events")
		return
	}

	// Convert to response format
	responses := make([]models.NotificationEventResponse, len(events))
	for i, event := range events {
		responses[i] = *event.ToResponse()
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: responses,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// GetAvailableEvents handles GET /notifications/events/types.
// Returns a list of available notification event types.
func (h *NotificationsHandler) GetAvailableEvents(c *gin.Context) {
	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{
			"events": services.AllEventTypes,
		},
	})
}

// RegisterNotificationRoutes registers notification-related routes.
func RegisterNotificationRoutes(router *gin.RouterGroup, handler *NotificationsHandler) {
	notifications := router.Group("/notifications")
	{
		// Preferences
		notifications.GET("/preferences", handler.GetNotificationPreferences)
		notifications.PUT("/preferences", handler.UpdateNotificationPreferences)

		// Events
		notifications.GET("/events", handler.ListNotificationEvents)
		notifications.GET("/events/types", handler.GetAvailableEvents)
	}
}