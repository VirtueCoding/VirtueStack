package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// BillingPaymentRepository provides database operations for billing payments.
type BillingPaymentRepository struct {
	db DB
}

// NewBillingPaymentRepository creates a new BillingPaymentRepository.
func NewBillingPaymentRepository(db DB) *BillingPaymentRepository {
	return &BillingPaymentRepository{db: db}
}

const billingPaymentSelectCols = `id, customer_id, gateway, gateway_payment_id,
	amount, currency, status, reuse_key, metadata, created_at, updated_at`

func scanBillingPayment(row pgx.Row) (models.BillingPayment, error) {
	var bp models.BillingPayment
	err := row.Scan(
		&bp.ID, &bp.CustomerID, &bp.Gateway, &bp.GatewayPaymentID,
		&bp.Amount, &bp.Currency, &bp.Status, &bp.ReuseKey,
		&bp.Metadata, &bp.CreatedAt, &bp.UpdatedAt,
	)
	return bp, err
}

// Create inserts a new billing payment record.
func (r *BillingPaymentRepository) Create(ctx context.Context, payment *models.BillingPayment) error {
	q := `INSERT INTO billing_payments
		(customer_id, gateway, gateway_payment_id, amount, currency, status, reuse_key, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + billingPaymentSelectCols

	row := r.db.QueryRow(ctx, q,
		payment.CustomerID, payment.Gateway, payment.GatewayPaymentID,
		payment.Amount, payment.Currency, payment.Status,
		payment.ReuseKey, payment.Metadata,
	)
	created, err := scanBillingPayment(row)
	if err != nil {
		return fmt.Errorf("creating billing payment: %w", err)
	}
	*payment = created
	return nil
}

// Update updates a billing payment's status and metadata.
func (r *BillingPaymentRepository) Update(ctx context.Context, payment *models.BillingPayment) error {
	q := `UPDATE billing_payments
		SET status = $1, metadata = $2, gateway_payment_id = $3, updated_at = NOW()
		WHERE id = $4
		RETURNING ` + billingPaymentSelectCols

	row := r.db.QueryRow(ctx, q,
		payment.Status, payment.Metadata,
		payment.GatewayPaymentID, payment.ID,
	)
	updated, err := scanBillingPayment(row)
	if err != nil {
		return fmt.Errorf("updating billing payment %s: %w", payment.ID, err)
	}
	*payment = updated
	return nil
}

// GetByID returns a billing payment by its UUID.
func (r *BillingPaymentRepository) GetByID(ctx context.Context, id string) (*models.BillingPayment, error) {
	q := `SELECT ` + billingPaymentSelectCols + ` FROM billing_payments WHERE id = $1`
	bp, err := ScanRow(ctx, r.db, q, []any{id}, scanBillingPayment)
	if err != nil {
		return nil, fmt.Errorf("getting billing payment %s: %w", id, err)
	}
	return &bp, nil
}

// GetByGatewayPaymentID returns a billing payment by gateway and gateway payment ID.
func (r *BillingPaymentRepository) GetByGatewayPaymentID(
	ctx context.Context, gateway, gatewayPaymentID string,
) (*models.BillingPayment, error) {
	q := `SELECT ` + billingPaymentSelectCols + `
		FROM billing_payments WHERE gateway = $1 AND gateway_payment_id = $2`
	bp, err := ScanRow(ctx, r.db, q, []any{gateway, gatewayPaymentID}, scanBillingPayment)
	if err != nil {
		return nil, fmt.Errorf("getting payment by gateway %s/%s: %w", gateway, gatewayPaymentID, err)
	}
	return &bp, nil
}

// ListByCustomer returns paginated billing payments for a customer.
func (r *BillingPaymentRepository) ListByCustomer(
	ctx context.Context,
	customerID string,
	filter models.PaginationParams,
) ([]models.BillingPayment, bool, string, error) {
	args := []any{customerID}
	clause := "customer_id = $1"
	idx := 2

	cp := filter.DecodeCursor()
	if cp.LastID != "" {
		clause += fmt.Sprintf(" AND id < $%d", idx)
		args = append(args, cp.LastID)
		idx++
	}

	q := fmt.Sprintf(`SELECT %s FROM billing_payments
		WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		billingPaymentSelectCols, clause, idx)
	args = append(args, filter.PerPage+1)

	payments, err := ScanRows(ctx, r.db, q, args,
		func(rows pgx.Rows) (models.BillingPayment, error) {
			return scanBillingPayment(rows)
		})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing billing payments: %w", err)
	}

	hasMore := len(payments) > filter.PerPage
	if hasMore {
		payments = payments[:filter.PerPage]
	}
	var lastID string
	if len(payments) > 0 {
		lastID = payments[len(payments)-1].ID
	}
	return payments, hasMore, lastID, nil
}
