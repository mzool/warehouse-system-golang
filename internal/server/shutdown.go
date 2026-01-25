package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// ShutdownConfig holds configuration for graceful shutdown
type ShutdownConfig struct {
	// Logger for structured logging
	Logger *slog.Logger

	// Timeout for graceful shutdown
	Timeout time.Duration

	// Signals to listen for (default: SIGINT, SIGTERM)
	Signals []os.Signal

	// OnShutdownStart is called when shutdown begins
	OnShutdownStart func()

	// OnShutdownComplete is called when shutdown completes
	OnShutdownComplete func()
}

// DefaultShutdownConfig returns a default shutdown configuration
func DefaultShutdownConfig() *ShutdownConfig {
	return &ShutdownConfig{
		Logger:  nil,
		Timeout: 30 * time.Second,
		Signals: []os.Signal{
			syscall.SIGINT,  // Ctrl+C
			syscall.SIGTERM, // Kubernetes/Docker stop
			syscall.SIGQUIT, // Ctrl+\
		},
		OnShutdownStart:    nil,
		OnShutdownComplete: nil,
	}
}

// Resource represents a resource that needs cleanup during shutdown
type Resource interface {
	Name() string
	Close(ctx context.Context) error
}

// ShutdownManager manages graceful shutdown of the application
type ShutdownManager struct {
	config    *ShutdownConfig
	logger    *slog.Logger
	resources []Resource
	mu        sync.RWMutex
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(config *ShutdownConfig) *ShutdownManager {
	if config == nil {
		config = DefaultShutdownConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &ShutdownManager{
		config:    config,
		logger:    logger,
		resources: make([]Resource, 0),
	}
}

// Register adds a resource to be cleaned up during shutdown
func (sm *ShutdownManager) Register(resource Resource) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.resources = append(sm.resources, resource)
	sm.logger.Debug("resource registered for shutdown", "resource", resource.Name())
}

// Wait blocks until a shutdown signal is received, then performs graceful shutdown
func (sm *ShutdownManager) Wait() error {
	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, sm.config.Signals...)

	sig := <-sigChan
	sm.logger.Info("shutdown signal received", "signal", sig.String())

	// Call OnShutdownStart callback
	if sm.config.OnShutdownStart != nil {
		sm.config.OnShutdownStart()
	}

	// Perform shutdown
	ctx, cancel := context.WithTimeout(context.Background(), sm.config.Timeout)
	defer cancel()

	err := sm.Shutdown(ctx)

	// Call OnShutdownComplete callback
	if sm.config.OnShutdownComplete != nil {
		sm.config.OnShutdownComplete()
	}

	return err
}

// Shutdown performs graceful shutdown of all registered resources
func (sm *ShutdownManager) Shutdown(ctx context.Context) error {
	sm.logger.Info("initiating graceful shutdown",
		"timeout", sm.config.Timeout.String(),
		"resources", len(sm.resources),
	)

	sm.mu.RLock()
	resources := make([]Resource, len(sm.resources))
	copy(resources, sm.resources)
	sm.mu.RUnlock()

	// Shutdown resources in reverse order (LIFO)
	var wg sync.WaitGroup
	errors := make(chan error, len(resources))

	for i := len(resources) - 1; i >= 0; i-- {
		resource := resources[i]
		wg.Add(1)
		go func(r Resource) {
			defer wg.Done()

			sm.logger.Info("closing resource", "resource", r.Name())
			start := time.Now()

			if err := r.Close(ctx); err != nil {
				sm.logger.Error("failed to close resource",
					"resource", r.Name(),
					"error", err,
					"duration", time.Since(start).String(),
				)
				errors <- err
			} else {
				sm.logger.Info("resource closed successfully",
					"resource", r.Name(),
					"duration", time.Since(start).String(),
				)
			}
		}(resource)
	}

	// Wait for all resources to close or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		sm.logger.Info("all resources closed successfully")
		return nil
	case <-ctx.Done():
		sm.logger.Warn("shutdown timeout exceeded, forcing shutdown")
		return ctx.Err()
	}
}

// HTTPServerResource wraps an HTTP server for graceful shutdown
type HTTPServerResource struct {
	server *http.Server
	name   string
}

