package observability

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck represents a health check function
type HealthCheck func(ctx context.Context) (HealthStatus, string, error)

// HealthConfig holds configuration for health check endpoints
type HealthConfig struct {
	// Logger for structured logging
	Logger *slog.Logger

	// Database pool for health checks
	DatabasePool *pgxpool.Pool

	// Custom health checks
	CustomChecks map[string]HealthCheck

	// Timeout for individual checks
	CheckTimeout time.Duration

	// Include system info in response
	IncludeSystemInfo bool

	// Include detailed info (e.g., database stats)
	IncludeDetails bool
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    HealthStatus           `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Uptime    string                 `json:"uptime,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
	System    *SystemInfo            `json:"system,omitempty"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Error   string       `json:"error,omitempty"`
	Latency string       `json:"latency,omitempty"`
}

// SystemInfo contains system-level information
type SystemInfo struct {
	Goroutines  int    `json:"goroutines"`
	MemoryAlloc uint64 `json:"memory_alloc_mb"`
	MemorySys   uint64 `json:"memory_sys_mb"`
	NumCPU      int    `json:"num_cpu"`
	NumGC       uint32 `json:"num_gc"`
}

var (
	startTime = time.Now()
	version   = "1.0.0" // Should be set at build time
)

// DefaultHealthConfig returns a default health configuration
func DefaultHealthConfig() *HealthConfig {
	return &HealthConfig{
		Logger:            nil,
		DatabasePool:      nil,
		CustomChecks:      make(map[string]HealthCheck),
		CheckTimeout:      5 * time.Second,
		IncludeSystemInfo: true,
		IncludeDetails:    false,
	}
}

// SetVersion sets the application version for health checks
func SetVersion(v string) {
	version = v
}

// HealthHandler returns an HTTP handler for comprehensive health checks
// Endpoint: GET /health
func HealthHandler(config *HealthConfig) http.HandlerFunc {
	if config == nil {
		config = DefaultHealthConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), config.CheckTimeout)
		defer cancel()

		response := &HealthResponse{
			Status:    StatusHealthy,
			Timestamp: time.Now().Format(time.RFC3339),
			Uptime:    time.Since(startTime).String(),
			Version:   version,
			Checks:    make(map[string]CheckResult),
		}

		// System info
		if config.IncludeSystemInfo {
			response.System = getSystemInfo()
		}

		// Database health check
		if config.DatabasePool != nil {
			checkResult := checkDatabase(ctx, config.DatabasePool, config.IncludeDetails)
			response.Checks["database"] = checkResult

			if checkResult.Status == StatusUnhealthy {
				response.Status = StatusUnhealthy
			} else if checkResult.Status == StatusDegraded && response.Status == StatusHealthy {
				response.Status = StatusDegraded
			}
		}

		// Custom health checks
		if len(config.CustomChecks) > 0 {
			for name, check := range config.CustomChecks {
				checkResult := runHealthCheck(ctx, check)
				response.Checks[name] = checkResult

				if checkResult.Status == StatusUnhealthy {
					response.Status = StatusUnhealthy
				} else if checkResult.Status == StatusDegraded && response.Status == StatusHealthy {
					response.Status = StatusDegraded
				}
			}
		}

		// Log health check result
		logger.Debug("health check performed",
			"status", response.Status,
			"checks_count", len(response.Checks),
		)

		// Set status code based on health
		statusCode := http.StatusOK
		if response.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		} else if response.Status == StatusDegraded {
			statusCode = http.StatusOK // Still return 200 for degraded
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}

// ReadinessHandler returns an HTTP handler for readiness checks
// Endpoint: GET /ready - Used by load balancers to determine if app can accept traffic
func ReadinessHandler(config *HealthConfig) http.HandlerFunc {
	if config == nil {
		config = DefaultHealthConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), config.CheckTimeout)
		defer cancel()

		// Readiness check focuses on dependencies
		ready := true
		checks := make(map[string]CheckResult)

		// Check database
		if config.DatabasePool != nil {
			checkResult := checkDatabase(ctx, config.DatabasePool, false)
			checks["database"] = checkResult

			if checkResult.Status == StatusUnhealthy {
				ready = false
			}
		}

		// Check custom dependencies
		for name, check := range config.CustomChecks {
			checkResult := runHealthCheck(ctx, check)
			checks[name] = checkResult

			if checkResult.Status == StatusUnhealthy {
				ready = false
			}
		}

		response := map[string]interface{}{
			"ready":     ready,
			"timestamp": time.Now().Format(time.RFC3339),
			"checks":    checks,
		}

		statusCode := http.StatusOK
		if !ready {
			statusCode = http.StatusServiceUnavailable
			logger.Warn("readiness check failed", "checks", checks)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}

// LivenessHandler returns an HTTP handler for liveness checks
// Endpoint: GET /live - Used by orchestrators to determine if app is alive
func LivenessHandler(config *HealthConfig) http.HandlerFunc {
	if config == nil {
		config = DefaultHealthConfig()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Liveness check is simple - is the app running?
		response := map[string]interface{}{
			"alive":     true,
			"timestamp": time.Now().Format(time.RFC3339),
			"uptime":    time.Since(startTime).String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// checkDatabase performs a database health check
func checkDatabase(ctx context.Context, pool *pgxpool.Pool, includeDetails bool) CheckResult {
	start := time.Now()

	err := pool.Ping(ctx)
	latency := time.Since(start)

	if err != nil {
		return CheckResult{
			Status:  StatusUnhealthy,
			Message: "Database connection failed",
			Error:   err.Error(),
			Latency: latency.String(),
		}
	}

	message := "Database is healthy"
	if includeDetails {
		stat := pool.Stat()
		message = "Database is healthy (conns: " +
			"total=" + string(rune(stat.TotalConns())) +
			", idle=" + string(rune(stat.IdleConns())) +
			", acquired=" + string(rune(stat.AcquiredConns())) + ")"
	}

	return CheckResult{
		Status:  StatusHealthy,
		Message: message,
		Latency: latency.String(),
	}
}

// runHealthCheck executes a custom health check with timeout
func runHealthCheck(ctx context.Context, check HealthCheck) CheckResult {
	start := time.Now()

	// Run check with timeout
	resultChan := make(chan CheckResult, 1)
	go func() {
		status, message, err := check(ctx)
		result := CheckResult{
			Status:  status,
			Message: message,
			Latency: time.Since(start).String(),
		}
		if err != nil {
			result.Error = err.Error()
			if result.Status == StatusHealthy {
				result.Status = StatusUnhealthy
			}
		}
		resultChan <- result
	}()

	select {
	case result := <-resultChan:
		return result
	case <-ctx.Done():
		return CheckResult{
			Status:  StatusUnhealthy,
			Message: "Health check timed out",
			Error:   ctx.Err().Error(),
			Latency: time.Since(start).String(),
		}
	}
}

// getSystemInfo retrieves system information
func getSystemInfo() *SystemInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &SystemInfo{
		Goroutines:  runtime.NumGoroutine(),
		MemoryAlloc: m.Alloc / 1024 / 1024, // Convert to MB
		MemorySys:   m.Sys / 1024 / 1024,   // Convert to MB
		NumCPU:      runtime.NumCPU(),
		NumGC:       m.NumGC,
	}
}

// RegisterHealthChecks is a helper to register multiple health checks
func RegisterHealthChecks(config *HealthConfig, checks map[string]HealthCheck) {
	if config.CustomChecks == nil {
		config.CustomChecks = make(map[string]HealthCheck)
	}
	for name, check := range checks {
		config.CustomChecks[name] = check
	}
}

// Example custom health checks

// RedisHealthCheck creates a health check for Redis
func RedisHealthCheck(pingFunc func(context.Context) error) HealthCheck {
	return func(ctx context.Context) (HealthStatus, string, error) {
		if err := pingFunc(ctx); err != nil {
			return StatusUnhealthy, "Redis connection failed", err
		}
		return StatusHealthy, "Redis is healthy", nil
	}
}

// URLHealthCheck creates a health check for external HTTP services
func URLHealthCheck(url string, client *http.Client) HealthCheck {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	return func(ctx context.Context) (HealthStatus, string, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return StatusUnhealthy, "Failed to create request", err
		}

		resp, err := client.Do(req)
		if err != nil {
			return StatusUnhealthy, "Service unreachable", err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return StatusHealthy, "Service is healthy", nil
		}

		return StatusDegraded, "Service returned non-2xx status", nil
	}
}

// ConcurrentHealthChecker runs multiple health checks concurrently
type ConcurrentHealthChecker struct {
	checks map[string]HealthCheck
	mu     sync.RWMutex
}

// NewConcurrentHealthChecker creates a new concurrent health checker
func NewConcurrentHealthChecker() *ConcurrentHealthChecker {
	return &ConcurrentHealthChecker{
		checks: make(map[string]HealthCheck),
	}
}

// Register adds a health check
func (c *ConcurrentHealthChecker) Register(name string, check HealthCheck) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = check
}

// RunAll executes all health checks concurrently
func (c *ConcurrentHealthChecker) RunAll(ctx context.Context) map[string]CheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make(map[string]CheckResult)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, check := range c.checks {
		wg.Add(1)
		go func(n string, chk HealthCheck) {
			defer wg.Done()
			result := runHealthCheck(ctx, chk)
			mu.Lock()
			results[n] = result
			mu.Unlock()
		}(name, check)
	}

	wg.Wait()
	return results
}
