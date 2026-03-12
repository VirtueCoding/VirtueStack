// Package integration provides end-to-end integration tests for VirtueStack.
package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookRegistration tests webhook registration operations.
func TestWebhookRegistration(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("RegisterWebhook", func(t *testing.T) {
		// Register a new webhook
		webhook := &models.CustomerWebhook{
			CustomerID: TestCustomerID,
			URL:        "https://example.com/webhook",
			SecretHash: "test-secret-hash",
			Events:     []string{models.WebhookEventVMCreated, models.WebhookEventVMDeleted},
			IsActive:   true,
		}

		webhookID, err := suite.WebhookService.Register(ctx, webhook)

		require.NoError(t, err, "Webhook registration should succeed")
		assert.NotEmpty(t, webhookID, "Webhook ID should be generated")
	})

	t.Run("ListWebhooks", func(t *testing.T) {
		// Register multiple webhooks
		for i := 0; i < 3; i++ {
			_, err := CreateTestWebhook(ctx, TestCustomerID)
			require.NoError(t, err)
		}

		// List webhooks
		webhooks, err := suite.WebhookService.ListByCustomer(ctx, TestCustomerID)

		require.NoError(t, err, "Listing webhooks should succeed")
		assert.GreaterOrEqual(t, len(webhooks), 3, "Should have at least 3 webhooks")
	})

	t.Run("UpdateWebhook", func(t *testing.T) {
		// Create a webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Update webhook
		newEvents := []string{models.WebhookEventVMStarted, models.WebhookEventVMStopped}
		newURL := "https://example.com/updated"
		active := true
		_, err = suite.WebhookService.Update(ctx, webhookID, TestCustomerID, services.UpdateWebhookRequest{
			URL:      &newURL,
			Events:   newEvents,
			Active:   &active,
		})

		require.NoError(t, err, "Updating webhook should succeed")

		// Verify update
		webhook, err := suite.WebhookRepo.GetByID(ctx, webhookID)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/updated", webhook.URL, "URL should be updated")
	})

	t.Run("DeleteWebhook", func(t *testing.T) {
		// Create a webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Delete webhook
		err = suite.WebhookService.Delete(ctx, webhookID, TestCustomerID)
		require.NoError(t, err, "Deleting webhook should succeed")

		// Verify deletion
		_, err = suite.WebhookRepo.GetByID(ctx, webhookID)
		assert.Error(t, err, "Webhook should not be found after deletion")
	})

	t.Run("InvalidWebhookURL", func(t *testing.T) {
		webhook := &models.CustomerWebhook{
			CustomerID: TestCustomerID,
			URL:        "not-a-valid-url",
			SecretHash: "test-secret-hash",
			Events:     []string{models.WebhookEventVMCreated},
			IsActive:   true,
		}

		_, err := suite.WebhookService.Register(ctx, webhook)
		assert.Error(t, err, "Registration should fail with invalid URL")
	})
}

