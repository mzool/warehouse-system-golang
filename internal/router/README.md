# Router Package - Refactored Version

A high-performance, production-ready HTTP router for Go with multi-tenant SaaS support, dependency injection, structured logging, and middleware support.

## Features

### ✅ Multi-Tenant SaaS Ready
- Domain-based routing for multi-tenant apps
- Tenant context propagation through request lifecycle
- Usage metering hooks for billing/analytics
- Per-domain TLS certificates (SNI support)

### ✅ Production Security
- Built-in panic recovery middleware
- Request body/header size limits
- Trusted proxy validation for IP extraction
- Slowloris attack prevention (ReadHeaderTimeout)
- Request ID generation and propagation

### ✅ Graceful Operations
- Active request tracking during shutdown
- HTTP→HTTPS redirect server management
- Route conflict detection (panic in dev, warn in prod)
- Health check endpoints (liveness/readiness)

### ✅ Dependency Injection
- No global state or config access
- All dependencies injected via constructor
- Easy to test and mock
- Follows SOLID principles

### ✅ Structured Logging (slog)
- Modern structured logging with `log/slog`
- Contextual log entries
- JSON and text formats
- Better observability and monitoring

### ✅ Flexible Middleware Architecture
- Global middlewares applied to all routes
- Per-route middlewares for specific needs
- Route groups with shared middlewares
- Clean separation of concerns

### ✅ Security
- Documentation routes only in dev mode
- Configurable timeouts to prevent slowloris attacks
- Security headers support
- TLS/HTTPS with auto HTTP redirect
- Input validation framework

### ✅ Performance
- Efficient middleware chaining
- Configurable server timeouts
- Proper resource cleanup
- Graceful shutdown support

### ✅ Scalability
- Route groups for organizing large APIs
- Modular architecture
- Optional features (templates, static files)
- Easy to extend

## Installation

```bash
go get yourapp/internal/router
```

## Quick Start

```go
package main

import (
    "log/slog"
    "os"
    "yourapp/internal/router"
)

func main() {
    // 1. Create logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    // 2. Configure router
    config := &router.RouterConfig{
        Version:  "v1",
        BasePath: "/api",
        Port:     "8080",
        Mode:     "dev",
    }

    // 3. Create router with global middlewares
    r := router.NewRouter(config, logger,
        RecoveryMiddleware(logger),
        LoggerMiddleware(logger),
    )

    // 4. Register routes
    r.Register(&router.Route{
        Method:      "GET",
        Path:        "/health",
        HandlerFunc: healthCheck,
    })

    // 5. Start server
    logger.Info("Starting server")
    if err := r.Start(); err != nil {
        logger.Error("Server failed", "error", err)
    }
}
```

## Configuration

### RouterConfig

```go
type RouterConfig struct {
    // Core settings
    Version  string        // API version (e.g., "v1")
    BasePath string        // Base path (e.g., "/api")
    Port     string        // Server port
    Domain   string        // Domain name
    Protocol string        // "http" or "https"
    Mode     string        // "dev" or "prod"

    // Optional features
    TLS       *TLSConfig       // TLS/HTTPS configuration
    Static    *StaticConfig    // Static file serving
    Templates *TemplateConfig  // Template rendering

    // Server timeouts
    ReadTimeout       time.Duration  // Default: 15s
    WriteTimeout      time.Duration  // Default: 15s
    IdleTimeout       time.Duration  // Default: 60s
    ReadHeaderTimeout time.Duration  // Default: 10s (Slowloris prevention)
    
    // Security configuration
    Security *SecurityConfig
    
    // Health check endpoints
    Health *HealthConfig
}
```

### Default Configuration

```go
config := router.DefaultRouterConfig()
// Returns:
// - Version: "v1"
// - BasePath: "/api"
// - Port: "8080"
// - Mode: "dev"
// - Timeouts: 15s/15s/60s
```

## Routes

### Basic Route

