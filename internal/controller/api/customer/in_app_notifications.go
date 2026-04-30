package customer

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// InAppNotificationsHandler handles in-app notification endpoints for customers.
type InAppNotificationsHandler struct {
	service *services.InAppNotificationService
	hub     *services.SSEHub
	authCfg middleware.AuthConfig
	logger  *slog.Logger
}

// NewInAppNotificationsHandler creates a new InAppNotificationsHandler.
func NewInAppNotificationsHandler(
	service *services.InAppNotificationService,
	hub *services.SSEHub,
	authCfg middleware.AuthConfig,
	logger *slog.Logger,
) *InAppNotificationsHandler {
	return &InAppNotificationsHandler{
		service: service,
		hub:     hub,
		authCfg: authCfg,
		logger:  logger.With("component", "customer-in-app-notifications-handler"),
	}
}

// ListNotifications handles GET /notifications.
func (h *InAppNotificationsHandler) ListNotifications(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	cursor := c.Query("cursor")
	unreadOnly := c.Query("unread") == "true"
	perPage := parsePerPage(c.Query("per_page"), 20, 100)

	results, hasMore, err := h.service.List(c.Request.Context(), customerID, "", unreadOnly, cursor, perPage)
	if err != nil {
		h.logger.Error("failed to list notifications", "error", err, "customer_id", customerID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list notifications")
		return
	}
	responses := make([]*models.InAppNotificationResponse, len(results))
	for i := range results {
		responses[i] = results[i].ToResponse()
	}
	lastCursor := ""
	if len(results) > 0 {
		lastCursor = results[len(results)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	c.JSON(http.StatusOK, models.ListResponse{
		Data: responses,
		Meta: models.NewCursorPaginationMeta(perPage, hasMore, lastCursor),
	})
}

// MarkAsRead handles POST /notifications/:id/read.
func (h *InAppNotificationsHandler) MarkAsRead(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	notifID := c.Param("id")
	if err := h.service.MarkAsRead(c.Request.Context(), notifID, customerID, ""); err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "Notification not found")
			return
		}
		h.logger.Error("failed to mark notification as read", "error", err, "id", notifID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to mark notification as read")
		return
	}
	c.Status(http.StatusNoContent)
}

// MarkAllAsRead handles POST /notifications/read-all.
func (h *InAppNotificationsHandler) MarkAllAsRead(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	if err := h.service.MarkAllAsRead(c.Request.Context(), customerID, ""); err != nil {
		h.logger.Error("failed to mark all notifications as read", "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to mark all as read")
		return
	}
	c.Status(http.StatusNoContent)
}

// GetUnreadCount handles GET /notifications/unread-count.
func (h *InAppNotificationsHandler) GetUnreadCount(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	count, err := h.service.GetUnreadCount(c.Request.Context(), customerID, "")
	if err != nil {
		h.logger.Error("failed to get unread count", "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get unread count")
		return
	}
	c.JSON(http.StatusOK, models.Response{Data: models.UnreadCountResponse{Count: count}})
}

const maxSSEConnectionsPerUser = 2

// StreamNotifications handles GET /notifications/stream (SSE).
func (h *InAppNotificationsHandler) StreamNotifications(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	if h.hub.ConnectionCount(customerID) >= maxSSEConnectionsPerUser {
		middleware.RespondWithError(c, http.StatusTooManyRequests, "TOO_MANY_CONNECTIONS",
			fmt.Sprintf("Maximum %d SSE connections per user", maxSSEConnectionsPerUser))
		return
	}
	h.streamSSE(c, customerID)
}

func (h *InAppNotificationsHandler) streamSSE(c *gin.Context, userID string) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan services.SSEEvent, 16)
	h.hub.Register(userID, ch)
	defer h.hub.Unregister(userID, ch)

	// Send initial unread count
	count, err := h.service.GetUnreadCount(c.Request.Context(), userID, "")
	if err == nil {
		data, _ := json.Marshal(models.UnreadCountResponse{Count: count})
		writeSSEEvent(c, "unread_count", data)
	}

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			writeSSEEvent(c, event.Type, event.Data)
		case <-heartbeat.C:
			writeSSEEvent(c, "heartbeat", json.RawMessage(`{}`))
		}
	}
}

func writeSSEEvent(c *gin.Context, eventType string, data json.RawMessage) {
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, string(data))
	c.Writer.Flush()
}

func parsePerPage(raw string, defaultVal, maxVal int) int {
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return defaultVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}
