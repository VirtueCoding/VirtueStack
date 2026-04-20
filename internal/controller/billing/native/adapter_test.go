package native

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAdapterTxRepo struct {
	creditAccountFunc  func(ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error)
	debitAccountFunc   func(ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string) (*models.BillingTransaction, error)
	getBalanceFunc     func(ctx context.Context, customerID string) (int64, error)
	listByCustomerFunc func(ctx context.Context, customerID string, filter models.PaginationParams) ([]models.BillingTransaction, bool, string, error)
}

func (m *mockAdapterTxRepo) CreditAccount(ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error) {
	return m.creditAccountFunc(ctx, customerID, amount, description, idempotencyKey)
}

func (m *mockAdapterTxRepo) DebitAccount(ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string) (*models.BillingTransaction, error) {
	return m.debitAccountFunc(ctx, customerID, amount, description, referenceType, referenceID, idempotencyKey)
}

func (m *mockAdapterTxRepo) GetBalance(ctx context.Context, customerID string) (int64, error) {
	return m.getBalanceFunc(ctx, customerID)
}

func (m *mockAdapterTxRepo) ListByCustomer(ctx context.Context, customerID string, filter models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
	return m.listByCustomerFunc(ctx, customerID, filter)
}

func newTestAdapter(repo *mockAdapterTxRepo) *Adapter {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ledger := services.NewBillingLedgerService(services.BillingLedgerServiceConfig{
		TransactionRepo: repo,
		Logger:          logger,
	})
	return NewAdapter(AdapterConfig{
		LedgerService: ledger,
		Logger:        logger,
	})
}

func TestAdapter_Name(t *testing.T) {
	adapter := newTestAdapter(&mockAdapterTxRepo{})
	assert.Equal(t, "native", adapter.Name())
}

func TestAdapter_OnVMCreated(t *testing.T) {
	adapter := newTestAdapter(&mockAdapterTxRepo{})
	err := adapter.OnVMCreated(context.Background(), billing.VMRef{
		ID: "vm-1", CustomerID: "cust-1",
	})
	require.NoError(t, err)
}

func TestAdapter_OnVMDeleted(t *testing.T) {
	adapter := newTestAdapter(&mockAdapterTxRepo{})
	err := adapter.OnVMDeleted(context.Background(), billing.VMRef{
		ID: "vm-1", CustomerID: "cust-1",
	})
	require.NoError(t, err)
}

func TestAdapter_OnVMResized(t *testing.T) {
	adapter := newTestAdapter(&mockAdapterTxRepo{})
	err := adapter.OnVMResized(context.Background(), billing.VMRef{
		ID: "vm-1", CustomerID: "cust-1",
	}, "plan-old", "plan-new")
	require.NoError(t, err)
}

func TestAdapter_GetBalance(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func() *mockAdapterTxRepo
		wantBalance int64
		wantErr     bool
	}{
		{
			name: "delegates to ledger service",
			setupRepo: func() *mockAdapterTxRepo {
				return &mockAdapterTxRepo{
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
			setupRepo: func() *mockAdapterTxRepo {
				return &mockAdapterTxRepo{
					getBalanceFunc: func(_ context.Context, _ string) (int64, error) {
						return 0, errors.New("db error")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newTestAdapter(tt.setupRepo())
			bal, err := adapter.GetBalance(context.Background(), "cust-1")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBalance, bal.BalanceCents)
			assert.Equal(t, "USD", bal.Currency)
			assert.Equal(t, "cust-1", bal.CustomerID)
		})
	}
}

func TestAdapter_ProcessTopUp(t *testing.T) {
	tests := []struct {
		name      string
		req       billing.TopUpRequest
		setupRepo func() *mockAdapterTxRepo
		wantErr   bool
		checkRes  func(t *testing.T, res *billing.TopUpResult)
	}{
		{
			name: "credits account",
			req: billing.TopUpRequest{
				CustomerID:  "cust-1",
				AmountCents: 2000,
				Currency:    "USD",
				Reference:   "pay-123",
			},
			setupRepo: func() *mockAdapterTxRepo {
				return &mockAdapterTxRepo{
					creditAccountFunc: func(_ context.Context, _ string, amount int64, _ string, _ *string) (*models.BillingTransaction, error) {
						return &models.BillingTransaction{
							ID:           "tx-topup",
							Amount:       amount,
							BalanceAfter: 7000,
						}, nil
					},
				}
			},
			wantErr: false,
			checkRes: func(t *testing.T, res *billing.TopUpResult) {
				assert.Equal(t, "tx-topup", res.TransactionID)
				assert.Equal(t, int64(7000), res.BalanceCents)
				assert.Equal(t, "USD", res.Currency)
			},
		},
		{
			name: "repo error propagated",
			req: billing.TopUpRequest{
				CustomerID:  "cust-1",
				AmountCents: 500,
				Currency:    "USD",
				Reference:   "pay-456",
			},
			setupRepo: func() *mockAdapterTxRepo {
				return &mockAdapterTxRepo{
					creditAccountFunc: func(_ context.Context, _ string, _ int64, _ string, _ *string) (*models.BillingTransaction, error) {
						return nil, errors.New("write failed")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newTestAdapter(tt.setupRepo())
			res, err := adapter.ProcessTopUp(context.Background(), tt.req)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.checkRes != nil {
				tt.checkRes(t, res)
			}
		})
	}
}

func TestAdapter_ValidateConfig(t *testing.T) {
	adapter := newTestAdapter(&mockAdapterTxRepo{})
	err := adapter.ValidateConfig()
	require.NoError(t, err)
}
