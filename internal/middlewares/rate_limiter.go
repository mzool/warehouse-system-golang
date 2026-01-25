package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"warehouse_system/internal/cache"
)

// RateLimitConfig holds configuration for rate limiting middleware using token bucket algorithm
type RateLimitConfig struct {
	// Cache system for storing rate limit state
	// If nil, uses in-memory store (not recommended for distributed systems)
	Cache cache.Cache

	// Logger for structured logging (optional, uses slog.Default if nil)
	Logger *slog.Logger

	// Capacity is the maximum number of tokens in the bucket
	// Default: 10
	Capacity int

	// RefillRate is the number of tokens added per second
	// Default: 5.0 (5 tokens per second)
	RefillRate float64

	// Message to return when rate limit is exceeded
	// Default: "Rate limit exceeded"
	Message string

	// Status code to return when rate limit is exceeded
	// Default: 429 (Too Many Requests)
	StatusCode int

	// Headers to include in rate limit response
	Headers map[string]string

	// KeyGenerator generates the key for rate limiting
	// Default: uses client IP
	KeyGenerator func(r *http.Request) string

	// Store defines the storage mechanism for rate limiting
	// Default: in-memory store
	Store TokenBucketStore

	// Skipper defines a function to skip middleware
	Skipper func(r *http.Request) bool

	// OnLimitReached is called when rate limit is exceeded
	OnLimitReached func(r *http.Request, key string)
}

// TokenBucket represents a token bucket state
type TokenBucket struct {
	Tokens     float64   // Current number of tokens
	LastRefill time.Time // Last time tokens were refilled
	Capacity   int       // Maximum tokens
	RefillRate float64   // Tokens added per second
}

// TokenBucketStore defines the interface for token bucket storage
type TokenBucketStore interface {
	// Allow checks if a request is allowed and updates the bucket
	Allow(ctx context.Context, key string, capacity int, refillRate float64) (allowed bool, remaining int, retryAfter time.Duration, err error)
	// Reset resets the bucket for a key
	Reset(ctx context.Context, key string) error
}

// MemoryTokenBucketStore implements an in-memory token bucket store
type MemoryTokenBucketStore struct {
	mu      sync.RWMutex
	buckets map[string]*TokenBucket
}

// NewMemoryTokenBucketStore creates a new in-memory token bucket store
func NewMemoryTokenBucketStore() *MemoryTokenBucketStore {
	store := &MemoryTokenBucketStore{
		buckets: make(map[string]*TokenBucket),
	}

	// Start cleanup goroutine to remove old buckets
	go store.cleanup()

	return store
}

// Allow checks if a request is allowed using token bucket algorithm
func (m *MemoryTokenBucketStore) Allow(ctx context.Context, key string, capacity int, refillRate float64) (bool, int, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	bucket, exists := m.buckets[key]

	// Initialize new bucket if it doesn't exist
	if !exists {
		bucket = &TokenBucket{
			Tokens:     float64(capacity),
			LastRefill: now,
			Capacity:   capacity,
			RefillRate: refillRate,
		}
		m.buckets[key] = bucket
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(bucket.LastRefill).Seconds()
	tokensToAdd := elapsed * refillRate
	bucket.Tokens = float64(min(int(capacity), int(bucket.Tokens+tokensToAdd)))
	bucket.LastRefill = now

	// Check if we have at least 1 token
	if bucket.Tokens >= 1.0 {
		bucket.Tokens -= 1.0
		remaining := int(bucket.Tokens)
		return true, remaining, 0, nil
	}

	// Calculate retry after duration
	tokensNeeded := 1.0 - bucket.Tokens
	retryAfter := time.Duration(tokensNeeded/refillRate) * time.Second

	return false, 0, retryAfter, nil
}

// Reset resets the bucket for a key
func (m *MemoryTokenBucketStore) Reset(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.buckets, key)
	return nil
}

// cleanup removes buckets that haven't been accessed in a while
func (m *MemoryTokenBucketStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		m.mu.Lock()
		for key, bucket := range m.buckets {
			// Remove buckets not accessed in the last 10 minutes
			if now.Sub(bucket.LastRefill) > 10*time.Minute {
				delete(m.buckets, key)
			}
		}
		m.mu.Unlock()
	}
}

