package customer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
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
