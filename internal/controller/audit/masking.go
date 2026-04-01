// Package audit provides utilities for audit logging with sensitive data masking.
package audit

import (
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

	result := make(map[string]any, len(data))
	for key, value := range data {
		lowerKey := strings.ToLower(key)

		// Check if this is a sensitive field
		if isSensitiveField(lowerKey) {
			result[key] = "[REDACTED]"
			continue
		}

		// Recursively mask nested maps
		switch v := value.(type) {
		case map[string]any:
			result[key] = MaskSensitiveFields(v)
		default:
			result[key] = value
		}
	}

	return result
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