# Router Refactoring Migration Guide

## Overview

The router has been refactored to improve:
- **Dependency Injection**: No more global config access
- **Logging**: Replaced `fmt` with structured `slog`
- **Middleware Architecture**: Better separation and flexibility
- **Security**: Dev-only documentation
- **Performance**: Cleaner code, better timeout handling
- **Scalability**: Route groups for large projects

## Breaking Changes

### 1. Constructor Changes

**Old:**
```go
router := router.NewRouter()
router.SetDBPool(dbPool)
```

**New:**
```go
config := &router.RouterConfig{
    Version:      "v1",
    BasePath:     "/api",
    Port:         "8080",
    Domain:       "localhost",
    Protocol:     "http",
    Mode:         "dev", // or "prod"
    ReadTimeout:  15 * time.Second,
    WriteTimeout: 15 * time.Second,
    IdleTimeout:  60 * time.Second,
    Static: &router.StaticConfig{
        Enabled:   true,
        Dir:       "./public",
        URLPrefix: "/static/",
    },
    Templates: &router.TemplateConfig{
        Enabled: true,
        Dir:     "./templates",
    },
}

logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Inject global middlewares
router := router.NewRouter(config, logger, 
    middlewares.Recovery(),
    middlewares.Logger(logger),
    middlewares.CORS(corsConfig),
)
```

### 2. No More Global Config Access

**Old:**
```go
// Router accessed config.AppSettings directly
basePath := config.AppSettings.BasePath
```

**New:**
```go
// All config passed to constructor
config := &router.RouterConfig{
    BasePath: "/api",
}
```

### 3. Middleware Injection

**Old:**
```go
// Middlewares hardcoded in Start() method
// createRateLimitConfig() accessed cache.RedisClient
```

**New:**
```go
// Middlewares injected at construction
router := router.NewRouter(config, logger,
    recoveryMiddleware,
    loggerMiddleware,
    rateLimitMiddleware, // Create this with your dependencies
    corsMiddleware,
)

// Per-route middleware still works
route := &router.Route{
    Method: "POST",
    Path: "/users",
    Middlewares: []router.MiddlewaresType{
        authMiddleware,
        validatorMiddleware,
    },
    HandlerFunc: createUser,
}
```

### 4. Documentation

**Old:**
```go
router.Documentation() // Always available
```

**New:**
```go
// Automatically registered only in dev mode
config := &router.RouterConfig{
    Mode: "dev", // Documentation routes auto-registered
}
// In prod mode, documentation routes are NOT registered
```

### 5. Logging

**Old:**
```go
fmt.Println("Starting server")
fmt.Printf("Error: %v\n", err)
```

**New:**
```go
logger.Info("Starting server", "port", port)
logger.Error("Failed to start", "error", err)
```

### 6. Route Groups (New Feature)

**New functionality for organizing routes:**
```go
// Group routes with shared configuration
adminGroup := &router.RouteGroup{
    Prefix: "/admin",
    Category: "Admin",
    Middlewares: []router.MiddlewaresType{
        authMiddleware,
        adminOnlyMiddleware,
    },
    Routes: []*router.Route{
        {
            Method: "GET",
            Path: "/users", // Final path: /api/v1/admin/users
            HandlerFunc: listUsers,
        },
        {
            Method: "DELETE",
            Path: "/users/{id}",
            HandlerFunc: deleteUser,
        },
    },
}

router.RegisterGroup(adminGroup)
```

### 7. Graceful Shutdown (New Feature)

**New:**
```go
// Graceful shutdown support
go func() {
    if err := router.Start(); err != nil {
        logger.Error("Server failed", "error", err)
    }
}()

// On signal
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
<-stop

if err := router.Shutdown(30 * time.Second); err != nil {
    logger.Error("Shutdown failed", "error", err)
}
```

## Complete Example

