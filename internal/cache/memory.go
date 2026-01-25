package cache

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// MemoryCache implements an in-memory cache with TTL support
type MemoryCache struct {
	config *Config
	items  map[string]*memoryCacheItem
	mu     sync.RWMutex
	stopCh chan struct{}
}

type memoryCacheItem struct {
	value      []byte
	expiration time.Time
	hasExpiry  bool
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(config *Config) *MemoryCache {
	if config == nil {
		config = DefaultConfig()
	}

	mc := &MemoryCache{
		config: config,
		items:  make(map[string]*memoryCacheItem),
		stopCh: make(chan struct{}),
	}

	// Start cleanup goroutine
	go mc.cleanupExpired()

	return mc
}

// Get retrieves a value from the cache
func (mc *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	if !mc.config.Enabled {
		return nil, ErrCacheDisabled
	}

	key = mc.prefixKey(key)

	mc.mu.RLock()
	item, exists := mc.items[key]
	mc.mu.RUnlock()

	if !exists {
		return nil, ErrCacheNotFound
	}

	// Check if item has expired
	if item.hasExpiry && time.Now().After(item.expiration) {
		mc.Delete(ctx, key)
		return nil, ErrCacheNotFound
	}

	return item.value, nil
}

// Set stores a value in the cache with optional TTL
func (mc *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if !mc.config.Enabled {
		return ErrCacheDisabled
	}

	key = mc.prefixKey(key)

	if ttl == 0 {
		ttl = mc.config.DefaultTTL
	}

	item := &memoryCacheItem{
		value:     value,
		hasExpiry: ttl > 0,
	}

	if item.hasExpiry {
		item.expiration = time.Now().Add(ttl)
	}

	mc.mu.Lock()
	mc.items[key] = item
	mc.mu.Unlock()

	return nil
}

// Delete removes a value from the cache
func (mc *MemoryCache) Delete(ctx context.Context, key string) error {
	if !mc.config.Enabled {
		return ErrCacheDisabled
	}

	key = mc.prefixKey(key)

	mc.mu.Lock()
	delete(mc.items, key)
	mc.mu.Unlock()

	return nil
}

// Exists checks if a key exists in the cache
func (mc *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	if !mc.config.Enabled {
		return false, ErrCacheDisabled
	}

	key = mc.prefixKey(key)

	mc.mu.RLock()
	item, exists := mc.items[key]
	mc.mu.RUnlock()

	if !exists {
		return false, nil
	}

	// Check if item has expired
	if item.hasExpiry && time.Now().After(item.expiration) {
		mc.Delete(ctx, key)
		return false, nil
	}

	return true, nil
}

// Clear removes all entries from the cache
func (mc *MemoryCache) Clear(ctx context.Context) error {
	if !mc.config.Enabled {
		return ErrCacheDisabled
	}

	mc.mu.Lock()
	mc.items = make(map[string]*memoryCacheItem)
	mc.mu.Unlock()

	return nil
}

// Keys returns all keys matching the pattern
func (mc *MemoryCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	if !mc.config.Enabled {
		return nil, ErrCacheDisabled
	}

	pattern = mc.prefixKey(pattern)

	mc.mu.RLock()
	defer mc.mu.RUnlock()

	keys := make([]string, 0)
	for key, item := range mc.items {
		// Check if item has expired
		if item.hasExpiry && time.Now().After(item.expiration) {
			continue
		}

		// Match pattern
		matched, _ := filepath.Match(pattern, key)
		if matched {
			keys = append(keys, mc.unprefixKey(key))
		}
	}

	return keys, nil
}

// GetMulti retrieves multiple values from the cache
func (mc *MemoryCache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	if !mc.config.Enabled {
		return nil, ErrCacheDisabled
	}

	result := make(map[string][]byte)

	for _, key := range keys {
		value, err := mc.Get(ctx, key)
		if err == nil {
			result[key] = value
		}
	}

	return result, nil
}

// SetMulti stores multiple values in the cache
func (mc *MemoryCache) SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	if !mc.config.Enabled {
		return ErrCacheDisabled
	}

	for key, value := range items {
		if err := mc.Set(ctx, key, value, ttl); err != nil {
			return err
		}
	}

	return nil
}

// DeleteMulti removes multiple values from the cache
func (mc *MemoryCache) DeleteMulti(ctx context.Context, keys []string) error {
	if !mc.config.Enabled {
		return ErrCacheDisabled
	}

	for _, key := range keys {
		mc.Delete(ctx, key)
	}

	return nil
}

// Increment increments a numeric value
func (mc *MemoryCache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	if !mc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	key = mc.prefixKey(key)

	mc.mu.Lock()
	defer mc.mu.Unlock()

	item, exists := mc.items[key]
	if !exists {
		// Create new item with initial value
		newValue := delta
		mc.items[key] = &memoryCacheItem{
			value:     []byte(strconv.FormatInt(newValue, 10)),
			hasExpiry: false,
		}
		return newValue, nil
	}

	// Parse current value
	currentValue, err := strconv.ParseInt(string(item.value), 10, 64)
	if err != nil {
		return 0, &CacheError{Op: "increment", Key: key, Err: err}
	}

	// Increment
	newValue := currentValue + delta
	item.value = []byte(strconv.FormatInt(newValue, 10))

	return newValue, nil
}

// Decrement decrements a numeric value
func (mc *MemoryCache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return mc.Increment(ctx, key, -delta)
}

// TTL returns the remaining time to live for a key
func (mc *MemoryCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if !mc.config.Enabled {
		return 0, ErrCacheDisabled
	}

	key = mc.prefixKey(key)

	mc.mu.RLock()
	item, exists := mc.items[key]
	mc.mu.RUnlock()

	if !exists {
		return 0, ErrCacheNotFound
	}

	if !item.hasExpiry {
		return -1, nil // No expiration
	}

	ttl := time.Until(item.expiration)
	if ttl < 0 {
		return 0, nil // Expired
	}

	return ttl, nil
}

// Expire sets a new TTL for an existing key
func (mc *MemoryCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if !mc.config.Enabled {
		return ErrCacheDisabled
	}

	key = mc.prefixKey(key)

	mc.mu.Lock()
	defer mc.mu.Unlock()

	item, exists := mc.items[key]
	if !exists {
		return ErrCacheNotFound
	}

	if ttl > 0 {
		item.hasExpiry = true
		item.expiration = time.Now().Add(ttl)
	} else {
		item.hasExpiry = false
	}

	return nil
}

// Ping checks if the cache is accessible
func (mc *MemoryCache) Ping(ctx context.Context) error {
	if !mc.config.Enabled {
		return ErrCacheDisabled
	}
	return nil
}

// Close closes the cache connection
func (mc *MemoryCache) Close() error {
	close(mc.stopCh)
	return nil
}

// cleanupExpired periodically removes expired items
func (mc *MemoryCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.removeExpiredItems()
		case <-mc.stopCh:
			return
		}
	}
}

func (mc *MemoryCache) removeExpiredItems() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	for key, item := range mc.items {
		if item.hasExpiry && now.After(item.expiration) {
			delete(mc.items, key)
		}
	}
}

func (mc *MemoryCache) prefixKey(key string) string {
	if mc.config.Prefix == "" {
		return key
	}
	return mc.config.Prefix + key
}

func (mc *MemoryCache) unprefixKey(key string) string {
	if mc.config.Prefix == "" {
		return key
	}
	if len(key) > len(mc.config.Prefix) {
		return key[len(mc.config.Prefix):]
	}
	return key
}
