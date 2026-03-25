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

	// permissionsContextKey is the gin context key for API key permissions.
	permissionsContextKey = "permissions"

	// vmIDsContextKey is the gin context key for API key VM scope.
	vmIDsContextKey = "vm_ids"

	// tempTokenPurposeClaim is the custom claim key for temp token purpose.
	tempTokenPurposeClaim = "purpose"

	// tempTokenPurposeValue is the expected value of the purpose claim on temp tokens.
	tempTokenPurposeValue = "2fa"

	// tempTokenDuration is the lifetime of a temp (2FA) token.
	tempTokenDuration = 5 * time.Minute

	// reauthTokenPurposeValue is the expected value of the purpose claim on re-auth tokens.
	reauthTokenPurposeValue = "reauth"

	// ReauthTokenDuration is the lifetime of a re-auth token.
	ReauthTokenDuration = 15 * time.Minute
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

// CustomerAPIKeyInfo contains the information returned by CustomerAPIKeyValidator.
type CustomerAPIKeyInfo struct {
	KeyID       string
	CustomerID  string
	Permissions []string
	AllowedIPs  []string
	VMIDs       []string // VMs this key is scoped to (empty = all VMs)
}

// CustomerAPIKeyValidator looks up a customer API key by its raw value.
// The validator is responsible for hashing the raw key (e.g., with HMAC-SHA256)
// before performing the database lookup. This allows the customer package to control
// the hashing algorithm without the middleware needing the server secret (F-068).
// Returns an error if the key is not found, revoked, or expired.
type CustomerAPIKeyValidator func(ctx context.Context, rawKey string) (CustomerAPIKeyInfo, error)

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

// CustomerAPIKeyAuth returns a Gin middleware that validates customer API keys.
// The key is read from the X-API-Key request header.
// The raw key is hashed with SHA-256 before database lookup via validator.
// If the key has allowed_ips configured, the request source IP is verified.
// On success it sets "user_id" (customer_id), "user_type"="customer", "api_key_id", "permissions", and "vm_ids".
// This middleware is for programmatic API access by customers.
func CustomerAPIKeyAuth(validator CustomerAPIKeyValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader("X-API-Key")
		if rawKey == "" {
			abortWithAuthError(c, http.StatusUnauthorized, "MISSING_API_KEY", "X-API-Key header is required")
			return
		}

		// F-068: Pass the raw key to the validator so it can apply HMAC-SHA256
		// with the server secret before the database lookup.
		info, err := validator(c.Request.Context(), rawKey)
		if err != nil {
			slog.Warn("customer api key validation failed",
				"error", err,
				"correlation_id", GetCorrelationID(c),
			)
			abortWithAuthError(c, http.StatusUnauthorized, "INVALID_API_KEY", "API key is invalid, revoked, or expired")
			return
		}

		// Check IP whitelist if configured
		if len(info.AllowedIPs) > 0 {
			if err := checkAllowedIP(c, info.AllowedIPs); err != nil {
				slog.Warn("customer api key ip check failed",
					"key_id", info.KeyID,
					"error", err,
					"correlation_id", GetCorrelationID(c),
				)
				abortWithAuthError(c, http.StatusForbidden, "IP_NOT_ALLOWED", "source IP is not permitted for this API key")
				return
			}
		}

		// Set the standard auth context keys for customer API key auth.
		// This allows downstream handlers to use GetUserID() consistently.
		c.Set(userIDContextKey, info.CustomerID)
		c.Set(userTypeContextKey, "customer")
		c.Set(apiKeyIDContextKey, info.KeyID)
		c.Set(actorTypeContextKey, "customer_api_key")
		c.Set(permissionsContextKey, info.Permissions)
		c.Set(vmIDsContextKey, info.VMIDs)
		c.Next()
	}
}

