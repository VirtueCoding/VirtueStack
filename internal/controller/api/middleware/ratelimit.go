// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// RateLimitConfig defines the parameters for a sliding-window rate limiter.
type RateLimitConfig struct {
	// Requests is the maximum number of requests allowed within Window.
	Requests int

	// Window is the sliding time window over which requests are counted.
	Window time.Duration

	// KeyFunc extracts the rate limit key from the request context.
	// Typical implementations key by IP, user ID, or API key ID.
	KeyFunc func(c *gin.Context) string
}

// windowEntry tracks request timestamps for a single rate-limit key.
type windowEntry struct {
	// timestamps holds the time of each request within the current window.
	timestamps []time.Time
}

// rateLimiter holds the in-memory state for a single RateLimitConfig.
type rateLimiter struct {
	config  RateLimitConfig
	mu      sync.RWMutex
	entries map[string]*windowEntry
	ctx     context.Context
	cancel  context.CancelFunc
}

// newRateLimiter constructs a rateLimiter and starts a background cleanup goroutine.
// The goroutine is stopped by calling Stop() on the returned limiter.
//
// Lifecycle note: middleware-registered rate limiters are intentionally scoped to
// the process lifetime. They are created once at server startup (inside
// RateLimit/RateLimit* helper functions) and live until the process exits. Callers
// that need early teardown should retain the *rateLimiter and invoke Stop() during
// graceful shutdown. The cleanup goroutine exits promptly when Stop() is called
// because it selects on rl.ctx.Done().
func newRateLimiter(config RateLimitConfig) *rateLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	rl := &rateLimiter{
		config:  config,
		entries: make(map[string]*windowEntry),
		ctx:     ctx,
		cancel:  cancel,
	}

	go rl.cleanupLoop()

	return rl
}

// cleanupLoop periodically removes expired entries to prevent unbounded memory growth.
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.ctx.Done():
			return
		case <-ticker.C:
			rl.removeExpired()
		}
	}
}

// removeExpired evicts keys whose latest timestamp is older than the window.
func (rl *rateLimiter) removeExpired() {
	cutoff := time.Now().Add(-rl.config.Window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, entry := range rl.entries {
		if len(entry.timestamps) == 0 || entry.timestamps[len(entry.timestamps)-1].Before(cutoff) {
			delete(rl.entries, key)
		}
	}
}

// Stop stops the background cleanup goroutine.
func (rl *rateLimiter) Stop() {
	rl.cancel()
}

// allow returns (allowed, remaining, resetAt).
// It atomically records the request and checks against the limit.
func (rl *rateLimiter) allow(key string) (bool, int, time.Time) {
	now := time.Now()
	cutoff := now.Add(-rl.config.Window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.entries[key]
	if !exists {
		entry = &windowEntry{}
		rl.entries[key] = entry
	}

	// Slide the window: discard timestamps older than the window.
	entry.timestamps = pruneOlderThan(entry.timestamps, cutoff)

	count := len(entry.timestamps)
	resetAt := now.Add(rl.config.Window)
	if count > 0 {
		// The window resets relative to the oldest retained timestamp.
		resetAt = entry.timestamps[0].Add(rl.config.Window)
	}

	if count >= rl.config.Requests {
		remaining := 0
		return false, remaining, resetAt
	}

	// Record this request.
	entry.timestamps = append(entry.timestamps, now)
	remaining := rl.config.Requests - len(entry.timestamps)
	return true, remaining, resetAt
}

// pruneOlderThan removes timestamps before cutoff, preserving order.
func pruneOlderThan(ts []time.Time, cutoff time.Time) []time.Time {
	idx := 0
	for idx < len(ts) && ts[idx].Before(cutoff) {
		idx++
	}
	return ts[idx:]
}

// RateLimit returns a Gin middleware that enforces sliding-window rate limiting.
// When the limit is exceeded, it responds with 429 Too Many Requests and sets
// standard rate-limit response headers.
//
// SECURITY WARNING: This in-memory implementation does NOT protect distributed
// deployments. Each controller instance maintains its own rate limit counters,
// allowing attackers to bypass limits by distributing requests across instances.
//
// For production deployments with multiple controller instances behind a load balancer,
// use RedisRateLimit instead to share rate limit state across all instances.
// See RedisRateLimit() for distributed rate limiting.
func RateLimit(config RateLimitConfig) gin.HandlerFunc {
	rl := newRateLimiter(config)

	return func(c *gin.Context) {
		key := config.KeyFunc(c)

		allowed, remaining, resetAt := rl.allow(key)

		setRateLimitHeaders(c, config.Requests, remaining, resetAt)

		if !allowed {
			retryAfter := int(time.Until(resetAt).Seconds()) + 1
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

			apiErr := &sharederrors.APIError{
				Code:       "RATE_LIMIT_EXCEEDED",
				Message:    "too many requests — please slow down and retry after the indicated period",
				HTTPStatus: http.StatusTooManyRequests,
			}

			resp := ErrorResponse{
				Error: ErrorDetail{
					Code:          apiErr.Code,
					Message:       apiErr.Message,
					CorrelationID: GetCorrelationID(c),
				},
			}

			c.AbortWithStatusJSON(http.StatusTooManyRequests, resp)
			return
		}

		c.Next()
	}
}

// setRateLimitHeaders sets the standard rate-limit informational headers on the response.
func setRateLimitHeaders(c *gin.Context, limit, remaining int, resetAt time.Time) {
	c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))
}

