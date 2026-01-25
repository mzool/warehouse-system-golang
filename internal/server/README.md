# Server Package

Production-ready HTTP server with graceful shutdown, resource management, and lifecycle handling.

## Features

- **Graceful Shutdown** - Handles SIGINT, SIGTERM, SIGQUIT signals
- **Resource Management** - Automatic cleanup of database pools, Redis clients, etc.
- **Configurable Timeouts** - Read, write, idle, and shutdown timeouts
- **TLS Support** - HTTPS with certificate management
- **Production & Development Presets** - Optimized configurations

## Quick Start

### Basic Server

```go
package main

import (
    "net/http"
    "your-module/internal/server"
)

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello, World!"))
    })

    // Simple server with defaults
    if err := server.Serve(":8080", mux); err != nil {
        panic(err)
    }
}
```

### Production Server with Resources

```go
package main

import (
    "log/slog"
    "net/http"
    "os"
    
    "your-module/internal/config"
    "your-module/internal/server"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    
    // Load config
    cfg, err := config.LoadConfig(logger)
    if err != nil {
        logger.Error("failed to load config", "error", err)
        os.Exit(1)
    }
    
    // Database pool
    dbConfig := config.DefaultDBConfig(cfg.Database.URL)
    dbPool, err := config.NewPool(dbConfig)
    if err != nil {
        logger.Error("failed to connect to database", "error", err)
        os.Exit(1)
    }
    
    // Redis client
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
    
    // Create router/handler
    handler := createHandler()
    
    // Server config
    serverConfig := server.ProductionConfig(":" + cfg.Server.Port)
    serverConfig.Logger = logger
    serverConfig.TLSCertFile = cfg.TLS.CertFile
    serverConfig.TLSKeyFile = cfg.TLS.KeyFile
    
    // Resources to cleanup on shutdown
    resources := []server.Resource{
        server.NewDatabaseResource("postgres", dbPool),
        server.NewRedisResource("redis", redisClient),
    }
    
    // Start server with graceful shutdown
    if err := server.Start(handler, serverConfig, resources); err != nil {
        logger.Error("server error", "error", err)
        os.Exit(1)
    }
}
```

## Configuration

### Default Config

```go
config := server.DefaultConfig(":8080")
// ReadTimeout: 15s
// WriteTimeout: 15s
// IdleTimeout: 60s
// MaxHeaderBytes: 1MB
// ShutdownTimeout: 30s
```

### Production Config

```go
config := server.ProductionConfig(":8080")
// ReadTimeout: 10s
// WriteTimeout: 10s
// IdleTimeout: 120s
// MaxHeaderBytes: 1MB
// ShutdownTimeout: 30s
```

### Development Config

```go
config := server.DevelopmentConfig(":8080")
// ReadTimeout: 30s
// WriteTimeout: 30s
// IdleTimeout: 300s
// MaxHeaderBytes: 2MB
// ShutdownTimeout: 10s
```

### Custom Config

```go
config := &server.Config{
    Addr:            ":8080",
    Logger:          logger,
    ReadTimeout:     20 * time.Second,
    WriteTimeout:    20 * time.Second,
    IdleTimeout:     90 * time.Second,
    MaxHeaderBytes:  2 << 20, // 2MB
    TLSCertFile:     "/path/to/cert.pem",
    TLSKeyFile:      "/path/to/key.pem",
    ShutdownTimeout: 45 * time.Second,
}
```

## Graceful Shutdown

### Automatic Shutdown

The server automatically handles shutdown signals:

```go
server.Start(handler, config, resources)
// Blocks until SIGINT, SIGTERM, or SIGQUIT received
// Then performs graceful shutdown
```

### Custom Shutdown Handling

```go
shutdownConfig := &server.ShutdownConfig{
    Logger:  logger,
    Timeout: 30 * time.Second,
    Signals: []os.Signal{syscall.SIGINT, syscall.SIGTERM},
    OnShutdownStart: func() {
        logger.Info("shutdown started")
        // Send notification, update load balancer, etc.
    },
    OnShutdownComplete: func() {
        logger.Info("shutdown complete")
        // Final cleanup, send metrics, etc.
    },
}

sm := server.NewShutdownManager(shutdownConfig)
sm.Register(server.NewHTTPServerResource("api-server", httpServer))
sm.Register(server.NewDatabaseResource("postgres", dbPool))
sm.Wait() // Blocks until signal received
```

## Resource Management

### Built-in Resources

#### HTTP Server
```go
resource := server.NewHTTPServerResource("my-server", httpServer)
sm.Register(resource)
```

#### Database Pool
```go
resource := server.NewDatabaseResource("postgres", dbPool)
sm.Register(resource)
```

