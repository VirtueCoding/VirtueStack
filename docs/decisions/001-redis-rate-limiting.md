# Redis Rate Limiting Implementation Plan

**Status:** Draft
**Created:** 2026-03-25
**Target:** Multi-instance HA deployments
**Backward Compatibility:** Yes - single-instance deployments continue to work without Redis

---

## Executive Summary

This document outlines the plan to implement Redis-backed distributed rate limiting for VirtueStack, enabling coordinated rate limiting across multiple controller instances while maintaining backward compatibility with single-instance deployments.

### Current State

- **Single controller**: In-memory rate limiting works correctly
- **Multi controller**: Each instance maintains independent rate limit counters, allowing N× the configured limits
- **Implementation**: Both in-memory and Redis rate limiters are already implemented in `middleware/ratelimit.go`
- **Warning**: `WarnIfInMemoryRateLimitInProduction()` logs a warning when running multi-instance without Redis

### Target State (document in docs/installation.md and README.md)

- **Single controller**: No change - in-memory rate limiting continues to work
- **Multi controller with Redis**: Coordinated rate limiting across all instances

---

## Architecture

### Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Request Flow                                   │
└─────────────────────────────────────────────────────────────────────────┘

                    ┌──────────────┐
                    │    Client    │
                    │   (browser)  │
                    └──────┬───────┘
                           │
                           ▼
                    ┌──────────────┐
                    │  Load Balancer│
                    │   (nginx)    │
                    └──────┬───────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
         ▼                 ▼                 ▼
  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
  │ Controller 1│  │ Controller 2│  │ Controller 3│
  │             │  │             │  │             │
  │ Rate Limiter│  │ Rate Limiter│  │ Rate Limiter│
  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
         │                 │                 │
         └─────────────────┼─────────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │    Redis     │
                    │   Cluster    │
                    │              │
                    │  SET resource:*
                    │  ZADD timestamp
                    │  EVAL Lua
                    └──────────────┘

                    ┌──────────────┐
                    │  PostgreSQL  │
                    │   (sessions, │
                    │    tasks)    │
                    └──────────────┘
```

### Rate Limit Key Namespaces

```
ratelimit:login:{ip_address}           # Login attempts per IP
ratelimit:refresh:{ip_address}          # Token refresh per IP
ratelimit:password:{ip_address}          # Password changes per IP
ratelimit:read:{user_id}                 # Customer read operations
ratelimit:write:{user_id}                # Customer write operations
ratelimit:admin:{user_id}                 # Admin operations
ratelimit:api:{api_key_id}               # Provisioning API key limits
```

---

## Implementation Phases

### Phase 1: Configuration and Infrastructure (Est. 1-2 hours)

#### 1.1 Add Redis Configuration

**File:** `internal/shared/config/config.go`

```go
type ControllerConfig struct {
    // ... existing fields ...
    
    // Redis Configuration (optional for HA deployments)
    RedisURL      string `mapstructure:"REDIS_URL"`       // e.g., "redis://:password@redis:6379"
    RedisPassword string `mapstructure:"REDIS_PASSWORD"` // Extracted from URL or separate
    
    // ... existing fields ...
}
```

**File:** `.env.example`

```bash
# Redis Configuration (optional - required for multi-controller deployments)
# REDIS_URL=redis://:your_secure_password@redis:6379
# REDIS_PASSWORD=your_secure_password
```

#### 1.2 Add Redis to Docker Compose

**File:** `docker-compose.yml`

```yaml
services:
  # ... existing services ...

  # ===========================================================================
  # Redis - Distributed Rate Limiting (optional for single-controller)
  # ===========================================================================
  redis:
    image: redis:7-alpine
    container_name: virtuestack-redis
    restart: unless-stopped
    command: ["redis-server", "--requirepass", "${REDIS_PASSWORD:-}", "--maxmemory", "128mb", "--maxmemory-policy", "allkeys-lru", "--save", "", "--appendonly", "no"]
    volumes:
      - redis_data:/data
    networks:
      - virtuestack-internal
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD:-}", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    deploy:
      resources:
        limits:
          memory: 256M
          cpus: "0.5"
    # Port not exposed externally for security

  # Controller - add Redis environment variable
  controller:
    # ... existing config ...
    environment:
      # ... existing ...
      REDIS_URL: ${REDIS_URL:-}  # Optional - only set for HA
    depends_on:
      # ... existing ...
      redis:
        condition: service_healthy
        # Note: Use 'condition: service_started' if Redis is optional

