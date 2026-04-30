package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
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

// PaymentCompletionCredit describes a payment completion and matching ledger credit.
type PaymentCompletionCredit struct {
	PaymentID        string
	Gateway          string
	GatewayPaymentID string
	CustomerID       string
	Amount           int64
	Description      string
	IdempotencyKey   string
}

// PayPalCaptureCredit describes a PayPal capture claim and matching ledger credit.
type PayPalCaptureCredit struct {
	PaymentID      string
	OrderID        string
	CaptureID      string
	CustomerID     string
	Amount         int64
	Description    string
	IdempotencyKey string
}

// PaymentRefundDebit describes a payment refund and matching ledger debit.
type PaymentRefundDebit struct {
	PaymentID      string
	CustomerID     string
	Amount         int64
	Description    string
	IdempotencyKey string
}

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

// UpdateStatus updates a billing payment's status and optionally the gateway payment ID.
func (r *BillingPaymentRepository) UpdateStatus(
	ctx context.Context, id, status string, gatewayPaymentID *string,
) error {
	q := `UPDATE billing_payments
		SET status = $1, gateway_payment_id = COALESCE($2, gateway_payment_id), updated_at = NOW()
		WHERE id = $3`

	tag, err := r.db.Exec(ctx, q, status, gatewayPaymentID, id)
	if err != nil {
		return fmt.Errorf("updating payment status %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("payment %s not found", id)
	}
	return nil
}

// CompleteWithGatewayPaymentID atomically claims a gateway payment ID and
// marks the local payment completed. It returns false if another local payment
// already claimed the gateway payment ID.
func (r *BillingPaymentRepository) CompleteWithGatewayPaymentID(
	ctx context.Context, id, gateway, gatewayPaymentID string,
) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin payment completion tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := lockGatewayPaymentID(ctx, tx, gateway, gatewayPaymentID); err != nil {
		return false, err
	}
	canClaim, err := canClaimLocalPayment(ctx, tx, id, gateway, gatewayPaymentID)
	if err != nil || !canClaim {
		return false, err
	}
	claimed, err := hasOtherGatewayPayment(ctx, tx, id, gateway, gatewayPaymentID)
	if err != nil || claimed {
		return false, err
	}
	if err := updatePaymentCompletedTx(ctx, tx, id, gateway, gatewayPaymentID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit payment completion tx: %w", err)
	}
	return true, nil
}

// CompleteWithGatewayPaymentIDAndCredit atomically completes a payment and
// credits the ledger in the same database transaction.
func (r *BillingPaymentRepository) CompleteWithGatewayPaymentIDAndCredit(
	ctx context.Context, req PaymentCompletionCredit,
) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin payment credit tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	claimed, err := claimGatewayPaymentTx(
		ctx, tx, req.PaymentID, req.Gateway, req.GatewayPaymentID)
	if err != nil || !claimed {
		return false, err
	}
	if _, err := creditAccountTx(
		ctx, tx, req.CustomerID, req.Amount, req.Description, &req.IdempotencyKey); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit payment credit tx: %w", err)
	}
	return true, nil
}

// CompletePayPalCapture records a PayPal capture ID without replacing the
// order ID stored in gateway_payment_id.
func (r *BillingPaymentRepository) CompletePayPalCapture(
	ctx context.Context, id, orderID, captureID string,
) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin paypal capture tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := lockGatewayPaymentID(ctx, tx, "paypal_capture", captureID); err != nil {
		return false, err
	}
	canClaim, err := canClaimPayPalCapture(ctx, tx, id, orderID, captureID)
	if err != nil || !canClaim {
		return false, err
	}
	claimed, err := hasOtherPayPalCapture(ctx, tx, id, captureID)
	if err != nil || claimed {
		return false, err
	}
	if err := updatePayPalCaptureCompletedTx(ctx, tx, id, orderID, captureID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit paypal capture tx: %w", err)
	}
	return true, nil
}

// CompletePayPalCaptureAndCredit atomically records a PayPal capture and
// credits the ledger in the same database transaction.
func (r *BillingPaymentRepository) CompletePayPalCaptureAndCredit(
	ctx context.Context, req PayPalCaptureCredit,
) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin paypal capture credit tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	claimed, err := claimPayPalCaptureTx(ctx, tx, req.PaymentID, req.OrderID, req.CaptureID)
	if err != nil || !claimed {
		return false, err
	}
	if _, err := creditAccountTx(
		ctx, tx, req.CustomerID, req.Amount, req.Description, &req.IdempotencyKey); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit paypal capture credit tx: %w", err)
	}
	return true, nil
}

// MarkRefundedAndDebit atomically marks a payment refunded and debits the ledger.
func (r *BillingPaymentRepository) MarkRefundedAndDebit(
	ctx context.Context, req PaymentRefundDebit,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin refund debit tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := updatePaymentRefundedTx(ctx, tx, req.PaymentID, req.CustomerID); err != nil {
		return err
	}
	refType := models.BillingRefTypeRefund
	if _, err := debitAccountTx(
		ctx, tx, req.CustomerID, req.Amount, req.Description,
		&refType, &req.PaymentID, &req.IdempotencyKey); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit refund debit tx: %w", err)
	}
	return nil
}