```go
r.Register(&router.Route{
    Method:      "GET",
    Path:        "/users",
    Category:    "Users",
    HandlerFunc: listUsers,
    Input: &router.RouteInput{
        QueryParameters: map[string]string{
            "page": "page",
            "limit": "limit",
        },
    },
})
```

### Route with Middleware

```go
r.Register(&router.Route{
    Method:   "POST",
    Path:     "/users",
    Category: "Users",
    Middlewares: []router.MiddlewaresType{
        AuthMiddleware,
        ValidationMiddleware,
    },
    HandlerFunc: createUser,
})
```

### Route Groups

Organize related routes with shared configuration:

```go
adminGroup := &router.RouteGroup{
    Prefix:   "/admin",
    Category: "Admin",
    Middlewares: []router.MiddlewaresType{
        AuthMiddleware,
        AdminOnlyMiddleware,
    },
    Routes: []*router.Route{
        {
            Method:      "GET",
            Path:        "/users",    // Final: /api/v1/admin/users
            HandlerFunc: listAllUsers,
        },
        {
            Method:      "DELETE",
            Path:        "/users/{id}",
            HandlerFunc: deleteUser,
        },
    },
}

r.RegisterGroup(adminGroup)
```

## Middlewares

### Global Middlewares

Applied to ALL routes in order:

```go
r := router.NewRouter(config, logger,
    RecoveryMiddleware(logger),   // 1st: Catches panics
    RateLimitMiddleware(),         // 2nd: Rate limiting
    LoggerMiddleware(logger),      // 3rd: Logs requests
    CORSMiddleware(),              // 4th: CORS headers
)
```

### Per-Route Middlewares

Additional middlewares for specific routes:

```go
r.Register(&router.Route{
    Method: "POST",
    Path:   "/sensitive",
    // Global middlewares + these route-specific ones
    Middlewares: []router.MiddlewaresType{
        ValidationMiddleware,
        StrictRateLimitMiddleware,
    },
    HandlerFunc: sensitiveOperation,
})
```

### Middleware Order

Final execution order:
1. Global middlewares (in order)
2. Route-specific middlewares (in order)
3. Handler function

Example:
```
Recovery -> RateLimit -> Logger -> CORS -> Auth -> Validation -> Handler
└─────────── Global ───────────┘  └──── Route ────┘
```

## Documentation

### Auto-Generated Docs (Dev Mode Only)

Documentation routes are automatically registered in dev mode:

```go
config := &router.RouterConfig{
    Mode: "dev", // Enables documentation
}
```

Access documentation at:
- JSON: `http://localhost:8080/documentation/json`
- HTML: `http://localhost:8080/documentation/html`

In production mode (`Mode: "prod"`), these routes are NOT registered.

### Route Documentation

```go
r.Register(&router.Route{
    Method:   "POST",
    Path:     "/users",
    Category: "Users",
    Input: &router.RouteInput{
        RequiredAuth: true,
        Body: map[string]string{
            "name":  "req_name",
            "email": "req_email",
        },
        Headers: map[string]string{
            "Authorization": "Authorization",
        },
    },
    Response: UserResponse{}, // Your response struct
    HandlerFunc: createUser,
})
```

## Optional Features

### TLS/HTTPS

```go
config := &router.RouterConfig{
    TLS: &router.TLSConfig{
        Enabled:  true,
        CertFile: "/path/to/cert.pem",
        KeyFile:  "/path/to/key.pem",
    },
}
```

Features:
- Automatic HTTP to HTTPS redirect on port 80
- Serves HTTPS on port 443
- Falls back to HTTP if certs not found

### Static File Serving

```go
config := &router.RouterConfig{
    Static: &router.StaticConfig{
        Enabled:   true,
        Dir:       "./public",
        URLPrefix: "/static/",
    },
}
```

Serves files from `./public` at `/static/*`

### Template Rendering

