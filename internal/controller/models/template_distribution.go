// Package models provides data model types and shared template helpers for VirtueStack Controller.
package models

import (
	"fmt"
	"net/url"
)

// ValidateTemplateDistributionSourceURL validates that a template distribution source
// is a controller-accessible HTTP(S) URL.
func ValidateTemplateDistributionSourceURL(sourceURL string) error {
	if sourceURL == "" {
		return fmt.Errorf("template distribution requires a controller-accessible HTTP(S) source URL")
	}

	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("template distribution source URL is invalid: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("template distribution currently supports only HTTP(S) template sources")
	}
	if parsed.Host == "" {
		return fmt.Errorf("template distribution source URL must include a host")
	}

	return nil
}