func claimGatewayPaymentTx(
	ctx context.Context, tx pgx.Tx, id, gateway, gatewayPaymentID string,
) (bool, error) {
	if err := lockGatewayPaymentID(ctx, tx, gateway, gatewayPaymentID); err != nil {
		return false, err
	}
	canClaim, err := canClaimLocalPayment(ctx, tx, id, gateway, gatewayPaymentID)
	if err != nil || !canClaim {
		return false, err
	}
	claimed, err := hasOtherGatewayPayment(ctx, tx, id, gateway, gatewayPaymentID)
	if err != nil || claimed {
		return false, err
	}
	return true, updatePaymentCompletedTx(ctx, tx, id, gateway, gatewayPaymentID)
}

func claimPayPalCaptureTx(
	ctx context.Context, tx pgx.Tx, id, orderID, captureID string,
) (bool, error) {
	if err := lockGatewayPaymentID(ctx, tx, "paypal_capture", captureID); err != nil {
		return false, err
	}
	canClaim, err := canClaimPayPalCapture(ctx, tx, id, orderID, captureID)
	if err != nil || !canClaim {
		return false, err
	}
	claimed, err := hasOtherPayPalCapture(ctx, tx, id, captureID)
	if err != nil || claimed {
		return false, err
	}
	return true, updatePayPalCaptureCompletedTx(ctx, tx, id, orderID, captureID)
}

func lockGatewayPaymentID(ctx context.Context, tx pgx.Tx, gateway, gatewayPaymentID string) error {
	_, err := tx.Exec(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtext($1 || ':' || $2))",
		gateway,
		gatewayPaymentID,
	)
	if err != nil {
		return fmt.Errorf("lock gateway payment id: %w", err)
	}
	return nil
}

func canClaimPayPalCapture(
	ctx context.Context, tx pgx.Tx, id, orderID, captureID string,
) (bool, error) {
	var status string
	var currentOrderID *string
	var currentCaptureID *string
	q := `SELECT status, gateway_payment_id, metadata->>'paypal_capture_id'
		FROM billing_payments
		WHERE id = $1 AND gateway = $2 FOR UPDATE`
	err := tx.QueryRow(ctx, q, id, models.PaymentGatewayPayPal).Scan(
		&status, &currentOrderID, &currentCaptureID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("payment %s not found: %w", id, sharederrors.ErrNotFound)
		}
		return false, fmt.Errorf("lock paypal payment: %w", err)
	}
	if currentOrderID == nil || *currentOrderID != orderID {
		return false, nil
	}
	if status == models.PaymentStatusCompleted {
		return currentCaptureID != nil && *currentCaptureID == captureID, nil
	}
	if status == models.PaymentStatusRefunded || status == models.PaymentStatusFailed {
		return false, nil
	}
	if currentCaptureID != nil && *currentCaptureID != captureID {
		return false, nil
	}
	return true, nil
}

func canClaimLocalPayment(
	ctx context.Context, tx pgx.Tx, id, gateway, gatewayPaymentID string,
) (bool, error) {
	var status string
	var currentGatewayID *string
	q := `SELECT status, gateway_payment_id FROM billing_payments
		WHERE id = $1 AND gateway = $2 FOR UPDATE`
	err := tx.QueryRow(ctx, q, id, gateway).Scan(&status, &currentGatewayID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("payment %s not found: %w", id, sharederrors.ErrNotFound)
		}
		return false, fmt.Errorf("lock local payment: %w", err)
	}
	if status == models.PaymentStatusCompleted &&
		(currentGatewayID == nil || *currentGatewayID != gatewayPaymentID) {
		return false, nil
	}
	if status == models.PaymentStatusRefunded || status == models.PaymentStatusFailed {
		return false, nil
	}
	return true, nil
}

