package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"EventVMCreated", EventVMCreated, "vm.created"},
		{"EventVMDeleted", EventVMDeleted, "vm.deleted"},
		{"EventVMSuspended", EventVMSuspended, "vm.suspended"},
		{"EventBackupFailed", EventBackupFailed, "backup.failed"},
		{"EventNodeOffline", EventNodeOffline, "node.offline"},
		{"EventBandwidthExceeded", EventBandwidthExceeded, "bandwidth.exceeded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

func TestAllEventTypes_Count(t *testing.T) {
	assert.Len(t, AllEventTypes, 6)
}

func TestAllEventTypes_ContainsAllConstants(t *testing.T) {
	expected := []string{
		EventVMCreated,
		EventVMDeleted,
		EventVMSuspended,
		EventBackupFailed,
		EventNodeOffline,
		EventBandwidthExceeded,
	}

	for _, e := range expected {
		assert.Contains(t, AllEventTypes, e)
	}
}

func TestIsValidEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		want      bool
	}{
		{"vm.created is valid", "vm.created", true},
		{"vm.deleted is valid", "vm.deleted", true},
		{"vm.suspended is valid", "vm.suspended", true},
		{"backup.failed is valid", "backup.failed", true},
		{"node.offline is valid", "node.offline", true},
		{"bandwidth.exceeded is valid", "bandwidth.exceeded", true},
		{"unknown is invalid", "unknown", false},
		{"empty is invalid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidEventType(tt.eventType))
		})
	}
}

func TestNotificationPayload_JSONRoundTrip(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	original := &NotificationPayload{
		EventType:     EventVMCreated,
		Timestamp:     ts,
		CustomerID:    "cust-123",
		CustomerEmail: "test@example.com",
		CustomerName:  "Test User",
		ResourceID:    "vm-456",
		ResourceType:  "vm",
		Data:          map[string]any{"hostname": "web-01"},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded NotificationPayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.EventType, decoded.EventType)
	assert.True(t, original.Timestamp.Equal(decoded.Timestamp))
	assert.Equal(t, original.CustomerID, decoded.CustomerID)
	assert.Equal(t, original.CustomerEmail, decoded.CustomerEmail)
	assert.Equal(t, original.CustomerName, decoded.CustomerName)
	assert.Equal(t, original.ResourceID, decoded.ResourceID)
	assert.Equal(t, original.ResourceType, decoded.ResourceType)
	assert.Equal(t, "web-01", decoded.Data["hostname"])
}

func TestNotificationConfig_ZeroValue(t *testing.T) {
	var cfg NotificationConfig
	assert.False(t, cfg.EmailEnabled)
	assert.False(t, cfg.TelegramEnabled)
}
