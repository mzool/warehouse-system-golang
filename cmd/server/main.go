package main

import (
	"log"
	"log/slog"
	"os"

	"warehouse_system/api/routes"
	"warehouse_system/internal/config"
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
	// dbConfig := config.DefaultDBConfig(cfg.Database.URL)
	// db, err := config.NewPool(dbConfig)
	// if err != nil {
	// 	log.Fatalf("Failed to connect to database: %v", err)
	// }
	// defer db.Close()

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

	// Register application routes
	routes.SetupRoutes(r)

	logger.Info("Starting server", "port", cfg.Server.Port)

	// Start server (this includes graceful shutdown handling)
	if err := r.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
