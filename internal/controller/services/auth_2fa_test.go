package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mock2FACustomerRepo struct {
	customer         *models.Customer
	session          *models.Session
	updateErr        error
	getErr           error
	getSessionErr    error
	deleteSessionErr error
	updateCalls      int
	createdSession   *models.Session
}

func (m *mock2FACustomerRepo) GetByEmail(ctx context.Context, email string) (*models.Customer, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.customer, nil
}

func (m *mock2FACustomerRepo) Create(ctx context.Context, customer *models.Customer) error {
	return nil
}

func (m *mock2FACustomerRepo) UpdateProfile(ctx context.Context, customerID string, params repository.ProfileUpdateParams) (*models.Customer, error) {
	return m.customer, nil
}

func (m *mock2FACustomerRepo) UpdateStatus(ctx context.Context, id, status string) error {
	return nil
}

func (m *mock2FACustomerRepo) SoftDelete(ctx context.Context, id string) error {
	return nil
}

func (m *mock2FACustomerRepo) CreateSession(ctx context.Context, session *models.Session) error {
	sessionCopy := *session
	m.createdSession = &sessionCopy
	return nil
}

func (m *mock2FACustomerRepo) GetSession(ctx context.Context, id string) (*models.Session, error) {
	if m.getSessionErr != nil {
		return nil, m.getSessionErr
	}
	if m.session != nil {
		return m.session, nil
	}
	return nil, sharederrors.ErrNotFound
}

func (m *mock2FACustomerRepo) GetSessionByRefreshToken(ctx context.Context, refreshTokenHash string) (*models.Session, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mock2FACustomerRepo) DeleteSession(ctx context.Context, id string) error {
	return m.deleteSessionErr
}

func (m *mock2FACustomerRepo) CountSessionsByUser(ctx context.Context, userID, userType string) (int, error) {
	return 0, nil
}

func (m *mock2FACustomerRepo) DeleteOldestSession(ctx context.Context, userID, userType string) error {
	return nil
}

func (m *mock2FACustomerRepo) DeleteExpiredSessions(ctx context.Context) error {
	return nil
}

func (m *mock2FACustomerRepo) GetSessionLastReauthAt(ctx context.Context, sessionID string) (*time.Time, error) {
	return nil, nil
}

func (m *mock2FACustomerRepo) UpdateSessionLastReauthAt(ctx context.Context, sessionID string, timestamp time.Time) error {
	return nil
}

func (m *mock2FACustomerRepo) GetFailedLoginCount(ctx context.Context, email string, window time.Duration) (int, error) {
	return 0, nil
}

func (m *mock2FACustomerRepo) RecordFailedLogin(ctx context.Context, email string) error {
	return nil
}

func (m *mock2FACustomerRepo) ClearFailedLogins(ctx context.Context, email string) error {
	return nil
}

func (m *mock2FACustomerRepo) UpdateCustomerPasswordHash(ctx context.Context, id, passwordHash string) error {
	return nil
}

func (m *mock2FACustomerRepo) CreatePasswordReset(ctx context.Context, reset *models.PasswordReset) error {
	return nil
}

func (m *mock2FACustomerRepo) GetPasswordResetByTokenHash(ctx context.Context, tokenHash string) (*models.PasswordReset, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mock2FACustomerRepo) ResetPasswordWithToken(ctx context.Context, tokenHash, passwordHash string) (*models.PasswordReset, error) {
	return nil, repository.ErrNoRowsAffected
}

func (m *mock2FACustomerRepo) MarkPasswordResetUsed(ctx context.Context, id string) error {
	return nil
}

func (m *mock2FACustomerRepo) UpdateBackupCodes(ctx context.Context, userID string, codes []string) error {
	if m.customer != nil {
		m.customer.TOTPBackupCodesHash = append([]string(nil), codes...)
	}
	return nil
}

func (m *mock2FACustomerRepo) UpdateBackupCodesShown(ctx context.Context, id string, shown bool) error {
	if m.customer != nil {
		m.customer.TOTPBackupCodesShown = shown
	}
	return nil
}

