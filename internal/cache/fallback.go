package cache

import (
	"context"
	"log/slog"
	"time"
)

// FallbackCache implements a cache with Redis primary and memory fallback
type FallbackCache struct {
	primary    Cache
	fallback   Cache
	logger     *slog.Logger
	usePrimary bool
}

// FallbackConfig holds fallback cache configuration
type FallbackConfig struct {
	// Redis configuration
	Redis *RedisConfig

	// Memory cache configuration
	Memory *Config

	// Logger for structured logging
	Logger *slog.Logger

	// Auto-fallback on Redis errors
	AutoFallback bool
}

// DefaultFallbackConfig returns a default fallback configuration
func DefaultFallbackConfig() *FallbackConfig {
	return &FallbackConfig{
		Redis:        DefaultRedisConfig(),
		Memory:       DefaultConfig(),
		Logger:       nil,
		AutoFallback: true,
	}
}

// NewFallbackCache creates a new fallback cache
func NewFallbackCache(config *FallbackConfig) (*FallbackCache, error) {
	if config == nil {
		config = DefaultFallbackConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Try to initialize Redis cache
	redisCache, err := NewRedisCache(config.Redis)
	var primary Cache
	usePrimary := false

	if err != nil {
		logger.Warn("redis cache unavailable, using memory cache only", "error", err)
		primary = nil
	} else {
		primary = redisCache
		usePrimary = true
		logger.Info("fallback cache initialized with redis primary")
	}

	// Initialize memory fallback
	fallback := NewMemoryCache(config.Memory)

	return &FallbackCache{
		primary:    primary,
		fallback:   fallback,
		logger:     logger,
		usePrimary: usePrimary,
	}, nil
}

// Get retrieves a value from cache (primary first, then fallback)
func (fc *FallbackCache) Get(ctx context.Context, key string) ([]byte, error) {
	if fc.usePrimary && fc.primary != nil {
		value, err := fc.primary.Get(ctx, key)
		if err == nil {
			return value, nil
		}

		// Check if error is cache miss (not connection error)
		if err == ErrCacheNotFound || err == ErrCacheMiss {
			return nil, err
		}

		// Log primary cache error
		fc.logger.Warn("primary cache get failed, trying fallback", "error", err, "key", key)
	}

	// Use fallback
	return fc.fallback.Get(ctx, key)
}

// Set stores a value in both caches
func (fc *FallbackCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	var primaryErr error

	// Try primary cache
	if fc.usePrimary && fc.primary != nil {
		primaryErr = fc.primary.Set(ctx, key, value, ttl)
		if primaryErr != nil {
			fc.logger.Warn("primary cache set failed", "error", primaryErr, "key", key)
		}
	}

	// Always update fallback
	fallbackErr := fc.fallback.Set(ctx, key, value, ttl)
	if fallbackErr != nil {
		fc.logger.Error("fallback cache set failed", "error", fallbackErr, "key", key)
		return fallbackErr
	}

	return primaryErr
}

// Delete removes a value from both caches
func (fc *FallbackCache) Delete(ctx context.Context, key string) error {
	// Delete from primary
	if fc.usePrimary && fc.primary != nil {
		if err := fc.primary.Delete(ctx, key); err != nil {
			fc.logger.Warn("primary cache delete failed", "error", err, "key", key)
		}
	}

	// Delete from fallback
	return fc.fallback.Delete(ctx, key)
}

// Exists checks if a key exists in either cache
func (fc *FallbackCache) Exists(ctx context.Context, key string) (bool, error) {
	if fc.usePrimary && fc.primary != nil {
		exists, err := fc.primary.Exists(ctx, key)
		if err == nil {
			return exists, nil
		}
		fc.logger.Warn("primary cache exists check failed, trying fallback", "error", err, "key", key)
	}

	return fc.fallback.Exists(ctx, key)
}

// Clear removes all entries from both caches
func (fc *FallbackCache) Clear(ctx context.Context) error {
	// Clear primary
	if fc.usePrimary && fc.primary != nil {
		if err := fc.primary.Clear(ctx); err != nil {
			fc.logger.Warn("primary cache clear failed", "error", err)
		}
	}

	// Clear fallback
	return fc.fallback.Clear(ctx)
}

// Keys returns all keys matching the pattern
func (fc *FallbackCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	if fc.usePrimary && fc.primary != nil {
		keys, err := fc.primary.Keys(ctx, pattern)
		if err == nil {
			return keys, nil
		}
		fc.logger.Warn("primary cache keys failed, trying fallback", "error", err)
	}

	return fc.fallback.Keys(ctx, pattern)
}

// GetMulti retrieves multiple values from cache
func (fc *FallbackCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	if fc.usePrimary && fc.primary != nil {
		values, err := fc.primary.GetMulti(ctx, keys)
		if err == nil {
			return values, nil
		}
		fc.logger.Warn("primary cache get multi failed, trying fallback", "error", err)
	}

	return fc.fallback.GetMulti(ctx, keys)
}

// SetMulti stores multiple values in both caches
func (fc *FallbackCache) SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	// Try primary
	if fc.usePrimary && fc.primary != nil {
		if err := fc.primary.SetMulti(ctx, items, ttl); err != nil {
			fc.logger.Warn("primary cache set multi failed", "error", err)
		}
	}

	// Always update fallback
	return fc.fallback.SetMulti(ctx, items, ttl)
}

