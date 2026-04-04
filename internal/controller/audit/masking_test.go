package audit

import (
	"testing"
)

func TestMaskSensitiveFields(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "password field",
			input: map[string]any{
				"username": "john",
				"password": "secret123",
			},
			expected: map[string]any{
				"username": "john",
				"password": "[REDACTED]",
			},
		},
		{
			name: "api_key field",
			input: map[string]any{
				"name":    "my-key",
				"api_key": "vs_abc123",
			},
			expected: map[string]any{
				"name":    "my-key",
				"api_key": "[REDACTED]",
			},
		},
		{
			name: "token field",
			input: map[string]any{
				"user_id": "123",
				"token":   "Bearer xyz",
			},
			expected: map[string]any{
				"user_id": "123",
				"token":   "[REDACTED]",
			},
		},
		{
			name: "nested map",
			input: map[string]any{
				"user": map[string]any{
					"name":     "john",
					"password": "secret",
				},
			},
			expected: map[string]any{
				"user": map[string]any{
					"name":     "john",
					"password": "[REDACTED]",
				},
			},
		},
		{
			name: "nested arrays",
			input: map[string]any{
				"steps": []any{
					map[string]any{
						"name":   "first",
						"secret": "top-secret",
					},
					map[string]any{
						"token": "abc123",
					},
				},
			},
			expected: map[string]any{
				"steps": []any{
					map[string]any{
						"name":   "first",
						"secret": "[REDACTED]",
					},
					map[string]any{
						"token": "[REDACTED]",
					},
				},
			},
		},
		{
			name: "case insensitive matching",
			input: map[string]any{
				"PASSWORD": "secret",
				"Api_Key":  "key123",
			},
			expected: map[string]any{
				"PASSWORD": "[REDACTED]",
				"Api_Key":  "[REDACTED]",
			},
		},
		{
			name: "partial match in field name",
			input: map[string]any{
				"root_password_encrypted": "encrypted_value",
			},
			expected: map[string]any{
				"root_password_encrypted": "[REDACTED]",
			},
		},
		{
			name: "url fields are sanitized",
			input: map[string]any{
				"url":          "https://user:pass@example.com/webhook/path?token=abc#frag",
				"callback_url": "https://api.example.com:8443/callback?signature=123",
			},
			expected: map[string]any{
				"url":          "https://example.com",
				"callback_url": "https://api.example.com:8443",
			},
		},
		{
			name: "non-sensitive fields unchanged",
			input: map[string]any{
				"name":  "test",
				"email": "test@example.com",
				"count": 42,
			},
			expected: map[string]any{
				"name":  "test",
				"email": "test@example.com",
				"count": 42,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskSensitiveFields(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			compareMaps(t, tt.expected, result)
		})
	}
}

func compareMaps(t *testing.T, expected, actual map[string]any) {
	if len(expected) != len(actual) {
		t.Errorf("map length mismatch: expected %d, got %d", len(expected), len(actual))
		return
	}

	for key, expectedVal := range expected {
		actualVal, ok := actual[key]
		if !ok {
			t.Errorf("missing key %q", key)
			return
		}

		switch e := expectedVal.(type) {
		case map[string]any:
			a, ok := actualVal.(map[string]any)
			if !ok {
				t.Errorf("key %q: expected map[string]any, got %T", key, actualVal)
				return
			}
			compareMaps(t, e, a)
		case []any:
			a, ok := actualVal.([]any)
			if !ok {
				t.Errorf("key %q: expected []any, got %T", key, actualVal)
				return
			}
			compareSlices(t, e, a)
		default:
			if expectedVal != actualVal {
				t.Errorf("key %q: expected %v, got %v", key, expectedVal, actualVal)
			}
		}
	}
}

func compareSlices(t *testing.T, expected, actual []any) {
	if len(expected) != len(actual) {
		t.Errorf("slice length mismatch: expected %d, got %d", len(expected), len(actual))
		return
	}

	for i, expectedVal := range expected {
		actualVal := actual[i]

		switch e := expectedVal.(type) {
		case map[string]any:
			a, ok := actualVal.(map[string]any)
			if !ok {
				t.Errorf("index %d: expected map[string]any, got %T", i, actualVal)
				return
			}
			compareMaps(t, e, a)
		case []any:
			a, ok := actualVal.([]any)
			if !ok {
				t.Errorf("index %d: expected []any, got %T", i, actualVal)
				return
			}
			compareSlices(t, e, a)
		default:
			if expectedVal != actualVal {
				t.Errorf("index %d: expected %v, got %v", i, expectedVal, actualVal)
			}
		}
	}
}

func TestIsSensitiveField(t *testing.T) {
	tests := []struct {
		field     string
		sensitive bool
	}{
		{"password", true},
		{"PASSWORD", true},
		{"api_key", true},
		{"token", true},
		{"secret", true},
		{"private_key", true},
		{"totp_secret", true},
		{"backup_code", true},
		{"encryption_key", true},
		{"ipmi_password", true},
		{"root_password", true},
		{"name", false},
		{"email", false},
		{"hostname", false},
		{"customer_id", false},
		{"vm_id", false},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			result := isSensitiveField(tt.field)
			if result != tt.sensitive {
				t.Errorf("isSensitiveField(%q) = %v, want %v", tt.field, result, tt.sensitive)
			}
		})
	}
}
