// Package models provides data model types for VirtueStack Controller.
package models

import (
	"encoding/json"
	"time"
)

const (
	SystemEventNodeOffline       = "system.node.offline"
	SystemEventNodeOnline        = "system.node.online"
	SystemEventNodeDegraded      = "system.node.degraded"
	SystemEventFailoverTriggered = "system.failover.triggered"
	SystemEventFailoverCompleted = "system.failover.completed"
	SystemEventStorageWarning    = "system.storage.warning"
	SystemEventStorageCritical   = "system.storage.critical"
)

// SystemWebhook defines an admin-managed webhook endpoint for platform-wide events.
type SystemWebhook struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	URL       string    `json:"url" db:"url"`
	Secret    string    `json:"-" db:"secret"`
	Events    []string  `json:"events" db:"events"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// SystemWebhookRequestBody is the canonical outbound payload sent to system
// webhook receivers and persisted with delivery records for retry safety.
type SystemWebhookRequestBody struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

// MarshalSystemWebhookRequestBody renders the canonical request body used for
// system webhook delivery records and outbound HTTP requests.
func MarshalSystemWebhookRequestBody(event string, data map[string]any) ([]byte, error) {
	return json.Marshal(SystemWebhookRequestBody{
		Event: event,
		Data:  data,
	})
}

// SystemWebhookDelivery represents a single delivery attempt lifecycle for a
// system webhook event.
type SystemWebhookDelivery struct {
	ID              string     `json:"id" db:"id"`
	SystemWebhookID string     `json:"system_webhook_id" db:"system_webhook_id"`
	Event           string     `json:"event" db:"event"`
	IdempotencyKey  string     `json:"idempotency_key" db:"idempotency_key"`
	Payload         []byte     `json:"payload" db:"payload"`
	Status          string     `json:"status" db:"status"`
	AttemptCount    int        `json:"attempt_count" db:"attempt_count"`
	MaxAttempts     int        `json:"max_attempts" db:"max_attempts"`
	NextRetryAt     *time.Time `json:"next_retry_at,omitempty" db:"next_retry_at"`
	ResponseStatus  *int       `json:"response_status,omitempty" db:"response_status"`
	ResponseBody    *string    `json:"response_body,omitempty" db:"response_body"`
	ErrorMessage    *string    `json:"error_message,omitempty" db:"error_message"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type SystemWebhookCreateRequest struct {
	Name     string   `json:"name" validate:"required,min=1,max=255"`
	URL      string   `json:"url" validate:"required,https_url,max=2048"`
	Secret   string   `json:"secret" validate:"required,min=16,max=128"`
	Events   []string `json:"events" validate:"required,min=1,dive,required"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// MarshalJSON redacts the Secret field to prevent accidental exposure in logs or error responses.
func (r SystemWebhookCreateRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string   `json:"name"`
		URL      string   `json:"url"`
		Secret   string   `json:"secret"`
		Events   []string `json:"events"`
		IsActive *bool    `json:"is_active,omitempty"`
	}{Name: r.Name, URL: r.URL, Secret: "[REDACTED]", Events: r.Events, IsActive: r.IsActive})
}

type SystemWebhookUpdateRequest struct {
	Name     *string  `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	URL      *string  `json:"url,omitempty" validate:"omitempty,https_url,max=2048"`
	Secret   *string  `json:"secret,omitempty" validate:"omitempty,min=16,max=128"`
	Events   []string `json:"events,omitempty" validate:"omitempty,min=1,dive,required"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// MarshalJSON redacts the Secret field to prevent accidental exposure in logs or error responses.
func (r SystemWebhookUpdateRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     *string  `json:"name,omitempty"`
		URL      *string  `json:"url,omitempty"`
		Secret   string   `json:"secret,omitempty"`
		Events   []string `json:"events,omitempty"`
		IsActive *bool    `json:"is_active,omitempty"`
	}{Name: r.Name, URL: r.URL, Secret: "[REDACTED]", Events: r.Events, IsActive: r.IsActive})
}
