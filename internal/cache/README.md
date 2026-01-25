# Cache Package

Production-ready caching layer with Redis primary and in-memory fallback.

## Features

- **Multiple Implementations**: Redis, Memory, and Fallback (Redis + Memory)
- **Cache Interface**: Unified API for all cache backends
- **HTTP Middleware**: Transparent response caching
- **TTL Support**: Per-key and default TTL
- **Pattern Matching**: Wildcard key search and bulk operations
- **Atomic Operations**: Increment/Decrement counters
- **Graceful Degradation**: Automatic fallback to memory on Redis failure
- **Thread-Safe**: All implementations are goroutine-safe
- **Structured Logging**: slog integration for observability

## Cache Implementations

### 1. Memory Cache

In-memory LRU cache with TTL support, perfect for development or as fallback:

```go
import "your-project/internal/cache"

config := cache.DefaultConfig()
config.DefaultTTL = 10 * time.Minute
config.Prefix = "app:"

memCache := cache.NewMemoryCache(config)
defer memCache.Close()

// Set value
err := memCache.Set(ctx, "user:123", []byte("john"), 5*time.Minute)

// Get value
value, err := memCache.Get(ctx, "user:123")

// Delete value
err = memCache.Delete(ctx, "user:123")
```

### 2. Redis Cache

Production-grade Redis cache with connection pooling:

```go
redisConfig := cache.DefaultRedisConfig()
redisConfig.Addr = "localhost:6379"
redisConfig.Password = "secret"
redisConfig.DB = 0
redisConfig.PoolSize = 20
redisConfig.DefaultTTL = 15 * time.Minute

redisCache, err := cache.NewRedisCache(redisConfig)
if err != nil {
    log.Fatal(err)
}
defer redisCache.Close()

// Check connection
err = redisCache.Ping(ctx)
```

### 3. Fallback Cache (Recommended)

Combines Redis with memory fallback for high availability:

```go
fallbackConfig := cache.DefaultFallbackConfig()
fallbackConfig.Redis.Addr = "localhost:6379"
fallbackConfig.AutoFallback = true

fallbackCache, err := cache.NewFallbackCache(fallbackConfig)
if err != nil {
    log.Fatal(err)
}
defer fallbackCache.Close()

// Automatically uses Redis, falls back to memory on error
value, err := fallbackCache.Get(ctx, "key")
```

## HTTP Cache Middleware

Cache HTTP responses transparently:

```go
import (
    "your-project/internal/cache"
    "net/http"
)

// Create cache
fallbackCache, _ := cache.NewFallbackCache(nil)

// Configure middleware
cacheConfig := cache.DefaultCacheMiddlewareConfig()
cacheConfig.Cache = fallbackCache
cacheConfig.DefaultTTL = 5 * time.Minute
cacheConfig.Methods = []string{"GET", "HEAD"}
cacheConfig.StatusCodes = []int{http.StatusOK}
cacheConfig.IncludeQuery = true

// Create middleware
cacheMiddleware := cache.CacheMiddleware(cacheConfig)

// Apply to routes
mux := http.NewServeMux()
mux.Handle("/api/products", cacheMiddleware(http.HandlerFunc(productsHandler)))
```

### Custom Cache Key Generator

```go
cacheConfig.KeyGenerator = func(r *http.Request) string {
    userID := r.Header.Get("X-User-ID")
    return fmt.Sprintf("user:%s:%s:%s", userID, r.Method, r.URL.Path)
}
```

### Skip Caching Conditionally

```go
cacheConfig.Skipper = func(r *http.Request) bool {
    // Don't cache authenticated requests
    return r.Header.Get("Authorization") != ""
}
```

## Cache Operations

### Basic Operations

```go
// Set with default TTL
cache.Set(ctx, "key", []byte("value"), 0)

// Set with custom TTL
cache.Set(ctx, "key", []byte("value"), 10*time.Minute)

// Get value
value, err := cache.Get(ctx, "key")
if err == cache.ErrCacheNotFound {
    // Key not found
}

// Check existence
exists, _ := cache.Exists(ctx, "key")

// Delete key
cache.Delete(ctx, "key")
```

### Bulk Operations

```go
// Get multiple keys
keys := []string{"user:1", "user:2", "user:3"}
values, err := cache.GetMulti(ctx, keys)

// Set multiple keys
items := map[string][]byte{
    "user:1": []byte("alice"),
    "user:2": []byte("bob"),
}
cache.SetMulti(ctx, items, 5*time.Minute)

// Delete multiple keys
cache.DeleteMulti(ctx, keys)
```

### Pattern Operations

```go
// Find all user keys
keys, _ := cache.Keys(ctx, "user:*")

// Delete all session keys
sessionKeys, _ := cache.Keys(ctx, "session:*")
cache.DeleteMulti(ctx, sessionKeys)

// Clear all keys with prefix
cache.Clear(ctx)
```

### Atomic Counters

```go
// Increment counter
newValue, _ := cache.Increment(ctx, "page_views", 1)

// Decrement counter
newValue, _ := cache.Decrement(ctx, "inventory:item:123", 1)

// Increment by custom delta
cache.Increment(ctx, "score", 10)
```

### TTL Management

```go
// Get remaining TTL
ttl, _ := cache.TTL(ctx, "key")
if ttl == -1 {
    // No expiration set
} else if ttl == 0 {
    // Key expired or doesn't exist
}

// Update TTL
cache.Expire(ctx, "key", 30*time.Minute)
```

## Advanced Usage

### Cache-Aside Pattern

