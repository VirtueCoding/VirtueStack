// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// AlertConfig holds configuration for notification channels.
type AlertConfig struct {
	// SMTP configuration for email notifications
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string // From address for emails

	// Admin notification recipients
	AdminEmails []string
	AdminWebhooks []string // Webhook URLs for admin notifications

	// Enable/disable specific channels
	EnableEmail   bool
	EnableWebhook bool
}

// AlertType represents the type of alert being sent.
type AlertType string

const (
	AlertTypeNodeFailure   AlertType = "node.failure"
	AlertTypeNodeRecovery  AlertType = "node.recovery"
	AlertTypeNodeDraining  AlertType = "node.draining"
	AlertTypeVMMigration   AlertType = "vm.migration"
	AlertTypeIPMIAttempt   AlertType = "ipmi.attempt"
	AlertTypeSystemCritical AlertType = "system.critical"
)

// Alert represents an alert notification to be sent.
type Alert struct {
	Type        AlertType
	Subject     string
	Message     string
	Details     map[string]interface{}
	Timestamp   time.Time
	NodeID      string
	NodeHostname string
}

// AlertService handles sending alert notifications through various channels.
// Supports email (SMTP) and webhook notifications for operational alerts.
type AlertService struct {
	config      *AlertConfig
	webhookRepo *repository.WebhookRepository
	httpClient  *http.Client
	logger      *slog.Logger
}

// NewAlertService creates a new NotificationService with the given configuration.
func NewAlertService(
	config *AlertConfig,
	webhookRepo *repository.WebhookRepository,
	logger *slog.Logger,
) *AlertService {
	return &AlertService{
		config:      config,
		webhookRepo: webhookRepo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.With("component", "notification-service"),
	}
}

// SendAlert sends an alert notification through all enabled channels.
// This is the primary entry point for sending alerts.
// Returns an error only if ALL channels fail; partial failures are logged but don't return error.
func (s *AlertService) SendAlert(ctx context.Context, alert *Alert) error {
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}

	var errors []string

	// Send email notifications
	if s.config.EnableEmail && len(s.config.AdminEmails) > 0 {
		if err := s.sendEmailAlert(ctx, alert); err != nil {
			s.logger.Error("failed to send email alert",
				"alert_type", alert.Type,
				"error", err)
			errors = append(errors, fmt.Sprintf("email: %v", err))
		}
	}

	// Send webhook notifications
	if s.config.EnableWebhook && len(s.config.AdminWebhooks) > 0 {
		if err := s.sendWebhookAlert(ctx, alert); err != nil {
			s.logger.Error("failed to send webhook alert",
				"alert_type", alert.Type,
				"error", err)
			errors = append(errors, fmt.Sprintf("webhook: %v", err))
		}
	}

	// If all channels failed, return an error
	if len(errors) > 0 && (!s.config.EnableEmail || len(s.config.AdminEmails) == 0) &&
		(!s.config.EnableWebhook || len(s.config.AdminWebhooks) == 0) {
		return fmt.Errorf("all notification channels failed: %s", strings.Join(errors, "; "))
	}

	// Log partial failures but don't return error
	if len(errors) > 0 {
		s.logger.Warn("some notification channels failed",
			"alert_type", alert.Type,
			"errors", strings.Join(errors, "; "))
	}

	return nil
}

// sendEmailAlert sends an email notification to configured admin addresses.
func (s *AlertService) sendEmailAlert(ctx context.Context, alert *Alert) error {
	if s.config.SMTPHost == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	// Build email body
	body := s.buildEmailBody(alert)
	subject := fmt.Sprintf("[VirtueStack] %s", alert.Subject)

	// Send to each admin
	var errors []string
	for _, to := range s.config.AdminEmails {
		if err := s.sendEmail(to, subject, body); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", to, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("email send failures: %s", strings.Join(errors, "; "))
	}

	s.logger.Info("email alert sent",
		"alert_type", alert.Type,
		"recipients", len(s.config.AdminEmails))

	return nil
}

// sendEmail sends a single email via SMTP.
func (s *AlertService) sendEmail(to, subject, body string) error {
	// Build message
	msg := fmt.Sprintf("From: %s\r\n", s.config.SMTPFrom)
	msg += fmt.Sprintf("To: %s\r\n", to)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "MIME-version: 1.0;\r\nContent-Type: text/plain; charset=\"UTF-8\";\r\n"
	msg += "\r\n" + body

	// SMTP authentication
	var auth smtp.Auth
	if s.config.SMTPUsername != "" && s.config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.config.SMTPUsername, s.config.SMTPPassword, s.config.SMTPHost)
	}

	// Send email
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	if err := smtp.SendMail(addr, auth, s.config.SMTPFrom, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("sending email to %s: %w", to, err)
	}

	return nil
}

// buildEmailBody creates the email body text from an alert.
func (s *AlertService) buildEmailBody(alert *Alert) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Alert Type: %s\n", alert.Type))
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", alert.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Subject: %s\n\n", alert.Subject))

	if alert.NodeID != "" {
		sb.WriteString(fmt.Sprintf("Node ID: %s\n", alert.NodeID))
	}
	if alert.NodeHostname != "" {
		sb.WriteString(fmt.Sprintf("Node Hostname: %s\n", alert.NodeHostname))
	}

	sb.WriteString(fmt.Sprintf("\nMessage:\n%s\n", alert.Message))

	if len(alert.Details) > 0 {
		sb.WriteString("\nDetails:\n")
		for k, v := range alert.Details {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}

	return sb.String()
}

