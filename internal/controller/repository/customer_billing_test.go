package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerBillingProviderConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"WHMCS", models.BillingProviderWHMCS, "whmcs"},
		{"Native", models.BillingProviderNative, "native"},
		{"Blesta", models.BillingProviderBlesta, "blesta"},
		{"Unmanaged", models.BillingProviderUnmanaged, "unmanaged"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

func TestCustomerRepository_UpdateBillingProvider(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		provider     string
		rowsAffected int64
		execErr      error
		wantErr      bool
		errContains  string
	}{
		{
			name:         "successful update to whmcs",
			id:           "550e8400-e29b-41d4-a716-446655440000",
			provider:     models.BillingProviderWHMCS,
			rowsAffected: 1,
			wantErr:      false,
		},
		{
			name:         "successful update to native",
			id:           "550e8400-e29b-41d4-a716-446655440000",
			provider:     models.BillingProviderNative,
			rowsAffected: 1,
			wantErr:      false,
		},
		{
			name:         "customer not found",
			id:           "550e8400-e29b-41d4-a716-446655440001",
			provider:     models.BillingProviderWHMCS,
			rowsAffected: 0,
			wantErr:      true,
			errContains:  "no rows affected",
		},
		{
			name:        "database error",
			id:          "550e8400-e29b-41d4-a716-446655440000",
			provider:    models.BillingProviderWHMCS,
			execErr:     errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCustomerDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewCustomerRepository(mock)
			err := repo.UpdateBillingProvider(context.Background(), tt.id, tt.provider)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
