package config

import (
	"encoding/json"
	"testing"
)

func TestSecret_String(t *testing.T) {
	t.Run("non-empty secret returns REDACTED", func(t *testing.T) {
		s := Secret("super-secret-value")
		got := s.String()
		if got != "[REDACTED]" {
			t.Errorf("String() = %q, want %q", got, "[REDACTED]")
		}
	})

	t.Run("empty secret returns REDACTED", func(t *testing.T) {
		s := Secret("")
		got := s.String()
		if got != "[REDACTED]" {
			t.Errorf("String() = %q, want %q", got, "[REDACTED]")
		}
	})
}

func TestSecret_Value(t *testing.T) {
	t.Run("returns underlying string", func(t *testing.T) {
		s := Secret("my-actual-secret")
		got := s.Value()
		if got != "my-actual-secret" {
			t.Errorf("Value() = %q, want %q", got, "my-actual-secret")
		}
	})

	t.Run("empty secret returns empty string", func(t *testing.T) {
		s := Secret("")
		got := s.Value()
		if got != "" {
			t.Errorf("Value() = %q, want %q", got, "")
		}
	})
}

func TestSecret_MarshalJSON(t *testing.T) {
	t.Run("returns JSON-encoded REDACTED", func(t *testing.T) {
		s := Secret("do-not-expose")
		b, err := s.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		// Raw JSON bytes should be: "\"[REDACTED]\""
		want := `"[REDACTED]"`
		if string(b) != want {
			t.Errorf("MarshalJSON() = %s, want %s", string(b), want)
		}
	})

	t.Run("empty secret also returns REDACTED JSON", func(t *testing.T) {
		s := Secret("")
		b, err := s.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON() error = %v", err)
		}
		want := `"[REDACTED]"`
		if string(b) != want {
			t.Errorf("MarshalJSON() = %s, want %s", string(b), want)
		}
	})
}

func TestSecret_StructMarshalJSON(t *testing.T) {
	type Config struct {
		Name   string `json:"name"`
		Token  Secret `json:"token"`
		APIKey Secret `json:"api_key"`
	}

	cfg := Config{
		Name:   "test-config",
		Token:  Secret("real-token-value"),
		APIKey: Secret("real-api-key"),
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Ensure the actual secret values do not appear in JSON output.
	jsonStr := string(data)
	if contains(jsonStr, "real-token-value") {
		t.Errorf("JSON output contains secret token value: %s", jsonStr)
	}
	if contains(jsonStr, "real-api-key") {
		t.Errorf("JSON output contains secret api_key value: %s", jsonStr)
	}

	// Unmarshal to verify the field values are "[REDACTED]".
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if raw["token"] != "[REDACTED]" {
		t.Errorf("token field = %q, want %q", raw["token"], "[REDACTED]")
	}
	if raw["api_key"] != "[REDACTED]" {
		t.Errorf("api_key field = %q, want %q", raw["api_key"], "[REDACTED]")
	}
	if raw["name"] != "test-config" {
		t.Errorf("name field = %q, want %q", raw["name"], "test-config")
	}
}

// contains is a helper to check substring presence without importing strings.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
