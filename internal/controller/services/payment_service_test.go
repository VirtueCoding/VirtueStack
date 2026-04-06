package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
)

type mockPaymentProvider struct {
	name              string
	createSessionFunc func(ctx context.Context, req payments.PaymentRequest) (*payments.PaymentSession, error)
	handleWebhookFunc func(ctx context.Context, payload []byte, sig string) (*payments.WebhookEvent, error)
	refundFunc        func(ctx context.Context, id string, amount int64, currency string) (*payments.RefundResult, error)
}

func (m *mockPaymentProvider) Name() string { return m.name }

func (m *mockPaymentProvider) CreatePaymentSession(
	ctx context.Context, req payments.PaymentRequest,
) (*payments.PaymentSession, error) {
	if m.createSessionFunc != nil {
		return m.createSessionFunc(ctx, req)
	}
	return &payments.PaymentSession{
		ID: "sess_1", GatewaySessionID: "cs_1", PaymentURL: "https://pay.example.com",
	}, nil
}

func (m *mockPaymentProvider) HandleWebhook(
	ctx context.Context, payload []byte, sig string,
) (*payments.WebhookEvent, error) {
	if m.handleWebhookFunc != nil {
		return m.handleWebhookFunc(ctx, payload, sig)
	}
	return nil, nil
}

func (m *mockPaymentProvider) GetPaymentStatus(
	_ context.Context, _ string,
) (*payments.PaymentStatus, error) {
	return nil, nil
}

func (m *mockPaymentProvider) RefundPayment(
	ctx context.Context, id string, amount int64, currency string,
) (*payments.RefundResult, error) {
	if m.refundFunc != nil {
		return m.refundFunc(ctx, id, amount, currency)
	}
	return &payments.RefundResult{
		GatewayRefundID: "re_1", GatewayPaymentID: id,
		AmountCents: amount, Currency: "usd", Status: "succeeded",
	}, nil
}

func (m *mockPaymentProvider) ValidateConfig() error { return nil }

type mockBillingPaymentRepo struct {
	createFunc         func(ctx context.Context, p *models.BillingPayment) error
	getByIDFunc        func(ctx context.Context, id string) (*models.BillingPayment, error)
	getByGatewayFunc   func(ctx context.Context, gw, id string) (*models.BillingPayment, error)
	updateStatusFunc   func(ctx context.Context, id, status string, gwID *string) error
	listByCustomerFunc func(ctx context.Context, cid string, f models.PaginationParams) ([]models.BillingPayment, bool, string, error)
	listAllFunc        func(ctx context.Context, f repository.BillingPaymentListFilter) ([]models.BillingPayment, bool, string, error)
}

func (m *mockBillingPaymentRepo) Create(ctx context.Context, p *models.BillingPayment) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, p)
	}
	p.ID = "pay_test_1"
	return nil
}

func (m *mockBillingPaymentRepo) GetByID(ctx context.Context, id string) (*models.BillingPayment, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, sharederrors.ErrNotFound
}

func (m *mockBillingPaymentRepo) GetByGatewayPaymentID(
	ctx context.Context, gw, id string,
) (*models.BillingPayment, error) {
	if m.getByGatewayFunc != nil {
		return m.getByGatewayFunc(ctx, gw, id)
	}
	return nil, sharederrors.ErrNotFound
}

func (m *mockBillingPaymentRepo) UpdateStatus(
	ctx context.Context, id, status string, gwID *string,
) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status, gwID)
	}
	return nil
}

func (m *mockBillingPaymentRepo) ListByCustomer(
	ctx context.Context, cid string, f models.PaginationParams,
) ([]models.BillingPayment, bool, string, error) {
	if m.listByCustomerFunc != nil {
		return m.listByCustomerFunc(ctx, cid, f)
	}
	return nil, false, "", nil
}

func (m *mockBillingPaymentRepo) ListAll(
	ctx context.Context, f repository.BillingPaymentListFilter,
) ([]models.BillingPayment, bool, string, error) {
	if m.listAllFunc != nil {
		return m.listAllFunc(ctx, f)
	}
	return nil, false, "", nil
}

