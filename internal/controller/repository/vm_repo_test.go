package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return mockVMRow{scanErr: pgx.ErrNoRows}
}

func (m *mockDB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, arguments...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, nil
}

type mockCommandTag struct {
	rowsAffected int64
}

func (m mockCommandTag) RowsAffected() int64 {
	return m.rowsAffected
}

func (m mockCommandTag) String() string {
	return ""
}

type mockVMRow struct {
	values  []any
	scanErr error
}

func (m mockVMRow) Scan(dest ...any) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	if len(dest) != len(m.values) {
		return fmt.Errorf("scan destination count mismatch: got %d want %d", len(dest), len(m.values))
	}
	for i := range dest {
		if err := assignVMTestValue(dest[i], m.values[i]); err != nil {
			return err
		}
	}
	return nil
}

func assignVMTestValue(dest any, val any) error {
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

func fullVMRow(storageBackendID *string) []any {
	now := time.Now().UTC()
	nodeID := "node-1"
	diskPath := "/var/lib/virtuestack/vms/vm-1-disk0.qcow2"
	return []any{
		"vm-1", "customer-1", &nodeID, "plan-1",
		"vm-host", models.VMStatusRunning, 2, 2048,
		40, 1000, 1000,
		int64(0), now,
		"52:54:00:12:34:56", (*string)(nil), (*string)(nil),
		(*string)(nil), (*int)(nil), (*string)(nil),
		now, now, (*time.Time)(nil),
		models.StorageBackendQcow, &diskPath, (*string)(nil), (*string)(nil), storageBackendID,
	}
}

func TestVMRepository_UpdatePassword(t *testing.T) {
	tests := []struct {
		name              string
		vmID              string
		encryptedPassword string
		rowsAffected      int64
		execErr           error
		wantErr           bool
		errContains       string
	}{
		{
			name:              "successful update",
			vmID:              "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "$argon2id$v=19$m=65536,t=3,p=4$...",
			rowsAffected:      1,
			execErr:           nil,
			wantErr:           false,
		},
		{
			name:              "vm not found",
			vmID:              "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "$argon2id$v=19$m=65536,t=3,p=4$...",
			rowsAffected:      0,
			execErr:           nil,
			wantErr:           true,
			errContains:       "no rows affected",
		},
		{
			name:              "database error",
			vmID:              "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "$argon2id$v=19$m=65536,t=3,p=4$...",
			rowsAffected:      0,
			execErr:           errors.New("connection refused"),
			wantErr:           true,
			errContains:       "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewVMRepository(mock)
			err := repo.UpdatePassword(context.Background(), tt.vmID, tt.encryptedPassword)

			if tt.wantErr {
				if err == nil {
					t.Errorf("UpdatePassword() expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("UpdatePassword() error = %v, should contain %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("UpdatePassword() unexpected error = %v", err)
			}
		})
	}
}

func TestVMRepository_UpdateMACAddress(t *testing.T) {
	tests := []struct {
		name         string
		vmID         string
		macAddress   string
		rowsAffected int64
		execErr      error
		wantErr      bool
		errContains  string
	}{
		{
			name:         "successful update",
			vmID:         "550e8400-e29b-41d4-a716-446655440000",
			macAddress:   "52:54:00:ab:cd:ef",
			rowsAffected: 1,
			execErr:      nil,
			wantErr:      false,
		},
		{
			name:         "vm not found",
			vmID:         "550e8400-e29b-41d4-a716-446655440000",
			macAddress:   "52:54:00:ab:cd:ef",
			rowsAffected: 0,
			execErr:      nil,
			wantErr:      true,
			errContains:  "no rows affected",
		},
		{
			name:         "database error",
			vmID:         "550e8400-e29b-41d4-a716-446655440000",
			macAddress:   "52:54:00:ab:cd:ef",
			rowsAffected: 0,
			execErr:      errors.New("connection refused"),
			wantErr:      true,
			errContains:  "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewVMRepository(mock)
			err := repo.UpdateMACAddress(context.Background(), tt.vmID, tt.macAddress)

			if tt.wantErr {
				if err == nil {
					t.Errorf("UpdateMACAddress() expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("UpdateMACAddress() error = %v, should contain %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("UpdateMACAddress() unexpected error = %v", err)
			}
		})
	}
}

func TestVMRepository_TransitionStatus(t *testing.T) {
	tests := []struct {
		name         string
		vmID         string
		fromStatus   string
		toStatus     string
		rowsAffected int64
		execErr      error
		wantErr      bool
		errContains  string
		errIs        error
	}{
		{
			name:         "successful transition",
			vmID:         "550e8400-e29b-41d4-a716-446655440000",
			fromStatus:   models.VMStatusRunning,
			toStatus:     models.VMStatusStopped,
			rowsAffected: 1,
		},
		{
			name:         "wrong current status returns conflict",
			vmID:         "550e8400-e29b-41d4-a716-446655440000",
			fromStatus:   models.VMStatusRunning,
			toStatus:     models.VMStatusStopped,
			rowsAffected: 0,
			wantErr:      true,
			errContains:  "not in expected state",
			errIs:        sharederrors.ErrConflict,
		},
		{
			name:        "invalid transition rejected before query",
			vmID:        "550e8400-e29b-41d4-a716-446655440000",
			fromStatus:  models.VMStatusDeleted,
			toStatus:    models.VMStatusRunning,
			wantErr:     true,
			errContains: "unknown VM source status",
			errIs:       sharederrors.ErrConflict,
		},
		{
			name:        "database error is wrapped",
			vmID:        "550e8400-e29b-41d4-a716-446655440000",
			fromStatus:  models.VMStatusRunning,
			toStatus:    models.VMStatusStopped,
			execErr:     errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCalled := false
			mock := &mockDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					execCalled = true
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewVMRepository(mock)
			err := repo.TransitionStatus(context.Background(), tt.vmID, tt.fromStatus, tt.toStatus)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs))
				}
				if tt.fromStatus == models.VMStatusDeleted {
					assert.False(t, execCalled)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestVMRepository_GetByIDScansStorageBackendID(t *testing.T) {
	t.Parallel()

	backendID := "backend-1"
	mock := &mockDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			assert.Contains(t, sql, "storage_backend_id")
			require.Equal(t, []any{"vm-1"}, args)
			return mockVMRow{values: fullVMRow(&backendID)}
		},
	}

	repo := NewVMRepository(mock)
	vm, err := repo.GetByID(context.Background(), "vm-1")
	require.NoError(t, err)
	require.NotNil(t, vm.StorageBackendID)
	assert.Equal(t, backendID, *vm.StorageBackendID)
}

func TestVMRepository_CreatePersistsStorageBackendID(t *testing.T) {
	t.Parallel()

	nodeID := "node-1"
	backendID := "backend-1"
	diskPath := "/var/lib/virtuestack/vms/vm-1-disk0.qcow2"
	vm := &models.VM{
		CustomerID:       "customer-1",
		NodeID:           &nodeID,
		PlanID:           "plan-1",
		Hostname:         "vm-host",
		Status:           models.VMStatusProvisioning,
		VCPU:             2,
		MemoryMB:         2048,
		DiskGB:           40,
		PortSpeedMbps:    1000,
		BandwidthLimitGB: 1000,
		MACAddress:       "52:54:00:12:34:56",
		StorageBackend:   models.StorageBackendQcow,
		StorageBackendID: &backendID,
		DiskPath:         &diskPath,
	}

	mock := &mockDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			assert.Contains(t, sql, "storage_backend_id")
			assert.Contains(t, args, any(&backendID))
			return mockVMRow{values: fullVMRow(&backendID)}
		},
	}

	repo := NewVMRepository(mock)
	err := repo.Create(context.Background(), vm)
	require.NoError(t, err)
	require.NotNil(t, vm.StorageBackendID)
	assert.Equal(t, backendID, *vm.StorageBackendID)
}

