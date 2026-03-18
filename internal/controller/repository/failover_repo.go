// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// FailoverRepository provides database operations for failover requests.
type FailoverRepository struct {
	db DB
}

// NewFailoverRepository creates a new FailoverRepository with the given database connection.
func NewFailoverRepository(db DB) *FailoverRepository {
	return &FailoverRepository{db: db}
}

const failoverRequestSelectCols = `
	id, node_id, requested_by, status,
	reason, result, approved_at, completed_at,
	created_at, updated_at`

func scanFailoverRequest(row pgx.Row) (models.FailoverRequest, error) {
	var fr models.FailoverRequest
	var result []byte
	err := row.Scan(
		&fr.ID, &fr.NodeID, &fr.RequestedBy, &fr.Status,
		&fr.Reason, &result, &fr.ApprovedAt, &fr.CompletedAt,
		&fr.CreatedAt, &fr.UpdatedAt,
	)
	if err != nil {
		return fr, err
	}
	if len(result) > 0 {
		fr.Result = json.RawMessage(result)
	}
	return fr, nil
}

// Create inserts a new failover request into the database.
func (r *FailoverRepository) Create(ctx context.Context, req *models.FailoverRequest) error {
	const q = `
		INSERT INTO failover_requests (
			node_id, requested_by, status, reason
		) VALUES ($1,$2,$3,$4)
		RETURNING ` + failoverRequestSelectCols

	row := r.db.QueryRow(ctx, q,
		req.NodeID, req.RequestedBy, req.Status, req.Reason,
	)
	created, err := scanFailoverRequest(row)
	if err != nil {
		return fmt.Errorf("creating failover request: %w", err)
	}
	*req = created
	return nil
}

// GetByID returns a failover request by its UUID. Returns ErrNotFound if no request matches.
func (r *FailoverRepository) GetByID(ctx context.Context, id string) (*models.FailoverRequest, error) {
	const q = `SELECT ` + failoverRequestSelectCols + ` FROM failover_requests WHERE id = $1`
	fr, err := ScanRow(ctx, r.db, q, []any{id}, scanFailoverRequest)
	if err != nil {
		return nil, fmt.Errorf("getting failover request %s: %w", id, err)
	}
	return &fr, nil
}

// GetByNodeID returns all failover requests for a specific node.
func (r *FailoverRepository) GetByNodeID(ctx context.Context, nodeID string) ([]models.FailoverRequest, error) {
	const q = `SELECT ` + failoverRequestSelectCols + ` FROM failover_requests WHERE node_id = $1 ORDER BY created_at DESC`
	requests, err := ScanRows(ctx, r.db, q, []any{nodeID}, func(rows pgx.Rows) (models.FailoverRequest, error) {
		return scanFailoverRequest(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("getting failover requests for node %s: %w", nodeID, err)
	}
	return requests, nil
}

// UpdateStatus updates the status of a failover request and sets timestamps accordingly.
// When status is "approved", approved_at is set. When status is "completed" or "failed",
// completed_at is set.
func (r *FailoverRepository) UpdateStatus(ctx context.Context, id, status string, result any) error {
	var resultJSON json.RawMessage
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("marshaling failover result for request %s: %w", id, err)
		}
		resultJSON = json.RawMessage(b)
	}

	var completedAt *time.Time
	if status == models.FailoverStatusCompleted || status == models.FailoverStatusFailed {
		t := time.Now()
		completedAt = &t
	}

	var approvedAt *time.Time
	if status == models.FailoverStatusApproved {
		t := time.Now()
		approvedAt = &t
	}

	const q = `
		UPDATE failover_requests
		SET status = $1, result = COALESCE($2, result),
		    approved_at = COALESCE($3, approved_at),
		    completed_at = COALESCE($4, completed_at),
		    updated_at = NOW()
		WHERE id = $5`
	tag, err := r.db.Exec(ctx, q, status, resultJSON, approvedAt, completedAt, id)
	if err != nil {
		return fmt.Errorf("updating failover request %s status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating failover request %s status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// List returns a paginated list of failover requests with optional filters and total count.
func (r *FailoverRepository) List(ctx context.Context, filter models.FailoverRequestListFilter) ([]models.FailoverRequest, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.NodeID != nil {
		where = append(where, fmt.Sprintf("node_id = $%d", idx))
		args = append(args, *filter.NodeID)
		idx++
	}
	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM failover_requests WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting failover requests: %w", err)
	}

	listQ := fmt.Sprintf(
		"SELECT %s FROM failover_requests WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		failoverRequestSelectCols, clause, idx, idx+1,
	)
	args = append(args, filter.Limit(), filter.Offset())

	requests, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.FailoverRequest, error) {
		return scanFailoverRequest(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing failover requests: %w", err)
	}
	return requests, total, nil
}