// ─── pre-built rate limit configurations ─────────────────────────────────────

// WarnIfInMemoryRateLimitInProduction logs a startup warning when in-memory rate
// limiting is used while the application is running in production mode.
//
// Each controller process maintains its own in-memory rate limit counters via
// sync.Map. In a horizontally-scaled deployment (multiple controller instances
// behind a load balancer), an attacker can bypass per-key limits by distributing
// requests across instances. Call this function during server startup and supply
// the Redis client when one is available to switch to RedisRateLimit instead.
//
// Usage:
//
//	middleware.WarnIfInMemoryRateLimitInProduction(isProduction, redisClient != nil)
func WarnIfInMemoryRateLimitInProduction(isProduction bool, redisConfigured bool) {
	if isProduction && !redisConfigured {
		slog.Warn("SECURITY: in-memory rate limiting is active in production mode. " +
			"Each controller process maintains independent counters, allowing distributed " +
			"bypass across multiple instances. Configure a Redis backend and use " +
			"RedisRateLimit() to enforce shared rate limits across all instances.")
	}
}

// LoginRateLimit limits login attempts to 5 per 15 minutes per source IP.
// Intended to protect authentication endpoints against brute-force attacks.
func LoginRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 5,
		Window:   15 * time.Minute,
		KeyFunc:  keyByIP,
	})
}

// RefreshRateLimit limits token refresh attempts to 20 per minute per source IP.
// Provides a separate, more permissive limit than LoginRateLimit because browsers
// silently refresh tokens in the background.
func RefreshRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 20,
		Window:   time.Minute,
		KeyFunc:  keyByIP,
	})
}

// PasswordChangeRateLimit limits password change attempts to 5 per 15 minutes per source IP.
// Intended to protect the password change endpoint against brute-force attacks.
func PasswordChangeRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 5,
		Window:   15 * time.Minute,
		KeyFunc:  keyByIP,
	})
}

// ProvisioningRateLimit allows up to 100 requests per minute per API key.
// 100 rpm is sufficient for WHMCS provisioning workflows: a single WHMCS instance
// does not realistically issue more than a handful of VM operations per second and
// each operation maps to one API call. The previous limit of 1000 rpm provided no
// meaningful protection against runaway automation or credential-stuffing via
// provisioning keys. Batch jobs that legitimately exceed this threshold should be
// redesigned to use queued/async operations rather than hammering the API.
func ProvisioningRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 100,
		Window:   time.Minute,
		KeyFunc:  keyByAPIKeyID,
	})
}

