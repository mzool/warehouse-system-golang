package cache

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CacheMiddlewareConfig holds cache middleware configuration
type CacheMiddlewareConfig struct {
	// Cache implementation
	Cache Cache

	// Logger for structured logging
	Logger *slog.Logger

	// Default TTL for cached responses
	DefaultTTL time.Duration

	// Cache only specific methods (default: GET, HEAD)
	Methods []string

	// Skip caching based on request
	Skipper func(r *http.Request) bool

	// Custom key generator
	KeyGenerator func(r *http.Request) string

	// Cache only specific status codes (default: 200)
	StatusCodes []int

	// Include query parameters in cache key
	IncludeQuery bool

	// Include request headers in cache key
	IncludeHeaders []string
}

// DefaultCacheMiddlewareConfig returns a default cache middleware configuration
func DefaultCacheMiddlewareConfig() *CacheMiddlewareConfig {
	return &CacheMiddlewareConfig{
		Cache:          nil,
		Logger:         nil,
		DefaultTTL:     5 * time.Minute,
		Methods:        []string{"GET", "HEAD"},
		Skipper:        nil,
		KeyGenerator:   nil,
		StatusCodes:    []int{http.StatusOK},
		IncludeQuery:   true,
		IncludeHeaders: []string{},
	}
}

// CacheMiddleware returns an HTTP caching middleware
func CacheMiddleware(config *CacheMiddlewareConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultCacheMiddlewareConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if config.Cache == nil {
		logger.Warn("cache middleware initialized without cache, will be disabled")
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	keyGen := config.KeyGenerator
	if keyGen == nil {
		keyGen = defaultKeyGenerator(config)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper returns true
			if config.Skipper != nil && config.Skipper(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Only cache specific methods
			if !contains(config.Methods, r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			// Generate cache key
			cacheKey := keyGen(r)

			// Try to get cached response
			cached, err := config.Cache.Get(r.Context(), cacheKey)
			if err == nil && len(cached) > 0 {
				// Cache hit
				logger.Debug("cache hit", "key", cacheKey, "method", r.Method, "path", r.URL.Path)

				// Parse cached response
				if parsedResp := parseCachedResponse(cached); parsedResp != nil {
					// Write cached headers
					for key, values := range parsedResp.Headers {
						for _, value := range values {
							w.Header().Add(key, value)
						}
					}

					// Add cache headers
					w.Header().Set("X-Cache", "HIT")
					w.Header().Set("X-Cache-Key", cacheKey)

					// Write status code and body
					w.WriteHeader(parsedResp.StatusCode)
					w.Write(parsedResp.Body)
					return
				}
			}

			// Cache miss - proceed with request
			logger.Debug("cache miss", "key", cacheKey, "method", r.Method, "path", r.URL.Path)

			// Create response recorder
			rec := &responseRecorder{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				body:           &bytes.Buffer{},
			}

			// Call next handler
			next.ServeHTTP(rec, r)

			// Add cache headers
			w.Header().Set("X-Cache", "MISS")
			w.Header().Set("X-Cache-Key", cacheKey)

			// Check if response should be cached
			if shouldCache(rec.statusCode, config.StatusCodes) {
				// Serialize response
				cachedResp := &cachedResponse{
					StatusCode: rec.statusCode,
					Headers:    rec.Header(),
					Body:       rec.body.Bytes(),
				}

				serialized := serializeCachedResponse(cachedResp)

				// Store in cache
				if err := config.Cache.Set(r.Context(), cacheKey, serialized, config.DefaultTTL); err != nil {
					logger.Error("failed to cache response", "error", err, "key", cacheKey)
				} else {
					logger.Debug("response cached", "key", cacheKey, "ttl", config.DefaultTTL.String())
				}
			}
		})
	}
}

// responseRecorder captures the response for caching
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (rec *responseRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *responseRecorder) Write(b []byte) (int, error) {
	rec.body.Write(b)
	return rec.ResponseWriter.Write(b)
}

// cachedResponse represents a cached HTTP response
type cachedResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// serializeCachedResponse converts response to bytes
func serializeCachedResponse(resp *cachedResponse) []byte {
	buf := &bytes.Buffer{}

	// Write status code
	buf.WriteString(strconv.Itoa(resp.StatusCode))
	buf.WriteByte('\n')

	// Write headers
	for key, values := range resp.Headers {
		for _, value := range values {
			buf.WriteString(key)
			buf.WriteByte(':')
			buf.WriteString(value)
			buf.WriteByte('\n')
		}
	}

	// Separator
	buf.WriteByte('\n')

	// Write body
	buf.Write(resp.Body)

	return buf.Bytes()
}

// parseCachedResponse converts bytes to response
func parseCachedResponse(data []byte) *cachedResponse {
	reader := bufio.NewReader(bytes.NewReader(data))

	// Read status code
	statusLine, _ := reader.ReadString('\n')
	statusCode, _ := strconv.Atoi(strings.TrimSpace(statusLine))

	// Read headers
	headers := make(http.Header)
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\n" {
			break
		}

		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 {
			headers.Add(parts[0], parts[1])
		}
	}

	// Read body
	body, _ := io.ReadAll(reader)

	return &cachedResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}
}

// defaultKeyGenerator generates cache key from request
func defaultKeyGenerator(config *CacheMiddlewareConfig) func(r *http.Request) string {
	return func(r *http.Request) string {
		h := sha256.New()

		// Method + Path
		h.Write([]byte(r.Method + ":" + r.URL.Path))

		// Query parameters
		if config.IncludeQuery && r.URL.RawQuery != "" {
			h.Write([]byte("?" + r.URL.RawQuery))
		}

		// Headers
		for _, headerName := range config.IncludeHeaders {
			if value := r.Header.Get(headerName); value != "" {
				h.Write([]byte(headerName + ":" + value))
			}
		}

		return "http:" + hex.EncodeToString(h.Sum(nil))
	}
}

// shouldCache checks if response should be cached
func shouldCache(statusCode int, allowedCodes []int) bool {
	for _, code := range allowedCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// contains checks if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// InvalidateCache is a helper to invalidate cache entries by pattern
func InvalidateCache(cache Cache, pattern string) error {
	ctx := context.Background()
	keys, err := cache.Keys(ctx, pattern)
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return cache.DeleteMulti(ctx, keys)
	}

	return nil
}
