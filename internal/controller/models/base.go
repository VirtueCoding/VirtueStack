// Package models provides base model types for VirtueStack.
package models

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Pagination constants.
const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

// PaginationMeta holds pagination metadata for list responses.
type PaginationMeta struct {
	Page       int    `json:"page,omitempty"`
	PerPage    int    `json:"per_page"`
	Total      int    `json:"total,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
}

// NewPaginationMeta creates pagination metadata from page, perPage, and total.
// It calculates the total pages based on the per-page value and whether more
// pages exist beyond the current page.
// Note: For cursor-based pagination, use NewCursorPaginationMeta instead.
func NewPaginationMeta(page, perPage, total int) PaginationMeta {
	totalPages := 0
	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}

	return PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
		HasMore:    page < totalPages,
	}
}

// NewCursorPaginationMeta creates pagination metadata for cursor-based pagination.
// It does not include total/total_pages as these are not typically available
// without a COUNT query, which cursor-based pagination avoids.
func NewCursorPaginationMeta(perPage int, hasMore bool, lastID string) PaginationMeta {
	meta := PaginationMeta{
		PerPage: perPage,
		HasMore: hasMore,
	}
	if hasMore && lastID != "" {
		meta.NextCursor = EncodeCursor(lastID, "next")
	}
	return meta
}

// PaginationParams holds pagination query parameters.
// Supports both offset-based (Page) and cursor-based (Cursor) pagination.
// Cursor-based pagination is preferred for large datasets as it avoids the
// performance cost of COUNT(*) queries and provides stable pagination.
type PaginationParams struct {
	Page    int
	PerPage int
	// Cursor is an opaque token for cursor-based pagination.
	// When provided, Page is ignored and pagination uses the cursor.
	Cursor string
}

// CursorPagination represents the decoded cursor for internal use.
// The cursor format is base64-encoded JSON: {"id":"<last_id>","dir":"next"|"prev"}
type CursorPagination struct {
	LastID   string `json:"id"`
	Direction string `json:"dir"` // "next" or "prev"
}

// ParsePagination extracts pagination from Gin context query params.
// Defaults: page=1, per_page=20. Max per_page=100.
// Supports cursor-based pagination via "cursor" query parameter.
// When cursor is provided, page is ignored.
func ParsePagination(c *gin.Context) PaginationParams {
	page := DefaultPage
	perPage := DefaultPerPage

	if pageStr := c.Query("page"); pageStr != "" {
		if p, ok := parsePositiveInt(pageStr); ok && p > 0 {
			page = p
		}
	}

	if perPageStr := c.Query("per_page"); perPageStr != "" {
		if pp, ok := parsePositiveInt(perPageStr); ok && pp > 0 {
			perPage = pp
			if perPage > MaxPerPage {
				perPage = MaxPerPage
			}
		}
	}

	return PaginationParams{
		Page:    page,
		PerPage: perPage,
		Cursor:  c.Query("cursor"),
	}
}

// IsCursorBased returns true if this pagination uses cursor-based pagination.
func (p PaginationParams) IsCursorBased() bool {
	return p.Cursor != ""
}

// DecodeCursor decodes the cursor into its components.
// Returns an empty CursorPagination if the cursor is invalid or empty.
func (p PaginationParams) DecodeCursor() CursorPagination {
	if p.Cursor == "" {
		return CursorPagination{}
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(p.Cursor)
	if err != nil {
		return CursorPagination{}
	}

	var cp CursorPagination
	if err := json.Unmarshal(decoded, &cp); err != nil {
		return CursorPagination{}
	}

	return cp
}

// EncodeCursor creates a cursor from the given last ID and direction.
func EncodeCursor(lastID, direction string) string {
	cp := CursorPagination{LastID: lastID, Direction: direction}
	data, err := json.Marshal(cp)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// Offset returns the SQL offset for the pagination params.
func (p PaginationParams) Offset() int {
	if p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.PerPage
}

// Limit returns the SQL limit for the pagination params.
func (p PaginationParams) Limit() int {
	return p.PerPage
}

// parsePositiveInt parses a string to a positive integer.
func parsePositiveInt(s string) (int, bool) {
	result, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}

	if result <= 0 || result > 10000 {
		return 0, false
	}

	return result, true
}

// Timestamps embeddable struct for created_at/updated_at fields.
type Timestamps struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SoftDelete embeddable struct for soft-deleted records.
type SoftDelete struct {
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// IsDeleted returns true if the record has been soft deleted.
func (s *SoftDelete) IsDeleted() bool {
	return s.DeletedAt != nil
}

// Response is the standard API response wrapper for single items.
// Data is typed as any because this generic envelope must accommodate every
// concrete resource type (VM, plan, customer, etc.) without duplicating the
// wrapper struct for each one.  Callers always populate Data with a concrete,
// JSON-serialisable value before writing the response.
type Response struct {
	Data any `json:"data"`
}

// ListResponse is the standard API response wrapper for paginated lists.
// Data is typed as any for the same reason as Response.Data: a single
// generic envelope is shared across all resource list endpoints.
type ListResponse struct {
	Data any            `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// ErrorResponse is the standard API error response.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error details.
type ErrorDetail struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id,omitempty"`
}
