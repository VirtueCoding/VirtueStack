package provisioning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func provisioningCustomerRow(id string, externalClientID int) []any {
	now := time.Now().UTC()
	return []any{
		id, "customer@example.test", (*string)(nil), "Customer", (*string)(nil),
		&externalClientID, (*string)(nil), "local", (*string)(nil), false,
		[]string{}, false, "active",
		now, now,
	}
}

func TestCreateSSOTokenAllowsCustomerOnlyScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &provisioningFakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM customers WHERE id = $1"):
				return &provisioningFakeRow{values: provisioningCustomerRow(customerID, 42)}
			case strings.Contains(sql, "INSERT INTO sso_tokens"):
				return &provisioningFakeRow{values: []any{"550e8400-e29b-41d4-a716-446655440099", time.Now().UTC()}}
			default:
				return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}
	handler := &ProvisioningHandler{
		customerRepo: repository.NewCustomerRepository(db),
		ssoTokenRepo: repository.NewSSOTokenRepository(db),
		logger:       testProvisioningLogger(),
	}
	router := gin.New()
	router.POST("/sso-tokens", handler.CreateSSOToken)

	body := `{"customer_id":"` + customerID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/sso-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data CreateSSOTokenResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Data.Token)
	assert.Empty(t, resp.Data.VMID)
	assert.Equal(t, "/vms", resp.Data.RedirectPath)
	assert.NotContains(t, resp.Data.RedirectPath, "://")
}

func TestCreateSSOTokenRequiresAllVMScopedAssertions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	vmID := "550e8400-e29b-41d4-a716-446655440010"
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		name        string
		body        string
		wantCode    int
		errCode     string
		wantCreated bool
	}{
		{
			name:     "service id only is rejected",
			body:     `{"external_service_id":123}`,
			wantCode: http.StatusBadRequest,
			errCode:  "VM_ID_REQUIRED",
		},
		{
			name:     "vm id with customer id but no service id is rejected",
			body:     `{"vm_id":"` + vmID + `","customer_id":"` + customerID + `"}`,
			wantCode: http.StatusBadRequest,
			errCode:  "EXTERNAL_SERVICE_ID_REQUIRED",
		},
		{
			name:     "service id with external client id but no vm id is rejected",
			body:     `{"external_service_id":123,"external_client_id":42}`,
			wantCode: http.StatusBadRequest,
			errCode:  "VM_ID_REQUIRED",
		},
		{
			name:     "service id with customer id but no vm id is rejected",
			body:     `{"external_service_id":123,"customer_id":"` + customerID + `"}`,
			wantCode: http.StatusBadRequest,
			errCode:  "VM_ID_REQUIRED",
		},
		{
			name:        "vm id with service id and external client id succeeds",
			body:        `{"vm_id":"` + vmID + `","external_service_id":123,"external_client_id":42}`,
			wantCode:    http.StatusCreated,
			wantCreated: true,
		},
		{
			name:        "vm id with service id and customer id succeeds",
			body:        `{"vm_id":"` + vmID + `","external_service_id":123,"customer_id":"` + customerID + `"}`,
			wantCode:    http.StatusCreated,
			wantCreated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := 0
			db := &provisioningFakeDB{
				queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "FROM vms WHERE id = $1"):
						return &provisioningFakeRow{values: provisioningVMRow(vmID, customerID, 123)}
					case strings.Contains(sql, "FROM vms WHERE external_service_id = $1"):
						return &provisioningFakeRow{values: provisioningVMRow(vmID, customerID, 123)}
					case strings.Contains(sql, "FROM customers WHERE id = $1"):
						return &provisioningFakeRow{values: provisioningCustomerRow(customerID, 42)}
					case strings.Contains(sql, "INSERT INTO sso_tokens"):
						created++
						return &provisioningFakeRow{values: []any{"550e8400-e29b-41d4-a716-446655440099", time.Now().UTC()}}
					default:
						return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
					}
				},
			}
			handler := &ProvisioningHandler{
				vmRepo:       repository.NewVMRepository(db),
				customerRepo: repository.NewCustomerRepository(db),
				ssoTokenRepo: repository.NewSSOTokenRepository(db),
				logger:       testProvisioningLogger(),
			}
			router := gin.New()
			router.POST("/sso-tokens", handler.CreateSSOToken)
			req := httptest.NewRequest(http.MethodPost, "/sso-tokens", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			if tt.errCode != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				errObj, ok := resp["error"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.errCode, errObj["code"])
				assert.Zero(t, created)
				return
			}
			if tt.wantCreated {
				assert.Equal(t, 1, created)
				var resp struct {
					Data CreateSSOTokenResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.Data.Token)
				assert.Equal(t, "/vms/"+vmID, resp.Data.RedirectPath)
			}
		})
	}
}