// CacheTokenBucketStore implements a cache-backed token bucket store
type CacheTokenBucketStore struct {
	cache     cache.Cache
	keyPrefix string
}

// NewCacheTokenBucketStore creates a new cache token bucket store
func NewCacheTokenBucketStore(c cache.Cache, keyPrefix string) *CacheTokenBucketStore {
	if keyPrefix == "" {
		keyPrefix = "rate_limit:"
	}
	return &CacheTokenBucketStore{
		cache:     c,
		keyPrefix: keyPrefix,
	}
}

// Allow checks if a request is allowed using token bucket algorithm with cache
func (c *CacheTokenBucketStore) Allow(ctx context.Context, key string, capacity int, refillRate float64) (bool, int, time.Duration, error) {
	fullKey := c.keyPrefix + key
	now := time.Now()

	// Get current bucket state from cache
	data, err := c.cache.Get(ctx, fullKey)
	var bucket *TokenBucket

	if err != nil || data == nil {
		// Initialize new bucket
		bucket = &TokenBucket{
			Tokens:     float64(capacity),
			LastRefill: now,
			Capacity:   capacity,
			RefillRate: refillRate,
		}
	} else {
		// Deserialize existing bucket
		bucket = &TokenBucket{}
		if err := json.Unmarshal(data, bucket); err != nil {
			return false, 0, 0, fmt.Errorf("failed to unmarshal bucket: %w", err)
		}
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(bucket.LastRefill).Seconds()
	tokensToAdd := elapsed * refillRate
	bucket.Tokens = float64(min(capacity, int(bucket.Tokens+tokensToAdd)))
	bucket.LastRefill = now

	// Check if we have at least 1 token
	allowed := bucket.Tokens >= 1.0
	if allowed {
		bucket.Tokens -= 1.0
	}

	remaining := int(bucket.Tokens)

	// Calculate retry after duration
	var retryAfter time.Duration
	if !allowed {
		tokensNeeded := 1.0 - bucket.Tokens
		retryAfter = time.Duration(tokensNeeded/refillRate) * time.Second
	}

	// Save updated bucket state
	// TTL: keep key alive for twice the time it takes to fully refill the bucket
	ttl := time.Duration(capacity/int(refillRate)) * 2 * time.Second
	if ttl < time.Minute {
		ttl = time.Minute // Minimum 1 minute
	}

	bucketData, err := json.Marshal(bucket)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to marshal bucket: %w", err)
	}

	if err := c.cache.Set(ctx, fullKey, bucketData, ttl); err != nil {
		return false, 0, 0, fmt.Errorf("failed to save bucket: %w", err)
	}

	return allowed, remaining, retryAfter, nil
}

// Reset resets the bucket for a key
func (c *CacheTokenBucketStore) Reset(ctx context.Context, key string) error {
	fullKey := c.keyPrefix + key
	return c.cache.Delete(ctx, fullKey)
}

// DefaultRateLimitConfig returns a default token bucket rate limit configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Capacity:   10,
		RefillRate: 1.0,
		Message:    "Rate limit exceeded",
		StatusCode: http.StatusTooManyRequests,
		Headers: map[string]string{
			"Content-Type": "application/json; charset=utf-8",
		},
		KeyGenerator:   defaultKeyGenerator,
		Store:          NewMemoryTokenBucketStore(),
		Skipper:        nil,
		OnLimitReached: nil,
	}
}

// defaultKeyGenerator generates a key based on client IP
func defaultKeyGenerator(r *http.Request) string {
	ip := getClientIP(r)
	return fmt.Sprintf("rate_limit:%s", ip)
}