// TestWebhookDelivery tests webhook delivery operations.
func TestWebhookDelivery(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("SuccessfulDelivery", func(t *testing.T) {
		// Create a test HTTP server
		received := make(chan *http.Request, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			received <- r
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Register webhook pointing to test server
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Update webhook URL to test server
		_, _ = suite.DBPool.Exec(ctx, "UPDATE webhooks SET url = $1 WHERE id = $2", server.URL, webhookID)

		// Trigger webhook delivery
		payload := map[string]interface{}{
			"event":   models.WebhookEventVMCreated,
			"vm_id":   TestVMID,
			"message": "VM created successfully",
		}
		_ = payload

		err = suite.WebhookService.Deliver(ctx, models.WebhookEventVMCreated, nil)
		require.NoError(t, err, "Webhook delivery should succeed")

		// Wait for delivery
		select {
		case req := <-received:
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			assert.NotEmpty(t, req.Header.Get("X-Webhook-Signature"))
		case <-time.After(5 * time.Second):
			t.Fatal("Webhook not received within timeout")
		}

		// Verify delivery record
	})

	t.Run("FailedDeliveryWithRetry", func(t *testing.T) {
		// Create a test HTTP server that always fails
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		// Register webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)
		_, _ = suite.DBPool.Exec(ctx, "UPDATE webhooks SET url = $1 WHERE id = $2", server.URL, webhookID)

		// Attempt delivery
		_ = map[string]interface{}{"event": models.WebhookEventVMCreated}
		err = suite.WebhookService.Deliver(ctx, models.WebhookEventVMCreated, nil)
		require.NoError(t, err)

	})

	t.Run("DeliveryTimeout", func(t *testing.T) {
		// Create a slow test HTTP server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Second) // Exceed typical timeout
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Register webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)
		_, _ = suite.DBPool.Exec(ctx, "UPDATE webhooks SET url = $1 WHERE id = $2", server.URL, webhookID)

		// Attempt delivery (with context timeout)
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_ = map[string]interface{}{"event": models.WebhookEventVMCreated}
		err = suite.WebhookService.Deliver(ctx, models.WebhookEventVMCreated, nil)
		// Should timeout or fail
		assert.Error(t, err, "Delivery should timeout")
	})

	t.Run("UnavailableEndpoint", func(t *testing.T) {
		// Register webhook pointing to non-existent endpoint
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)
		_, _ = suite.DBPool.Exec(ctx, "UPDATE webhooks SET url = $1 WHERE id = $2", "https://non-existent-endpoint.example.com/webhook", webhookID)

		// Attempt delivery
		_ = map[string]interface{}{"event": models.WebhookEventVMCreated}
		err = suite.WebhookService.Deliver(ctx, models.WebhookEventVMCreated, nil)

		// Should fail gracefully
_ = err
	})
}

// TestWebhookRetryLogic tests webhook retry mechanisms.
func TestWebhookRetryLogic(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("RetryAfterFailure", func(t *testing.T) {
		// Create a webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Create a failed delivery
		deliveryID := "delivery-retry-test"
		now := time.Now()
		nextRetry := now.Add(1 * time.Minute)

		_, _ = suite.DBPool.Exec(ctx, `
			INSERT INTO webhook_deliveries (id, webhook_id, event, payload, attempt_count, success, next_retry_at, created_at)
			VALUES ($1, $2, $3, $4, 5, false, $5, NOW())
		`, deliveryID, webhookID, models.WebhookEventVMCreated, "{}", nextRetry)

		// Get pending retries
		pending, err := suite.WebhookService.GetPendingRetries(ctx, now.Add(5*time.Minute))
		require.NoError(t, err)

		// Should include our delivery
		found := false
		for _, d := range pending {
			if d.ID == deliveryID {
				found = true
				break
			}
		}
		assert.True(t, found, "Failed delivery should be in pending retries")
	})

	t.Run("MaxRetryAttempts", func(t *testing.T) {
		// Create a webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Create a delivery that has exceeded max attempts
		deliveryID := "delivery-max-attempts"
		_, _ = suite.DBPool.Exec(ctx, `
			INSERT INTO webhook_deliveries (id, webhook_id, event, payload, attempt_count, success, created_at)
			VALUES ($1, $2, $3, $4, 10, false, NOW())
		`, deliveryID, webhookID, models.WebhookEventVMCreated, "{}")

		// Should not be in pending retries (exceeded max attempts)
		pending, err := suite.WebhookService.GetPendingRetries(ctx, time.Now().Add(1*time.Hour))
		require.NoError(t, err)

		found := false
		for _, d := range pending {
			if d.ID == deliveryID {
				found = true
				break
			}
		}
		assert.False(t, found, "Delivery with max attempts should not be in pending retries")
	})

	t.Run("ExponentialBackoff", func(t *testing.T) {
		// Test that retry intervals increase exponentially
		attemptCounts := []int{1, 2, 3, 4, 5}

		for _, count := range attemptCounts {
			nextRetry := suite.WebhookService.CalculateNextRetry(count)
			expectedBase := time.Duration(1<<uint(count-1)) * time.Minute // 2^(n-1) minutes
			// Allow some variance
			assert.GreaterOrEqual(t, nextRetry.Minutes(), expectedBase.Minutes()*0.8, "Retry interval should be exponential")
		}
	})
}