```go
config := &router.RouterConfig{
    Templates: &router.TemplateConfig{
        Enabled: true,
        Dir:     "./templates",
        FuncMap: template.FuncMap{
            "custom": customFunc,
        },
    },
}

// Use in routes
r.Register(&router.Route{
    Method:       "GET",
    Path:         "/page",
    TemplateName: "page.html",
    Input: &router.RouteInput{
        QueryParameters: map[string]string{
            "title": "title",
        },
    },
})
```

## Graceful Shutdown

```go
import (
    "os"
    "os/signal"
    "syscall"
    "time"
)

// Start server in goroutine
go func() {
    if err := r.Start(); err != nil {
        logger.Error("Server error", "error", err)
    }
}()

// Wait for interrupt
stop := make(chan os.Signal, 1)
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
<-stop

// Graceful shutdown with timeout
if err := r.Shutdown(30 * time.Second); err != nil {
    logger.Error("Shutdown failed", "error", err)
}
```

The shutdown process:
1. Signals all handlers to stop accepting new requests
2. Waits for in-flight requests to complete (up to half the timeout)
3. Shuts down the main HTTP server
4. Shuts down the HTTP→HTTPS redirect server (if running)
5. Returns when fully stopped

## Multi-Domain SaaS Features

### Domain-Based Routing

Register different routes for different domains or tenants:

```go
// Register tenant-specific domain
r.RegisterDomain(&router.DomainConfig{
    Domain:   "tenant1.example.com",
    TenantID: "tenant-001",
    BasePath: "/api",
    Version:  "v1",
    Routes: []*router.Route{
        {Method: "GET", Path: "/dashboard", HandlerFunc: tenant1Dashboard},
    },
})

// Wildcard domains for multi-tenant SaaS
r.RegisterDomain(&router.DomainConfig{
    Domain:   "*.myapp.com",          // Matches any subdomain
    TenantID: "",                      // Extracted from subdomain
    BasePath: "/api",
    Middlewares: []router.MiddlewaresType{
        extractTenantMiddleware,       // Extract tenant from subdomain
    },
    Routes: tenantRoutes,
})

// Default domain for unmatched hosts
r.RegisterDomain(&router.DomainConfig{
    Domain:    "api.example.com",
    IsDefault: true,                   // Fallback for unmatched hosts
    Routes:    defaultRoutes,
})
```

### SNI-Based TLS (Per-Domain Certificates)

Serve different SSL certificates for different domains:

```go
config := &router.RouterConfig{
    TLS: &router.TLSConfig{
        Enabled: true,
        DomainCerts: map[string]*router.DomainCertificate{
            "tenant1.example.com": {
                CertFile: "/certs/tenant1.crt",
                KeyFile:  "/certs/tenant1.key",
            },
            "*.myapp.com": {
                CertFile: "/certs/wildcard-myapp.crt",
                KeyFile:  "/certs/wildcard-myapp.key",
            },
        },
    },
}
```

### Context Enrichment

Access domain and tenant information from request context:

```go
func handler(w http.ResponseWriter, req *http.Request) {
    ctx := req.Context()
    
    // Get domain
    domain := router.GetDomain(ctx)           // "tenant1.example.com"
    
    // Get tenant ID
    tenantID := router.GetTenantID(ctx)       // "tenant-001"
    
    // Get request ID (for tracing)
    requestID := router.GetRequestID(ctx)     // "uuid-xxxx-xxxx"
    
    // Get client IP (respects trusted proxies)
    clientIP := router.GetClientIP(ctx)       // "192.168.1.100"
    
    // Get route info
    routeInfo := router.GetRouteInfo(ctx)     // map[method, path, category]
}
```

## Production Security Features

### Security Configuration

