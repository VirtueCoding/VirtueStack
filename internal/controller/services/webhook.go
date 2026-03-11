// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
)

// WebhookService provides business logic for managing webhook endpoints and deliveries.
type WebhookService struct {
	webhookRepo    *repository.WebhookRepository
	taskPublisher  TaskPublisher
	logger         *slog.Logger
	encryptionKey  string
}

// NewWebhookService creates a new WebhookService with the given dependencies.
func NewWebhookService(
	webhookRepo *repository.WebhookRepository,
	taskPublisher TaskPublisher,
	logger *slog.Logger,
	encryptionKey string,
) *WebhookService {
	return &WebhookService{
		webhookRepo:   webhookRepo,
		taskPublisher: taskPublisher,
		logger:        logger.With("component", "webhook-service"),
		encryptionKey: encryptionKey,
	}
}

// Valid webhook events that can be subscribed to.
var ValidWebhookEvents = map[string]bool{
	"vm.created":       true,
	"vm.deleted":       true,
	"vm.started":       true,
	"vm.stopped":       true,
	"vm.reinstalled":   true,
	"vm.migrated":      true,
	"backup.completed": true,
	"backup.failed":    true,
}

// MaxWebhooksPerCustomer is the maximum number of webhooks a customer can have.
const MaxWebhooksPerCustomer = 5

// Errors returned by the webhook service.
var (
	ErrInvalidURL        = errors.New("webhook URL must be HTTPS")
	ErrInvalidEvent      = errors.New("invalid webhook event")
	ErrTooManyWebhooks   = errors.New("maximum webhook limit reached")
	ErrWebhookNotFound   = errors.New("webhook not found")
	ErrSecretTooShort    = errors.New("secret must be at least 16 characters")
	ErrSecretTooLong     = errors.New("secret must be at most 128 characters")
)

// CreateWebhookRequest contains parameters for creating a webhook.
type CreateWebhookRequest struct {
	CustomerID string
	URL        string
	Secret     string
	Events     []string
}

// UpdateWebhookRequest contains parameters for updating a webhook.
type UpdateWebhookRequest struct {
	URL    *string
	Secret *string
	Events []string
	Active *bool
}

