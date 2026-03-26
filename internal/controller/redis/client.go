package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps go-redis and exposes the middleware.RedisClient-compatible Eval API.
type Client struct {
	*goredis.Client
}

// NewClient creates and health-checks a Redis client from a redis:// URL.
func NewClient(ctx context.Context, redisURL string) (*Client, error) {
	options, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing REDIS_URL: %w", err)
	}

	client := goredis.NewClient(options)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return &Client{Client: client}, nil
}

// Eval executes a Lua script and returns the raw Result payload expected by the middleware.
func (c *Client) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return c.Client.Eval(ctx, script, keys, args...).Result()
}
