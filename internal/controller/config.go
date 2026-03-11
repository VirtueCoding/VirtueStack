// Package controller provides the VirtueStack Controller application.
package controller

import (
	"encoding/hex"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
)

// Config wraps ControllerConfig with parsed encryption key.
type Config struct {
	*config.ControllerConfig
	EncryptionKeyBytes []byte
}

// LoadConfig loads and validates the controller configuration.
// It parses the hex-encoded encryption key and validates it's 32 bytes.
func LoadConfig() (*Config, error) {
	cfg, err := config.LoadControllerConfig()
	if err != nil {
		return nil, fmt.Errorf("loading controller config: %w", err)
	}

	// Parse encryption key from hex string
	keyBytes, err := hex.DecodeString(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decoding encryption key: %w", err)
	}

	// Validate key is 32 bytes (AES-256)
	if len(keyBytes) != crypto.EncryptionKeySize {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d",
			crypto.EncryptionKeySize, len(keyBytes))
	}

	return &Config{
		ControllerConfig:   cfg,
		EncryptionKeyBytes: keyBytes,
	}, nil
}