volumes:
  # ... existing ...
  redis_data:
    name: virtuestack-redis-data
```

#### 1.3 Production Override

**File:** `docker-compose.prod.yml`

```yaml
services:
  redis:
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 128M
    logging:
      driver: json-file
      options:
        max-size: "20m"
        max-file: "3"
        labels: "service"
        tag: "{{.Name}}"
    command: >
      redis-server
      --requirepass ${REDIS_PASSWORD}
      --maxmemory 256mb
      --maxmemory-policy allkeys-lru
      --save ""
      --appendonly no
      --tcp-backlog 511
      --tcp-keepalive 300

  controller:
    environment:
      REDIS_URL: redis://:${REDIS_PASSWORD}@redis:6379
      # Enable Redis rate limiting for production HA
      REDIS_RATE_LIMIT_ENABLED: "true"
    deploy:
      replicas: 3  # Run 3 controller instances
      resources:
        limits:
          cpus: '2'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M
```

---

### Phase 2: Redis Client Integration (Est. 1-2 hours)

#### 2.1 Add Redis Dependency

**File:** `go.mod`

```go
require (
    // ... existing ...
    github.com/redis/go-redis/v9 v9.7.0
)
```

#### 2.2 Create Redis Client Wrapper

**File:** `internal/controller/redis/client.go` (NEW)

```go
// Package redis provides Redis client initialization and health checking.
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds Redis connection configuration.
type Config struct {
	URL      string // Full Redis URL: redis://:password@host:port/db
	Password string // Password (extracted from URL or set separately)
}

// Client wraps the Redis client with health check support.
type Client struct {
	*redis.Client
}

// NewClient creates a new Redis client from configuration.
// Returns nil if URL is not configured (single-controller mode).
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
	if cfg.URL == "" {
		logger.Info("Redis not configured - using in-memory rate limiting")
		return nil, nil
	}

	// Parse Redis URL
	opt, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing Redis URL: %w", err)
	}

	// Override password if provided separately
	if cfg.Password != "" {
		opt.Password = cfg.Password
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to Redis: %w", err)
	}

	logger.Info("Redis client connected successfully",
		"addr", opt.Addr,
		"db", opt.DB)

	return &Client{Client: client}, nil
}

// Implement middleware.RedisClient interface methods
// (this client already implements the interface via redis.Client)
```

#### 2.3 Update Server Initialization

**File:** `internal/controller/server.go`

```go
// Add to Server struct
type Server struct {
    // ... existing fields ...
    
    // Redis client (optional - nil for single-controller deployments)
    redisClient *redis.Client
}

// Add to InitializeServices()
func (s *Server) InitializeServices() error {
    // ... existing initialization ...
    
    // Initialize Redis client (optional)
    var redisClient *redis.Client
    if s.config.RedisURL != "" {
        var err error
        redisClient = redis.NewClient(&redis.Options{
            Addr:     extractRedisAddr(s.config.RedisURL),
            Password: s.config.RedisPassword,
            DB:       0,
        })
        
        // Test connection
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        
        if err := redisClient.Ping(ctx).Err(); err != nil {
            s.logger.Error("failed to connect to Redis", "error", err)
            // Continue without Redis - fall back to in-memory
            redisClient = nil
        } else {
            s.logger.Info("Redis client connected")
        }
    }
    s.redisClient = redisClient
    
    // Initialize rate limiters
    isProduction := s.config.Environment == "production"
    redisConfigured := redisClient != nil
    middleware.WarnIfInMemoryRateLimitInProduction(isProduction, redisConfigured)
    
    // ... rest of initialization ...
}
```

#### 2.4 Create Rate Limiter Factory

**File:** `internal/controller/api/middleware/ratelimit_factory.go` (NEW)

```go
package middleware

