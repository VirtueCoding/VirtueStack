// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// Task type constant for webhook delivery.
const TaskTypeWebhookDeliver = "webhook.deliver"

// Retry intervals for webhook delivery: 10s, 60s, 5m, 30m, 2h
var retryIntervals = []time.Duration{
	10 * time.Second,
	60 * time.Second,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// MaxConsecutiveFailures aliases repository.WebhookMaxFailCount for use within task handlers.
const MaxConsecutiveFailures = repository.WebhookMaxFailCount

// WebhookDeliveryPayload represents the payload for webhook.deliver tasks.
type WebhookDeliveryPayload struct {
	DeliveryID string `json:"delivery_id"`
	WebhookID  string `json:"webhook_id"`
}

// WebhookDeliveryDeps contains dependencies for webhook delivery tasks.
// Security Note (QG-02): EncryptionKey is stored as a plaintext string in memory.
// This is an inherent limitation of symmetric encryption at rest - the key must be
// accessible to encrypt/decrypt webhook secrets. The key is loaded from an environment
// variable at startup and never logged or exposed in error messages. For higher security
// requirements, consider using a hardware security module (HSM) or cloud key management
// service (e.g., AWS KMS, HashiCorp Vault) which would require network calls for each
// encryption operation. The current approach is appropriate for the threat model where
// memory access by an attacker implies full system compromise.
type WebhookDeliveryDeps struct {
	WebhookRepo        *repository.WebhookRepository
	HTTPClient         *http.Client
	Logger             *slog.Logger
	EncryptionKey      string
	OnWebhookDisabled  func(customerID, webhookID, url string, failCount int)
}

// RegisterWebhookDeliveryHandler registers the webhook delivery task handler.
func RegisterWebhookDeliveryHandler(worker *Worker, deps *WebhookDeliveryDeps) {
	worker.RegisterHandler(TaskTypeWebhookDeliver, func(ctx context.Context, task *models.Task) error {
		return handleWebhookDeliver(ctx, task, deps)
	})

	deps.Logger.Info("webhook delivery handler registered", "task_type", TaskTypeWebhookDeliver)
}

// handleWebhookDeliver handles the webhook delivery task.
// Steps:
//  1. Parse payload
//  2. Get delivery record
//  3. Get webhook configuration
//  4. Generate HMAC signature
//  5. Send HTTP POST request
//  6. Update delivery status
//  7. Update webhook fail_count
func handleWebhookDeliver(ctx context.Context, task *models.Task, deps *WebhookDeliveryDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", TaskTypeWebhookDeliver)

	// Parse payload
	var payload WebhookDeliveryPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse webhook.deliver payload", "error", err)
		return fmt.Errorf("parsing webhook.deliver payload: %w", err)
	}

	logger = logger.With("delivery_id", payload.DeliveryID, "webhook_id", payload.WebhookID)
	logger.Info("webhook.deliver task started")

	// Get delivery record
	delivery, err := deps.WebhookRepo.GetDeliveryByID(ctx, payload.DeliveryID)
	if err != nil {
		logger.Error("failed to get delivery record", "error", err)
		return fmt.Errorf("getting delivery %s: %w", payload.DeliveryID, err)
	}

	// Check if already delivered (idempotency)
	if delivery.Status == repository.DeliveryStatusDelivered {
		logger.Info("delivery already completed, skipping")
		return nil
	}

	// Check if max attempts reached
	if delivery.AttemptCount >= delivery.MaxAttempts {
		logger.Warn("delivery max attempts reached, marking as failed",
			"attempt_count", delivery.AttemptCount)
		_ = deps.WebhookRepo.MarkDeliveryFailed(ctx, delivery.ID, "max attempts reached")
		return nil
	}

	// Get webhook configuration
	webhook, err := deps.WebhookRepo.GetByID(ctx, delivery.WebhookID)
	if err != nil {
		logger.Error("failed to get webhook", "error", err)
		return fmt.Errorf("getting webhook %s: %w", delivery.WebhookID, err)
	}

	// Check if webhook is still active
	if !webhook.IsActive {
		logger.Warn("webhook is disabled, skipping delivery")
		_ = deps.WebhookRepo.MarkDeliveryFailed(ctx, delivery.ID, "webhook disabled")
		return nil
	}

	// Perform the HTTP delivery attempt, update records, and handle auto-disable.
	// The success/failure outcome is logged inside executeDeliveryAttempt.
	executeDeliveryAttempt(ctx, deps, webhook, delivery, logger)

	return nil
}

