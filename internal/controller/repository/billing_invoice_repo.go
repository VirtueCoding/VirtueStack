// Package repository provides database access layer for VirtueStack Controller.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
)

// billingInvoiceColumns lists all columns selected from billing_invoices.
const billingInvoiceColumns = `id, customer_id, invoice_number, period_start, period_end,
	subtotal, tax_amount, total, currency, status,
	line_items, issued_at, paid_at, pdf_path, created_at, updated_at`

// BillingInvoiceRepository provides database operations for billing invoices.
type BillingInvoiceRepository struct {
	db DB
}

// NewBillingInvoiceRepository creates a new BillingInvoiceRepository.
func NewBillingInvoiceRepository(db DB) *BillingInvoiceRepository {
	return &BillingInvoiceRepository{db: db}
}

// NextInvoiceNumber atomically increments the invoice counter and returns
// a formatted invoice number like "INV-000001".
func (r *BillingInvoiceRepository) NextInvoiceNumber(ctx context.Context) (string, error) {
	var nextNum int
	err := r.db.QueryRow(ctx,
		`UPDATE billing_invoice_counters
		 SET last_number = last_number + 1
		 WHERE prefix = 'INV'
		 RETURNING last_number`,
	).Scan(&nextNum)
	if err != nil {
		return "", fmt.Errorf("next invoice number: %w", err)
	}
	return fmt.Sprintf("INV-%06d", nextNum), nil
}

// Create inserts a new invoice into the database.
func (r *BillingInvoiceRepository) Create(ctx context.Context, inv *models.BillingInvoice) error {
	lineItemsJSON, err := json.Marshal(inv.LineItems)
	if err != nil {
		return fmt.Errorf("marshal line items: %w", err)
	}

	_, err = r.db.Exec(ctx,
		`INSERT INTO billing_invoices
			(id, customer_id, invoice_number, period_start, period_end,
			 subtotal, tax_amount, total, currency, status,
			 line_items, issued_at, paid_at, pdf_path)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		inv.ID, inv.CustomerID, inv.InvoiceNumber, inv.PeriodStart, inv.PeriodEnd,
		inv.Subtotal, inv.TaxAmount, inv.Total, inv.Currency, inv.Status,
		lineItemsJSON, inv.IssuedAt, inv.PaidAt, inv.PDFPath,
	)
	if err != nil {
		return fmt.Errorf("create invoice: %w", err)
	}
	return nil
}

// GetByID returns a single invoice by UUID.
func (r *BillingInvoiceRepository) GetByID(ctx context.Context, id string) (*models.BillingInvoice, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+billingInvoiceColumns+` FROM billing_invoices WHERE id = $1`, id)
	return r.scanInvoice(row)
}

// GetByNumber returns a single invoice by its display number.
func (r *BillingInvoiceRepository) GetByNumber(ctx context.Context, number string) (*models.BillingInvoice, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+billingInvoiceColumns+` FROM billing_invoices WHERE invoice_number = $1`, number)
	return r.scanInvoice(row)
}

// ListByCustomer returns invoices for a specific customer, ordered by creation
// date descending. Uses cursor-based pagination keyed on ID.
func (r *BillingInvoiceRepository) ListByCustomer(
	ctx context.Context, customerID string, cursor string, perPage int,
) ([]models.BillingInvoice, string, error) {
	args := []any{customerID}
	clause := "customer_id = $1"
	idx := 2

	if cursor != "" {
		cp := models.PaginationParams{Cursor: cursor}.DecodeCursor()
		if cp.LastID != "" {
			clause += fmt.Sprintf(" AND id < $%d", idx)
			args = append(args, cp.LastID)
			idx++
		}
	}

	query := fmt.Sprintf(`SELECT %s FROM billing_invoices WHERE %s
		ORDER BY created_at DESC, id DESC LIMIT $%d`,
		billingInvoiceColumns, clause, idx)
	args = append(args, perPage+1)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list invoices by customer: %w", err)
	}
	defer rows.Close()

	return r.collectInvoices(rows, perPage)
}

// ListAll returns all invoices with optional filtering, ordered by creation
// date descending. Uses cursor-based pagination.
func (r *BillingInvoiceRepository) ListAll(
	ctx context.Context, filter models.InvoiceListFilter, cursor string, perPage int,
) ([]models.BillingInvoice, string, error) {
	clause := "1=1"
	args := []any{}
	argIdx := 1

	if filter.CustomerID != nil {
		clause += fmt.Sprintf(` AND customer_id = $%d`, argIdx)
		args = append(args, *filter.CustomerID)
		argIdx++
	}
	if filter.Status != nil {
		clause += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.StartDate != nil {
		clause += fmt.Sprintf(` AND period_start >= $%d`, argIdx)
		args = append(args, *filter.StartDate)
		argIdx++
	}
	if filter.EndDate != nil {
		clause += fmt.Sprintf(` AND period_end <= $%d`, argIdx)
		args = append(args, *filter.EndDate)
		argIdx++
	}

	if cursor != "" {
		cp := models.PaginationParams{Cursor: cursor}.DecodeCursor()
		if cp.LastID != "" {
			clause += fmt.Sprintf(` AND id < $%d`, argIdx)
			args = append(args, cp.LastID)
			argIdx++
		}
	}

	query := fmt.Sprintf(`SELECT %s FROM billing_invoices WHERE %s
		ORDER BY created_at DESC, id DESC LIMIT $%d`,
		billingInvoiceColumns, clause, argIdx)
	args = append(args, perPage+1)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list all invoices: %w", err)
	}
	defer rows.Close()

	return r.collectInvoices(rows, perPage)
}

// UpdateStatus sets the status of an invoice and updates the timestamp.
func (r *BillingInvoiceRepository) UpdateStatus(ctx context.Context, id, status string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE billing_invoices SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, id)
	if err != nil {
		return fmt.Errorf("update invoice status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNotFound
	}
	return nil
}

// SetIssuedAt marks an invoice as issued with the given timestamp.
func (r *BillingInvoiceRepository) SetIssuedAt(ctx context.Context, id string, issuedAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE billing_invoices SET issued_at = $1, status = $2, updated_at = NOW() WHERE id = $3`,
		issuedAt, models.InvoiceStatusIssued, id)
	if err != nil {
		return fmt.Errorf("set invoice issued_at: %w", err)
	}
	return nil
}

