package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSSOTokenTTL(t *testing.T) {
	assert.Equal(t, 5*time.Minute, SSOTokenTTL)
}

func TestSSOToken_Fields(t *testing.T) {
	now := time.Now()
	token := SSOToken{
		ID:           "sso-123",
		TokenHash:    []byte("sha256-hash"),
		CustomerID:   "cust-456",
		VMID:         "vm-789",
		RedirectPath: "/vms/vm-789",
		CreatedAt:    now,
		ExpiresAt:    now.Add(SSOTokenTTL),
	}

	assert.Equal(t, "sso-123", token.ID)
	assert.Equal(t, "cust-456", token.CustomerID)
	assert.Equal(t, "vm-789", token.VMID)
	assert.Equal(t, "/vms/vm-789", token.RedirectPath)
	assert.True(t, token.ExpiresAt.After(token.CreatedAt))
}