// CustomerReadRateLimit limits read operations to 100 per minute per customer.
// Applies to GET endpoints consumed by end customers.
func CustomerReadRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 100,
		Window:   time.Minute,
		KeyFunc:  keyByUserID,
	})
}

// CustomerWriteRateLimit limits write operations to 30 per minute per customer.
// Applies to mutation endpoints consumed by end customers.
func CustomerWriteRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 30,
		Window:   time.Minute,
		KeyFunc:  keyByUserID,
	})
}

// AdminRateLimit allows up to 500 requests per minute per admin user.
// Relaxed limit befitting internal tooling and administrative operations.
func AdminRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 500,
		Window:   time.Minute,
		KeyFunc:  keyByUserID,
	})
}

// ─── KeyFunc implementations ─────────────────────────────────────────────────

// keyByIP returns the client IP address as the rate limit key.
func keyByIP(c *gin.Context) string {
	return "ip:" + c.ClientIP()
}

// keyByUserID returns the authenticated user ID as the rate limit key.
// Falls back to client IP when no user ID is available (unauthenticated requests).
func keyByUserID(c *gin.Context) string {
	if uid := GetUserID(c); uid != "" {
		return "user:" + uid
	}
	return "ip:" + c.ClientIP()
}

// keyByAPIKeyID returns the API key ID as the rate limit key.
// Falls back to client IP for unauthenticated requests.
func keyByAPIKeyID(c *gin.Context) string {
	if v, exists := c.Get(apiKeyIDContextKey); exists {
		if s, ok := v.(string); ok && s != "" {
			return "apikey:" + s
		}
	}
	return "ip:" + c.ClientIP()
}

// ─── Role-based helpers ───────────────────────────────────────────────────────

// GetIsAdmin returns true if the authenticated user has an admin role.
// Admin roles are "admin" and "super_admin".
func GetIsAdmin(c *gin.Context) bool {
	role := GetRole(c)
	return role == "admin" || role == "super_admin"
}

// GetRateLimitForUser returns the appropriate rate limit config based on user role.
// Admin users receive adminConfig, all other users receive customerConfig.
func GetRateLimitForUser(c *gin.Context, customerConfig, adminConfig RateLimitConfig) RateLimitConfig {
	if GetIsAdmin(c) {
		return adminConfig
	}
	return customerConfig
}

// ─── Per-endpoint rate limit configurations ───────────────────────────────────

// VMCreateRateLimit limits VM creation: Customer 10/min, Admin 100/min.
// Endpoint: POST /vms
func VMCreateRateLimit(isAdmin bool) gin.HandlerFunc {
	if isAdmin {
		return RateLimit(RateLimitConfig{Requests: 100, Window: time.Minute, KeyFunc: keyByUserID})
	}
	return RateLimit(RateLimitConfig{Requests: 10, Window: time.Minute, KeyFunc: keyByUserID})
}

// VMDeleteRateLimit limits VM deletion: Customer 5/min, Admin 50/min.
// Endpoint: DELETE /vms/{id}
func VMDeleteRateLimit(isAdmin bool) gin.HandlerFunc {
	if isAdmin {
		return RateLimit(RateLimitConfig{Requests: 50, Window: time.Minute, KeyFunc: keyByUserID})
	}
	return RateLimit(RateLimitConfig{Requests: 5, Window: time.Minute, KeyFunc: keyByUserID})
}

// VMStartRateLimit limits VM start operations: Customer 20/min, Admin 200/min.
// Endpoint: POST /vms/{id}/start
func VMStartRateLimit(isAdmin bool) gin.HandlerFunc {
	if isAdmin {
		return RateLimit(RateLimitConfig{Requests: 200, Window: time.Minute, KeyFunc: keyByUserID})
	}
	return RateLimit(RateLimitConfig{Requests: 20, Window: time.Minute, KeyFunc: keyByUserID})
}

