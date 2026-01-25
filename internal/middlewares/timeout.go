package middlewares

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// TimeoutConfig holds configuration for timeout middleware
type TimeoutConfig struct {
	// Logger for structured logging (optional, uses slog.Default if nil)
	Logger *slog.Logger

	// Timeout duration for requests
	// Default: 10 seconds
	Timeout time.Duration

	// Message to return when timeout occurs
	// Default: "Request timeout"
	Message string

	// Status code to return when timeout occurs
	// Default: 408 (Request Timeout)
	StatusCode int

	// Headers to include in timeout response
	Headers map[string]string

	// ErrorHandler handles timeout errors
	// Default: returns JSON error response
	ErrorHandler func(w http.ResponseWriter, r *http.Request)

	// Skipper defines a function to skip middleware
	Skipper func(r *http.Request) bool

	// OnTimeout is called when a timeout occurs
	OnTimeout func(r *http.Request, duration time.Duration)

	// SkipTimeoutForPaths defines paths that should not have timeout applied
	SkipTimeoutForPaths []string
}

// DefaultTimeoutConfig returns a default timeout configuration
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		Timeout:    5 * time.Minute,
		Message:    "Request timeout",
		StatusCode: http.StatusRequestTimeout,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		ErrorHandler:        defaultTimeoutErrorHandler,
		Skipper:             nil,
		OnTimeout:           nil,
		SkipTimeoutForPaths: []string{},
	}
}

// defaultTimeoutErrorHandler is the default timeout error handler
func defaultTimeoutErrorHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestTimeout)

	response := map[string]interface{}{
		"error":     "Request Timeout",
		"message":   "The request took too long to process",
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

// Timeout returns a timeout middleware
func Timeout(config *TimeoutConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultTimeoutConfig()
	}

	// Set defaults
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.StatusCode <= 0 {
		config.StatusCode = http.StatusRequestTimeout
	}
	if config.Message == "" {
		config.Message = "Request timeout"
	}
	if config.Headers == nil {
		config.Headers = map[string]string{
			"Content-Type": "application/json",
		}
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = defaultTimeoutErrorHandler
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("timeout middleware initialized",
		"timeout", config.Timeout.String(),
		"skip_paths_count", len(config.SkipTimeoutForPaths),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				logger.Debug("timeout skipped by skipper",
					"method", r.Method,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			// Skip timeout for specific paths
			for _, path := range config.SkipTimeoutForPaths {
				if r.URL.Path == path {
					logger.Debug("timeout skipped for path",
						"method", r.Method,
						"path", r.URL.Path,
					)
					next.ServeHTTP(w, r)
					return
				}
			}

			// Create context with timeout
			ctx, cancel := context.WithTimeout(r.Context(), config.Timeout)
			defer cancel()

			// Create new request with timeout context
			r = r.WithContext(ctx)

			// Channel to receive completion signal
			done := make(chan struct{})

			// Start time for measuring duration
			start := time.Now()

			// Run the handler in a goroutine
			go func() {
				defer func() {
					if r := recover(); r != nil {
						// Handle panic in the handler
						// The panic will be caught by recovery middleware if present
						panic(r)
					}
					close(done)
				}()
				next.ServeHTTP(w, r)
			}()

			// Wait for either completion or timeout
			select {
			case <-done:
				// Request completed successfully
				duration := time.Since(start)
				logger.Debug("request completed within timeout",
					"method", r.Method,
					"path", r.URL.Path,
					"duration", duration.String(),
					"timeout", config.Timeout.String(),
				)
				return
			case <-ctx.Done():
				// Request timed out
				duration := time.Since(start)

				logger.Warn("request timeout",
					"method", r.Method,
					"path", r.URL.Path,
					"duration", duration.String(),
					"timeout", config.Timeout.String(),
				)

				// Call OnTimeout callback if set
				if config.OnTimeout != nil {
					config.OnTimeout(r, duration)
				}

				// Set custom headers
				for k, v := range config.Headers {
					w.Header().Set(k, v)
				}

				// Call error handler
				config.ErrorHandler(w, r)
				return
			}
		})
	}
}

// FastTimeout creates a timeout middleware with a short timeout (5 seconds)
func FastTimeout() *TimeoutConfig {
	config := DefaultTimeoutConfig()
	config.Timeout = 5 * time.Second
	config.Message = "Request timeout - please try again"
	return config
}

// SlowTimeout creates a timeout middleware with a long timeout (5 minutes)
func SlowTimeout() *TimeoutConfig {
	config := DefaultTimeoutConfig()
	config.Timeout = 5 * time.Minute
	config.Message = "Request timeout - operation took too long"
	return config
}

// APITimeout creates a timeout middleware optimized for API endpoints
func APITimeout(timeout time.Duration) *TimeoutConfig {
	return &TimeoutConfig{
		Timeout:    timeout,
		Message:    "API request timeout",
		StatusCode: http.StatusRequestTimeout,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestTimeout)

			response := map[string]interface{}{
				"success":   false,
				"error":     "timeout",
				"message":   "The API request timed out",
				"timeout":   timeout.String(),
				"timestamp": time.Now().Unix(),
				"path":      r.URL.Path,
				"method":    r.Method,
			}

			json.NewEncoder(w).Encode(response)
		},
		SkipTimeoutForPaths: []string{"/health", "/metrics", "/ping"},
	}
}

// UploadTimeout creates a timeout middleware for file upload endpoints
func UploadTimeout(timeout time.Duration) *TimeoutConfig {
	return &TimeoutConfig{
		Timeout:    timeout,
		Message:    "Upload timeout",
		StatusCode: http.StatusRequestTimeout,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestTimeout)

			response := map[string]interface{}{
				"success": false,
				"error":   "upload_timeout",
				"message": "File upload took too long to complete",
				"timeout": timeout.String(),
			}

			json.NewEncoder(w).Encode(response)
		},
		OnTimeout: func(r *http.Request, duration time.Duration) {
			// Log upload timeout for monitoring
			// log.Printf("Upload timeout: %s %s took %v", r.Method, r.URL.Path, duration)
		},
	}
}

// StreamingTimeout creates a timeout middleware for streaming endpoints
func StreamingTimeout(timeout time.Duration) *TimeoutConfig {
	return &TimeoutConfig{
		Timeout:    timeout,
		Message:    "Streaming timeout",
		StatusCode: http.StatusRequestTimeout,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusRequestTimeout)
			w.Write([]byte("Streaming request timed out"))
		},
	}
}

// BatchProcessTimeout creates a timeout middleware for batch processing endpoints
func BatchProcessTimeout(timeout time.Duration) *TimeoutConfig {
	return &TimeoutConfig{
		Timeout:    timeout,
		Message:    "Batch processing timeout",
		StatusCode: http.StatusRequestTimeout,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestTimeout)

			response := map[string]interface{}{
				"success":    false,
				"error":      "batch_timeout",
				"message":    "Batch processing operation timed out",
				"suggestion": "Try reducing batch size or increasing timeout",
				"timeout":    timeout.String(),
			}

			json.NewEncoder(w).Encode(response)
		},
		OnTimeout: func(r *http.Request, duration time.Duration) {
			// Could trigger batch job retry or notification
			// alerting.NotifyBatchTimeout(r.URL.Path, duration)
		},
	}
}
