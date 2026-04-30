package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

type mockBillingInvoiceDB struct {
	execFunc func(context.Context, string, ...any) (pgconn.CommandTag, error)
	tx       *mockBillingInvoiceTx
}

func (m *mockBillingInvoiceDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return mockBillingInvoiceRow{err: pgx.ErrNoRows}
}

func (m *mockBillingInvoiceDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockBillingInvoiceDB) Exec(
	ctx context.Context, sql string, args ...any,
) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (m *mockBillingInvoiceDB) Begin(context.Context) (pgx.Tx, error) {
	if m.tx != nil {
		return m.tx, nil
	}
	return nil, errors.New("not implemented")
}

type mockBillingInvoiceRow struct {
	number int
	err    error
}

func (m mockBillingInvoiceRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if len(dest) == 1 {
		*dest[0].(*int) = m.number
	}
	return nil
}

type mockBillingInvoiceTx struct {
	execFunc   func(context.Context, string, ...any) (pgconn.CommandTag, error)
	committed  bool
	rolledBack bool
}

func (m *mockBillingInvoiceTx) Begin(context.Context) (pgx.Tx, error) { return m, nil }

func (m *mockBillingInvoiceTx) Commit(context.Context) error {
	m.committed = true
	return nil
}

func (m *mockBillingInvoiceTx) Rollback(context.Context) error {
	m.rolledBack = true
	return nil
}

func (m *mockBillingInvoiceTx) CopyFrom(
	context.Context, pgx.Identifier, []string, pgx.CopyFromSource,
) (int64, error) {
	return 0, nil
}

func (m *mockBillingInvoiceTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (m *mockBillingInvoiceTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *mockBillingInvoiceTx) Prepare(
	context.Context, string, string,
) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (m *mockBillingInvoiceTx) Exec(
	ctx context.Context, sql string, args ...any,
) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (m *mockBillingInvoiceTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockBillingInvoiceTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return mockBillingInvoiceRow{number: 1}
}

func (m *mockBillingInvoiceTx) Conn() *pgx.Conn { return nil }

func TestBillingInvoiceRepository_CreateDuplicatePeriodReturnsConflict(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		wantErrIs  error
	}{
		{
			name:       "customer period uniqueness violation",
			constraint: "idx_billing_invoices_customer_period_unique",
			wantErrIs:  sharederrors.ErrConflict,
		},
		{
			name:       "invoice number uniqueness violation",
			constraint: "billing_invoices_invoice_number_key",
			wantErrIs:  sharederrors.ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewBillingInvoiceRepository(&mockBillingInvoiceDB{
				execFunc: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
					return pgconn.CommandTag{}, &pgconn.PgError{
						Code:           "23505",
						ConstraintName: tt.constraint,
					}
				},
			})

			err := repo.Create(context.Background(), sampleBillingInvoice())

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErrIs)
		})
	}
}

func TestBillingInvoiceRepository_CreateRejectsLineItemTotalMismatch(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*models.BillingInvoice)
		wantError bool
	}{
		{
			name: "subtotal does not equal line item sum",
			mutate: func(inv *models.BillingInvoice) {
				inv.Subtotal = 200
				inv.Total = 200
			},
			wantError: true,
		},
		{
			name: "total does not equal subtotal plus tax",
			mutate: func(inv *models.BillingInvoice) {
				inv.TaxAmount = 10
				inv.Total = 100
			},
			wantError: true,
		},
		{
			name:      "matching totals are accepted",
			mutate:    func(*models.BillingInvoice) {},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := sampleBillingInvoice()
			tt.mutate(inv)
			repo := NewBillingInvoiceRepository(&mockBillingInvoiceDB{})

			err := repo.Create(context.Background(), inv)

			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, sharederrors.ErrValidation)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBillingInvoiceRepository_CreateWithNextNumberRollsBackCounterOnDuplicatePeriod(t *testing.T) {
	mockTx := &mockBillingInvoiceTx{
		execFunc: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "idx_billing_invoices_customer_period_unique",
			}
		},
	}
	repo := NewBillingInvoiceRepository(&mockBillingInvoiceDB{tx: mockTx})
	inv := sampleBillingInvoice()
	inv.InvoiceNumber = ""

	err := repo.CreateWithNextNumber(context.Background(), inv)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateInvoicePeriod)
	assert.False(t, mockTx.committed)
	assert.True(t, mockTx.rolledBack)
	assert.Equal(t, "INV-000001", inv.InvoiceNumber)
}

func sampleBillingInvoice() *models.BillingInvoice {
	now := time.Unix(0, 0).UTC()
	return &models.BillingInvoice{
		ID:            "00000000-0000-0000-0000-000000000001",
		CustomerID:    "00000000-0000-0000-0000-000000000002",
		InvoiceNumber: "INV-000001",
		PeriodStart:   now,
		PeriodEnd:     now.AddDate(0, 1, 0),
		Subtotal:      100,
		Total:         100,
		Currency:      "USD",
		Status:        models.InvoiceStatusIssued,
		LineItems: []models.InvoiceLineItem{
			{Description: "VM Usage", Quantity: 1, UnitPrice: 100, Amount: 100},
		},
		IssuedAt: &now,
	}
}