// VMStopRateLimit limits VM stop operations: Customer 20/min, Admin 200/min.
// Endpoint: POST /vms/{id}/stop
func VMStopRateLimit(isAdmin bool) gin.HandlerFunc {
	if isAdmin {
		return RateLimit(RateLimitConfig{Requests: 200, Window: time.Minute, KeyFunc: keyByUserID})
	}
	return RateLimit(RateLimitConfig{Requests: 20, Window: time.Minute, KeyFunc: keyByUserID})
}

// VMListRateLimit limits VM listing: Customer 60/min, Admin 300/min.
// Endpoint: GET /vms
func VMListRateLimit(isAdmin bool) gin.HandlerFunc {
	if isAdmin {
		return RateLimit(RateLimitConfig{Requests: 300, Window: time.Minute, KeyFunc: keyByUserID})
	}
	return RateLimit(RateLimitConfig{Requests: 60, Window: time.Minute, KeyFunc: keyByUserID})
}

// ConsoleTokenRateLimit limits console token requests: Customer 10/min, Admin 50/min.
// Endpoint: POST /vms/{id}/console-token
func ConsoleTokenRateLimit(isAdmin bool) gin.HandlerFunc {
	if isAdmin {
		return RateLimit(RateLimitConfig{Requests: 50, Window: time.Minute, KeyFunc: keyByUserID})
	}
	return RateLimit(RateLimitConfig{Requests: 10, Window: time.Minute, KeyFunc: keyByUserID})
}

// BackupCreateRateLimit limits backup creation: Customer 5/hour, Admin 50/hour.
// Endpoint: POST /backups
func BackupCreateRateLimit(isAdmin bool) gin.HandlerFunc {
	if isAdmin {
		return RateLimit(RateLimitConfig{Requests: 50, Window: time.Hour, KeyFunc: keyByUserID})
	}
	return RateLimit(RateLimitConfig{Requests: 5, Window: time.Hour, KeyFunc: keyByUserID})
}

// RDNSUpdateRateLimit limits rDNS update operations: Customer 10/hour, Admin unlimited.
// Endpoint: PUT /vms/:id/ips/:ipId/rdns
func RDNSUpdateRateLimit() gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Requests: 10,
		Window:   time.Hour,
		KeyFunc:  keyByUserID,
	})
}

// ─── Redis-backed distributed rate limiter ───────────────────────────────────

// RateLimiterBackend defines the interface for rate limiting backends.
// Both in-memory and Redis implementations must satisfy this interface.
type RateLimiterBackend interface {
	// Allow checks if a request is allowed under the rate limit.
	// Returns (allowed, remaining, resetAt).
	Allow(ctx context.Context, key string) (bool, int, time.Time)
}

// RedisClient is a minimal interface for Redis operations needed by rate limiting.
// This allows the rate limiter to work with any Redis client implementation.
type RedisClient interface {
	// ZAdd adds a member with score to a sorted set.
	ZAdd(ctx context.Context, key string, members ...RedisZMember) (int64, error)
	// ZRemRangeByScore removes members with scores between min and max.
	ZRemRangeByScore(ctx context.Context, key string, min, max string) (int64, error)
	// ZCard returns the cardinality (number of elements) of a sorted set.
	ZCard(ctx context.Context, key string) (int64, error)
	// Eval executes a Lua script.
	Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)
}

// RedisZMember represents a member in a Redis sorted set.
type RedisZMember struct {
	Score  float64
	Member string
}

// slidingWindowScript is a Lua script that implements atomic sliding window rate limiting.
// It executes the following operations atomically:
//  1. Removes entries older than the window (ZREMRANGEBYSCORE)
//  2. Counts remaining entries (ZCARD)
//  3. If under limit, adds new entry (ZADD) and sets expiration (EXPIRE)
//  4. Returns remaining count on success, -1 when rate limited
const slidingWindowScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local cutoff = now - window

redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)
local count = redis.call('ZCARD', key)

