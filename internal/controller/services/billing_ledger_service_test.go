package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBillingTransactionRepo struct {
	creditAccountFunc  func(ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error)
	debitAccountFunc   func(ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string) (*models.BillingTransaction, error)
	getBalanceFunc     func(ctx context.Context, customerID string) (int64, error)
	listByCustomerFunc func(ctx context.Context, customerID string, filter models.PaginationParams) ([]models.BillingTransaction, bool, string, error)
}

func (m *mockBillingTransactionRepo) CreditAccount(ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error) {
	return m.creditAccountFunc(ctx, customerID, amount, description, idempotencyKey)
}

func (m *mockBillingTransactionRepo) DebitAccount(ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string) (*models.BillingTransaction, error) {
	return m.debitAccountFunc(ctx, customerID, amount, description, referenceType, referenceID, idempotencyKey)
}

func (m *mockBillingTransactionRepo) GetBalance(ctx context.Context, customerID string) (int64, error) {
	return m.getBalanceFunc(ctx, customerID)
}

func (m *mockBillingTransactionRepo) ListByCustomer(ctx context.Context, customerID string, filter models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
	return m.listByCustomerFunc(ctx, customerID, filter)
}

func newTestLedgerService(repo *mockBillingTransactionRepo) *BillingLedgerService {
	return NewBillingLedgerService(BillingLedgerServiceConfig{
		TransactionRepo: repo,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestBillingLedgerService_CreditAccount(t *testing.T) {
	tests := []struct {
		name      string
		amount    int64
		setupRepo func() *mockBillingTransactionRepo
		wantErr   bool
		checkErr  func(t *testing.T, err error)
		checkTx   func(t *testing.T, tx *models.BillingTransaction)
	}{
		{
			name:   "positive amount succeeds",
			amount: 1000,
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					creditAccountFunc: func(_ context.Context, customerID string, amount int64, _ string, _ *string) (*models.BillingTransaction, error) {
						return &models.BillingTransaction{
							ID:           "tx-1",
							CustomerID:   customerID,
							Type:         models.BillingTxTypeCredit,
							Amount:       amount,
							BalanceAfter: 1000,
						}, nil
					},
				}
			},
			wantErr: false,
			checkTx: func(t *testing.T, tx *models.BillingTransaction) {
				assert.Equal(t, "tx-1", tx.ID)
				assert.Equal(t, int64(1000), tx.Amount)
				assert.Equal(t, int64(1000), tx.BalanceAfter)
				assert.Equal(t, models.BillingTxTypeCredit, tx.Type)
			},
		},
		{
			name:   "zero amount returns validation error",
			amount: 0,
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{}
			},
			wantErr: true,
			checkErr: func(t *testing.T, err error) {
				var ve *sharederrors.ValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, "amount", ve.Field)
			},
		},
		{
			name:   "negative amount returns validation error",
			amount: -500,
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{}
			},
			wantErr: true,
			checkErr: func(t *testing.T, err error) {
				var ve *sharederrors.ValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, "amount", ve.Field)
			},
		},
		{
			name:   "repo error propagated",
			amount: 500,
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					creditAccountFunc: func(_ context.Context, _ string, _ int64, _ string, _ *string) (*models.BillingTransaction, error) {
						return nil, errors.New("db connection failed")
					},
				}
			},
			wantErr: true,
			checkErr: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "credit account")
				assert.Contains(t, err.Error(), "db connection failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestLedgerService(tt.setupRepo())
			tx, err := svc.CreditAccount(context.Background(), "cust-1", tt.amount, "test credit", nil)
			if tt.wantErr {
				require.Error(t, err)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
				return
			}
			require.NoError(t, err)
			if tt.checkTx != nil {
				tt.checkTx(t, tx)
			}
		})
	}
}

