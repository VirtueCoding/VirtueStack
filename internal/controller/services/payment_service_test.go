package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gostripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	stripeprovider "github.com/AbuGosok/VirtueStack/internal/controller/payments/stripe"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
)

type mockPaymentProvider struct {
	name              string
	createSessionFunc func(ctx context.Context, req payments.PaymentRequest) (*payments.PaymentSession, error)
	handleWebhookFunc func(ctx context.Context, payload []byte, sig string) (*payments.WebhookEvent, error)
	refundFunc        func(ctx context.Context, id string, amount int64, currency string) (*payments.RefundResult, error)
	captureFunc       func(ctx context.Context, orderID string) (captureID, status, currency string, amountCents int64, err error)
	verifyWebhookFunc func(ctx context.Context, headers http.Header, body []byte) error
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

func (m *mockPaymentProvider) CaptureOrder(
	ctx context.Context, orderID string,
) (captureID, status, currency string, amountCents int64, err error) {
	if m.captureFunc != nil {
		return m.captureFunc(ctx, orderID)
	}
	return "", "", "", 0, errors.New("capture not configured")
}

func (m *mockPaymentProvider) VerifyWebhookSignature(
	ctx context.Context, headers http.Header, body []byte,
) error {
	if m.verifyWebhookFunc != nil {
		return m.verifyWebhookFunc(ctx, headers, body)
	}
	return nil
}

type mockBillingPaymentRepo struct {
	createFunc                func(ctx context.Context, p *models.BillingPayment) error
	getByIDFunc               func(ctx context.Context, id string) (*models.BillingPayment, error)
	getByGatewayFunc          func(ctx context.Context, gw, id string) (*models.BillingPayment, error)
	updateStatusFunc          func(ctx context.Context, id, status string, gwID *string) error
	completeFunc              func(ctx context.Context, id, gateway, gatewayPaymentID string) (bool, error)
	completeAndCreditFunc     func(ctx context.Context, req repository.PaymentCompletionCredit) (bool, error)
	completePayPalCaptureFunc func(ctx context.Context, id, orderID, captureID string) (bool, error)
	completePayPalCreditFunc  func(ctx context.Context, req repository.PayPalCaptureCredit) (bool, error)
	refundAndDebitFunc        func(ctx context.Context, req repository.PaymentRefundDebit) error
	listByCustomerFunc        func(ctx context.Context, cid string, f models.PaginationParams) ([]models.BillingPayment, bool, string, error)
	listAllFunc               func(ctx context.Context, f repository.BillingPaymentListFilter) ([]models.BillingPayment, bool, string, error)
	txRepo                    BillingTransactionRepo
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

func (m *mockBillingPaymentRepo) CompleteWithGatewayPaymentID(
	ctx context.Context, id, gateway, gatewayPaymentID string,
) (bool, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, id, gateway, gatewayPaymentID)
	}
	if m.updateStatusFunc != nil {
		if err := m.updateStatusFunc(
			ctx, id, models.PaymentStatusCompleted, &gatewayPaymentID,
		); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (m *mockBillingPaymentRepo) CompleteWithGatewayPaymentIDAndCredit(
	ctx context.Context, req repository.PaymentCompletionCredit,
) (bool, error) {
	if m.completeAndCreditFunc != nil {
		return m.completeAndCreditFunc(ctx, req)
	}
	claimed, err := m.CompleteWithGatewayPaymentID(
		ctx, req.PaymentID, req.Gateway, req.GatewayPaymentID)
	if err != nil || !claimed {
		return claimed, err
	}
	if m.txRepo != nil {
		_, err = m.txRepo.CreditAccount(
			ctx, req.CustomerID, req.Amount, req.Description, &req.IdempotencyKey)
	}
	return claimed, err
}

func (m *mockBillingPaymentRepo) CompletePayPalCapture(
	ctx context.Context, id, orderID, captureID string,
) (bool, error) {
	if m.completePayPalCaptureFunc != nil {
		return m.completePayPalCaptureFunc(ctx, id, orderID, captureID)
	}
	if m.updateStatusFunc != nil {
		if err := m.updateStatusFunc(
			ctx, id, models.PaymentStatusCompleted, &captureID,
		); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (m *mockBillingPaymentRepo) CompletePayPalCaptureAndCredit(
	ctx context.Context, req repository.PayPalCaptureCredit,
) (bool, error) {
	if m.completePayPalCreditFunc != nil {
		return m.completePayPalCreditFunc(ctx, req)
	}
	claimed, err := m.CompletePayPalCapture(ctx, req.PaymentID, req.OrderID, req.CaptureID)
	if err != nil || !claimed {
		return claimed, err
	}
	if m.txRepo != nil {
		_, err = m.txRepo.CreditAccount(
			ctx, req.CustomerID, req.Amount, req.Description, &req.IdempotencyKey)
	}
	return claimed, err
}

func (m *mockBillingPaymentRepo) MarkRefundedAndDebit(
	ctx context.Context, req repository.PaymentRefundDebit,
) error {
	if m.refundAndDebitFunc != nil {
		return m.refundAndDebitFunc(ctx, req)
	}
	if err := m.UpdateStatus(ctx, req.PaymentID, models.PaymentStatusRefunded, nil); err != nil {
		return err
	}
	if m.txRepo != nil {
		refType := models.BillingRefTypeRefund
		_, err := m.txRepo.DebitAccount(
			ctx, req.CustomerID, req.Amount, req.Description,
			&refType, &req.PaymentID, &req.IdempotencyKey)
		return err
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
	reg := payments.NewPaymentRegistry()
	if provider != nil {
		if err := reg.Register(provider.name, provider); err != nil {
			panic(err)
		}
	}
	logger := logging.NewLogger("error")
	return NewPaymentService(PaymentServiceConfig{
		PaymentRegistry: reg,
		PaymentRepo:     repo,
		Logger:          logger,
	})
}

func TestPaymentServiceSettlePayment(t *testing.T) {
	payment := &models.BillingPayment{ID: "pay-1", CustomerID: "cust-1"}
	tests := []struct {
		name               string
		settlement         paymentSettlement
		wantGenericGateway string
		wantPayPalOrderID  string
	}{
		{
			name: "generic settlement credits by gateway payment id",
			settlement: paymentSettlement{
				Payment:          payment,
				Gateway:          "stripe",
				GatewayPaymentID: "pi_1",
				CustomerID:       "cust-1",
				AmountCents:      1500,
				Description:      "Top-up via stripe",
				IdempotencyKey:   "stripe:payment:pi_1",
			},
			wantGenericGateway: "stripe",
		},
		{
			name: "paypal settlement claims order and capture ids",
			settlement: paymentSettlement{
				Payment:          payment,
				Gateway:          models.PaymentGatewayPayPal,
				GatewayPaymentID: "capture-1",
				PayPalOrderID:    "order-1",
				CustomerID:       "cust-1",
				AmountCents:      2500,
				Description:      "Top-up via paypal",
				IdempotencyKey:   "paypal:capture-1",
			},
			wantPayPalOrderID: "order-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var genericReq repository.PaymentCompletionCredit
			var paypalReq repository.PayPalCaptureCredit
			repo := &mockBillingPaymentRepo{
				completeAndCreditFunc: func(_ context.Context, req repository.PaymentCompletionCredit) (bool, error) {
					genericReq = req
					return true, nil
				},
				completePayPalCreditFunc: func(_ context.Context, req repository.PayPalCaptureCredit) (bool, error) {
					paypalReq = req
					return true, nil
				},
			}
			svc := newTestPaymentService(nil, repo)

			claimed, err := svc.settlePayment(context.Background(), tt.settlement)

			require.NoError(t, err)
			assert.True(t, claimed)
			if tt.wantPayPalOrderID != "" {
				assert.Equal(t, tt.wantPayPalOrderID, paypalReq.OrderID)
				assert.Equal(t, tt.settlement.GatewayPaymentID, paypalReq.CaptureID)
				assert.Empty(t, genericReq.PaymentID)
				return
			}
			assert.Equal(t, tt.wantGenericGateway, genericReq.Gateway)
			assert.Equal(t, tt.settlement.GatewayPaymentID, genericReq.GatewayPaymentID)
			assert.Empty(t, paypalReq.PaymentID)
		})
	}
}

type trackingBillingTxRepo struct {
	balances map[string]int64
	seen     map[string]*models.BillingTransaction
}

func newTrackingBillingTxRepo() *trackingBillingTxRepo {
	return &trackingBillingTxRepo{
		balances: make(map[string]int64),
		seen:     make(map[string]*models.BillingTransaction),
	}
}

func (m *trackingBillingTxRepo) CreditAccount(
	_ context.Context, customerID string, amount int64, _ string, idempotencyKey *string,
) (*models.BillingTransaction, error) {
	key := ""
	if idempotencyKey != nil {
		key = customerID + ":" + *idempotencyKey
		if existing := m.seen[key]; existing != nil {
			return existing, nil
		}
	}
	m.balances[customerID] += amount
	tx := &models.BillingTransaction{
		ID: "tx_tracking", CustomerID: customerID,
		Amount: amount, BalanceAfter: m.balances[customerID],
	}
	if key != "" {
		m.seen[key] = tx
	}
	return tx, nil
}

func (m *trackingBillingTxRepo) DebitAccount(
	_ context.Context, customerID string, amount int64, _ string, _, _, _ *string,
) (*models.BillingTransaction, error) {
	m.balances[customerID] -= amount
	return &models.BillingTransaction{
		ID: "tx_debit", CustomerID: customerID,
		Amount: amount, BalanceAfter: m.balances[customerID],
	}, nil
}

func (m *trackingBillingTxRepo) GetBalance(_ context.Context, customerID string) (int64, error) {
	return m.balances[customerID], nil
}

func (m *trackingBillingTxRepo) ListByCustomer(
	_ context.Context, _ string, _ models.PaginationParams,
) ([]models.BillingTransaction, bool, string, error) {
	return nil, false, "", nil
}

type failingBillingTxRepo struct{}

func (m *failingBillingTxRepo) CreditAccount(
	_ context.Context, _ string, _ int64, _ string, _ *string,
) (*models.BillingTransaction, error) {
	return nil, errors.New("ledger unavailable")
}

func (m *failingBillingTxRepo) DebitAccount(
	_ context.Context, _ string, _ int64, _ string, _, _, _ *string,
) (*models.BillingTransaction, error) {
	return nil, errors.New("ledger unavailable")
}

func (m *failingBillingTxRepo) GetBalance(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *failingBillingTxRepo) ListByCustomer(
	_ context.Context, _ string, _ models.PaginationParams,
) ([]models.BillingTransaction, bool, string, error) {
	return nil, false, "", nil
}

func newPaymentServiceWithLedger(
	provider payments.PaymentProvider,
	repo *mockBillingPaymentRepo,
	txRepo BillingTransactionRepo,
) *PaymentService {
	repo.txRepo = txRepo
	reg := payments.NewPaymentRegistry()
	if provider != nil {
		if err := reg.Register(provider.Name(), provider); err != nil {
			panic(err)
		}
	}
	logger := logging.NewLogger("error")
	return NewPaymentService(PaymentServiceConfig{
		PaymentRegistry: reg,
		PaymentRepo:     repo,
		Logger:          logger,
	})
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
		getByIDFunc: func(_ context.Context, id string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: id, CustomerID: "cust_1", Gateway: "stripe",
				Amount: 1000, Currency: "usd",
			}, nil
		},
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

func TestPaymentService_HandleWebhook_StripePaymentIntentSucceededCreditsLedger(t *testing.T) {
	// #nosec G101 -- test webhook signing secret.
	secret := "whsec_test_service_pi"
	provider := stripeprovider.NewProvider(stripeprovider.ProviderConfig{
		SecretKey:      "sk_test_fake",
		WebhookSecret:  secret,
		PublishableKey: "pk_test_fake",
		Logger:         logging.NewLogger("error"),
	})
	payload, sig := makeStripePaymentIntentPayload(t, secret)

	completedGatewayID := ""
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, id string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: id, CustomerID: "cust_1", Gateway: "stripe",
				Amount: 2500, Currency: "usd",
			}, nil
		},
		completeFunc: func(_ context.Context, id, gateway, gatewayID string) (bool, error) {
			assert.Equal(t, "pay_1", id)
			assert.Equal(t, "stripe", gateway)
			completedGatewayID = gatewayID
			return true, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	err := svc.HandleWebhook(context.Background(), "stripe", payload, sig)
	require.NoError(t, err)
	assert.Equal(t, "pi_test_789", completedGatewayID)
	balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
	require.NoError(t, balanceErr)
	assert.Equal(t, int64(2500), balance)
}

func makeStripePaymentIntentPayload(t *testing.T, secret string) ([]byte, string) {
	t.Helper()
	pi := gostripe.PaymentIntent{
		ID:       "pi_test_789",
		Amount:   2500,
		Currency: "usd",
		Metadata: map[string]string{
			"customer_id":  "cust_1",
			"payment_id":   "pay_1",
			"amount_cents": "2500",
		},
	}
	piBytes, err := json.Marshal(pi) // #nosec G117 -- Stripe test payload must contain client_secret-shaped SDK fields.
	require.NoError(t, err)
	event := gostripe.Event{
		ID:   "evt_test_service_pi",
		Type: "payment_intent.succeeded",
		Data: &gostripe.EventData{Raw: piBytes},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  secret,
	})
	return payload, signed.Header
}

func TestPaymentService_HandleWebhook_PayPalCompletedClaimsCaptureWithoutReplacingOrderID(t *testing.T) {
	orderID := "ORDER-1"
	captureID := "CAP-1"
	provider := &mockPaymentProvider{
		name: "paypal",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				PaymentID:      captureID,
				AmountCents:    1000,
				Currency:       "USD",
				IdempotencyKey: "paypal:capture:" + captureID,
				Metadata: map[string]string{
					"customer_id": "cust_1",
					"payment_id":  orderID,
				},
			}, nil
		},
	}

	claimedCapture := false
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, id string) (*models.BillingPayment, error) {
			if id != orderID {
				return nil, sharederrors.ErrNotFound
			}
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
			}, nil
		},
		completeFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			return false, errors.New("generic completion should not run")
		},
		completePayPalCaptureFunc: func(_ context.Context, _, gotOrderID, gotCaptureID string) (bool, error) {
			assert.Equal(t, orderID, gotOrderID)
			assert.Equal(t, captureID, gotCaptureID)
			claimedCapture = true
			return true, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	err := svc.HandleWebhook(context.Background(), "paypal", []byte("{}"), "sig")
	require.NoError(t, err)
	assert.True(t, claimedCapture)
	balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
	require.NoError(t, balanceErr)
	assert.Equal(t, int64(1000), balance)
}