func (m *mock2FACustomerRepo) UpdateBackupCodesWithShown(ctx context.Context, id string, backupCodesHash []string) error {
	if m.customer != nil {
		m.customer.TOTPBackupCodesHash = append([]string(nil), backupCodesHash...)
		m.customer.TOTPBackupCodesShown = false
	}
	return nil
}

func (m *mock2FACustomerRepo) List(ctx context.Context, filter repository.CustomerListFilter) ([]models.Customer, bool, string, error) {
	return nil, false, "", nil
}

func (m *mock2FACustomerRepo) UpdateExternalClientID(ctx context.Context, id string, externalClientID int) error {
	return nil
}

func (m *mock2FACustomerRepo) GetByID(ctx context.Context, id string) (*models.Customer, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.customer, nil
}

func (m *mock2FACustomerRepo) UpdateTOTPEnabled(ctx context.Context, id string, enabled bool, secretEncrypted *string, backupCodesHash []string) error {
	m.updateCalls++
	if m.updateErr != nil {
		return m.updateErr
	}
	m.customer.TOTPEnabled = enabled
	m.customer.TOTPSecretEncrypted = secretEncrypted
	m.customer.TOTPBackupCodesHash = backupCodesHash
	return nil
}

func (m *mock2FACustomerRepo) DeleteSessionsByUser(ctx context.Context, userID, userType string) error {
	return nil
}

func test2FALogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

type mockRefreshSessionRepo struct {
	*mock2FACustomerRepo
	sessionByRefreshToken *models.Session
	deleteSessionIDs      []string
}

type atomicRotateResult struct {
	session *models.Session
	err     error
}

type atomicRefreshSessionRepo struct {
	*mock2FACustomerRepo
	sessionByRefreshToken *models.Session
	result                atomicRotateResult
	rotateFunc            func(
		ctx context.Context,
		currentRefreshTokenHash string,
		newSession *models.Session,
	) (*models.Session, error)
	calls atomic.Int32
}

func (m *mockRefreshSessionRepo) GetSessionByRefreshToken(ctx context.Context, refreshTokenHash string) (*models.Session, error) {
	if m.sessionByRefreshToken == nil {
		return nil, sharederrors.ErrNotFound
	}
	return m.sessionByRefreshToken, nil
}

func (m *mockRefreshSessionRepo) DeleteSession(ctx context.Context, id string) error {
	m.deleteSessionIDs = append(m.deleteSessionIDs, id)
	if m.mock2FACustomerRepo != nil {
		return m.mock2FACustomerRepo.DeleteSession(ctx, id)
	}
	return nil
}

func (m *atomicRefreshSessionRepo) RotateSession(
	ctx context.Context,
	currentRefreshTokenHash string,
	newSession *models.Session,
) (*models.Session, error) {
	m.calls.Add(1)
	if m.rotateFunc != nil {
		return m.rotateFunc(ctx, currentRefreshTokenHash, newSession)
	}
	if m.mock2FACustomerRepo != nil {
		sessionCopy := *newSession
		m.createdSession = &sessionCopy
	}
	return m.result.session, m.result.err
}

func (m *atomicRefreshSessionRepo) GetSessionByRefreshToken(ctx context.Context, refreshTokenHash string) (*models.Session, error) {
	if m.sessionByRefreshToken != nil {
		return m.sessionByRefreshToken, nil
	}
	if m.result.session != nil {
		return m.result.session, nil
	}
	return nil, sharederrors.ErrNotFound
}

func TestCreateLoginSession_BindsAccessTokenToSessionID(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	repo := &mock2FACustomerRepo{}
	authService := &AuthService{
		customerRepo: repo,
		authConfig: middleware.AuthConfig{
			JWTSecret: "test-secret-key-that-is-32-bytes-long!!",
			Issuer:    "virtuestack",
		},
		logger: logger,
	}

	tokens, _, err := authService.CreateLoginSession(
		ctx,
		"customer-123",
		"customer",
		"",
		"127.0.0.1",
		"unit-test",
		CustomerRefreshTokenDuration,
	)
	require.NoError(t, err)
	require.NotNil(t, repo.createdSession)

	claims, err := middleware.ValidateJWT(authService.authConfig, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, repo.createdSession.ID, claims.SessionID)
	assert.Equal(t, "customer-123", claims.UserID)
}