import (
	"log/slog"
	
	"github.com/redis/go-redis/v9"
)

// RateLimiterFactory creates rate limiters based on configuration.
// It gracefully falls back to in-memory rate limiting when Redis is unavailable.
type RateLimiterFactory struct {
	redisClient  *redis.Client
	isProduction bool
	logger       *slog.Logger
}

// NewRateLimiterFactory creates a new factory.
func NewRateLimiterFactory(redisClient *redis.Client, isProduction bool, logger *slog.Logger) *RateLimiterFactory {
	return &RateLimiterFactory{
		redisClient:  redisClient,
		isProduction: isProduction,
		logger:       logger,
	}
}

// CreateLoginRateLimit creates a rate limiter for login attempts.
func (f *RateLimiterFactory) CreateLoginRateLimit() gin.HandlerFunc {
	if f.redisClient != nil {
		return RedisRateLimit(f.redisClient, "ratelimit:login:", RateLimitConfig{
			Requests: 5,
			Window:   15 * time.Minute,
			KeyFunc:  keyByIP,
		})
	}
	return LoginRateLimit()
}

// CreateCustomerRateLimits creates method-based rate limits for the customer API.
func (f *RateLimiterFactory) CreateCustomerRateLimits() gin.HandlerFunc {
	if f.redisClient != nil {
		return methodBasedRateLimit(
			RedisRateLimit(f.redisClient, "ratelimit:read:", RateLimitConfig{
				Requests: 100,
				Window:   time.Minute,
				KeyFunc:  keyByUserID,
			}),
			RedisRateLimit(f.redisClient, "ratelimit:write:", RateLimitConfig{
				Requests: 30,
				Window:   time.Minute,
				KeyFunc:  keyByUserID,
			}),
		)
	}
	return CustomerRateLimits()
}

// CreateAdminRateLimit creates a rate limiter for the admin API.
func (f *RateLimiterFactory) CreateAdminRateLimit() gin.HandlerFunc {
	if f.redisClient != nil {
		return RedisRateLimit(f.redisClient, "ratelimit:admin:", RateLimitConfig{
			Requests: 500,
			Window:   time.Minute,
			KeyFunc:  keyByUserID,
		})
	}
	return AdminRateLimit()
}

// CreateProvisioningRateLimit creates a rate limiter for the provisioning API.
func (f *RateLimiterFactory) CreateProvisioningRateLimit() gin.HandlerFunc {
	if f.redisClient != nil {
		return RedisRateLimit(f.redisClient, "ratelimit:provisioning:", RateLimitConfig{
			Requests: 100,
			Window:   time.Minute,
			KeyFunc:  keyByAPIKeyID,
		})
	}
	return ProvisioningRateLimit()
}

