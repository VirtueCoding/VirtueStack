// Package nodeagent provides the VirtueStack Node Agent application.
// It runs on bare-metal servers and manages VMs via libvirt.
package nodeagent

import (
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/google/uuid"
)

// LoadConfig loads and validates the Node Agent configuration.
// It delegates to the shared config package and performs additional validation.
func LoadConfig() (*config.NodeAgentConfig, error) {
	cfg, err := config.LoadNodeAgentConfig()
	if err != nil {
		return nil, fmt.Errorf("loading node agent config: %w", err)
	}

	// Validate NodeID is a valid UUID format
	if _, err := uuid.Parse(cfg.NodeID); err != nil {
		return nil, fmt.Errorf("invalid NodeID format: %w", err)
	}

	return cfg, nil
}