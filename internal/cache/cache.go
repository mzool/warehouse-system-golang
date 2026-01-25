package cache

import (
	"context"
	"time"
)

// Cache defines the interface for all cache implementations
type Cache interface {
	// Get retrieves a value from the cache
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value in the cache with optional TTL (0 = no expiration)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a value from the cache
	Delete(ctx context.Context, key string) error

	// Exists checks if a key exists in the cache
	Exists(ctx context.Context, key string) (bool, error)

	// Clear removes all entries from the cache
	Clear(ctx context.Context) error

	// Keys returns all keys matching the pattern (use "*" for all keys)
	Keys(ctx context.Context, pattern string) ([]string, error)

	// GetMulti retrieves multiple values from the cache
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)

	// SetMulti stores multiple values in the cache
	SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error

	// DeleteMulti removes multiple values from the cache
	DeleteMulti(ctx context.Context, keys []string) error

	// Increment increments a numeric value
	Increment(ctx context.Context, key string, delta int64) (int64, error)

	// Decrement decrements a numeric value
	Decrement(ctx context.Context, key string, delta int64) (int64, error)

	// TTL returns the remaining time to live for a key
	TTL(ctx context.Context, key string) (time.Duration, error)

	// Expire sets a new TTL for an existing key
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// Ping checks if the cache is accessible
	Ping(ctx context.Context) error

	// Close closes the cache connection
	Close() error
}

// Config holds common cache configuration
type Config struct {
	// Default TTL for cache entries (0 = no expiration)
	DefaultTTL time.Duration

	// Key prefix for all cache keys
	Prefix string

	// Enable/disable cache (useful for testing)
	Enabled bool
}

// DefaultConfig returns a default cache configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultTTL: 5 * time.Minute,
		Prefix:     "goengine:",
		Enabled:    true,
	}
}

// CacheError represents a cache operation error
type CacheError struct {
	Op      string // Operation that failed
	Key     string // Cache key involved
	Err     error  // Underlying error
	Retried bool   // Whether operation was retried
}

func (e *CacheError) Error() string {
	if e.Retried {
		return "cache " + e.Op + " failed (retried): " + e.Err.Error()
	}
	return "cache " + e.Op + " failed: " + e.Err.Error()
}

func (e *CacheError) Unwrap() error {
	return e.Err
}

// Common cache errors
var (
	ErrCacheNotFound    = &CacheError{Op: "get", Err: errKeyNotFound}
	ErrCacheMiss        = &CacheError{Op: "get", Err: errKeyNotFound}
	ErrCacheUnavailable = &CacheError{Op: "connection", Err: errUnavailable}
	ErrCacheDisabled    = &CacheError{Op: "operation", Err: errDisabled}
)

var (
	errKeyNotFound = customError("key not found")
	errUnavailable = customError("cache unavailable")
	errDisabled    = customError("cache disabled")
)

type customError string

func (e customError) Error() string {
	return string(e)
}
