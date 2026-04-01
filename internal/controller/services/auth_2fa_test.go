package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mock2FACustomerRepo struct {
	customer    *models.Customer
	updateErr   error
	getErr      error
	updateCalls int
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
	return nil
}

func (m *mock2FACustomerRepo) GetSession(ctx context.Context, id string) (*models.Session, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mock2FACustomerRepo) GetSessionByRefreshToken(ctx context.Context, refreshTokenHash string) (*models.Session, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mock2FACustomerRepo) DeleteSession(ctx context.Context, id string) error {
	return nil
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

func (m *mock2FACustomerRepo) MarkPasswordResetUsed(ctx context.Context, id string) error {
	return nil
}

func (m *mock2FACustomerRepo) UpdateBackupCodes(ctx context.Context, userID string, codes []string) error {
	return nil
}

func (m *mock2FACustomerRepo) UpdateBackupCodesShown(ctx context.Context, id string, shown bool) error {
	return nil
}

func (m *mock2FACustomerRepo) UpdateBackupCodesWithShown(ctx context.Context, id string, backupCodesHash []string) error {
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
		assert.Len(t, result.BackupCodes, 10)
		assert.Len(t, customer.TOTPBackupCodesHash, 10)
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
		assert.Contains(t, err.Error(), "already enabled")
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

		err = authService.Enable2FA(ctx, "test-customer-id", validCode)
		require.NoError(t, err)
		assert.True(t, customer.TOTPEnabled)
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

		err := authService.Enable2FA(ctx, "test-customer-id", "123456")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already enabled")
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

		err := authService.Enable2FA(ctx, "test-customer-id", "123456")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not initiated")
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

		err = authService.Enable2FA(ctx, "test-customer-id", "000000")
		require.Error(t, err)
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
		assert.Contains(t, err.Error(), "not enabled")
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