func TestPaymentService_HandleWebhook_DuplicateExternalPaymentDoesNotCreditAnotherCustomer(t *testing.T) {
	ctx := context.Background()
	events := []*payments.WebhookEvent{
		{
			Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_duplicate",
			AmountCents: 1000, Currency: "usd",
			IdempotencyKey: "stripe:event:evt_first",
			Metadata:       map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
		},
		{
			Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_duplicate",
			AmountCents: 1000, Currency: "usd",
			IdempotencyKey: "stripe:event:evt_second",
			Metadata:       map[string]string{"customer_id": "cust_2", "payment_id": "pay_2"},
		},
	}
	call := 0
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			event := events[call]
			call++
			return event, nil
		},
	}
	paymentsByID := map[string]*models.BillingPayment{
		"pay_1": {ID: "pay_1", CustomerID: "cust_1", Gateway: "stripe", Amount: 1000, Currency: "usd"},
		"pay_2": {ID: "pay_2", CustomerID: "cust_2", Gateway: "stripe", Amount: 1000, Currency: "usd"},
	}
	gatewayIDs := map[string]string{}
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, id string) (*models.BillingPayment, error) {
			if payment := paymentsByID[id]; payment != nil {
				return payment, nil
			}
			return nil, sharederrors.ErrNotFound
		},
		completeFunc: func(_ context.Context, id, _, gatewayID string) (bool, error) {
			if gatewayIDs[gatewayID] != "" && gatewayIDs[gatewayID] != id {
				return false, nil
			}
			gatewayIDs[gatewayID] = id
			return true, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	require.NoError(t, svc.HandleWebhook(ctx, "stripe", []byte("{}"), "sig"))
	require.NoError(t, svc.HandleWebhook(ctx, "stripe", []byte("{}"), "sig"))

	cust1Balance, err := txRepo.GetBalance(ctx, "cust_1")
	require.NoError(t, err)
	cust2Balance, err := txRepo.GetBalance(ctx, "cust_2")
	require.NoError(t, err)
	assert.Equal(t, int64(1000), cust1Balance)
	assert.Equal(t, int64(0), cust2Balance)
}

