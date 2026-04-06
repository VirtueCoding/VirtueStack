package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// SystemEventService publishes platform events and queues system webhook deliveries.
type SystemEventService struct {
	systemWebhookRepo  *repository.SystemWebhookRepository
	systemDeliveryRepo *repository.SystemWebhookDeliveryRepository
	taskPublisher      TaskPublisher
	logger             *slog.Logger
}

func NewSystemEventService(
	systemWebhookRepo *repository.SystemWebhookRepository,
	systemDeliveryRepo *repository.SystemWebhookDeliveryRepository,
	taskPublisher TaskPublisher,
	logger *slog.Logger,
) *SystemEventService {
	return &SystemEventService{
		systemWebhookRepo:  systemWebhookRepo,
		systemDeliveryRepo: systemDeliveryRepo,
		taskPublisher:      taskPublisher,
		logger:             logger.With("component", "system-event-service"),
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
		if s.systemDeliveryRepo == nil {
			return fmt.Errorf("system webhook delivery repository is not configured")
		}

		bodyBytes, err := models.MarshalSystemWebhookRequestBody(eventType, payload)
		if err != nil {
			return fmt.Errorf("marshaling system webhook request body: %w", err)
		}

		delivery := &models.SystemWebhookDelivery{
			SystemWebhookID: wh.ID,
			Event:           eventType,
			IdempotencyKey:  uuid.NewString(),
			Payload:         bodyBytes,
			Status:          repository.DeliveryStatusPending,
			MaxAttempts:     5,
		}
		if err := s.systemDeliveryRepo.CreateDelivery(ctx, delivery); err != nil {
			s.logger.Warn("failed to create system webhook delivery record", "system_webhook_id", wh.ID, "error", err)
			continue
		}

		if s.taskPublisher != nil {
			if _, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeSystemWebhookDeliver, map[string]any{
				"delivery_id": delivery.ID,
			}); err != nil {
				s.logger.Warn("failed to publish system webhook delivery task", "delivery_id", delivery.ID, "system_webhook_id", wh.ID, "error", err)
			}
		}
	}
	return nil
}