func hasOtherGatewayPayment(
	ctx context.Context, tx pgx.Tx, id, gateway, gatewayPaymentID string,
) (bool, error) {
	var existingID string
	q := `SELECT id FROM billing_payments
		WHERE gateway = $1 AND gateway_payment_id = $2 AND id <> $3
		LIMIT 1`
	err := tx.QueryRow(ctx, q, gateway, gatewayPaymentID, id).Scan(&existingID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("check gateway payment claim: %w", err)
}

func hasOtherPayPalCapture(
	ctx context.Context, tx pgx.Tx, id, captureID string,
) (bool, error) {
	var existingID string
	q := `SELECT id FROM billing_payments
		WHERE gateway = $1 AND metadata->>'paypal_capture_id' = $2 AND id <> $3
		LIMIT 1`
	err := tx.QueryRow(ctx, q, models.PaymentGatewayPayPal, captureID, id).Scan(&existingID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("check paypal capture claim: %w", err)
}

func updatePaymentCompletedTx(
	ctx context.Context, tx pgx.Tx, id, gateway, gatewayPaymentID string,
) error {
	q := `UPDATE billing_payments
		SET status = $1, gateway_payment_id = $2, updated_at = NOW()
		WHERE id = $3 AND gateway = $4`
	tag, err := tx.Exec(ctx, q,
		models.PaymentStatusCompleted, gatewayPaymentID, id, gateway)
	if err != nil {
		return fmt.Errorf("complete payment %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("payment %s not found: %w", id, sharederrors.ErrNotFound)
	}
	return nil
}

func updatePayPalCaptureCompletedTx(
	ctx context.Context, tx pgx.Tx, id, orderID, captureID string,
) error {
	q := `UPDATE billing_payments
		SET status = $1,
			gateway_payment_id = $2,
			metadata = jsonb_set(
				COALESCE(metadata, '{}'::jsonb),
				'{paypal_capture_id}',
				to_jsonb($3::text),
				true
			),
			updated_at = NOW()
		WHERE id = $4 AND gateway = $5`
	tag, err := tx.Exec(ctx, q,
		models.PaymentStatusCompleted, orderID, captureID, id, models.PaymentGatewayPayPal)
	if err != nil {
		return fmt.Errorf("complete paypal capture %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("payment %s not found: %w", id, sharederrors.ErrNotFound)
	}
	return nil
}

func updatePaymentRefundedTx(ctx context.Context, tx pgx.Tx, id, customerID string) error {
	q := `UPDATE billing_payments
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND customer_id = $3`
	tag, err := tx.Exec(ctx, q, models.PaymentStatusRefunded, id, customerID)
	if err != nil {
		return fmt.Errorf("refund payment %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("payment %s not found: %w", id, sharederrors.ErrNotFound)
	}
	return nil
}

func creditAccountTx(
	ctx context.Context,
	tx pgx.Tx,
	customerID string,
	amount int64,
	description string,
	idempotencyKey *string,
) (*models.BillingTransaction, error) {
	existing, err := findMatchingIdempotencyKey(
		ctx, tx, customerID, *idempotencyKey, models.BillingTxTypeCredit, amount, nil, nil)
	if err != nil || existing != nil {
		return existing, err
	}
	newBalance, err := lockAndUpdateBalance(ctx, tx, customerID, amount)
	if err != nil {
		return nil, err
	}
	return insertTransaction(ctx, tx, customerID,
		models.BillingTxTypeCredit, amount, newBalance,
		description, nil, nil, idempotencyKey)
}

func debitAccountTx(
	ctx context.Context,
	tx pgx.Tx,
	customerID string,
	amount int64,
	description string,
	referenceType, referenceID, idempotencyKey *string,
) (*models.BillingTransaction, error) {
	existing, err := findMatchingIdempotencyKey(
		ctx, tx, customerID, *idempotencyKey,
		models.BillingTxTypeDebit, amount, referenceType, referenceID)
	if err != nil || existing != nil {
		return existing, err
	}
	newBalance, err := lockAndUpdateBalance(ctx, tx, customerID, -amount)
	if err != nil {
		return nil, err
	}
	return insertTransaction(ctx, tx, customerID,
		models.BillingTxTypeDebit, amount, newBalance,
		description, referenceType, referenceID, idempotencyKey)
}

// BillingPaymentListFilter holds optional filters for listing payments.
type BillingPaymentListFilter struct {
	CustomerID *string
	Gateway    *string
	Status     *string
	models.PaginationParams
}

// ListAll returns paginated payments with optional filters (admin use).
func (r *BillingPaymentRepository) ListAll(
	ctx context.Context, filter BillingPaymentListFilter,
) ([]models.BillingPayment, bool, string, error) {
	var args []any
	argPos := 1
	conditions := []string{}

	if filter.CustomerID != nil {
		conditions = append(conditions, fmt.Sprintf("customer_id = $%d", argPos))
		args = append(args, *filter.CustomerID)
		argPos++
	}
	if filter.Gateway != nil {
		conditions = append(conditions, fmt.Sprintf("gateway = $%d", argPos))
		args = append(args, *filter.Gateway)
		argPos++
	}
	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *filter.Status)
		argPos++
	}

	cp := filter.DecodeCursor()
	if cp.LastID != "" {
		conditions = append(conditions, fmt.Sprintf("id < $%d", argPos))
		args = append(args, cp.LastID)
		argPos++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	q := fmt.Sprintf(
		"SELECT %s FROM billing_payments %s ORDER BY created_at DESC, id DESC LIMIT $%d",
		billingPaymentSelectCols, where, argPos,
	)
	args = append(args, filter.PerPage+1)

	pymnts, err := ScanRows(ctx, r.db, q, args,
		func(rows pgx.Rows) (models.BillingPayment, error) {
			return scanBillingPayment(rows)
		})
	if err != nil {
		return nil, false, "", fmt.Errorf("list all payments: %w", err)
	}

	hasMore := len(pymnts) > filter.PerPage
	if hasMore {
		pymnts = pymnts[:filter.PerPage]
	}

	var lastID string
	if len(pymnts) > 0 {
		lastID = pymnts[len(pymnts)-1].ID
	}

	return pymnts, hasMore, lastID, nil
}
