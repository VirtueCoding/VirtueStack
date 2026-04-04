package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// SystemEventService publishes platform events and queues system webhook deliveries.
type SystemEventService struct {
	systemWebhookRepo *repository.SystemWebhookRepository
	taskPublisher     TaskPublisher
	logger            *slog.Logger
}

func NewSystemEventService(
	systemWebhookRepo *repository.SystemWebhookRepository,
	taskPublisher TaskPublisher,
	logger *slog.Logger,
) *SystemEventService {
	return &SystemEventService{
		systemWebhookRepo: systemWebhookRepo,
		taskPublisher:     taskPublisher,
		logger:            logger.With("component", "system-event-service"),
	}
}

// PublishSystemEvent publishes a system event and queues system webhook deliveries.
func (s *SystemEventService) PublishSystemEvent(ctx context.Context, eventType string, payload map[string]any) error {
	webhooks, err := s.systemWebhookRepo.ListActiveForEvent(ctx, eventType)
	if err != nil {
		return fmt.Errorf("listing system webhooks for event %s: %w", eventType, err)
	}
	if len(webhooks) == 0 {
		return nil
	}

	for _, wh := range webhooks {
		if s.taskPublisher != nil {
			if _, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeSystemWebhookDeliver, map[string]any{
				"system_webhook_id": wh.ID,
				"event":             eventType,
				"payload":           payload,
			}); err != nil {
				s.logger.Warn("failed to publish system webhook delivery task", "system_webhook_id", wh.ID, "error", err)
			}
		}
	}
	return nil
}
