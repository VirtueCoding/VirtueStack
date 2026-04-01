package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// OAuthServiceConfig holds dependencies for the OAuth service.
type OAuthServiceConfig struct {
	DB                repository.DB
	OAuthLinkRepo     *repository.OAuthLinkRepository
	CustomerRepo      *repository.CustomerRepository
	AuthService       *AuthService
	Providers         map[string]OAuthProvider
	EncryptionKey     string
	AllowRegistration bool
	Logger            *slog.Logger
}

// OAuthService handles OAuth authentication and account linking.
type OAuthService struct {
	db                repository.DB
	oauthLinkRepo     *repository.OAuthLinkRepository
	customerRepo      *repository.CustomerRepository
	authService       *AuthService
	providers         map[string]OAuthProvider
	encryptionKey     string
	allowRegistration bool
	logger            *slog.Logger
}

// NewOAuthService creates a new OAuthService.
func NewOAuthService(cfg OAuthServiceConfig) *OAuthService {
	return &OAuthService{
		db:                cfg.DB,
		oauthLinkRepo:     cfg.OAuthLinkRepo,
		customerRepo:      cfg.CustomerRepo,
		authService:       cfg.AuthService,
		providers:         cfg.Providers,
		encryptionKey:     cfg.EncryptionKey,
		allowRegistration: cfg.AllowRegistration,
		logger:            cfg.Logger.With("component", "oauth-service"),
	}
}

// GetAuthorizationURL generates the OAuth provider redirect URL with PKCE.
func (s *OAuthService) GetAuthorizationURL(
	provider, codeChallenge, state, redirectURI string,
) (string, error) {
	p, err := s.getProvider(provider)
	if err != nil {
		return "", err
	}
	return p.AuthorizationURL(codeChallenge, state, redirectURI), nil
}

// HandleCallback processes the OAuth callback after user authorization.
// Returns auth tokens for an existing or newly created customer.
func (s *OAuthService) HandleCallback(
	ctx context.Context,
	provider, code, codeVerifier, redirectURI, ipAddress, userAgent string,
) (*models.AuthTokens, string, error) {
	p, err := s.getProvider(provider)
	if err != nil {
		return nil, "", err
	}

	tokens, err := p.ExchangeCode(ctx, code, codeVerifier, redirectURI)
	if err != nil {
		return nil, "", fmt.Errorf("exchange oauth code: %w", err)
	}

	userInfo, err := p.GetUserInfo(ctx, tokens.AccessToken)
	if err != nil {
		return nil, "", fmt.Errorf("get oauth user info: %w", err)
	}
	userInfo.Email = strings.ToLower(strings.TrimSpace(userInfo.Email))

	if userInfo.Email == "" {
		return nil, "", sharederrors.NewValidationError(
			"email", "OAuth provider did not return an email address")
	}

	customer, err := s.resolveCustomer(ctx, provider, userInfo)
	if err != nil {
		return nil, "", err
	}

	if err := s.storeOAuthTokens(ctx, provider, userInfo, tokens); err != nil {
		s.logger.Warn("failed to store oauth tokens",
			"customer_id", customer.ID, "provider", provider, "error", err)
	}

	s.authService.EnforceCustomerSessionLimit(ctx, customer.ID)
	authTokens, refreshToken, err := s.authService.CreateLoginSession(
		ctx, customer.ID, "customer", "",
		ipAddress, userAgent, CustomerRefreshTokenDuration,
	)
	if err != nil {
		return nil, "", fmt.Errorf("create oauth session: %w", err)
	}

	s.logger.Info("oauth login successful",
		"customer_id", customer.ID, "provider", provider)
	return authTokens, refreshToken, nil
}