func TestPaymentService_HandleWebhook_SamePaymentDifferentGatewayIDCreditsOnce(t *testing.T) {
	ctx := context.Background()
	events := []*payments.WebhookEvent{
		{
			Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_first",
			AmountCents: 1000, Currency: "usd",
			Metadata: map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
		},
		{
			Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_second",
			AmountCents: 1000, Currency: "usd",
			Metadata: map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
		},
	}
	call := 0
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			event := events[call]
			call++
			return event, nil
		},
	}
	completedGatewayID := ""
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, id string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: id, CustomerID: "cust_1", Gateway: "stripe",
				Amount: 1000, Currency: "usd",
			}, nil
		},
		completeFunc: func(_ context.Context, _, _, gatewayID string) (bool, error) {
			if completedGatewayID != "" && completedGatewayID != gatewayID {
				return false, nil
			}
			completedGatewayID = gatewayID
			return true, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	require.NoError(t, svc.HandleWebhook(ctx, "stripe", []byte("{}"), "sig"))
	require.NoError(t, svc.HandleWebhook(ctx, "stripe", []byte("{}"), "sig"))

	balance, err := txRepo.GetBalance(ctx, "cust_1")
	require.NoError(t, err)
	assert.Equal(t, int64(1000), balance)
}

func TestPaymentService_HandleWebhook_RejectsPaymentMismatch(t *testing.T) {
	tests := []struct {
		name    string
		payment models.BillingPayment
		event   payments.WebhookEvent
	}{
		{
			name:    "customer mismatch",
			payment: models.BillingPayment{ID: "pay_1", CustomerID: "cust_1", Amount: 1000, Currency: "usd"},
			event: payments.WebhookEvent{
				Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_1",
				AmountCents: 1000, Currency: "usd",
				Metadata: map[string]string{"customer_id": "cust_2", "payment_id": "pay_1"},
			},
		},
		{
			name:    "amount mismatch",
			payment: models.BillingPayment{ID: "pay_1", CustomerID: "cust_1", Amount: 1000, Currency: "usd"},
			event: payments.WebhookEvent{
				Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_1",
				AmountCents: 2000, Currency: "usd",
				Metadata: map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
			},
		},
		{
			name:    "currency mismatch",
			payment: models.BillingPayment{ID: "pay_1", CustomerID: "cust_1", Amount: 1000, Currency: "usd"},
			event: payments.WebhookEvent{
				Type: payments.WebhookEventPaymentCompleted, PaymentID: "pi_1",
				AmountCents: 1000, Currency: "eur",
				Metadata: map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockPaymentProvider{
				name: "stripe",
				handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
					return &tt.event, nil
				},
			}
			repo := &mockBillingPaymentRepo{
				getByIDFunc: func(_ context.Context, _ string) (*models.BillingPayment, error) {
					return &tt.payment, nil
				},
				updateStatusFunc: func(_ context.Context, _, _ string, _ *string) error {
					return errors.New("status update should not run")
				},
			}
			txRepo := newTrackingBillingTxRepo()
			svc := newPaymentServiceWithLedger(provider, repo, txRepo)

			err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")
			require.Error(t, err)
			balance, balanceErr := txRepo.GetBalance(context.Background(), tt.payment.CustomerID)
			require.NoError(t, balanceErr)
			assert.Equal(t, int64(0), balance)
		})
	}
}

