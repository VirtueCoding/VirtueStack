package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type mockDB struct {
	execFunc func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
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

func TestVMRepository_UpdatePassword(t *testing.T) {
	tests := []struct {
		name             string
		vmID             string
		encryptedPassword string
		rowsAffected     int64
		execErr          error
		wantErr          bool
		errContains      string
	}{
		{
			name:             "successful update",
			vmID:             "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "$argon2id$v=19$m=65536,t=3,p=4$...",
			rowsAffected:     1,
			execErr:          nil,
			wantErr:          false,
		},
		{
			name:             "vm not found",
			vmID:             "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "$argon2id$v=19$m=65536,t=3,p=4$...",
			rowsAffected:     0,
			execErr:          nil,
			wantErr:          true,
			errContains:      "no rows affected",
		},
		{
			name:             "database error",
			vmID:             "550e8400-e29b-41d4-a716-446655440000",
			encryptedPassword: "$argon2id$v=19$m=65536,t=3,p=4$...",
			rowsAffected:     0,
			execErr:          errors.New("connection refused"),
			wantErr:          true,
			errContains:      "connection refused",
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

