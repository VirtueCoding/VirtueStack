// Package audit provides utilities for audit logging with sensitive data masking.
package audit

import (
	"net/url"
	"strings"
)

// sensitiveFieldPatterns defines field names that should be masked in audit logs.
// Patterns are matched case-insensitively.
var sensitiveFieldPatterns = []string{
	"password",
	"passwd",
	"secret",
	"token",
	"api_key",
	"apikey",
	"private_key",
	"privatekey",
	"totp_secret",
	"backup_code",
	"encryption_key",
	"ipmi_password",
	"root_password",
}

// MaskSensitiveFields recursively masks sensitive fields in a map.
// Fields matching sensitiveFieldPatterns are replaced with "[REDACTED]".
// This prevents sensitive data from being logged in audit_logs table.
func MaskSensitiveFields(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}

	masked, ok := maskValue(data).(map[string]any)
	if !ok {
		return nil
	}
	return masked
}

// isSensitiveField checks if a field name matches a sensitive pattern.
// The comparison is case-insensitive.
func isSensitiveField(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	for _, pattern := range sensitiveFieldPatterns {
		if strings.Contains(lowerField, pattern) {
			return true
		}
	}
	return false
}

func isURLField(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	return lowerField == "url" || strings.HasSuffix(lowerField, "_url")
}

// SanitizeURLForAudit removes credentials, path, query, and fragment from URLs
// before they are persisted in logs or audit changes.
func SanitizeURLForAudit(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}

	return parsed.Scheme + "://" + parsed.Host
}

func maskValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, nestedValue := range v {
			if isSensitiveField(key) {
				result[key] = "[REDACTED]"
				continue
			}
			if isURLField(key) {
				if rawURL, ok := nestedValue.(string); ok {
					result[key] = SanitizeURLForAudit(rawURL)
					continue
				}
			}
			result[key] = maskValue(nestedValue)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = maskValue(item)
		}
		return result
	default:
		return value
	}
}
