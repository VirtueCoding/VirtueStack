package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock repo ---

type mockBillingTxRepo struct {
	creditAccountFn  func(ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error)
	debitAccountFn   func(ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string) (*models.BillingTransaction, error)
	getBalanceFn     func(ctx context.Context, customerID string) (int64, error)
	listByCustomerFn func(ctx context.Context, customerID string, filter models.PaginationParams) ([]models.BillingTransaction, bool, string, error)
}

func (m *mockBillingTxRepo) CreditAccount(ctx context.Context, customerID string, amount int64, description string, idempotencyKey *string) (*models.BillingTransaction, error) {
	return m.creditAccountFn(ctx, customerID, amount, description, idempotencyKey)
}

func (m *mockBillingTxRepo) DebitAccount(ctx context.Context, customerID string, amount int64, description string, referenceType, referenceID, idempotencyKey *string) (*models.BillingTransaction, error) {
	return m.debitAccountFn(ctx, customerID, amount, description, referenceType, referenceID, idempotencyKey)
}

func (m *mockBillingTxRepo) GetBalance(ctx context.Context, customerID string) (int64, error) {
	return m.getBalanceFn(ctx, customerID)
}

func (m *mockBillingTxRepo) ListByCustomer(ctx context.Context, customerID string, filter models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
	return m.listByCustomerFn(ctx, customerID, filter)
}

// --- helpers ---

func newTestBillingService(txRepo services.BillingTransactionRepo) *services.BillingLedgerService {
	return services.NewBillingLedgerService(services.BillingLedgerServiceConfig{
		TransactionRepo: txRepo,
		Logger:          testAuthHandlerLogger(),
	})
}

func billingCustomerHandler(svc *services.BillingLedgerService) *CustomerHandler {
	return &CustomerHandler{
		billingLedgerService: svc,
		logger:               testAuthHandlerLogger(),
	}
}

func billingRouter(userID string) *gin.Engine {
	router := setupTestRouter()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	})
	return router
}

type topUpPaymentProviderStub struct {
	name              string
	createSessionFunc func(ctx context.Context, req payments.PaymentRequest) (*payments.PaymentSession, error)
}

func (s *topUpPaymentProviderStub) Name() string { return s.name }

func (s *topUpPaymentProviderStub) CreatePaymentSession(
	ctx context.Context, req payments.PaymentRequest,
) (*payments.PaymentSession, error) {
	if s.createSessionFunc != nil {
		return s.createSessionFunc(ctx, req)
	}
	return &payments.PaymentSession{
		ID:               "sess_1",
		GatewaySessionID: "gw_1",
		PaymentURL:       "https://pay.example.test/checkout",
	}, nil
}

func (*topUpPaymentProviderStub) HandleWebhook(
	context.Context, []byte, string,
) (*payments.WebhookEvent, error) {
	return nil, nil
}

func (*topUpPaymentProviderStub) GetPaymentStatus(
	context.Context, string,
) (*payments.PaymentStatus, error) {
	return nil, nil
}

func (*topUpPaymentProviderStub) RefundPayment(
	context.Context, string, int64, string,
) (*payments.RefundResult, error) {
	return nil, nil
}

func (*topUpPaymentProviderStub) ValidateConfig() error { return nil }

type topUpPaymentRepoStub struct{}

func (*topUpPaymentRepoStub) Create(_ context.Context, payment *models.BillingPayment) error {
	payment.ID = "pay_test_1"
	return nil
}

func (*topUpPaymentRepoStub) GetByID(context.Context, string) (*models.BillingPayment, error) {
	return nil, errors.New("not implemented")
}

func (*topUpPaymentRepoStub) GetByGatewayPaymentID(
	context.Context, string, string,
) (*models.BillingPayment, error) {
	return nil, errors.New("not implemented")
}

func (*topUpPaymentRepoStub) UpdateStatus(context.Context, string, string, *string) error {
	return nil
}

func (*topUpPaymentRepoStub) ListByCustomer(
	context.Context, string, models.PaginationParams,
) ([]models.BillingPayment, bool, string, error) {
	return nil, false, "", nil
}

func (*topUpPaymentRepoStub) ListAll(
	context.Context, repository.BillingPaymentListFilter,
) ([]models.BillingPayment, bool, string, error) {
	return nil, false, "", nil
}

type topUpCustomerRepoStub struct {
	customer *models.Customer
}

