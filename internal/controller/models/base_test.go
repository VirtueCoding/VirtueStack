package models

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewCursorPaginationMeta(t *testing.T) {
	tests := []struct {
		name       string
		perPage    int
		hasMore    bool
		lastID     string
		wantCursor bool
	}{
		{
			name:       "has more with lastID",
			perPage:    20,
			hasMore:    true,
			lastID:     "abc-123",
			wantCursor: true,
		},
		{
			name:       "no more results",
			perPage:    20,
			hasMore:    false,
			lastID:     "abc-123",
			wantCursor: false,
		},
		{
			name:       "has more but empty lastID",
			perPage:    20,
			hasMore:    true,
			lastID:     "",
			wantCursor: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := NewCursorPaginationMeta(tt.perPage, tt.hasMore, tt.lastID)
			assert.Equal(t, tt.perPage, meta.PerPage)
			assert.Equal(t, tt.hasMore, meta.HasMore)
			if tt.wantCursor {
				assert.NotEmpty(t, meta.NextCursor)
			} else {
				assert.Empty(t, meta.NextCursor)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name        string
		queryParams map[string]string
		wantPerPage int
		wantCursor  string
	}{
		{
			name:        "defaults when no params",
			queryParams: map[string]string{},
			wantPerPage: DefaultPerPage,
		},
		{
			name:        "custom per_page",
			queryParams: map[string]string{"per_page": "50"},
			wantPerPage: 50,
		},
		{
			name:        "per_page capped at max",
			queryParams: map[string]string{"per_page": "200"},
			wantPerPage: MaxPerPage,
		},
		{
			name:        "negative per_page falls back to default",
			queryParams: map[string]string{"per_page": "-5"},
			wantPerPage: DefaultPerPage,
		},
		{
			name:        "cursor is passed through",
			queryParams: map[string]string{"cursor": "some-cursor-value"},
			wantPerPage: DefaultPerPage,
			wantCursor:  "some-cursor-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()
			c.Request = req

			params := ParsePagination(c)
			assert.Equal(t, tt.wantPerPage, params.PerPage)
			assert.Equal(t, tt.wantCursor, params.Cursor)
		})
	}
}

func TestEncodeCursor_DecodeCursor(t *testing.T) {
	tests := []struct {
		name      string
		lastID    string
		direction string
	}{
		{"next cursor", "uuid-123", "next"},
		{"prev cursor", "uuid-456", "prev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeCursor(tt.lastID, tt.direction)
			require.NotEmpty(t, encoded)

			p := PaginationParams{Cursor: encoded}
			decoded := p.DecodeCursor()
			assert.Equal(t, tt.lastID, decoded.LastID)
			assert.Equal(t, tt.direction, decoded.Direction)
		})
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
	}{
		{"empty cursor", ""},
		{"invalid base64", "not-base64!!!"},
		{"valid base64 but invalid JSON", base64.StdEncoding.EncodeToString([]byte("not json"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PaginationParams{Cursor: tt.cursor}
			decoded := p.DecodeCursor()
			assert.Empty(t, decoded.LastID)
			assert.Empty(t, decoded.Direction)
		})
	}
}

func TestSoftDelete_IsDeleted(t *testing.T) {
	t.Run("not deleted", func(t *testing.T) {
		sd := &SoftDelete{}
		assert.False(t, sd.IsDeleted())
	})

	t.Run("deleted", func(t *testing.T) {
		ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		sd := &SoftDelete{DeletedAt: &ts}
		assert.True(t, sd.IsDeleted())
	})
}

func TestParsePositiveInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantVal int
		wantOK  bool
	}{
		{"valid positive", "42", 42, true},
		{"one", "1", 1, true},
		{"max boundary", "10000", 10000, true},
		{"over max boundary", "10001", 0, false},
		{"zero", "0", 0, false},
		{"negative", "-1", 0, false},
		{"not a number", "abc", 0, false},
		{"empty", "", 0, false},
		{"float", "1.5", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := parsePositiveInt(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantVal, val)
			}
		})
	}
}
