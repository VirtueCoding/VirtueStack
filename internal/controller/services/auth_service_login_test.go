package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/alexedwards/argon2id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLoginCustomerRepo is a minimal CustomerRepo that tracks failed-login
// counters keyed by whatever string the caller supplies, so the test can
// observe whether record/clear and check operations agree on the same key.
type mockLoginCustomerRepo struct {
	customer              *models.Customer
	sessionByRefreshToken *models.Session
	deleteSessionErr      error
	rotateSessionErr      error
	createdSessionID      string
	deletedSessionIDs     []string
	rotatedOldSessionID   string
	rotatedNewSession     *models.Session
	failedByKey           map[string]int
	clearedByKey          map[string]int
	recordedByKey         map[string]int
	getCountByKey         map[string]int
	lastCheckedKey        string
	lastRecordedKey       string
}

func newMockLoginCustomerRepo(c *models.Customer) *mockLoginCustomerRepo {
	return &mockLoginCustomerRepo{
		customer:      c,
		failedByKey:   map[string]int{},
		clearedByKey:  map[string]int{},
		recordedByKey: map[string]int{},
		getCountByKey: map[string]int{},
	}
}

func (m *mockLoginCustomerRepo) GetByEmail(ctx context.Context, email string) (*models.Customer, error) {
	if m.customer == nil {
		return nil, sharederrors.ErrNotFound
	}
	return m.customer, nil
}

func (m *mockLoginCustomerRepo) GetByID(ctx context.Context, id string) (*models.Customer, error) {
	if m.customer == nil {
		return nil, sharederrors.ErrNotFound
	}
	return m.customer, nil
}

func (m *mockLoginCustomerRepo) Create(ctx context.Context, customer *models.Customer) error {
	return nil
}

func (m *mockLoginCustomerRepo) UpdateProfile(ctx context.Context, customerID string, params repository.ProfileUpdateParams) (*models.Customer, error) {
	return m.customer, nil
}

func (m *mockLoginCustomerRepo) UpdateStatus(ctx context.Context, id, status string) error {
	return nil
}

func (m *mockLoginCustomerRepo) SoftDelete(ctx context.Context, id string) error {
	return nil
}

func (m *mockLoginCustomerRepo) CreateSession(ctx context.Context, session *models.Session) error {
	m.createdSessionID = session.ID
	return nil
}