#### Redis Client
```go
resource := server.NewRedisResource("redis", redisClient)
sm.Register(resource)
```

### Custom Resources

```go
resource := server.NewCustomResource("my-service", func(ctx context.Context) error {
    // Custom cleanup logic
    myService.Stop()
    return myService.WaitForShutdown(ctx)
})
sm.Register(resource)
```

Implement the `Resource` interface:

```go
type MyResource struct {
    service *MyService
}

func (r *MyResource) Name() string {
    return "my-service"
}

func (r *MyResource) Close(ctx context.Context) error {
    return r.service.Shutdown(ctx)
}
```

## TLS/HTTPS

### With Certificate Files

```go
config := server.ProductionConfig(":443")
config.TLSCertFile = "/etc/ssl/certs/server.crt"
config.TLSKeyFile = "/etc/ssl/private/server.key"

server.Start(handler, config, resources)
```

### With Let's Encrypt

```go
import "golang.org/x/crypto/acme/autocert"

certManager := &autocert.Manager{
    Prompt:     autocert.AcceptTOS,
    Cache:      autocert.DirCache("certs"),
    HostPolicy: autocert.HostWhitelist("example.com"),
}

httpServer := &http.Server{
    Addr:      ":443",
    Handler:   handler,
    TLSConfig: certManager.TLSConfig(),
}

// HTTP to HTTPS redirect server
go http.ListenAndServe(":80", certManager.HTTPHandler(nil))

// HTTPS server with graceful shutdown
server.RunWithResources(httpServer, resources, shutdownConfig)
```

## Shutdown Sequence

1. **Signal Received** - SIGINT, SIGTERM, or SIGQUIT
2. **OnShutdownStart Callback** - Execute custom pre-shutdown logic
3. **HTTP Server Shutdown** - Stop accepting new connections
4. **Drain Connections** - Complete in-flight requests (with timeout)
5. **Close Resources** - Cleanup database pools, Redis, etc. (in reverse order)
6. **OnShutdownComplete Callback** - Execute post-shutdown logic
7. **Exit** - Process terminates

## Kubernetes Integration

### Deployment with Probes

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: myapp
        ports:
        - containerPort: 8080
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
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "sleep 5"]
```

### Graceful Termination

```go
config := server.ProductionConfig(":8080")
config.ShutdownTimeout = 30 * time.Second

// Kubernetes sends SIGTERM, waits terminationGracePeriodSeconds (default 30s)
server.Start(handler, config, resources)
```

## Docker Integration

### Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o server cmd/server/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
```

### Docker Stop

```bash
# Sends SIGTERM, waits 10s by default
docker stop myapp

# Custom timeout
docker stop -t 30 myapp
```

## Best Practices

1. **Always use graceful shutdown in production** - Prevents connection errors during deployments
2. **Set appropriate timeouts** - Match your application's response time SLAs
3. **Register resources in dependency order** - They're closed in reverse order (LIFO)
4. **Test shutdown behavior** - Use `kill -TERM <pid>` to simulate
5. **Monitor shutdown duration** - Alert if shutdowns take too long
6. **Use separate health check server** - For sidecar health monitoring
7. **Configure load balancer drain period** - Should be > shutdown timeout

## Testing Shutdown

### Manual Test

```bash
# Start server
go run cmd/server/main.go

# In another terminal, send SIGTERM
kill -TERM $(pgrep server)

# Or Ctrl+C (SIGINT)
```

### Automated Test

```go
func TestGracefulShutdown(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(2 * time.Second) // Simulate slow request
        w.Write([]byte("OK"))
    })
    
    config := server.DefaultConfig(":8080")
    config.ShutdownTimeout = 5 * time.Second
    
    srv := server.New(handler, config)
    
    go srv.ListenAndServe()
    time.Sleep(100 * time.Millisecond) // Let server start
    
    // Make request
    go http.Get("http://localhost:8080/")
    time.Sleep(50 * time.Millisecond)
    
    // Trigger shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    err := srv.Shutdown(ctx)
    if err != nil {
        t.Errorf("shutdown failed: %v", err)
    }
}
```

## Troubleshooting

### Shutdown Hangs

- Check for goroutines that don't respect context cancellation
- Verify database queries have timeouts
- Look for stuck HTTP clients without timeouts

### Connections Reset

- Increase shutdown timeout
- Check load balancer drain period
- Verify health checks return 503 during shutdown

### Resource Cleanup Fails

- Check resource Close() implementations respect context
- Verify timeout is sufficient for all resources
- Add logging to identify which resource is slow

## Dependencies

- `github.com/jackc/pgx/v5` - PostgreSQL driver
- `github.com/redis/go-redis/v9` - Redis client
