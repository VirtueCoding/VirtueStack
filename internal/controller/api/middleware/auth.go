// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	googleuuid "github.com/google/uuid"
)

const (
	// userIDContextKey is the gin context key for the authenticated user ID.
	userIDContextKey = "user_id"

	// userTypeContextKey is the gin context key for the user type.
	userTypeContextKey = "user_type"

	// roleContextKey is the gin context key for the user role.
	roleContextKey = "role"

	// apiKeyIDContextKey is the gin context key for an authenticated API key ID.
	apiKeyIDContextKey = "api_key_id"

	// actorTypeContextKey is the gin context key for the actor type.
	actorTypeContextKey = "actor_type"

	// tempTokenPurposeClaim is the custom claim key for temp token purpose.
	tempTokenPurposeClaim = "purpose"

	// tempTokenPurposeValue is the expected value of the purpose claim on temp tokens.
	tempTokenPurposeValue = "2fa"

	// tempTokenDuration is the lifetime of a temp (2FA) token.
	tempTokenDuration = 5 * time.Minute
)

// AuthConfig holds JWT configuration for the VirtueStack Controller.
type AuthConfig struct {
	// JWTSecret is the HMAC-SHA256 signing secret. Must be kept confidential.
	JWTSecret string

	// Issuer is the expected issuer claim. Should be "virtuestack".
	Issuer string
}

// JWTClaims represents the claims embedded in a VirtueStack JWT.
type JWTClaims struct {
	// UserID is the subject identifier for the token owner.
	UserID string `json:"sub"`

	// UserType distinguishes "customer" from "admin" users.
	UserType string `json:"user_type"`

	// Role is populated only for admin users ("admin" or "super_admin").
	Role string `json:"role,omitempty"`

	// Purpose is set on temp tokens only ("2fa").
	Purpose string `json:"purpose,omitempty"`

	jwt.RegisteredClaims
}

// APIKeyValidator looks up an API key by its SHA-256 hash.
// Returns the key's database ID and the set of IPs allowed to use it.
// Returns an error if the key is not found or inactive.
type APIKeyValidator func(ctx context.Context, keyHash string) (keyID string, allowedIPs []string, err error)

// JWTAuth returns a Gin middleware that validates JWT tokens from HttpOnly cookies.
// Falls back to Authorization header for API clients that don't use cookies.
// On success it sets "user_id", "user_type", and "role" in gin.Context.
// On failure it aborts with 401 and a standard APIError body.
func JWTAuth(config AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := GetAccessTokenFromCookie(c)
		if tokenString == "" {
			abortWithAuthError(c, http.StatusUnauthorized, "MISSING_TOKEN", "authentication token is required")
			return
		}

		claims, err := parseAndValidateJWT(config, tokenString)
		if err != nil {
			slog.Warn("jwt validation failed",
				"fingerprint", tokenFingerprint(tokenString),
				"error", err,
				"correlation_id", GetCorrelationID(c),
			)
			abortWithAuthError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token is invalid or expired")
			return
		}

		if claims.Purpose == tempTokenPurposeValue {
			abortWithAuthError(c, http.StatusUnauthorized, "INVALID_TOKEN", "temp tokens are not accepted here")
			return
		}

		setAuthContext(c, claims)
		c.Next()
	}
}

// OptionalJWTAuth is like JWTAuth but does not abort when no token is present.
// If a token is provided and valid, the auth context keys are set; otherwise
// the request proceeds as anonymous. Useful for endpoints that serve both
// authenticated and anonymous clients.
func OptionalJWTAuth(config AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := GetAccessTokenFromCookie(c)
		if tokenString == "" {
			c.Next()
			return
		}

		claims, err := parseAndValidateJWT(config, tokenString)
		if err != nil {
			slog.Warn("optional jwt validation failed",
				"fingerprint", tokenFingerprint(tokenString),
				"error", err,
				"correlation_id", GetCorrelationID(c),
			)
			c.Next()
			return
		}

		if claims.Purpose != tempTokenPurposeValue {
			setAuthContext(c, claims)
		}

		c.Next()
	}
}