func TestRefreshToken_BindsReplacementAccessTokenToNewSessionID(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	refreshToken, err := middleware.GenerateRefreshToken()
	require.NoError(t, err)

	repo := &mockRefreshSessionRepo{
		mock2FACustomerRepo: &mock2FACustomerRepo{
			customer: &models.Customer{
				ID:     "customer-123",
				Status: models.CustomerStatusActive,
			},
		},
		sessionByRefreshToken: &models.Session{
			ID:               "old-session-id",
			UserID:           "customer-123",
			UserType:         "customer",
			RefreshTokenHash: crypto.HashSHA256(refreshToken),
			ExpiresAt:        time.Now().Add(time.Hour),
		},
	}
	authService := &AuthService{
		customerRepo: repo,
		authConfig: middleware.AuthConfig{
			JWTSecret: "test-secret-key-that-is-32-bytes-long!!",
			Issuer:    "virtuestack",
		},
		logger: logger,
	}

	tokens, _, err := authService.RefreshToken(
		ctx,
		refreshToken,
		"127.0.0.1",
		"unit-test",
	)
	require.NoError(t, err)
	require.NotNil(t, repo.createdSession)

	claims, err := middleware.ValidateJWT(authService.authConfig, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, repo.createdSession.ID, claims.SessionID)
	assert.Contains(t, repo.deleteSessionIDs, "old-session-id")
}

func TestRefreshToken_AtomicRotationRejectsConcurrentReuse(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	refreshToken, err := middleware.GenerateRefreshToken()
	require.NoError(t, err)

	repo := &atomicRefreshSessionRepo{
		mock2FACustomerRepo: &mock2FACustomerRepo{
			customer: &models.Customer{
				ID:     "customer-123",
				Status: models.CustomerStatusActive,
			},
		},
		sessionByRefreshToken: &models.Session{
			ID:               "old-session-id",
			UserID:           "customer-123",
			UserType:         "customer",
			RefreshTokenHash: crypto.HashSHA256(refreshToken),
			ExpiresAt:        time.Now().Add(time.Hour),
		},
		result: atomicRotateResult{
			session: &models.Session{
				ID:               "old-session-id",
				UserID:           "customer-123",
				UserType:         "customer",
				RefreshTokenHash: crypto.HashSHA256(refreshToken),
				ExpiresAt:        time.Now().Add(time.Hour),
			},
		},
	}

	authService := &AuthService{
		customerRepo: repo,
		authConfig: middleware.AuthConfig{
			JWTSecret: "test-secret-key-that-is-32-bytes-long!!",
			Issuer:    "virtuestack",
		},
		logger: logger,
	}

	tokens, _, err := authService.RefreshToken(ctx, refreshToken, "127.0.0.1", "unit-test")
	require.NoError(t, err)
	require.NotNil(t, repo.createdSession)
	assert.Equal(t, int32(1), repo.calls.Load())

	claims, err := middleware.ValidateJWT(authService.authConfig, tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, repo.createdSession.ID, claims.SessionID)
}