// resolveCustomer finds or creates a customer from OAuth user info.
func (s *OAuthService) resolveCustomer(
	ctx context.Context, provider string, info *models.OAuthUserInfo,
) (*models.Customer, error) {
	link, err := s.oauthLinkRepo.GetByProviderUserID(
		ctx, provider, info.ProviderUserID)
	if err == nil && link != nil {
		return s.customerRepo.GetByID(ctx, link.CustomerID)
	}
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, fmt.Errorf("lookup oauth link: %w", err)
	}

	existing, err := s.customerRepo.GetByEmail(ctx, info.Email)
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, fmt.Errorf("lookup customer by email: %w", err)
	}

	if existing != nil {
		return s.linkExistingCustomer(ctx, provider, info, existing)
	}

	return s.createOAuthCustomer(ctx, provider, info)
}

// linkExistingCustomer handles the case where a customer with matching
// email exists but has no OAuth link for this provider.
func (s *OAuthService) linkExistingCustomer(
	ctx context.Context,
	provider string,
	info *models.OAuthUserInfo,
	customer *models.Customer,
) (*models.Customer, error) {
	switch customer.Status {
	case models.CustomerStatusPendingVerification:
		return nil, sharederrors.NewValidationError(
			"email", "Please verify your email address first")
	case models.CustomerStatusSuspended, models.CustomerStatusDeleted:
		return nil, sharederrors.ErrForbidden
	case models.CustomerStatusActive:
		// Continue to linking logic below
	default:
		return nil, fmt.Errorf("unexpected customer status: %s", customer.Status)
	}

	if customer.ExternalClientID != nil {
		return nil, sharederrors.NewValidationError("email",
			"This email is linked to a billing account. "+
				"Log in with your password and link OAuth from account settings.")
	}

	oauthLink := &models.OAuthLink{
		CustomerID:     customer.ID,
		Provider:       provider,
		ProviderUserID: info.ProviderUserID,
		Email:          info.Email,
		DisplayName:    info.Name,
		AvatarURL:      info.AvatarURL,
	}
	if _, err := s.oauthLinkRepo.Create(ctx, oauthLink); err != nil {
		return nil, fmt.Errorf("auto-link oauth: %w", err)
	}

	s.logger.Info("auto-linked oauth account",
		"customer_id", customer.ID, "provider", provider)
	return customer, nil
}

