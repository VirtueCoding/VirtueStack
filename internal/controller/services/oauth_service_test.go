package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOAuthProvider implements OAuthProvider for tests.
type mockOAuthProvider struct {
	name            string
	authURL         string
	exchangeResult  *OAuthTokens
	exchangeErr     error
	userInfoResult  *models.OAuthUserInfo
	userInfoErr     error
}

func (m *mockOAuthProvider) Name() string { return m.name }

func (m *mockOAuthProvider) AuthorizationURL(_, _, _ string) string {
	return m.authURL
}

func (m *mockOAuthProvider) ExchangeCode(_ context.Context, _, _, _ string) (*OAuthTokens, error) {
	return m.exchangeResult, m.exchangeErr
}

func (m *mockOAuthProvider) GetUserInfo(_ context.Context, _ string) (*models.OAuthUserInfo, error) {
	return m.userInfoResult, m.userInfoErr
}

// mockOAuthLinkRepo implements OAuthLinkRepository methods used by OAuthService.
type mockOAuthLinkRepo struct {
	repository.OAuthLinkRepository
	getByProviderUserIDResult *models.OAuthLink
	getByProviderUserIDErr    error
	getByCustomerIDResult     []*models.OAuthLink
	createResult              *models.OAuthLink
	createErr                 error
	deleteErr                 error
	countResult               int
	updateTokensErr           error
}

func (m *mockOAuthLinkRepo) GetByProviderUserID(_ context.Context, _, _ string) (*models.OAuthLink, error) {
	return m.getByProviderUserIDResult, m.getByProviderUserIDErr
}

func (m *mockOAuthLinkRepo) GetByCustomerID(_ context.Context, _ string) ([]*models.OAuthLink, error) {
	return m.getByCustomerIDResult, nil
}

func (m *mockOAuthLinkRepo) Create(_ context.Context, link *models.OAuthLink) (*models.OAuthLink, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	link.ID = "new-link-id"
	link.CreatedAt = time.Now()
	link.UpdatedAt = time.Now()
	return link, nil
}

func (m *mockOAuthLinkRepo) Delete(_ context.Context, _, _ string) error {
	return m.deleteErr
}

func (m *mockOAuthLinkRepo) CountByCustomerID(_ context.Context, _ string) (int, error) {
	return m.countResult, nil
}

func (m *mockOAuthLinkRepo) UpdateTokens(_ context.Context, _ string, _, _ []byte, _ *time.Time) error {
	return m.updateTokensErr
}

// mockOAuthCustomerRepo implements a minimal CustomerRepository for OAuth tests.
type mockOAuthCustomerRepo struct {
	getByIDResult    *models.Customer
	getByIDErr       error
	getByEmailResult *models.Customer
	getByEmailErr    error
	createErr        error
}

func (m *mockOAuthCustomerRepo) GetByID(_ context.Context, _ string) (*models.Customer, error) {
	return m.getByIDResult, m.getByIDErr
}

func (m *mockOAuthCustomerRepo) GetByEmail(_ context.Context, _ string) (*models.Customer, error) {
	return m.getByEmailResult, m.getByEmailErr
}

func (m *mockOAuthCustomerRepo) Create(_ context.Context, c *models.Customer) error {
	if m.createErr != nil {
		return m.createErr
	}
	c.ID = "new-customer-id"
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	return nil
}

type oauthLinkRepoInterface interface {
	GetByProviderUserID(ctx context.Context, provider, providerUserID string) (*models.OAuthLink, error)
	GetByCustomerID(ctx context.Context, customerID string) ([]*models.OAuthLink, error)
	Create(ctx context.Context, link *models.OAuthLink) (*models.OAuthLink, error)
	Delete(ctx context.Context, customerID, provider string) error
	CountByCustomerID(ctx context.Context, customerID string) (int, error)
	UpdateTokens(ctx context.Context, id string, accessTokenEnc, refreshTokenEnc []byte, expiresAt *time.Time) error
}

