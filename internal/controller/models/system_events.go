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

type SystemWebhookCreateRequest struct {
	Name     string   `json:"name" validate:"required,min=1,max=255"`
	URL      string   `json:"url" validate:"required,url,max=2048"`
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
	URL      *string  `json:"url,omitempty" validate:"omitempty,url,max=2048"`
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
