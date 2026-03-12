// Package models provides base model types for VirtueStack.
package models

import (
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
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// NewPaginationMeta creates pagination metadata from page, perPage, and total.
// It calculates the total pages based on the per-page value.
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
	}
}

// PaginationParams holds pagination query parameters.
type PaginationParams struct {
	Page    int
	PerPage int
}

// ParsePagination extracts pagination from Gin context query params.
// Defaults: page=1, per_page=20. Max per_page=100.
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
	}
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
type Response struct {
	Data any `json:"data"`
}

// ListResponse is the standard API response wrapper for paginated lists.
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
