package provisioning

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type provisioningFakeDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (f *provisioningFakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFunc != nil {
		return f.queryRowFunc(ctx, sql, args...)
	}
	return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
}

func (f *provisioningFakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *provisioningFakeDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, fmt.Errorf("not implemented")
}

func (f *provisioningFakeDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, fmt.Errorf("not implemented")
}

type provisioningFakeRow struct {
	values  []any
	scanErr error
}

func (f *provisioningFakeRow) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	if len(dest) != len(f.values) {
		return fmt.Errorf("scan destination count mismatch: got %d want %d", len(dest), len(f.values))
	}
	for i := range dest {
		if err := assignProvisioningValue(dest[i], f.values[i]); err != nil {
			return err
		}
	}
	return nil
}

func assignProvisioningValue(dest any, val any) error {
	if target, ok := dest.(*sql.NullString); ok {
		if val == nil {
			*target = sql.NullString{}
			return nil
		}
		str, ok := val.(string)
		if !ok {
			return fmt.Errorf("cannot assign %T to %T", val, dest)
		}
		*target = sql.NullString{String: str, Valid: true}
		return nil
	}

	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return fmt.Errorf("destination is not a pointer")
	}
	if val == nil {
		dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
		return nil
	}
	v := reflect.ValueOf(val)
	if v.Type().AssignableTo(dv.Elem().Type()) {
		dv.Elem().Set(v)
		return nil
	}
	if v.Type().ConvertibleTo(dv.Elem().Type()) {
		dv.Elem().Set(v.Convert(dv.Elem().Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %T to %T", val, dest)
}

func provisioningVMRow(id, customerID string, externalServiceID int) []any {
	now := time.Now().UTC()
	return []any{
		id, customerID, (*string)(nil), "550e8400-e29b-41d4-a716-446655440001",
		"vm-host", models.VMStatusRunning, 2, 2048,
		40, 1000, 1000,
		int64(0), now,
		"52:54:00:12:34:56", (*string)(nil), (*string)(nil),
		(*string)(nil), &externalServiceID, (*string)(nil),
		now, now, (*time.Time)(nil),
		models.StorageBackendCeph, (*string)(nil), (*string)(nil), (*string)(nil),
	}
}

func TestCreateVMReturnsConflictForExternalServiceOwnedByAnotherCustomer(t *testing.T) {
	tests := []struct {
		name             string
		requestCustomer  string
		existingCustomer string
		wantCode         int
		errCode          string
	}{
		{
			name:             "cross customer duplicate external service",
			requestCustomer:  "550e8400-e29b-41d4-a716-446655440000",
			existingCustomer: "550e8400-e29b-41d4-a716-446655440099",
			wantCode:         http.StatusConflict,
			errCode:          "EXTERNAL_SERVICE_CONFLICT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			externalServiceID := 123
			db := &provisioningFakeDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					if strings.Contains(sql, "FROM vms WHERE external_service_id = $1") {
						require.Equal(t, externalServiceID, args[0])
						return &provisioningFakeRow{values: provisioningVMRow("550e8400-e29b-41d4-a716-446655440010", tt.existingCustomer, externalServiceID)}
					}
					return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
				},
			}
			vmRepo := repository.NewVMRepository(db)
			handler := &ProvisioningHandler{
				vmService: services.NewVMService(services.VMServiceConfig{
					VMRepo: vmRepo,
					Logger: testProvisioningLogger(),
				}),
				logger: testProvisioningLogger(),
			}
			router := gin.New()
			router.POST("/vms", handler.CreateVM)

			body := `{"customer_id":"` + tt.requestCustomer + `","plan_id":"550e8400-e29b-41d4-a716-446655440001","template_id":"550e8400-e29b-41d4-a716-446655440002","hostname":"vm.example.test","external_service_id":123}`
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/vms", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.errCode, errObj["code"])
			assert.NotContains(t, w.Body.String(), "550e8400-e29b-41d4-a716-446655440010")
		})
	}
}

func TestCreateVMReturnsConflictForSameCustomerExternalServiceWithoutTask(t *testing.T) {
	tests := []struct {
		name       string
		customerID string
		wantCode   int
		errCode    string
	}{
		{
			name:       "same customer duplicate external service has no durable task",
			customerID: "550e8400-e29b-41d4-a716-446655440000",
			wantCode:   http.StatusConflict,
			errCode:    "EXTERNAL_SERVICE_CONFLICT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			externalServiceID := 123
			db := &provisioningFakeDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					if strings.Contains(sql, "FROM vms WHERE external_service_id = $1") {
						require.Equal(t, externalServiceID, args[0])
						return &provisioningFakeRow{values: provisioningVMRow("550e8400-e29b-41d4-a716-446655440010", tt.customerID, externalServiceID)}
					}
					return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
				},
			}
			handler := &ProvisioningHandler{
				vmService: services.NewVMService(services.VMServiceConfig{
					VMRepo: repository.NewVMRepository(db),
					Logger: testProvisioningLogger(),
				}),
				logger: testProvisioningLogger(),
			}
			router := gin.New()
			router.POST("/vms", handler.CreateVM)

			body := `{"customer_id":"` + tt.customerID + `","plan_id":"550e8400-e29b-41d4-a716-446655440001","template_id":"550e8400-e29b-41d4-a716-446655440002","hostname":"vm.example.test","external_service_id":123}`
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/vms", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			assert.NotContains(t, w.Body.String(), `"task_id":""`)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.errCode, errObj["code"])
		})
	}
}