func TestPaymentService_HandleWebhook_PaymentCompletedLedgerFailureDoesNotCompletePayment(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				PaymentID:      "pi_1",
				AmountCents:    1000,
				Currency:       "usd",
				IdempotencyKey: "stripe:event:evt_1",
				Metadata:       map[string]string{"customer_id": "cust_1", "payment_id": "pay_1"},
			}, nil
		},
	}
	paymentCompleted := false
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, id string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: id, CustomerID: "cust_1", Gateway: "stripe",
				Amount: 1000, Currency: "usd",
			}, nil
		},
		completeFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			paymentCompleted = true
			return true, nil
		},
		completeAndCreditFunc: func(context.Context, repository.PaymentCompletionCredit) (bool, error) {
			return false, errors.New("ledger unavailable")
		},
	}
	svc := newPaymentServiceWithLedger(provider, repo, &failingBillingTxRepo{})

	err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")

	require.Error(t, err)
	assert.False(t, paymentCompleted)
}

func TestPaymentService_HandleWebhook_RefundLedgerFailureDoesNotRefundPayment(t *testing.T) {
	provider := &mockPaymentProvider{
		name: "stripe",
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			return &payments.WebhookEvent{
				Type:           payments.WebhookEventRefundCompleted,
				PaymentID:      "pi_1",
				AmountCents:    1000,
				IdempotencyKey: "stripe:refund:re_1",
			}, nil
		},
	}
	paymentRefunded := false
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "stripe",
				Amount: 1000, Currency: "usd",
			}, nil
		},
		updateStatusFunc: func(_ context.Context, _, status string, _ *string) error {
			paymentRefunded = status == models.PaymentStatusRefunded
			return nil
		},
		refundAndDebitFunc: func(context.Context, repository.PaymentRefundDebit) error {
			return errors.New("ledger unavailable")
		},
	}
	svc := newPaymentServiceWithLedger(provider, repo, &failingBillingTxRepo{})

	err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")

	require.Error(t, err)
	assert.False(t, paymentRefunded)
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