// JWTOrCustomerAPIKeyAuth returns a middleware that accepts either JWT or customer API key auth.
// JWT authentication is attempted first (from cookie or Authorization header).
// If no JWT is present, falls back to X-API-Key header for API key authentication.
// This allows customers to use either browser sessions (JWT) or programmatic access (API keys).
func JWTOrCustomerAPIKeyAuth(jwtConfig AuthConfig, keyValidator CustomerAPIKeyValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try JWT authentication first
		tokenString := GetAccessTokenFromCookie(c)
		if tokenString != "" {
			claims, err := parseAndValidateJWT(jwtConfig, tokenString)
			if err == nil && claims.Purpose != tempTokenPurposeValue {
				setAuthContext(c, claims)
				c.Next()
				return
			}
			// JWT was present but invalid or is a temp token - abort immediately.
			// Do NOT fall through to API key auth when a JWT token was supplied;
			// falling through would allow an attacker with an expired/tampered token
			// to bypass JWT validation by also supplying an API key.
			slog.Warn("jwt validation failed, rejecting request",
				"fingerprint", tokenFingerprint(tokenString),
				"error", err,
				"correlation_id", GetCorrelationID(c),
			)
			abortWithAuthError(c, http.StatusUnauthorized, "INVALID_TOKEN", "token is invalid or expired")
			return
		}

		// Fall back to API key authentication
		rawKey := c.GetHeader("X-API-Key")
		if rawKey == "" {
			abortWithAuthError(c, http.StatusUnauthorized, "MISSING_AUTH",
				"either authentication cookie or X-API-Key header is required")
			return
		}

		// Pass the raw key to the validator, which computes HMAC-SHA256 internally
		// before the database lookup. This matches the behavior of CustomerAPIKeyAuth.
		info, err := keyValidator(c.Request.Context(), rawKey)
		if err != nil {
			slog.Warn("customer api key validation failed",
				"error", err,
				"correlation_id", GetCorrelationID(c),
			)
			abortWithAuthError(c, http.StatusUnauthorized, "INVALID_API_KEY",
				"API key is invalid, revoked, or expired")
			return
		}

		// Set the standard auth context keys for customer API key auth.
		c.Set(userIDContextKey, info.CustomerID)
		c.Set(userTypeContextKey, "customer")
		c.Set(apiKeyIDContextKey, info.KeyID)
		c.Set(actorTypeContextKey, "customer_api_key")
		c.Set(permissionsContextKey, info.Permissions)
		c.Set(vmIDsContextKey, info.VMIDs)
		c.Next()
	}
}

// GetPermissions extracts the permissions set by CustomerAPIKeyAuth from gin.Context.
// Returns nil if not present (i.e., JWT auth was used instead).
func GetPermissions(c *gin.Context) []string {
	v, _ := c.Get(permissionsContextKey)
	perms, _ := v.([]string)
	return perms
}

// GetVMIDs extracts the VM IDs scope set by CustomerAPIKeyAuth from gin.Context.
// Returns nil if not present or empty (i.e., JWT auth or API key with no VM restriction).
// An empty/nil return means all VMs are accessible.
func GetVMIDs(c *gin.Context) []string {
	v, _ := c.Get(vmIDsContextKey)
	vmIDs, _ := v.([]string)
	return vmIDs
}

// IsVMAllowed checks if a specific VM ID is accessible with the current authentication.
// For JWT auth, returns true (JWT auth has access to all customer's VMs).
// For API key auth with empty vm_ids, returns true (key has access to all VMs).
// For API key auth with vm_ids set, checks if the VM is in the allowed list.
func IsVMAllowed(c *gin.Context, vmID string) bool {
	vmIDs := GetVMIDs(c)
	if len(vmIDs) == 0 {
		// JWT auth or API key with no VM restriction
		return true
	}
	for _, id := range vmIDs {
		if id == vmID {
			return true
		}
	}
	return false
}

// CheckVMScope is a helper function for handlers to check vm_ids scope enforcement.
// It returns true if the VM is allowed, or false and sends an error response if not.
// Use this in handlers that receive vm_id in the request body or need to check
// scope after fetching a resource (backups, snapshots, etc.).
//
// Example usage:
//
//	if !middleware.CheckVMScope(c, req.VMID) {
//	    return
//	}
func CheckVMScope(c *gin.Context, vmID string) bool {
	if IsVMAllowed(c, vmID) {
		return true
	}
	slog.Warn("API key VM scope violation",
		"requested_vm", vmID,
		"allowed_vms", GetVMIDs(c),
		"correlation_id", GetCorrelationID(c),
	)
	abortWithAuthError(c, http.StatusForbidden, "VM_NOT_IN_SCOPE",
		"API key is not authorized for this VM")
	return false
}

// RequireVMScope returns a middleware that checks if the VM in the request path
// is within the API key's allowed VM scope.
// For JWT-authenticated requests, all VMs are accessible.
// Must be used after CustomerAPIKeyAuth or JWTOrCustomerAPIKeyAuth.
func RequireVMScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if vmID == "" {
			// No VM ID in path, let the handler deal with it
			c.Next()
			return
		}

		if !IsVMAllowed(c, vmID) {
			slog.Warn("API key VM scope violation",
				"requested_vm", vmID,
				"allowed_vms", GetVMIDs(c),
				"correlation_id", GetCorrelationID(c),
			)
			abortWithAuthError(c, http.StatusForbidden, "VM_NOT_IN_SCOPE",
				"API key is not authorized for this VM")
			return
		}
		c.Next()
	}
}