func TestRefreshToken_ConcurrentReuseFailsClosed(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	refreshToken, err := middleware.GenerateRefreshToken()
	require.NoError(t, err)

	baseSession := &models.Session{
		ID:               "old-session-id",
		UserID:           "customer-123",
		UserType:         "customer",
		RefreshTokenHash: crypto.HashSHA256(refreshToken),
		ExpiresAt:        time.Now().Add(time.Hour),
	}

	repo := &atomicRefreshSessionRepo{
		mock2FACustomerRepo: &mock2FACustomerRepo{
			customer: &models.Customer{
				ID:     "customer-123",
				Status: models.CustomerStatusActive,
			},
		},
		sessionByRefreshToken: baseSession,
	}
	var rotateMu sync.Mutex
	consumed := false
	repo.rotateFunc = func(
		ctx context.Context,
		currentRefreshTokenHash string,
		newSession *models.Session,
	) (*models.Session, error) {
		rotateMu.Lock()
		defer rotateMu.Unlock()

		if consumed {
			return nil, repository.ErrNoRowsAffected
		}

		consumed = true
		sessionCopy := *newSession
		repo.createdSession = &sessionCopy
		return baseSession, nil
	}
	authService := &AuthService{
		customerRepo: repo,
		authConfig: middleware.AuthConfig{
			JWTSecret: "test-secret-key-that-is-32-bytes-long!!",
			Issuer:    "virtuestack",
		},
		logger: logger,
	}

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)

	for i := range results {
		go func(idx int) {
			defer wg.Done()
			_, _, results[idx] = authService.RefreshToken(ctx, refreshToken, "127.0.0.1", "unit-test")
		}(i)
	}

	wg.Wait()

	successCount := 0
	unauthorizedCount := 0
	for _, refreshErr := range results {
		if refreshErr == nil {
			successCount++
			continue
		}
		if sharederrors.Is(refreshErr, sharederrors.ErrUnauthorized) {
			unauthorizedCount++
		}
	}

	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, unauthorizedCount)
}

func TestRefreshToken_RejectsSuspendedCustomer(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	refreshToken, err := middleware.GenerateRefreshToken()
	require.NoError(t, err)

	repo := &mockRefreshSessionRepo{
		mock2FACustomerRepo: &mock2FACustomerRepo{
			customer: &models.Customer{
				ID:     "customer-123",
				Status: models.CustomerStatusSuspended,
			},
		},
		sessionByRefreshToken: &models.Session{
			ID:               "old-session-id",
			UserID:           "customer-123",
			UserType:         "customer",
			RefreshTokenHash: crypto.HashSHA256(refreshToken),
			ExpiresAt:        time.Now().Add(time.Hour),
		},
	}
	authService := &AuthService{
		customerRepo: repo,
		authConfig: middleware.AuthConfig{
			JWTSecret: "test-secret-key-that-is-32-bytes-long!!",
			Issuer:    "virtuestack",
		},
		logger: logger,
	}

	tokens, newRefreshToken, err := authService.RefreshToken(ctx, refreshToken, "127.0.0.1", "unit-test")
	require.Error(t, err)
	assert.ErrorIs(t, err, sharederrors.ErrUnauthorized)
	assert.Nil(t, tokens)
	assert.Empty(t, newRefreshToken)
	assert.Nil(t, repo.createdSession)
}

func TestLogout_IgnoresMissingSessionDeleteSentinels(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	tests := []struct {
		name      string
		deleteErr error
	}{
		{
			name:      "err not found",
			deleteErr: sharederrors.ErrNotFound,
		},
		{
			name:      "err no rows affected",
			deleteErr: sharederrors.ErrNoRowsAffected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mock2FACustomerRepo{deleteSessionErr: tt.deleteErr}
			authService := &AuthService{
				customerRepo: repo,
				logger:       logger,
			}

			err := authService.Logout(ctx, "missing-session")
			require.NoError(t, err)
		})
	}
}

