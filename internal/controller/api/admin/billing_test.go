package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock repos for billing tests ---

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

type mockExchangeRateRepo struct {
	getRateFn    func(ctx context.Context, currency string) (*models.ExchangeRate, error)
	upsertRateFn func(ctx context.Context, currency string, rate float64, source string) error
	listAllFn    func(ctx context.Context) ([]models.ExchangeRate, error)
}

func (m *mockExchangeRateRepo) GetRate(ctx context.Context, currency string) (*models.ExchangeRate, error) {
	return m.getRateFn(ctx, currency)
}

func (m *mockExchangeRateRepo) UpsertRate(ctx context.Context, currency string, rate float64, source string) error {
	return m.upsertRateFn(ctx, currency, rate, source)
}

func (m *mockExchangeRateRepo) ListAll(ctx context.Context) ([]models.ExchangeRate, error) {
	return m.listAllFn(ctx)
}

// noopAuditDB satisfies repository.DB so AuditRepository.Append succeeds without a real database.
type noopAuditDB struct{}

func (noopAuditDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return nil }
func (noopAuditDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}
func (noopAuditDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noopAuditDB) Begin(_ context.Context) (pgx.Tx, error) { return nil, nil }

// --- helpers ---

func newTestBillingService(txRepo services.BillingTransactionRepo) *services.BillingLedgerService {
	return services.NewBillingLedgerService(services.BillingLedgerServiceConfig{
		TransactionRepo: txRepo,
		Logger:          testAdminLogger(),
	})
}

func newTestExchangeRateService(rateRepo services.ExchangeRateRepo) *services.ExchangeRateService {
	return services.NewExchangeRateService(services.ExchangeRateServiceConfig{
		RateRepo: rateRepo,
		Logger:   testAdminLogger(),
	})
}

func newTestAuditRepo() *repository.AuditRepository {
	return repository.NewAuditRepository(&noopAuditDB{})
}

// --- admin billing handler tests ---