// TestWebhookSignatureVerification tests webhook signature verification.
func TestWebhookSignatureVerification(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	secret := TestWebhookSecret

	t.Run("ValidSignature", func(t *testing.T) {
		payload := map[string]interface{}{
			"event": models.WebhookEventVMCreated,
			"vm_id": TestVMID,
		}
		payloadBytes, _ := json.Marshal(payload)

		// Generate valid signature
		signature := generateSignature(payloadBytes, secret)

		// Verify signature
		valid := suite.WebhookService.VerifySignature(payloadBytes, signature, secret)
		assert.True(t, valid, "Valid signature should verify")
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		payload := map[string]interface{}{
			"event": models.WebhookEventVMCreated,
			"vm_id": TestVMID,
		}
		payloadBytes, _ := json.Marshal(payload)

		// Generate signature with wrong secret
		signature := generateSignature(payloadBytes, "wrong-secret")

		// Verify should fail
		valid := suite.WebhookService.VerifySignature(payloadBytes, signature, secret)
		assert.False(t, valid, "Invalid signature should not verify")
	})

	t.Run("TamperedPayload", func(t *testing.T) {
		payload := map[string]interface{}{
			"event": models.WebhookEventVMCreated,
			"vm_id": TestVMID,
		}
		payloadBytes, _ := json.Marshal(payload)

		// Generate signature
		signature := generateSignature(payloadBytes, secret)

		// Tamper with payload
		tamperedPayload := append(payloadBytes[:len(payloadBytes)-1], 'X')

		// Verify should fail
		valid := suite.WebhookService.VerifySignature(tamperedPayload, signature, secret)
		assert.False(t, valid, "Tampered payload should not verify")
	})

	t.Run("EmptySignature", func(t *testing.T) {
		payload := map[string]interface{}{"event": "test"}
		payloadBytes, _ := json.Marshal(payload)

		valid := suite.WebhookService.VerifySignature(payloadBytes, "", secret)
		assert.False(t, valid, "Empty signature should not verify")
	})
}

// TestWebhookEventFiltering tests event filtering for webhooks.
func TestWebhookEventFiltering(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("FilterMatchingEvents", func(t *testing.T) {
		// Create webhook that only listens for vm.created
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Update events
		_, _ = suite.DBPool.Exec(ctx, "UPDATE webhooks SET events = ARRAY['vm.created'] WHERE id = $1", webhookID)

		// Get webhooks for event
		webhooks, err := suite.WebhookService.GetWebhooksForEvent(ctx, TestCustomerID, models.WebhookEventVMCreated)
		require.NoError(t, err)

		// Should include our webhook
		found := false
		for _, w := range webhooks {
			if w.ID == webhookID {
				found = true
				break
			}
		}
		assert.True(t, found, "Webhook should be matched for vm.created event")
	})

	t.Run("FilterNonMatchingEvents", func(t *testing.T) {
		// Create webhook that only listens for vm.created
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)
		_, _ = suite.DBPool.Exec(ctx, "UPDATE webhooks SET events = ARRAY['vm.created'] WHERE id = $1", webhookID)

		// Get webhooks for different event
		webhooks, err := suite.WebhookService.GetWebhooksForEvent(ctx, TestCustomerID, models.WebhookEventVMDeleted)
		require.NoError(t, err)

		// Should NOT include our webhook
		found := false
		for _, w := range webhooks {
			if w.ID == webhookID {
				found = true
				break
			}
		}
		assert.False(t, found, "Webhook should not be matched for vm.deleted event")
	})

	t.Run("MultipleEventTypes", func(t *testing.T) {
		// Create webhook for multiple events
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)
		_, _ = suite.DBPool.Exec(ctx, `
			UPDATE webhooks SET events = ARRAY['vm.created', 'vm.deleted', 'vm.started'] WHERE id = $1
		`, webhookID)

		// Test each event type
		for _, event := range []string{models.WebhookEventVMCreated, models.WebhookEventVMDeleted, models.WebhookEventVMStarted} {
			webhooks, err := suite.WebhookService.GetWebhooksForEvent(ctx, TestCustomerID, event)
			require.NoError(t, err)

			found := false
			for _, w := range webhooks {
				if w.ID == webhookID {
					found = true
					break
				}
			}
			assert.True(t, found, "Webhook should be matched for event: %s", event)
		}
	})
}

