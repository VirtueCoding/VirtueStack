package provisioning

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateOrGetCustomer_ValidationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		body    string
		wantHTTP int
		errCode string
	}{
		{
			name:     "missing email",
			body:     `{"name":"Test User"}`,
			wantHTTP: http.StatusBadRequest,
			errCode:  "VALIDATION_ERROR",
		},
		{
			name:     "missing name",
			body:     `{"email":"test@example.com"}`,
			wantHTTP: http.StatusBadRequest,
			errCode:  "VALIDATION_ERROR",
		},
		{
			name:     "invalid email format",
			body:     `{"email":"not-valid","name":"Test User"}`,
			wantHTTP: http.StatusBadRequest,
			errCode:  "VALIDATION_ERROR",
		},
		{
			name:     "empty body",
			body:     `{}`,
			wantHTTP: http.StatusBadRequest,
			errCode:  "VALIDATION_ERROR",
		},
		{
			name:     "invalid JSON",
			body:     `{invalid json}`,
			wantHTTP: http.StatusBadRequest,
			errCode:  "INVALID_REQUEST_BODY",
		},
		{
			name:     "zero external_client_id",
			body:     `{"email":"test@example.com","name":"Test User","external_client_id":0}`,
			wantHTTP: http.StatusBadRequest,
			errCode:  "VALIDATION_ERROR",
		},
		{
			name:     "negative external_client_id",
			body:     `{"email":"test@example.com","name":"Test User","external_client_id":-1}`,
			wantHTTP: http.StatusBadRequest,
			errCode:  "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/customers",
				bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			handler := &ProvisioningHandler{
				logger: testProvisioningLogger(),
			}
			handler.CreateOrGetCustomer(c)

			assert.Equal(t, tt.wantHTTP, w.Code)
			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok, "response should have error object")
			assert.Equal(t, tt.errCode, errObj["code"])
		})
	}
}

func TestCreateCustomerResponse_JSONFormat(t *testing.T) {
	tests := []struct {
		name string
		resp CreateCustomerResponse
		want map[string]interface{}
	}{
		{
			name: "new customer",
			resp: CreateCustomerResponse{
				ID:      "new-uuid",
				Email:   "new@example.com",
				Name:    "New User",
				Created: true,
			},
			want: map[string]interface{}{
				"id":      "new-uuid",
				"email":   "new@example.com",
				"name":    "New User",
				"created": true,
			},
		},
		{
			name: "existing customer",
			resp: CreateCustomerResponse{
				ID:      "existing-uuid",
				Email:   "existing@example.com",
				Name:    "Existing User",
				Created: false,
			},
			want: map[string]interface{}{
				"id":      "existing-uuid",
				"email":   "existing@example.com",
				"name":    "Existing User",
				"created": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			require.NoError(t, err)

			var parsed map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &parsed))

			assert.Equal(t, tt.want["id"], parsed["id"])
			assert.Equal(t, tt.want["email"], parsed["email"])
			assert.Equal(t, tt.want["name"], parsed["name"])
			assert.Equal(t, tt.want["created"], parsed["created"])
		})
	}
}

func TestCreateCustomerRequest_ValidFields(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		email string
		cname string
	}{
		{
			name:  "standard email and name",
			body:  `{"email":"test@example.com","name":"Test User"}`,
			email: "test@example.com",
			cname: "Test User",
		},
		{
			name:  "with external_client_id",
			body:  `{"email":"test@example.com","name":"Test User","external_client_id":42}`,
			email: "test@example.com",
			cname: "Test User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req CreateCustomerRequest
			err := json.Unmarshal([]byte(tt.body), &req)
			require.NoError(t, err)
			assert.Equal(t, tt.email, req.Email)
			assert.Equal(t, tt.cname, req.Name)
		})
	}
}
