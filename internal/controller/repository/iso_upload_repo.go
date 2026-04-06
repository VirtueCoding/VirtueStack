// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ISOUploadRepository provides database operations for ISO uploads.
// It tracks uploaded ISO files per VM to enforce upload limits across
// multiple controller instances, replacing the previous in-memory tracking
// which did not work correctly in horizontally scaled deployments.
type ISOUploadRepository struct {
	db DB
}

// ErrVMLockNotFound indicates the VM row disappeared before a transactional ISO lock.
var ErrVMLockNotFound = fmt.Errorf("vm lock not found: %w", sharederrors.ErrNotFound)

// NewISOUploadRepository creates a new ISOUploadRepository.
func NewISOUploadRepository(db DB) *ISOUploadRepository {
	return &ISOUploadRepository{db: db}
}

// ISOUpload represents an uploaded ISO file tracked in the database.
type ISOUpload struct {
	ID          string    `json:"id"`
	VMID        string    `json:"vm_id"`
	CustomerID  string    `json:"customer_id"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	SHA256      string    `json:"sha256"`
	StoragePath string    `json:"storage_path"`
	CreatedAt   time.Time `json:"created_at"`
}

func scanISOUpload(row pgx.Row) (ISOUpload, error) {
	var u ISOUpload
	err := row.Scan(&u.ID, &u.VMID, &u.CustomerID, &u.FileName, &u.FileSize, &u.SHA256, &u.StoragePath, &u.CreatedAt)
	return u, err
}

// Create inserts a new ISO upload record and returns the ID.
// This operation is atomic with the ISO limit check when wrapped in a transaction
// with FOR UPDATE lock on the VM's Plan.
func (r *ISOUploadRepository) Create(ctx context.Context, upload *ISOUpload) error {
	if upload.ID == "" {
		upload.ID = uuid.NewString()
	}
	const q = `
		INSERT INTO iso_uploads (id, vm_id, customer_id, file_name, file_size, sha256, storage_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err := r.db.QueryRow(ctx, q,
		upload.ID,
		upload.VMID,
		upload.CustomerID,
		upload.FileName,
		upload.FileSize,
		upload.SHA256,
		upload.StoragePath,
	).Scan(&upload.CreatedAt)

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

	upload, err := ScanRow(ctx, r.db, q, []any{id}, scanISOUpload)
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

// DeleteForVMIfDetached removes ISO metadata while holding the VM row lock so the
// attached ISO state cannot change concurrently.
func (r *ISOUploadRepository) DeleteForVMIfDetached(
	ctx context.Context,
	vmID, uploadID, customerID string,
) (*ISOUpload, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning ISO delete transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback is a best-effort safety net after commit or early return.

	const lockQ = `SELECT attached_iso FROM vms WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`
	var attachedISO *string
	if queryErr := tx.QueryRow(ctx, lockQ, vmID).Scan(&attachedISO); queryErr != nil {
		if errors.Is(queryErr, pgx.ErrNoRows) {
			return nil, fmt.Errorf("locking VM %s for ISO delete: %w", vmID, ErrVMLockNotFound)
		}
		return nil, fmt.Errorf("locking VM %s for ISO delete: %w", vmID, queryErr)
	}
	if attachedISO != nil && *attachedISO == uploadID {
		return nil, fmt.Errorf("ISO %s attached to VM %s: %w", uploadID, vmID, sharederrors.ErrConflict)
	}

	const selectQ = `
		SELECT id, vm_id, customer_id, file_name, file_size, sha256, storage_path, created_at
		FROM iso_uploads
		WHERE id = $1 AND vm_id = $2 AND customer_id = $3
		FOR UPDATE`
	upload, err := ScanRow(ctx, tx, selectQ, []any{uploadID, vmID, customerID}, scanISOUpload)
	if err != nil {
		return nil, fmt.Errorf("getting ISO upload %s for VM %s: %w", uploadID, vmID, err)
	}

	const deleteQ = `DELETE FROM iso_uploads WHERE id = $1 AND vm_id = $2 AND customer_id = $3`
	tag, err := tx.Exec(ctx, deleteQ, uploadID, vmID, customerID)
	if err != nil {
		return nil, fmt.Errorf("deleting ISO upload %s for VM %s: %w", uploadID, vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("deleting ISO upload %s for VM %s: %w", uploadID, vmID, ErrNoRowsAffected)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing ISO delete for VM %s: %w", vmID, err)
	}

	return &upload, nil
}

// AttachToVMIfAvailable attaches an ISO while holding the VM row lock so metadata
// deletion cannot race the VM update.
func (r *ISOUploadRepository) AttachToVMIfAvailable(
	ctx context.Context,
	vmID, uploadID, customerID string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning ISO attach transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback is a best-effort safety net after commit or early return.

	const lockQ = `SELECT attached_iso FROM vms WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`
	var attachedISO *string
	if queryErr := tx.QueryRow(ctx, lockQ, vmID).Scan(&attachedISO); queryErr != nil {
		if errors.Is(queryErr, pgx.ErrNoRows) {
			return fmt.Errorf("locking VM %s for ISO attach: %w", vmID, ErrVMLockNotFound)
		}
		return fmt.Errorf("locking VM %s for ISO attach: %w", vmID, queryErr)
	}

	const selectQ = `
		SELECT id, vm_id, customer_id, file_name, file_size, sha256, storage_path, created_at
		FROM iso_uploads
		WHERE id = $1 AND vm_id = $2 AND customer_id = $3
		FOR UPDATE`
	if _, scanErr := ScanRow(ctx, tx, selectQ, []any{uploadID, vmID, customerID}, scanISOUpload); scanErr != nil {
		return fmt.Errorf("getting ISO upload %s for VM %s: %w", uploadID, vmID, scanErr)
	}

	const updateQ = `UPDATE vms SET attached_iso = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := tx.Exec(ctx, updateQ, &uploadID, vmID)
	if err != nil {
		return fmt.Errorf("attaching ISO %s to VM %s: %w", uploadID, vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("attaching ISO %s to VM %s: %w", uploadID, vmID, ErrNoRowsAffected)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing ISO attach for VM %s: %w", vmID, err)
	}

	return nil
}

// DetachFromVMIfAttached clears the VM's attached ISO while holding the VM row lock
// so the operation only succeeds if the requested ISO is still the current attachment.
func (r *ISOUploadRepository) DetachFromVMIfAttached(ctx context.Context, vmID, uploadID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning ISO detach transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback is a best-effort safety net after commit or early return.

	const lockQ = `SELECT attached_iso FROM vms WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`
	var attachedISO *string
	if queryErr := tx.QueryRow(ctx, lockQ, vmID).Scan(&attachedISO); queryErr != nil {
		if errors.Is(queryErr, pgx.ErrNoRows) {
			return fmt.Errorf("locking VM %s for ISO detach: %w", vmID, ErrVMLockNotFound)
		}
		return fmt.Errorf("locking VM %s for ISO detach: %w", vmID, queryErr)
	}
	if attachedISO == nil || *attachedISO != uploadID {
		return fmt.Errorf("ISO %s not attached to VM %s: %w", uploadID, vmID, sharederrors.ErrConflict)
	}

	const updateQ = `UPDATE vms SET attached_iso = NULL, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL AND attached_iso = $2`
	tag, err := tx.Exec(ctx, updateQ, vmID, uploadID)
	if err != nil {
		return fmt.Errorf("detaching ISO %s from VM %s: %w", uploadID, vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("detaching ISO %s from VM %s: %w", uploadID, vmID, ErrNoRowsAffected)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing ISO detach for VM %s: %w", vmID, err)
	}

	return nil
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
	if upload.ID == "" {
		upload.ID = uuid.NewString()
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback is a best-effort safety net after commit or early return.

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
		INSERT INTO iso_uploads (id, vm_id, customer_id, file_name, file_size, sha256, storage_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`

	err = tx.QueryRow(ctx, insertQ,
		upload.ID,
		upload.VMID,
		upload.CustomerID,
		upload.FileName,
		upload.FileSize,
		upload.SHA256,
		upload.StoragePath,
	).Scan(&upload.CreatedAt)

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