func (m *mockLoginCustomerRepo) GetSession(ctx context.Context, id string) (*models.Session, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockLoginCustomerRepo) GetSessionByRefreshToken(ctx context.Context, refreshTokenHash string) (*models.Session, error) {
	if m.sessionByRefreshToken != nil && m.sessionByRefreshToken.RefreshTokenHash == refreshTokenHash {
		return m.sessionByRefreshToken, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockLoginCustomerRepo) DeleteSession(ctx context.Context, id string) error {
	m.deletedSessionIDs = append(m.deletedSessionIDs, id)
	return m.deleteSessionErr
}

func (m *mockLoginCustomerRepo) RotateSession(ctx context.Context, oldSessionID string, newSession *models.Session) error {
	m.rotatedOldSessionID = oldSessionID
	m.rotatedNewSession = newSession
	return m.rotateSessionErr
}

func (m *mockLoginCustomerRepo) CountSessionsByUser(ctx context.Context, userID, userType string) (int, error) {
	return 0, nil
}

func (m *mockLoginCustomerRepo) DeleteOldestSession(ctx context.Context, userID, userType string) error {
	return nil
}

func (m *mockLoginCustomerRepo) DeleteExpiredSessions(ctx context.Context) error {
	return nil
}

func (m *mockLoginCustomerRepo) DeleteSessionsByUser(ctx context.Context, userID, userType string) error {
	return nil
}

func (m *mockLoginCustomerRepo) GetSessionLastReauthAt(ctx context.Context, sessionID string) (*time.Time, error) {
	return nil, nil
}

func (m *mockLoginCustomerRepo) UpdateSessionLastReauthAt(ctx context.Context, sessionID string, timestamp time.Time) error {
	return nil
}

func (m *mockLoginCustomerRepo) GetFailedLoginCount(ctx context.Context, key string, window time.Duration) (int, error) {
	m.lastCheckedKey = key
	m.getCountByKey[key]++
	return m.failedByKey[key], nil
}

func (m *mockLoginCustomerRepo) RecordFailedLogin(ctx context.Context, key string) error {
	m.lastRecordedKey = key
	m.failedByKey[key]++
	m.recordedByKey[key]++
	return nil
}

func (m *mockLoginCustomerRepo) ClearFailedLogins(ctx context.Context, key string) error {
	m.failedByKey[key] = 0
	m.clearedByKey[key]++
	return nil
}

func (m *mockLoginCustomerRepo) UpdateCustomerPasswordHash(ctx context.Context, id, passwordHash string) error {
	return nil
}

func (m *mockLoginCustomerRepo) CreatePasswordReset(ctx context.Context, reset *models.PasswordReset) error {
	return nil
}

func (m *mockLoginCustomerRepo) GetPasswordResetByTokenHash(ctx context.Context, tokenHash string) (*models.PasswordReset, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockLoginCustomerRepo) MarkPasswordResetUsed(ctx context.Context, id string) error {
	return nil
}

func (m *mockLoginCustomerRepo) UpdateBackupCodes(ctx context.Context, userID string, codes []string) error {
	return nil
}

func (m *mockLoginCustomerRepo) UpdateBackupCodesShown(ctx context.Context, id string, shown bool) error {
	return nil
}

func (m *mockLoginCustomerRepo) UpdateBackupCodesWithShown(ctx context.Context, id string, backupCodesHash []string) error {
	return nil
}

func (m *mockLoginCustomerRepo) List(ctx context.Context, filter repository.CustomerListFilter) ([]models.Customer, bool, string, error) {
	return nil, false, "", nil
}

func (m *mockLoginCustomerRepo) UpdateExternalClientID(ctx context.Context, id string, externalClientID int) error {
	return nil
}

func (m *mockLoginCustomerRepo) UpdateTOTPEnabled(ctx context.Context, id string, enabled bool, secretEncrypted *string, backupCodesHash []string) error {
	return nil
}

func testLoginLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestCustomerLoginLockout verifies that customer login lockout activates
// once MaxFailedLoginAttempts wrong-password attempts have been recorded.
//
// Regression for GO-1: previously, customer login wrote the failed-login
// counter under the raw email but the lockout check read it under
// "customer:<email>", so the counter never tripped the threshold.
func TestCustomerLoginLockout(t *testing.T) {
	ctx := context.Background()
	logger := testLoginLogger()

	const email = "lockout@example.com"
	const correctPassword = "correct horse battery staple"
	const wrongPassword = "definitely-wrong"

	// Real Argon2id hash so verifyPassword exercises the real comparison
	// path; the password we send is intentionally different.
	hash, err := argon2id.CreateHash(correctPassword, Argon2idParams)
	require.NoError(t, err)

	cases := []struct {
		name           string
		failedAttempts int
		password       string
		wantErr        error
	}{
		{
			name:           "fifth wrong password still allows attempt",
			failedAttempts: MaxFailedLoginAttempts - 1,
			password:       wrongPassword,
			wantErr:        sharederrors.ErrUnauthorized,
		},
		{
			name:           "after threshold any subsequent login is locked",
			failedAttempts: MaxFailedLoginAttempts,
			password:       wrongPassword,
			wantErr:        sharederrors.ErrAccountLocked,
		},
		{
			name:           "after threshold even correct password is locked",
			failedAttempts: MaxFailedLoginAttempts,
			password:       correctPassword,
			wantErr:        sharederrors.ErrAccountLocked,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			customer := &models.Customer{
				ID:           "cust-1",
				Email:        email,
				PasswordHash: &hash,
				Status:       models.CustomerStatusActive,
			}
			mock := newMockLoginCustomerRepo(customer)

			svc := &AuthService{
				customerRepo: mock,
				logger:       logger,
			}

			// Drive the failure counter via the public Login flow so the
			// test exercises the same key-construction path the production
			// code uses (rather than poking the mock map directly).
			for i := 0; i < tc.failedAttempts; i++ {
				_, _, err := svc.Login(ctx, email, wrongPassword, "1.2.3.4", "ua")
				require.ErrorIs(t, err, sharederrors.ErrUnauthorized,
					"failure-priming attempt %d should return ErrUnauthorized", i+1)
			}

			_, _, err := svc.Login(ctx, email, tc.password, "1.2.3.4", "ua")
			require.ErrorIs(t, err, tc.wantErr)

			// Lockout check must read using the "customer:" prefix.
			assert.Equal(t, "customer:"+email, mock.lastCheckedKey,
				"lockout check should key on customer:<email>")
		})
	}
}