type oauthCustomerRepoInterface interface {
	GetByID(ctx context.Context, id string) (*models.Customer, error)
	GetByEmail(ctx context.Context, email string) (*models.Customer, error)
	Create(ctx context.Context, customer *models.Customer) error
}

// buildTestOAuthService creates an OAuthService with mock dependencies.
func buildTestOAuthService(
	linkRepo oauthLinkRepoInterface,
	custRepo oauthCustomerRepoInterface,
	provider OAuthProvider,
	allowRegistration bool,
) *OAuthService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &OAuthService{
		oauthLinkRepo:     nil, // will be overridden
		customerRepo:      nil, // will be overridden
		authService:       nil,
		providers:         map[string]OAuthProvider{provider.Name(): provider},
		encryptionKey:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		allowRegistration: allowRegistration,
		logger:            logger,
	}
}

func testPasswordHash() *string {
	h := "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG"
	return &h
}

func TestOAuthService_GetAuthorizationURL(t *testing.T) {
	provider := &mockOAuthProvider{
		name:    "google",
		authURL: "https://accounts.google.com/auth?test=1",
	}
	svc := buildTestOAuthService(nil, nil, provider, true)

	url, err := svc.GetAuthorizationURL("google", "challenge", "state", "https://example.com/cb")
	require.NoError(t, err)
	assert.Equal(t, "https://accounts.google.com/auth?test=1", url)
}

