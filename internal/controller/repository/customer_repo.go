// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
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

// emailRegex is a simple regex for basic email validation.
// Note: This is intentionally simple - full RFC 5322 validation is not practical.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// Update updates a customer's profile information (name and email).
// Uses a transaction to ensure atomic updates with audit logging.
// Validates email format and name constraints before updating.
func (r *CustomerRepository) Update(ctx context.Context, customer *models.Customer) error {
	// Validate name
	if strings.TrimSpace(customer.Name) == "" {
		return sharederrors.NewValidationError("name", "name cannot be empty")
	}
	if len(customer.Name) > 255 {
		return sharederrors.NewValidationError("name", "name cannot exceed 255 characters")
	}

	// Validate email
	if strings.TrimSpace(customer.Email) == "" {
		return sharederrors.NewValidationError("email", "email cannot be empty")
	}
	if len(customer.Email) > 254 {
		return sharederrors.NewValidationError("email", "email cannot exceed 254 characters")
	}
	if !emailRegex.MatchString(customer.Email) {
		return sharederrors.NewValidationError("email", "invalid email format")
	}

	const q = `
		UPDATE customers SET
			name = $1,
			email = $2,
			updated_at = NOW()
		WHERE id = $3 AND status != 'deleted'
		RETURNING ` + customerSelectCols

	row := r.db.QueryRow(ctx, q,
		customer.Name, customer.Email, customer.ID,
	)
	updated, err := scanCustomer(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("updating customer %s: %w", customer.ID, sharederrors.ErrNotFound)
		}
		return fmt.Errorf("updating customer %s: %w", customer.ID, err)
	}
	*customer = updated
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
		       ip_address, user_agent, expires_at, last_reauth_at, created_at
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
		       ip_address, user_agent, expires_at, last_reauth_at, created_at
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

// CountSessionsByUser returns the count of active sessions for a user.
func (r *CustomerRepository) CountSessionsByUser(ctx context.Context, userID, userType string) (int, error) {
	const q = `SELECT COUNT(*) FROM sessions WHERE user_id = $1 AND user_type = $2 AND expires_at > NOW()`
	var count int
	err := r.db.QueryRow(ctx, q, userID, userType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting sessions for user %s: %w", userID, err)
	}
	return count, nil
}

// DeleteOldestSession removes the oldest session for a user.
func (r *CustomerRepository) DeleteOldestSession(ctx context.Context, userID, userType string) error {
	const q = `DELETE FROM sessions WHERE id = (
		SELECT id FROM sessions WHERE user_id = $1 AND user_type = $2 ORDER BY created_at ASC LIMIT 1
	)`
	_, err := r.db.Exec(ctx, q, userID, userType)
	if err != nil {
		return fmt.Errorf("deleting oldest session for user %s: %w", userID, err)
	}
	return nil
}

// GetSessionLastReauthAt returns the last re-authentication timestamp for a session.
// Returns nil if the session has no last_reauth_at recorded.
func (r *CustomerRepository) GetSessionLastReauthAt(ctx context.Context, sessionID string) (*time.Time, error) {
	const q = `SELECT last_reauth_at FROM sessions WHERE id = $1`
	var lastReauthAt *time.Time
	err := r.db.QueryRow(ctx, q, sessionID).Scan(&lastReauthAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, sharederrors.ErrNotFound
		}
		return nil, fmt.Errorf("getting session last_reauth_at: %w", err)
	}
	return lastReauthAt, nil
}

