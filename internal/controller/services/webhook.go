// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// WebhookService provides business logic for managing webhook endpoints and deliveries.
type WebhookService struct {
	webhookRepo       *repository.WebhookRepository
	taskPublisher     TaskPublisher
	logger            *slog.Logger
	encryptionKey     string
	httpClient        *http.Client
	skipURLValidation bool // For testing only
}

const (
	webhookSchedulerInterval  = 30 * time.Second
	webhookSchedulerBatchSize = 100
)

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
		// Use the SSRF-safe HTTP client so that outbound requests from the
		// service (e.g. the verification ping in Register) are subject to the
		// same IP-range enforcement as async task deliveries.
		httpClient: tasks.DefaultHTTPClient(),
	}
}

// StartScheduler runs the customer webhook recovery loop. It processes due pending
// and retrying deliveries so publish failures or expired leases are retried.
func (s *WebhookService) StartScheduler(ctx context.Context) {
	s.logger.Info("webhook delivery scheduler started")

	ticker := time.NewTicker(webhookSchedulerInterval)
	defer ticker.Stop()

	s.runSchedulerTick(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("webhook delivery scheduler stopped", "reason", ctx.Err())
			return
		case <-ticker.C:
			s.runSchedulerTick(ctx)
		}
	}
}

func (s *WebhookService) runSchedulerTick(ctx context.Context) {
	deps := &tasks.WebhookDeliveryDeps{
		WebhookRepo:   s.webhookRepo,
		HTTPClient:    s.httpClient,
		Logger:        s.logger,
		EncryptionKey: s.encryptionKey,
	}

	if err := tasks.ProcessPendingDeliveries(ctx, deps, webhookSchedulerBatchSize); err != nil {
		s.logger.Warn("webhook delivery scheduler tick failed", "error", err)
	}
}

// SetHTTPClient sets a custom HTTP client for the webhook service.
// This is primarily intended for testing with mock servers.
// If client is nil, resets to the default SSRF-safe HTTP client.
func (s *WebhookService) SetHTTPClient(client *http.Client) {
	if client == nil {
		s.httpClient = tasks.DefaultHTTPClient()
		return
	}
	s.httpClient = client
}

// SetSkipURLValidation enables or disables URL validation for testing.
// When true, private IP checks are skipped (use with caution - test only).
// Panics if called in a production environment (APP_ENV=production) (F-071).
func (s *WebhookService) SetSkipURLValidation(skip bool) {
	if os.Getenv("APP_ENV") == "production" {
		panic("SetSkipURLValidation must not be called in production")
	}
	s.skipURLValidation = skip
}

// Valid webhook events that can be subscribed to.
var ValidWebhookEvents = map[string]bool{
	"vm.created":          true,
	"vm.deleted":          true,
	"vm.started":          true,
	"vm.stopped":          true,
	"vm.reinstalled":      true,
	"vm.migrated":         true,
	"backup.completed":    true,
	"backup.failed":       true,
	"snapshot.created":    true,
	"bandwidth.threshold": true,
}

// MaxWebhooksPerCustomer is the maximum number of webhooks a customer can have.
const MaxWebhooksPerCustomer = 5

