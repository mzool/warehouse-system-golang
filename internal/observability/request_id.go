package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey contextKey = "request_id"
)

// RequestIDConfig holds configuration for request ID middleware
type RequestIDConfig struct {
	// Logger for structured logging (optional, uses slog.Default if nil)
	Logger *slog.Logger

	// Header name for request ID
	// Default: X-Request-ID
	Header string

	// Generator function to create request IDs
	// Default: generates random hex string
	Generator func() string

	// Skipper defines a function to skip middleware
	Skipper func(r *http.Request) bool
}

// DefaultRequestIDConfig returns a default request ID configuration
func DefaultRequestIDConfig() *RequestIDConfig {
	return &RequestIDConfig{
		Logger:    nil,
		Header:    "X-Request-ID",
		Generator: defaultRequestIDGenerator,
		Skipper:   nil,
	}
}

// defaultRequestIDGenerator generates a random 16-byte hex string
func defaultRequestIDGenerator() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a simpler method if crypto/rand fails
		return hex.EncodeToString([]byte("fallback-request-id"))
	}
	return hex.EncodeToString(b)
}

// RequestID returns a middleware that adds request ID to context and response headers
func RequestID(config *RequestIDConfig) func(next http.Handler) http.Handler {
	// Use provided config or default
	if config == nil {
		config = DefaultRequestIDConfig()
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Set defaults
	if config.Header == "" {
		config.Header = "X-Request-ID"
	}

	if config.Generator == nil {
		config.Generator = defaultRequestIDGenerator
	}

	logger.Debug("request ID middleware initialized", "header", config.Header)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Check if request ID already exists in header
			requestID := r.Header.Get(config.Header)

			// Generate new request ID
			if requestID == "" {
				requestID = config.Generator()
			}

			// Add request ID to response header
			w.Header().Set(config.Header, requestID)

			// Add request ID to context
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			r = r.WithContext(ctx)

			logger.Debug("request ID assigned",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
			)

			next.ServeHTTP(w, r)
		})
	}
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}

	return ""
}

// GetRequestIDFromRequest retrieves the request ID from HTTP request context
func GetRequestIDFromRequest(r *http.Request) string {
	return GetRequestID(r.Context())
}

// WithRequestID returns a context with the given request ID
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}