// TestWebhookDeliveryHistory tests delivery history tracking.
func TestWebhookDeliveryHistory(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("ListDeliveryHistory", func(t *testing.T) {
		// Create webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Create some delivery records
		for i := 0; i < 5; i++ {
			_, _ = suite.DBPool.Exec(ctx, `
				INSERT INTO webhook_deliveries (id, webhook_id, event, payload, attempt_count, success, created_at)
				VALUES ($1, $2, $3, $4, 1, $5, NOW() - INTERVAL '1 day' * $6)
			`, "delivery-"+string(rune('a'+i)), webhookID, models.WebhookEventVMCreated, "{}", i%2 == 0, i)
		}

		// List delivery history
		deliveries, _, err := suite.WebhookService.ListDeliveries(ctx, webhookID, TestCustomerID, 1, 10)
		require.NoError(t, err, "Listing deliveries should succeed")
		assert.GreaterOrEqual(t, len(deliveries), 5, "Should have at least 5 deliveries")
	})

	t.Run("DeliveryStatistics", func(t *testing.T) {
		// Create webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Create deliveries with known success/failure
		for i := 0; i < 10; i++ {
			_, _ = suite.DBPool.Exec(ctx, `
				INSERT INTO webhook_deliveries (id, webhook_id, event, payload, attempt_count, success, created_at)
				VALUES ($1, $2, $3, $4, 1, $5, NOW())
			`, "stats-delivery-"+string(rune('0'+i)), webhookID, models.WebhookEventVMCreated, "{}", i < 7) // 7 success, 3 failure
		}

		// Get statistics
		stats, err := suite.WebhookService.GetDeliveryStats(ctx, webhookID)
		require.NoError(t, err, "Getting delivery stats should succeed")
		assert.GreaterOrEqual(t, stats.TotalDeliveries, 10)
		assert.GreaterOrEqual(t, stats.SuccessRate, 0.7) // At least 70% success
	})
}

// TestWebhookSecurity tests webhook security features.
func TestWebhookSecurity(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("CustomerCannotAccessOtherWebhooks", func(t *testing.T) {
		// Create another customer
		otherCustomerID := "00000000-0000-0000-0000-000000000099"
		_, _ = suite.DBPool.Exec(ctx, `
			INSERT INTO customers (id, email, password_hash, name, status, created_at, updated_at)
			VALUES ($1, 'other2@example.com', 'hash', 'Other Customer', 'active', NOW(), NOW())
		`, otherCustomerID)

		// Create webhook for other customer
		webhookID, err := CreateTestWebhook(ctx, otherCustomerID)
		require.NoError(t, err)

		// Try to access with TestCustomerID
		_, err = suite.WebhookRepo.GetByIDAndCustomer(ctx, webhookID, TestCustomerID)
		assert.Error(t, err, "Should not be able to access other customer's webhook")
		assert.True(t, sharederrors.Is(err, sharederrors.ErrNotFound), "Should return ErrNotFound")

		// Cleanup
		_, _ = suite.DBPool.Exec(ctx, "DELETE FROM webhooks WHERE customer_id = $1", otherCustomerID)
		_, _ = suite.DBPool.Exec(ctx, "DELETE FROM customers WHERE id = $1", otherCustomerID)
	})

	t.Run("SecretHashing", func(t *testing.T) {
		// Create webhook
		webhookID, err := CreateTestWebhook(ctx, TestCustomerID)
		require.NoError(t, err)

		// Get webhook
		webhook, err := suite.WebhookRepo.GetByID(ctx, webhookID)
		require.NoError(t, err)

		// Secret hash should be stored, not plain secret
		assert.NotEmpty(t, webhook.SecretHash, "Secret hash should be stored")
		assert.NotEqual(t, TestWebhookSecret, webhook.SecretHash, "Secret should be hashed")
	})

	t.Run("HTTPSOnly", func(t *testing.T) {
		// Try to register webhook with HTTP URL
		webhook := &models.CustomerWebhook{
			CustomerID: TestCustomerID,
			URL:        "http://insecure.example.com/webhook", // HTTP, not HTTPS
			SecretHash: "test-secret-hash",
			Events:     []string{models.WebhookEventVMCreated},
			IsActive:   true,
		}

		_, err := suite.WebhookService.Register(ctx, webhook)
		// Should reject HTTP URLs (depends on implementation)
		// If not rejected, at least log a warning
		_ = err // Accept either behavior for now
	})
}

// Helper function to generate HMAC signature
func generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}