// createOAuthCustomer creates a new customer from OAuth user info
// inside a single transaction for atomicity.
func (s *OAuthService) createOAuthCustomer(
	ctx context.Context, provider string, info *models.OAuthUserInfo,
) (*models.Customer, error) {
	if !s.allowRegistration {
		return nil, sharederrors.NewValidationError("registration",
			"Self-registration is disabled. Contact your provider.")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	txCustomerRepo := repository.NewCustomerRepository(tx)
	txOAuthRepo := repository.NewOAuthLinkRepository(tx)

	customer := &models.Customer{
		Email:        info.Email,
		Name:         info.Name,
		AuthProvider: provider,
		Status:       models.CustomerStatusActive,
	}
	if err := txCustomerRepo.Create(ctx, customer); err != nil {
		return nil, fmt.Errorf("create oauth customer: %w", err)
	}

	oauthLink := &models.OAuthLink{
		CustomerID:     customer.ID,
		Provider:       provider,
		ProviderUserID: info.ProviderUserID,
		Email:          info.Email,
		DisplayName:    info.Name,
		AvatarURL:      info.AvatarURL,
	}
	if _, err := txOAuthRepo.Create(ctx, oauthLink); err != nil {
		return nil, fmt.Errorf("create oauth link: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit oauth customer: %w", err)
	}

	s.logger.Info("created oauth customer",
		"customer_id", customer.ID, "provider", provider)
	return customer, nil
}

// storeOAuthTokens encrypts and persists OAuth access/refresh tokens.
func (s *OAuthService) storeOAuthTokens(
	ctx context.Context,
	provider string,
	info *models.OAuthUserInfo,
	tokens *OAuthTokens,
) error {
	link, err := s.oauthLinkRepo.GetByProviderUserID(
		ctx, provider, info.ProviderUserID)
	if err != nil {
		return fmt.Errorf("get link for token storage: %w", err)
	}

	var accessEnc, refreshEnc []byte

	if tokens.AccessToken != "" {
		encrypted, err := crypto.Encrypt(tokens.AccessToken, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt access token: %w", err)
		}
		accessEnc = []byte(encrypted)
	}

	if tokens.RefreshToken != "" {
		encrypted, err := crypto.Encrypt(tokens.RefreshToken, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt refresh token: %w", err)
		}
		refreshEnc = []byte(encrypted)
	}

	var expiresAt *time.Time
	if !tokens.ExpiresAt.IsZero() {
		expiresAt = &tokens.ExpiresAt
	}

	return s.oauthLinkRepo.UpdateTokens(ctx, link.ID, accessEnc, refreshEnc, expiresAt)
}

// LinkAccount links an OAuth provider to an authenticated customer's account.
func (s *OAuthService) LinkAccount(
	ctx context.Context,
	customerID, provider, code, codeVerifier, redirectURI string,
) error {
	p, err := s.getProvider(provider)
	if err != nil {
		return err
	}

	tokens, err := p.ExchangeCode(ctx, code, codeVerifier, redirectURI)
	if err != nil {
		return fmt.Errorf("exchange code for link: %w", err)
	}

	userInfo, err := p.GetUserInfo(ctx, tokens.AccessToken)
	if err != nil {
		return fmt.Errorf("get user info for link: %w", err)
	}

	existing, err := s.oauthLinkRepo.GetByProviderUserID(
		ctx, provider, userInfo.ProviderUserID)
	if err == nil && existing != nil {
		if existing.CustomerID == customerID {
			return nil
		}
		return sharederrors.NewValidationError("provider",
			"This OAuth account is already linked to another customer")
	}

	oauthLink := &models.OAuthLink{
		CustomerID:     customerID,
		Provider:       provider,
		ProviderUserID: userInfo.ProviderUserID,
		Email:          userInfo.Email,
		DisplayName:    userInfo.Name,
		AvatarURL:      userInfo.AvatarURL,
	}
	if _, err := s.oauthLinkRepo.Create(ctx, oauthLink); err != nil {
		return fmt.Errorf("create link: %w", err)
	}

	if err := s.storeOAuthTokens(ctx, provider, userInfo, tokens); err != nil {
		s.logger.Warn("failed to store tokens after link",
			"customer_id", customerID, "provider", provider, "error", err)
	}

	s.logger.Info("linked oauth account",
		"customer_id", customerID, "provider", provider)
	return nil
}

// UnlinkAccount removes an OAuth provider from a customer's account.
// Prevents unlinking the last auth method when no password is set.
func (s *OAuthService) UnlinkAccount(
	ctx context.Context, customerID, provider string,
) error {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return fmt.Errorf("get customer for unlink: %w", err)
	}

	linkCount, err := s.oauthLinkRepo.CountByCustomerID(ctx, customerID)
	if err != nil {
		return fmt.Errorf("count oauth links: %w", err)
	}

	hasPassword := customer.PasswordHash != nil && *customer.PasswordHash != ""
	if !hasPassword && linkCount <= 1 {
		return sharederrors.NewValidationError("provider",
			"Cannot unlink last OAuth provider without a password set. "+
				"Set a password first.")
	}

	return s.oauthLinkRepo.Delete(ctx, customerID, provider)
}

// GetLinkedAccounts returns all OAuth links for a customer.
func (s *OAuthService) GetLinkedAccounts(
	ctx context.Context, customerID string,
) ([]*models.OAuthLink, error) {
	return s.oauthLinkRepo.GetByCustomerID(ctx, customerID)
}

func (s *OAuthService) getProvider(name string) (OAuthProvider, error) {
	p, ok := s.providers[name]
	if !ok {
		return nil, sharederrors.NewValidationError(
			"provider", fmt.Sprintf("OAuth provider %q is not enabled", name))
	}
	return p, nil
}