// APIKeyAuth returns a Gin middleware that validates provisioning API keys.
// The key is read from the X-API-Key request header.
// The raw key is hashed with SHA-256 before database lookup via validator.
// If the key has allowed_ips configured, the request source IP is verified.
// On success it sets "api_key_id" and "actor_type"="provisioning" in gin.Context.
func APIKeyAuth(validator APIKeyValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader("X-API-Key")
		if rawKey == "" {
			abortWithAuthError(c, http.StatusUnauthorized, "MISSING_API_KEY", "X-API-Key header is required")
			return
		}

		keyHash := hashAPIKey(rawKey)
		keyID, allowedIPs, err := validator(c.Request.Context(), keyHash)
		if err != nil {
			slog.Warn("api key validation failed",
				"error", err,
				"correlation_id", GetCorrelationID(c),
			)
			abortWithAuthError(c, http.StatusUnauthorized, "INVALID_API_KEY", "API key is invalid or inactive")
			return
		}

		if len(allowedIPs) > 0 {
			if err := checkAllowedIP(c, allowedIPs); err != nil {
				slog.Warn("api key ip check failed",
					"key_id", keyID,
					"error", err,
					"correlation_id", GetCorrelationID(c),
				)
				abortWithAuthError(c, http.StatusForbidden, "IP_NOT_ALLOWED", "source IP is not permitted for this API key")
				return
			}
		}

		c.Set(apiKeyIDContextKey, keyID)
		c.Set(actorTypeContextKey, "provisioning")
		c.Next()
	}
}

// RequireRole returns a middleware that enforces that the authenticated user
// holds one of the given roles. Must be used after JWTAuth.
// Returns 403 Forbidden if the role does not match.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(c *gin.Context) {
		role := GetRole(c)
		if _, ok := allowed[role]; !ok {
			abortWithAuthError(c, http.StatusForbidden, "INSUFFICIENT_ROLE",
				fmt.Sprintf("role %q is not permitted for this endpoint", role))
			return
		}
		c.Next()
	}
}

// RequireUserType returns a middleware that enforces the user type.
// Must be used after JWTAuth. Returns 403 Forbidden on mismatch.
func RequireUserType(userTypes ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(userTypes))
	for _, ut := range userTypes {
		allowed[ut] = struct{}{}
	}

	return func(c *gin.Context) {
		ut := GetUserType(c)
		if _, ok := allowed[ut]; !ok {
			abortWithAuthError(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS",
				"your account type is not permitted for this endpoint")
			return
		}
		c.Next()
	}
}

// GenerateAccessToken creates a signed JWT access token for the given user.
// duration controls how long the token is valid (typically 15 minutes).
func GenerateAccessToken(config AuthConfig, userID, userType, role string, duration time.Duration) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:   userID,
		UserType: userType,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    config.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			ID:        googleuuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(config.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}

	return signed, nil
}

// GenerateRefreshToken creates a cryptographically secure opaque refresh token.
// Returns a 64-character hex string (32 random bytes).
func GenerateRefreshToken() (string, error) {
	token, err := crypto.GenerateRandomToken(32)
	if err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	return token, nil
}

// GenerateTempToken creates a short-lived token used as a 2FA challenge ticket.
// The token carries a "purpose"="2fa" claim so it cannot be used as an access token.
func GenerateTempToken(config AuthConfig, userID, userType string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:   userID,
		UserType: userType,
		Purpose:  tempTokenPurposeValue,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    config.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tempTokenDuration)),
			ID:        googleuuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(config.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing temp token: %w", err)
	}

	return signed, nil
}

// ValidateTempToken validates a temp (2FA) token and returns its claims.
// Returns an error if the token is invalid, expired, or is not a temp token.
func ValidateTempToken(config AuthConfig, tokenString string) (*JWTClaims, error) {
	claims, err := parseAndValidateJWT(config, tokenString)
	if err != nil {
		return nil, fmt.Errorf("parsing temp token: %w", err)
	}

	if claims.Purpose != tempTokenPurposeValue {
		return nil, fmt.Errorf("token is not a temp token")
	}

	return claims, nil
}

// GetUserID extracts the user_id set by JWTAuth from gin.Context.
// Returns an empty string if not present.
func GetUserID(c *gin.Context) string {
	v, _ := c.Get(userIDContextKey)
	s, _ := v.(string)
	return s
}

// GetUserType extracts the user_type set by JWTAuth from gin.Context.
// Returns an empty string if not present.
func GetUserType(c *gin.Context) string {
	v, _ := c.Get(userTypeContextKey)
	s, _ := v.(string)
	return s
}

// GetRole extracts the role set by JWTAuth from gin.Context.
// Returns an empty string if not present (e.g., for customer users).
func GetRole(c *gin.Context) string {
	v, _ := c.Get(roleContextKey)
	s, _ := v.(string)
	return s
}

// ─── internal helpers ────────────────────────────────────────────────────────

// extractBearerToken reads the Authorization header and returns the raw token.
func extractBearerToken(c *gin.Context) (string, error) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", fmt.Errorf("Authorization header is missing")
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", fmt.Errorf("Authorization header format must be 'Bearer <token>'")
	}

	if parts[1] == "" {
		return "", fmt.Errorf("Bearer token is empty")
	}

	return parts[1], nil
}