func TestListBillingTransactions_ValidCustomerID(t *testing.T) {
	router := setupAdminTestRouter()

	txRepo := &mockBillingTxRepo{
		listByCustomerFn: func(_ context.Context, customerID string, _ models.PaginationParams) ([]models.BillingTransaction, bool, string, error) {
			return []models.BillingTransaction{
				{
					ID:           "tx-1",
					CustomerID:   customerID,
					Type:         models.BillingTxTypeCredit,
					Amount:       10000,
					BalanceAfter: 10000,
					Description:  "initial credit",
					CreatedAt:    time.Now(),
				},
			}, false, "tx-1", nil
		},
	}

	handler := &AdminHandler{
		billingLedgerService: newTestBillingService(txRepo),
		logger:               testAdminLogger(),
	}
	router.GET("/billing/transactions", handler.ListBillingTransactions)

	customerID := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest(http.MethodGet, "/billing/transactions?customer_id="+customerID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

func TestListBillingTransactions_InvalidCustomerID(t *testing.T) {
	router := setupAdminTestRouter()

	handler := &AdminHandler{
		billingLedgerService: newTestBillingService(&mockBillingTxRepo{}),
		logger:               testAdminLogger(),
	}
	router.GET("/billing/transactions", handler.ListBillingTransactions)

	req := httptest.NewRequest(http.MethodGet, "/billing/transactions?customer_id=not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_CUSTOMER_ID", errorObj["code"])
}

func TestAdminCreditAdjustment_PositiveAmount(t *testing.T) {
	router := setupAdminTestRouter()

	txRepo := &mockBillingTxRepo{
		creditAccountFn: func(_ context.Context, customerID string, amount int64, description string, _ *string) (*models.BillingTransaction, error) {
			return &models.BillingTransaction{
				ID:           "tx-credit-1",
				CustomerID:   customerID,
				Type:         models.BillingTxTypeCredit,
				Amount:       amount,
				BalanceAfter: amount,
				Description:  description,
				CreatedAt:    time.Now(),
			}, nil
		},
	}

	handler := &AdminHandler{
		billingLedgerService: newTestBillingService(txRepo),
		auditRepo:            newTestAuditRepo(),
		logger:               testAdminLogger(),
	}
	router.POST("/billing/credit", handler.AdminCreditAdjustment)

	customerID := "550e8400-e29b-41d4-a716-446655440000"
	body := map[string]any{"amount": 5000, "description": "Manual top-up"}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/billing/credit?customer_id="+customerID, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "tx-credit-1", data["id"])
	assert.Equal(t, models.BillingTxTypeCredit, data["type"])
}

func TestAdminCreditAdjustment_MissingCustomerID(t *testing.T) {
	router := setupAdminTestRouter()

	handler := &AdminHandler{
		billingLedgerService: newTestBillingService(&mockBillingTxRepo{}),
		logger:               testAdminLogger(),
	}
	router.POST("/billing/credit", handler.AdminCreditAdjustment)

	body := map[string]any{"amount": 100, "description": "Test credit"}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/billing/credit", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_CUSTOMER_ID", errorObj["code"])
}

func TestGetCustomerBalance_ValidCustomer(t *testing.T) {
	router := setupAdminTestRouter()

	txRepo := &mockBillingTxRepo{
		getBalanceFn: func(_ context.Context, _ string) (int64, error) {
			return 25000, nil
		},
	}

	handler := &AdminHandler{
		billingLedgerService: newTestBillingService(txRepo),
		logger:               testAdminLogger(),
	}
	router.GET("/billing/balance", handler.GetCustomerBalance)

	customerID := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest(http.MethodGet, "/billing/balance?customer_id="+customerID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(25000), data["balance"])
	assert.Equal(t, "USD", data["currency"])
	assert.Equal(t, customerID, data["customer_id"])
}

func TestGetCustomerBalance_InvalidCustomerID(t *testing.T) {
	router := setupAdminTestRouter()

	handler := &AdminHandler{
		billingLedgerService: newTestBillingService(&mockBillingTxRepo{}),
		logger:               testAdminLogger(),
	}
	router.GET("/billing/balance", handler.GetCustomerBalance)

	req := httptest.NewRequest(http.MethodGet, "/billing/balance?customer_id=bad-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_CUSTOMER_ID", errorObj["code"])
}

func TestListExchangeRates_Success(t *testing.T) {
	router := setupAdminTestRouter()

	rateRepo := &mockExchangeRateRepo{
		listAllFn: func(_ context.Context) ([]models.ExchangeRate, error) {
			return []models.ExchangeRate{
				{Currency: "EUR", RateToUSD: 1.08, Source: models.ExchangeRateSourceAdmin, UpdatedAt: time.Now()},
				{Currency: "GBP", RateToUSD: 1.27, Source: models.ExchangeRateSourceAPI, UpdatedAt: time.Now()},
			}, nil
		},
	}

	handler := &AdminHandler{
		exchangeRateService: newTestExchangeRateService(rateRepo),
		logger:              testAdminLogger(),
	}
	router.GET("/exchange-rates", handler.ListExchangeRates)

	req := httptest.NewRequest(http.MethodGet, "/exchange-rates", nil)
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

func TestUpdateExchangeRate_ValidUpdate(t *testing.T) {
	router := setupAdminTestRouter()

	rateRepo := &mockExchangeRateRepo{
		upsertRateFn: func(_ context.Context, _ string, _ float64, _ string) error {
			return nil
		},
	}

	handler := &AdminHandler{
		exchangeRateService: newTestExchangeRateService(rateRepo),
		auditRepo:           newTestAuditRepo(),
		logger:              testAdminLogger(),
	}
	router.PUT("/exchange-rates/:currency", handler.UpdateExchangeRate)

	body := map[string]any{"rate_to_usd": 1.12}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/exchange-rates/EUR", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "EUR", data["currency"])
	assert.Equal(t, 1.12, data["rate_to_usd"])
}

func TestUpdateExchangeRate_InvalidCurrency(t *testing.T) {
	router := setupAdminTestRouter()

	handler := &AdminHandler{
		exchangeRateService: newTestExchangeRateService(&mockExchangeRateRepo{}),
		logger:              testAdminLogger(),
	}
	router.PUT("/exchange-rates/:currency", handler.UpdateExchangeRate)

	body := map[string]any{"rate_to_usd": 1.12}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/exchange-rates/EU", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_CURRENCY", errorObj["code"])
}
