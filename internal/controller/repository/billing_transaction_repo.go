package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// BillingTransactionRepository provides database operations for the credit ledger.
type BillingTransactionRepository struct {
	db DB
}

// NewBillingTransactionRepository creates a new BillingTransactionRepository.
func NewBillingTransactionRepository(db DB) *BillingTransactionRepository {
	return &BillingTransactionRepository{db: db}
}

const billingTxSelectCols = `id, customer_id, type, amount, balance_after,
	description, reference_type, reference_id, idempotency_key, created_at`

func scanBillingTx(row pgx.Row) (models.BillingTransaction, error) {
	var bt models.BillingTransaction
	err := row.Scan(
		&bt.ID, &bt.CustomerID, &bt.Type, &bt.Amount, &bt.BalanceAfter,
		&bt.Description, &bt.ReferenceType, &bt.ReferenceID,
		&bt.IdempotencyKey, &bt.CreatedAt,
	)
	return bt, err
}

// CreditAccount atomically adds funds to a customer's balance and records
// the ledger entry. Returns the new transaction or an idempotency-matched
// existing transaction. Uses SELECT FOR UPDATE for serialization.
func (r *BillingTransactionRepository) CreditAccount(
	ctx context.Context,
	customerID string,
	amount int64,
	description string,
	idempotencyKey *string,
) (*models.BillingTransaction, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin credit tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if idempotencyKey != nil {
		if existing, idErr := findByIdempotencyKey(ctx, tx, *idempotencyKey); idErr == nil && existing != nil {
			return existing, nil
		}
	}

	newBalance, err := lockAndUpdateBalance(ctx, tx, customerID, amount)
	if err != nil {
		return nil, err
	}

	bt, err := insertTransaction(ctx, tx, customerID,
		models.BillingTxTypeCredit, amount, newBalance,
		description, nil, nil, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit credit tx: %w", err)
	}
	return bt, nil
}

// DebitAccount atomically deducts funds from a customer's balance.
func (r *BillingTransactionRepository) DebitAccount(
	ctx context.Context,
	customerID string,
	amount int64,
	description string,
	referenceType *string,
	referenceID *string,
	idempotencyKey *string,
) (*models.BillingTransaction, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin debit tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if idempotencyKey != nil {
		if existing, idErr := findByIdempotencyKey(ctx, tx, *idempotencyKey); idErr == nil && existing != nil {
			return existing, nil
		}
	}

	newBalance, err := lockAndUpdateBalance(ctx, tx, customerID, -amount)
	if err != nil {
		return nil, err
	}

	bt, err := insertTransaction(ctx, tx, customerID,
		models.BillingTxTypeDebit, amount, newBalance,
		description, referenceType, referenceID, idempotencyKey)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit debit tx: %w", err)
	}
	return bt, nil
}

// GetBalance returns the current balance for a customer.
func (r *BillingTransactionRepository) GetBalance(
	ctx context.Context, customerID string,
) (int64, error) {
	var balance int64
	q := `SELECT balance FROM customers WHERE id = $1`
	if err := r.db.QueryRow(ctx, q, customerID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("get customer balance: %w", err)
	}
	return balance, nil
}

// ListByCustomer returns paginated billing transactions for a customer.
func (r *BillingTransactionRepository) ListByCustomer(
	ctx context.Context,
	customerID string,
	filter models.PaginationParams,
) ([]models.BillingTransaction, bool, string, error) {
	args := []any{customerID}
	clause := "customer_id = $1"
	idx := 2

	cp := filter.DecodeCursor()
	if cp.LastID != "" {
		clause += fmt.Sprintf(" AND id < $%d", idx)
		args = append(args, cp.LastID)
		idx++
	}

	q := fmt.Sprintf(`SELECT %s FROM billing_transactions
		WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		billingTxSelectCols, clause, idx)
	args = append(args, filter.PerPage+1)

	txs, err := ScanRows(ctx, r.db, q, args,
		func(rows pgx.Rows) (models.BillingTransaction, error) {
			return scanBillingTx(rows)
		})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing billing transactions: %w", err)
	}

	hasMore := len(txs) > filter.PerPage
	if hasMore {
		txs = txs[:filter.PerPage]
	}
	var lastID string
	if len(txs) > 0 {
		lastID = txs[len(txs)-1].ID
	}
	return txs, hasMore, lastID, nil
}

// lockAndUpdateBalance locks the customer row and updates the balance.
func lockAndUpdateBalance(
	ctx context.Context, tx pgx.Tx, customerID string, delta int64,
) (int64, error) {
	var currentBalance int64
	lockQ := `SELECT balance FROM customers WHERE id = $1 FOR UPDATE`
	if err := tx.QueryRow(ctx, lockQ, customerID).Scan(&currentBalance); err != nil {
		return 0, fmt.Errorf("lock customer balance: %w", err)
	}

	newBalance := currentBalance + delta
	updateQ := `UPDATE customers SET balance = $1 WHERE id = $2`
	if _, err := tx.Exec(ctx, updateQ, newBalance, customerID); err != nil {
		return 0, fmt.Errorf("update customer balance: %w", err)
	}
	return newBalance, nil
}

// insertTransaction inserts a ledger entry and returns the created record.
func insertTransaction(
	ctx context.Context, tx pgx.Tx,
	customerID, txType string,
	amount, balanceAfter int64,
	description string,
	referenceType, referenceID, idempotencyKey *string,
) (*models.BillingTransaction, error) {
	q := `INSERT INTO billing_transactions
		(customer_id, type, amount, balance_after, description,
		 reference_type, reference_id, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + billingTxSelectCols

	var bt models.BillingTransaction
	err := tx.QueryRow(ctx, q,
		customerID, txType, amount, balanceAfter,
		description, referenceType, referenceID, idempotencyKey,
	).Scan(
		&bt.ID, &bt.CustomerID, &bt.Type, &bt.Amount, &bt.BalanceAfter,
		&bt.Description, &bt.ReferenceType, &bt.ReferenceID,
		&bt.IdempotencyKey, &bt.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert %s transaction: %w", txType, err)
	}
	return &bt, nil
}

func findByIdempotencyKey(
	ctx context.Context, tx pgx.Tx, key string,
) (*models.BillingTransaction, error) {
	q := `SELECT ` + billingTxSelectCols + `
		FROM billing_transactions WHERE idempotency_key = $1`
	var bt models.BillingTransaction
	err := tx.QueryRow(ctx, q, key).Scan(
		&bt.ID, &bt.CustomerID, &bt.Type, &bt.Amount, &bt.BalanceAfter,
		&bt.Description, &bt.ReferenceType, &bt.ReferenceID,
		&bt.IdempotencyKey, &bt.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &bt, nil
}