```go
config := &router.RouterConfig{
    Mode: "prod",
    ReadHeaderTimeout: 10 * time.Second,  // Prevents Slowloris attacks
    Security: &router.SecurityConfig{
        // Request limits
        MaxRequestBodySize: 10 * 1024 * 1024,  // 10MB max body
        MaxHeaderBytes:     1 * 1024 * 1024,   // 1MB max headers
        
        // Trusted proxies (for X-Forwarded-For)
        TrustedProxies: []string{
            "10.0.0.0/8",        // Private network
            "192.168.0.0/16",    // Private network
            "172.16.0.0/12",     // Private network
        },
        
        // Built-in middlewares
        EnableRecovery:  true,   // Panic recovery (on by default in prod)
        EnableRequestID: true,   // Request ID generation (on by default in prod)
        RequestIDHeader: "X-Request-ID",
    },
}
```

### Built-in Security Middlewares

These middlewares are automatically added in production mode:

1. **Panic Recovery**: Catches panics, logs stack trace, returns 500
2. **Request ID**: Generates UUID for each request, propagates `X-Request-ID`
3. **Body Size Limit**: Rejects oversized requests with 413
4. **Shutdown Aware**: Returns 503 during shutdown, tracks active requests

### Health Check Endpoints

```go
config := &router.RouterConfig{
    Health: &router.HealthConfig{
        Enabled:       true,
        LivenessPath:  "/health/live",   // Kubernetes liveness probe
        ReadinessPath: "/health/ready",  // Kubernetes readiness probe
    },
}

// Register custom health checks
r.RegisterHealthCheck(router.HealthCheck{
    Name:    "database",
    Timeout: 5 * time.Second,
    Check: func(ctx context.Context) error {
        return db.PingContext(ctx)
    },
})

r.RegisterHealthCheck(router.HealthCheck{
    Name:    "redis",
    Timeout: 2 * time.Second,
    Check: func(ctx context.Context) error {
        return redis.Ping(ctx).Err()
    },
})
```

Liveness endpoint (`/health/live`): Always returns 200 if server is running  
Readiness endpoint (`/health/ready`): Runs all health checks, returns 200 only if all pass

## Usage Metering for SaaS

Track request metrics for billing, analytics, or rate limiting:

```go
// Set metering hook
r.SetMeteringHook(func(ctx context.Context, metrics *router.RequestMetrics) {
    // Send to your metrics/billing system
    billing.RecordUsage(metrics.TenantID, billing.Usage{
        Method:       metrics.Method,
        Path:         metrics.Path,
        StatusCode:   metrics.StatusCode,
        Duration:     metrics.Duration,
        RequestSize:  metrics.RequestSize,
        ResponseSize: metrics.ResponseSize,
        Timestamp:    metrics.Timestamp,
    })
    
    // Or send to Prometheus/StatsD
    prometheus.RequestDuration.WithLabelValues(
        metrics.TenantID,
        metrics.Method,
        metrics.Path,
    ).Observe(metrics.Duration.Seconds())
})
```

`RequestMetrics` includes:
- `TenantID`: Tenant identifier from route or domain
- `Domain`: The domain that served the request
- `Method`, `Path`: HTTP method and path
- `StatusCode`: Response status code
- `Duration`: Request processing time
- `RequestSize`, `ResponseSize`: Bytes transferred
- `Timestamp`: When the request started

**Note**: The metering hook runs asynchronously to not block responses.

## Route Conflict Detection

The router detects conflicting routes at registration time:

```go
// In dev mode: panics on conflict (fail fast)
// In prod mode: logs warning and overwrites

r.Register(&router.Route{Method: "GET", Path: "/users"})
r.Register(&router.Route{Method: "GET", Path: "/users"})  // Conflict!
// Dev: panic("route conflict: GET /api/v1/users conflicts with...")
// Prod: WARN "route conflict detected, overwriting"
```

Compiled routes include metadata for debugging:
```go
routes := r.GetRoutes()  // Returns []*CompiledRoute
for _, route := range routes {
    fmt.Printf("%s %s (registered at %s)\n", 
        route.Method, route.FullPattern, route.RegisteredAt)
}
```

## Best Practices

### 1. Dependency Injection

