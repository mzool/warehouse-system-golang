package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Config holds HTTP server configuration
type Config struct {
	// Server address (host:port)
	Addr string

	// Logger for structured logging
	Logger *slog.Logger

	// ReadTimeout is the maximum duration for reading the entire request
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response
	WriteTimeout time.Duration

	// IdleTimeout is the maximum amount of time to wait for the next request
	IdleTimeout time.Duration

	// MaxHeaderBytes controls the maximum number of bytes the server will read parsing the request header
	MaxHeaderBytes int

	// TLS configuration
	TLSCertFile string
	TLSKeyFile  string

	// ShutdownTimeout is the maximum duration for graceful shutdown
	ShutdownTimeout time.Duration
}

// DefaultConfig returns a default server configuration
func DefaultConfig(addr string) *Config {
	return &Config{
		Addr:            addr,
		Logger:          nil,
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     60 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1 MB
		ShutdownTimeout: 30 * time.Second,
	}
}

// ProductionConfig returns a production-optimized server configuration
func ProductionConfig(addr string) *Config {
	return &Config{
		Addr:            addr,
		Logger:          nil,
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		MaxHeaderBytes:  1 << 20, // 1 MB
		ShutdownTimeout: 30 * time.Second,
	}
}

// DevelopmentConfig returns a development-friendly server configuration
func DevelopmentConfig(addr string) *Config {
	return &Config{
		Addr:            addr,
		Logger:          nil,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     300 * time.Second,
		MaxHeaderBytes:  2 << 20, // 2 MB
		ShutdownTimeout: 10 * time.Second,
	}
}

// New creates a new HTTP server with the given configuration
func New(handler http.Handler, config *Config) *http.Server {
	if config == nil {
		config = DefaultConfig(":8080")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	server := &http.Server{
		Addr:           config.Addr,
		Handler:        handler,
		ReadTimeout:    config.ReadTimeout,
		WriteTimeout:   config.WriteTimeout,
		IdleTimeout:    config.IdleTimeout,
		MaxHeaderBytes: config.MaxHeaderBytes,
		ErrorLog:       slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	logger.Info("http server configured",
		"addr", config.Addr,
		"read_timeout", config.ReadTimeout.String(),
		"write_timeout", config.WriteTimeout.String(),
		"idle_timeout", config.IdleTimeout.String(),
	)

	return server
}

// Start starts the HTTP server with graceful shutdown
func Start(handler http.Handler, config *Config, resources []Resource) error {
	if config == nil {
		config = DefaultConfig(":8080")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	server := New(handler, config)

	shutdownConfig := &ShutdownConfig{
		Logger:  logger,
		Timeout: config.ShutdownTimeout,
		OnShutdownStart: func() {
			logger.Info("shutdown initiated, stopping server gracefully")
		},
		OnShutdownComplete: func() {
			logger.Info("shutdown complete")
		},
	}

	// Start with TLS if configured
	if config.TLSCertFile != "" && config.TLSKeyFile != "" {
		logger.Info("tls enabled", "cert", config.TLSCertFile, "key", config.TLSKeyFile)
		return RunTLSWithResources(server, config.TLSCertFile, config.TLSKeyFile, resources, shutdownConfig)
	}

	return RunWithResources(server, resources, shutdownConfig)
}

// Serve starts the server and blocks until shutdown
// This is a simple helper for basic use cases
func Serve(addr string, handler http.Handler) error {
	config := DefaultConfig(addr)
	return Start(handler, config, nil)
}

// ServeWithShutdown starts the server with custom shutdown handling
func ServeWithShutdown(addr string, handler http.Handler, onShutdown func()) error {
	config := DefaultConfig(addr)
	server := New(handler, config)

	shutdownConfig := &ShutdownConfig{
		Logger:  config.Logger,
		Timeout: config.ShutdownTimeout,
		OnShutdownStart: func() {
			if onShutdown != nil {
				onShutdown()
			}
		},
	}

	return Run(server, shutdownConfig)
}

// HealthServer creates a simple health check server (useful for sidecar containers)
func HealthServer(port int, healthHandler http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/health", healthHandler)
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
}

// GracefulShutdown performs graceful shutdown on a running server
func GracefulShutdown(ctx context.Context, server *http.Server, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("shutting down server gracefully")

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		return err
	}

	logger.Info("server shutdown completed")
	return nil
}