// WebhookResponse represents a webhook in API responses.
type WebhookResponse struct {
	ID            string     `json:"id"`
	URL           string     `json:"url"`
	Events        []string   `json:"events"`
	Active        bool       `json:"active"`
	FailCount     int        `json:"fail_count"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// WebhookPayload represents the structure of a webhook delivery payload.
type WebhookPayload struct {
	Event          string          `json:"event"`
	Timestamp      string          `json:"timestamp"`
	IdempotencyKey string          `json:"idempotency_key"`
	Data           json.RawMessage `json:"data"`
}

// ============================================================================
// Webhook CRUD Operations
// ============================================================================

// Create creates a new webhook endpoint for a customer.
func (s *WebhookService) Create(ctx context.Context, req CreateWebhookRequest) (*repository.Webhook, error) {
	// Validate URL
	if err := s.validateWebhookURL(req.URL); err != nil {
		return nil, err
	}

	// Validate secret
	if len(req.Secret) < 16 {
		return nil, ErrSecretTooShort
	}
	if len(req.Secret) > 128 {
		return nil, ErrSecretTooLong
	}

	// Validate events
	for _, event := range req.Events {
		if !ValidWebhookEvents[event] {
			return nil, fmt.Errorf("%w: %s", ErrInvalidEvent, event)
		}
	}

	// Check webhook limit
	count, err := s.webhookRepo.CountByCustomer(ctx, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("checking webhook count: %w", err)
	}
	if count >= MaxWebhooksPerCustomer {
		return nil, ErrTooManyWebhooks
	}

	// Encrypt the secret for storage
	encryptedSecret, err := s.encryptSecret(req.Secret)
	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	// Create webhook
	webhook := &repository.Webhook{
		CustomerID: req.CustomerID,
		URL:        req.URL,
		SecretHash: encryptedSecret, // Store encrypted secret (field name kept for compatibility)
		Events:     req.Events,
		Active:     true,
	}

	if err := s.webhookRepo.Create(ctx, webhook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	s.logger.Info("webhook created",
		"webhook_id", webhook.ID,
		"customer_id", req.CustomerID,
		"url", req.URL,
		"events", req.Events)

	return webhook, nil
}

// Get retrieves a webhook by ID for a specific customer.
func (s *WebhookService) Get(ctx context.Context, id, customerID string) (*repository.Webhook, error) {
	webhook, err := s.webhookRepo.GetByIDAndCustomer(ctx, id, customerID)
	if err != nil {
		return nil, ErrWebhookNotFound
	}
	return webhook, nil
}

// List retrieves all webhooks for a customer.
func (s *WebhookService) List(ctx context.Context, customerID string) ([]repository.Webhook, error) {
	return s.webhookRepo.ListByCustomer(ctx, customerID)
}

// Update updates a webhook endpoint.
func (s *WebhookService) Update(ctx context.Context, id, customerID string, req UpdateWebhookRequest) (*repository.Webhook, error) {
	// Verify webhook exists and belongs to customer
	_, err := s.webhookRepo.GetByIDAndCustomer(ctx, id, customerID)
	if err != nil {
		return nil, ErrWebhookNotFound
	}

	// Validate URL if provided
	if req.URL != nil {
		if err := s.validateWebhookURL(*req.URL); err != nil {
			return nil, err
		}
	}

	// Validate events if provided
	if req.Events != nil {
		for _, event := range req.Events {
			if !ValidWebhookEvents[event] {
				return nil, fmt.Errorf("%w: %s", ErrInvalidEvent, event)
			}
		}
	}

	// Update webhook
	if err := s.webhookRepo.Update(ctx, id, req.URL, req.Events, req.Active); err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}

	// Update secret if provided
	if req.Secret != nil {
		if len(*req.Secret) < 16 {
			return nil, ErrSecretTooShort
		}
		if len(*req.Secret) > 128 {
			return nil, ErrSecretTooLong
		}
		encryptedSecret, err := s.encryptSecret(*req.Secret)
		if err != nil {
			return nil, fmt.Errorf("encrypting secret: %w", err)
		}
		if err := s.webhookRepo.UpdateSecret(ctx, id, encryptedSecret); err != nil {
			return nil, fmt.Errorf("updating webhook secret: %w", err)
		}
	}

	// Return updated webhook
	return s.webhookRepo.GetByID(ctx, id)
}

// Delete removes a webhook endpoint.
func (s *WebhookService) Delete(ctx context.Context, id, customerID string) error {
	err := s.webhookRepo.DeleteByCustomer(ctx, id, customerID)
	if err != nil {
		return ErrWebhookNotFound
	}

	s.logger.Info("webhook deleted",
		"webhook_id", id,
		"customer_id", customerID)

	return nil
}

// ============================================================================
// Webhook Delivery Operations
// ============================================================================

// Deliver queues a webhook delivery for an event.
// This creates a delivery record and publishes a task for async processing.
func (s *WebhookService) Deliver(ctx context.Context, event string, data json.RawMessage) error {
	// Get all active webhooks subscribed to this event
	webhooks, err := s.webhookRepo.ListActiveForEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("listing webhooks for event %s: %w", event, err)
	}

	if len(webhooks) == 0 {
		s.logger.Debug("no webhooks subscribed to event", "event", event)
		return nil
	}

	s.logger.Info("delivering webhook event",
		"event", event,
		"webhook_count", len(webhooks))

	// Queue delivery for each webhook
	for _, webhook := range webhooks {
		if err := s.queueDelivery(ctx, &webhook, event, data); err != nil {
			s.logger.Error("failed to queue webhook delivery",
				"webhook_id", webhook.ID,
				"event", event,
				"error", err)
			// Continue with other webhooks
		}
	}

	return nil
}

// DeliverForCustomer queues webhook deliveries for an event, but only for a specific customer's webhooks.
func (s *WebhookService) DeliverForCustomer(ctx context.Context, customerID, event string, data json.RawMessage) error {
	// Get all webhooks for this customer
	webhooks, err := s.webhookRepo.ListByCustomer(ctx, customerID)
	if err != nil {
		return fmt.Errorf("listing webhooks for customer %s: %w", customerID, err)
	}

	// Filter to active webhooks subscribed to this event
	var activeWebhooks []repository.Webhook
	for _, w := range webhooks {
		if !w.Active {
			continue
		}
		for _, e := range w.Events {
			if e == event {
				activeWebhooks = append(activeWebhooks, w)
				break
			}
		}
	}

	if len(activeWebhooks) == 0 {
		return nil
	}

	s.logger.Info("delivering webhook event for customer",
		"event", event,
		"customer_id", customerID,
		"webhook_count", len(activeWebhooks))

	// Queue delivery for each webhook
	for _, webhook := range activeWebhooks {
		if err := s.queueDelivery(ctx, &webhook, event, data); err != nil {
			s.logger.Error("failed to queue webhook delivery",
				"webhook_id", webhook.ID,
				"event", event,
				"error", err)
		}
	}

	return nil
}

// queueDelivery creates a delivery record and queues it for processing.
func (s *WebhookService) queueDelivery(ctx context.Context, webhook *repository.Webhook, event string, data json.RawMessage) error {
	// Generate idempotency key
	idempotencyKey := uuid.New().String()

	// Build payload
	payload := WebhookPayload{
		Event:          event,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		IdempotencyKey: idempotencyKey,
		Data:           data,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling webhook payload: %w", err)
	}

	// Create delivery record
	delivery := &repository.WebhookDelivery{
		WebhookID:      webhook.ID,
		Event:          event,
		IdempotencyKey: idempotencyKey,
		Payload:        payloadBytes,
		Status:         repository.DeliveryStatusPending,
		MaxAttempts:    5,
	}

	if err := s.webhookRepo.CreateDelivery(ctx, delivery); err != nil {
		return fmt.Errorf("creating delivery record: %w", err)
	}

	// Publish task for async delivery
	if s.taskPublisher != nil {
		taskID, err := s.taskPublisher.PublishTask(ctx, "webhook.deliver", map[string]any{
			"delivery_id": delivery.ID,
			"webhook_id":  webhook.ID,
		})
		if err != nil {
			s.logger.Error("failed to publish webhook delivery task",
				"delivery_id", delivery.ID,
				"error", err)
			// Don't return error - the delivery can be picked up by the pending delivery processor
		} else {
			s.logger.Debug("published webhook delivery task",
				"task_id", taskID,
				"delivery_id", delivery.ID)
		}
	}

	return nil
}

// ListDeliveries retrieves delivery history for a webhook.
func (s *WebhookService) ListDeliveries(ctx context.Context, webhookID, customerID string, page, perPage int) ([]repository.WebhookDelivery, int, error) {
	// Verify webhook exists and belongs to customer
	_, err := s.webhookRepo.GetByIDAndCustomer(ctx, webhookID, customerID)
	if err != nil {
		return nil, 0, ErrWebhookNotFound
	}

	limit := perPage
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	return s.webhookRepo.ListDeliveriesByWebhook(ctx, webhookID, limit, offset)
}

// ============================================================================
// Helper Methods
// ============================================================================

// validateWebhookURL validates that a webhook URL is properly formatted and uses HTTPS.
func (s *WebhookService) validateWebhookURL(webhookURL string) error {
	// Parse URL
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL format", ErrInvalidURL)
	}

	// Must be HTTPS
	if !strings.EqualFold(parsed.Scheme, "https") {
		return ErrInvalidURL
	}

	// Must have a host
	if parsed.Host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}

	return nil
}

// encryptSecret encrypts the webhook secret for storage using AES-256-GCM.
func (s *WebhookService) encryptSecret(secret string) (string, error) {
	encrypted, err := crypto.Encrypt(secret, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypting secret: %w", err)
	}
	return encrypted, nil
}

// decryptSecret decrypts the stored encrypted secret.
func (s *WebhookService) decryptSecret(encryptedSecret string) (string, error) {
	decrypted, err := crypto.Decrypt(encryptedSecret, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("decrypting secret: %w", err)
	}
	return decrypted, nil
}

// GenerateSignature generates an HMAC-SHA256 signature for a webhook payload.
// The signature is used to verify the authenticity of webhook deliveries.
func GenerateSignature(secret string, payload []byte) string {
	signature := hmac.New(sha256.New, []byte(secret))
	signature.Write(payload)
	return hex.EncodeToString(signature.Sum(nil))
}

// VerifySignature verifies an HMAC-SHA256 signature for a webhook payload.
func VerifySignature(secret string, payload []byte, signature string) bool {
	expected := GenerateSignature(secret, payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ToResponse converts a webhook to an API response format.
func ToResponse(webhook *repository.Webhook) WebhookResponse {
	return WebhookResponse{
		ID:            webhook.ID,
		URL:           webhook.URL,
		Events:        webhook.Events,
		Active:        webhook.Active,
		FailCount:     webhook.FailCount,
		LastSuccessAt: webhook.LastSuccessAt,
		LastFailureAt: webhook.LastFailureAt,
		CreatedAt:     webhook.CreatedAt,
		UpdatedAt:     webhook.UpdatedAt,
	}
}