package models

import "time"

const (
	ConsoleTypeVNC    = "vnc"
	ConsoleTypeSerial = "serial"
	ConsoleTokenTTL   = 5 * time.Minute
)

// ConsoleToken represents a short-lived, single-use token for WebSocket console authentication.
type ConsoleToken struct {
	ID          string    `json:"id"`
	TokenHash   []byte    `json:"-"` // SHA-256 hash, never serialized
	UserID      string    `json:"user_id"`
	UserType    string    `json:"user_type"`    // "customer" or "admin"
	VMID        string    `json:"vm_id"`
	ConsoleType string    `json:"console_type"` // "vnc" or "serial"
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}