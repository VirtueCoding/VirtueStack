package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// PreActionWebhookService evaluates pre-action webhooks before protected operations.
type PreActionWebhookService struct {
	repo   *repository.PreActionWebhookRepository
	client *http.Client
	logger *slog.Logger
}

// NewPreActionWebhookService creates a new PreActionWebhookService.
func NewPreActionWebhookService(
	repo *repository.PreActionWebhookRepository,
	logger *slog.Logger,
) *PreActionWebhookService {
	return &PreActionWebhookService{
		repo:   repo,
		client: &http.Client{},
		logger: logger.With("component", "pre-action-webhook-service"),
	}
}

// PreActionPayload is the payload sent to pre-action webhooks for approval.
type PreActionPayload struct {
	Event      string         `json:"event"`
	CustomerID string         `json:"customer_id"`
	Data       map[string]any `json:"data"`
	Timestamp  time.Time      `json:"timestamp"`
}

// preActionResponse is the expected response from a pre-action webhook.
type preActionResponse struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

// CheckPreAction evaluates all active pre-action webhooks for the given event.
// Returns nil if approved (or no webhooks configured), error if rejected.
func (s *PreActionWebhookService) CheckPreAction(ctx context.Context, event string, customerID string, data map[string]any) error {
	webhooks, err := s.repo.ListActiveForEvent(ctx, event)
	if err != nil {
		s.logger.Error("failed to list pre-action webhooks", "event", event, "error", err)
		return nil
	}

	if len(webhooks) == 0 {
		return nil
	}

	payload := PreActionPayload{
		Event:      event,
		CustomerID: customerID,
		Data:       data,
		Timestamp:  time.Now().UTC(),
	}

	for _, wh := range webhooks {
		if err := s.callWebhook(ctx, &wh, &payload); err != nil {
			return err
		}
	}

	return nil
}

func (s *PreActionWebhookService) callWebhook(ctx context.Context, wh *models.PreActionWebhook, payload *PreActionPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("failed to marshal pre-action payload", "webhook_id", wh.ID, "error", err)
		if wh.FailOpen {
			return nil
		}
		return fmt.Errorf("pre-action webhook %s: marshal error", wh.Name)
	}

	timeout := time.Duration(wh.TimeoutMs) * time.Millisecond
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		s.logger.Error("failed to create pre-action request", "webhook_id", wh.ID, "error", err)
		if wh.FailOpen {
			return nil
		}
		return fmt.Errorf("pre-action webhook %s: request creation error", wh.Name)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Secret", wh.Secret)

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Warn("pre-action webhook call failed",
			"webhook_id", wh.ID, "webhook_name", wh.Name, "error", err, "fail_open", wh.FailOpen)
		if wh.FailOpen {
			return nil
		}
		return fmt.Errorf("pre-action webhook %q is unreachable and configured as fail-closed: %w", wh.Name, sharederrors.ErrForbidden)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		s.logger.Warn("failed to read pre-action response", "webhook_id", wh.ID, "error", err)
		if wh.FailOpen {
			return nil
		}
		return fmt.Errorf("pre-action webhook %q returned unreadable response: %w", wh.Name, sharederrors.ErrForbidden)
	}

	if resp.StatusCode >= 500 {
		s.logger.Warn("pre-action webhook returned server error",
			"webhook_id", wh.ID, "status", resp.StatusCode, "fail_open", wh.FailOpen)
		if wh.FailOpen {
			return nil
		}
		return fmt.Errorf("pre-action webhook %q returned server error: %w", wh.Name, sharederrors.ErrForbidden)
	}

	var result preActionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		s.logger.Warn("failed to parse pre-action response",
			"webhook_id", wh.ID, "body", string(respBody), "error", err)
		if wh.FailOpen {
			return nil
		}
		return fmt.Errorf("pre-action webhook %q returned invalid response: %w", wh.Name, sharederrors.ErrForbidden)
	}

	if !result.Approved {
		reason := result.Reason
		if reason == "" {
			reason = "rejected by policy"
		}
		s.logger.Info("pre-action webhook rejected request",
			"webhook_id", wh.ID, "webhook_name", wh.Name, "reason", reason)
		return fmt.Errorf("action rejected by pre-action webhook %q: %s: %w", wh.Name, reason, sharederrors.ErrForbidden)
	}

	return nil
}