// sendWebhookAlert sends a webhook notification to configured URLs.
func (s *AlertService) sendWebhookAlert(ctx context.Context, alert *Alert) error {
	if len(s.config.AdminWebhooks) == 0 {
		return nil
	}

	payload := map[string]interface{}{
		"type":         string(alert.Type),
		"subject":      alert.Subject,
		"message":      alert.Message,
		"timestamp":    alert.Timestamp.Format(time.RFC3339),
		"node_id":      alert.NodeID,
		"node_hostname": alert.NodeHostname,
		"details":      alert.Details,
	}

	var errors []string
	for _, webhookURL := range s.config.AdminWebhooks {
		if err := s.sendWebhook(ctx, webhookURL, "", payload); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", webhookURL, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("webhook send failures: %s", strings.Join(errors, "; "))
	}

	s.logger.Info("webhook alerts sent",
		"alert_type", alert.Type,
		"webhooks", len(s.config.AdminWebhooks))

	return nil
}

// sendWebhook sends a POST request to a webhook URL with the given payload.
// If secret is provided, adds an HMAC-SHA256 signature header.
func (s *AlertService) sendWebhook(ctx context.Context, url, secret string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "VirtueStack-Webhook/1.0")
	req.Header.Set("X-Event-Type", "alert")

	// Add HMAC signature if secret is provided
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Signature", "sha256="+signature)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// SendCustomerWebhook sends a webhook to a customer-configured endpoint.
// This is used for customer-specific notifications like VM events.
func (s *AlertService) SendCustomerWebhook(ctx context.Context, event string, customerID string, payload map[string]interface{}) error {
	if s.webhookRepo == nil {
		return fmt.Errorf("webhook repository not configured")
	}

	// Get active webhooks for this event
	webhooks, err := s.webhookRepo.ListActiveForEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("listing webhooks for event %s: %w", event, err)
	}

	// Add event type to payload
	payload["event"] = event
	payload["timestamp"] = time.Now().Format(time.RFC3339)

	var errors []string
	for _, webhook := range webhooks {
		if webhook.CustomerID != customerID {
			continue
		}

		if err := s.sendWebhook(ctx, webhook.URL, webhook.SecretHash, payload); err != nil {
			errors = append(errors, fmt.Sprintf("webhook %s: %v", webhook.ID, err))
			if updateErr := s.webhookRepo.UpdateDeliveryStatus(ctx, webhook.ID, false); updateErr != nil {
				s.logger.Error("failed to update webhook status",
					"webhook_id", webhook.ID,
					"error", updateErr)
			}
		} else {
			if updateErr := s.webhookRepo.UpdateDeliveryStatus(ctx, webhook.ID, true); updateErr != nil {
				s.logger.Error("failed to update webhook status",
					"webhook_id", webhook.ID,
					"error", updateErr)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("webhook failures: %s", strings.Join(errors, "; "))
	}

	return nil
}

// NotifyNodeFailure sends alerts for a node failure event.
func (s *AlertService) NotifyNodeFailure(ctx context.Context, nodeID, hostname string, affectedVMs int, ipmiConfigured bool) error {
	alert := &Alert{
		Type:         AlertTypeNodeFailure,
		Subject:      fmt.Sprintf("Node Failure: %s", hostname),
		Message:      fmt.Sprintf("Node %s has been marked as failed. %d VMs may be affected.", hostname, affectedVMs),
		NodeID:       nodeID,
		NodeHostname: hostname,
		Timestamp:    time.Now(),
		Details: map[string]interface{}{
			"affected_vms":    affectedVMs,
			"ipmi_configured": ipmiConfigured,
		},
	}
	return s.SendAlert(ctx, alert)
}

// NotifyIPMIAttempt sends alerts for IPMI power cycle attempts.
func (s *AlertService) NotifyIPMIAttempt(ctx context.Context, nodeID, hostname string, success bool, errMsg string) error {
	subject := fmt.Sprintf("IPMI Power Cycle Success: %s", hostname)
	message := fmt.Sprintf("IPMI power cycle succeeded for node %s", hostname)
	alertType := AlertTypeNodeRecovery

	if !success {
		subject = fmt.Sprintf("IPMI Power Cycle Failed: %s", hostname)
		message = fmt.Sprintf("IPMI power cycle failed for node %s: %s", hostname, errMsg)
		alertType = AlertTypeIPMIAttempt
	}

	alert := &Alert{
		Type:         alertType,
		Subject:      subject,
		Message:      message,
		NodeID:       nodeID,
		NodeHostname: hostname,
		Timestamp:    time.Now(),
		Details: map[string]interface{}{
			"success": success,
			"error":   errMsg,
		},
	}
	return s.SendAlert(ctx, alert)
}

// NotifyVMMigration sends alerts for VM migration events.
func (s *AlertService) NotifyVMMigration(ctx context.Context, nodeID, hostname string, migratedVMs, failedVMs int) error {
	alert := &Alert{
		Type:         AlertTypeVMMigration,
		Subject:      fmt.Sprintf("VM Migration: %s", hostname),
		Message:      fmt.Sprintf("Migrated %d VMs from failed node %s. %d migrations failed.", migratedVMs, hostname, failedVMs),
		NodeID:       nodeID,
		NodeHostname: hostname,
		Timestamp:    time.Now(),
		Details: map[string]interface{}{
			"migrated_vms": migratedVMs,
			"failed_vms":   failedVMs,
		},
	}
	return s.SendAlert(ctx, alert)
}