if count < limit then
    redis.call('ZADD', key, now, member)
    redis.call('EXPIRE', key, math.ceil(window / 1000))
    return limit - count - 1
else
    return -1
end
`

// RedisRateLimiter implements distributed rate limiting using Redis sorted sets.
// It uses the sliding window algorithm with atomic operations for accuracy
// in distributed deployments where multiple controller instances share rate limit state.
//
// The implementation stores request timestamps as sorted set members:
//   - Key: ratelimit:<prefix>:<identifier>
//   - Score: timestamp in milliseconds
//   - Member: unique request ID (timestamp + random suffix)
//
// This approach provides O(log n) complexity for add/remove operations
// and automatic expiration via time-based score queries.
type RedisRateLimiter struct {
	client RedisClient
	prefix string
	config RateLimitConfig
}

// NewRedisRateLimiter creates a Redis-backed rate limiter.
// The prefix is used to namespace rate limit keys (e.g., "ratelimit:api:").
func NewRedisRateLimiter(client RedisClient, prefix string, config RateLimitConfig) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		prefix: prefix,
		config: config,
	}
}

// Allow checks if a request is allowed using an atomic Lua script for Redis.
// This eliminates race conditions that could occur with separate Redis commands.
// The script atomically:
//  1. Removes expired entries (older than the window)
//  2. Counts remaining entries
//  3. If under limit, adds new entry and allows
//  4. If at/over limit, denies
func (rl *RedisRateLimiter) Allow(ctx context.Context, key string) (bool, int, time.Time) {
	now := time.Now()
	nowMs := float64(now.UnixMilli())
	windowMs := float64(rl.config.Window.Milliseconds())
	limit := rl.config.Requests
	fullKey := rl.prefix + key
	member := fmt.Sprintf("%d:%d", now.UnixNano(), now.Nanosecond())

	result, err := rl.client.Eval(ctx, slidingWindowScript, []string{fullKey}, nowMs, windowMs, limit, member)

	// Calculate reset time
	resetAt := now.Add(rl.config.Window)

	if err != nil {
		// SECURITY: Fail closed on Redis errors to prevent rate limit bypass.
		// When Redis is unavailable, deny all requests rather than allowing
		// unauthenticated bypass of rate limiting (CWE-693).
		// This protects against brute force attacks during Redis outages.
		// Log the error for debugging while maintaining security.
		// Note: Using fmt.Printf since we don't have access to a logger here.
		// The middleware chain will log via structured logging at higher levels.
		return false, 0, resetAt
	}

	remaining, ok := result.(int64)
	if !ok {
		// SECURITY: Fail closed on type assertion failure to prevent rate limit bypass.
		return false, 0, resetAt
	}

	if remaining < 0 {
		return false, 0, resetAt
	}

	return true, int(remaining), resetAt
}

// RedisRateLimit returns a Gin middleware using Redis-backed rate limiting.
// This is suitable for distributed deployments where multiple controller
// instances need to share rate limit state.
func RedisRateLimit(client RedisClient, prefix string, config RateLimitConfig) gin.HandlerFunc {
	rl := NewRedisRateLimiter(client, prefix, config)

	return func(c *gin.Context) {
		key := config.KeyFunc(c)

		allowed, remaining, resetAt := rl.Allow(c.Request.Context(), key)

		setRateLimitHeaders(c, config.Requests, remaining, resetAt)

		if !allowed {
			retryAfter := int(time.Until(resetAt).Seconds()) + 1
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

			apiErr := &sharederrors.APIError{
				Code:       "RATE_LIMIT_EXCEEDED",
				Message:    "too many requests — please slow down and retry after the indicated period",
				HTTPStatus: http.StatusTooManyRequests,
			}

			resp := ErrorResponse{
				Error: ErrorDetail{
					Code:          apiErr.Code,
					Message:       apiErr.Message,
					CorrelationID: GetCorrelationID(c),
				},
			}

			c.AbortWithStatusJSON(http.StatusTooManyRequests, resp)
			return
		}

		c.Next()
	}
}