func (s *topUpCustomerRepoStub) GetByID(context.Context, string) (*models.Customer, error) {
	if s.customer == nil {
		return nil, errors.New("customer not found")
	}
	return s.customer, nil
}

func (*topUpCustomerRepoStub) GetByEmail(context.Context, string) (*models.Customer, error) {
	return nil, errors.New("not implemented")
}

func (*topUpCustomerRepoStub) UpdateStatus(context.Context, string, string) error {
	return nil
}

func newTopUpPaymentService(t *testing.T, provider *topUpPaymentProviderStub) *services.PaymentService {
	t.Helper()

	reg := payments.NewPaymentRegistry()
	require.NoError(t, reg.Register(provider.name, provider))

	return services.NewPaymentService(services.PaymentServiceConfig{
		PaymentRegistry: reg,
		LedgerService:   newTestBillingService(&mockBillingTxRepo{}),
		PaymentRepo:     &topUpPaymentRepoStub{},
		Logger:          testAuthHandlerLogger(),
	})
}

// --- customer billing handler tests ---

func TestGetBillingBalance_Success(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	txRepo := &mockBillingTxRepo{
		getBalanceFn: func(_ context.Context, _ string) (int64, error) {
			return 15000, nil
		},
	}

	router := billingRouter(testCustomerID)
	handler := billingCustomerHandler(newTestBillingService(txRepo))
	router.GET("/billing/balance", handler.GetBillingBalance)

	req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(15000), data["balance"])
	assert.Equal(t, "USD", data["currency"])
}

func TestGetBillingBalance_ServiceError(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	txRepo := &mockBillingTxRepo{
		getBalanceFn: func(_ context.Context, _ string) (int64, error) {
			return 0, errors.New("database connection lost")
		},
	}

	router := billingRouter(testCustomerID)
	handler := billingCustomerHandler(newTestBillingService(txRepo))
	router.GET("/billing/balance", handler.GetBillingBalance)

	req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "BILLING_BALANCE_FAILED", errorObj["code"])
}

