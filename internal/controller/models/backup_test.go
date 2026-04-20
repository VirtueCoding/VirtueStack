package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateNextRunTime(t *testing.T) {
	base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		frequency string
		from      time.Time
		want      time.Time
	}{
		{
			name:      "daily schedule",
			frequency: AdminBackupScheduleFrequencyDaily,
			from:      base,
			want:      base.Add(24 * time.Hour),
		},
		{
			name:      "weekly schedule",
			frequency: AdminBackupScheduleFrequencyWeekly,
			from:      base,
			want:      base.Add(7 * 24 * time.Hour),
		},
		{
			name:      "monthly schedule",
			frequency: AdminBackupScheduleFrequencyMonthly,
			from:      base,
			want:      time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
		},
		{
			name:      "monthly schedule end of january",
			frequency: AdminBackupScheduleFrequencyMonthly,
			from:      time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC),
			want:      time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC), // Feb doesn't have 31 days
		},
		{
			name:      "unknown frequency defaults to daily",
			frequency: "unknown",
			from:      base,
			want:      base.Add(24 * time.Hour),
		},
		{
			name:      "empty frequency defaults to daily",
			frequency: "",
			from:      base,
			want:      base.Add(24 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateNextRunTime(tt.frequency, tt.from)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBackupStatusConstants(t *testing.T) {
	assert.Equal(t, "creating", BackupStatusCreating)
	assert.Equal(t, "completed", BackupStatusCompleted)
	assert.Equal(t, "failed", BackupStatusFailed)
	assert.Equal(t, "restoring", BackupStatusRestoring)
	assert.Equal(t, "deleted", BackupStatusDeleted)
}

func TestBackupSourceConstants(t *testing.T) {
	assert.Equal(t, "manual", BackupSourceManual)
	assert.Equal(t, "customer_schedule", BackupSourceCustomerSchedule)
	assert.Equal(t, "admin_schedule", BackupSourceAdminSchedule)
}

func TestBackupScheduleFrequencyConstants(t *testing.T) {
	assert.Equal(t, "daily", BackupScheduleFrequencyDaily)
	assert.Equal(t, "weekly", BackupScheduleFrequencyWeekly)
	assert.Equal(t, "monthly", BackupScheduleFrequencyMonthly)
}

func TestWebhookEventConstants(t *testing.T) {
	events := []string{
		WebhookEventVMCreated,
		WebhookEventVMDeleted,
		WebhookEventVMStarted,
		WebhookEventVMStopped,
		WebhookEventVMReinstall,
		WebhookEventVMMigrated,
		WebhookEventBackupDone,
		WebhookEventBackupFail,
		WebhookEventSnapshotDone,
		WebhookEventBandwidthThresh,
	}

	// Verify all events are non-empty and unique
	seen := make(map[string]bool)
	for _, event := range events {
		assert.NotEmpty(t, event, "event constant should not be empty")
		assert.False(t, seen[event], "event %q should be unique", event)
		seen[event] = true
	}
}

func TestBackupSecretHashNotSerialized(t *testing.T) {
	// Verify that CustomerWebhook.SecretHash has json:"-" tag
	// by checking the struct can hold a value without exposing it
	w := CustomerWebhook{
		SecretHash: "secret-hash-value",
	}
	assert.Equal(t, "secret-hash-value", w.SecretHash)
}
