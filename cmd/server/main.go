package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"warehouse_system/api/routes"
	"warehouse_system/internal/cache"
	"warehouse_system/internal/config"
	dbq "warehouse_system/internal/database/db"
	"warehouse_system/internal/middlewares"
	"warehouse_system/internal/observability"
	"warehouse_system/internal/router"
)

func main() {
	// Setup logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration
	cfg, err := config.LoadConfig(logger)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database connection if needed
	dbConfig := config.DBConfig{
		DatabaseURL:       cfg.Database.URL,
		Logger:            logger,
		MaxConns:          10,
		MinConns:          2,
		HealthCheckPeriod: time.Minute,
		ConnectTimeout:    5 * time.Second,
		MaxRetries:        3,
		RetryDelay:        time.Second,
	}
	db, err := config.NewPool(&dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Setup router configuration
	routerConfig := &router.RouterConfig{
		Version:  "v1",
		BasePath: "/api",
		Port:     cfg.Server.Port,
		Domain:   cfg.Server.Domain,
		Protocol: cfg.Server.Protocol,
		Mode:     "dev",
		Templates: &router.TemplateConfig{
			Enabled: false,
			Dir:     "web/templates",
		},
		Static: &router.StaticConfig{
			Enabled:   false,
			Dir:       "web/static",
			URLPrefix: "/static",
		},
	}
	// Redis cache (no fallback)
	cacheSystem, err := cache.NewRedisCache(&cache.RedisConfig{
		Config: &cache.Config{
			DefaultTTL: 30 * time.Minute,
			Prefix:     "app:",
			Enabled:    true,
		},
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		MaxRetries:   3,
		PoolSize:     10,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		Logger:       logger,
	})
	if err != nil {
		logger.Error("Failed to initialize cache system", "error", err)
		return
	}
	defer cacheSystem.Close()

	// 1. Recovery - Catch panics first (outermost middleware)
	recoveryConfig := &middlewares.RecoveryConfig{
		Logger:            logger,
		Development:       cfg.App.Environment == "dev",
		DisableStackTrace: false,
	}

	// 2. Request ID - For tracing and logging
	requestIDConfig := &observability.RequestIDConfig{
		Logger: logger,
		Header: "X-Request-ID",
	}

	// 3. Logger - Log requests with request ID
	loggerConfig := middlewares.DefaultLoggerConfig(logger)

	// 4. Security Headers - Apply security headers early
	securityConfig := &middlewares.SecurityConfig{
		Logger:                logger,
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000, // 1 year
		HSTSIncludeSubdomains: true,
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	}

	// 5. CORS - Handle CORS requests
	corsConfig := &middlewares.CORSConfig{
		Logger:           logger,
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     cfg.CORS.AllowedMethods,
		AllowHeaders:     cfg.CORS.AllowedHeaders,
		ExposeHeaders:    []string{"X-Request-ID", "X-Total-Count"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	// 6. Rate Limiter - Protect against abuse
	rateLimiterConfig := &middlewares.RateLimitConfig{
		Logger:     logger,
		Cache:      cacheSystem,
		Capacity:   100, // 100 requests
		RefillRate: 5.0, // 5 requests per second
		Message:    "Too many requests, please try again later",
		StatusCode: http.StatusTooManyRequests,
	}

	// 7. Timeout - Prevent long-running requests
	timeoutConfig := &middlewares.TimeoutConfig{
		Logger:              logger,
		Timeout:             30 * time.Second,
		Message:             "Request timeout",
		StatusCode:          http.StatusRequestTimeout,
		SkipTimeoutForPaths: []string{"/health"},
	}

	// 8. Session Auth - Authentication and authorization
	sessionConfig := &middlewares.SessionConfig{
		Cache:                cacheSystem,
		SecretKey:            []byte(cfg.Auth.SessionSecret),
		SessionKeyPrefix:     "session:",
		RoleKeyPrefix:        "user:roles:",
		PermissionKeyPrefix:  "user:perms:",
		AuthVersionKeyPrefix: "user:authver:",
		CookieName:           "session",
		CookiePath:           "/",
		CookieSecure:         cfg.Server.Protocol == "https",
		CookieHTTPOnly:       true,
		CookieSameSite:       http.SameSiteLaxMode,
		SessionDuration:      24 * time.Hour,
		RoleCacheDuration:    15 * time.Minute,
		Logger:               logger,
		ErrorHandler:         middlewares.NewSmartErrorHandler("/login"), // Smart handler: JSON for API, redirect for UI
		SkipPaths:            []string{"/api/v1/login", "/login", "/api/v1/register", "/register", "forget-password", "/api/v1/forget-password", "/health"},
	}

	// 9. Pagination - Parse pagination query parameters
	paginationConfig := &middlewares.PaginationConfig{
		DefaultPage:     cfg.Pagination.DefaultPage,
		DefaultPageSize: cfg.Pagination.DefaultPageSize,
		MaxPageSize:     cfg.Pagination.MaxPageSize,
		Logger:          logger,
	}

	// Request → Recovery → RequestID → Logger → Security → CORS → RateLimiter → Timeout → SessionAuth → Pagination → Handler
	r := router.NewRouter(routerConfig, logger,
		middlewares.Recovery(recoveryConfig),     // 1. Catch panics
		observability.RequestID(requestIDConfig), // 2. Add request ID
		middlewares.Logger(loggerConfig),         // 3. Log requests
		middlewares.Security(securityConfig),     // 4. Security headers
		middlewares.CORS(corsConfig),             // 5. CORS
		middlewares.RateLimit(rateLimiterConfig), // 6. Rate limiting
		middlewares.Timeout(timeoutConfig),       // 7. Request timeout
		middlewares.SessionAuth(sessionConfig),   // 8. Authentication
		middlewares.Pagination(paginationConfig), // 9. Pagination
	)
	queries := dbq.New(db)
	// Register application routes
	routes.SetupRoutes(r, db, queries, logger, cacheSystem, cfg)

	logger.Info("Starting server", "port", cfg.Server.Port)

	// Start server (this includes graceful shutdown handling)
	if err := r.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