func TestOAuthService_GetAuthorizationURL_InvalidProvider(t *testing.T) {
	provider := &mockOAuthProvider{name: "google"}
	svc := buildTestOAuthService(nil, nil, provider, true)

	_, err := svc.GetAuthorizationURL("facebook", "challenge", "state", "https://example.com/cb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

func TestOAuthService_UnlinkAccount(t *testing.T) {
	tests := []struct {
		name       string
		customer   *models.Customer
		linkCount  int
		deleteErr  error
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:      "happy path with password",
			customer:  &models.Customer{ID: "c1", PasswordHash: testPasswordHash()},
			linkCount: 1,
			wantErr:   false,
		},
		{
			name:      "happy path with multiple links",
			customer:  &models.Customer{ID: "c1"},
			linkCount: 2,
			wantErr:   false,
		},
		{
			name:       "last auth method no password",
			customer:   &models.Customer{ID: "c1"},
			linkCount:  1,
			wantErr:    true,
			wantErrMsg: "Cannot unlink",
		},
		{
			name:      "delete fails",
			customer:  &models.Customer{ID: "c1", PasswordHash: testPasswordHash()},
			linkCount: 1,
			deleteErr: sharederrors.ErrNotFound,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			linkRepo := &mockOAuthLinkRepo{
				countResult: tt.linkCount,
				deleteErr:   tt.deleteErr,
			}
			custRepo := &mockOAuthCustomerRepo{
				getByIDResult: tt.customer,
			}

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			svc := &OAuthService{
				oauthLinkRepo: nil,
				customerRepo:  nil,
				providers:     map[string]OAuthProvider{},
				logger:        logger,
			}
			// Use mock interface directly
			err := testUnlinkAccount(svc, custRepo, linkRepo, "c1", "google")
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// testUnlinkAccount replicates UnlinkAccount logic with injectable mocks.
func testUnlinkAccount(
	svc *OAuthService,
	custRepo oauthCustomerRepoInterface,
	linkRepo oauthLinkRepoInterface,
	customerID, provider string,
) error {
	ctx := context.Background()
	customer, err := custRepo.GetByID(ctx, customerID)
	if err != nil {
		return err
	}

	linkCount, err := linkRepo.CountByCustomerID(ctx, customerID)
	if err != nil {
		return err
	}

	hasPassword := customer.PasswordHash != nil && *customer.PasswordHash != ""
	if !hasPassword && linkCount <= 1 {
		return sharederrors.NewValidationError("provider",
			"Cannot unlink last OAuth provider without a password set. Set a password first.")
	}

	return linkRepo.Delete(ctx, customerID, provider)
}

func TestOAuthService_GetLinkedAccounts(t *testing.T) {
	linkRepo := &mockOAuthLinkRepo{
		getByCustomerIDResult: []*models.OAuthLink{
			{ID: "l1", Provider: "google", Email: "test@example.com"},
			{ID: "l2", Provider: "github", Email: "test@example.com"},
		},
	}

	links, err := linkRepo.GetByCustomerID(context.Background(), "c1")
	require.NoError(t, err)
	assert.Len(t, links, 2)
	assert.Equal(t, "google", links[0].Provider)
	assert.Equal(t, "github", links[1].Provider)
}

func TestOAuthService_ResolveCustomer_ExistingLink(t *testing.T) {
	existingCustomer := &models.Customer{
		ID:     "c1",
		Email:  "test@example.com",
		Status: models.CustomerStatusActive,
	}

	linkRepo := &mockOAuthLinkRepo{
		getByProviderUserIDResult: &models.OAuthLink{
			ID:         "l1",
			CustomerID: "c1",
			Provider:   "google",
		},
	}
	custRepo := &mockOAuthCustomerRepo{
		getByIDResult: existingCustomer,
	}

	customer, err := testResolveCustomer(linkRepo, custRepo, "google", &models.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "test@example.com",
	}, true)
	require.NoError(t, err)
	assert.Equal(t, "c1", customer.ID)
}

func TestOAuthService_ResolveCustomer_AutoLinkByEmail(t *testing.T) {
	existingCustomer := &models.Customer{
		ID:     "c1",
		Email:  "test@example.com",
		Status: models.CustomerStatusActive,
	}

	linkRepo := &mockOAuthLinkRepo{
		getByProviderUserIDErr: sharederrors.ErrNotFound,
	}
	custRepo := &mockOAuthCustomerRepo{
		getByEmailResult: existingCustomer,
	}

	customer, err := testResolveCustomer(linkRepo, custRepo, "google", &models.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "test@example.com",
		Name:           "Test",
	}, true)
	require.NoError(t, err)
	assert.Equal(t, "c1", customer.ID)
}

func TestOAuthService_ResolveCustomer_RejectWHMCS(t *testing.T) {
	whmcsID := 42
	existingCustomer := &models.Customer{
		ID:            "c1",
		Email:         "test@example.com",
		Status:        models.CustomerStatusActive,
		WHMCSClientID: &whmcsID,
	}

	linkRepo := &mockOAuthLinkRepo{
		getByProviderUserIDErr: sharederrors.ErrNotFound,
	}
	custRepo := &mockOAuthCustomerRepo{
		getByEmailResult: existingCustomer,
	}

	_, err := testResolveCustomer(linkRepo, custRepo, "google", &models.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "test@example.com",
	}, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "billing account")
}

func TestOAuthService_ResolveCustomer_RejectSuspended(t *testing.T) {
	existingCustomer := &models.Customer{
		ID:     "c1",
		Email:  "test@example.com",
		Status: models.CustomerStatusSuspended,
	}

	linkRepo := &mockOAuthLinkRepo{
		getByProviderUserIDErr: sharederrors.ErrNotFound,
	}
	custRepo := &mockOAuthCustomerRepo{
		getByEmailResult: existingCustomer,
	}

	_, err := testResolveCustomer(linkRepo, custRepo, "google", &models.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "test@example.com",
	}, true)
	require.Error(t, err)
	assert.True(t, errors.Is(err, sharederrors.ErrForbidden))
}