// NewHTTPServerResource creates a new HTTP server resource
func NewHTTPServerResource(name string, server *http.Server) *HTTPServerResource {
	return &HTTPServerResource{
		server: server,
		name:   name,
	}
}

func (h *HTTPServerResource) Name() string {
	return h.name
}

func (h *HTTPServerResource) Close(ctx context.Context) error {
	return h.server.Shutdown(ctx)
}

// DatabaseResource wraps a database pool for graceful shutdown
type DatabaseResource struct {
	pool *pgxpool.Pool
	name string
}

// NewDatabaseResource creates a new database resource
func NewDatabaseResource(name string, pool *pgxpool.Pool) *DatabaseResource {
	return &DatabaseResource{
		pool: pool,
		name: name,
	}
}

func (d *DatabaseResource) Name() string {
	return d.name
}

// pgxpool.Close() doesn't accept context, but we can wait for it
func (d *DatabaseResource) Close(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		d.pool.Close()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// Force close anyway
		d.pool.Close()
		return ctx.Err()
	}
}

// RedisResource wraps a Redis client for graceful shutdown
type RedisResource struct {
	client *redis.Client
	name   string
}

// NewRedisResource creates a new Redis resource
func NewRedisResource(name string, client *redis.Client) *RedisResource {
	return &RedisResource{
		client: client,
		name:   name,
	}
}

func (r *RedisResource) Name() string {
	return r.name
}

func (r *RedisResource) Close(ctx context.Context) error {
	return r.client.Close()
}

// CustomResource wraps a custom cleanup function
type CustomResource struct {
	name      string
	closeFunc func(ctx context.Context) error
}

// NewCustomResource creates a new custom resource
func NewCustomResource(name string, closeFunc func(ctx context.Context) error) *CustomResource {
	return &CustomResource{
		name:      name,
		closeFunc: closeFunc,
	}
}

func (c *CustomResource) Name() string {
	return c.name
}

func (c *CustomResource) Close(ctx context.Context) error {
	return c.closeFunc(ctx)
}

// Run starts an HTTP server with graceful shutdown handling
func Run(server *http.Server, config *ShutdownConfig) error {
	if config == nil {
		config = DefaultShutdownConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sm := NewShutdownManager(config)
	sm.Register(NewHTTPServerResource("http-server", server))

	// Start server in goroutine
	go func() {
		logger.Info("starting http server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	return sm.Wait()
}

// RunTLS starts an HTTPS server with graceful shutdown handling
func RunTLS(server *http.Server, certFile, keyFile string, config *ShutdownConfig) error {
	if config == nil {
		config = DefaultShutdownConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sm := NewShutdownManager(config)
	sm.Register(NewHTTPServerResource("https-server", server))

	// Start server in goroutine
	go func() {
		logger.Info("starting https server", "addr", server.Addr)
		if err := server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	return sm.Wait()
}

// RunWithResources starts an HTTP server with additional resources and graceful shutdown
func RunWithResources(server *http.Server, resources []Resource, config *ShutdownConfig) error {
	if config == nil {
		config = DefaultShutdownConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sm := NewShutdownManager(config)

	// Register HTTP server first
	sm.Register(NewHTTPServerResource("http-server", server))

	// Register additional resources
	for _, resource := range resources {
		sm.Register(resource)
	}

	// Start server in goroutine
	go func() {
		logger.Info("starting http server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	return sm.Wait()
}

// RunTLSWithResources starts an HTTPS server with resources and graceful shutdown
func RunTLSWithResources(server *http.Server, certFile, keyFile string, resources []Resource, config *ShutdownConfig) error {
	if config == nil {
		config = DefaultShutdownConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sm := NewShutdownManager(config)

	// Register HTTPS server first
	sm.Register(NewHTTPServerResource("https-server", server))

	// Register additional resources
	for _, resource := range resources {
		sm.Register(resource)
	}

	// Start server in goroutine
	go func() {
		logger.Info("starting https server", "addr", server.Addr)
		if err := server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	return sm.Wait()
}

// WaitForShutdown blocks until a shutdown signal is received
func WaitForShutdown(signals ...os.Signal) os.Signal {
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)
	return <-sigChan
}

// NotifyShutdown sends a shutdown notification
func NotifyShutdown() {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return
	}
	p.Signal(syscall.SIGTERM)
}
