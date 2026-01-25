package middlewares

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture response details for logging
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseBody *bytes.Buffer
	bytesWritten int64
}

// WriteHeader captures the status code for logging
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures response data and size for logging
func (rw *responseWriter) Write(data []byte) (int, error) {
	// Capture response body if buffer is available
	if rw.responseBody != nil {
		rw.responseBody.Write(data)
	}

	bytesWritten, err := rw.ResponseWriter.Write(data)
	rw.bytesWritten += int64(bytesWritten)

	return bytesWritten, err
}

// Hijack implements the http.Hijacker interface for WebSocket support
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("the ResponseWriter doesn't support Hijacker")
	}
	return hijacker.Hijack()
}

// LoggerConfig holds configuration options for the HTTP request logger middleware
// Follows v2 design: no global dependencies, explicit configuration
type LoggerConfig struct {
	Logger              *slog.Logger // Structured logger instance (stdlib)
	IncludeRequestBody  bool         // Whether to log request body content
	IncludeResponseBody bool         // Whether to log response body content
	MaxBodySize         int64        // Maximum body size to log (prevents memory issues)
	SkipPaths           []string     // Paths to skip logging (e.g., health checks)
	IncludeUserAgent    bool         // Whether to include User-Agent header
	IncludeReferer      bool         // Whether to include Referer header
	IncludeQueryParams  bool         // Whether to include query parameters
}

// DefaultLoggerConfig creates a production-ready logger configuration with sensible defaults
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Logger:              slog.Default(),
		IncludeRequestBody:  false,
		IncludeResponseBody: false,
		MaxBodySize:         4096, // 4KB limit
		SkipPaths:           []string{"/health", "/metrics", "/favicon.ico"},
		IncludeUserAgent:    true,
		IncludeReferer:      false,
		IncludeQueryParams:  true,
	}
}

// Logger creates an HTTP logging middleware that captures request/response details
// Uses stdlib slog for structured logging - no external dependencies
func Logger(config *LoggerConfig) func(http.Handler) http.Handler {
	// Use default configuration if none provided
	if config == nil {
		config = DefaultLoggerConfig()
	}

	// Set defaults
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.MaxBodySize <= 0 {
		config.MaxBodySize = 4096
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for specified paths (health checks, metrics, etc.)
			if shouldSkipPath(r.URL.Path, config.SkipPaths) {
				next.ServeHTTP(w, r)
				return
			}

			startTime := time.Now()

			// Prepare response body capture if enabled
			var responseBodyBuffer *bytes.Buffer
			if config.IncludeResponseBody {
				responseBodyBuffer = &bytes.Buffer{}
			}

			// Create custom response writer to capture response details
			wrappedWriter := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status
				responseBody:   responseBodyBuffer,
			}

			// Capture request body if enabled
			var requestBodyContent string
			if config.IncludeRequestBody && r.Body != nil {
				requestBodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, config.MaxBodySize))
				requestBodyContent = string(requestBodyBytes)
				// Restore the request body for downstream handlers
				r.Body = io.NopCloser(bytes.NewBuffer(requestBodyBytes))
			}

			// Process the request through the next handler
			next.ServeHTTP(wrappedWriter, r)

			// Calculate request processing time
			requestDuration := time.Since(startTime)

			// Build structured log fields
			logFields := buildLogFields(r, wrappedWriter, requestDuration, requestBodyContent, config)

			// Log with appropriate level based on response status
			logRequest(config.Logger, wrappedWriter.statusCode, logFields)
		})
	}
}

// shouldSkipPath checks if the given path should be skipped from logging
func shouldSkipPath(path string, skipPaths []string) bool {
	for _, skipPath := range skipPaths {
		if path == skipPath {
			return true
		}
	}
	return false
}

// buildLogFields creates structured log fields from request and response data
func buildLogFields(r *http.Request, rw *responseWriter, duration time.Duration, requestBody string, config *LoggerConfig) []any {
	fields := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"status", rw.statusCode,
		"latency_ms", duration.Milliseconds(),
		"latency", duration.String(),
		"client_ip", r.RemoteAddr,
		"host", r.Host,
		"response_size", rw.bytesWritten,
	}

	// Add query parameters if enabled
	if config.IncludeQueryParams && len(r.URL.RawQuery) > 0 {
		fields = append(fields, "query", r.URL.RawQuery)
	}

	// Add User-Agent if enabled
	if config.IncludeUserAgent {
		if userAgent := r.Header.Get("User-Agent"); userAgent != "" {
			fields = append(fields, "user_agent", userAgent)
		}
	}

	// Add Referer if enabled
	if config.IncludeReferer {
		if referer := r.Header.Get("Referer"); referer != "" {
			fields = append(fields, "referer", referer)
		}
	}

	// Add request body if enabled and present
	if config.IncludeRequestBody && requestBody != "" {
		fields = append(fields, "request_body", requestBody)
	}

	// Add response body if enabled and captured
	if config.IncludeResponseBody && rw.responseBody != nil {
		responseContent := rw.responseBody.String()
		if len(responseContent) > int(config.MaxBodySize) {
			responseContent = responseContent[:config.MaxBodySize] + "..."
		}
		fields = append(fields, "response_body", responseContent)
	}

	return fields
}

// logRequest logs the request with appropriate level based on status code
func logRequest(logger *slog.Logger, statusCode int, fields []any) {
	switch {
	case statusCode >= 500:
		logger.Error("server error", fields...)
	case statusCode >= 400:
		logger.Warn("client error", fields...)
	default:
		logger.Info("request handled", fields...)
	}
}
