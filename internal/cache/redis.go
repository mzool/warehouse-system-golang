package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements a Redis-backed cache
type RedisCache struct {
	client *redis.Client
	config *Config
	logger *slog.Logger
}

// RedisConfig holds Redis-specific configuration
type RedisConfig struct {
	// Common cache config
	*Config

	// Redis connection address
	Addr string

	// Redis password
	Password string

	// Redis database number
	DB int

	// Maximum number of retries
	MaxRetries int

	// Connection pool size
	PoolSize int

	// Connection timeout
	DialTimeout time.Duration

	// Read timeout
	ReadTimeout time.Duration

	// Write timeout
	WriteTimeout time.Duration

	// Logger for structured logging
	Logger *slog.Logger
}

// DefaultRedisConfig returns a default Redis configuration
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Config:       DefaultConfig(),
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		MaxRetries:   3,
		PoolSize:     10,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		Logger:       nil,
	}
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(config *RedisConfig) (*RedisCache, error) {
	if config == nil {
		config = DefaultRedisConfig()
	}

	if config.Config == nil {
		config.Config = DefaultConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	client := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		MaxRetries:   config.MaxRetries,
		PoolSize:     config.PoolSize,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Error("redis connection failed", "error", err, "addr", config.Addr)
		return nil, &CacheError{Op: "connect", Err: err}
	}

	logger.Info("redis cache initialized", "addr", config.Addr, "db", config.DB)

	return &RedisCache{
		client: client,
		config: config.Config,
		logger: logger,
	}, nil
}

// Get retrieves a value from Redis
func (rc *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	if !rc.config.Enabled {
		return nil, ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	result, err := rc.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheNotFound
		}
		rc.logger.Error("redis get failed", "error", err, "key", key)
		return nil, &CacheError{Op: "get", Key: key, Err: err}
	}

	return result, nil
}

// Set stores a value in Redis with optional TTL
func (rc *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	if ttl == 0 {
		ttl = rc.config.DefaultTTL
	}

	err := rc.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		rc.logger.Error("redis set failed", "error", err, "key", key)
		return &CacheError{Op: "set", Key: key, Err: err}
	}

	return nil
}

// Delete removes a value from Redis
func (rc *RedisCache) Delete(ctx context.Context, key string) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	err := rc.client.Del(ctx, key).Err()
	if err != nil {
		rc.logger.Error("redis delete failed", "error", err, "key", key)
		return &CacheError{Op: "delete", Key: key, Err: err}
	}

	return nil
}

// Exists checks if a key exists in Redis
func (rc *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	if !rc.config.Enabled {
		return false, ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	count, err := rc.client.Exists(ctx, key).Result()
	if err != nil {
		rc.logger.Error("redis exists failed", "error", err, "key", key)
		return false, &CacheError{Op: "exists", Key: key, Err: err}
	}

	return count > 0, nil
}

// Clear removes all entries from Redis (use with caution!)
func (rc *RedisCache) Clear(ctx context.Context) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	// Only clear keys with our prefix
	keys, err := rc.Keys(ctx, "*")
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	// Prefix keys for deletion
	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = rc.prefixKey(key)
	}

	err = rc.client.Del(ctx, prefixedKeys...).Err()
	if err != nil {
		rc.logger.Error("redis clear failed", "error", err)
		return &CacheError{Op: "clear", Err: err}
	}

	return nil
}

// Keys returns all keys matching the pattern
func (rc *RedisCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	if !rc.config.Enabled {
		return nil, ErrCacheDisabled
	}

	pattern = rc.prefixKey(pattern)

	keys, err := rc.client.Keys(ctx, pattern).Result()
	if err != nil {
		rc.logger.Error("redis keys failed", "error", err, "pattern", pattern)
		return nil, &CacheError{Op: "keys", Err: err}
	}

	// Remove prefix from keys
	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = rc.unprefixKey(key)
	}

	return result, nil
}

