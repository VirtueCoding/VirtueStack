// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// AuditRepository provides database operations for audit logs.
// Audit logs are APPEND-ONLY - no UPDATE or DELETE operations are permitted.
// This is enforced at the database level by REVOKE UPDATE, DELETE ON audit_logs FROM app_user.
type AuditRepository struct {
	db DB
}

// NewAuditRepository creates a new AuditRepository with the given database connection.
func NewAuditRepository(db DB) *AuditRepository {
	return &AuditRepository{db: db}
}

// scanAuditLog scans a single audit log row into a models.AuditLog struct.
func scanAuditLog(row pgx.Row) (models.AuditLog, error) {
	var a models.AuditLog
	err := row.Scan(
		&a.ID, &a.Timestamp, &a.ActorID, &a.ActorType, &a.ActorIP,
		&a.Action, &a.ResourceType, &a.ResourceID, &a.Changes,
		&a.CorrelationID, &a.Success, &a.ErrorMessage,
	)
	return a, err
}

const auditLogSelectCols = `
	id, timestamp, actor_id, actor_type, actor_ip,
	action, resource_type, resource_id, changes,
	correlation_id, success, error_message`

// Append inserts a new audit log record into the database.
// This is the ONLY write operation permitted on audit_logs (append-only for audit trail integrity).
// No RETURNING clause needed - audit logs are write-once, no need to return the created record.
func (r *AuditRepository) Append(ctx context.Context, audit *models.AuditLog) error {
	const q = `
		INSERT INTO audit_logs (
			actor_id, actor_type, actor_ip, action,
			resource_type, resource_id, changes,
			correlation_id, success, error_message
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	_, err := r.db.Exec(ctx, q,
		audit.ActorID, audit.ActorType, audit.ActorIP, audit.Action,
		audit.ResourceType, audit.ResourceID, audit.Changes,
		audit.CorrelationID, audit.Success, audit.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("appending audit log: %w", err)
	}
	return nil
}

// GetByID returns an audit log by its UUID. Returns ErrNotFound if no audit log matches.
func (r *AuditRepository) GetByID(ctx context.Context, id string) (*models.AuditLog, error) {
	const q = `SELECT ` + auditLogSelectCols + ` FROM audit_logs WHERE id = $1`
	audit, err := ScanRow(ctx, r.db, q, []any{id}, scanAuditLog)
	if err != nil {
		return nil, fmt.Errorf("getting audit log %s: %w", id, err)
	}
	return &audit, nil
}

// List returns a paginated list of audit logs with optional filters.
func (r *AuditRepository) List(ctx context.Context, filter models.AuditLogFilter) ([]models.AuditLog, bool, string, error) {
	baseBuilder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	whereBuilder := baseBuilder.Select("1").From("audit_logs")

	if filter.ActorID != nil {
		whereBuilder = whereBuilder.Where(sq.Eq{"actor_id": *filter.ActorID})
	}
	if filter.ActorType != nil {
		whereBuilder = whereBuilder.Where(sq.Eq{"actor_type": *filter.ActorType})
	}
	if filter.Action != nil {
		whereBuilder = whereBuilder.Where(sq.Eq{"action": *filter.Action})
	}
	if filter.ResourceType != nil {
		whereBuilder = whereBuilder.Where(sq.Eq{"resource_type": *filter.ResourceType})
	}
	if filter.ResourceID != nil {
		whereBuilder = whereBuilder.Where(sq.Eq{"resource_id": *filter.ResourceID})
	}
	if filter.Success != nil {
		whereBuilder = whereBuilder.Where(sq.Eq{"success": *filter.Success})
	}
	if filter.StartTime != nil {
		whereBuilder = whereBuilder.Where(sq.GtOrEq{"timestamp": *filter.StartTime})
	}
	if filter.EndTime != nil {
		whereBuilder = whereBuilder.Where(sq.LtOrEq{"timestamp": *filter.EndTime})
	}

	listBuilder := whereBuilder.Columns(auditLogSelectCols)
	cp := filter.DecodeCursor()
	if cp.LastID != "" {
		listBuilder = listBuilder.Where(sq.Lt{"id": cp.LastID})
	}
	listBuilder = listBuilder.OrderBy("id DESC").Limit(uint64(filter.PerPage + 1))
	listQ, listArgs, err := listBuilder.ToSql()
	if err != nil {
		return nil, false, "", fmt.Errorf("building audit log list query: %w", err)
	}
	logs, err := ScanRows(ctx, r.db, listQ, listArgs, func(rows pgx.Rows) (models.AuditLog, error) {
		return scanAuditLog(rows)
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing audit logs: %w", err)
	}

	hasMore := len(logs) > filter.PerPage
	if hasMore {
		logs = logs[:filter.PerPage]
	}
	var lastID string
	if len(logs) > 0 {
		lastID = logs[len(logs)-1].ID
	}
	return logs, hasMore, lastID, nil
}

// ListByActor returns audit logs for a specific actor with pagination.
func (r *AuditRepository) ListByActor(ctx context.Context, actorID string, filter models.AuditLogFilter) ([]models.AuditLog, bool, string, error) {
	filter.ActorID = &actorID
	return r.List(ctx, filter)
}

// ListByResource returns audit logs for a specific resource type and ID with pagination.
func (r *AuditRepository) ListByResource(ctx context.Context, resourceType string, resourceID string, filter models.AuditLogFilter) ([]models.AuditLog, bool, string, error) {
	filter.ResourceType = &resourceType
	filter.ResourceID = &resourceID
	return r.List(ctx, filter)
}

// ListByCorrelationID returns all audit logs for a given correlation ID.
// This is useful for tracing a request flow through multiple operations.
func (r *AuditRepository) ListByCorrelationID(ctx context.Context, correlationID string) ([]models.AuditLog, error) {
	const q = `SELECT ` + auditLogSelectCols + ` FROM audit_logs WHERE correlation_id = $1 ORDER BY timestamp ASC`
	logs, err := ScanRows(ctx, r.db, q, []any{correlationID}, func(rows pgx.Rows) (models.AuditLog, error) {
		return scanAuditLog(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing audit logs by correlation ID %s: %w", correlationID, err)
	}
	return logs, nil
}

// ListRecent returns the most recent audit logs up to the specified limit.
// Useful for dashboard widgets and activity feeds.
func (r *AuditRepository) ListRecent(ctx context.Context, limit int) ([]models.AuditLog, error) {
	if limit <= 0 {
		limit = 20
	}
	const q = `SELECT ` + auditLogSelectCols + ` FROM audit_logs ORDER BY timestamp DESC LIMIT $1`
	logs, err := ScanRows(ctx, r.db, q, []any{limit}, func(rows pgx.Rows) (models.AuditLog, error) {
		return scanAuditLog(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing recent audit logs: %w", err)
	}
	return logs, nil
}

// GetPartitionStats returns statistics about audit log partitions.
// This is useful for monitoring partition sizes and planning maintenance.
func (r *AuditRepository) GetPartitionStats(ctx context.Context) ([]AuditPartitionStats, error) {
	const q = `
		SELECT
			p.relname AS partition_name,
			COALESCE(c.reltuples::bigint, 0) AS row_count,
			COALESCE(pg_total_relation_size(p.relname::regclass), 0) AS size_bytes
		FROM pg_class p
		JOIN pg_inherits i ON p.oid = i.inhrelid
		JOIN pg_class c ON c.oid = i.inhparent
		WHERE c.relname = 'audit_logs'
		ORDER BY p.relname ASC`

	stats, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (AuditPartitionStats, error) {
		var s AuditPartitionStats
		err := rows.Scan(&s.PartitionName, &s.RowCount, &s.SizeBytes)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("getting audit log partition stats: %w", err)
	}
	return stats, nil
}

// AuditPartitionStats holds statistics about an audit log partition.
type AuditPartitionStats struct {
	PartitionName string `db:"partition_name" json:"partition_name"`
	RowCount      int64  `db:"row_count" json:"row_count"`
	SizeBytes     int64  `db:"size_bytes" json:"size_bytes"`
}