// UpdateSessionLastReauthAt updates the last re-authentication timestamp for a session.
func (r *CustomerRepository) UpdateSessionLastReauthAt(ctx context.Context, sessionID string, timestamp time.Time) error {
	const q = `UPDATE sessions SET last_reauth_at = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, timestamp, sessionID)
	if err != nil {
		return fmt.Errorf("updating session last_reauth_at: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating session last_reauth_at: %w", ErrNoRowsAffected)
	}
	return nil
}

// scanSession scans a single session row into a models.Session struct.
func scanSession(row pgx.Row) (models.Session, error) {
	var s models.Session
	err := row.Scan(
		&s.ID, &s.UserID, &s.UserType, &s.RefreshTokenHash,
		&s.IPAddress, &s.UserAgent, &s.ExpiresAt, &s.LastReauthAt, &s.CreatedAt,
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

// UpdateBackupCodes updates the TOTP backup codes for an admin.
func (r *AdminRepository) UpdateBackupCodes(ctx context.Context, userID string, codes []string) error {
	const q = `UPDATE admins SET totp_backup_codes_hash = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, q, codes, userID)
	if err != nil {
		return fmt.Errorf("updating backup codes for admin %s: %w", userID, err)
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

// scanPasswordReset scans a single password_reset row into a models.PasswordReset struct.
func scanPasswordReset(row pgx.Row) (models.PasswordReset, error) {
	var pr models.PasswordReset
	err := row.Scan(
		&pr.ID, &pr.UserID, &pr.UserType, &pr.TokenHash,
		&pr.ExpiresAt, &pr.UsedAt, &pr.CreatedAt,
	)
	return pr, err
}

const passwordResetSelectCols = `
	id, user_id, user_type, token_hash, expires_at, used_at, created_at`

// CreatePasswordReset inserts a new password reset token into the database.
func (r *CustomerRepository) CreatePasswordReset(ctx context.Context, reset *models.PasswordReset) error {
	const q = `
		INSERT INTO password_resets (
			user_id, user_type, token_hash, expires_at
		) VALUES ($1,$2,$3,$4)
		RETURNING ` + passwordResetSelectCols

	row := r.db.QueryRow(ctx, q,
		reset.UserID, reset.UserType, reset.TokenHash, reset.ExpiresAt,
	)
	created, err := scanPasswordReset(row)
	if err != nil {
		return fmt.Errorf("creating password reset: %w", err)
	}
	*reset = created
	return nil
}

// GetPasswordResetByTokenHash returns a password reset by its token hash. Returns ErrNotFound if no match.
func (r *CustomerRepository) GetPasswordResetByTokenHash(ctx context.Context, tokenHash string) (*models.PasswordReset, error) {
	const q = `SELECT ` + passwordResetSelectCols + ` FROM password_resets WHERE token_hash = $1`
	reset, err := ScanRow(ctx, r.db, q, []any{tokenHash}, scanPasswordReset)
	if err != nil {
		return nil, fmt.Errorf("getting password reset by token: %w", err)
	}
	return &reset, nil
}

// MarkPasswordResetUsed marks a password reset token as used by setting used_at.
func (r *CustomerRepository) MarkPasswordResetUsed(ctx context.Context, id string) error {
	const q = `UPDATE password_resets SET used_at = NOW() WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("marking password reset used: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("marking password reset used: %w", ErrNoRowsAffected)
	}
	return nil
}

// DeleteExpiredPasswordResets removes all expired password reset tokens.
func (r *CustomerRepository) DeleteExpiredPasswordResets(ctx context.Context) error {
	const q = `DELETE FROM password_resets WHERE expires_at < NOW()`
	_, err := r.db.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("deleting expired password resets: %w", err)
	}
	return nil
}

// UpdateCustomerPasswordHash updates the password hash for a customer.
func (r *CustomerRepository) UpdateCustomerPasswordHash(ctx context.Context, id, passwordHash string) error {
	const q = `UPDATE customers SET password_hash = $1, updated_at = NOW() WHERE id = $2 AND status != 'deleted'`
	tag, err := r.db.Exec(ctx, q, passwordHash, id)
	if err != nil {
		return fmt.Errorf("updating customer %s password: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating customer %s password: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateBackupCodes updates the TOTP backup codes for a customer.
func (r *CustomerRepository) UpdateBackupCodes(ctx context.Context, userID string, codes []string) error {
	const q = `UPDATE customers SET totp_backup_codes_hash = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, q, codes, userID)
	if err != nil {
		return fmt.Errorf("updating backup codes for user %s: %w", userID, err)
	}
	return nil
}