func newTestPaymentService(
	provider *mockPaymentProvider,
	repo *mockBillingPaymentRepo,
) *PaymentService {
	return newTestPaymentServiceWithTx(provider, repo, &mockBillingTxRepo{})
}

func newTestPaymentServiceWithTx(
	provider *mockPaymentProvider,
	repo *mockBillingPaymentRepo,
	txRepo BillingTransactionRepo,
) *PaymentService {
	reg := payments.NewPaymentRegistry()
	if provider != nil {
		if err := reg.Register(provider.name, provider); err != nil {
			panic(err)
		}
	}
	logger := logging.NewLogger("error")
	ledger := NewBillingLedgerService(BillingLedgerServiceConfig{
		TransactionRepo: txRepo,
		Logger:          logger,
	})
	return NewPaymentService(PaymentServiceConfig{
		PaymentRegistry: reg,
		LedgerService:   ledger,
		PaymentRepo:     repo,
		Logger:          logger,
	})
}

type mockBillingTxRepo struct {
	creditAccountFunc func(ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error)
	debitAccountFunc  func(ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string) (*models.BillingTransaction, error)
}

func (m *mockBillingTxRepo) CreditAccount(
	ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string,
) (*models.BillingTransaction, error) {
	if m.creditAccountFunc != nil {
		return m.creditAccountFunc(ctx, customerID, amount, description, idempotencyKey)
	}
	return &models.BillingTransaction{
		ID: "tx_1", Amount: amount, BalanceAfter: amount,
	}, nil
}

func (m *mockBillingTxRepo) DebitAccount(
	ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string,
) (*models.BillingTransaction, error) {
	if m.debitAccountFunc != nil {
		return m.debitAccountFunc(ctx, customerID, amount, description, referenceType, referenceID, idempotencyKey)
	}
	return &models.BillingTransaction{
		ID: "tx_2", Amount: amount,
	}, nil
}

func (m *mockBillingTxRepo) GetBalance(_ context.Context, _ string) (int64, error) {
	return 1000, nil
}

func (m *mockBillingTxRepo) ListByCustomer(
	_ context.Context, _ string, _ models.PaginationParams,
) ([]models.BillingTransaction, bool, string, error) {
	return nil, false, "", nil
}

func TestPaymentService_InitiateTopUp(t *testing.T) {
	tests := []struct {
		name     string
		gateway  string
		wantErr  bool
		provider *mockPaymentProvider
	}{
		{
			"valid request",
			"stripe",
			false,
			&mockPaymentProvider{name: "stripe"},
		},
		{
			"unknown gateway",
			"unknown",
			true,
			&mockPaymentProvider{name: "stripe"},
		},
		{
			"provider error",
			"stripe",
			true,
			&mockPaymentProvider{
				name: "stripe",
				createSessionFunc: func(_ context.Context, _ payments.PaymentRequest) (*payments.PaymentSession, error) {
					return nil, errors.New("stripe error")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestPaymentService(tt.provider, &mockBillingPaymentRepo{})
			sess, paymentID, err := svc.InitiateTopUp(
				context.Background(),
				"cust_1", "test@example.com", 1000, "usd",
				tt.gateway, "https://return.url", "https://cancel.url",
			)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, sess)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, paymentID)
				assert.NotEmpty(t, sess.PaymentURL)
			}
		})
	}
}

func TestPaymentService_HandleWebhook_PaymentCompleted(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				GatewayEventID: "evt_1",
				PaymentID:      "pi_1",
				AmountCents:    1000,
				Currency:       "usd",
				Status:         "completed",
				IdempotencyKey: "stripe:event:evt_1",
				Metadata:       map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
			}, nil
		},
	}

	statusUpdated := false
	repo := &mockBillingPaymentRepo{
		updateStatusFunc: func(_ context.Context, _, _ string, _ *string) error {
			statusUpdated = true
			return nil
		},
	}

	svc := newTestPaymentService(provider, repo)
	err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")
	require.NoError(t, err)
	assert.True(t, statusUpdated)
}

func TestPaymentService_HandleWebhook_UnknownGateway(t *testing.T) {
	svc := newTestPaymentService(
		&mockPaymentProvider{name: "stripe"},
		&mockBillingPaymentRepo{},
	)
	err := svc.HandleWebhook(context.Background(), "unknown", []byte("{}"), "sig")
	require.Error(t, err)
}