```go
func GetUser(ctx context.Context, cache cache.Cache, db *sql.DB, id int) (*User, error) {
    cacheKey := fmt.Sprintf("user:%d", id)
    
    // Try cache first
    cached, err := cache.Get(ctx, cacheKey)
    if err == nil {
        var user User
        json.Unmarshal(cached, &user)
        return &user, nil
    }
    
    // Cache miss - fetch from database
    user, err := fetchUserFromDB(db, id)
    if err != nil {
        return nil, err
    }
    
    // Store in cache for next time
    data, _ := json.Marshal(user)
    cache.Set(ctx, cacheKey, data, 10*time.Minute)
    
    return user, nil
}
```

### Cache Invalidation

```go
// Invalidate specific key
cache.Delete(ctx, "user:123")

// Invalidate by pattern
cache.InvalidateCache(cache, "user:*")

// Invalidate on update
func UpdateUser(cache cache.Cache, db *sql.DB, user *User) error {
    // Update database
    err := db.UpdateUser(user)
    if err != nil {
        return err
    }
    
    // Invalidate cache
    cacheKey := fmt.Sprintf("user:%d", user.ID)
    cache.Delete(context.Background(), cacheKey)
    
    return nil
}
```

### Health Checks

```go
import "your-project/internal/observability"

healthConfig := &observability.HealthConfig{
    CustomChecks: map[string]observability.HealthCheck{
        "cache": func(ctx context.Context) (observability.HealthStatus, string, error) {
            err := cache.Ping(ctx)
            if err != nil {
                return observability.StatusUnhealthy, "Cache unavailable", err
            }
            return observability.StatusHealthy, "Cache is healthy", nil
        },
    },
}
```

## Configuration Best Practices

### Production Redis Configuration

```go
redisConfig := &cache.RedisConfig{
    Config: &cache.Config{
        DefaultTTL: 15 * time.Minute,
        Prefix:     "prod:",
        Enabled:    true,
    },
    Addr:         os.Getenv("REDIS_ADDR"),
    Password:     os.Getenv("REDIS_PASSWORD"),
    DB:           0,
    MaxRetries:   3,
    PoolSize:     50,
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,
}
```

### Development Configuration

```go
// Use memory cache for development
devConfig := cache.DefaultConfig()
devConfig.DefaultTTL = 1 * time.Minute
devConfig.Enabled = true

cache := cache.NewMemoryCache(devConfig)
```

### Testing Configuration

```go
// Disable cache for testing
testConfig := cache.DefaultConfig()
testConfig.Enabled = false

cache := cache.NewMemoryCache(testConfig)
```

## Error Handling

```go
value, err := cache.Get(ctx, "key")
switch {
case err == cache.ErrCacheNotFound:
    // Key doesn't exist - fetch from source
case err == cache.ErrCacheDisabled:
    // Cache is disabled - bypass
case err != nil:
    // Connection or other error
    log.Error("cache error", "error", err)
}
```

## Performance Tips

1. **Use Batch Operations**: Prefer `GetMulti`/`SetMulti` over loops
2. **Set Appropriate TTLs**: Balance freshness vs database load
3. **Use Key Prefixes**: Organize keys logically for easier management
4. **Monitor Cache Hit Rate**: Track hits/misses in metrics
5. **Connection Pooling**: Configure adequate pool size for Redis
6. **Fallback Cache**: Use for high availability in production

## Integration with Observability

```go
import (
    "your-project/internal/cache"
    "your-project/internal/observability"
)

// Track cache metrics
metrics := observability.NewCacheMetrics("main_cache")

// Wrap cache operations
func getCached(ctx context.Context, cache cache.Cache, key string) ([]byte, error) {
    start := time.Now()
    value, err := cache.Get(ctx, key)
    
    if err == nil {
        metrics.RecordHit(time.Since(start))
    } else {
        metrics.RecordMiss(time.Since(start))
    }
    
    return value, err
}
```

## Thread Safety

All cache implementations are thread-safe and can be used concurrently from multiple goroutines:

```go
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        key := fmt.Sprintf("key:%d", id)
        cache.Set(ctx, key, []byte("value"), 0)
    }(i)
}
wg.Wait()
```

## Migration Guide

### From Global Cache to Dependency Injection

**Before:**
```go
// Global cache instance
var globalCache cache.Cache

func GetUser(id int) (*User, error) {
    // Uses global cache
}
```

**After:**
```go
type UserService struct {
    cache cache.Cache
    db    *sql.DB
}

func NewUserService(cache cache.Cache, db *sql.DB) *UserService {
    return &UserService{cache: cache, db: db}
}

func (s *UserService) GetUser(ctx context.Context, id int) (*User, error) {
    // Uses injected cache
}
```

## Common Patterns

### Warm-Up Cache

```go
func WarmUpCache(cache cache.Cache, db *sql.DB) error {
    users, _ := db.GetActiveUsers()
    
    items := make(map[string][]byte)
    for _, user := range users {
        key := fmt.Sprintf("user:%d", user.ID)
        data, _ := json.Marshal(user)
        items[key] = data
    }
    
    return cache.SetMulti(context.Background(), items, 1*time.Hour)
}
```

### Rate Limiting with Cache

```go
func CheckRateLimit(cache cache.Cache, userID string) (bool, error) {
    key := fmt.Sprintf("ratelimit:%s", userID)
    
    count, err := cache.Increment(ctx, key, 1)
    if err != nil {
        return false, err
    }
    
    if count == 1 {
        // Set TTL on first request
        cache.Expire(ctx, key, 1*time.Minute)
    }
    
    return count <= 100, nil // 100 requests per minute
}
```

## License

Part of the goengine framework.