// HasPermission checks if the authenticated request has a specific permission.
// For JWT auth, returns true (JWT auth has full permissions).
// For API key auth, checks if the permission is in the key's permission list.
func HasPermission(c *gin.Context, permission string) bool {
	perms := GetPermissions(c)
	if perms == nil {
		// JWT auth has full permissions
		return true
	}
	for _, p := range perms {
		if p == permission {
			return true
		}
	}
	return false
}

// RequirePermission returns a middleware that checks for a specific API key permission.
// For JWT-authenticated requests, all permissions are granted.
// For API key requests, the permission must be in the key's permission list.
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HasPermission(c, permission) {
			abortWithAuthError(c, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS",
				fmt.Sprintf("API key lacks required permission: %s", permission))
			return
		}
		c.Next()
	}
}

// GenerateAccessToken creates a signed JWT access token for the given user.
// duration controls how long the token is valid (typically 15 minutes).
//
// F-215 / Architecture note: Tokens are currently signed with HMAC-SHA256
// (symmetric). This means every service that verifies tokens must share the
// same JWTSecret. If the architecture ever evolves to multiple independent
// services that need to verify tokens without being able to issue them, migrate
// to RS256 (asymmetric signing): the private key stays with the auth service
// and the public key is distributed to verifying services. See:
// https://pkg.go.dev/github.com/golang-jwt/jwt/v5#SigningMethodRSA
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
			NotBefore: jwt.NewNumericDate(now),
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

// GenerateReauthToken creates a short-lived re-auth token for destructive admin operations.
// The token carries a "purpose"="reauth" claim and a 15-minute expiry so it cannot be
// used as a regular access token. This is issued after successful password confirmation
// for operations like DELETE node/VM/customer/storage-backend.
func GenerateReauthToken(config AuthConfig, userID, userType string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:   userID,
		UserType: userType,
		Purpose:  reauthTokenPurposeValue,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    config.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ReauthTokenDuration)),
			ID:        googleuuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(config.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("signing reauth token: %w", err)
	}

	return signed, nil
}

// ValidateReauthToken validates a re-auth token and returns its claims.
// Returns an error if the token is invalid, expired, or is not a re-auth token.
func ValidateReauthToken(config AuthConfig, tokenString string) (*JWTClaims, error) {
	claims, err := parseAndValidateJWT(config, tokenString)
	if err != nil {
		return nil, fmt.Errorf("parsing reauth token: %w", err)
	}

	if claims.Purpose != reauthTokenPurposeValue {
		return nil, fmt.Errorf("token is not a reauth token")
	}

	return claims, nil
}

// ValidateJWT validates a JWT token and returns its claims.
// This is the exported version of parseAndValidateJWT for use by services
// that need to validate tokens outside of the middleware context (e.g., SSO exchange).
func ValidateJWT(config AuthConfig, tokenString string) (*JWTClaims, error) {
	return parseAndValidateJWT(config, tokenString)
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
			allowedIP := net.ParseIP(allowed)
			if allowedIP != nil && allowedIP.Equal(clientIP) {
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
// Access token: 15 min expiry, path=/api/v1/ (restricted to avoid sending to non-API routes)
// Refresh token: configurable expiry, path=/api/v1/{userType}/auth/refresh
func SetAuthCookies(c *gin.Context, accessToken, refreshToken string, accessTokenMaxAge, refreshTokenMaxAge int, refreshPath string) {
	// Set access token cookie with restricted path so it is only sent with API requests.
	// Using path="/" would cause the browser to transmit the token to every request
	// (including static assets, WebSocket upgrades, etc.), unnecessarily widening
	// its exposure. Restricting to /api/v1/ limits transmission to API endpoints only.
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessToken,
		Path:     "/api/v1/",
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
	// Path must match the path used in SetAuthCookies for the browser to clear it.
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/api/v1/",
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
// Falls back to Authorization: Bearer header for API clients that don't use cookies.
//
// SECURITY NOTE (F-209): The Bearer header fallback is intentional for programmatic
// API clients (e.g., CI pipelines, WHMCS provisioning) that cannot store cookies.
// However, browser-facing endpoints should rely exclusively on the HttpOnly cookie
// path: any endpoint that accepts both cookies and Bearer tokens widens the attack
// surface because the Bearer path is accessible from JavaScript (XSS risk) while the
// cookie path is not. If an endpoint must be browser-only, use c.Cookie() directly
// instead of this function so the Bearer fallback is not available to those flows.
// API-key-only flows (APIKeyAuth, CustomerAPIKeyAuth) do not call this function and
// are therefore unaffected by the Bearer fallback.
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