func TestPaymentService_HandleWebhook_InvalidSignature(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return nil, errors.New("invalid signature")
		},
	}
	svc := newTestPaymentService(provider, &mockBillingPaymentRepo{})
	err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "bad")
	require.Error(t, err)
}

func TestPaymentService_HandleWebhook_UnhandledEvent(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return nil, nil
		},
	}
	svc := newTestPaymentService(provider, &mockBillingPaymentRepo{})
	err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")
	require.NoError(t, err)
}

func TestPaymentService_HandleWebhook_PaymentFailed(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type:     payments.WebhookEventPaymentFailed,
				Metadata: map[string]string{"payment_id": "pay_1"},
			}, nil
		},
	}

	statusUpdated := false
	repo := &mockBillingPaymentRepo{
		updateStatusFunc: func(_ context.Context, id, status string, _ *string) error {
			assert.Equal(t, "pay_1", id)
			assert.Equal(t, models.PaymentStatusFailed, status)
			statusUpdated = true
			return nil
		},
	}

	svc := newTestPaymentService(provider, repo)
	err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")
	require.NoError(t, err)
	assert.True(t, statusUpdated)
}

func TestPaymentService_HandleWebhook_PayPalUsesResolvedLocalPayment(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "paypal",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				GatewayEventID: "evt_paypal_1",
				PaymentID:      "CAP-123",
				AmountCents:    5000,
				Currency:       "USD",
				Status:         "completed",
				IdempotencyKey: "paypal:capture:CAP-123",
				Metadata: map[string]string{
					"customer_id": "cust_1",
					"payment_id":  "ORDER-123",
				},
			}, nil
		},
	}

	statusUpdated := false
	creditedCustomerID := ""
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error) {
			assert.Equal(t, "paypal", gateway)
			assert.Equal(t, "ORDER-123", gatewayPaymentID)
			return &models.BillingPayment{
				ID:         "pay_local_1",
				CustomerID: "cust_1",
				Gateway:    "paypal",
				Amount:     5000,
				Currency:   "USD",
				Status:     models.PaymentStatusPending,
			}, nil
		},
		updateStatusFunc: func(_ context.Context, id, status string, gatewayPaymentID *string) error {
			statusUpdated = true
			assert.Equal(t, "pay_local_1", id)
			assert.Equal(t, models.PaymentStatusCompleted, status)
			require.NotNil(t, gatewayPaymentID)
			assert.Equal(t, "CAP-123", *gatewayPaymentID)
			return nil
		},
	}
	txRepo := &mockBillingTxRepo{
		creditAccountFunc: func(_ context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error) {
			creditedCustomerID = customerID
			assert.Equal(t, int64(5000), amount)
			assert.Equal(t, "Top-up via paypal", description)
			require.NotNil(t, idempotencyKey)
			assert.Equal(t, "paypal:capture:CAP-123", *idempotencyKey)
			return &models.BillingTransaction{ID: "tx_1", Amount: amount, BalanceAfter: amount}, nil
		},
	}

	svc := newTestPaymentServiceWithTx(provider, repo, txRepo)
	err := svc.HandleWebhook(context.Background(), "paypal", []byte("{}"), "")

	require.NoError(t, err)
	assert.True(t, statusUpdated)
	assert.Equal(t, "cust_1", creditedCustomerID)
}

