package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidWebhookEvents_AllPresent(t *testing.T) {
	expectedEvents := []string{
		"vm.created",
		"vm.deleted",
		"vm.started",
		"vm.stopped",
		"vm.reinstalled",
		"vm.migrated",
		"backup.completed",
		"backup.failed",
		"snapshot.created",
		"bandwidth.threshold",
	}

	assert.Equal(t, len(expectedEvents), len(ValidWebhookEvents))

	for _, event := range expectedEvents {
		assert.True(t, ValidWebhookEvents[event], "expected event %q to be present", event)
	}
}

func TestValidWebhookEvents_UnknownEventAbsent(t *testing.T) {
	assert.False(t, ValidWebhookEvents["unknown.event"])
}

func TestMaxWebhooksPerCustomer(t *testing.T) {
	assert.Equal(t, 5, MaxWebhooksPerCustomer)
}

func TestWebhookErrorVariables(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"ErrInvalidURL", ErrInvalidURL, "webhook URL must be HTTPS"},
		{"ErrInvalidEvent", ErrInvalidEvent, "invalid webhook event"},
		{"ErrTooManyWebhooks", ErrTooManyWebhooks, "maximum webhook limit reached"},
		{"ErrWebhookNotFound", ErrWebhookNotFound, "webhook not found"},
		{"ErrSecretTooShort", ErrSecretTooShort, "secret must be at least 16 characters"},
		{"ErrSecretTooLong", ErrSecretTooLong, "secret must be at most 128 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.err)
			assert.Equal(t, tt.wantMsg, tt.err.Error())
		})
	}
}

func TestSetSkipURLValidation_PanicsInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	svc := &WebhookService{}
	assert.Panics(t, func() {
		svc.SetSkipURLValidation(true)
	})
}

func TestSetSkipURLValidation_NoPanicOutsideProduction(t *testing.T) {
	t.Setenv("APP_ENV", "test")

	svc := &WebhookService{}
	assert.NotPanics(t, func() {
		svc.SetSkipURLValidation(true)
	})
}
