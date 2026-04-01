package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockCustomerDB implements the DB interface for testing.
type mockCustomerDB struct {
	execFunc     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockCustomerDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockCustomerDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return nil
}

func (m *mockCustomerDB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, arguments...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockCustomerDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return &mockTx{mockCustomerDB: m}, nil
}

// mockTx implements pgx.Tx for testing transactions.
type mockTx struct {
	*mockCustomerDB
}

func (m *mockTx) Commit(ctx context.Context) error {
	return nil
}

func (m *mockTx) Rollback(ctx context.Context) error {
	return nil
}

func (m *mockTx) Conn() *pgx.Conn {
	return nil
}


func (m *mockTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (m *mockTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}

func (m *mockTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *mockTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (m *mockTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return m, nil
}

func (m *mockTx) ExecParams(ctx context.Context, sql string, arguments []any, oid uint32, paramOIDs []uint32) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

// mockCustomerRow implements pgx.Row for testing QueryRow results.
type mockCustomerRow struct {
	customer models.Customer
	err      error
}

func (m mockCustomerRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	// The actual scanCustomer function scans into 14 fields:
	// id, email, password_hash, name, phone, external_client_id, billing_provider,
	// auth_provider, totp_secret_encrypted, totp_enabled, totp_backup_codes_hash,
	// totp_backup_codes_shown, status, created_at, updated_at
	// Note: phone, external_client_id, totp_secret_encrypted are pointers to pointers (**string, **int)
	c := m.customer
	if len(dest) >= 15 {
		if id, ok := dest[0].(*string); ok {
			*id = c.ID
		}
		if email, ok := dest[1].(*string); ok {
			*email = c.Email
		}
		if pw, ok := dest[2].(**string); ok {
			*pw = c.PasswordHash
		}
		if name, ok := dest[3].(*string); ok {
			*name = c.Name
		}
		// Phone is **string - scan into the pointer
		if phone, ok := dest[4].(**string); ok {
			*phone = c.Phone
		}
		if whmcsID, ok := dest[5].(**int); ok {
			*whmcsID = c.ExternalClientID
		}
		if bp, ok := dest[6].(**string); ok {
			*bp = c.BillingProvider
		}
		if ap, ok := dest[7].(*string); ok {
			*ap = c.AuthProvider
		}
		if totpSecret, ok := dest[8].(**string); ok {
			*totpSecret = c.TOTPSecretEncrypted
		}
		if totpEnabled, ok := dest[9].(*bool); ok {
			*totpEnabled = c.TOTPEnabled
		}
		if totpCodes, ok := dest[10].(*[]string); ok {
			*totpCodes = c.TOTPBackupCodesHash
		}
		if totpShown, ok := dest[11].(*bool); ok {
			*totpShown = c.TOTPBackupCodesShown
		}
		if status, ok := dest[12].(*string); ok {
			*status = c.Status
		}
		if createdAt, ok := dest[13].(*time.Time); ok {
			*createdAt = c.CreatedAt
		}
		if updatedAt, ok := dest[14].(*time.Time); ok {
			*updatedAt = c.UpdatedAt
		}
	}
	return nil
}

// TestCustomerUpdate tests the CustomerRepository.Update method.
func TestCustomerUpdate(t *testing.T) {
	now := time.Now()
	validCustomer := models.Customer{
		ID:     "550e8400-e29b-41d4-a716-446655440000",
		Email:  "test@example.com",
		Name:   "Test User",
		Status: "active",
		Timestamps: models.Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	tests := []struct {
		name         string
		customer     *models.Customer
		rowsAffected int64
		queryRowErr  error
		wantErr      bool
		errContains  string
	}{
		{
			name: "successful update",
			customer: &models.Customer{
				ID:    validCustomer.ID,
				Email: "newemail@example.com",
				Name:  "New Name",
			},
			rowsAffected: 1,
			queryRowErr:  nil,
			wantErr:      false,
		},
		{
			name: "validation error - empty name",
			customer: &models.Customer{
				ID:    validCustomer.ID,
				Email: "test@example.com",
				Name:  "",
			},
			wantErr:     true,
			errContains: "name cannot be empty",
		},
		{
			name: "validation error - name too long",
			customer: &models.Customer{
				ID:    validCustomer.ID,
				Email: "test@example.com",
				Name:  string(make([]byte, 256)), // 256 chars > 255
			},
			wantErr:     true,
			errContains: "name cannot exceed 255",
		},
		{
			name: "validation error - empty email",
			customer: &models.Customer{
				ID:    validCustomer.ID,
				Email: "",
				Name:  "Test",
			},
			wantErr:     true,
			errContains: "email cannot be empty",
		},
		{
			name: "validation error - email too long",
			customer: &models.Customer{
				ID:    validCustomer.ID,
				Email: string(make([]byte, 255)) + "@example.com", // > 254 chars
				Name:  "Test",
			},
			wantErr:     true,
			errContains: "email cannot exceed 254",
		},
		{
			name: "validation error - invalid email format",
			customer: &models.Customer{
				ID:    validCustomer.ID,
				Email: "not-an-email",
				Name:  "Test",
			},
			wantErr:     true,
			errContains: "invalid email format",
		},
		{
			name: "customer not found",
			customer: &models.Customer{
				ID:    "550e8400-e29b-41d4-a716-446655440001",
				Email: "test@example.com",
				Name:  "Test User",
			},
			queryRowErr: pgx.ErrNoRows,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name: "database error",
			customer: &models.Customer{
				ID:    validCustomer.ID,
				Email: "test@example.com",
				Name:  "Test User",
			},
			queryRowErr: errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCustomerDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tt.queryRowErr != nil {
						return mockCustomerRow{err: tt.queryRowErr}
					}
					// Return the updated customer with fields from the input
					updated := validCustomer
					if tt.customer != nil {
						updated.ID = tt.customer.ID
						if tt.customer.Name != "" {
							updated.Name = tt.customer.Name
						}
						if tt.customer.Email != "" {
							updated.Email = tt.customer.Email
						}
						if tt.customer.Phone != nil {
							updated.Phone = tt.customer.Phone
						}
					}
					updated.UpdatedAt = time.Now()
					return mockCustomerRow{customer: updated}
				},
			}

			repo := NewCustomerRepository(mock)
			err := repo.Update(context.Background(), tt.customer)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}


func TestCustomerUpdateProfile(t *testing.T) {
	now := time.Now()
	existingCustomer := models.Customer{
		ID:     "550e8400-e29b-41d4-a716-446655440000",
		Email:  "test@example.com",
		Name:   "Test User",
		Status: "active",
		Timestamps: models.Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	tests := []struct {
		name        string
		params      ProfileUpdateParams
		wantErr     bool
		errContains string
	}{
		{
			name: "update name only",
			params: ProfileUpdateParams{
				Name: ptr("New Name"),
			},
			wantErr: false,
		},
		{
			name: "update email only",
			params: ProfileUpdateParams{
				Email: ptr("newemail@example.com"),
			},
			wantErr: false,
		},
		{
			name: "update phone only",
			params: ProfileUpdateParams{
				Phone: ptr("+1234567890"),
			},
			wantErr: false,
		},
		{
			name: "update all fields",
			params: ProfileUpdateParams{
				Name:  ptr("New Name"),
				Email: ptr("new@example.com"),
				Phone: ptr("+1234567890"),
			},
			wantErr: false,
		},
		{
			name: "name too long",
			params: ProfileUpdateParams{
				Name: ptr(string(make([]byte, 101))),
			},
			wantErr:     true,
			errContains: "name cannot exceed 100",
		},
		{
			name: "invalid email format",
			params: ProfileUpdateParams{
				Email: ptr("not-an-email"),
			},
			wantErr:     true,
			errContains: "invalid email format",
		},
		{
			name: "phone too long",
			params: ProfileUpdateParams{
				Phone: ptr(string(make([]byte, 21))),
			},
			wantErr:     true,
			errContains: "phone cannot exceed 20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCustomerDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					updated := existingCustomer
					updated.UpdatedAt = time.Now()
					if tt.params.Phone != nil {
						updated.Phone = tt.params.Phone
					}
					return mockCustomerRow{customer: updated}
				},
			}

			repo := NewCustomerRepository(mock)
			result, err := repo.UpdateProfile(context.Background(), existingCustomer.ID, tt.params)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("expected result, got nil")
				}
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}
