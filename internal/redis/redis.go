// Package redis connects to the shared, persistent Redis that buffers
// analytics/view events (IMPLEMENTATION.md §10.1). It is never a cache for
// early chapter bodies or credentials.
package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client is the go-redis client type.
type Client = goredis.Client

// Open parses url and returns a client with bounded timeouts. It does not
// contact the server; Redis being down must not stop the API from starting.
func Open(url string) (*Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: REDIS_URL: %w", err)
	}
	opts.DialTimeout = 2 * time.Second
	opts.ReadTimeout = time.Second
	opts.WriteTimeout = time.Second
	opts.PoolTimeout = 2 * time.Second
	return goredis.NewClient(opts), nil
}

// Health pings Redis and reports AOF persistence and the eviction policy.
// The analytics buffer requires appendonly=yes and noeviction.
func Health(ctx context.Context, c *Client) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		return "degraded: unreachable"
	}
	cfg, err := c.ConfigGet(ctx, "appendonly").Result()
	if err == nil && cfg["appendonly"] != "yes" {
		return "degraded: appendonly is off"
	}
	pol, err := c.ConfigGet(ctx, "maxmemory-policy").Result()
	if err == nil && !strings.EqualFold(pol["maxmemory-policy"], "noeviction") {
		return "degraded: maxmemory-policy is not noeviction"
	}
	return "ok"
}