// GetMulti retrieves multiple values from Redis
func (rc *RedisCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	if !rc.config.Enabled {
		return nil, ErrCacheDisabled
	}

	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	// Prefix keys
	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = rc.prefixKey(key)
	}

	values, err := rc.client.MGet(ctx, prefixedKeys...).Result()
	if err != nil {
		rc.logger.Error("redis mget failed", "error", err)
		return nil, &CacheError{Op: "mget", Err: err}
	}

	result := make(map[string][]byte)
	for i, value := range values {
		if value != nil {
			if str, ok := value.(string); ok {
				result[keys[i]] = []byte(str)
			}
		}
	}

	return result, nil
}

// SetMulti stores multiple values in Redis
func (rc *RedisCache) SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	if ttl == 0 {
		ttl = rc.config.DefaultTTL
	}

	pipe := rc.client.Pipeline()

	for key, value := range items {
		prefixedKey := rc.prefixKey(key)
		pipe.Set(ctx, prefixedKey, value, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		rc.logger.Error("redis mset failed", "error", err)
		return &CacheError{Op: "mset", Err: err}
	}

	return nil
}

// DeleteMulti removes multiple values from Redis
func (rc *RedisCache) DeleteMulti(ctx context.Context, keys []string) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	if len(keys) == 0 {
		return nil
	}

	// Prefix keys
	prefixedKeys := make([]string, len(keys))
	for i, key := range keys {
		prefixedKeys[i] = rc.prefixKey(key)
	}

	err := rc.client.Del(ctx, prefixedKeys...).Err()
	if err != nil {
		rc.logger.Error("redis del failed", "error", err)
		return &CacheError{Op: "del", Err: err}
	}

	return nil
}

// Increment increments a numeric value
func (rc *RedisCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	if !rc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	result, err := rc.client.IncrBy(ctx, key, delta).Result()
	if err != nil {
		rc.logger.Error("redis incr failed", "error", err, "key", key)
		return 0, &CacheError{Op: "incr", Key: key, Err: err}
	}

	return result, nil
}

// Decrement decrements a numeric value
func (rc *RedisCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	if !rc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	result, err := rc.client.DecrBy(ctx, key, delta).Result()
	if err != nil {
		rc.logger.Error("redis decr failed", "error", err, "key", key)
		return 0, &CacheError{Op: "decr", Key: key, Err: err}
	}

	return result, nil
}

// TTL returns the remaining time to live for a key
func (rc *RedisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if !rc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	ttl, err := rc.client.TTL(ctx, key).Result()
	if err != nil {
		rc.logger.Error("redis ttl failed", "error", err, "key", key)
		return 0, &CacheError{Op: "ttl", Key: key, Err: err}
	}

	return ttl, nil
}

// Expire sets a new TTL for an existing key
func (rc *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	key = rc.prefixKey(key)

	err := rc.client.Expire(ctx, key, ttl).Err()
	if err != nil {
		rc.logger.Error("redis expire failed", "error", err, "key", key)
		return &CacheError{Op: "expire", Key: key, Err: err}
	}

	return nil
}

// Ping checks if Redis is accessible
func (rc *RedisCache) Ping(ctx context.Context) error {
	if !rc.config.Enabled {
		return ErrCacheDisabled
	}

	err := rc.client.Ping(ctx).Err()
	if err != nil {
		return &CacheError{Op: "ping", Err: err}
	}

	return nil
}

// Close closes the Redis connection
func (rc *RedisCache) Close() error {
	return rc.client.Close()
}

func (rc *RedisCache) prefixKey(key string) string {
	if rc.config.Prefix == "" {
		return key
	}
	return rc.config.Prefix + key
}

func (rc *RedisCache) unprefixKey(key string) string {
	if rc.config.Prefix == "" {
		return key
	}
	if len(key) > len(rc.config.Prefix) {
		return key[len(rc.config.Prefix):]
	}
	return key
}
