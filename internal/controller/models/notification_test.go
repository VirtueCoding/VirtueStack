package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationPreferences_ToResponse(t *testing.T) {
	created := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 3, 20, 14, 30, 0, 0, time.UTC)

	prefs := &NotificationPreferences{
		ID:              "pref-123",
		CustomerID:      "cust-456",
		EmailEnabled:    true,
		TelegramEnabled: false,
		Events:          []string{"vm.started", "vm.stopped"},
		CreatedAt:       created,
		UpdatedAt:       updated,
	}

	resp := prefs.ToResponse()

	assert.Equal(t, "pref-123", resp.ID)
	assert.True(t, resp.EmailEnabled)
	assert.False(t, resp.TelegramEnabled)
	assert.Equal(t, []string{"vm.started", "vm.stopped"}, resp.Events)
	assert.Equal(t, "2026-01-15T10:00:00Z", resp.CreatedAt)
	assert.Equal(t, "2026-03-20T14:30:00Z", resp.UpdatedAt)
}

func TestNotificationPreferences_ToResponse_EmptyEvents(t *testing.T) {
	prefs := &NotificationPreferences{
		ID:              "pref-123",
		EmailEnabled:    false,
		TelegramEnabled: false,
		Events:          nil,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	resp := prefs.ToResponse()
	assert.Nil(t, resp.Events)
}

func TestNotificationEvent_ToResponse_ValidJSON(t *testing.T) {
	created := time.Date(2026, 2, 10, 8, 0, 0, 0, time.UTC)

	event := &NotificationEvent{
		ID:           "evt-123",
		CustomerID:   "cust-456",
		EventType:    "vm.started",
		ResourceType: "vm",
		ResourceID:   "vm-789",
		Data:         json.RawMessage(`{"hostname":"test-vm"}`),
		Status:       "sent",
		CreatedAt:    created,
	}

	resp := event.ToResponse()

	assert.Equal(t, "evt-123", resp.ID)
	assert.Equal(t, "vm.started", resp.EventType)
	assert.Equal(t, "vm", resp.ResourceType)
	assert.Equal(t, "vm-789", resp.ResourceID)
	assert.Equal(t, "sent", resp.Status)
	assert.Equal(t, "2026-02-10T08:00:00Z", resp.CreatedAt)

	// Verify JSON data is passed through
	var data map[string]string
	err := json.Unmarshal(resp.Data, &data)
	require.NoError(t, err)
	assert.Equal(t, "test-vm", data["hostname"])
}

func TestNotificationEvent_ToResponse_InvalidJSON(t *testing.T) {
	event := &NotificationEvent{
		ID:        "evt-123",
		Data:      json.RawMessage(`{invalid json`),
		CreatedAt: time.Now(),
	}

	resp := event.ToResponse()

	// Should fall back to empty JSON object for invalid data
	assert.Equal(t, json.RawMessage(`{}`), resp.Data)
}

func TestNotificationEvent_ToResponse_EmptyData(t *testing.T) {
	event := &NotificationEvent{
		ID:        "evt-123",
		Data:      nil,
		CreatedAt: time.Now(),
	}

	resp := event.ToResponse()
	assert.Nil(t, resp.Data)
}

func TestNotificationEvent_ToResponse_EmptySliceData(t *testing.T) {
	event := &NotificationEvent{
		ID:        "evt-123",
		Data:      json.RawMessage{},
		CreatedAt: time.Now(),
	}

	resp := event.ToResponse()
	assert.Nil(t, resp.Data)
}