func TestPaymentService_HandleWebhook_PayPalRejectsInvalidPaymentContext(t *testing.T) {
	tests := []struct {
		name       string
		event      *payments.WebhookEvent
		getPayment func(ctx context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error)
		wantStatus bool
		wantCredit bool
	}{
		{
			name: "unknown order id",
			event: &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				GatewayEventID: "evt_paypal_unknown",
				PaymentID:      "CAP-404",
				AmountCents:    5000,
				Currency:       "USD",
				Status:         "completed",
				IdempotencyKey: "paypal:capture:CAP-404",
				Metadata: map[string]string{
					"customer_id": "cust_1",
					"payment_id":  "ORDER-404",
				},
			},
			getPayment: func(_ context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error) {
				assert.Equal(t, "paypal", gateway)
				assert.Equal(t, "ORDER-404", gatewayPaymentID)
				return nil, sharederrors.ErrNotFound
			},
		},
		{
			name: "customer mismatch",
			event: &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				GatewayEventID: "evt_paypal_customer_mismatch",
				PaymentID:      "CAP-201",
				AmountCents:    5000,
				Currency:       "USD",
				Status:         "completed",
				IdempotencyKey: "paypal:capture:CAP-201",
				Metadata: map[string]string{
					"customer_id": "cust_payload",
					"payment_id":  "ORDER-201",
				},
			},
			getPayment: func(_ context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error) {
				return &models.BillingPayment{
					ID:         "pay_local_201",
					CustomerID: "cust_db",
					Gateway:    "paypal",
					Amount:     5000,
					Currency:   "USD",
					Status:     models.PaymentStatusPending,
				}, nil
			},
		},
		{
			name: "amount mismatch",
			event: &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				GatewayEventID: "evt_paypal_amount_mismatch",
				PaymentID:      "CAP-202",
				AmountCents:    7000,
				Currency:       "USD",
				Status:         "completed",
				IdempotencyKey: "paypal:capture:CAP-202",
				Metadata: map[string]string{
					"customer_id": "cust_1",
					"payment_id":  "ORDER-202",
				},
			},
			getPayment: func(_ context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error) {
				return &models.BillingPayment{
					ID:         "pay_local_202",
					CustomerID: "cust_1",
					Gateway:    "paypal",
					Amount:     5000,
					Currency:   "USD",
					Status:     models.PaymentStatusPending,
				}, nil
			},
		},
		{
			name: "currency mismatch",
			event: &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				GatewayEventID: "evt_paypal_currency_mismatch",
				PaymentID:      "CAP-203",
				AmountCents:    5000,
				Currency:       "EUR",
				Status:         "completed",
				IdempotencyKey: "paypal:capture:CAP-203",
				Metadata: map[string]string{
					"customer_id": "cust_1",
					"payment_id":  "ORDER-203",
				},
			},
			getPayment: func(_ context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error) {
				return &models.BillingPayment{
					ID:         "pay_local_203",
					CustomerID: "cust_1",
					Gateway:    "paypal",
					Amount:     5000,
					Currency:   "USD",
					Status:     models.PaymentStatusPending,
				}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockPaymentProvider{
				name: "paypal",
				handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
					return tt.event, nil
				},
			}

			statusUpdated := false
			creditCalled := false
			repo := &mockBillingPaymentRepo{
				getByGatewayFunc: tt.getPayment,
				updateStatusFunc: func(_ context.Context, _, _ string, _ *string) error {
					statusUpdated = true
					return nil
				},
			}
			txRepo := &mockBillingTxRepo{
				creditAccountFunc: func(_ context.Context, _ string, amount int64, _ string, _ *string) (*models.BillingTransaction, error) {
					creditCalled = true
					return &models.BillingTransaction{ID: "tx_invalid", Amount: amount, BalanceAfter: amount}, nil
				},
			}

			svc := newTestPaymentServiceWithTx(provider, repo, txRepo)
			err := svc.HandleWebhook(context.Background(), "paypal", []byte("{}"), "")

			require.Error(t, err)
			assert.ErrorIs(t, err, sharederrors.ErrValidation)
			assert.Equal(t, tt.wantStatus, statusUpdated)
			assert.Equal(t, tt.wantCredit, creditCalled)
		})
	}
}

