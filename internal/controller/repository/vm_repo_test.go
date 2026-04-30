package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDB struct {
	execFunc     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return nil
}

func (m *mockDB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, arguments...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockDB) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, nil
}

type errorRow struct {
	err error
}

func (r errorRow) Scan(...any) error {
	return r.err
}

func TestVMRepository_CreateDuplicateExternalServiceReturnsConflict(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		wantErrIs  error
	}{
		{
			name:       "active external service uniqueness violation",
			constraint: "idx_vms_external_service_id_active_unique",
			wantErrIs:  sharederrors.ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewVMRepository(&mockDB{
				queryRowFunc: func(context.Context, string, ...any) pgx.Row {
					return errorRow{err: &pgconn.PgError{
						Code:           "23505",
						ConstraintName: tt.constraint,
					}}
				},
			})
			externalServiceID := 123

			err := repo.Create(context.Background(), &models.VM{
				CustomerID:         "customer-1",
				Hostname:           "vm-1",
				Status:             models.VMStatusProvisioning,
				ExternalServiceID:  &externalServiceID,
				StorageBackend:     models.StorageBackendCeph,
				PortSpeedMbps:      1000,
				BandwidthLimitGB:   1000,
				BandwidthUsedBytes: 0,
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErrIs)
		})
	}
}

func TestVMRepository_ListByCustomerBuildsScanableSelect(t *testing.T) {
	t.Parallel()

	var gotSQL string
	mock := &mockDB{
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			gotSQL = sql
			return &emptyRows{}, nil
		},
	}

	repo := NewVMRepository(mock)
	_, _, _, err := repo.ListByCustomer(context.Background(), "88888888-8888-8888-8888-888888888001", models.PaginationParams{
		PerPage: 20,
	})

	require.NoError(t, err)
	require.NotEmpty(t, gotSQL)
	assert.Regexp(t, regexp.MustCompile(`(?is)^select\s+id,\s*customer_id`), gotSQL)
	assert.NotRegexp(t, regexp.MustCompile(`(?is)^select\s+1\s*,`), gotSQL)
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
			encryptedPassword: "encrypted-secret-placeholder",
			rowsAffected:      1,
			execErr:           nil,
			wantErr:           false,
		},
		{
			name:              "vm not found",
			vmID:              "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "encrypted-secret-placeholder",
			rowsAffected:      0,
			execErr:           nil,
			wantErr:           true,
			errContains:       "no rows affected",
		},
		{
			name:              "database error",
			vmID:              "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "encrypted-secret-placeholder",
			rowsAffected:      0,
			execErr:           errors.New("connection refused"),
			wantErr:           true,
			errContains:       "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDB{
				execFunc: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
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
				execFunc: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
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
				execFunc: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
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