func TestPaymentService_RefundPayment_PayPalUsesCaptureIDFromMetadata(t *testing.T) {
	orderID := "ORDER-1"
	provider := &mockPaymentProvider{
		name: "paypal",
		refundFunc: func(_ context.Context, id string, amount int64, currency string) (*payments.RefundResult, error) {
			assert.Equal(t, "CAP-1", id)
			return &payments.RefundResult{
				GatewayRefundID: "refund_1", GatewayPaymentID: id,
				AmountCents: amount, Currency: currency, Status: "completed",
			}, nil
		},
	}
	repo := &mockBillingPaymentRepo{
		getByIDFunc: func(_ context.Context, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
				Status:   models.PaymentStatusCompleted,
				Metadata: []byte(`{"paypal_capture_id":"CAP-1"}`),
			}, nil
		},
	}

	svc := newTestPaymentService(provider, repo)
	result, err := svc.RefundPayment(context.Background(), "pay_1", 500)
	require.NoError(t, err)
	assert.Equal(t, "CAP-1", result.GatewayPaymentID)
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

func TestPaymentService_CreditFromPayment_RejectsCryptoPaymentMismatch(t *testing.T) {
	tests := []struct {
		name      string
		payment   models.BillingPayment
		accountID string
		amount    int64
		currency  string
	}{
		{
			name: "customer mismatch",
			payment: models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "btcpay",
				Amount: 1000, Currency: "USD",
			},
			accountID: "cust_2", amount: 1000, currency: "USD",
		},
		{
			name: "amount mismatch",
			payment: models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "btcpay",
				Amount: 1000, Currency: "USD",
			},
			accountID: "cust_1", amount: 2000, currency: "USD",
		},
		{
			name: "currency mismatch",
			payment: models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "btcpay",
				Amount: 1000, Currency: "USD",
			},
			accountID: "cust_1", amount: 1000, currency: "EUR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockBillingPaymentRepo{
				getByGatewayFunc: func(_ context.Context, _, _ string) (*models.BillingPayment, error) {
					return &tt.payment, nil
				},
				updateStatusFunc: func(_ context.Context, _, _ string, _ *string) error {
					return errors.New("status update should not run")
				},
			}
			txRepo := newTrackingBillingTxRepo()
			svc := newPaymentServiceWithLedger(&mockPaymentProvider{name: "btcpay"}, repo, txRepo)

			err := svc.CreditFromPayment(
				context.Background(), tt.accountID, tt.amount, tt.currency,
				"btcpay", "invoice_1", "invoice_1", "btcpay:invoice:invoice_1",
			)
			require.Error(t, err)
			balance, balanceErr := txRepo.GetBalance(context.Background(), tt.accountID)
			require.NoError(t, balanceErr)
			assert.Equal(t, int64(0), balance)
		})
	}
}

