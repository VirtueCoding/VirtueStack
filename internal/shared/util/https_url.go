package util

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateHTTPSURL validates that a URL is parseable, uses HTTPS, and includes a host.
func ValidateHTTPSURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("URL must use HTTPS")
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL must include host")
	}
	return nil
}