func TestCustomerListBillingTransactions_Success(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	txRepo := &mockBillingTxRepo{
		listByCustomerFn: func(_ context.Context, customerID string, _ models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
			return []models.BillingTransaction{
				{
					ID:           "tx-1",
					CustomerID:   customerID,
					Type:         models.BillingTxTypeCredit,
					Amount:       10000,
					BalanceAfter: 10000,
					Description:  "deposit",
					CreatedAt:    time.Now(),
				},
				{
					ID:           "tx-2",
					CustomerID:   customerID,
					Type:         models.BillingTxTypeDebit,
					Amount:       500,
					BalanceAfter: 9500,
					Description:  "vm usage",
					CreatedAt:    time.Now(),
				},
			}, false, "tx-2", nil
		},
	}

	router := billingRouter(testCustomerID)
	handler := billingCustomerHandler(newTestBillingService(txRepo))
	router.GET("/billing/transactions", handler.ListBillingTransactions)

	req := httptest.NewRequest(http.MethodGet, "/billing/transactions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)
}

func TestCustomerListBillingTransactions_Empty(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	txRepo := &mockBillingTxRepo{
		listByCustomerFn: func(_ context.Context, _ string, _ models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
			return []models.BillingTransaction{}, false, "", nil
		},
	}

	router := billingRouter(testCustomerID)
	handler := billingCustomerHandler(newTestBillingService(txRepo))
	router.GET("/billing/transactions", handler.ListBillingTransactions)

	req := httptest.NewRequest(http.MethodGet, "/billing/transactions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Empty(t, data)
}

func TestGetBillingUsage_Success(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	txRepo := &mockBillingTxRepo{
		getBalanceFn: func(_ context.Context, _ string) (int64, error) {
			return 8750, nil
		},
	}

	router := billingRouter(testCustomerID)
	handler := billingCustomerHandler(newTestBillingService(txRepo))
	router.GET("/billing/usage", handler.GetBillingUsage)

	req := httptest.NewRequest(http.MethodGet, "/billing/usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(8750), data["balance"])
	assert.Equal(t, "USD", data["currency"])
}

func TestInitiateTopUp_ValidationFailureReturnsErrorResponse(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	providerCalled := false
	paymentService := newTopUpPaymentService(t, &topUpPaymentProviderStub{
		name: "stripe",
		createSessionFunc: func(_ context.Context, _ payments.PaymentRequest) (*payments.PaymentSession, error) {
			providerCalled = true
			return &payments.PaymentSession{
				ID:               "sess_invalid",
				GatewaySessionID: "gw_invalid",
				PaymentURL:       "https://pay.example.test/invalid",
			}, nil
		},
	})

	handler := &CustomerHandler{
		billingLedgerService: newTestBillingService(&mockBillingTxRepo{}),
		paymentService:       paymentService,
		customerRepo: &topUpCustomerRepoStub{
			customer: &models.Customer{Email: "crypto@example.test"},
		},
		consoleBaseURL: "https://portal.example.test",
		logger:         testAuthHandlerLogger(),
	}

	router := billingRouter(testCustomerID)
	router.POST("/billing/top-up", handler.InitiateTopUp)

	req := httptest.NewRequest(
		http.MethodPost,
		"/billing/top-up",
		bytes.NewBufferString(`{"gateway":"stripe","amount":1000}`),
	)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, providerCalled)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	errorObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", errorObj["code"])
}

func TestInitiateTopUp_AcceptsCryptoGatewayWithoutClientRedirects(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	providerCalled := false
	var gotReq payments.PaymentRequest
	paymentService := newTopUpPaymentService(t, &topUpPaymentProviderStub{
		name: "crypto",
		createSessionFunc: func(_ context.Context, req payments.PaymentRequest) (*payments.PaymentSession, error) {
			providerCalled = true
			gotReq = req
			return &payments.PaymentSession{
				ID:               "sess_crypto",
				GatewaySessionID: "gw_crypto",
				PaymentURL:       "https://pay.example.test/crypto",
			}, nil
		},
	})

	handler := &CustomerHandler{
		billingLedgerService: newTestBillingService(&mockBillingTxRepo{}),
		paymentService:       paymentService,
		customerRepo: &topUpCustomerRepoStub{
			customer: &models.Customer{Email: "crypto@example.test"},
		},
		consoleBaseURL: "https://portal.example.test",
		logger:         testAuthHandlerLogger(),
	}

	router := billingRouter(testCustomerID)
	router.POST("/billing/top-up", handler.InitiateTopUp)

	req := httptest.NewRequest(
		http.MethodPost,
		"/billing/top-up",
		bytes.NewBufferString(`{"gateway":"crypto","amount":1000,"currency":"USD"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, providerCalled)
	assert.Equal(t, "crypto@example.test", gotReq.CustomerEmail)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://pay.example.test/crypto", data["payment_url"])
}

func TestInitiateTopUp_DerivesTrustedPayPalRedirects(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	var gotReq payments.PaymentRequest
	paymentService := newTopUpPaymentService(t, &topUpPaymentProviderStub{
		name: "paypal",
		createSessionFunc: func(_ context.Context, req payments.PaymentRequest) (*payments.PaymentSession, error) {
			gotReq = req
			return &payments.PaymentSession{
				ID:               "sess_paypal",
				GatewaySessionID: "gw_paypal",
				PaymentURL:       "https://pay.example.test/paypal",
			}, nil
		},
	})

	handler := &CustomerHandler{
		billingLedgerService: newTestBillingService(&mockBillingTxRepo{}),
		paymentService:       paymentService,
		customerRepo: &topUpCustomerRepoStub{
			customer: &models.Customer{Email: "paypal@example.test"},
		},
		consoleBaseURL: "https://portal.example.test",
		logger:         testAuthHandlerLogger(),
	}

	router := billingRouter(testCustomerID)
	router.POST("/billing/top-up", handler.InitiateTopUp)

	req := httptest.NewRequest(
		http.MethodPost,
		"/billing/top-up",
		bytes.NewBufferString(`{"gateway":"paypal","amount":1000,"currency":"USD","return_url":"https://evil.example.test/approved","cancel_url":"https://evil.example.test/cancelled"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://portal.example.test/billing/paypal-return", gotReq.ReturnURL)
	assert.Equal(t, "https://portal.example.test/billing", gotReq.CancelURL)
}

func TestCapturePayPalPayment_ValidationFailureReturnsErrorResponse(t *testing.T) {
	const testCustomerID = "550e8400-e29b-41d4-a716-446655440000"

	handler := &CustomerHandler{
		logger: testAuthHandlerLogger(),
	}

	router := billingRouter(testCustomerID)
	router.POST("/billing/payments/paypal/capture", handler.CapturePayPalPayment)

	req := httptest.NewRequest(
		http.MethodPost,
		"/billing/payments/paypal/capture",
		bytes.NewBufferString(`{}`),
	)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	errorObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", errorObj["code"])
}
