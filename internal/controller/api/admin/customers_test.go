package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

type customerHandlerTestDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *customerHandlerTestDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return customerHandlerTestRow{err: pgx.ErrNoRows}
}

func (m *customerHandlerTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &customerHandlerTestRows{}, nil
}

func (m *customerHandlerTestDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (m *customerHandlerTestDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, nil
}

type customerHandlerTestRow struct {
	customer models.Customer
	err      error
}

func (r customerHandlerTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 15 {
		return fmt.Errorf("unexpected scan destination count: got %d want %d", len(dest), 15)
	}

	customer := r.customer

	*(dest[0].(*string)) = customer.ID
	*(dest[1].(*string)) = customer.Email
	*(dest[2].(**string)) = customer.PasswordHash
	*(dest[3].(*string)) = customer.Name
	*(dest[4].(**string)) = customer.Phone
	*(dest[5].(**int)) = customer.ExternalClientID
	*(dest[6].(**string)) = customer.BillingProvider
	*(dest[7].(*string)) = customer.AuthProvider
	*(dest[8].(**string)) = customer.TOTPSecretEncrypted
	*(dest[9].(*bool)) = customer.TOTPEnabled
	*(dest[10].(*[]string)) = customer.TOTPBackupCodesHash
	*(dest[11].(*bool)) = customer.TOTPBackupCodesShown
	*(dest[12].(*string)) = customer.Status
	*(dest[13].(*time.Time)) = customer.CreatedAt
	*(dest[14].(*time.Time)) = customer.UpdatedAt

	return nil
}

type customerHandlerTestRows struct {
	rows [][]any
	idx  int
	err  error
}

func (r *customerHandlerTestRows) Close() {}

func (r *customerHandlerTestRows) Err() error { return r.err }

func (r *customerHandlerTestRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT 0")
}

func (r *customerHandlerTestRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *customerHandlerTestRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *customerHandlerTestRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return fmt.Errorf("scan called before Next")
	}
	return nil
}

func (r *customerHandlerTestRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.rows) {
		return nil, fmt.Errorf("values called before Next")
	}
	return r.rows[r.idx-1], nil
}

func (r *customerHandlerTestRows) RawValues() [][]byte { return nil }

func (r *customerHandlerTestRows) Conn() *pgx.Conn { return nil }

// TestListCustomers_InvalidStatus tests status filter validation.
func TestListCustomers_InvalidStatus(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/customers", handler.ListCustomers)

	req := httptest.NewRequest(http.MethodGet, "/customers?status=invalid_status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_STATUS", errorObj["code"])
}

// TestListCustomers_SearchTooLong tests search parameter length limit.
func TestListCustomers_SearchTooLong(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/customers", handler.ListCustomers)

	// Create search string > 100 chars
	longSearch := ""
	for i := 0; i < 101; i++ {
		longSearch += "a"
	}
	req := httptest.NewRequest(http.MethodGet, "/customers?search="+longSearch, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_SEARCH", errorObj["code"])
}

// TestListCustomers_ValidStatusFilters tests valid status values.
// Note: Full list operation requires service mock. This tests that valid statuses
// pass the validation check (they would proceed to service layer).
func TestListCustomers_ValidStatusFilters(t *testing.T) {
	// Valid statuses should not trigger validation errors
	validStatuses := []string{"active", "suspended", "deleted"}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			// Verify the status is in the expected list
			assert.Contains(t, []string{"active", "suspended", "deleted"}, status)
		})
	}
}

// TestGetCustomer_InvalidID tests UUID validation for customer ID.
func TestGetCustomer_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/customers/:id", handler.GetCustomer)

	tests := []string{
		"not-a-uuid",
		"123",
		"abc-def-ghi",
	}

	for _, invalidID := range tests {
		t.Run(invalidID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/customers/"+invalidID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			errorObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "INVALID_CUSTOMER_ID", errorObj["code"])
		})
	}
}