// methodBasedRateLimit applies read/write rate limits based on HTTP method.
func methodBasedRateLimit(readLimiter, writeLimiter gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead:
			readLimiter(c)
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			writeLimiter(c)
		default:
			c.Next()
		}
	}
}
```

---

### Phase 3: Route Wiring (Est. 30 minutes)

#### 3.1 Update Customer Routes

**File:** `internal/controller/api/customer/routes.go`

```go
func RegisterRoutes(r *gin.RouterGroup, h *CustomerHandler, authCfg middleware.AuthConfig, rateLimitFactory *middleware.RateLimiterFactory) {
    // Auth routes (no rate limiting for login, but limit refresh)
    auth := r.Group("/auth")
    {
        auth.POST("/login", rateLimitFactory.CreateLoginRateLimit(), h.Login)
        auth.POST("/verify-2fa", h.Verify2FA)
        auth.POST("/refresh", rateLimitFactory.CreateRefreshRateLimit(), h.Refresh)
        auth.POST("/logout", middleware.JWTAuth(authCfg), h.Logout)
        // ... other auth routes
    }
    
    // Protected routes with rate limiting
    protected := r.Group("")
    protected.Use(middleware.JWTOrCustomerAPIKeyAuth(authCfg, h.apiKeyValidator))
    protected.Use(rateLimitFactory.CreateCustomerRateLimits())
    {
        // ... all protected routes
    }
}
```

#### 3.2 Update Admin Routes

**File:** `internal/controller/api/admin/routes.go`

```go
func RegisterRoutes(r *gin.RouterGroup, h *AdminHandler, authCfg middleware.AuthConfig, rateLimitFactory *middleware.RateLimiterFactory) {
    // Auth routes
    auth := r.Group("/auth")
    {
        auth.POST("/login", rateLimitFactory.CreateLoginRateLimit(), h.Login)
        // ... other auth routes
    }
    
    // Protected routes with rate limiting
    protected := r.Group("")
    protected.Use(middleware.JWTAuth(authCfg))
    protected.Use(middleware.Require2FA(authCfg))
    protected.Use(rateLimitFactory.CreateAdminRateLimit())
    {
        // ... all protected routes
    }
}
```

---

### Phase 4: Nginx Load Balancing (Est. 30 minutes)

#### 4.1 Update Nginx Configuration

**File:** `nginx/conf.d/upstream.conf` (NEW)

```nginx
# Upstream for controller instances
upstream controller_backend {
    least_conn;  # Route to least-loaded backend
    server controller:8080 max_fails=3 fail_timeout=30s;
    # Add more instances when scaling:
    # server controller-2:8080 max_fails=3 fail_timeout=30s;
    # server controller-3:8080 max_fails=3 fail_timeout=30s;
    keepalive 32;  # Connection pool
}

# Upstream for admin-webui
upstream admin_webui_backend {
    least_conn;
    server admin-webui:3000 max_fails=3 fail_timeout=30s;
    keepalive 16;
}