// executeDeliveryAttempt performs a single delivery attempt for the given
// webhook/delivery pair, then updates the delivery record and webhook status in
// the database. On failure it increments the webhook's fail counter and
// auto-disables the webhook if MaxConsecutiveFailures is reached.
//
// It is extracted from handleWebhookDeliver and ProcessPendingDeliveries to
// eliminate the duplicate retry logic that previously existed in both callers.
//
// Returns true if the delivery was successful, false otherwise.
func executeDeliveryAttempt(
	ctx context.Context,
	deps *WebhookDeliveryDeps,
	webhook *models.CustomerWebhook,
	delivery *models.WebhookDelivery,
	logger *slog.Logger,
) bool {
	success, responseStatus, responseBody, errMsg := deliverWebhook(ctx, deps.HTTPClient, webhook, delivery, deps.EncryptionKey, logger)

	// Calculate next retry time if failed.
	var nextRetryAt *time.Time
	if !success && delivery.AttemptCount+1 < delivery.MaxAttempts {
		nextRetry := calculateNextRetry(delivery.AttemptCount)
		nextRetryAt = &nextRetry
	}

	// Persist delivery outcome.
	if err := deps.WebhookRepo.UpdateDeliveryAttempt(ctx, delivery.ID, success, responseStatus, responseBody, errMsg, nextRetryAt); err != nil {
		logger.Error("failed to update delivery record", "delivery_id", delivery.ID, "error", err)
	}

	// Update aggregate delivery status on the parent webhook.
	if err := deps.WebhookRepo.UpdateDeliveryStatus(ctx, webhook.ID, success); err != nil {
		logger.Error("failed to update webhook delivery status", "webhook_id", webhook.ID, "error", err)
	}

	if success {
		logger.Info("webhook delivered successfully",
			"delivery_id", delivery.ID,
			"attempt_count", delivery.AttemptCount+1,
			"response_status", responseStatus)
	} else {
		logger.Warn("webhook delivery failed",
			"delivery_id", delivery.ID,
			"attempt_count", delivery.AttemptCount+1,
			"error", errMsg,
			"next_retry_at", nextRetryAt)

		newFailCount := webhook.FailCount + 1
		if newFailCount >= MaxConsecutiveFailures {
			disable := false
			if err := deps.WebhookRepo.Update(ctx, webhook.ID, nil, nil, &disable); err != nil {
				logger.Error("failed to persist webhook disable state",
					"webhook_id", webhook.ID,
					"error", err)
			} else {
				logger.Warn("webhook auto-disabled due to consecutive failures",
					"webhook_id", webhook.ID,
					"customer_id", webhook.CustomerID,
					"url", webhook.URL,
					"fail_count", newFailCount)

				if deps.OnWebhookDisabled != nil {
					deps.OnWebhookDisabled(webhook.CustomerID, webhook.ID, webhook.URL, newFailCount)
				}
			}
		}
	}

	return success
}

// deliverWebhook performs the actual HTTP delivery of a webhook.
// Returns success status, HTTP response code, response body, and error message.
func deliverWebhook(ctx context.Context, client *http.Client, webhook *models.CustomerWebhook, delivery *models.WebhookDelivery, encryptionKey string, logger *slog.Logger) (bool, int, string, string) {
	// Validate the URL is parseable. SSRF protection is enforced at TCP connect
	// time by the custom dialer in the HTTP client (see newSSRFSafeDialContext).
	_, err := url.Parse(webhook.URL)
	if err != nil {
		return false, 0, "", fmt.Sprintf("parsing webhook URL: %s", err.Error())
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewReader(delivery.Payload))
	if err != nil {
		return false, 0, "", fmt.Sprintf("creating request: %s", err.Error())
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VirtueStack-Webhooks/1.0")
	req.Header.Set("X-Webhook-ID", webhook.ID)
	req.Header.Set("X-Delivery-ID", delivery.ID)
	req.Header.Set("X-Idempotency-Key", delivery.IdempotencyKey)

	// Decrypt the secret and generate HMAC signature
	decryptedSecret, err := crypto.Decrypt(webhook.SecretHash, encryptionKey)
	if err != nil {
		return false, 0, "", fmt.Sprintf("decrypting webhook secret: %s", err.Error())
	}

	signature := crypto.GenerateHMACSignature(decryptedSecret, delivery.Payload)
	req.Header.Set("X-Webhook-Signature", signature)

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return false, 0, "", fmt.Sprintf("sending request: %s", err.Error())
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Warn("failed to close response body", "error", closeErr)
		}
	}()

	// Read response body (limited to 4KB)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false, resp.StatusCode, "", fmt.Sprintf("reading response: %s", err.Error())
	}
	responseBody := string(bodyBytes)

	// Consider 2xx status codes as success
	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	if !success {
		return false, resp.StatusCode, responseBody, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncateString(responseBody, 200))
	}

	return true, resp.StatusCode, responseBody, ""
}