func TestPaymentService_CreditFromPayment_LedgerFailureDoesNotCompletePayment(t *testing.T) {
	paymentCompleted := false
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "btcpay",
				Amount: 1000, Currency: "USD",
			}, nil
		},
		completeFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			paymentCompleted = true
			return true, nil
		},
		completeAndCreditFunc: func(context.Context, repository.PaymentCompletionCredit) (bool, error) {
			return false, errors.New("ledger unavailable")
		},
	}
	svc := newPaymentServiceWithLedger(
		&mockPaymentProvider{name: "btcpay"}, repo, &failingBillingTxRepo{})

	err := svc.CreditFromPayment(
		context.Background(), "cust_1", 1000, "USD",
		"btcpay", "invoice_1", "invoice_1", "btcpay:invoice:invoice_1",
	)

	require.Error(t, err)
	assert.False(t, paymentCompleted)
}

func TestPaymentService_CapturePayPalOrder_RejectsUncompletedOrMismatchedCapture(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		amount   int64
		currency string
	}{
		{"pending status", "PENDING", 1000, "USD"},
		{"amount mismatch", "COMPLETED", 2000, "USD"},
		{"currency mismatch", "COMPLETED", 1000, "EUR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orderID := "ORDER-1"
			provider := &mockPaymentProvider{
				name: "paypal",
				captureFunc: func(_ context.Context, _ string) (string, string, string, int64, error) {
					return "CAP-1", tt.status, tt.currency, tt.amount, nil
				},
			}
			repo := &mockBillingPaymentRepo{
				getByGatewayFunc: func(_ context.Context, _, _ string) (*models.BillingPayment, error) {
					return &models.BillingPayment{
						ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
						GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
					}, nil
				},
				updateStatusFunc: func(_ context.Context, _, _ string, _ *string) error {
					return errors.New("status update should not run")
				},
			}
			txRepo := newTrackingBillingTxRepo()
			svc := newPaymentServiceWithLedger(provider, repo, txRepo)

			result, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
			require.Error(t, err)
			assert.Nil(t, result)
			balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
			require.NoError(t, balanceErr)
			assert.Equal(t, int64(0), balance)
		})
	}
}