// DeleteMulti removes multiple values from both caches
func (fc *FallbackCache) DeleteMulti(ctx context.Context, keys []string) error {
	// Delete from primary
	if fc.usePrimary && fc.primary != nil {
		if err := fc.primary.DeleteMulti(ctx, keys); err != nil {
			fc.logger.Warn("primary cache delete multi failed", "error", err)
		}
	}

	// Delete from fallback
	return fc.fallback.DeleteMulti(ctx, keys)
}

// Increment increments a numeric value
func (fc *FallbackCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	if fc.usePrimary && fc.primary != nil {
		value, err := fc.primary.Increment(ctx, key, delta)
		if err == nil {
			return value, nil
		}
		fc.logger.Warn("primary cache increment failed, trying fallback", "error", err, "key", key)
	}

	return fc.fallback.Increment(ctx, key, delta)
}

// Decrement decrements a numeric value
func (fc *FallbackCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	if fc.usePrimary && fc.primary != nil {
		value, err := fc.primary.Decrement(ctx, key, delta)
		if err == nil {
			return value, nil
		}
		fc.logger.Warn("primary cache decrement failed, trying fallback", "error", err, "key", key)
	}

	return fc.fallback.Decrement(ctx, key, delta)
}

// TTL returns the remaining time to live for a key
func (fc *FallbackCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if fc.usePrimary && fc.primary != nil {
		ttl, err := fc.primary.TTL(ctx, key)
		if err == nil {
			return ttl, nil
		}
		fc.logger.Warn("primary cache ttl failed, trying fallback", "error", err, "key", key)
	}

	return fc.fallback.TTL(ctx, key)
}

// Expire sets a new TTL for an existing key
func (fc *FallbackCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	// Try primary
	if fc.usePrimary && fc.primary != nil {
		if err := fc.primary.Expire(ctx, key, ttl); err != nil {
			fc.logger.Warn("primary cache expire failed", "error", err, "key", key)
		}
	}

	// Always update fallback
	return fc.fallback.Expire(ctx, key, ttl)
}

// Ping checks if the primary cache is accessible
func (fc *FallbackCache) Ping(ctx context.Context) error {
	if fc.usePrimary && fc.primary != nil {
		return fc.primary.Ping(ctx)
	}
	return fc.fallback.Ping(ctx)
}

// Close closes both cache connections
func (fc *FallbackCache) Close() error {
	if fc.primary != nil {
		fc.primary.Close()
	}
	return fc.fallback.Close()
}
