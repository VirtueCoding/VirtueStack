package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// InAppNotificationServiceConfig holds dependencies for InAppNotificationService.
type InAppNotificationServiceConfig struct {
	Repo   *repository.InAppNotificationRepository
	Hub    *SSEHub
	Logger *slog.Logger
}

// InAppNotificationService manages in-app notifications with real-time delivery.
type InAppNotificationService struct {
	repo   *repository.InAppNotificationRepository
	hub    *SSEHub
	logger *slog.Logger
}

// NewInAppNotificationService creates a new InAppNotificationService.
func NewInAppNotificationService(cfg InAppNotificationServiceConfig) *InAppNotificationService {
	return &InAppNotificationService{
		repo:   cfg.Repo,
		hub:    cfg.Hub,
		logger: cfg.Logger.With("component", "in-app-notification-service"),
	}
}

// Notify creates and stores a notification, then broadcasts it via SSE.
func (s *InAppNotificationService) Notify(ctx context.Context, req *models.CreateInAppNotificationRequest) error {
	if err := validateRecipient(req); err != nil {
		return err
	}
	n := &models.InAppNotification{
		CustomerID: req.CustomerID,
		AdminID:    req.AdminID,
		Type:       req.Type,
		Title:      req.Title,
		Message:    req.Message,
		Data:       req.Data,
	}
	if err := s.repo.Create(ctx, n); err != nil {
		return fmt.Errorf("creating notification: %w", err)
	}
	s.broadcastNotification(n)
	return nil
}

func validateRecipient(req *models.CreateInAppNotificationRequest) error {
	hasCustomer := req.CustomerID != nil && *req.CustomerID != ""
	hasAdmin := req.AdminID != nil && *req.AdminID != ""
	if hasCustomer == hasAdmin {
		return fmt.Errorf("exactly one of customer_id or admin_id must be set")
	}
	return nil
}

func (s *InAppNotificationService) broadcastNotification(n *models.InAppNotification) {
	data, err := json.Marshal(n.ToResponse())
	if err != nil {
		s.logger.Warn("failed to marshal notification for SSE", "error", err)
		return
	}
	userID := recipientID(n.CustomerID, n.AdminID)
	s.hub.Broadcast(userID, SSEEvent{Type: "notification", Data: data})
}

func recipientID(customerID, adminID *string) string {
	if customerID != nil && *customerID != "" {
		return *customerID
	}
	if adminID != nil && *adminID != "" {
		return *adminID
	}
	return ""
}

// List returns notifications for a recipient with cursor-based pagination.
func (s *InAppNotificationService) List(
	ctx context.Context, customerID, adminID string, unreadOnly bool, cursor string, perPage int,
) ([]models.InAppNotification, bool, error) {
	if customerID != "" {
		return s.repo.ListByCustomer(ctx, customerID, unreadOnly, cursor, perPage)
	}
	return s.repo.ListByAdmin(ctx, adminID, unreadOnly, cursor, perPage)
}

// MarkAsRead marks a notification as read and broadcasts an unread count update.
func (s *InAppNotificationService) MarkAsRead(ctx context.Context, id, customerID, adminID string) error {
	if err := s.repo.MarkAsRead(ctx, id, customerID, adminID); err != nil {
		return err
	}
	s.broadcastUnreadCount(ctx, customerID, adminID)
	return nil
}

// MarkAllAsRead marks all notifications as read and broadcasts zero count.
func (s *InAppNotificationService) MarkAllAsRead(ctx context.Context, customerID, adminID string) error {
	if err := s.repo.MarkAllAsRead(ctx, customerID, adminID); err != nil {
		return err
	}
	userID := customerID
	if userID == "" {
		userID = adminID
	}
	data, _ := json.Marshal(models.UnreadCountResponse{Count: 0})
	s.hub.Broadcast(userID, SSEEvent{Type: "unread_count_changed", Data: data})
	return nil
}

// GetUnreadCount returns the unread notification count for a recipient.
func (s *InAppNotificationService) GetUnreadCount(ctx context.Context, customerID, adminID string) (int, error) {
	return s.repo.GetUnreadCount(ctx, customerID, adminID)
}

func (s *InAppNotificationService) broadcastUnreadCount(ctx context.Context, customerID, adminID string) {
	count, err := s.repo.GetUnreadCount(ctx, customerID, adminID)
	if err != nil {
		s.logger.Warn("failed to get unread count for broadcast", "error", err)
		return
	}
	userID := customerID
	if userID == "" {
		userID = adminID
	}
	data, _ := json.Marshal(models.UnreadCountResponse{Count: count})
	s.hub.Broadcast(userID, SSEEvent{Type: "unread_count_changed", Data: data})
}

// StartCleanupScheduler periodically removes old read notifications.
func (s *InAppNotificationService) StartCleanupScheduler(ctx context.Context, interval, maxAge time.Duration) {
	s.logger.Info("starting notification cleanup scheduler",
		"interval", interval.String(), "max_age", maxAge.String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("notification cleanup scheduler stopped")
			return
		case <-ticker.C:
			deleted, err := s.repo.DeleteOld(ctx, maxAge)
			if err != nil {
				s.logger.Warn("notification cleanup failed", "error", err)
				continue
			}
			if deleted > 0 {
				s.logger.Info("cleaned up old notifications", "deleted", deleted)
			}
		}
	}
}
