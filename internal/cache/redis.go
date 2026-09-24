package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"mega-magic/internal/config"
)

type Client struct {
	rdb    *redis.Client
	prefix string
}

func New(cfg config.RedisConfig) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &Client{rdb: rdb, prefix: "mm"}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) IsConnected() bool {
	return c.rdb.Ping(context.Background()).Err() == nil
}

// Key builds a namespaced key: mm:{business_id}:{suffix}
func (c *Client) Key(businessID, suffix string) string {
	return fmt.Sprintf("%s:%s:%s", c.prefix, businessID, suffix)
}

// Generic key without tenant scope
func (c *Client) GlobalKey(suffix string) string {
	return fmt.Sprintf("%s:%s", c.prefix, suffix)
}

// --- String operations ---

func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) GetInt(ctx context.Context, key string) (int64, error) {
	return c.rdb.Get(ctx, key).Int64()
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// --- JSON operations ---

func (c *Client) SetJSON(ctx context.Context, key string, v interface{}, ttl time.Duration) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	return c.rdb.Set(ctx, key, data, ttl).Err()
}

func (c *Client) GetJSON(ctx context.Context, key string, v interface{}) error {
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return fmt.Errorf("redis get: %w", err)
	}
	return json.Unmarshal(data, v)
}

// --- Rate limiting ---

func (c *Client) RateLimit(ctx context.Context, key string, max int, window time.Duration) (int, time.Duration, error) {
	pipe := c.rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("rate limit: %w", err)
	}

	ttl, _ := c.rdb.TTL(ctx, key).Result()
	count := int(incr.Val())
	remaining := max - count
	if remaining < 0 {
		remaining = 0
	}
	return remaining, ttl, nil
}

// --- Session cache ---

func (c *Client) CacheSession(ctx context.Context, sessionID string, data interface{}, ttl time.Duration) error {
	return c.SetJSON(ctx, c.GlobalKey("session:"+sessionID), data, ttl)
}

func (c *Client) GetSession(ctx context.Context, sessionID string, v interface{}) error {
	return c.GetJSON(ctx, c.GlobalKey("session:"+sessionID), v)
}

// --- Atomic operations ---

func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *Client) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return c.rdb.IncrBy(ctx, key, n).Result()
}

// --- Set operations for token sets ---

func (c *Client) SAdd(ctx context.Context, key string, members ...string) error {
	return c.rdb.SAdd(ctx, key, members).Err()
}

func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *Client) SRem(ctx context.Context, key string, members ...string) error {
	return c.rdb.SRem(ctx, key, members).Err()
}

// --- Rate limit check (returns true if allowed) ---

func (c *Client) CheckRateLimit(ctx context.Context, key string, maxRequests int, window time.Duration) (bool, error) {
	count, err := c.Incr(ctx, key)
	if err != nil {
		return false, err
	}
	if count == 1 {
		c.rdb.Expire(ctx, key, window)
	}
	return count <= int64(maxRequests), nil
}