func TestBillingLedgerService_DebitAccount(t *testing.T) {
	tests := []struct {
		name      string
		amount    int64
		setupRepo func() *mockBillingTransactionRepo
		wantErr   bool
		checkErr  func(t *testing.T, err error)
		checkTx   func(t *testing.T, tx *models.BillingTransaction)
	}{
		{
			name:   "positive amount succeeds",
			amount: 300,
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					debitAccountFunc: func(_ context.Context, customerID string, amount int64, _ string, _, _, _ *string) (*models.BillingTransaction, error) {
						return &models.BillingTransaction{
							ID:           "tx-2",
							CustomerID:   customerID,
							Type:         models.BillingTxTypeDebit,
							Amount:       amount,
							BalanceAfter: 700,
						}, nil
					},
				}
			},
			wantErr: false,
			checkTx: func(t *testing.T, tx *models.BillingTransaction) {
				assert.Equal(t, "tx-2", tx.ID)
				assert.Equal(t, int64(300), tx.Amount)
				assert.Equal(t, int64(700), tx.BalanceAfter)
			},
		},
		{
			name:   "zero amount returns validation error",
			amount: 0,
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{}
			},
			wantErr: true,
			checkErr: func(t *testing.T, err error) {
				var ve *sharederrors.ValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, "amount", ve.Field)
			},
		},
		{
			name:   "repo error propagated",
			amount: 100,
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					debitAccountFunc: func(_ context.Context, _ string, _ int64, _ string, _, _, _ *string) (*models.BillingTransaction, error) {
						return nil, errors.New("insufficient balance")
					},
				}
			},
			wantErr: true,
			checkErr: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "debit account")
				assert.Contains(t, err.Error(), "insufficient balance")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestLedgerService(tt.setupRepo())
			refType := "vm_usage"
			refID := "vm-1"
			tx, err := svc.DebitAccount(context.Background(), "cust-1", tt.amount, "test debit", &refType, &refID, nil)
			if tt.wantErr {
				require.Error(t, err)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
				return
			}
			require.NoError(t, err)
			if tt.checkTx != nil {
				tt.checkTx(t, tx)
			}
		})
	}
}

func TestBillingLedgerService_GetBalance(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func() *mockBillingTransactionRepo
		wantBalance int64
		wantErr     bool
	}{
		{
			name: "returns balance from repo",
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					getBalanceFunc: func(_ context.Context, _ string) (int64, error) {
						return 5000, nil
					},
				}
			},
			wantBalance: 5000,
			wantErr:     false,
		},
		{
			name: "repo error propagated",
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					getBalanceFunc: func(_ context.Context, _ string) (int64, error) {
						return 0, errors.New("customer not found")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestLedgerService(tt.setupRepo())
			balance, err := svc.GetBalance(context.Background(), "cust-1")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBalance, balance)
		})
	}
}

func TestBillingLedgerService_GetTransactionHistory(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func() *mockBillingTransactionRepo
		wantCount   int
		wantHasMore bool
		wantLastID  string
		wantErr     bool
	}{
		{
			name: "returns paginated results",
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					listByCustomerFunc: func(_ context.Context, _ string, _ models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
						return []models.BillingTransaction{
							{ID: "tx-1", Amount: 100},
							{ID: "tx-2", Amount: 200},
						}, true, "tx-2", nil
					},
				}
			},
			wantCount:   2,
			wantHasMore: true,
			wantLastID:  "tx-2",
			wantErr:     false,
		},
		{
			name: "repo error propagated",
			setupRepo: func() *mockBillingTransactionRepo {
				return &mockBillingTransactionRepo{
					listByCustomerFunc: func(_ context.Context, _ string, _ models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
						return nil, false, "", errors.New("query failed")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestLedgerService(tt.setupRepo())
			txs, hasMore, lastID, err := svc.GetTransactionHistory(context.Background(), "cust-1", models.PaginationParams{PerPage: 20})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, txs, tt.wantCount)
			assert.Equal(t, tt.wantHasMore, hasMore)
			assert.Equal(t, tt.wantLastID, lastID)
		})
	}
}