func TestPaymentService_CapturePayPalOrder_UpdateFailureCanRetryCreditIdempotently(t *testing.T) {
	orderID := "ORDER-1"
	captureCalls := 0
	updateCalls := 0
	provider := &mockPaymentProvider{
		name: "paypal",
		captureFunc: func(_ context.Context, _ string) (string, string, string, int64, error) {
			captureCalls++
			return "CAP-1", "COMPLETED", "USD", 1000, nil
		},
	}
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
			}, nil
		},
		updateStatusFunc: func(_ context.Context, _, _ string, _ *string) error {
			updateCalls++
			if updateCalls == 1 {
				return errors.New("database unavailable")
			}
			return nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	result, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.Error(t, err)
	assert.Nil(t, result)
	result, err = svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.NoError(t, err)
	require.NotNil(t, result)
	balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
	require.NoError(t, balanceErr)
	assert.Equal(t, int64(1000), balance)
	assert.Equal(t, 2, captureCalls, "retry reuses provider-level idempotent capture result")
}

func TestPaymentService_CapturePayPalOrder_DoesNotCreditWhenCaptureClaimFails(t *testing.T) {
	orderID := "ORDER-1"
	provider := &mockPaymentProvider{
		name: "paypal",
		captureFunc: func(_ context.Context, _ string) (string, string, string, int64, error) {
			return "CAP-1", "COMPLETED", "USD", 1000, nil
		},
	}
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, _ string) (*models.BillingPayment, error) {
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
			}, nil
		},
		completePayPalCaptureFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			return false, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	result, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.Error(t, err)
	assert.Nil(t, result)
	balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
	require.NoError(t, balanceErr)
	assert.Equal(t, int64(0), balance)
}

func TestPaymentService_CapturePayPalOrder_RetryByOrderIDAfterSuccessIsIdempotent(t *testing.T) {
	orderID := "ORDER-1"
	captureID := "CAP-1"
	gatewayIDByPayment := map[string]string{"pay_1": orderID}
	provider := &mockPaymentProvider{
		name: "paypal",
		captureFunc: func(_ context.Context, _ string) (string, string, string, int64, error) {
			return captureID, "COMPLETED", "USD", 1000, nil
		},
	}
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, id string) (*models.BillingPayment, error) {
			if gatewayIDByPayment["pay_1"] != id {
				return nil, sharederrors.ErrNotFound
			}
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
			}, nil
		},
		completePayPalCaptureFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			return true, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	first, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.NoError(t, err)
	second, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, first.CaptureID, second.CaptureID)
	balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
	require.NoError(t, balanceErr)
	assert.Equal(t, int64(1000), balance)
}

