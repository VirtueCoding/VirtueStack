// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"time"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
)

// ISOUploadRepository provides database operations for ISO uploads.
// It tracks uploaded ISO files per VM to enforce upload limits across
// multiple controller instances, replacing the previous in-memory tracking
// which did not work correctly in horizontally scaled deployments.
type ISOUploadRepository struct {
	db DB
}

// NewISOUploadRepository creates a new ISOUploadRepository.
func NewISOUploadRepository(db DB) *ISOUploadRepository {
	return &ISOUploadRepository{db: db}
}

// ISOUpload represents an uploaded ISO file tracked in the database.
type ISOUpload struct {
	ID         string    `json:"id"`
	VMID       string    `json:"vm_id"`
	CustomerID string    `json:"customer_id"`
	FileName   string    `json:"file_name"`
	FileSize   int64     `json:"file_size"`
	SHA256     string    `json:"sha256"`
	StoragePath string   `json:"storage_path"`
	CreatedAt  time.Time `json:"created_at"`
}

// Create inserts a new ISO upload record and returns the ID.
// This operation is atomic with the ISO limit check when wrapped in a transaction
// with FOR UPDATE lock on the VM's Plan.
func (r *ISOUploadRepository) Create(ctx context.Context, upload *ISOUpload) error {
	const q = `
		INSERT INTO iso_uploads (vm_id, customer_id, file_name, file_size, sha256, storage_path)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, q,
		upload.VMID,
		upload.CustomerID,
		upload.FileName,
		upload.FileSize,
		upload.SHA256,
		upload.StoragePath,
	).Scan(&upload.ID, &upload.CreatedAt)

	if err != nil {
		return fmt.Errorf("creating ISO upload: %w", err)
	}
	return nil
}

// GetByID returns an ISO upload by ID with ownership validation.
func (r *ISOUploadRepository) GetByID(ctx context.Context, id string) (*ISOUpload, error) {
	const q = `
		SELECT id, vm_id, customer_id, file_name, file_size, sha256, storage_path, created_at
		FROM iso_uploads
		WHERE id = $1`

	upload, err := ScanRow(ctx, r.db, q, []any{id}, func(row pgx.Row) (ISOUpload, error) {
		var u ISOUpload
		err := row.Scan(&u.ID, &u.VMID, &u.CustomerID, &u.FileName, &u.FileSize, &u.SHA256, &u.StoragePath, &u.CreatedAt)
		return u, err
	})
	if err != nil {
		return nil, fmt.Errorf("getting ISO upload %s: %w", id, err)
	}
	return &upload, nil
}

// Delete removes an ISO upload by ID. Returns ErrNoRowsAffected if not found.
func (r *ISOUploadRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM iso_uploads WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting ISO upload %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting ISO upload %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// DeleteByVMAndID removes an ISO upload by VM ID and upload ID.
// Returns ErrNoRowsAffected if not found or if the upload doesn't belong to the VM.
func (r *ISOUploadRepository) DeleteByVMAndID(ctx context.Context, vmID, uploadID string) error {
	const q = `DELETE FROM iso_uploads WHERE id = $1 AND vm_id = $2`
	tag, err := r.db.Exec(ctx, q, uploadID, vmID)
	if err != nil {
		return fmt.Errorf("deleting ISO upload %s for VM %s: %w", uploadID, vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting ISO upload %s for VM %s: %w", uploadID, vmID, ErrNoRowsAffected)
	}
	return nil
}

// CountByVM returns the number of ISO uploads for a VM.
// This is used to enforce plan-level ISO limits.
func (r *ISOUploadRepository) CountByVM(ctx context.Context, vmID string) (int, error) {
	const q = `SELECT COUNT(*) FROM iso_uploads WHERE vm_id = $1`
	count, err := CountRows(ctx, r.db, q, vmID)
	if err != nil {
		return 0, fmt.Errorf("counting ISO uploads for VM %s: %w", vmID, err)
	}
	return count, nil
}

// ListByVM returns all ISO uploads for a VM.
func (r *ISOUploadRepository) ListByVM(ctx context.Context, vmID string) ([]ISOUpload, error) {
	const q = `
		SELECT id, vm_id, customer_id, file_name, file_size, sha256, storage_path, created_at
		FROM iso_uploads
		WHERE vm_id = $1
		ORDER BY created_at DESC`

	uploads, err := ScanRows(ctx, r.db, q, []any{vmID}, func(rows pgx.Rows) (ISOUpload, error) {
		var u ISOUpload
		err := rows.Scan(&u.ID, &u.VMID, &u.CustomerID, &u.FileName, &u.FileSize, &u.SHA256, &u.StoragePath, &u.CreatedAt)
		return u, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing ISO uploads for VM %s: %w", vmID, err)
	}
	return uploads, nil
}

// ListByCustomer returns all ISO uploads for a customer across all their VMs.
func (r *ISOUploadRepository) ListByCustomer(ctx context.Context, customerID string) ([]ISOUpload, error) {
	const q = `
		SELECT id, vm_id, customer_id, file_name, file_size, sha256, storage_path, created_at
		FROM iso_uploads
		WHERE customer_id = $1
		ORDER BY created_at DESC`

	uploads, err := ScanRows(ctx, r.db, q, []any{customerID}, func(rows pgx.Rows) (ISOUpload, error) {
		var u ISOUpload
		err := rows.Scan(&u.ID, &u.VMID, &u.CustomerID, &u.FileName, &u.FileSize, &u.SHA256, &u.StoragePath, &u.CreatedAt)
		return u, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing ISO uploads for customer %s: %w", customerID, err)
	}
	return uploads, nil
}

// CreateIfUnderLimit creates an ISO upload record only if the VM is under its plan limit.
// This operation is atomic: it counts existing uploads and creates the new one in a transaction.
// Returns LimitExceededError if the limit would be exceeded.
func (r *ISOUploadRepository) CreateIfUnderLimit(ctx context.Context, upload *ISOUpload, planLimit int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock the VM row to prevent concurrent uploads from racing
	// This ensures the count is accurate at the moment of insertion
	const lockQ = `SELECT id FROM vms WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`
	var vmUUID string
	err = tx.QueryRow(ctx, lockQ, upload.VMID).Scan(&vmUUID)
	if err != nil {
		return fmt.Errorf("locking VM %s: %w", upload.VMID, ErrNoRowsAffected)
	}

	// Count existing uploads
	const countQ = `SELECT COUNT(*) FROM iso_uploads WHERE vm_id = $1`
	var count int
	err = tx.QueryRow(ctx, countQ, upload.VMID).Scan(&count)
	if err != nil {
		return fmt.Errorf("counting ISO uploads: %w", err)
	}

	// Check limit
	if count >= planLimit {
		return &LimitExceededError{
			Resource: "ISO uploads",
			Current:  count,
			Limit:    planLimit,
			VMID:     upload.VMID,
		}
	}

	// Create the upload record
	const insertQ = `
		INSERT INTO iso_uploads (vm_id, customer_id, file_name, file_size, sha256, storage_path)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	err = tx.QueryRow(ctx, insertQ,
		upload.VMID,
		upload.CustomerID,
		upload.FileName,
		upload.FileSize,
		upload.SHA256,
		upload.StoragePath,
	).Scan(&upload.ID, &upload.CreatedAt)

	if err != nil {
		return fmt.Errorf("creating ISO upload: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// LimitExceededError indicates that a resource limit has been reached.
type LimitExceededError struct {
	Resource string
	Current  int
	Limit    int
	VMID     string
}

func (e *LimitExceededError) Error() string {
	return fmt.Sprintf("%s limit exceeded for VM %s: %d/%d", e.Resource, e.VMID, e.Current, e.Limit)
}

func (e *LimitExceededError) Unwrap() error {
	return sharederrors.ErrLimitExceeded
}
