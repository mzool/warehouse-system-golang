# Observability Package

Production-ready observability tools for monitoring, tracing, and metrics.

## Features

- **Request ID Middleware** - Unique IDs for request tracing
- **Health Checks** - `/health`, `/ready`, `/live` endpoints
- **Prometheus Metrics** - Comprehensive HTTP and business metrics
- **Structured Logging** - Integrated with slog

## Components

### 1. Request ID

Adds unique request IDs to every request for distributed tracing.

```go
import "your-module/internal/observability"

// Basic usage
router.Use(observability.RequestID(nil))

// Custom configuration
requestIDConfig := &observability.RequestIDConfig{
    Logger: logger,
    Header: "X-Trace-ID",
    Generator: func() string {
        return uuid.New().String()
    },
}
router.Use(observability.RequestID(requestIDConfig))

// Get request ID in handlers
func MyHandler(w http.ResponseWriter, r *http.Request) {
    requestID := observability.GetRequestIDFromRequest(r)
    log.Info("processing request", "request_id", requestID)
}
```

### 2. Health Checks

Kubernetes-compatible health check endpoints.

```go
healthConfig := &observability.HealthConfig{
    Logger:            logger,
    DatabasePool:      dbPool,
    CheckTimeout:      5 * time.Second,
    IncludeSystemInfo: true,
    IncludeDetails:    true,
}

// Register endpoints
router.HandleFunc("/health", observability.HealthHandler(healthConfig))
router.HandleFunc("/ready", observability.ReadinessHandler(healthConfig))
router.HandleFunc("/live", observability.LivenessHandler(healthConfig))

// Add custom health checks
observability.RegisterHealthChecks(healthConfig, map[string]observability.HealthCheck{
    "redis": observability.RedisHealthCheck(redisClient.Ping),
    "api": observability.URLHealthCheck("https://api.example.com/health", nil),
})
```

**Endpoints:**

- `GET /health` - Comprehensive health status with all checks
- `GET /ready` - Readiness probe (can app accept traffic?)
- `GET /live` - Liveness probe (is app alive?)

**Response Example:**

```json
{
  "status": "healthy",
  "timestamp": "2025-12-20T10:30:00Z",
  "uptime": "2h15m30s",
  "version": "1.0.0",
  "checks": {
    "database": {
      "status": "healthy",
      "message": "Database is healthy",
      "latency": "5ms"
    },
    "redis": {
      "status": "healthy",
      "message": "Redis is healthy",
      "latency": "2ms"
    }
  },
  "system": {
    "goroutines": 42,
    "memory_alloc_mb": 128,
    "memory_sys_mb": 256,
    "num_cpu": 8,
    "num_gc": 15
  }
}
```

### 3. Prometheus Metrics

Automatic HTTP metrics collection.

```go
metricsConfig := observability.DefaultMetricsConfig("myapp")
metricsConfig.Logger = logger

metrics := observability.NewMetrics(metricsConfig)

// Apply middleware
router.Use(metrics.Middleware(metricsConfig))

// Expose metrics endpoint
router.Handle("/metrics", observability.MetricsHandler())
```

**Collected Metrics:**

- `http_requests_total` - Total HTTP requests (method, path, status)
- `http_request_duration_seconds` - Request latency histogram
- `http_request_size_bytes` - Request body size
- `http_response_size_bytes` - Response body size
- `http_requests_active` - Active concurrent requests

**Custom Business Metrics:**

```go
// Database metrics
dbMetrics := observability.NewDatabaseMetrics("myapp")

// In your database layer
start := time.Now()
result, err := db.Query(ctx, "SELECT * FROM users")
dbMetrics.QueryDuration.WithLabelValues("get_users", "success").Observe(time.Since(start).Seconds())

// Cache metrics
cacheMetrics := observability.NewCacheMetrics("myapp")

// In your cache layer
start := time.Now()
value, found := cache.Get("user:123")
if found {
    cacheMetrics.Hits.WithLabelValues("redis").Inc()
} else {
    cacheMetrics.Misses.WithLabelValues("redis").Inc()
}
cacheMetrics.GetDuration.WithLabelValues("redis").Observe(time.Since(start).Seconds())
```

## Complete Example

```go
package main

import (
    "log/slog"
    "net/http"
    "os"
    
    "your-module/internal/config"
    "your-module/internal/observability"
    "your-module/internal/router"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    
    // Load config
    cfg, err := config.LoadConfig(logger)
    if err != nil {
        logger.Error("failed to load config", "error", err)
        os.Exit(1)
    }
    
    // Set version for health checks
    observability.SetVersion(cfg.App.Version)
    
    // Database pool
    dbConfig := config.DefaultDBConfig(cfg.Database.URL)
    dbPool, err := config.NewPool(dbConfig)
    if err != nil {
        logger.Error("failed to connect to database", "error", err)
        os.Exit(1)
    }
    defer dbPool.Close()
    
    // Create router
    r := router.NewRouter()
    
    // Observability middleware
    r.Use(observability.RequestID(nil))
    
    metricsConfig := observability.DefaultMetricsConfig(cfg.App.Version)
    metricsConfig.Logger = logger
    metrics := observability.NewMetrics(metricsConfig)
    r.Use(metrics.Middleware(metricsConfig))
    
    // Health check endpoints
    healthConfig := &observability.HealthConfig{
        Logger:            logger,
        DatabasePool:      dbPool,
        IncludeSystemInfo: true,
        IncludeDetails:    cfg.IsDevelopment(),
    }
    
    r.HandleFunc("/health", observability.HealthHandler(healthConfig))
    r.HandleFunc("/ready", observability.ReadinessHandler(healthConfig))
    r.HandleFunc("/live", observability.LivenessHandler(healthConfig))
    r.Handle("/metrics", observability.MetricsHandler())
    
    // Your application routes
    r.HandleFunc("/api/users", GetUsersHandler)
    
    logger.Info("server starting",
        "port", cfg.Server.Port,
        "version", cfg.App.Version,
    )
    
    if err := http.ListenAndServe(":"+cfg.Server.Port, r); err != nil {
        logger.Error("server failed", "error", err)
        os.Exit(1)
    }
}
```

## Monitoring Setup

### Prometheus Configuration

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'myapp'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

### Grafana Dashboard

Example queries:

```promql
# Request rate
rate(myapp_http_requests_total[5m])

# Error rate
rate(myapp_http_requests_total{status=~"5.."}[5m])

# P95 latency
histogram_quantile(0.95, rate(myapp_http_request_duration_seconds_bucket[5m]))

# Active connections
myapp_http_requests_active
```

### Kubernetes Probes

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: myapp
    livenessProbe:
      httpGet:
        path: /live
        port: 8080
      initialDelaySeconds: 30
      periodSeconds: 10
    readinessProbe:
      httpGet:
        path: /ready
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 5
```

## Best Practices

1. **Always use request IDs** - Makes debugging production issues much easier
2. **Set appropriate health check timeouts** - Don't let checks hang
3. **Don't expose sensitive data in health checks** - Especially in production
4. **Use custom health checks for critical dependencies** - Database, cache, external APIs
5. **Monitor the /metrics endpoint regularly** - Set up alerts for anomalies
6. **Include version in health response** - Helps track deployments
7. **Use histogram buckets appropriate for your latency** - Adjust based on your SLAs

## Dependencies

- `github.com/prometheus/client_golang` - Prometheus client
