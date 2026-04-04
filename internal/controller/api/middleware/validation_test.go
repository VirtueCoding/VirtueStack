package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestContext creates a gin test context with the given method, path, and body.
func createTestContext(method, path, body string) (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return w, c
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid UUID v4",
			id:      "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "another valid UUID v4",
			id:      "6ba7b810-9dad-41d4-80b4-00c04fd430c8",
			wantErr: false,
		},
		{
			name:    "empty string",
			id:      "",
			wantErr: true,
		},
		{
			name:    "not a UUID",
			id:      "not-a-uuid",
			wantErr: true,
		},
		{
			name:    "UUID v1 rejected",
			id:      "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			wantErr: true,
		},
		{
			name:    "too short",
			id:      "550e8400-e29b-41d4",
			wantErr: true,
		},
		{
			name:    "SQL injection attempt",
			id:      "'; DROP TABLE vms; --",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUUID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
				var validErr *sharederrors.ValidationError
				assert.ErrorAs(t, err, &validErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple hostname",
			input:   "web-server-01",
			wantErr: false,
		},
		{
			name:    "FQDN",
			input:   "web.example.com",
			wantErr: false,
		},
		{
			name:    "single label",
			input:   "localhost",
			wantErr: false,
		},
		{
			name:    "numeric hostname",
			input:   "server123",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "starts with hyphen",
			input:   "-invalid",
			wantErr: true,
		},
		{
			name:    "ends with hyphen",
			input:   "invalid-",
			wantErr: true,
		},
		{
			name:    "contains underscore",
			input:   "invalid_hostname",
			wantErr: true,
		},
		{
			name:    "contains space",
			input:   "invalid hostname",
			wantErr: true,
		},
		{
			name:    "non-ASCII characters",
			input:   "sérver",
			wantErr: true,
		},
		{
			name:    "too long (254 chars)",
			input:   string(make([]byte, 254)),
			wantErr: true,
		},
		{
			name:    "exactly 253 chars",
			input:   generateHostname(253),
			wantErr: false,
		},
		{
			name:    "contains dots - valid multi-label",
			input:   "a.b.c.d",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// generateHostname creates a valid hostname of approximately the given length.
func generateHostname(length int) string {
	if length <= 0 {
		return ""
	}

	result := make([]byte, 0, length)
	labelLen := 0
	for len(result) < length {
		if labelLen > 0 && labelLen%62 == 0 && len(result) < length-1 {
			result = append(result, '.')
			labelLen = 0
			continue
		}
		result = append(result, 'a')
		labelLen++
	}
	return string(result)
}

func TestBindAndValidate(t *testing.T) {
	// Test the validation error conversion with a struct
	type testReq struct {
		Name  string `json:"name" validate:"required,min=2"`
		Email string `json:"email" validate:"required,email"`
	}

	t.Run("invalid JSON returns parse error", func(t *testing.T) {
		w, c := createTestContext("POST", "/test", `{invalid json`)
		_ = w

		err := BindAndValidate(c, &testReq{})
		require.Error(t, err)
		var apiErr *sharederrors.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "INVALID_REQUEST_BODY", apiErr.Code)
	})

	t.Run("valid JSON but fails validation", func(t *testing.T) {
		w, c := createTestContext("POST", "/test", `{"name":"a","email":"invalid"}`)
		_ = w

		err := BindAndValidate(c, &testReq{})
		require.Error(t, err)
		var apiErr *sharederrors.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "VALIDATION_ERROR", apiErr.Code)
		assert.NotEmpty(t, apiErr.Details)
	})

	t.Run("valid request passes", func(t *testing.T) {
		w, c := createTestContext("POST", "/test", `{"name":"John","email":"john@example.com"}`)
		_ = w

		var req testReq
		err := BindAndValidate(c, &req)
		assert.NoError(t, err)
		assert.Equal(t, "John", req.Name)
		assert.Equal(t, "john@example.com", req.Email)
	})
}

func TestBindOptionalJSON(t *testing.T) {
	type testReq struct {
		Name string `json:"name"`
	}

	t.Run("empty body is allowed", func(t *testing.T) {
		w, c := createTestContext("POST", "/test", "")
		_ = w

		var req testReq
		err := BindOptionalJSON(c, &req)
		require.NoError(t, err)
		assert.Empty(t, req.Name)
	})

	t.Run("whitespace body is allowed", func(t *testing.T) {
		w, c := createTestContext("POST", "/test", " \n\t ")
		_ = w

		var req testReq
		err := BindOptionalJSON(c, &req)
		require.NoError(t, err)
		assert.Empty(t, req.Name)
	})

	t.Run("malformed JSON returns parse error", func(t *testing.T) {
		w, c := createTestContext("POST", "/test", `{"name":`)
		_ = w

		err := BindOptionalJSON(c, &testReq{})
		require.Error(t, err)
		var apiErr *sharederrors.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "INVALID_REQUEST_BODY", apiErr.Code)
	})

	t.Run("valid JSON binds and preserves body for reread", func(t *testing.T) {
		w, c := createTestContext("POST", "/test", `{"name":"John"}`)
		_ = w

		var req testReq
		err := BindOptionalJSON(c, &req)
		require.NoError(t, err)
		assert.Equal(t, "John", req.Name)

		raw, readErr := c.GetRawData()
		require.NoError(t, readErr)
		assert.JSONEq(t, `{"name":"John"}`, string(raw))
	})
}

func TestSlugRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"simple slug", "standard", true},
		{"slug with hyphens", "standard-1", true},
		{"multi-hyphen", "pro-plan-v2", true},
		{"uppercase invalid", "Standard", false},
		{"starts with hyphen", "-invalid", false},
		{"ends with hyphen", "invalid-", false},
		{"consecutive hyphens", "invalid--slug", false},
		{"empty", "", false},
		{"with underscore", "invalid_slug", false},
		{"numeric", "123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, slugRegex.MatchString(tt.input))
		})
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		name string
		s    string
		sep  byte
		want int
	}{
		{"found at start", ".hello", '.', 0},
		{"found in middle", "hello.world", '.', 5},
		{"not found", "hello", '.', -1},
		{"empty string", "", '.', -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, indexOf(tt.s, tt.sep))
		})
	}
}
