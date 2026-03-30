package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
)

// EventBus publishes platform events to NATS JetStream subjects.
type EventBus struct {
	js     nats.JetStreamContext
	logger *slog.Logger
}

func NewEventBus(js nats.JetStreamContext, logger *slog.Logger) *EventBus {
	return &EventBus{
		js:     js,
		logger: logger.With("component", "event-bus"),
	}
}

func (eb *EventBus) Publish(ctx context.Context, subject string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling event payload: %w", err)
	}
	_, err = eb.js.Publish(subject, payload, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("publishing event to %s: %w", subject, err)
	}
	eb.logger.Debug("published event", "subject", subject)
	return nil
}