func TestPaymentService_CapturePayPalOrder_RetryAfterLocalCompletionDoesNotRecapture(t *testing.T) {
	orderID := "ORDER-1"
	captureID := "CAP-1"
	captureCalls := 0
	paymentStatus := models.PaymentStatusPending
	paymentMetadata := []byte("{}")
	provider := &mockPaymentProvider{
		name: "paypal",
		captureFunc: func(_ context.Context, _ string) (string, string, string, int64, error) {
			captureCalls++
			if captureCalls > 1 {
				return "", "", "", 0, errors.New("paypal order already captured")
			}
			return captureID, "COMPLETED", "USD", 1000, nil
		},
	}
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, id string) (*models.BillingPayment, error) {
			if id != orderID {
				return nil, sharederrors.ErrNotFound
			}
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
				Status: paymentStatus, Metadata: paymentMetadata,
			}, nil
		},
		completePayPalCaptureFunc: func(_ context.Context, _, _, _ string) (bool, error) {
			paymentStatus = models.PaymentStatusCompleted
			paymentMetadata = []byte(`{"paypal_capture_id":"CAP-1"}`)
			return true, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	first, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.NoError(t, err)
	second, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, first.CaptureID, second.CaptureID)
	assert.Equal(t, 1, captureCalls)
	balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
	require.NoError(t, balanceErr)
	assert.Equal(t, int64(1000), balance)
}

func TestPaymentService_PayPalWebhookAfterDirectCaptureDoesNotDoubleCredit(t *testing.T) {
	orderID := "ORDER-1"
	captureID := "CAP-1"
	call := 0
	paymentStatus := models.PaymentStatusPending
	paymentMetadata := []byte("{}")
	provider := &mockPaymentProvider{
		name: "paypal",
		captureFunc: func(_ context.Context, _ string) (string, string, string, int64, error) {
			return captureID, "COMPLETED", "USD", 1000, nil
		},
		handleWebhookFunc: func(_ context.Context, _ []byte, _ string) (*payments.WebhookEvent, error) {
			call++
			return &payments.WebhookEvent{
				Type:           payments.WebhookEventPaymentCompleted,
				PaymentID:      captureID,
				AmountCents:    1000,
				Currency:       "USD",
				IdempotencyKey: "paypal:capture:" + captureID,
				Metadata: map[string]string{
					"customer_id": "cust_1",
					"payment_id":  orderID,
				},
			}, nil
		},
	}
	repo := &mockBillingPaymentRepo{
		getByGatewayFunc: func(_ context.Context, _, id string) (*models.BillingPayment, error) {
			if id != orderID {
				return nil, sharederrors.ErrNotFound
			}
			return &models.BillingPayment{
				ID: "pay_1", CustomerID: "cust_1", Gateway: "paypal",
				GatewayPaymentID: &orderID, Amount: 1000, Currency: "USD",
				Status: paymentStatus, Metadata: paymentMetadata,
			}, nil
		},
		completePayPalCaptureFunc: func(_ context.Context, _, _, gotCaptureID string) (bool, error) {
			if paymentStatus == models.PaymentStatusCompleted {
				return gotCaptureID == captureID, nil
			}
			paymentStatus = models.PaymentStatusCompleted
			paymentMetadata = []byte(`{"paypal_capture_id":"CAP-1"}`)
			return true, nil
		},
	}
	txRepo := newTrackingBillingTxRepo()
	svc := newPaymentServiceWithLedger(provider, repo, txRepo)

	_, err := svc.CapturePayPalOrder(context.Background(), "cust_1", orderID)
	require.NoError(t, err)
	err = svc.HandleWebhook(context.Background(), "paypal", []byte("{}"), "sig")
	require.NoError(t, err)
	assert.Equal(t, 1, call)
	balance, balanceErr := txRepo.GetBalance(context.Background(), "cust_1")
	require.NoError(t, balanceErr)
	assert.Equal(t, int64(1000), balance)
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