# Upstream for customer-webui
upstream customer_webui_backend {
    least_conn;
    server customer-webui:3001 max_fails=3 fail_timeout=30s;
    keepalive 16;
}
```

**File:** `nginx/conf.d/default.conf` (UPDATE)

```nginx
server {
    listen 80;
    server_name _;

    # Health check endpoint
    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }

    # Controller API
    location /api/ {
        proxy_pass http://controller_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
        
        # Timeouts for long-running operations
        proxy_connect_timeout 60s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }

    # Admin WebUI
    location /admin/ {
        proxy_pass http://admin_webui_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # Customer WebUI
    location / {
        proxy_pass http://customer_webui_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}

# HTTPS server (production)
server {
    listen 443 ssl http2;
    server_name _;

    # SSL configuration
    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;

    # Same location blocks as HTTP server
    # ... (copy from above)
}
```

---

### Phase 5: Health Check Endpoints (Est. 15 minutes)

#### 5.1 Update Controller Health Check

**File:** `internal/controller/server.go`

```go
// Health check endpoint needs to check Redis connectivity
func (s *Server) healthHandler(c *gin.Context) {
    health := map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now().UTC(),
        "components": map[string]interface{}{
            "database": "healthy",
            "nats":      "healthy",
        },
    }
    
    // Check database
    ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
    defer cancel()
    
    if err := s.dbPool.Ping(ctx); err != nil {
        health["status"] = "degraded"
        health["components"].(map[string]interface{})["database"] = "unhealthy: " + err.Error()
    }
    
    // Check NATS
    if _, err := s.jetstream.StreamInfo("TASKS"); err != nil {
        health["status"] = "degraded"
        health["components"].(map[string]interface{})["nats"] = "unhealthy: " + err.Error()
    }
    
    // Check Redis (if configured)
    if s.redisClient != nil {
        if err := s.redisClient.Ping(ctx).Err(); err != nil {
            health["status"] = "degraded"
            health["components"].(map[string]interface{})["redis"] = "unhealthy: " + err.Error()
        } else {
            health["components"].(map[string]interface{})["redis"] = "healthy"
        }
    }
    
    // Return appropriate status code
    statusCode := http.StatusOK
    if health["status"] == "degraded" {
        statusCode = http.StatusServiceUnavailable
    }
    
    c.JSON(statusCode, health)
}
```

---

### Phase 6: Testing (Est. 2-3 hours)

#### 6.1 Unit Tests

**File:** `internal/controller/api/middleware/ratelimit_test.go` (NEW)

```go
package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisRateLimiter_Allow(t *testing.T) {
	// Create mini Redis server
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Create Redis client
	client := NewRedisClient(mr.Addr(), "")

	config := RateLimitConfig{
		Requests: 5,
		Window:   time.Minute,
		KeyFunc:  func(c *gin.Context) string { return "test-key" },
	}

	limiter := NewRedisRateLimiter(client, "test:", config)
	ctx := context.Background()

	// Test allowing requests under limit
	for i := 0; i < 5; i++ {
		allowed, remaining, _ := limiter.Allow(ctx, "user:123")
		assert.True(t, allowed, "Request %d should be allowed", i+1)
		assert.Equal(t, 4-i, remaining, "Request %d should have %d remaining", i+1, 4-i)
	}

	// Test blocking request over limit
	allowed, remaining, resetAt := limiter.Allow(ctx, "user:123")
	assert.False(t, allowed, "Request over limit should be blocked")
	assert.Equal(t, 0, remaining)
	assert.True(t, resetAt.After(time.Now()))
}

func TestRedisRateLimiter_WindowExpiry(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := NewRedisClient(mr.Addr(), "")

	config := RateLimitConfig{
		Requests: 2,
		Window:   100 * time.Millisecond, // Short window for testing
		KeyFunc:  func(c *gin.Context) string { return "test-key" },
	}

	limiter := NewRedisRateLimiter(client, "test:", config)
	ctx := context.Background()

	// Use up the limit
	limiter.Allow(ctx, "user:1")
	limiter.Allow(ctx, "user:1")

	// Should be blocked
	allowed, _, _ := limiter.Allow(ctx, "user:1")
	assert.False(t, allowed, "Should be rate limited")

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Fast-forward miniredis time (needed for expiry)
	mr.FastForward(150 * time.Millisecond)

	// Should be allowed again
	allowed, _, _ = limiter.Allow(ctx, "user:1")
	assert.True(t, allowed, "Should be allowed after window expiry")
}

func TestRedisRateLimiter_FailClosed(t *testing.T) {
	config := RateLimitConfig{
		Requests: 5,
		Window:   time.Minute,
		KeyFunc:  func(c *gin.Context) string { return "test-key" },
	}

	// Create limiter with nil client (simulating Redis unavailable)
	limiter := NewRedisRateLimiter(nil, "test:", config)
	ctx := context.Background()

	// Should fail closed (deny all requests)
	allowed, _, _ := limiter.Allow(ctx, "user:123")
	assert.False(t, allowed, "Should deny requests when Redis is unavailable")
}
```

#### 6.2 Integration Tests

**File:** `tests/integration/rate_limit_test.go` (NEW)

```go
package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitIntegration_Login(t *testing.T) {
	SetupTest(t)
	defer TeardownTest(t)

	// Test login rate limiting with Redis
	// This requires Redis to be running in the test suite
	
	for i := 0; i < 5; i++ {
		resp := makeLoginRequest(t, "test@example.com", "wrongpassword")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Login %d should fail", i+1)
	}

	// 6th request should be rate limited
	resp := makeLoginRequest(t, "test@example.com", "wrongpassword")
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "6th login should be rate limited")

	// Wait for rate limit window to pass
	time.Sleep(15 * time.Minute) // This would be shorted in real tests
}