func TestPaymentService_HandleWebhook_PayPalLookupFailureReturnsServerError(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "paypal",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				GatewayEventID: "evt_paypal_db_failure",
				PaymentID:      "CAP-500",
				AmountCents:    5000,
				Currency:       "USD",
				Status:         "completed",
				IdempotencyKey: "paypal:capture:CAP-500",
				Metadata: map[string]string{
					"customer_id": "cust_1",
					"payment_id":  "ORDER-500",
				},
			}, nil
		},
	}

	statusUpdated := false
	creditCalled := false
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, gateway, gatewayPaymentID string) (*models.BillingPayment, error) {
			assert.Equal(t, "paypal", gateway)
			assert.Equal(t, "ORDER-500", gatewayPaymentID)
			return nil, errors.New("database unavailable")
		},
		updateStatusFunc: func(_ context.Context, _, _ string, _ *string) error {
			statusUpdated = true
			return nil
		},
	}
	txRepo := &mockBillingTxRepo{
		creditAccountFunc: func(_ context.Context, _ string, amount int64, _ string, _ *string) (*models.BillingTransaction, error) {
			creditCalled = true
			return &models.BillingTransaction{ID: "tx_500", Amount: amount, BalanceAfter: amount}, nil
		},
	}

	svc := newTestPaymentServiceWithTx(provider, repo, txRepo)
	err := svc.HandleWebhook(context.Background(), "paypal", []byte("{}"), "")

	require.Error(t, err)
	assert.NotErrorIs(t, err, sharederrors.ErrValidation)
	assert.False(t, statusUpdated)
	assert.False(t, creditCalled)
}

func TestPaymentService_GetPaymentHistory(t *testing.T) {
	repo := &mockBillingPaymentRepo{
		listByCustomerFunc: func(_ context.Context, _ string, _ models.PaginationParams) ([]models.BillingPayment, bool, string, error) {
			return []models.BillingPayment{{ID: "p1"}}, true, "p1", nil
		},
	}
	svc := newTestPaymentService(&mockPaymentProvider{name: "stripe"}, repo)
	items, hasMore, lastID, err := svc.GetPaymentHistory(
		context.Background(), "cust_1", models.PaginationParams{PerPage: 20},
	)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.True(t, hasMore)
	assert.Equal(t, "p1", lastID)
}

func TestPaymentService_GetTopUpConfig_Defaults(t *testing.T) {
	svc := newTestPaymentService(&mockPaymentProvider{name: "stripe"}, &mockBillingPaymentRepo{})
	config, err := svc.GetTopUpConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(500), config.MinAmountCents)
	assert.Equal(t, int64(50000), config.MaxAmountCents)
	assert.Len(t, config.Presets, 5)
	assert.Contains(t, config.Gateways, "stripe")
	assert.Equal(t, "USD", config.Currency)
}

func TestPaymentService_RefundPayment_Valid(t *testing.T) {
	gwPaymentID := "pi_test"
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID:               "pay_1",
				CustomerID:       "cust_1",
				Gateway:          "stripe",
				GatewayPaymentID: &gwPaymentID,
				Amount:           1000,
				Status:           models.PaymentStatusCompleted,
			}, nil
		},
	}

	svc := newTestPaymentService(&mockPaymentProvider{name: "stripe"}, repo)
	result, err := svc.RefundPayment(context.Background(), "pay_1", 500)
	require.NoError(t, err)
	assert.Equal(t, int64(500), result.AmountCents)
}

func TestPaymentService_RefundPayment_PendingStatus(t *testing.T) {
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID:     "pay_1",
				Status: models.PaymentStatusPending,
			}, nil
		},
	}
	svc := newTestPaymentService(&mockPaymentProvider{name: "stripe"}, repo)
	_, err := svc.RefundPayment(context.Background(), "pay_1", 500)
	require.Error(t, err)
	var valErr *sharederrors.ValidationError
	assert.True(t, errors.As(err, &valErr))
}

func TestPaymentService_RefundPayment_ExceedsAmount(t *testing.T) {
	gwPaymentID := "pi_test"
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID:               "pay_1",
				Gateway:          "stripe",
				GatewayPaymentID: &gwPaymentID,
				Amount:           1000,
				Status:           models.PaymentStatusCompleted,
			}, nil
		},
	}
	svc := newTestPaymentService(&mockPaymentProvider{name: "stripe"}, repo)
	_, err := svc.RefundPayment(context.Background(), "pay_1", 2000)
	require.Error(t, err)
	var valErr *sharederrors.ValidationError
	assert.True(t, errors.As(err, &valErr))
}

func TestParseInt64Slice(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int64
	}{
		{"valid", "100,200,300", []int64{100, 200, 300}},
		{"spaces", " 100 , 200 ", []int64{100, 200}},
		{"empty", "", nil},
		{"invalid entries", "100,abc,300", []int64{100, 300}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInt64Slice(tt.input)
			if tt.want == nil {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