// RateLimit returns a token bucket rate limiting middleware
func RateLimit(config *RateLimitConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	// Set defaults
	if config.Store == nil {
		if config.Cache != nil {
			// Use cache-backed store if cache is provided
			config.Store = NewCacheTokenBucketStore(config.Cache, "rate_limit:")
		} else {
			// Fallback to in-memory store
			config.Store = NewMemoryTokenBucketStore()
		}
	}
	if config.KeyGenerator == nil {
		config.KeyGenerator = defaultKeyGenerator
	}
	if config.Capacity <= 0 {
		config.Capacity = 10
	}
	if config.RefillRate <= 0 {
		config.RefillRate = 5.0
	}
	if config.StatusCode <= 0 {
		config.StatusCode = http.StatusTooManyRequests
	}
	if config.Message == "" {
		config.Message = "Rate limit exceeded"
	}
	if config.Headers == nil {
		config.Headers = map[string]string{
			"Content-Type": "application/json; charset=utf-8",
		}
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Determine store type for logging
	storeType := "memory"
	if _, ok := config.Store.(*CacheTokenBucketStore); ok {
		storeType = "cache"
	}

	logger.Debug("rate limiter middleware initialized",
		"capacity", config.Capacity,
		"refill_rate", config.RefillRate,
		"store_type", storeType,
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				logger.Debug("rate limiter skipped",
					"method", r.Method,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			key := config.KeyGenerator(r)
			ctx := r.Context()

			// Check if request is allowed using token bucket
			allowed, remaining, retryAfter, err := config.Store.Allow(ctx, key, config.Capacity, config.RefillRate)
			if err != nil {
				logger.Error("rate limiter store error",
					"method", r.Method,
					"path", r.URL.Path,
					"key", key,
					"error", err,
				)
				// Log error but don't block the request
				http.Error(w, "Rate limiter error", http.StatusInternalServerError)
				return
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(config.Capacity))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			// Check if rate limit exceeded
			if !allowed {
				logger.Warn("rate limit exceeded",
					"method", r.Method,
					"path", r.URL.Path,
					"key", key,
					"retry_after_seconds", int(retryAfter.Seconds())+1,
				)

				// Call OnLimitReached callback if set
				if config.OnLimitReached != nil {
					config.OnLimitReached(r, key)
				}

				// Set custom headers
				for k, v := range config.Headers {
					w.Header().Set(k, v)
				}

				// Set retry after header
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))

				w.WriteHeader(config.StatusCode)

				// Send JSON response
				response := map[string]interface{}{
					"error":               "rate_limit_exceeded",
					"message":             config.Message,
					"retry_after_seconds": int(retryAfter.Seconds()) + 1,
				}
				w.Write([]byte(fmt.Sprintf(`{"error":"%s","message":"%s","retry_after_seconds":%d}`,
					response["error"], response["message"], response["retry_after_seconds"])))
				return
			}
			logger.Debug("rate limit check passed",
				"method", r.Method,
				"path", r.URL.Path,
				"key", key,
				"remaining", remaining,
			)
			next.ServeHTTP(w, r)
		})
	}
}

// PerIP creates a token bucket rate limit configuration for per-IP limiting
func PerIP(capacity int, refillRate float64) *RateLimitConfig {
	config := DefaultRateLimitConfig()
	config.Capacity = capacity
	config.RefillRate = refillRate
	return config
}

// PerUser creates a token bucket rate limit configuration for per-user limiting
func PerUser(capacity int, refillRate float64, userIDExtractor func(r *http.Request) string) *RateLimitConfig {
	config := DefaultRateLimitConfig()
	config.Capacity = capacity
	config.RefillRate = refillRate
	config.KeyGenerator = func(r *http.Request) string {
		userID := userIDExtractor(r)
		if userID == "" {
			return defaultKeyGenerator(r) // Fall back to IP-based limiting
		}
		return fmt.Sprintf("rate_limit:user:%s", userID)
	}
	return config
}

// WithCache creates a rate limit configuration using cache storage
func WithCache(c cache.Cache, capacity int, refillRate float64) *RateLimitConfig {
	config := DefaultRateLimitConfig()
	config.Cache = c
	config.Capacity = capacity
	config.RefillRate = refillRate
	config.Store = NewCacheTokenBucketStore(c, "rate_limit:")
	return config
}

// WithMemory creates a rate limit configuration using in-memory storage
func WithMemory(capacity int, refillRate float64) *RateLimitConfig {
	config := DefaultRateLimitConfig()
	config.Capacity = capacity
	config.RefillRate = refillRate
	config.Store = NewMemoryTokenBucketStore()
	return config
}
