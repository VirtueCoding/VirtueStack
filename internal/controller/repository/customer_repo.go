// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// CustomerRepository provides database operations for customer accounts.
type CustomerRepository struct {
	db DB
}

// NewCustomerRepository creates a new CustomerRepository with the given database connection.
func NewCustomerRepository(db DB) *CustomerRepository {
	return &CustomerRepository{db: db}
}

// scanCustomer scans a single customer row into a models.Customer struct.
func scanCustomer(row pgx.Row) (models.Customer, error) {
	var c models.Customer
	err := row.Scan(
		&c.ID, &c.Email, &c.PasswordHash, &c.Name,
		&c.WHMCSClientID, &c.TOTPSecretEncrypted, &c.TOTPEnabled,
		&c.TOTPBackupCodesHash, &c.Status,
		&c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

const customerSelectCols = `
	id, email, password_hash, name,
	whmcs_client_id, totp_secret_encrypted, totp_enabled,
	totp_backup_codes_hash, status,
	created_at, updated_at`

// Create inserts a new customer record into the database.
// The customer's ID, CreatedAt, and UpdatedAt are populated by the database.
func (r *CustomerRepository) Create(ctx context.Context, customer *models.Customer) error {
	const q = `
		INSERT INTO customers (
			email, password_hash, name, whmcs_client_id,
			totp_secret_encrypted, totp_enabled, totp_backup_codes_hash, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING ` + customerSelectCols

	row := r.db.QueryRow(ctx, q,
		customer.Email, customer.PasswordHash, customer.Name, customer.WHMCSClientID,
		customer.TOTPSecretEncrypted, customer.TOTPEnabled, customer.TOTPBackupCodesHash, customer.Status,
	)
	created, err := scanCustomer(row)
	if err != nil {
		return fmt.Errorf("creating customer: %w", err)
	}
	*customer = created
	return nil
}

// GetByID returns a customer by their UUID. Returns ErrNotFound if no customer matches.
func (r *CustomerRepository) GetByID(ctx context.Context, id string) (*models.Customer, error) {
	const q = `SELECT ` + customerSelectCols + ` FROM customers WHERE id = $1 AND status != 'deleted'`
	customer, err := ScanRow(ctx, r.db, q, []any{id}, scanCustomer)
	if err != nil {
		return nil, fmt.Errorf("getting customer %s: %w", id, err)
	}
	return &customer, nil
}

// GetByEmail returns a customer by their email address. Returns ErrNotFound if no customer matches.
func (r *CustomerRepository) GetByEmail(ctx context.Context, email string) (*models.Customer, error) {
	const q = `SELECT ` + customerSelectCols + ` FROM customers WHERE email = $1 AND status != 'deleted'`
	customer, err := ScanRow(ctx, r.db, q, []any{email}, scanCustomer)
	if err != nil {
		return nil, fmt.Errorf("getting customer by email %s: %w", email, err)
	}
	return &customer, nil
}

// List returns a paginated list of customers with optional filters and total count.
func (r *CustomerRepository) List(ctx context.Context, filter CustomerListFilter) ([]models.Customer, int, error) {
	where := []string{"status != 'deleted'"}
	args := []any{}
	idx := 1

	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}
	if filter.Search != nil {
		where = append(where, fmt.Sprintf("(email ILIKE $%d OR name ILIKE $%d)", idx, idx))
		args = append(args, "%"+*filter.Search+"%")
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM customers WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting customers: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM customers WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		customerSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	customers, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Customer, error) {
		return scanCustomer(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing customers: %w", err)
	}
	return customers, total, nil
}

// UpdateStatus updates the status field of a customer.
func (r *CustomerRepository) UpdateStatus(ctx context.Context, id, status string) error {
	const q = `UPDATE customers SET status = $1, updated_at = NOW() WHERE id = $2 AND status != 'deleted'`
	tag, err := r.db.Exec(ctx, q, status, id)
	if err != nil {
		return fmt.Errorf("updating customer %s status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating customer %s status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateWHMCSClientID updates the WHMCS client ID for a customer.
func (r *CustomerRepository) UpdateWHMCSClientID(ctx context.Context, id string, whmcsClientID int) error {
	const q = `UPDATE customers SET whmcs_client_id = $1, updated_at = NOW() WHERE id = $2 AND status != 'deleted'`
	tag, err := r.db.Exec(ctx, q, whmcsClientID, id)
	if err != nil {
		return fmt.Errorf("updating customer %s WHMCS client ID: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating customer %s WHMCS client ID: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SoftDelete marks a customer as deleted by setting status to "deleted".
func (r *CustomerRepository) SoftDelete(ctx context.Context, id string) error {
	const q = `UPDATE customers SET status = 'deleted', updated_at = NOW() WHERE id = $1 AND status != 'deleted'`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("soft-deleting customer %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("soft-deleting customer %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// CreateSession inserts a new session record into the database.
func (r *CustomerRepository) CreateSession(ctx context.Context, session *models.Session) error {
	const q = `
		INSERT INTO sessions (
			id, user_id, user_type, refresh_token_hash,
			ip_address, user_agent, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING created_at`

	row := r.db.QueryRow(ctx, q,
		session.ID, session.UserID, session.UserType, session.RefreshTokenHash,
		session.IPAddress, session.UserAgent, session.ExpiresAt,
	)
	if err := row.Scan(&session.CreatedAt); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

// GetSession returns a session by its UUID. Returns ErrNotFound if no session matches.
func (r *CustomerRepository) GetSession(ctx context.Context, id string) (*models.Session, error) {
	const q = `
		SELECT id, user_id, user_type, refresh_token_hash,
		       ip_address, user_agent, expires_at, created_at
		FROM sessions WHERE id = $1`
	session, err := ScanRow(ctx, r.db, q, []any{id}, scanSession)
	if err != nil {
		return nil, fmt.Errorf("getting session %s: %w", id, err)
	}
	return &session, nil
}

// GetSessionByRefreshToken returns a session by its refresh token hash. Returns ErrNotFound if no session matches.
func (r *CustomerRepository) GetSessionByRefreshToken(ctx context.Context, refreshTokenHash string) (*models.Session, error) {
	const q = `
		SELECT id, user_id, user_type, refresh_token_hash,
		       ip_address, user_agent, expires_at, created_at
		FROM sessions WHERE refresh_token_hash = $1`
	session, err := ScanRow(ctx, r.db, q, []any{refreshTokenHash}, scanSession)
	if err != nil {
		return nil, fmt.Errorf("getting session by refresh token: %w", err)
	}
	return &session, nil
}

// DeleteSession removes a session from the database.
func (r *CustomerRepository) DeleteSession(ctx context.Context, id string) error {
	const q = `DELETE FROM sessions WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting session %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting session %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// DeleteExpiredSessions removes all sessions that have expired.
func (r *CustomerRepository) DeleteExpiredSessions(ctx context.Context) error {
	const q = `DELETE FROM sessions WHERE expires_at < NOW()`
	_, err := r.db.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("deleting expired sessions: %w", err)
	}
	return nil
}

// DeleteSessionsByUser removes all sessions for a specific user (logout all devices).
func (r *CustomerRepository) DeleteSessionsByUser(ctx context.Context, userID, userType string) error {
	const q = `DELETE FROM sessions WHERE user_id = $1 AND user_type = $2`
	_, err := r.db.Exec(ctx, q, userID, userType)
	if err != nil {
		return fmt.Errorf("deleting sessions for user %s: %w", userID, err)
	}
	return nil
}

// scanSession scans a single session row into a models.Session struct.
func scanSession(row pgx.Row) (models.Session, error) {
	var s models.Session
	err := row.Scan(
		&s.ID, &s.UserID, &s.UserType, &s.RefreshTokenHash,
		&s.IPAddress, &s.UserAgent, &s.ExpiresAt, &s.CreatedAt,
	)
	return s, err
}

// AdminRepository provides database operations for administrative user accounts.
type AdminRepository struct {
	db DB
}

// NewAdminRepository creates a new AdminRepository with the given database connection.
func NewAdminRepository(db DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// scanAdmin scans a single admin row into a models.Admin struct.
func scanAdmin(row pgx.Row) (models.Admin, error) {
	var a models.Admin
	err := row.Scan(
		&a.ID, &a.Email, &a.PasswordHash, &a.Name,
		&a.TOTPSecretEncrypted, &a.TOTPEnabled, &a.TOTPBackupCodesHash,
		&a.Role, &a.MaxSessions, &a.CreatedAt,
	)
	return a, err
}

const adminSelectCols = `
	id, email, password_hash, name,
	totp_secret_encrypted, totp_enabled, totp_backup_codes_hash,
	role, max_sessions, created_at`

// Create inserts a new admin record into the database.
func (r *AdminRepository) Create(ctx context.Context, admin *models.Admin) error {
	const q = `
		INSERT INTO admins (
			email, password_hash, name,
			totp_secret_encrypted, totp_enabled, totp_backup_codes_hash,
			role, max_sessions
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING ` + adminSelectCols

	row := r.db.QueryRow(ctx, q,
		admin.Email, admin.PasswordHash, admin.Name,
		admin.TOTPSecretEncrypted, admin.TOTPEnabled, admin.TOTPBackupCodesHash,
		admin.Role, admin.MaxSessions,
	)
	created, err := scanAdmin(row)
	if err != nil {
		return fmt.Errorf("creating admin: %w", err)
	}
	*admin = created
	return nil
}

// GetByID returns an admin by their UUID. Returns ErrNotFound if no admin matches.
func (r *AdminRepository) GetByID(ctx context.Context, id string) (*models.Admin, error) {
	const q = `SELECT ` + adminSelectCols + ` FROM admins WHERE id = $1`
	admin, err := ScanRow(ctx, r.db, q, []any{id}, scanAdmin)
	if err != nil {
		return nil, fmt.Errorf("getting admin %s: %w", id, err)
	}
	return &admin, nil
}

// GetByEmail returns an admin by their email address. Returns ErrNotFound if no admin matches.
func (r *AdminRepository) GetByEmail(ctx context.Context, email string) (*models.Admin, error) {
	const q = `SELECT ` + adminSelectCols + ` FROM admins WHERE email = $1`
	admin, err := ScanRow(ctx, r.db, q, []any{email}, scanAdmin)
	if err != nil {
		return nil, fmt.Errorf("getting admin by email %s: %w", email, err)
	}
	return &admin, nil
}

// List returns a paginated list of admins with optional filters and total count.
func (r *AdminRepository) List(ctx context.Context, filter AdminListFilter) ([]models.Admin, int, error) {
	where := "1=1"
	args := []any{}
	idx := 1

	if filter.Role != nil {
		where += fmt.Sprintf(" AND role = $%d", idx)
		args = append(args, *filter.Role)
		idx++
	}

	total, err := CountRows(ctx, r.db, "SELECT COUNT(*) FROM admins WHERE "+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting admins: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM admins WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		adminSelectCols, where, idx, idx+1,
	)
	args = append(args, limit, offset)

	admins, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Admin, error) {
		return scanAdmin(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing admins: %w", err)
	}
	return admins, total, nil
}

// UpdateTOTPEnabled updates the TOTP configuration for an admin.
func (r *AdminRepository) UpdateTOTPEnabled(ctx context.Context, id string, enabled bool, secretEncrypted string, backupCodesHash []string) error {
	const q = `
		UPDATE admins SET
			totp_enabled = $1,
			totp_secret_encrypted = $2,
			totp_backup_codes_hash = $3
		WHERE id = $4`
	tag, err := r.db.Exec(ctx, q, enabled, secretEncrypted, backupCodesHash, id)
	if err != nil {
		return fmt.Errorf("updating admin %s TOTP settings: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating admin %s TOTP settings: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdatePasswordHash updates the password hash for an admin.
func (r *AdminRepository) UpdatePasswordHash(ctx context.Context, id, passwordHash string) error {
	const q = `UPDATE admins SET password_hash = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, passwordHash, id)
	if err != nil {
		return fmt.Errorf("updating admin %s password: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating admin %s password: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// CustomerListFilter holds query parameters for filtering and paginating customer list results.
type CustomerListFilter struct {
	Status *string `form:"status"`
	Search *string `form:"search"` // email or name search
	models.PaginationParams
}

// AdminListFilter holds query parameters for filtering and paginating admin list results.
type AdminListFilter struct {
	Role *string `form:"role"`
	models.PaginationParams
}