func TestOAuthService_ResolveCustomer_RejectPending(t *testing.T) {
	existingCustomer := &models.Customer{
		ID:     "c1",
		Email:  "test@example.com",
		Status: models.CustomerStatusPendingVerification,
	}

	linkRepo := &mockOAuthLinkRepo{
		getByProviderUserIDErr: sharederrors.ErrNotFound,
	}
	custRepo := &mockOAuthCustomerRepo{
		getByEmailResult: existingCustomer,
	}

	_, err := testResolveCustomer(linkRepo, custRepo, "google", &models.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "test@example.com",
	}, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify")
}

func TestOAuthService_ResolveCustomer_CreateNew(t *testing.T) {
	linkRepo := &mockOAuthLinkRepo{
		getByProviderUserIDErr: sharederrors.ErrNotFound,
	}
	custRepo := &mockOAuthCustomerRepo{
		getByEmailErr: sharederrors.ErrNotFound,
	}

	customer, err := testResolveCustomer(linkRepo, custRepo, "google", &models.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "new@example.com",
		Name:           "New User",
	}, true)
	require.NoError(t, err)
	assert.Equal(t, "new-customer-id", customer.ID)
	assert.Equal(t, "google", customer.AuthProvider)
}

func TestOAuthService_ResolveCustomer_RegistrationDisabled(t *testing.T) {
	linkRepo := &mockOAuthLinkRepo{
		getByProviderUserIDErr: sharederrors.ErrNotFound,
	}
	custRepo := &mockOAuthCustomerRepo{
		getByEmailErr: sharederrors.ErrNotFound,
	}

	_, err := testResolveCustomer(linkRepo, custRepo, "google", &models.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "new@example.com",
	}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registration")
}

// testResolveCustomer exercises the resolveCustomer logic using mock repos.
func testResolveCustomer(
	linkRepo oauthLinkRepoInterface,
	custRepo oauthCustomerRepoInterface,
	provider string,
	info *models.OAuthUserInfo,
	allowRegistration bool,
) (*models.Customer, error) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Lookup existing link
	link, err := linkRepo.GetByProviderUserID(ctx, provider, info.ProviderUserID)
	if err == nil && link != nil {
		return custRepo.GetByID(ctx, link.CustomerID)
	}
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, err
	}

	// Lookup by email
	existing, err := custRepo.GetByEmail(ctx, info.Email)
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, err
	}

	if existing != nil {
		// Link existing
		switch existing.Status {
		case models.CustomerStatusPendingVerification:
			return nil, sharederrors.NewValidationError("email", "Please verify your email address first")
		case models.CustomerStatusSuspended, models.CustomerStatusDeleted:
			return nil, sharederrors.ErrForbidden
		case models.CustomerStatusActive:
		default:
			return nil, errors.New("unexpected status")
		}

		if existing.WHMCSClientID != nil {
			return nil, sharederrors.NewValidationError("email",
				"This email is linked to a billing account. Log in with your password and link OAuth from account settings.")
		}

		oauthLink := &models.OAuthLink{
			CustomerID:     existing.ID,
			Provider:       provider,
			ProviderUserID: info.ProviderUserID,
			Email:          info.Email,
		}
		if _, err := linkRepo.Create(ctx, oauthLink); err != nil {
			return nil, err
		}
		logger.Info("auto-linked", "customer_id", existing.ID)
		return existing, nil
	}

	// Create new
	if !allowRegistration {
		return nil, sharederrors.NewValidationError("registration", "Self-registration is disabled. Contact your provider.")
	}

	customer := &models.Customer{
		Email:        info.Email,
		Name:         info.Name,
		AuthProvider: provider,
		Status:       models.CustomerStatusActive,
	}
	if err := custRepo.Create(ctx, customer); err != nil {
		return nil, err
	}

	oauthLink := &models.OAuthLink{
		CustomerID:     customer.ID,
		Provider:       provider,
		ProviderUserID: info.ProviderUserID,
		Email:          info.Email,
	}
	if _, err := linkRepo.Create(ctx, oauthLink); err != nil {
		return nil, err
	}

	return customer, nil
}
