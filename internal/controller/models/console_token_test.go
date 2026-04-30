package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConsoleTypeConstants(t *testing.T) {
	assert.Equal(t, "vnc", ConsoleTypeVNC)
	assert.Equal(t, "serial", ConsoleTypeSerial)
}

func TestConsoleTokenTTL(t *testing.T) {
	assert.Equal(t, 5*time.Minute, ConsoleTokenTTL)
}

func TestConsoleToken_Fields(t *testing.T) {
	now := time.Now()
	token := ConsoleToken{
		ID:          "token-123",
		TokenHash:   []byte("sha256-hash"),
		UserID:      "user-456",
		UserType:    "customer",
		VMID:        "vm-789",
		ConsoleType: ConsoleTypeVNC,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ConsoleTokenTTL),
	}

	assert.Equal(t, "token-123", token.ID)
	assert.Equal(t, "customer", token.UserType)
	assert.Equal(t, ConsoleTypeVNC, token.ConsoleType)
	assert.True(t, token.ExpiresAt.After(token.CreatedAt))
}
