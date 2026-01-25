package main

import (
	"log"
	"log/slog"
	"os"
	"time"

	"warehouse_system/api/routes"
	"warehouse_system/internal/config"
	dbq "warehouse_system/internal/database/db"
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
			Enabled: true,
			Dir:     "web/templates",
		},
		Static: &router.StaticConfig{
			Enabled:   true,
			Dir:       "web/static",
			URLPrefix: "/static",
		},
	}

	// Create router
	r := router.NewRouter(routerConfig, logger)
	queries := dbq.New(db)
	// Register application routes
	routes.SetupRoutes(r, db, queries, logger, nil)

	logger.Info("Starting server", "port", cfg.Server.Port)

	// Start server (this includes graceful shutdown handling)
	if err := r.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