❌ **Bad:** Direct global access
```go
func handler(w http.ResponseWriter, r *http.Request) {
    db := config.AppSettings.Database // Global access
}
```

✅ **Good:** Inject dependencies
```go
func MakeHandler(db *Database, logger *slog.Logger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Use injected db and logger
    }
}
```

### 2. Middleware Creation

❌ **Bad:** Middlewares access global state
```go
func RateLimitMiddleware() MiddlewareType {
    client := cache.RedisClient // Global access
    // ...
}
```

✅ **Good:** Inject dependencies into middleware
```go
func RateLimitMiddleware(redisClient *redis.Client) MiddlewareType {
    return func(next http.Handler) http.Handler {
        // Use injected redisClient
    }
}
```

### 3. Error Handling

❌ **Bad:** Expose internal errors
```go
http.Error(w, err.Error(), 500) // Exposes stack traces
```

✅ **Good:** Log internally, return generic error
```go
logger.Error("Operation failed", "error", err, "user", userID)
http.Error(w, "Internal server error", http.StatusInternalServerError)
```

### 4. Logging

❌ **Bad:** Unstructured logging
```go
fmt.Printf("User %s logged in at %v\n", userID, time.Now())
```

✅ **Good:** Structured logging
```go
logger.Info("User logged in",
    "user_id", userID,
    "timestamp", time.Now(),
    "ip", r.RemoteAddr,
)
```

### 5. Route Organization

For large projects, use route groups:

```go
// api/users/routes.go
func RegisterUserRoutes(r router.Router, deps *Dependencies) {
    group := &router.RouteGroup{
        Prefix:   "/users",
        Category: "Users",
        Middlewares: []router.MiddlewaresType{
            deps.AuthMiddleware,
        },
        Routes: []*router.Route{
            {Method: "GET", Path: "/", HandlerFunc: deps.ListUsers},
            {Method: "POST", Path: "/", HandlerFunc: deps.CreateUser},
            {Method: "GET", Path: "/{id}", HandlerFunc: deps.GetUser},
        },
    }
    r.RegisterGroup(group)
}
```

## Testing

```go
func TestRouter(t *testing.T) {
    // Create test logger
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))

    // Create test config
    config := router.DefaultRouterConfig()
    config.Port = "0" // Random port

    // Create router
    r := router.NewRouter(config, logger)

    // Register test route
    r.Register(&router.Route{
        Method: "GET",
        Path:   "/test",
        HandlerFunc: func(w http.ResponseWriter, req *http.Request) {
            w.WriteHeader(http.StatusOK)
        },
    })

    // Test with httptest
    req := httptest.NewRequest("GET", "/api/v1/test", nil)
    w := httptest.NewRecorder()
    r.mux.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
}
```

## Migration from Old Version

See [MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md) for detailed migration instructions.

Key changes:
- Constructor now requires config and logger
- No more global config access
- Middlewares injected, not created internally
- Documentation dev-only by default
- Graceful shutdown support added

## Performance Tips

1. **Use appropriate timeouts**: Prevents slowloris attacks
2. **Connection pooling**: Reuse database connections
3. **Middleware order**: Put cheaper middlewares first
4. **Rate limiting**: Use Redis for distributed rate limiting
5. **Caching**: Cache responses where appropriate

## Security Checklist

- [ ] Set `Mode: "prod"` in production
- [ ] Enable TLS with valid certificates
- [ ] Configure `Security.MaxRequestBodySize` appropriately
- [ ] Configure `Security.TrustedProxies` for your infrastructure
- [ ] Enable `ReadHeaderTimeout` (default: 10s)
- [ ] Implement rate limiting middleware
- [ ] Validate all inputs
- [ ] Use authentication on sensitive routes
- [ ] Log security events with request ID
- [ ] Set appropriate timeouts
- [ ] Don't expose internal errors to clients
- [ ] Use CORS appropriately
- [ ] Register health checks for critical dependencies
- [ ] Set up usage metering for billing/abuse detection

## License

MIT License