```go
package main

import (
    "log/slog"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "yourapp/internal/router"
    "yourapp/internal/middlewares"
)

func main() {
    // Setup logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    // Configure router
    config := &router.RouterConfig{
        Version:      "v1",
        BasePath:     "/api",
        Port:         "8080",
        Domain:       "localhost",
        Protocol:     "http",
        Mode:         os.Getenv("APP_MODE"), // "dev" or "prod"
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
        
        // Optional: TLS
        TLS: &router.TLSConfig{
            Enabled:  false,
            CertFile: "./certs/cert.pem",
            KeyFile:  "./certs/key.pem",
        },
        
        // Optional: Static files
        Static: &router.StaticConfig{
            Enabled:   true,
            Dir:       "./public",
            URLPrefix: "/static/",
        },
        
        // Optional: Templates
        Templates: &router.TemplateConfig{
            Enabled: true,
            Dir:     "./templates",
        },
    }

    // Create middlewares with your dependencies
    recoveryMW := middlewares.Recovery()
    loggerMW := middlewares.Logger(logger)
    rateLimitMW := middlewares.RateLimit(redisClient, 100, 1.67)
    corsMW := middlewares.CORS(&middlewares.CORSConfig{
        AllowOrigins:     []string{"*"},
        AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
        AllowHeaders:     []string{"Content-Type", "Authorization"},
        AllowCredentials: true,
    })

    // Create router with global middlewares
    r := router.NewRouter(config, logger,
        recoveryMW,
        rateLimitMW,
        loggerMW,
        corsMW,
    )

    // Register public routes
    r.Register(&router.Route{
        Method:      "GET",
        Path:        "/health",
        Category:    "System",
        HandlerFunc: healthCheck,
    })

    // Register authenticated routes
    authGroup := &router.RouteGroup{
        Prefix:   "/users",
        Category: "Users",
        Middlewares: []router.MiddlewaresType{
            middlewares.Auth(jwtSecret),
        },
        Routes: []*router.Route{
            {
                Method:      "GET",
                Path:        "/profile",
                HandlerFunc: getProfile,
                Input: &router.RouteInput{
                    RequiredAuth: true,
                    Headers: map[string]string{
                        "Authorization": "Authorization",
                    },
                },
            },
            {
                Method:      "PUT",
                Path:        "/profile",
                HandlerFunc: updateProfile,
                Input: &router.RouteInput{
                    RequiredAuth: true,
                },
            },
        },
    }
    r.RegisterGroup(authGroup)

    // Start server in goroutine
    go func() {
        logger.Info("Starting server")
        if err := r.Start(); err != nil {
            logger.Error("Server failed", "error", err)
            os.Exit(1)
        }
    }()

    // Graceful shutdown
    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
    <-stop

    logger.Info("Shutting down server")
    if err := r.Shutdown(30 * time.Second); err != nil {
        logger.Error("Shutdown failed", "error", err)
    }
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

## Benefits

1. **No Global State**: Everything injected, easier to test
2. **Better Logging**: Structured logs with context
3. **Flexible Middlewares**: Global and per-route, in any order
4. **Security**: Documentation only in dev mode
5. **Scalability**: Route groups for organizing large APIs
6. **Graceful Shutdown**: Clean server shutdown
7. **Type Safety**: Proper config structs
8. **Optional Features**: Templates and static files are opt-in

## Migration Checklist

- [ ] Replace `router.NewRouter()` with config-based constructor
- [ ] Remove `router.SetDBPool()` calls
- [ ] Pass global middlewares to constructor
- [ ] Replace all `config.AppSettings` access with config struct
- [ ] Update middleware creation (no more `createRateLimitConfig`)
- [ ] Set `Mode: "dev"` or `"prod"` in config
- [ ] Replace `fmt` with `slog` in your handlers
- [ ] Add graceful shutdown handler
- [ ] Consider using `RouteGroup` for related routes
- [ ] Update template and static file configuration