// parseAndValidateJWT parses the token string, verifies the signature and
// standard claims (expiry, issuer, algorithm).
func parseAndValidateJWT(config AuthConfig, tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(config.JWTSecret), nil
	},
		jwt.WithIssuer(config.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return nil, fmt.Errorf("parsing jwt: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	return claims, nil
}

// setAuthContext populates the gin context with user info from JWT claims.
func setAuthContext(c *gin.Context, claims *JWTClaims) {
	c.Set(userIDContextKey, claims.UserID)
	c.Set(userTypeContextKey, claims.UserType)
	c.Set(roleContextKey, claims.Role)
}

// hashAPIKey returns the hex-encoded SHA-256 hash of a raw API key.
// The hash is used for database lookup — the plaintext key is never stored.
func hashAPIKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// checkAllowedIP verifies that the request's source IP is in the allowed list.
func checkAllowedIP(c *gin.Context, allowedIPs []string) error {
	clientIP := net.ParseIP(c.ClientIP())
	if clientIP == nil {
		return fmt.Errorf("could not parse client IP")
	}

	for _, allowed := range allowedIPs {
		// Support both plain IPs and CIDR ranges.
		if strings.Contains(allowed, "/") {
			_, network, err := net.ParseCIDR(allowed)
			if err != nil {
				continue
			}
			if network.Contains(clientIP) {
				return nil
			}
		} else {
			if net.ParseIP(allowed).Equal(clientIP) {
				return nil
			}
		}
	}

	return fmt.Errorf("client IP %s is not in allowed list", clientIP)
}

// abortWithAuthError aborts the request with a structured APIError response.
func abortWithAuthError(c *gin.Context, status int, code, message string) {
	apiErr := &sharederrors.APIError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
	}

	resp := ErrorResponse{
		Error: ErrorDetail{
			Code:          apiErr.Code,
			Message:       apiErr.Message,
			CorrelationID: GetCorrelationID(c),
		},
	}

	c.AbortWithStatusJSON(status, resp)
}

// tokenFingerprint returns the first 8 hex characters of sha256(token) for safe logging.
// Never logs any portion of the raw token value.
func tokenFingerprint(token string) string {
	if token == "" {
		return "***"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:8]
}

// Cookie configuration constants for secure token storage.
const (
	// AccessTokenCookieName is the cookie name for access tokens.
	AccessTokenCookieName = "vs_access_token"

	// RefreshTokenCookieName is the cookie name for refresh tokens.
	RefreshTokenCookieName = "vs_refresh_token"

	// AccessTokenMaxAge is the max age for access token cookies (15 minutes).
	AccessTokenMaxAge = 15 * 60

	// RefreshTokenMaxAge is the max age for refresh token cookies (7 days).
	RefreshTokenMaxAge = 7 * 24 * 60 * 60

	// RefreshTokenMaxAgeAdmin is the max age for admin refresh token cookies (4 hours).
	RefreshTokenMaxAgeAdmin = 4 * 60 * 60
)

// SetAuthCookies sets HttpOnly, Secure, SameSite=Strict cookies for tokens.
// Access token: 15 min expiry, path=/
// Refresh token: configurable expiry, path=/api/v1/{userType}/auth/refresh
func SetAuthCookies(c *gin.Context, accessToken, refreshToken string, accessTokenMaxAge, refreshTokenMaxAge int, refreshPath string) {
	// Set access token cookie
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessToken,
		Path:     "/",
		MaxAge:   accessTokenMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	// Set refresh token cookie with restricted path
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshToken,
		Path:     refreshPath,
		MaxAge:   refreshTokenMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearAuthCookies clears both access and refresh token cookies.
func ClearAuthCookies(c *gin.Context, refreshPath string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    "",
		Path:     refreshPath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// GetAccessTokenFromCookie extracts the access token from the cookie.
// Falls back to Authorization header for API clients that don't use cookies.
func GetAccessTokenFromCookie(c *gin.Context) string {
	// First try cookie
	token, err := c.Cookie(AccessTokenCookieName)
	if err == nil && token != "" {
		return token
	}

	// Fall back to Authorization header for API-only clients
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}

// GetRefreshTokenFromCookie extracts the refresh token from the cookie.
// Also checks for refresh_token in request body as fallback.
func GetRefreshTokenFromCookie(c *gin.Context) string {
	// Try cookie first
	token, err := c.Cookie(RefreshTokenCookieName)
	if err == nil && token != "" {
		return token
	}

	return ""
}
