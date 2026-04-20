package models

import (
	"encoding/json"
	"time"
)

// PreActionWebhook represents an admin-managed webhook called synchronously
// before specific actions (e.g., VM creation) for approval/rejection.
type PreActionWebhook struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	URL       string    `json:"url" db:"url"`
	Secret    string    `json:"-" db:"secret"`
	Events    []string  `json:"events" db:"events"`
	TimeoutMs int       `json:"timeout_ms" db:"timeout_ms"`
	FailOpen  bool      `json:"fail_open" db:"fail_open"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// PreActionWebhookCreateRequest is the request body for creating a pre-action webhook.
type PreActionWebhookCreateRequest struct {
	Name      string   `json:"name" validate:"required,min=1,max=255"`
	URL       string   `json:"url" validate:"required,url,max=2048"`
	Secret    string   `json:"secret" validate:"required,min=16,max=128"`
	Events    []string `json:"events" validate:"required,min=1,dive,required"`
	TimeoutMs *int     `json:"timeout_ms,omitempty" validate:"omitempty,min=500,max=30000"`
	FailOpen  *bool    `json:"fail_open,omitempty"`
	IsActive  *bool    `json:"is_active,omitempty"`
}

// MarshalJSON redacts the Secret field to prevent accidental exposure in logs or error responses.
func (r PreActionWebhookCreateRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name      string   `json:"name"`
		URL       string   `json:"url"`
		Secret    string   `json:"secret"`
		Events    []string `json:"events"`
		TimeoutMs *int     `json:"timeout_ms,omitempty"`
		FailOpen  *bool    `json:"fail_open,omitempty"`
		IsActive  *bool    `json:"is_active,omitempty"`
	}{Name: r.Name, URL: r.URL, Secret: "[REDACTED]", Events: r.Events, TimeoutMs: r.TimeoutMs, FailOpen: r.FailOpen, IsActive: r.IsActive})
}

// PreActionWebhookUpdateRequest is the request body for updating a pre-action webhook.
type PreActionWebhookUpdateRequest struct {
	Name      *string  `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	URL       *string  `json:"url,omitempty" validate:"omitempty,url,max=2048"`
	Secret    *string  `json:"secret,omitempty" validate:"omitempty,min=16,max=128"`
	Events    []string `json:"events,omitempty" validate:"omitempty,min=1,dive,required"`
	TimeoutMs *int     `json:"timeout_ms,omitempty" validate:"omitempty,min=500,max=30000"`
	FailOpen  *bool    `json:"fail_open,omitempty"`
	IsActive  *bool    `json:"is_active,omitempty"`
}

// MarshalJSON redacts the Secret field to prevent accidental exposure in logs or error responses.
func (r PreActionWebhookUpdateRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name      *string  `json:"name,omitempty"`
		URL       *string  `json:"url,omitempty"`
		Secret    string   `json:"secret,omitempty"`
		Events    []string `json:"events,omitempty"`
		TimeoutMs *int     `json:"timeout_ms,omitempty"`
		FailOpen  *bool    `json:"fail_open,omitempty"`
		IsActive  *bool    `json:"is_active,omitempty"`
	}{Name: r.Name, URL: r.URL, Secret: "[REDACTED]", Events: r.Events, TimeoutMs: r.TimeoutMs, FailOpen: r.FailOpen, IsActive: r.IsActive})
}

// Pre-action webhook event constants.
const (
	PreActionEventVMCreate = "vm.create"
)
