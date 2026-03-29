// Package cursor provides cursor-based pagination utilities for repository methods.
// Cursor-based pagination (keyset pagination) is more efficient than offset-based
// pagination for large datasets because:
//  1. It avoids expensive COUNT(*) queries
//  2. It uses indexes to seek directly to the next page
//  3. It provides stable pagination when new data is inserted
//
// Usage pattern in repositories:
//
//	func (r *Repo) List(ctx context.Context, params PaginationParams) ([]Item, PaginationMeta, error) {
//	    cp := cursor.ParseParams(params)
//	    return r.listWithCursor(ctx, cp)
//	}
package cursor

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// Direction indicates the direction of pagination.
type Direction string

const (
	Next     Direction = "next"
	Previous Direction = "prev"
)

// Params contains cursor pagination parameters for internal use.
type Params struct {
	PerPage   int
	LastID    string
	Direction Direction
}

// ParseParams extracts cursor pagination params from models.PaginationParams.
func ParseParams(p models.PaginationParams) Params {
	if p.Cursor == "" {
		return Params{PerPage: p.PerPage}
	}
	cp := p.DecodeCursor()
	dir := Next
	if cp.Direction == "prev" {
		dir = Previous
	}
	return Params{
		PerPage:   p.PerPage,
		LastID:    cp.LastID,
		Direction: dir,
	}
}

// BuildWhereClause builds the WHERE clause for cursor-based pagination.
// For next pages: WHERE ... AND id < $lastID (for DESC order)
// For prev pages: WHERE ... AND id > $lastID (for DESC order)
//
// Note: This assumes id is the primary key and ordering is DESC.
// For ASC ordering, the operators should be reversed.
func BuildWhereClause(baseClause string, params Params, orderDesc bool, idx int) (string, int, any) {
	if params.LastID == "" {
		return baseClause, idx, nil
	}

	var op string
	if orderDesc {
		// For DESC order: next page has smaller IDs
		if params.Direction == Next {
			op = "<"
		} else {
			op = ">"
		}
	} else {
		// For ASC order: next page has larger IDs
		if params.Direction == Next {
			op = ">"
		} else {
			op = "<"
		}
	}

	var extraArg any
	var clause string
	if baseClause != "" {
		clause = fmt.Sprintf("%s AND id %s $%d", baseClause, op, idx)
	} else {
		clause = fmt.Sprintf("id %s $%d", op, idx)
	}
	extraArg = params.LastID

	return clause, idx + 1, extraArg
}

// ComputeHasMore determines if there are more results by checking if
// we fetched more items than requested (using the (n+1) pattern).
func ComputeHasMore(items []any, perPage int) (hasMore bool, lastID string) {
	if len(items) > perPage {
		hasMore = true
		items = items[:perPage] // Trim to requested size
	}
	if len(items) > 0 {
		// Extract lastID from the last item
		// This requires the item to have an ID field
		// The caller should handle this based on their item type
	}
	return hasMore, lastID
}

// QueryResult represents the result of a cursor-paginated query.
type QueryResult[T any] struct {
	Items   []T
	HasMore bool
	LastID  string
}

// ComputeResult computes the pagination result from a query that fetched
// one extra item to detect hasMore.
func ComputeResult[T interface{ GetID() string }](items []T, perPage int) QueryResult[T] {
	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}
	var lastID string
	if len(items) > 0 {
		lastID = items[len(items)-1].GetID()
	}
	return QueryResult[T]{
		Items:   items,
		HasMore: hasMore,
		LastID:  lastID,
	}
}

// ScanRows is a generic helper to scan pgx.Rows into a slice.
// It's similar to the ScanRows function in the repository package.
func ScanRows[T any](ctx context.Context, rows pgx.Rows, scan func(pgx.Rows) (T, error)) ([]T, error) {
	var results []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}