func TestValidateAccessSession_RejectsMissingSession(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	authService := &AuthService{
		customerRepo: &mock2FACustomerRepo{getSessionErr: sharederrors.ErrNotFound},
		logger:       logger,
	}

	err := authService.ValidateAccessSession(ctx, "missing-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, sharederrors.ErrUnauthorized)
}

func TestValidateAccessSession_RejectsExpiredSession(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	authService := &AuthService{
		customerRepo: &mock2FACustomerRepo{
			session: &models.Session{
				ID:        "expired-session",
				ExpiresAt: time.Now().Add(-time.Minute),
			},
		},
		logger: logger,
	}

	err := authService.ValidateAccessSession(ctx, "expired-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, sharederrors.ErrUnauthorized)
}

func TestValidateAccessSession_PropagatesLookupErrors(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	authService := &AuthService{
		customerRepo: &mock2FACustomerRepo{getSessionErr: errors.New("database unavailable")},
		logger:       logger,
	}

	err := authService.ValidateAccessSession(ctx, "broken-session")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting session")
	assert.Contains(t, err.Error(), "database unavailable")
}

func TestLogout_PropagatesUnexpectedDeleteErrors(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	repo := &mock2FACustomerRepo{deleteSessionErr: errors.New("database unavailable")}
	authService := &AuthService{
		customerRepo: repo,
		logger:       logger,
	}

	err := authService.Logout(ctx, "session-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deleting session")
	assert.Contains(t, err.Error(), "database unavailable")
}

func TestInitiate2FA(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	encryptionKey, err := crypto.GenerateEncryptionKey()
	require.NoError(t, err)

	t.Run("Success", func(t *testing.T) {
		customer := &models.Customer{
			ID:          "test-customer-id",
			Email:       "test@example.com",
			TOTPEnabled: false,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		result, err := authService.Initiate2FA(ctx, "test-customer-id", "test@example.com")
		require.NoError(t, err)
		assert.NotEmpty(t, result.Secret)
		assert.NotEmpty(t, result.QRURL)
		assert.Empty(t, customer.TOTPBackupCodesHash)
		assert.NotNil(t, customer.TOTPSecretEncrypted)
		assert.False(t, customer.TOTPEnabled)
	})

	t.Run("AlreadyEnabled", func(t *testing.T) {
		secret := "encrypted-secret"
		customer := &models.Customer{
			ID:                  "test-customer-id",
			Email:               "test@example.com",
			TOTPEnabled:         true,
			TOTPSecretEncrypted: &secret,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		result, err := authService.Initiate2FA(ctx, "test-customer-id", "test@example.com")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, sharederrors.ErrTwoFAAlreadyEnabled)
	})

	t.Run("CustomerNotFound", func(t *testing.T) {
		mock := &mock2FACustomerRepo{getErr: sharederrors.ErrNotFound}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		result, err := authService.Initiate2FA(ctx, "nonexistent-id", "test@example.com")
		require.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestEnable2FA(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	encryptionKey, err := crypto.GenerateEncryptionKey()
	require.NoError(t, err)

	t.Run("Success", func(t *testing.T) {
		encryptedSecret, err := crypto.Encrypt("JBSWY3DPEHPK3PXP", encryptionKey)
		require.NoError(t, err)

		customer := &models.Customer{
			ID:                  "test-customer-id",
			TOTPEnabled:         false,
			TOTPSecretEncrypted: &encryptedSecret,
			TOTPBackupCodesHash: []string{"hash1", "hash2"},
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		validCode := generateValidTOTPCode("JBSWY3DPEHPK3PXP")

		backupCodes, err := authService.Enable2FA(ctx, "test-customer-id", validCode)
		require.NoError(t, err)
		assert.Len(t, backupCodes, 10)
		assert.True(t, customer.TOTPEnabled)
		assert.Len(t, customer.TOTPBackupCodesHash, 10)
	})

	t.Run("AlreadyEnabled", func(t *testing.T) {
		secret := "encrypted-secret"
		customer := &models.Customer{
			ID:                  "test-customer-id",
			TOTPEnabled:         true,
			TOTPSecretEncrypted: &secret,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		backupCodes, err := authService.Enable2FA(ctx, "test-customer-id", "123456")
		require.Error(t, err)
		assert.Nil(t, backupCodes)
		assert.ErrorIs(t, err, sharederrors.ErrTwoFAAlreadyEnabled)
	})

	t.Run("NotInitiated", func(t *testing.T) {
		customer := &models.Customer{
			ID:                  "test-customer-id",
			TOTPEnabled:         false,
			TOTPSecretEncrypted: nil,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		backupCodes, err := authService.Enable2FA(ctx, "test-customer-id", "123456")
		require.Error(t, err)
		assert.Nil(t, backupCodes)
		assert.ErrorIs(t, err, sharederrors.ErrTwoFASetupNotInitiated)
	})

	t.Run("InvalidCode", func(t *testing.T) {
		encryptedSecret, err := crypto.Encrypt("JBSWY3DPEHPK3PXP", encryptionKey)
		require.NoError(t, err)

		customer := &models.Customer{
			ID:                  "test-customer-id",
			TOTPEnabled:         false,
			TOTPSecretEncrypted: &encryptedSecret,
			TOTPBackupCodesHash: []string{"hash1", "hash2"},
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		backupCodes, err := authService.Enable2FA(ctx, "test-customer-id", "000000")
		require.Error(t, err)
		assert.Nil(t, backupCodes)
		assert.True(t, errors.Is(err, sharederrors.ErrUnauthorized))
		assert.False(t, customer.TOTPEnabled)
	})
}

func TestDisable2FA(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	encryptionKey, err := crypto.GenerateEncryptionKey()
	require.NoError(t, err)

	t.Run("Success", func(t *testing.T) {
		passwordHash, err := hashTestPassword("correct-password")
		require.NoError(t, err)

		secret := "encrypted-secret"
		customer := &models.Customer{
			ID:                  "test-customer-id",
			PasswordHash:        &passwordHash,
			TOTPEnabled:         true,
			TOTPSecretEncrypted: &secret,
			TOTPBackupCodesHash: []string{"hash1", "hash2"},
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		err = authService.Disable2FA(ctx, "test-customer-id", "correct-password")
		require.NoError(t, err)
		assert.False(t, customer.TOTPEnabled)
		assert.Nil(t, customer.TOTPSecretEncrypted)
	})

	t.Run("NotEnabled", func(t *testing.T) {
		customer := &models.Customer{
			ID:          "test-customer-id",
			TOTPEnabled: false,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		err := authService.Disable2FA(ctx, "test-customer-id", "password")
		require.Error(t, err)
		assert.ErrorIs(t, err, sharederrors.ErrTwoFANotEnabled)
	})

	t.Run("InvalidPassword", func(t *testing.T) {
		passwordHash, err := hashTestPassword("correct-password")
		require.NoError(t, err)

		secret := "encrypted-secret"
		customer := &models.Customer{
			ID:                  "test-customer-id",
			PasswordHash:        &passwordHash,
			TOTPEnabled:         true,
			TOTPSecretEncrypted: &secret,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		err = authService.Disable2FA(ctx, "test-customer-id", "wrong-password")
		require.Error(t, err)
		assert.True(t, errors.Is(err, sharederrors.ErrUnauthorized))
		assert.True(t, customer.TOTPEnabled)
	})
}

func TestGet2FAStatus(t *testing.T) {
	ctx := context.Background()
	logger := test2FALogger()

	encryptionKey, err := crypto.GenerateEncryptionKey()
	require.NoError(t, err)

	t.Run("Enabled", func(t *testing.T) {
		secret := "encrypted-secret"
		customer := &models.Customer{
			ID:                  "test-customer-id",
			TOTPEnabled:         true,
			TOTPSecretEncrypted: &secret,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		enabled, _, err := authService.Get2FAStatus(ctx, "test-customer-id")
		require.NoError(t, err)
		assert.True(t, enabled)
	})

	t.Run("Disabled", func(t *testing.T) {
		customer := &models.Customer{
			ID:          "test-customer-id",
			TOTPEnabled: false,
		}
		mock := &mock2FACustomerRepo{customer: customer}

		authService := &AuthService{
			customerRepo:  mock,
			encryptionKey: encryptionKey,
			logger:        logger,
		}

		enabled, _, err := authService.Get2FAStatus(ctx, "test-customer-id")
		require.NoError(t, err)
		assert.False(t, enabled)
	})
}

func hashTestPassword(password string) (string, error) {
	authService := &AuthService{}
	return authService.hashPassword(password)
}

func generateValidTOTPCode(secret string) string {
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		panic("failed to generate TOTP code: " + err.Error())
	}
	return code
}
