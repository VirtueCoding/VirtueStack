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

func TestNewPaginationMeta(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		perPage    int
		total      int
		wantPages  int
		wantMore   bool
	}{
		{
			name:      "first page of many",
			page:      1,
			perPage:   20,
			total:     100,
			wantPages: 5,
			wantMore:  true,
		},
		{
			name:      "last page",
			page:      5,
			perPage:   20,
			total:     100,
			wantPages: 5,
			wantMore:  false,
		},
		{
			name:      "single page",
			page:      1,
			perPage:   20,
			total:     10,
			wantPages: 1,
			wantMore:  false,
		},
		{
			name:      "empty result",
			page:      1,
			perPage:   20,
			total:     0,
			wantPages: 0,
			wantMore:  false,
		},
		{
			name:      "partial last page",
			page:      1,
			perPage:   20,
			total:     21,
			wantPages: 2,
			wantMore:  true,
		},
		{
			name:      "exact page boundary",
			page:      2,
			perPage:   10,
			total:     20,
			wantPages: 2,
			wantMore:  false,
		},
		{
			name:      "zero per_page does not panic",
			page:      1,
			perPage:   0,
			total:     100,
			wantPages: 0,
			wantMore:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := NewPaginationMeta(tt.page, tt.perPage, tt.total)
			assert.Equal(t, tt.page, meta.Page)
			assert.Equal(t, tt.perPage, meta.PerPage)
			assert.Equal(t, tt.total, meta.Total)
			assert.Equal(t, tt.wantPages, meta.TotalPages)
			assert.Equal(t, tt.wantMore, meta.HasMore)
		})
	}
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
		wantPage    int
		wantPerPage int
		wantCursor  string
	}{
		{
			name:        "defaults when no params",
			queryParams: map[string]string{},
			wantPage:    DefaultPage,
			wantPerPage: DefaultPerPage,
		},
		{
			name:        "custom page and per_page",
			queryParams: map[string]string{"page": "3", "per_page": "50"},
			wantPage:    3,
			wantPerPage: 50,
		},
		{
			name:        "per_page capped at max",
			queryParams: map[string]string{"per_page": "200"},
			wantPage:    DefaultPage,
			wantPerPage: MaxPerPage,
		},
		{
			name:        "invalid page falls back to default",
			queryParams: map[string]string{"page": "abc"},
			wantPage:    DefaultPage,
			wantPerPage: DefaultPerPage,
		},
		{
			name:        "negative page falls back to default",
			queryParams: map[string]string{"page": "-1"},
			wantPage:    DefaultPage,
			wantPerPage: DefaultPerPage,
		},
		{
			name:        "zero page falls back to default",
			queryParams: map[string]string{"page": "0"},
			wantPage:    DefaultPage,
			wantPerPage: DefaultPerPage,
		},
		{
			name:        "negative per_page falls back to default",
			queryParams: map[string]string{"per_page": "-5"},
			wantPage:    DefaultPage,
			wantPerPage: DefaultPerPage,
		},
		{
			name:        "cursor is passed through",
			queryParams: map[string]string{"cursor": "some-cursor-value"},
			wantPage:    DefaultPage,
			wantPerPage: DefaultPerPage,
			wantCursor:  "some-cursor-value",
		},
		{
			name:        "very large page number falls back to default",
			queryParams: map[string]string{"page": "99999"},
			wantPage:    DefaultPage,
			wantPerPage: DefaultPerPage,
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
			assert.Equal(t, tt.wantPage, params.Page)
			assert.Equal(t, tt.wantPerPage, params.PerPage)
			assert.Equal(t, tt.wantCursor, params.Cursor)
		})
	}
}

func TestPaginationParams_IsCursorBased(t *testing.T) {
	assert.True(t, PaginationParams{Cursor: "abc"}.IsCursorBased())
	assert.False(t, PaginationParams{}.IsCursorBased())
}

func TestPaginationParams_Offset(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		perPage    int
		wantOffset int
	}{
		{"page 1", 1, 20, 0},
		{"page 2", 2, 20, 20},
		{"page 3 with 10 per page", 3, 10, 20},
		{"page 0 treated as page 1", 0, 20, 0},
		{"negative page treated as page 1", -1, 20, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PaginationParams{Page: tt.page, PerPage: tt.perPage}
			assert.Equal(t, tt.wantOffset, p.Offset())
		})
	}
}

func TestPaginationParams_Limit(t *testing.T) {
	p := PaginationParams{PerPage: 50}
	assert.Equal(t, 50, p.Limit())
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
