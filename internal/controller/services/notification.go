// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/notifications"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// Notification event types.
const (
	EventVMCreated         = "vm.created"
	EventVMDeleted         = "vm.deleted"
	EventVMSuspended       = "vm.suspended"
	EventBackupFailed      = "backup.failed"
	EventNodeOffline       = "node.offline"
	EventBandwidthExceeded = "bandwidth.exceeded"
)

// AllEventTypes contains all valid notification event types.
var AllEventTypes = []string{
	EventVMCreated,
	EventVMDeleted,
	EventVMSuspended,
	EventBackupFailed,
	EventNodeOffline,
	EventBandwidthExceeded,
}

// IsValidEventType checks if the given event type is valid.
func IsValidEventType(eventType string) bool {
	for _, t := range AllEventTypes {
		if t == eventType {
			return true
		}
	}
	return false
}

// NotificationConfig holds configuration for the notification service.
type NotificationConfig struct {
	EmailEnabled    bool
	TelegramEnabled bool
}

// NotificationPayload contains data for a notification.
type NotificationPayload struct {
	EventType     string         `json:"event_type"`
	Timestamp     time.Time      `json:"timestamp"`
	CustomerID    string         `json:"customer_id,omitempty"`
	CustomerEmail string         `json:"customer_email,omitempty"`
	CustomerName  string         `json:"customer_name,omitempty"`
	ResourceID    string         `json:"resource_id,omitempty"`
	ResourceType  string         `json:"resource_type,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
}

// NotificationService provides notification functionality.
// It manages multiple notification providers (email, Telegram) and routes
// notifications based on event type and customer preferences.
type NotificationService struct {
	emailProvider    *notifications.EmailProvider
	telegramProvider *notifications.TelegramProvider
	preferenceRepo   *repository.NotificationPreferenceRepository
	customerRepo     *repository.CustomerRepository
	config           NotificationConfig
	logger           *slog.Logger
}

// NewNotificationService creates a new NotificationService with the given dependencies.
func NewNotificationService(
	emailProvider *notifications.EmailProvider,
	telegramProvider *notifications.TelegramProvider,
	preferenceRepo *repository.NotificationPreferenceRepository,
	customerRepo *repository.CustomerRepository,
	config NotificationConfig,
	logger *slog.Logger,
) *NotificationService {
	return &NotificationService{
		emailProvider:    emailProvider,
		telegramProvider: telegramProvider,
		preferenceRepo:   preferenceRepo,
		customerRepo:     customerRepo,
		config:           config,
		logger:           logger.With("component", "notification-service"),
	}
}

// SendNotification sends a notification through enabled providers.
// It checks customer preferences and routes the notification accordingly.
// This method blocks until all notification providers complete.
func (s *NotificationService) SendNotification(ctx context.Context, payload *NotificationPayload) {
	// Process notification with internal concurrency management
	s.sendNotificationAsync(ctx, payload)
}

// sendNotificationAsync handles the actual notification sending.
func (s *NotificationService) sendNotificationAsync(ctx context.Context, payload *NotificationPayload) {
	// Create a timeout context for the notification
	notifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.logger.Info("sending notification",
		"event_type", payload.EventType,
		"customer_id", payload.CustomerID,
		"resource_id", payload.ResourceID)

	// Get customer preferences if customer ID is provided
	var preferences *models.NotificationPreferences
	if payload.CustomerID != "" {
		var err error
		preferences, err = s.preferenceRepo.GetByCustomerID(notifyCtx, payload.CustomerID)
		if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
			s.logger.Error("failed to get notification preferences",
				"customer_id", payload.CustomerID,
				"error", err)
		}
	}

	// Send notifications in parallel
	var wg sync.WaitGroup

	// Send email notification
	if s.config.EmailEnabled && s.emailProvider != nil {
		if s.shouldSendEmail(preferences, payload.EventType) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := s.sendEmailNotification(notifyCtx, payload); err != nil {
					s.logger.Error("failed to send email notification",
						"event_type", payload.EventType,
						"customer_id", payload.CustomerID,
						"error", err)
				}
			}()
		}
	}

	// Send Telegram notification for admin-level events
	if s.config.TelegramEnabled && s.telegramProvider != nil {
		if s.isAdminEvent(payload.EventType) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := s.sendTelegramNotification(notifyCtx, payload); err != nil {
					s.logger.Error("failed to send telegram notification",
						"event_type", payload.EventType,
						"error", err)
				}
			}()
		}
	}

	wg.Wait()
}

// shouldSendEmail determines if an email should be sent based on preferences.
func (s *NotificationService) shouldSendEmail(preferences *models.NotificationPreferences, eventType string) bool {
	if preferences == nil {
		// Default: send email for all events if no preferences set
		return true
	}

	if !preferences.EmailEnabled {
		return false
	}

	// Check if this event type is enabled in preferences
	for _, event := range preferences.Events {
		if event == eventType {
			return true
		}
	}

	return false
}

// isAdminEvent returns true if the event should trigger admin notifications.
func (s *NotificationService) isAdminEvent(eventType string) bool {
	adminEvents := []string{
		EventNodeOffline,
		EventBackupFailed,
		EventBandwidthExceeded,
	}
	for _, e := range adminEvents {
		if e == eventType {
			return true
		}
	}
	return false
}

// sendEmailNotification sends an email notification.
func (s *NotificationService) sendEmailNotification(ctx context.Context, payload *NotificationPayload) error {
	if payload.CustomerEmail == "" {
		// Try to get customer email
		if payload.CustomerID != "" {
			customer, err := s.customerRepo.GetByID(ctx, payload.CustomerID)
			if err != nil {
				return fmt.Errorf("getting customer email: %w", err)
			}
			payload.CustomerEmail = customer.Email
			payload.CustomerName = customer.Name
		}
		if payload.CustomerEmail == "" {
			return fmt.Errorf("no customer email available for notification")
		}
	}

	subject := s.getSubjectForEvent(payload.EventType)
	templateName := s.getTemplateForEvent(payload.EventType)

	return s.emailProvider.Send(ctx, &notifications.EmailPayload{
		To:           payload.CustomerEmail,
		Subject:      subject,
		Template:     templateName,
		CustomerName: payload.CustomerName,
		Data:         payload.Data,
	})
}

// sendTelegramNotification sends a Telegram notification to admin chats.
func (s *NotificationService) sendTelegramNotification(ctx context.Context, payload *NotificationPayload) error {
	message := s.formatTelegramMessage(payload)
	return s.telegramProvider.Send(ctx, &notifications.TelegramPayload{
		Message: message,
	})
}

// getSubjectForEvent returns the email subject for an event type.
func (s *NotificationService) getSubjectForEvent(eventType string) string {
	subjects := map[string]string{
		EventVMCreated:         "Your VM has been created",
		EventVMDeleted:         "Your VM has been deleted",
		EventVMSuspended:       "Your VM has been suspended",
		EventBackupFailed:      "Backup failed for your VM",
		EventNodeOffline:       "Alert: Node offline detected",
		EventBandwidthExceeded: "Bandwidth limit exceeded",
	}
	if subject, ok := subjects[eventType]; ok {
		return subject
	}
	return "VirtueStack Notification"
}

// getTemplateForEvent returns the email template name for an event type.
func (s *NotificationService) getTemplateForEvent(eventType string) string {
	templates := map[string]string{
		EventVMCreated:         "vm-created",
		EventVMDeleted:         "vm-deleted",
		EventVMSuspended:       "vm-suspended",
		EventBackupFailed:      "backup-failed",
		EventNodeOffline:       "node-offline",
		EventBandwidthExceeded: "bandwidth-exceeded",
	}
	if template, ok := templates[eventType]; ok {
		return template
	}
	return "default"
}

// formatTelegramMessage formats a notification for Telegram.
func (s *NotificationService) formatTelegramMessage(payload *NotificationPayload) string {
	var message string

	// Add emoji based on event type
	emoji := s.getEmojiForEvent(payload.EventType)

	message = fmt.Sprintf("%s *%s*\n\n", emoji, s.getSubjectForEvent(payload.EventType))

	if payload.CustomerName != "" {
		message += fmt.Sprintf("Customer: %s\n", payload.CustomerName)
	}
	if payload.ResourceType != "" && payload.ResourceID != "" {
		message += fmt.Sprintf("Resource: %s (%s)\n", payload.ResourceType, payload.ResourceID)
	}

	// Add additional data if present
	if payload.Data != nil {
		if vmName, ok := payload.Data["hostname"].(string); ok {
			message += fmt.Sprintf("VM: %s\n", vmName)
		}
		if nodeName, ok := payload.Data["node_name"].(string); ok {
			message += fmt.Sprintf("Node: %s\n", nodeName)
		}
		if reason, ok := payload.Data["reason"].(string); ok {
			message += fmt.Sprintf("Reason: %s\n", reason)
		}
	}

	message += fmt.Sprintf("\n_Time: %s_", payload.Timestamp.Format(time.RFC3339))

	return message
}

// getEmojiForEvent returns an emoji for the event type.
func (s *NotificationService) getEmojiForEvent(eventType string) string {
	emojis := map[string]string{
		EventVMCreated:         "🎉",
		EventVMDeleted:         "🗑️",
		EventVMSuspended:       "⏸️",
		EventBackupFailed:      "⚠️",
		EventNodeOffline:       "🔴",
		EventBandwidthExceeded: "📊",
	}
	if emoji, ok := emojis[eventType]; ok {
		return emoji
	}
	return "📢"
}

// TriggerVMCreated triggers a VM created notification.
func (s *NotificationService) TriggerVMCreated(ctx context.Context, customerID, vmID, hostname string) {
	s.SendNotification(ctx, &NotificationPayload{
		EventType:    EventVMCreated,
		Timestamp:    time.Now(),
		CustomerID:   customerID,
		ResourceID:   vmID,
		ResourceType: "vm",
		Data: map[string]any{
			"hostname": hostname,
		},
	})
}

// TriggerVMDeleted triggers a VM deleted notification.
func (s *NotificationService) TriggerVMDeleted(ctx context.Context, customerID, vmID, hostname string) {
	s.SendNotification(ctx, &NotificationPayload{
		EventType:    EventVMDeleted,
		Timestamp:    time.Now(),
		CustomerID:   customerID,
		ResourceID:   vmID,
		ResourceType: "vm",
		Data: map[string]any{
			"hostname": hostname,
		},
	})
}

// TriggerVMSuspended triggers a VM suspended notification.
func (s *NotificationService) TriggerVMSuspended(ctx context.Context, customerID, vmID, hostname, reason string) {
	s.SendNotification(ctx, &NotificationPayload{
		EventType:    EventVMSuspended,
		Timestamp:    time.Now(),
		CustomerID:   customerID,
		ResourceID:   vmID,
		ResourceType: "vm",
		Data: map[string]any{
			"hostname": hostname,
			"reason":   reason,
		},
	})
}

// TriggerBackupFailed triggers a backup failed notification.
func (s *NotificationService) TriggerBackupFailed(ctx context.Context, customerID, vmID, hostname, backupID, errorMsg string) {
	s.SendNotification(ctx, &NotificationPayload{
		EventType:    EventBackupFailed,
		Timestamp:    time.Now(),
		CustomerID:   customerID,
		ResourceID:   backupID,
		ResourceType: "backup",
		Data: map[string]any{
			"hostname": hostname,
			"vm_id":    vmID,
			"error":    errorMsg,
		},
	})
}

// TriggerNodeOffline triggers a node offline notification (admin only).
func (s *NotificationService) TriggerNodeOffline(ctx context.Context, nodeID, nodeName string) {
	s.SendNotification(ctx, &NotificationPayload{
		EventType:    EventNodeOffline,
		Timestamp:    time.Now(),
		ResourceID:   nodeID,
		ResourceType: "node",
		Data: map[string]any{
			"node_name": nodeName,
		},
	})
}

// TriggerBandwidthExceeded triggers a bandwidth exceeded notification.
func (s *NotificationService) TriggerBandwidthExceeded(ctx context.Context, customerID, vmID, hostname string, usedGB, limitGB int64) {
	s.SendNotification(ctx, &NotificationPayload{
		EventType:    EventBandwidthExceeded,
		Timestamp:    time.Now(),
		CustomerID:   customerID,
		ResourceID:   vmID,
		ResourceType: "vm",
		Data: map[string]any{
			"hostname": hostname,
			"used_gb":  usedGB,
			"limit_gb": limitGB,
		},
	})
}

// MarshalJSON implements json.Marshaler for NotificationPayload.
func (p *NotificationPayload) MarshalJSON() ([]byte, error) {
	type Alias NotificationPayload
	return json.Marshal((*Alias)(p))
}