func TestVMRepository_UpdatePlacement(t *testing.T) {
	tests := []struct {
		name             string
		storageBackendID *string
		diskPath         *string
		rowsAffected     int64
		execErr          error
		wantErr          bool
		errContains      string
	}{
		{
			name:             "updates placement metadata",
			storageBackendID: ptrTo("backend-2"),
			diskPath:         ptrTo("/srv/target/vm-1-disk0.qcow2"),
			rowsAffected:     1,
		},
		{
			name:             "clears disk path when nil",
			storageBackendID: ptrTo("backend-3"),
			diskPath:         nil,
			rowsAffected:     1,
		},
		{
			name:             "vm not found",
			storageBackendID: ptrTo("backend-4"),
			diskPath:         ptrTo("/srv/target/vm-1-disk0.qcow2"),
			rowsAffected:     0,
			wantErr:          true,
			errContains:      "no rows affected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					assert.Contains(t, sql, "storage_backend_id")
					assert.Contains(t, sql, "disk_path")
					require.Len(t, arguments, 4)
					assert.Equal(t, "node-2", arguments[0])
					if tt.storageBackendID == nil {
						assert.Nil(t, arguments[1])
					} else {
						assert.Equal(t, *tt.storageBackendID, arguments[1])
					}
					if tt.diskPath == nil {
						assert.Nil(t, arguments[2])
					} else {
						assert.Equal(t, *tt.diskPath, arguments[2])
					}
					assert.Equal(t, "vm-1", arguments[3])

					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewVMRepository(mock)
			err := repo.UpdatePlacement(context.Background(), "vm-1", "node-2", tt.storageBackendID, tt.diskPath)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func ptrTo[T any](v T) *T {
	return &v
}
