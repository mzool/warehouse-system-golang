package middlewares

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"
)

// RecoveryConfig holds configuration for recovery middleware
type RecoveryConfig struct {
	// Logger for structured logging (optional, uses slog.Default if nil)
	Logger *slog.Logger

	// Skipper defines a function to skip middleware
	Skipper func(r *http.Request) bool

	// DisableStackTrace disables stack trace in panic recovery
	// Default: false
	DisableStackTrace bool

	// DisablePrintStack disables printing stack trace to stderr
	// Default: false
	DisablePrintStack bool

	// Recovery function that handles the panic
	RecoveryHandler func(w http.ResponseWriter, r *http.Request, err interface{}, stack []byte)

	// Development mode provides more detailed error responses
	// Default: false (should be true only in development)
	Development bool
}

// PanicInfo contains information about a panic
type PanicInfo struct {
	Timestamp   string `json:"timestamp"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	ClientIP    string `json:"client_ip"`
	UserAgent   string `json:"user_agent,omitempty"`
	Error       string `json:"error"`
	Stack       string `json:"stack,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	Development bool   `json:"development,omitempty"`
}

// DefaultRecoveryConfig returns a default recovery configuration
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		Logger:            nil, // Will use slog.Default()
		Skipper:           nil,
		DisableStackTrace: false,
		DisablePrintStack: false,
		RecoveryHandler:   defaultRecoveryHandler,
		Development:       false,
	}
}

// defaultRecoveryHandler is the default recovery handler
func defaultRecoveryHandler(w http.ResponseWriter, r *http.Request, err interface{}, stack []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	response := map[string]interface{}{
		"error":   "Internal Server Error",
		"message": "An unexpected error occurred",
	}

	json.NewEncoder(w).Encode(response)
}

// developmentRecoveryHandler provides detailed error information for development
func developmentRecoveryHandler(w http.ResponseWriter, r *http.Request, err interface{}, stack []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	response := map[string]interface{}{
		"error":      "Internal Server Error",
		"message":    fmt.Sprintf("Panic: %v", err),
		"stack":      string(stack),
		"method":     r.Method,
		"path":       r.URL.Path,
		"timestamp":  time.Now().Format(time.RFC3339),
		"request_id": r.Header.Get("X-Request-ID"),
	}

	json.NewEncoder(w).Encode(response)
}

// Recovery returns a recovery middleware that recovers from panics
func Recovery(config *RecoveryConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	// Set defaults
	if config.RecoveryHandler == nil {
		if config.Development {
			config.RecoveryHandler = developmentRecoveryHandler
		} else {
			config.RecoveryHandler = defaultRecoveryHandler
		}
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("recovery middleware initialized",
		"development", config.Development,
		"disable_stack_trace", config.DisableStackTrace,
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				logger.Debug("recovery skipped",
					"method", r.Method,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			defer func() {
				if err := recover(); err != nil {
					var stack []byte
					if !config.DisableStackTrace {
						stack = debug.Stack()
					}

					// Log the panic with structured logging
					logAttrs := []any{
						"method", r.Method,
						"path", r.URL.Path,
						"client_ip", getClientIP(r),
						"user_agent", r.UserAgent(),
						"error", fmt.Sprintf("%v", err),
					}

					if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
						logAttrs = append(logAttrs, "request_id", requestID)
					}

					if !config.DisableStackTrace {
						logAttrs = append(logAttrs, "stack", string(stack))
					}

					logger.Error("panic recovered", logAttrs...)
					logger.Error("panic recovered", logAttrs...)

					// Call recovery handler
					config.RecoveryHandler(w, r, err, stack)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// logPanic is kept for backward compatibility but now uses slog internally
// Deprecated: Use slog.Logger directly instead
func logPanic(logger *slog.Logger, format string, panicInfo *PanicInfo) {
	if logger == nil {
		logger = slog.Default()
	}

	logAttrs := []any{
		"timestamp", panicInfo.Timestamp,
		"method", panicInfo.Method,
		"path", panicInfo.Path,
		"client_ip", panicInfo.ClientIP,
		"error", panicInfo.Error,
	}

	if panicInfo.UserAgent != "" {
		logAttrs = append(logAttrs, "user_agent", panicInfo.UserAgent)
	}
	if panicInfo.RequestID != "" {
		logAttrs = append(logAttrs, "request_id", panicInfo.RequestID)
	}
	if panicInfo.Stack != "" {
		logAttrs = append(logAttrs, "stack", panicInfo.Stack)
	}
	if panicInfo.Development {
		logAttrs = append(logAttrs, "development", true)
	}

	logger.Error("panic", logAttrs...)
}

// ProductionRecoveryConfig returns a production-ready recovery configuration
func ProductionRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		Logger:            nil, // Will use slog.Default()
		Skipper:           nil,
		DisableStackTrace: false,
		DisablePrintStack: true, // Don't print to stderr in production
		RecoveryHandler:   productionRecoveryHandler,
		Development:       false,
	}
}

// productionRecoveryHandler provides minimal error information for production
func productionRecoveryHandler(w http.ResponseWriter, r *http.Request, err interface{}, stack []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	response := map[string]interface{}{
		"error":      "Internal Server Error",
		"message":    "An unexpected error occurred. Please try again later.",
		"timestamp":  time.Now().Unix(),
		"request_id": r.Header.Get("X-Request-ID"),
	}

	json.NewEncoder(w).Encode(response)
}

// DevelopmentRecoveryConfig returns a development-friendly recovery configuration
func DevelopmentRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		Logger:            nil, // Will use slog.Default()
		Skipper:           nil,
		DisableStackTrace: false,
		DisablePrintStack: false,
		RecoveryHandler:   developmentRecoveryHandler,
		Development:       true,
	}
}

// WithPanicDetails adds additional context to panic logs
func WithPanicDetails(details map[string]interface{}) func(*RecoveryConfig) {
	return func(config *RecoveryConfig) {
		originalHandler := config.RecoveryHandler
		config.RecoveryHandler = func(w http.ResponseWriter, r *http.Request, err interface{}, stack []byte) {
			// Log additional details with slog
			logger := config.Logger
			if logger == nil {
				logger = slog.Default()
			}

			logAttrs := []any{}
			for k, v := range details {
				logAttrs = append(logAttrs, k, v)
			}
			logger.Info("additional panic details", logAttrs...)

			// Call original handler
			originalHandler(w, r, err, stack)
		}
	}
}

// GetGoroutineCount returns the current number of goroutines
func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}

// GetMemStats returns current memory statistics
func GetMemStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// HealthAwareRecoveryHandler creates a recovery handler that includes system health info
func HealthAwareRecoveryHandler(includeSystemInfo bool) func(w http.ResponseWriter, r *http.Request, err interface{}, stack []byte) {
	return func(w http.ResponseWriter, r *http.Request, err interface{}, stack []byte) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		response := map[string]interface{}{
			"error":      "Internal Server Error",
			"message":    "An unexpected error occurred",
			"timestamp":  time.Now().Unix(),
			"request_id": r.Header.Get("X-Request-ID"),
		}

		if includeSystemInfo {
			memStats := GetMemStats()
			response["system_info"] = map[string]interface{}{
				"goroutines":   GetGoroutineCount(),
				"memory_alloc": memStats.Alloc,
				"memory_sys":   memStats.Sys,
				"gc_cycles":    memStats.NumGC,
			}
		}

		json.NewEncoder(w).Encode(response)
	}
}