// Errors returned by the webhook service.
var (
	ErrInvalidURL      = errors.New("webhook URL must be HTTPS")
	ErrInvalidEvent    = errors.New("invalid webhook event")
	ErrTooManyWebhooks = errors.New("maximum webhook limit reached")
	ErrWebhookNotFound = errors.New("webhook not found")
	ErrSecretTooShort  = errors.New("secret must be at least 16 characters")
	ErrSecretTooLong   = errors.New("secret must be at most 128 characters")
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
func (s *WebhookService) Create(ctx context.Context, req CreateWebhookRequest) (*models.CustomerWebhook, error) {
	// Validate URL
	if err := s.validateWebhookURL(ctx, req.URL); err != nil {
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
	webhook := &models.CustomerWebhook{
		CustomerID: req.CustomerID,
		URL:        req.URL,
		SecretHash: encryptedSecret, // Store encrypted secret (field name kept for compatibility)
		Events:     req.Events,
		IsActive:   true,
	}

	if err := s.webhookRepo.Create(ctx, webhook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	parsedURL, parseErr := url.Parse(req.URL)
	safeURL := ""
	if parseErr != nil {
		s.logger.Warn("failed to parse webhook URL for audit log", "url", req.URL, "error", parseErr)
	} else if parsedURL != nil {
		safeURL = parsedURL.Scheme + "://" + parsedURL.Host
	}
	s.logger.Info("webhook created",
		"webhook_id", webhook.ID,
		"customer_id", req.CustomerID,
		"url_domain", safeURL,
		"events", req.Events)

	return webhook, nil
}

// Get retrieves a webhook by ID for a specific customer.
func (s *WebhookService) Get(ctx context.Context, id, customerID string) (*models.CustomerWebhook, error) {
	webhook, err := s.webhookRepo.GetByIDAndCustomer(ctx, id, customerID)
	if err != nil {
		return nil, ErrWebhookNotFound
	}
	return webhook, nil
}

// List retrieves all webhooks for a customer.
func (s *WebhookService) List(ctx context.Context, customerID string) ([]models.CustomerWebhook, error) {
	return s.webhookRepo.ListByCustomer(ctx, customerID)
}

// Update updates a webhook endpoint.
func (s *WebhookService) Update(ctx context.Context, id, customerID string, req UpdateWebhookRequest) (*models.CustomerWebhook, error) {
	// Verify webhook exists and belongs to customer
	_, err := s.webhookRepo.GetByIDAndCustomer(ctx, id, customerID)
	if err != nil {
		return nil, ErrWebhookNotFound
	}

	var encryptedSecret string
	var secretHash *string

	// Validate URL if provided
	if req.URL != nil {
		if err := s.validateWebhookURL(ctx, *req.URL); err != nil {
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

	// Validate and prepare secret before any database mutation so updates remain atomic.
	if req.Secret != nil {
		if len(*req.Secret) < 16 {
			return nil, ErrSecretTooShort
		}
		if len(*req.Secret) > 128 {
			return nil, ErrSecretTooLong
		}
		encryptedSecret, err = s.encryptSecret(*req.Secret)
		if err != nil {
			return nil, fmt.Errorf("encrypting secret: %w", err)
		}
		secretHash = &encryptedSecret
	}

	// Update webhook and optional secret in a single repository write to keep the
	// mutation atomic from the caller's perspective.
	if err := s.webhookRepo.Update(ctx, id, req.URL, req.Events, req.Active, secretHash); err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
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
	var activeWebhooks []models.CustomerWebhook
	for _, w := range webhooks {
		if !w.IsActive {
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
func (s *WebhookService) queueDelivery(ctx context.Context, webhook *models.CustomerWebhook, event string, data json.RawMessage) error {
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
	delivery := &models.WebhookDelivery{
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
		taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeWebhookDeliver, map[string]any{
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
func (s *WebhookService) ListDeliveries(ctx context.Context, webhookID, customerID string, pagination models.PaginationParams) ([]models.WebhookDelivery, bool, string, error) {
	// Verify webhook exists and belongs to customer
	_, err := s.webhookRepo.GetByIDAndCustomer(ctx, webhookID, customerID)
	if err != nil {
		return nil, false, "", ErrWebhookNotFound
	}

	return s.webhookRepo.ListDeliveriesByWebhook(ctx, webhookID, pagination.PerPage, pagination.Cursor)
}

// ============================================================================
// Helper Methods
// ============================================================================

// validateWebhookURL validates that a webhook URL is properly formatted, uses HTTPS,
// and does not resolve to a private/internal IP address (SSRF protection).
func (s *WebhookService) validateWebhookURL(ctx context.Context, webhookURL string) error {
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

	// Skip private IP check for testing
	if s.skipURLValidation {
		return nil
	}

	// Defense-in-depth: resolve the hostname at registration time and reject
	// any URL that currently maps to a private/reserved IP. This provides a
	// fast-fail user-visible error. The primary SSRF enforcement is at TCP
	// connect time via the SSRF-safe transport in tasks.DefaultHTTPClient(),
	// which eliminates the DNS-rebinding TOCTOU window that this pre-flight
	// check cannot close on its own.
	addrs, err := net.DefaultResolver.LookupHost(ctx, parsed.Hostname())
	if err != nil {
		return fmt.Errorf("cannot resolve webhook URL hostname: %w", err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if util.IsPrivateIP(ip) {
			return fmt.Errorf("webhook URL resolves to private/internal IP address")
		}
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
	return crypto.GenerateHMACSignature(secret, payload)
}

// VerifySignature verifies an HMAC-SHA256 signature for a webhook payload.
func VerifySignature(secret string, payload []byte, signature string) bool {
	expected := GenerateSignature(secret, payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ToResponse converts a webhook to an API response format.
func ToResponse(webhook *models.CustomerWebhook) WebhookResponse {
	return WebhookResponse{
		ID:            webhook.ID,
		URL:           webhook.URL,
		Events:        webhook.Events,
		Active:        webhook.IsActive,
		FailCount:     webhook.FailCount,
		LastSuccessAt: webhook.LastSuccessAt,
		LastFailureAt: webhook.LastFailureAt,
		CreatedAt:     webhook.CreatedAt,
		UpdatedAt:     webhook.UpdatedAt,
	}
}

// DeliveryStats holds statistics for webhook deliveries.
type DeliveryStats struct {
	TotalDeliveries int
	SuccessRate     float64
}

// ProcessPendingDeliveriesSync processes pending webhook deliveries synchronously.
// This is primarily intended for testing without a task queue.
func (s *WebhookService) ProcessPendingDeliveriesSync(ctx context.Context, batchSize int) error {
	deps := &tasks.WebhookDeliveryDeps{
		WebhookRepo:   s.webhookRepo,
		HTTPClient:    s.httpClient,
		Logger:        s.logger,
		EncryptionKey: s.encryptionKey,
	}
	return tasks.ProcessPendingDeliveries(ctx, deps, batchSize)
}

// Register validates a webhook URL, sends a test ping, and creates the webhook.
// Returns the signing secret for the created webhook.
func (s *WebhookService) Register(ctx context.Context, webhook *models.CustomerWebhook) (string, error) {
	// Validate URL (must be HTTPS)
	if err := s.validateWebhookURL(ctx, webhook.URL); err != nil {
		return "", err
	}

	// Validate events
	for _, event := range webhook.Events {
		if !ValidWebhookEvents[event] {
			return "", fmt.Errorf("%w: %s", ErrInvalidEvent, event)
		}
	}

	// Generate signing secret
	secret, err := crypto.GenerateRandomToken(32)
	if err != nil {
		return "", fmt.Errorf("generating webhook secret: %w", err)
	}

	// Send a test ping to verify the endpoint is reachable
	testPayload := WebhookPayload{
		Event:     "webhook.test",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      json.RawMessage(`{"message":"webhook verification ping"}`),
	}
	body, err := json.Marshal(testPayload)
	if err != nil {
		return "", fmt.Errorf("marshal webhook verification payload: %w", err)
	}
	sig := GenerateSignature(secret, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", sig)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("webhook endpoint unreachable: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.logger.Debug("failed to close webhook response body", "error", err)
		}
	}()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
	}

	// Create the webhook via existing Create method
	created, err := s.Create(ctx, CreateWebhookRequest{
		CustomerID: webhook.CustomerID,
		URL:        webhook.URL,
		Secret:     secret,
		Events:     webhook.Events,
	})
	if err != nil {
		return "", fmt.Errorf("creating webhook: %w", err)
	}

	s.logger.Info("webhook registered", "webhook_id", created.ID, "customer_id", webhook.CustomerID)

	return secret, nil
}

func (s *WebhookService) ListByCustomer(ctx context.Context, customerID string) ([]models.CustomerWebhook, error) {
	return s.List(ctx, customerID)
}

func (s *WebhookService) GetPendingRetries(ctx context.Context, before time.Time) ([]models.WebhookDelivery, error) {
	return s.webhookRepo.GetPendingRetries(ctx, before)
}

func (s *WebhookService) CalculateNextRetry(attemptCount int) time.Duration {
	if attemptCount < 1 {
		return time.Minute
	}
	delay := time.Duration(1<<uint(attemptCount-1)) * time.Minute
	maxDelay := 24 * time.Hour
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func (s *WebhookService) VerifySignature(payload []byte, signature, secret string) bool {
	expected := GenerateSignature(secret, payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (s *WebhookService) GetWebhooksForEvent(ctx context.Context, customerID, event string) ([]models.CustomerWebhook, error) {
	if customerID == "" {
		return s.webhookRepo.ListActiveForEvent(ctx, event)
	}

	webhooks, err := s.webhookRepo.ListByCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("listing customer webhooks: %w", err)
	}

	filtered := make([]models.CustomerWebhook, 0, len(webhooks))
	for _, w := range webhooks {
		if !w.IsActive {
			continue
		}
		for _, e := range w.Events {
			if e == event {
				filtered = append(filtered, w)
				break
			}
		}
	}

	return filtered, nil
}

func (s *WebhookService) GetDeliveryStats(ctx context.Context, webhookID string) (*DeliveryStats, error) {
	total, counts, err := s.webhookRepo.CountDeliveriesByStatus(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("counting deliveries by status: %w", err)
	}

	if total == 0 {
		return &DeliveryStats{TotalDeliveries: 0, SuccessRate: 0}, nil
	}

	successCount := 0
	for _, sc := range counts {
		if sc.Status == repository.DeliveryStatusDelivered {
			successCount = sc.Count
			break
		}
	}

	return &DeliveryStats{
		TotalDeliveries: total,
		SuccessRate:     float64(successCount) / float64(total),
	}, nil
}

func (s *WebhookService) RetryDelivery(ctx context.Context, deliveryID string) error {
	delivery, err := s.webhookRepo.GetDeliveryByID(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("getting delivery: %w", err)
	}

	if err := s.webhookRepo.ResetDeliveryForRetry(ctx, deliveryID); err != nil {
		return fmt.Errorf("resetting delivery: %w", err)
	}

	if s.taskPublisher != nil {
		if _, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeWebhookDeliver, map[string]any{
			"delivery_id": deliveryID,
			"webhook_id":  delivery.WebhookID,
		}); err != nil {
			return fmt.Errorf("publishing retry delivery task: %w", err)
		}
	}

	return nil
}

func (s *WebhookService) GetDeliveryLogs(ctx context.Context, webhookID string, page, perPage int) ([]models.WebhookDelivery, bool, string, error) {
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	filter := repository.DeliveryListFilter{
		PaginationParams: models.PaginationParams{PerPage: perPage},
	}
	if webhookID != "" {
		filter.WebhookID = &webhookID
	}

	return s.webhookRepo.ListDeliveries(ctx, filter)
}

func (s *WebhookService) TestWebhook(ctx context.Context, webhookID string) error {
	webhook, err := s.webhookRepo.GetByID(ctx, webhookID)
	if err != nil {
		return fmt.Errorf("getting webhook: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"test":       true,
		"webhook_id": webhookID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshaling test payload: %w", err)
	}

	if err := s.queueDelivery(ctx, webhook, "webhook.test", payload); err != nil {
		return fmt.Errorf("queueing test delivery: %w", err)
	}

	return nil
}

func (s *WebhookService) RotateSecret(ctx context.Context, webhookID string) (string, error) {
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}

	plainSecret := hex.EncodeToString(secretBytes)
	encrypted, err := s.encryptSecret(plainSecret)
	if err != nil {
		return "", fmt.Errorf("encrypting secret: %w", err)
	}

	if err := s.webhookRepo.Update(ctx, webhookID, nil, nil, nil, &encrypted); err != nil {
		return "", fmt.Errorf("updating webhook secret: %w", err)
	}

	return plainSecret, nil
}
