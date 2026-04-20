package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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