// SetPaidAt marks an invoice as paid with the given timestamp.
func (r *BillingInvoiceRepository) SetPaidAt(ctx context.Context, id string, paidAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE billing_invoices SET paid_at = $1, status = $2, updated_at = NOW() WHERE id = $3`,
		paidAt, models.InvoiceStatusPaid, id)
	if err != nil {
		return fmt.Errorf("set invoice paid_at: %w", err)
	}
	return nil
}

// SetPDFPath stores the path to the generated PDF file.
func (r *BillingInvoiceRepository) SetPDFPath(ctx context.Context, id, path string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE billing_invoices SET pdf_path = $1, updated_at = NOW() WHERE id = $2`,
		path, id)
	if err != nil {
		return fmt.Errorf("set invoice pdf_path: %w", err)
	}
	return nil
}

// ExistsForPeriod checks whether an invoice already exists for the given
// customer and billing period. Used to prevent duplicate invoice generation.
func (r *BillingInvoiceRepository) ExistsForPeriod(
	ctx context.Context, customerID string, periodStart, periodEnd time.Time,
) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM billing_invoices
			WHERE customer_id = $1
			  AND period_start = $2
			  AND period_end = $3
			  AND status != $4
		)`,
		customerID, periodStart, periodEnd, models.InvoiceStatusVoid,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check invoice exists for period: %w", err)
	}
	return exists, nil
}

// scanInvoice scans a single row into a BillingInvoice struct.
func (r *BillingInvoiceRepository) scanInvoice(row pgx.Row) (*models.BillingInvoice, error) {
	var inv models.BillingInvoice
	var lineItemsJSON []byte

	err := row.Scan(
		&inv.ID, &inv.CustomerID, &inv.InvoiceNumber, &inv.PeriodStart, &inv.PeriodEnd,
		&inv.Subtotal, &inv.TaxAmount, &inv.Total, &inv.Currency, &inv.Status,
		&lineItemsJSON, &inv.IssuedAt, &inv.PaidAt, &inv.PDFPath, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, sharederrors.ErrNotFound
		}
		return nil, fmt.Errorf("scan invoice: %w", err)
	}

	if err := json.Unmarshal(lineItemsJSON, &inv.LineItems); err != nil {
		return nil, fmt.Errorf("unmarshal line items: %w", err)
	}

	inv.HasPDF = inv.PDFPath != nil && *inv.PDFPath != ""
	return &inv, nil
}

// collectInvoices scans rows into a slice and extracts the next cursor.
func (r *BillingInvoiceRepository) collectInvoices(
	rows pgx.Rows, perPage int,
) ([]models.BillingInvoice, string, error) {
	var invoices []models.BillingInvoice
	for rows.Next() {
		var inv models.BillingInvoice
		var lineItemsJSON []byte

		err := rows.Scan(
			&inv.ID, &inv.CustomerID, &inv.InvoiceNumber, &inv.PeriodStart, &inv.PeriodEnd,
			&inv.Subtotal, &inv.TaxAmount, &inv.Total, &inv.Currency, &inv.Status,
			&lineItemsJSON, &inv.IssuedAt, &inv.PaidAt, &inv.PDFPath, &inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("scan invoice row: %w", err)
		}

		if err := json.Unmarshal(lineItemsJSON, &inv.LineItems); err != nil {
			return nil, "", fmt.Errorf("unmarshal line items: %w", err)
		}

		inv.HasPDF = inv.PDFPath != nil && *inv.PDFPath != ""
		invoices = append(invoices, inv)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate invoice rows: %w", err)
	}

	var nextCursor string
	if len(invoices) > perPage {
		invoices = invoices[:perPage]
		nextCursor = models.EncodeCursor(invoices[perPage-1].ID, "next")
	}

	return invoices, nextCursor, nil
}