// calculateNextRetry calculates the next retry time based on attempt count.
// Retry schedule: 10s, 60s, 5m, 30m, 2h
func calculateNextRetry(attemptCount int) time.Time {
	// Ensure attemptCount is within bounds
	if attemptCount < 0 {
		attemptCount = 0
	}
	if attemptCount >= len(retryIntervals) {
		attemptCount = len(retryIntervals) - 1
	}

	retryInterval := retryIntervals[attemptCount]
	return time.Now().UTC().Add(retryInterval)
}

// truncateString truncates a string to a maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ProcessPendingDeliveries processes all pending webhook deliveries.
// This should be called periodically by a scheduler to handle retries.
func ProcessPendingDeliveries(ctx context.Context, deps *WebhookDeliveryDeps, batchSize int) error {
	logger := deps.Logger.With("component", "webhook-delivery-processor")

	// Get pending deliveries
	deliveries, err := deps.WebhookRepo.GetPendingDeliveries(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("getting pending deliveries: %w", err)
	}

	if len(deliveries) == 0 {
		logger.Info("no pending deliveries to process")
		return nil
	}

	logger.Info("processing pending webhook deliveries", "count", len(deliveries))

	successCount := 0
	failCount := 0

	for _, delivery := range deliveries {
		// Get webhook
		webhook, err := deps.WebhookRepo.GetByID(ctx, delivery.WebhookID)
		if err != nil {
			logger.Error("failed to get webhook for delivery",
				"delivery_id", delivery.ID,
				"webhook_id", delivery.WebhookID,
				"error", err)
			failCount++
			continue
		}

		// Skip if webhook is disabled
		if !webhook.IsActive {
			_ = deps.WebhookRepo.MarkDeliveryFailed(ctx, delivery.ID, "webhook disabled")
			failCount++
			continue
		}

		// Perform delivery attempt, update records, and handle auto-disable.
		if executeDeliveryAttempt(ctx, deps, webhook, &delivery, logger) {
			successCount++
		} else {
			failCount++
		}
	}

	logger.Info("finished processing pending deliveries",
		"total", len(deliveries),
		"success", successCount,
		"failed", failCount)

	return nil
}

// newSSRFSafeDialContext returns a DialContext function that resolves the target
// hostname and validates every resolved IP against the private-range blocklist
// before opening the TCP connection. Because the check and the dial happen
// atomically inside the same call, this eliminates the DNS-rebinding TOCTOU
// window that exists when a pre-flight lookup is performed separately.
func newSSRFSafeDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	baseDialer := &net.Dialer{Timeout: 10 * time.Second}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("parsing dial address %q: %w", addr, err)
		}

		// Resolve the hostname to IP addresses.
		ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolving webhook hostname %q: %w", host, err)
		}

		// Block any address that falls in a private or reserved range.
		for _, ia := range ipAddrs {
			if isPrivateIPDeliver(ia.IP) {
				return nil, fmt.Errorf("webhook URL resolves to private/internal IP: %s", addr)
			}
		}

		// Reconnect using the resolved address so the exact IP that passed the
		// check is the one being dialed (not a second resolution).
		if len(ipAddrs) == 0 {
			return nil, fmt.Errorf("no addresses resolved for webhook hostname %q", host)
		}
		resolvedAddr := net.JoinHostPort(ipAddrs[0].IP.String(), port)
		return baseDialer.DialContext(ctx, network, resolvedAddr)
	}
}

// DefaultHTTPClient returns a configured HTTP client for webhook deliveries.
// SSRF protection is enforced at TCP connection time via newSSRFSafeDialContext.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:         newSSRFSafeDialContext(),
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("webhook delivery does not follow redirects")
		},
	}
}

// isPrivateIPDeliver delegates to the shared util.IsPrivateIP function.
// All private/reserved range logic is maintained in one place.
func isPrivateIPDeliver(ip net.IP) bool {
	return util.IsPrivateIP(ip)
}