func TestRateLimitIntegration_MultiInstance(t *testing.T) {
	// This test verifies that rate limits are coordinated across instances
	// when Redis is available
	
	SetupTest(t)
	defer TeardownTest(t)
	
	// Simulate requests hitting different controller instances
	// All should share the same rate limit counter via Redis
	
	// First instance: 3 requests
	for i := 0; i < 3; i++ {
		resp := makeLoginRequest(t, "test@example.com", "wrongpassword")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	
	// Second instance: 2 more requests
	for i := 0; i < 2; i++ {
		resp := makeLoginRequest(t, "test@example.com", "wrongpassword")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	
	// Third instance: should be rate limited
	resp := makeLoginRequest(t, "test@example.com", "wrongpassword")
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}
```

---

### Phase 7: Documentation (Est. 30 minutes)

#### 7.1 Update Deployment Documentation

**File:** `docs/installation.md`

```markdown
## High Availability Deployment

### Prerequisites

- Docker and Docker Compose
- PostgreSQL 16+ (or managed service like RDS, Cloud SQL)
- NATS JetStream 2.10+ (or NATS managed service)
- Redis 7+ (for distributed rate limiting)
- SSL certificates

### Multi-Instance Deployment

1. **Configure Environment**

```bash
# .env file for production
POSTGRES_PASSWORD=your_secure_password
NATS_AUTH_TOKEN=your_secure_token
JWT_SECRET=your_jwt_secret
ENCRYPTION_KEY=your_encryption_key
REDIS_PASSWORD=your_redis_password  # NEW
```

2. **Deploy with Replicas**

```bash
# Deploy with 3 controller instances
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

3. **Verify Health**

```bash
# Check all services are healthy
docker compose ps

# Check Redis connectivity
docker compose exec controller wget -q -O - http://localhost:8080/health | jq .
```

### Single-Instance Deployment

If you don't need HA, single-instance deployment works without Redis:

```bash
# Single controller, no Redis required
docker compose up -d
```

Redis is only required for multi-instance deployments. In single-instance mode, rate limiting uses in-memory counters.
```

#### 7.2 Update AGENTS.md

**File:** `AGENTS.md`

```markdown
## Environment Variables

### Controller

| Variable | Required | Description |
|----------|----------|-------------|
| DATABASE_URL | Yes | PostgreSQL connection string |
| NATS_URL | Yes | NATS server URL |
| JWT_SECRET | Yes | HMAC secret for JWT signing |
| ENCRYPTION_KEY | Yes | AES-256 key for secret encryption |
| REDIS_URL | No | Redis URL for distributed rate limiting (required for HA) |
| REDIS_PASSWORD | No | Redis password (extracted from URL or set separately) |
| ... | ... | ... |

### Redis Configuration

Redis is **optional** for single-controller deployments but **required** for multiple controllers:

| Deployment | Redis Required? | Rate Limiting |
|------------|-----------------|---------------|
| Single controller | No | In-memory (correct) |
| Multi controller (no Redis) | No | In-memory per instance (N× limits) |
| Multi controller (with Redis) | Yes | Redis (coordinated) |

Redis connection: `redis://:password@host:6379/0`
```

---

### Phase 8: Migration Path

#### 8.1 Single-Instance to Multi-Instance Migration

```bash
# Step 1: Add Redis to existing deployment
# No downtime - just add the service
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d redis

# Step 2: Wait for Redis to be healthy
docker compose exec redis redis-cli -a $REDIS_PASSWORD ping

# Step 3: Configure controllers to use Redis
# Add REDIS_URL to controller environment
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d controller

# Step 4: Scale controllers
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --scale controller=3

# Step 5: Update nginx to load balance
# Add additional controller instances to upstream
```

#### 8.2 Rollback Plan

```bash
# If issues arise, rollback to single instance:

# Step 1: Scale down to single controller
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --scale controller=1

# Step 2: Remove Redis configuration
# Remove REDIS_URL from controller environment

# Step 3: Restart controller
docker compose -f docker-compose.yml -f docker-compose.prod.yml restart controller

# Step 4: (Optional) Stop Redis service
docker compose -f docker-compose.yml -f docker-compose.prod.yml stop redis
```

---

## Estimated Timeline

| Phase | Task | Estimated Time |
|-------|------|-----------------|
| 1 | Configuration & Infrastructure | 1-2 hours |
| 2 | Redis Client Integration | 1-2 hours |
| 3 | Route Wiring | 30 minutes |
| 4 | Nginx Load Balancing | 30 minutes |
| 5 | Health Check Endpoints | 15 minutes |
| 6 | Testing | 2-3 hours |
| 7 | Documentation | 30 minutes |
| 8 | Migration Path | Documentation only |
| **Total** | | **6-9 hours** |

---

## Testing Checklist

- [ ] Unit tests pass for Redis rate limiter
- [ ] In-memory rate limiter still works when Redis is unavailable
- [ ] Integration tests pass with Redis
- [ ] Multi-instance rate limiting works correctly
- [ ] Health checks include Redis status
- [ ] Redis connection failures are handled gracefully
- [ ] Rate limits reset correctly after window expiry
- [ ] Load balancing distributes requests across controllers
- [ ] Graceful degradation when Redis is unavailable
- [ ] Single-instance deployment still works without Redis
- [ ] All existing API endpoints work with Redis rate limiting
- [ ] Performance tests show acceptable latency for Redis operations

---

## Files Changed Summary

| File | Action | Description |
|------|--------|-------------|
| `docker-compose.yml` | Modify | Add Redis service |
| `docker-compose.prod.yml` | Modify | Add production Redis config |
| `internal/shared/config/config.go` | Modify | Add Redis config |
| `internal/controller/redis/client.go` | Create | Redis client wrapper |
| `internal/controller/server.go` | Modify | Initialize Redis client |
| `internal/controller/api/middleware/ratelimit_factory.go` | Create | Rate limiter factory |
| `internal/controller/api/middleware/ratelimit.go` | Modify | Minor updates |
| `internal/controller/api/customer/routes.go` | Modify | Use rate limit factory |
| `internal/controller/api/admin/routes.go` | Modify | Use rate limit factory |
| `nginx/conf.d/upstream.conf` | Create | Upstream configuration |
| `nginx/conf.d/default.conf` | Modify | Load balancing config |
| `go.mod` | Modify | Add Redis dependency |
| `.env.example` | Modify | Add Redis variables |
| `docs/installation.md` | Modify | HA deployment docs |
| `AGENTS.md` | Modify | Environment variables |
| `tests/integration/rate_limit_test.go` | Create | Integration tests |
| `internal/controller/api/middleware/ratelimit_test.go` | Create | Unit tests |

---

## Success Criteria

1. **Functional Requirements**
   - Single-controller deployment works without Redis
   - Multi-controller deployment uses Redis for coordinated rate limiting
   - Rate limits are enforced correctly across all instances
   - Graceful degradation when Redis is unavailable

2. **Performance Requirements**
   - Redis rate limit check: < 5ms latency (99th percentile)
   - In-memory rate limit check: < 1ms latency
   - No measurable impact on request throughput

3. **Reliability Requirements**
   - 99.9% availability with single-controller outage
   - Automatic failover to in-memory when Redis unavailable
   - No data loss during controller failover

4. **Security Requirements**
   - Redis password required for production
   - Rate limit keys are namespaced to prevent collisions
   - No sensitive data stored in Redis

---

## Appendix: Redis Memory Calculation

### Memory Usage per Key

```
Key: ratelimit:login:{ip_address}
- Key string: ~50 bytes
- Sorted set entries: 5 entries × 32 bytes = 160 bytes
- Total per IP: ~210 bytes
```

### Estimation for 10,000 Active Users

```
- Login rate limit: 10,000 × 210 bytes = 2.1 MB
- Customer read rate limit: 10,000 × 160 bytes = 1.6 MB
- Customer write rate limit: 10,000 × 48 bytes = 480 KB
- Admin rate limit: 100 admins × 800 bytes = 80 KB
- Total: ~4.3 MB + overhead ≈ 10 MB
```

### Recommended Redis Configuration

```bash
# Minimum for 10,000 users
--maxmemory 128mb
--maxmemory-policy allkeys-lru
--save ""              # No persistence to disk
--appendonly no         # No AOF
```

This provides plenty of headroom for peak loads and rate limit window overlap.
