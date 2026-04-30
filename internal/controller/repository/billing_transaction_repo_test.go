package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type billingTxQueryCall struct {
	sql  string
	args []any
}

type mockBillingTxDB struct {
	tx *mockBillingTxTx
}

func (m *mockBillingTxDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func (m *mockBillingTxDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockBillingTxDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *mockBillingTxDB) Begin(context.Context) (pgx.Tx, error) {
	return m.tx, nil
}

type mockBillingTxTx struct {
	queryCalls []billingTxQueryCall
	rows       []pgx.Row
	commitErr  error
}

func (m *mockBillingTxTx) Begin(context.Context) (pgx.Tx, error) {
	return m, nil
}

func (m *mockBillingTxTx) Commit(context.Context) error {
	return m.commitErr
}

func (m *mockBillingTxTx) Rollback(context.Context) error {
	return nil
}

func (m *mockBillingTxTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (m *mockBillingTxTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (m *mockBillingTxTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *mockBillingTxTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (m *mockBillingTxTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (m *mockBillingTxTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockBillingTxTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	m.queryCalls = append(m.queryCalls, billingTxQueryCall{
		sql:  sql,
		args: append([]any(nil), args...),
	})
	if len(m.rows) == 0 {
		return mockBillingTxRow{err: pgx.ErrNoRows}
	}
	row := m.rows[0]
	m.rows = m.rows[1:]
	return row
}

func (m *mockBillingTxTx) Conn() *pgx.Conn {
	return nil
}

type mockBillingTxRow struct {
	tx      models.BillingTransaction
	balance int64
	err     error
}

func (m mockBillingTxRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if len(dest) == 1 {
		if balance, ok := dest[0].(*int64); ok {
			*balance = m.balance
			return nil
		}
	}
	if len(dest) == 10 {
		if id, ok := dest[0].(*string); ok {
			*id = m.tx.ID
		}
		if customerID, ok := dest[1].(*string); ok {
			*customerID = m.tx.CustomerID
		}
		if txType, ok := dest[2].(*string); ok {
			*txType = m.tx.Type
		}
		if amount, ok := dest[3].(*int64); ok {
			*amount = m.tx.Amount
		}
		if balanceAfter, ok := dest[4].(*int64); ok {
			*balanceAfter = m.tx.BalanceAfter
		}
		if description, ok := dest[5].(*string); ok {
			*description = m.tx.Description
		}
		if referenceType, ok := dest[6].(**string); ok {
			*referenceType = m.tx.ReferenceType
		}
		if referenceID, ok := dest[7].(**string); ok {
			*referenceID = m.tx.ReferenceID
		}
		if idempotencyKey, ok := dest[8].(**string); ok {
			*idempotencyKey = m.tx.IdempotencyKey
		}
		if createdAt, ok := dest[9].(*time.Time); ok {
			*createdAt = m.tx.CreatedAt
		}
	}
	return nil
}

func TestBillingTransactionRepository_IdempotencyLookupIsCustomerScoped(t *testing.T) {
	tests := []struct {
		name       string
		invoke     func(context.Context, *BillingTransactionRepository, *string) (*models.BillingTransaction, error)
		wantType   string
		wantAmount int64
	}{
		{
			name: "credit account scopes idempotency query",
			invoke: func(ctx context.Context, repo *BillingTransactionRepository, key *string) (*models.BillingTransaction, error) {
				return repo.CreditAccount(ctx, "cust-1", 25, "top-up", key)
			},
			wantType:   models.BillingTxTypeCredit,
			wantAmount: 25,
		},
		{
			name: "debit account scopes idempotency query",
			invoke: func(ctx context.Context, repo *BillingTransactionRepository, key *string) (*models.BillingTransaction, error) {
				return repo.DebitAccount(ctx, "cust-1", 25, "hourly charge", nil, nil, key)
			},
			wantType:   models.BillingTxTypeDebit,
			wantAmount: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idempotencyKey := "shared-key"
			mockTx := &mockBillingTxTx{
				rows: []pgx.Row{
					mockBillingTxRow{err: pgx.ErrNoRows},
					mockBillingTxRow{balance: 100},
					mockBillingTxRow{tx: models.BillingTransaction{
						ID:             "tx-1",
						CustomerID:     "cust-1",
						Type:           tt.wantType,
						Amount:         tt.wantAmount,
						BalanceAfter:   125,
						Description:    "ledger entry",
						IdempotencyKey: &idempotencyKey,
						CreatedAt:      time.Unix(0, 0).UTC(),
					}},
				},
			}
			repo := NewBillingTransactionRepository(&mockBillingTxDB{tx: mockTx})

			tx, err := tt.invoke(context.Background(), repo, &idempotencyKey)

			require.NoError(t, err)
			require.NotNil(t, tx)
			require.NotEmpty(t, mockTx.queryCalls)
			assert.Contains(t, strings.ToLower(mockTx.queryCalls[0].sql), "customer_id = $1")
			assert.Equal(t, []any{"cust-1", idempotencyKey}, mockTx.queryCalls[0].args)
		})
	}
}

func TestBillingTransactionRepository_IdempotencyLookupErrorAbortsMutation(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	idempotencyKey := "gateway-event-1"

	tests := []struct {
		name   string
		invoke func(context.Context, *BillingTransactionRepository, *string) (*models.BillingTransaction, error)
	}{
		{
			name: "credit account aborts on idempotency lookup error",
			invoke: func(ctx context.Context, repo *BillingTransactionRepository, key *string) (*models.BillingTransaction, error) {
				return repo.CreditAccount(ctx, "cust-1", 50, "top-up", key)
			},
		},
		{
			name: "debit account aborts on idempotency lookup error",
			invoke: func(ctx context.Context, repo *BillingTransactionRepository, key *string) (*models.BillingTransaction, error) {
				return repo.DebitAccount(ctx, "cust-1", 50, "hourly charge", nil, nil, key)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTx := &mockBillingTxTx{
				rows: []pgx.Row{
					mockBillingTxRow{err: lookupErr},
					mockBillingTxRow{balance: 100},
					mockBillingTxRow{tx: models.BillingTransaction{
						ID:             "tx-created",
						CustomerID:     "cust-1",
						Amount:         50,
						BalanceAfter:   150,
						IdempotencyKey: &idempotencyKey,
						CreatedAt:      time.Unix(0, 0).UTC(),
					}},
				},
			}
			repo := NewBillingTransactionRepository(&mockBillingTxDB{tx: mockTx})

			tx, err := tt.invoke(context.Background(), repo, &idempotencyKey)

			require.Error(t, err)
			require.Nil(t, tx)
			assert.ErrorIs(t, err, lookupErr)
			require.Len(t, mockTx.queryCalls, 1)
			assert.Contains(t, strings.ToLower(mockTx.queryCalls[0].sql), "idempotency_key")
		})
	}
}

func TestBillingTransactionRepository_IdempotencyMismatchReturnsConflict(t *testing.T) {
	idempotencyKey := "gateway-event-1"
	refType := models.BillingRefTypeVMUsage
	refID := "vm-1"

	tests := []struct {
		name       string
		existing   models.BillingTransaction
		invoke     func(context.Context, *BillingTransactionRepository, *string) (*models.BillingTransaction, error)
		wantErrIs  error
		wantErrMsg string
	}{
		{
			name: "credit account rejects amount mismatch",
			existing: models.BillingTransaction{
				ID:             "tx-existing",
				CustomerID:     "cust-1",
				Type:           models.BillingTxTypeCredit,
				Amount:         25,
				BalanceAfter:   125,
				IdempotencyKey: &idempotencyKey,
			},
			invoke: func(ctx context.Context, repo *BillingTransactionRepository, key *string) (*models.BillingTransaction, error) {
				return repo.CreditAccount(ctx, "cust-1", 50, "top-up", key)
			},
			wantErrIs:  sharederrors.ErrConflict,
			wantErrMsg: "idempotency",
		},
		{
			name: "debit account rejects reference mismatch",
			existing: models.BillingTransaction{
				ID:             "tx-existing",
				CustomerID:     "cust-1",
				Type:           models.BillingTxTypeDebit,
				Amount:         50,
				BalanceAfter:   50,
				ReferenceType:  nil,
				ReferenceID:    nil,
				IdempotencyKey: &idempotencyKey,
			},
			invoke: func(ctx context.Context, repo *BillingTransactionRepository, key *string) (*models.BillingTransaction, error) {
				return repo.DebitAccount(ctx, "cust-1", 50, "hourly charge", &refType, &refID, key)
			},
			wantErrIs:  sharederrors.ErrConflict,
			wantErrMsg: "idempotency",
		},
		{
			name: "credit account rejects type mismatch",
			existing: models.BillingTransaction{
				ID:             "tx-existing",
				CustomerID:     "cust-1",
				Type:           models.BillingTxTypeDebit,
				Amount:         50,
				BalanceAfter:   50,
				IdempotencyKey: &idempotencyKey,
			},
			invoke: func(ctx context.Context, repo *BillingTransactionRepository, key *string) (*models.BillingTransaction, error) {
				return repo.CreditAccount(ctx, "cust-1", 50, "top-up", key)
			},
			wantErrIs:  sharederrors.ErrConflict,
			wantErrMsg: "idempotency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTx := &mockBillingTxTx{
				rows: []pgx.Row{mockBillingTxRow{tx: tt.existing}},
			}
			repo := NewBillingTransactionRepository(&mockBillingTxDB{tx: mockTx})

			tx, err := tt.invoke(context.Background(), repo, &idempotencyKey)

			require.Error(t, err)
			require.Nil(t, tx)
			assert.ErrorIs(t, err, tt.wantErrIs)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
			require.Len(t, mockTx.queryCalls, 1)
		})
	}
}