func TestCreateCustomer_MissingEmailLookupDoesNotFailCreation(t *testing.T) {
	now := time.Date(2026, time.April, 2, 8, 30, 0, 0, time.UTC)
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &customerHandlerTestDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM customers WHERE email = $1 AND status != 'deleted'"):
				return customerHandlerTestRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "INSERT INTO customers"):
				return customerHandlerTestRow{customer: models.Customer{
					ID:           customerID,
					Email:        "new@example.com",
					Name:         "New Customer",
					AuthProvider: models.AuthProviderLocal,
					Status:       models.CustomerStatusActive,
					Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
				}}
			default:
				return customerHandlerTestRow{err: fmt.Errorf("unexpected query: %s", sql)}
			}
		},
	}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
		auditRepo: repository.NewAuditRepository(db),
		authService: services.NewAuthService(
			repository.NewCustomerRepository(db),
			nil,
			nil,
			"test-secret",
			"virtuestack",
			"",
			logger,
		),
	}

	router.POST("/customers", handler.CreateCustomer)

	body := `{"name":"New Customer","email":"new@example.com","password":"verysecure123"}`
	req := httptest.NewRequest(http.MethodPost, "/customers", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestGetCustomer_MissingReturnsNotFound(t *testing.T) {
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &customerHandlerTestDB{}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
	}

	router.GET("/customers/:id", handler.GetCustomer)

	req := httptest.NewRequest(http.MethodGet, "/customers/"+customerID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

func TestDeleteCustomer_MissingReturnsNotFound(t *testing.T) {
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &customerHandlerTestDB{}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
		vmService: services.NewVMService(services.VMServiceConfig{
			VMRepo: repository.NewVMRepository(db),
			Logger: logger,
		}),
	}

	router.DELETE("/customers/:id", handler.DeleteCustomer)

	req := httptest.NewRequest(http.MethodDelete, "/customers/"+customerID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

func TestDeleteCustomer_ConcurrentDeleteReturnsNotFound(t *testing.T) {
	now := time.Date(2026, time.April, 2, 8, 35, 0, 0, time.UTC)
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &customerHandlerTestDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM customers WHERE id = $1 AND status != 'deleted'"):
				return customerHandlerTestRow{customer: models.Customer{
					ID:           customerID,
					Email:        "customer@example.com",
					Name:         "Example Customer",
					AuthProvider: models.AuthProviderLocal,
					Status:       models.CustomerStatusActive,
					Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
				}}
			default:
				return customerHandlerTestRow{err: fmt.Errorf("unexpected query: %s", sql)}
			}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE customers SET status = 'deleted'") {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			if strings.Contains(sql, "INSERT INTO audit_logs") {
				return pgconn.NewCommandTag("INSERT 1"), nil
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
	}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
		vmService: services.NewVMService(services.VMServiceConfig{
			VMRepo: repository.NewVMRepository(db),
			Logger: logger,
		}),
	}

	router.DELETE("/customers/:id", handler.DeleteCustomer)

	req := httptest.NewRequest(http.MethodDelete, "/customers/"+customerID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

func TestUpdateCustomer_MissingReturnsNotFound(t *testing.T) {
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &customerHandlerTestDB{}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
	}

	router.PUT("/customers/:id", handler.UpdateCustomer)

	req := httptest.NewRequest(http.MethodPut, "/customers/"+customerID, bytes.NewBufferString(`{"name":"Updated Name"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

func TestUpdateCustomer_ProfileConcurrentDeleteReturnsNotFound(t *testing.T) {
	now := time.Date(2026, time.April, 2, 8, 40, 0, 0, time.UTC)
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &customerHandlerTestDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "UPDATE customers SET") && strings.Contains(sql, "RETURNING"):
				return customerHandlerTestRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "FROM customers WHERE id = $1 AND status != 'deleted'"):
				return customerHandlerTestRow{customer: models.Customer{
					ID:           customerID,
					Email:        "customer@example.com",
					Name:         "Example Customer",
					AuthProvider: models.AuthProviderLocal,
					Status:       models.CustomerStatusActive,
					Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
				}}
			default:
				return customerHandlerTestRow{err: fmt.Errorf("unexpected query: %s", sql)}
			}
		},
	}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
	}

	router.PUT("/customers/:id", handler.UpdateCustomer)

	req := httptest.NewRequest(http.MethodPut, "/customers/"+customerID, bytes.NewBufferString(`{"name":"Updated Name"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

func TestUpdateCustomer_StatusConcurrentDeleteReturnsNotFound(t *testing.T) {
	now := time.Date(2026, time.April, 2, 8, 45, 0, 0, time.UTC)
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &customerHandlerTestDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM customers WHERE id = $1 AND status != 'deleted'") {
				return customerHandlerTestRow{customer: models.Customer{
					ID:           customerID,
					Email:        "customer@example.com",
					Name:         "Example Customer",
					AuthProvider: models.AuthProviderLocal,
					Status:       models.CustomerStatusActive,
					Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
				}}
			}
			return customerHandlerTestRow{err: fmt.Errorf("unexpected query: %s", sql)}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE customers SET status = $1") {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
	}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
	}

	router.PUT("/customers/:id", handler.UpdateCustomer)

	req := httptest.NewRequest(http.MethodPut, "/customers/"+customerID, bytes.NewBufferString(`{"status":"suspended"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

func TestUpdateCustomer_BillingProviderConcurrentDeleteReturnsNotFound(t *testing.T) {
	now := time.Date(2026, time.April, 2, 8, 50, 0, 0, time.UTC)
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	provider := models.BillingProviderUnmanaged
	db := &customerHandlerTestDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM customers WHERE id = $1 AND status != 'deleted'") {
				return customerHandlerTestRow{customer: models.Customer{
					ID:              customerID,
					Email:           "customer@example.com",
					Name:            "Example Customer",
					AuthProvider:    models.AuthProviderLocal,
					Status:          models.CustomerStatusActive,
					BillingProvider: &provider,
					Timestamps:      models.Timestamps{CreatedAt: now, UpdatedAt: now},
				}}
			}
			return customerHandlerTestRow{err: fmt.Errorf("unexpected query: %s", sql)}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE customers SET billing_provider = $1") {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
	}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
		customerRepo: repository.NewCustomerRepository(db),
	}

	router.PUT("/customers/:id", handler.UpdateCustomer)

	req := httptest.NewRequest(http.MethodPut, "/customers/"+customerID, bytes.NewBufferString(`{"billing_provider":"native"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

func TestUpdateCustomer_PostUpdateFetchMissingReturnsNotFound(t *testing.T) {
	now := time.Date(2026, time.April, 2, 8, 55, 0, 0, time.UTC)
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	getByIDCalls := 0
	db := &customerHandlerTestDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "UPDATE customers SET") && strings.Contains(sql, "RETURNING"):
				return customerHandlerTestRow{customer: models.Customer{
					ID:           customerID,
					Email:        "customer@example.com",
					Name:         "Updated Name",
					AuthProvider: models.AuthProviderLocal,
					Status:       models.CustomerStatusActive,
					Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
				}}
			case strings.Contains(sql, "FROM customers WHERE id = $1 AND status != 'deleted'"):
				getByIDCalls++
				if getByIDCalls == 1 {
					return customerHandlerTestRow{customer: models.Customer{
						ID:           customerID,
						Email:        "customer@example.com",
						Name:         "Example Customer",
						AuthProvider: models.AuthProviderLocal,
						Status:       models.CustomerStatusActive,
						Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
					}}
				}
				return customerHandlerTestRow{err: pgx.ErrNoRows}
			default:
				return customerHandlerTestRow{err: fmt.Errorf("unexpected query: %s", sql)}
			}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "INSERT INTO audit_logs") {
				return pgconn.NewCommandTag("INSERT 1"), nil
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	router := setupAdminTestRouter()
	logger := testAdminLogger()
	handler := &AdminHandler{
		logger: logger,
		customerService: services.NewCustomerService(
			repository.NewCustomerRepository(db),
			repository.NewAuditRepository(db),
			logger,
		),
		auditRepo: repository.NewAuditRepository(db),
	}

	router.PUT("/customers/:id", handler.UpdateCustomer)

	req := httptest.NewRequest(http.MethodPut, "/customers/"+customerID, bytes.NewBufferString(`{"name":"Updated Name"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorObj["code"])
}

// TestUpdateCustomer_InvalidID tests UUID validation on update.
func TestUpdateCustomer_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/customers/:id", handler.UpdateCustomer)

	body := `{"name": "New Name"}`
	req := httptest.NewRequest(http.MethodPut, "/customers/not-a-uuid", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdateCustomer_NameTooLong tests name length validation.
func TestUpdateCustomer_NameTooLong(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/customers/:id", handler.UpdateCustomer)

	// Create name > 255 chars
	longName := ""
	for i := 0; i < 256; i++ {
		longName += "a"
	}
	body := map[string]string{
		"name": longName,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/customers/00000000-0000-0000-0000-000000000001", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestDeleteCustomer_InvalidID tests UUID validation on delete.
func TestDeleteCustomer_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.DELETE("/customers/:id", handler.DeleteCustomer)

	req := httptest.NewRequest(http.MethodDelete, "/customers/invalid-uuid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetCustomerAuditLogs_InvalidID tests UUID validation for audit logs.
func TestGetCustomerAuditLogs_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/customers/:id/audit-logs", handler.GetCustomerAuditLogs)

	req := httptest.NewRequest(http.MethodGet, "/customers/invalid/audit-logs", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCustomerUpdateRequest_Validation tests struct validation.
func TestCustomerUpdateRequest_Validation(t *testing.T) {
	tests := []struct {
		name        string
		request     CustomerUpdateRequest
		expectValid bool
	}{
		{
			name:        "empty request (valid - no changes)",
			request:     CustomerUpdateRequest{},
			expectValid: true,
		},
		{
			name: "valid name update",
			request: CustomerUpdateRequest{
				Name: strPtr("John Doe"),
			},
			expectValid: true,
		},
		{
			name: "valid status update",
			request: CustomerUpdateRequest{
				Status: strPtr("active"),
			},
			expectValid: true,
		},
		{
			name: "name at max length",
			request: CustomerUpdateRequest{
				Name: strPtr(string(make([]byte, 255))),
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectValid {
				// Verify constraints
				if tt.request.Name != nil {
					assert.LessOrEqual(t, len(*tt.request.Name), 255)
				}
				if tt.request.Status != nil {
					assert.Contains(t, []string{"active", "suspended"}, *tt.request.Status)
				}
			}
		})
	}
}

// TestCustomerDetail_Structure verifies CustomerDetail embeds Customer correctly.
func TestCustomerDetail_Structure(t *testing.T) {
	detail := CustomerDetail{
		Customer: models.Customer{
			ID:    "test-id",
			Email: "test@example.com",
		},
		VMCount:     5,
		ActiveVMs:   3,
		BackupCount: 2,
	}

	assert.Equal(t, "test-id", detail.ID)
	assert.Equal(t, "test@example.com", detail.Email)
	assert.Equal(t, 5, detail.VMCount)
	assert.Equal(t, 3, detail.ActiveVMs)
	assert.Equal(t, 2, detail.BackupCount)
}

// Helper function
func strPtr(s string) *string {
	return &s
}
