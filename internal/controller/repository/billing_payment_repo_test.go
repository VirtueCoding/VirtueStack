package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type billingPaymentDB struct {
	tx *billingPaymentTx
}

func (m *billingPaymentDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return billingPaymentRow{err: pgx.ErrNoRows}
}

func (m *billingPaymentDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *billingPaymentDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *billingPaymentDB) Begin(context.Context) (pgx.Tx, error) {
	return m.tx, nil
}

type billingPaymentTx struct {
	rows       []pgx.Row
	queries    []string
	execs      []string
	committed  bool
	rolledBack bool
}

func (m *billingPaymentTx) Begin(context.Context) (pgx.Tx, error) { return m, nil }

func (m *billingPaymentTx) Commit(context.Context) error {
	m.committed = true
	return nil
}

func (m *billingPaymentTx) Rollback(context.Context) error {
	m.rolledBack = true
	return nil
}

func (m *billingPaymentTx) CopyFrom(
	context.Context, pgx.Identifier, []string, pgx.CopyFromSource,
) (int64, error) {
	return 0, nil
}

func (m *billingPaymentTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (m *billingPaymentTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *billingPaymentTx) Prepare(
	context.Context, string, string,
) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (m *billingPaymentTx) Exec(
	_ context.Context, sql string, _ ...any,
) (pgconn.CommandTag, error) {
	m.execs = append(m.execs, sql)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (m *billingPaymentTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *billingPaymentTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	m.queries = append(m.queries, sql)
	if len(m.rows) == 0 {
		return billingPaymentRow{err: pgx.ErrNoRows}
	}
	row := m.rows[0]
	m.rows = m.rows[1:]
	return row
}

func (m *billingPaymentTx) Conn() *pgx.Conn { return nil }

type billingPaymentRow struct {
	values []any
	err    error
}

func (m billingPaymentRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	for i := range dest {
		if err := scanBillingPaymentTestValue(dest[i], m.values[i]); err != nil {
			return err
		}
	}
	return nil
}

func scanBillingPaymentTestValue(dest, value any) error {
	switch ptr := dest.(type) {
	case *string:
		*ptr = value.(string)
	case **string:
		if value == nil {
			*ptr = nil
			return nil
		}
		if valuePtr, ok := value.(*string); ok {
			*ptr = valuePtr
			return nil
		}
		v := value.(string)
		*ptr = &v
	case *int64:
		*ptr = value.(int64)
	case *time.Time:
		*ptr = value.(time.Time)
	default:
		return errors.New("unsupported scan destination")
	}
	return nil
}

func paymentClaimRow(status string, gatewayID *string) pgx.Row {
	var value any
	if gatewayID != nil {
		value = *gatewayID
	}
	return billingPaymentRow{values: []any{status, value}}
}

func paypalClaimRow(status string, orderID, captureID *string) pgx.Row {
	values := []any{status, nil, nil}
	if orderID != nil {
		values[1] = *orderID
	}
	if captureID != nil {
		values[2] = *captureID
	}
	return billingPaymentRow{values: values}
}

func noPaymentRow() pgx.Row {
	return billingPaymentRow{err: pgx.ErrNoRows}
}

func balanceRow(balance int64) pgx.Row {
	return billingPaymentRow{values: []any{balance}}
}

func transactionRow(tx models.BillingTransaction) pgx.Row {
	return billingPaymentRow{values: []any{
		tx.ID, tx.CustomerID, tx.Type, tx.Amount, tx.BalanceAfter,
		tx.Description, tx.ReferenceType, tx.ReferenceID, tx.IdempotencyKey, tx.CreatedAt,
	}}
}

func queryErrorRow(err error) pgx.Row {
	return billingPaymentRow{err: err}
}

func TestBillingPaymentRepository_CompleteWithGatewayPaymentIDAndCreditCommitsAtomically(t *testing.T) {
	idempotencyKey := "stripe:payment:pi-1"
	mockTx := &billingPaymentTx{rows: []pgx.Row{
		paymentClaimRow(models.PaymentStatusPending, nil),
		noPaymentRow(),
		noPaymentRow(),
		balanceRow(100),
		transactionRow(paymentCreditTx(idempotencyKey)),
	}}
	repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

	claimed, err := repo.CompleteWithGatewayPaymentIDAndCredit(
		context.Background(), stripeCreditReq(idempotencyKey))

	require.NoError(t, err)
	assert.True(t, claimed)
	assert.True(t, mockTx.committed)
	assert.True(t, mockTx.rolledBack)
	assertExecContains(t, mockTx.execs, "SET status = $1, gateway_payment_id = $2")
	assertExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
	assertQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
}

func TestBillingPaymentRepository_CompleteWithGatewayPaymentIDAndCreditRollbackOnLedgerFailure(t *testing.T) {
	ledgerErr := errors.New("balance unavailable")
	idempotencyKey := "stripe:payment:pi-1"
	mockTx := &billingPaymentTx{rows: []pgx.Row{
		paymentClaimRow(models.PaymentStatusPending, nil),
		noPaymentRow(),
		noPaymentRow(),
		queryErrorRow(ledgerErr),
	}}
	repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

	claimed, err := repo.CompleteWithGatewayPaymentIDAndCredit(
		context.Background(), stripeCreditReq(idempotencyKey))

	require.Error(t, err)
	assert.False(t, claimed)
	assert.ErrorIs(t, err, ledgerErr)
	assert.False(t, mockTx.committed)
	assert.True(t, mockTx.rolledBack)
	assertExecContains(t, mockTx.execs, "SET status = $1, gateway_payment_id = $2")
	assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
	assertNoQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
}

func TestBillingPaymentRepository_CompleteWithGatewayPaymentIDAndCreditDuplicateDoesNotDoubleCredit(t *testing.T) {
	gatewayID := "pi-1"
	idempotencyKey := "stripe:payment:pi-1"
	mockTx := &billingPaymentTx{rows: []pgx.Row{
		paymentClaimRow(models.PaymentStatusCompleted, &gatewayID),
		noPaymentRow(),
		transactionRow(paymentCreditTx(idempotencyKey)),
	}}
	repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

	claimed, err := repo.CompleteWithGatewayPaymentIDAndCredit(
		context.Background(), stripeCreditReq(idempotencyKey))

	require.NoError(t, err)
	assert.True(t, claimed)
	assert.True(t, mockTx.committed)
	assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
	assertNoQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
}

func TestBillingPaymentRepository_CompleteWithGatewayPaymentIDAndCreditDoesNotRecompleteTerminalPayment(t *testing.T) {
	for _, status := range []string{models.PaymentStatusRefunded, models.PaymentStatusFailed} {
		t.Run(status, func(t *testing.T) {
			gatewayID := "pi-1"
			idempotencyKey := "stripe:payment:pi-1"
			mockTx := &billingPaymentTx{rows: []pgx.Row{
				paymentClaimRow(status, &gatewayID),
			}}
			repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

			claimed, err := repo.CompleteWithGatewayPaymentIDAndCredit(
				context.Background(), stripeCreditReq(idempotencyKey))

			require.NoError(t, err)
			assert.False(t, claimed)
			assert.False(t, mockTx.committed)
			assert.True(t, mockTx.rolledBack)
			assertNoExecContains(t, mockTx.execs, "SET status = $1, gateway_payment_id = $2")
			assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
			assertNoQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
		})
	}
}

func TestBillingPaymentRepository_CompletePayPalCaptureAndCreditRollbackOnLedgerFailure(t *testing.T) {
	ledgerErr := errors.New("balance unavailable")
	orderID := "ORDER-1"
	idempotencyKey := "paypal:capture:CAP-1"
	mockTx := &billingPaymentTx{rows: []pgx.Row{
		paypalClaimRow(models.PaymentStatusPending, &orderID, nil),
		noPaymentRow(),
		noPaymentRow(),
		queryErrorRow(ledgerErr),
	}}
	repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

	claimed, err := repo.CompletePayPalCaptureAndCredit(
		context.Background(), paypalCreditReq(idempotencyKey))

	require.Error(t, err)
	assert.False(t, claimed)
	assert.ErrorIs(t, err, ledgerErr)
	assert.False(t, mockTx.committed)
	assert.True(t, mockTx.rolledBack)
	assertExecContains(t, mockTx.execs, "metadata = jsonb_set")
	assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
}

func TestBillingPaymentRepository_CompletePayPalCaptureAndCreditDuplicateDoesNotDoubleCredit(t *testing.T) {
	orderID := "ORDER-1"
	captureID := "CAP-1"
	idempotencyKey := "paypal:capture:CAP-1"
	mockTx := &billingPaymentTx{rows: []pgx.Row{
		paypalClaimRow(models.PaymentStatusCompleted, &orderID, &captureID),
		noPaymentRow(),
		transactionRow(paymentCreditTx(idempotencyKey)),
	}}
	repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

	claimed, err := repo.CompletePayPalCaptureAndCredit(
		context.Background(), paypalCreditReq(idempotencyKey))

	require.NoError(t, err)
	assert.True(t, claimed)
	assert.True(t, mockTx.committed)
	assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
	assertNoQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
}

func TestBillingPaymentRepository_CompletePayPalCaptureAndCreditDoesNotRecompleteTerminalPayment(t *testing.T) {
	for _, status := range []string{models.PaymentStatusRefunded, models.PaymentStatusFailed} {
		t.Run(status, func(t *testing.T) {
			orderID := "ORDER-1"
			captureID := "CAP-1"
			idempotencyKey := "paypal:capture:CAP-1"
			mockTx := &billingPaymentTx{rows: []pgx.Row{
				paypalClaimRow(status, &orderID, &captureID),
			}}
			repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

			claimed, err := repo.CompletePayPalCaptureAndCredit(
				context.Background(), paypalCreditReq(idempotencyKey))

			require.NoError(t, err)
			assert.False(t, claimed)
			assert.False(t, mockTx.committed)
			assert.True(t, mockTx.rolledBack)
			assertNoExecContains(t, mockTx.execs, "metadata = jsonb_set")
			assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
			assertNoQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
		})
	}
}

func TestBillingPaymentRepository_MarkRefundedAndDebitRollbackOnLedgerFailure(t *testing.T) {
	ledgerErr := errors.New("balance unavailable")
	idempotencyKey := "stripe:refund:re-1"
	mockTx := &billingPaymentTx{rows: []pgx.Row{
		noPaymentRow(),
		queryErrorRow(ledgerErr),
	}}
	repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

	err := repo.MarkRefundedAndDebit(context.Background(), refundDebitReq(idempotencyKey))

	require.Error(t, err)
	assert.ErrorIs(t, err, ledgerErr)
	assert.False(t, mockTx.committed)
	assert.True(t, mockTx.rolledBack)
	assertExecContains(t, mockTx.execs, "SET status = $1, updated_at = NOW()")
	assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
	assertNoQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
}

func TestBillingPaymentRepository_MarkRefundedAndDebitDuplicateDoesNotDoubleDebit(t *testing.T) {
	idempotencyKey := "stripe:refund:re-1"
	mockTx := &billingPaymentTx{rows: []pgx.Row{
		transactionRow(refundDebitTx(idempotencyKey)),
	}}
	repo := NewBillingPaymentRepository(&billingPaymentDB{tx: mockTx})

	err := repo.MarkRefundedAndDebit(context.Background(), refundDebitReq(idempotencyKey))

	require.NoError(t, err)
	assert.True(t, mockTx.committed)
	assertNoExecContains(t, mockTx.execs, "UPDATE customers SET balance = $1")
	assertNoQueryContains(t, mockTx.queries, "INSERT INTO billing_transactions")
}

func stripeCreditReq(idempotencyKey string) PaymentCompletionCredit {
	return PaymentCompletionCredit{
		PaymentID:        "pay-1",
		Gateway:          models.PaymentGatewayStripe,
		GatewayPaymentID: "pi-1",
		CustomerID:       "cust-1",
		Amount:           1000,
		Description:      "Top-up via stripe",
		IdempotencyKey:   idempotencyKey,
	}
}

func paypalCreditReq(idempotencyKey string) PayPalCaptureCredit {
	return PayPalCaptureCredit{
		PaymentID:      "pay-1",
		OrderID:        "ORDER-1",
		CaptureID:      "CAP-1",
		CustomerID:     "cust-1",
		Amount:         1000,
		Description:    "Top-up via paypal",
		IdempotencyKey: idempotencyKey,
	}
}

func refundDebitReq(idempotencyKey string) PaymentRefundDebit {
	return PaymentRefundDebit{
		PaymentID:      "pay-1",
		CustomerID:     "cust-1",
		Amount:         500,
		Description:    "Refund via stripe",
		IdempotencyKey: idempotencyKey,
	}
}

func paymentCreditTx(idempotencyKey string) models.BillingTransaction {
	return models.BillingTransaction{
		ID:             "tx-1",
		CustomerID:     "cust-1",
		Type:           models.BillingTxTypeCredit,
		Amount:         1000,
		BalanceAfter:   1100,
		Description:    "Top-up",
		IdempotencyKey: &idempotencyKey,
		CreatedAt:      time.Unix(0, 0).UTC(),
	}
}

func refundDebitTx(idempotencyKey string) models.BillingTransaction {
	refType := models.BillingRefTypeRefund
	refID := "pay-1"
	return models.BillingTransaction{
		ID:             "tx-1",
		CustomerID:     "cust-1",
		Type:           models.BillingTxTypeDebit,
		Amount:         500,
		BalanceAfter:   500,
		Description:    "Refund",
		ReferenceType:  &refType,
		ReferenceID:    &refID,
		IdempotencyKey: &idempotencyKey,
		CreatedAt:      time.Unix(0, 0).UTC(),
	}
}

func assertExecContains(t *testing.T, execs []string, needle string) {
	t.Helper()
	assert.True(t, containsSQL(execs, needle), "expected exec containing %q in %v", needle, execs)
}

func assertNoExecContains(t *testing.T, execs []string, needle string) {
	t.Helper()
	assert.False(t, containsSQL(execs, needle), "unexpected exec containing %q in %v", needle, execs)
}

func assertQueryContains(t *testing.T, queries []string, needle string) {
	t.Helper()
	assert.True(t, containsSQL(queries, needle), "expected query containing %q in %v", needle, queries)
}

func assertNoQueryContains(t *testing.T, queries []string, needle string) {
	t.Helper()
	assert.False(t, containsSQL(queries, needle), "unexpected query containing %q in %v", needle, queries)
}

func containsSQL(statements []string, needle string) bool {
	for _, stmt := range statements {
		if strings.Contains(stmt, needle) {
			return true
		}
	}
	return false
}
