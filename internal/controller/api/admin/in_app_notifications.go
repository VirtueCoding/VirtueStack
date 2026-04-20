package admin

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

// AdminInAppNotificationsHandler handles in-app notification endpoints for admins.
type AdminInAppNotificationsHandler struct {
	service *services.InAppNotificationService
	hub     *services.SSEHub
	authCfg middleware.AuthConfig
	logger  *slog.Logger
}

// NewAdminInAppNotificationsHandler creates a new AdminInAppNotificationsHandler.
func NewAdminInAppNotificationsHandler(
	service *services.InAppNotificationService,
	hub *services.SSEHub,
	authCfg middleware.AuthConfig,
	logger *slog.Logger,
) *AdminInAppNotificationsHandler {
	return &AdminInAppNotificationsHandler{
		service: service,
		hub:     hub,
		authCfg: authCfg,
		logger:  logger.With("component", "admin-in-app-notifications-handler"),
	}
}

// ListNotifications handles GET /notifications.
func (h *AdminInAppNotificationsHandler) ListNotifications(c *gin.Context) {
	adminID := middleware.GetUserID(c)
	cursor := c.Query("cursor")
	unreadOnly := c.Query("unread") == "true"
	perPage := adminParsePerPage(c.Query("per_page"), 20, 100)

	results, hasMore, err := h.service.List(c.Request.Context(), "", adminID, unreadOnly, cursor, perPage)
	if err != nil {
		h.logger.Error("failed to list notifications", "error", err, "admin_id", adminID)
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
func (h *AdminInAppNotificationsHandler) MarkAsRead(c *gin.Context) {
	adminID := middleware.GetUserID(c)
	notifID := c.Param("id")
	if err := h.service.MarkAsRead(c.Request.Context(), notifID, "", adminID); err != nil {
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
func (h *AdminInAppNotificationsHandler) MarkAllAsRead(c *gin.Context) {
	adminID := middleware.GetUserID(c)
	if err := h.service.MarkAllAsRead(c.Request.Context(), "", adminID); err != nil {
		h.logger.Error("failed to mark all notifications as read", "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to mark all as read")
		return
	}
	c.Status(http.StatusNoContent)
}

// GetUnreadCount handles GET /notifications/unread-count.
func (h *AdminInAppNotificationsHandler) GetUnreadCount(c *gin.Context) {
	adminID := middleware.GetUserID(c)
	count, err := h.service.GetUnreadCount(c.Request.Context(), "", adminID)
	if err != nil {
		h.logger.Error("failed to get unread count", "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get unread count")
		return
	}
	c.JSON(http.StatusOK, models.Response{Data: models.UnreadCountResponse{Count: count}})
}

const adminMaxSSEConnectionsPerUser = 2

// StreamNotifications handles GET /notifications/stream (SSE).
func (h *AdminInAppNotificationsHandler) StreamNotifications(c *gin.Context) {
	adminID := middleware.GetUserID(c)
	if h.hub.ConnectionCount(adminID) >= adminMaxSSEConnectionsPerUser {
		middleware.RespondWithError(c, http.StatusTooManyRequests, "TOO_MANY_CONNECTIONS",
			fmt.Sprintf("Maximum %d SSE connections per user", adminMaxSSEConnectionsPerUser))
		return
	}
	h.streamSSE(c, adminID)
}

func (h *AdminInAppNotificationsHandler) streamSSE(c *gin.Context, userID string) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan services.SSEEvent, 16)
	h.hub.Register(userID, ch)
	defer h.hub.Unregister(userID, ch)

	count, err := h.service.GetUnreadCount(c.Request.Context(), "", userID)
	if err == nil {
		data, _ := json.Marshal(models.UnreadCountResponse{Count: count})
		adminWriteSSEEvent(c, "unread_count", data)
	}

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			adminWriteSSEEvent(c, event.Type, event.Data)
		case <-heartbeat.C:
			adminWriteSSEEvent(c, "heartbeat", json.RawMessage(`{}`))
		}
	}
}

func adminWriteSSEEvent(c *gin.Context, eventType string, data json.RawMessage) {
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, string(data))
	c.Writer.Flush()
}

func adminParsePerPage(raw string, defaultVal, maxVal int) int {
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
