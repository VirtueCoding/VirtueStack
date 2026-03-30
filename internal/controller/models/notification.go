// Package models provides data model types for VirtueStack Controller.
package models

import (
	"encoding/json"
	"log/slog"
	"time"
)

// NotificationPreferences represents a customer's notification preferences.
type NotificationPreferences struct {
	ID             string    `json:"id" db:"id"`
	CustomerID     string    `json:"customer_id" db:"customer_id"`
	EmailEnabled   bool      `json:"email_enabled" db:"email_enabled"`
	TelegramEnabled bool     `json:"telegram_enabled" db:"telegram_enabled"`
	Events         []string  `json:"events" db:"events"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// NotificationPreferencesRequest represents the request body for updating preferences.
type NotificationPreferencesRequest struct {
	EmailEnabled    *bool    `json:"email_enabled,omitempty"`
	TelegramEnabled *bool    `json:"telegram_enabled,omitempty"`
	Events          []string `json:"events,omitempty" validate:"omitempty,max=50,dive,required,max=100"`
}

// NotificationPreferencesResponse represents the response for preferences.
type NotificationPreferencesResponse struct {
	ID              string   `json:"id"`
	EmailEnabled    bool     `json:"email_enabled"`
	TelegramEnabled bool     `json:"telegram_enabled"`
	Events          []string `json:"events"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// ToResponse converts NotificationPreferences to a response.
func (p *NotificationPreferences) ToResponse() *NotificationPreferencesResponse {
	return &NotificationPreferencesResponse{
		ID:              p.ID,
		EmailEnabled:    p.EmailEnabled,
		TelegramEnabled: p.TelegramEnabled,
		Events:          p.Events,
		CreatedAt:       p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       p.UpdatedAt.Format(time.RFC3339),
	}
}

// NotificationEvent represents a notification event log entry.
type NotificationEvent struct {
	ID           string          `json:"id" db:"id"`
	CustomerID   string          `json:"customer_id" db:"customer_id"`
	EventType    string          `json:"event_type" db:"event_type"`
	ResourceType string          `json:"resource_type" db:"resource_type"`
	ResourceID   string          `json:"resource_id" db:"resource_id"`
	Data         json.RawMessage `json:"data" db:"data"`
	Status       string          `json:"status" db:"status"`
	Error        *string         `json:"error,omitempty" db:"error"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// NotificationEventResponse represents a notification event in responses.
type NotificationEventResponse struct {
	ID           string          `json:"id"`
	EventType    string          `json:"event_type"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Data         json.RawMessage `json:"data,omitempty"`
	Status       string          `json:"status"`
	CreatedAt    string          `json:"created_at"`
}

// ToResponse converts NotificationEvent to a response.
func (e *NotificationEvent) ToResponse() *NotificationEventResponse {
	var data json.RawMessage
	if len(e.Data) > 0 {
		// Validate that the stored JSON is well-formed before passing it through.
		if !json.Valid(e.Data) {
			slog.Warn("notification event contains invalid JSON in Data field",
				"event_id", e.ID, "event_type", e.EventType)
			data = json.RawMessage(`{}`)
		} else {
			data = e.Data
		}
	}
	return &NotificationEventResponse{
		ID:           e.ID,
		EventType:    e.EventType,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		Data:         data,
		Status:       e.Status,
		CreatedAt:    e.CreatedAt.Format(time.RFC3339),
	}
}