func TestCreateVMRejectsNonPositiveExternalServiceID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ProvisioningHandler{logger: testProvisioningLogger()}
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/vms", handler.CreateVM)

	tests := []struct {
		name              string
		externalServiceID int
		wantCode          int
		errCode           string
	}{
		{name: "zero external service id", externalServiceID: 0, wantCode: http.StatusBadRequest, errCode: "VALIDATION_ERROR"},
		{name: "negative external service id", externalServiceID: -99, wantCode: http.StatusBadRequest, errCode: "VALIDATION_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"customer_id":"550e8400-e29b-41d4-a716-446655440000","plan_id":"550e8400-e29b-41d4-a716-446655440001","template_id":"550e8400-e29b-41d4-a716-446655440002","hostname":"vm.example.test","external_service_id":%d}`, tt.externalServiceID)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/vms", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.errCode, errObj["code"])
		})
	}
}

func TestGetStatusRequiresMatchingProvisioningOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)

	vmID := "550e8400-e29b-41d4-a716-446655440010"
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &provisioningFakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1") {
				row := provisioningVMRow(vmID, customerID, 123)
				row[5] = models.VMStatusStopped
				return &provisioningFakeRow{values: row}
			}
			if strings.Contains(sql, "FROM customers WHERE id = $1") {
				return &provisioningFakeRow{values: provisioningCustomerRow(customerID, 42)}
			}
			return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
		},
	}
	handler := &ProvisioningHandler{
		vmRepo:       repository.NewVMRepository(db),
		customerRepo: repository.NewCustomerRepository(db),
		logger:       testProvisioningLogger(),
	}
	router := gin.New()
	router.GET("/vms/:id/status", handler.GetStatus)

	tests := []struct {
		name     string
		query    string
		wantCode int
		errCode  string
	}{
		{name: "missing external service id", query: "", wantCode: http.StatusNotFound, errCode: "VM_NOT_FOUND"},
		{name: "service id without owner assertion", query: "?external_service_id=123", wantCode: http.StatusBadRequest, errCode: "OWNERSHIP_ASSERTION_REQUIRED"},
		{name: "mismatched external service id", query: "?external_service_id=999&customer_id=" + customerID, wantCode: http.StatusNotFound, errCode: "VM_NOT_FOUND"},
		{name: "matching customer id", query: "?external_service_id=123&customer_id=" + customerID, wantCode: http.StatusOK},
		{name: "matching external client id", query: "?external_service_id=123&external_client_id=42", wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/vms/"+vmID+"/status"+tt.query, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			if tt.errCode == "" {
				return
			}
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.errCode, errObj["code"])
		})
	}
}

func TestDeleteVMSoftDeletedWithoutTaskReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	vmID := "550e8400-e29b-41d4-a716-446655440010"
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &provisioningFakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1") {
				row := provisioningVMRow(vmID, customerID, 123)
				row[5] = models.VMStatusDeleted
				deletedAt := time.Now().UTC()
				row[21] = &deletedAt
				return &provisioningFakeRow{values: row}
			}
			return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
		},
	}
	vmRepo := repository.NewVMRepository(db)
	handler := &ProvisioningHandler{
		vmRepo: vmRepo,
		vmService: services.NewVMService(services.VMServiceConfig{
			VMRepo: vmRepo,
			Logger: testProvisioningLogger(),
		}),
		logger: testProvisioningLogger(),
	}
	router := gin.New()
	router.DELETE("/vms/:id", handler.DeleteVM)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/vms/"+vmID+"?external_service_id=123&customer_id="+customerID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.NotContains(t, w.Body.String(), `"task_id":""`)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VM_DELETE_CONFLICT", errObj["code"])
}

func TestDeleteVMDeletingTransitionRaceReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	vmID := "550e8400-e29b-41d4-a716-446655440010"
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &provisioningFakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1") {
				return &provisioningFakeRow{values: provisioningVMRow(vmID, customerID, 123)}
			}
			return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "WHERE id = $2 AND status = $3 AND deleted_at IS NULL") {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	vmRepo := repository.NewVMRepository(db)
	handler := &ProvisioningHandler{
		vmRepo: vmRepo,
		vmService: services.NewVMService(services.VMServiceConfig{
			VMRepo: vmRepo,
			Logger: testProvisioningLogger(),
		}),
		logger: testProvisioningLogger(),
	}
	router := gin.New()
	router.DELETE("/vms/:id", handler.DeleteVM)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/vms/"+vmID+"?external_service_id=123&customer_id="+customerID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.NotContains(t, w.Body.String(), `"task_id":""`)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VM_DELETE_CONFLICT", errObj["code"])
}
