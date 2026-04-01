package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statefulBillingTxRepo is an in-memory BillingTransactionRepo that tracks balance
// and transaction history, simulating the behaviour of the real database repo.
type statefulBillingTxRepo struct {
	balance      int64
	transactions []models.BillingTransaction
	mu           sync.Mutex
}

func (m *statefulBillingTxRepo) CreditAccount(
	_ context.Context, customerID string, amount int64,
	description string, idempotencyKey *string,
) (*models.BillingTransaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if idempotencyKey != nil {
		for i := range m.transactions {
			if m.transactions[i].IdempotencyKey != nil && *m.transactions[i].IdempotencyKey == *idempotencyKey {
				return &m.transactions[i], nil
			}
		}
	}

	m.balance += amount
	tx := models.BillingTransaction{
		ID:             fmt.Sprintf("tx-%d", len(m.transactions)+1),
		CustomerID:     customerID,
		Type:           models.BillingTxTypeCredit,
		Amount:         amount,
		BalanceAfter:   m.balance,
		Description:    description,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      time.Now(),
	}
	m.transactions = append(m.transactions, tx)
	return &tx, nil
}

func (m *statefulBillingTxRepo) DebitAccount(
	_ context.Context, customerID string, amount int64,
	description string, referenceType, referenceID, idempotencyKey *string,
) (*models.BillingTransaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.balance -= amount
	tx := models.BillingTransaction{
		ID:             fmt.Sprintf("tx-%d", len(m.transactions)+1),
		CustomerID:     customerID,
		Type:           models.BillingTxTypeDebit,
		Amount:         amount,
		BalanceAfter:   m.balance,
		Description:    description,
		ReferenceType:  referenceType,
		ReferenceID:    referenceID,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      time.Now(),
	}
	m.transactions = append(m.transactions, tx)
	return &tx, nil
}

func (m *statefulBillingTxRepo) GetBalance(_ context.Context, _ string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.balance, nil
}

func (m *statefulBillingTxRepo) ListByCustomer(
	_ context.Context, _ string, _ models.PaginationParams,
) ([]models.BillingTransaction, bool, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]models.BillingTransaction, len(m.transactions))
	copy(result, m.transactions)

	// Newest-first order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	lastID := ""
	if len(result) > 0 {
		lastID = result[len(result)-1].ID
	}
	return result, false, lastID, nil
}

func TestBillingFullCycle(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	customerID := "cust-001"

	repo := &statefulBillingTxRepo{}
	svc := NewBillingLedgerService(BillingLedgerServiceConfig{
		TransactionRepo: repo,
		Logger:          logger,
	})

	// Step 1: Credit $100 (10000 cents)
	creditKey := "credit-key-1"
	creditTx, err := svc.CreditAccount(ctx, customerID, 10000, "initial deposit", &creditKey)
	require.NoError(t, err)
	assert.Equal(t, int64(10000), creditTx.Amount)
	assert.Equal(t, int64(10000), creditTx.BalanceAfter)
	assert.Equal(t, models.BillingTxTypeCredit, creditTx.Type)

	// Step 2: Verify balance = 10000
	balance, err := svc.GetBalance(ctx, customerID)
	require.NoError(t, err)
	assert.Equal(t, int64(10000), balance)

	// Step 3: Debit $5 (500 cents)
	refType := models.BillingRefTypeVMUsage
	debitTx, err := svc.DebitAccount(ctx, customerID, 500, "vm hourly charge", &refType, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(500), debitTx.Amount)
	assert.Equal(t, int64(9500), debitTx.BalanceAfter)
	assert.Equal(t, models.BillingTxTypeDebit, debitTx.Type)

	// Step 4: Verify balance = 9500
	balance, err = svc.GetBalance(ctx, customerID)
	require.NoError(t, err)
	assert.Equal(t, int64(9500), balance)

	// Step 5: Verify transaction history has 2 entries
	txs, _, _, err := svc.GetTransactionHistory(ctx, customerID, models.PaginationParams{PerPage: 20})
	require.NoError(t, err)
	assert.Len(t, txs, 2)

	// Step 6: Idempotency — same credit key returns existing transaction
	dupTx, err := svc.CreditAccount(ctx, customerID, 10000, "initial deposit", &creditKey)
	require.NoError(t, err)
	assert.Equal(t, creditTx.ID, dupTx.ID, "idempotent call should return the same transaction")

	// Balance must be unchanged after idempotent replay
	balance, err = svc.GetBalance(ctx, customerID)
	require.NoError(t, err)
	assert.Equal(t, int64(9500), balance)

	// Transaction list must still have exactly 2 entries
	txs, _, _, err = svc.GetTransactionHistory(ctx, customerID, models.PaginationParams{PerPage: 20})
	require.NoError(t, err)
	assert.Len(t, txs, 2)
}
