package config

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBConfig holds database connection configuration
type DBConfig struct {
	// DatabaseURL is the PostgreSQL connection string
	DatabaseURL string

	// Logger for structured logging (optional, uses slog.Default if nil)
	Logger *slog.Logger

	// MaxConns is the maximum number of connections in the pool
	// Default: 10
	MaxConns int32

	// MinConns is the minimum number of connections in the pool
	// Default: 2
	MinConns int32

	// MaxConnLifetime is the maximum lifetime of a connection
	// Set to 0 for infinite when using external connection pooler
	// Default: 0 (infinite for managed databases with connection pooler)
	MaxConnLifetime time.Duration

	// MaxConnIdleTime is the maximum idle time of a connection
	// Set to 0 for no timeout when using external connection pooler
	// Default: 0 (no timeout for managed databases)
	MaxConnIdleTime time.Duration

	// HealthCheckPeriod is the period between health checks
	// Default: 1 minute
	HealthCheckPeriod time.Duration

	// ConnectTimeout is the timeout for establishing connections
	// Default: 10 seconds
	ConnectTimeout time.Duration

	// MaxRetries is the maximum number of connection attempts
	// Default: 3
	MaxRetries int

	// RetryDelay is the initial delay between retry attempts
	// Uses exponential backoff
	// Default: 1 second
	RetryDelay time.Duration
}

// DefaultDBConfig returns a default database configuration
// Optimized for managed databases with external connection pooler (e.g., PgBouncer)
func DefaultDBConfig(databaseURL string) *DBConfig {
	return &DBConfig{
		DatabaseURL:       databaseURL,
		Logger:            nil, // Will use slog.Default()
		MaxConns:          10,
		MinConns:          2,
		MaxConnLifetime:   0,               // Infinite - managed bouncer handles lifecycle
		MaxConnIdleTime:   0,               // No timeout - managed bouncer handles idle
		HealthCheckPeriod: 1 * time.Minute, // Minimal health checks
		ConnectTimeout:    10 * time.Second,
		MaxRetries:        3,
		RetryDelay:        1 * time.Second,
	}
}

// ProductionDBConfig returns a production-optimized database configuration
// For databases without external connection pooler
func ProductionDBConfig(databaseURL string) *DBConfig {
	return &DBConfig{
		DatabaseURL:       databaseURL,
		Logger:            nil,
		MaxConns:          25,
		MinConns:          5,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
		ConnectTimeout:    10 * time.Second,
		MaxRetries:        5,
		RetryDelay:        2 * time.Second,
	}
}

// DevelopmentDBConfig returns a development-optimized database configuration
func DevelopmentDBConfig(databaseURL string) *DBConfig {
	return &DBConfig{
		DatabaseURL:       databaseURL,
		Logger:            nil,
		MaxConns:          5,
		MinConns:          1,
		MaxConnLifetime:   30 * time.Minute,
		MaxConnIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
		ConnectTimeout:    5 * time.Second,
		MaxRetries:        3,
		RetryDelay:        500 * time.Millisecond,
	}
}

// NewPool creates a new database connection pool with the given configuration
func NewPool(config *DBConfig) (*pgxpool.Pool, error) {
	if config == nil {
		return nil, fmt.Errorf("database config cannot be nil")
	}

	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL cannot be empty")
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("initializing database connection pool",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns,
		"max_conn_lifetime", config.MaxConnLifetime.String(),
		"health_check_period", config.HealthCheckPeriod.String(),
	)

	// Parse connection string
	dbConfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		logger.Error("failed to parse database URL", "error", err)
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Apply pool settings
	dbConfig.MaxConns = config.MaxConns
	dbConfig.MinConns = config.MinConns
	dbConfig.MaxConnLifetime = config.MaxConnLifetime
	dbConfig.MaxConnIdleTime = config.MaxConnIdleTime
	dbConfig.HealthCheckPeriod = config.HealthCheckPeriod

	// Set connect timeout
	if config.ConnectTimeout > 0 {
		dbConfig.ConnConfig.ConnectTimeout = config.ConnectTimeout
	}

	// Retry logic with exponential backoff
	var pool *pgxpool.Pool
	var lastErr error

	for attempt := 1; attempt <= config.MaxRetries; attempt++ {
		logger.Debug("attempting database connection",
			"attempt", attempt,
			"max_retries", config.MaxRetries,
		)

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)

		// Create pool
		pool, err = pgxpool.NewWithConfig(ctx, dbConfig)
		cancel()

		if err != nil {
			lastErr = fmt.Errorf("failed to create pool (attempt %d/%d): %w", attempt, config.MaxRetries, err)
			logger.Warn("failed to create database pool",
				"attempt", attempt,
				"max_retries", config.MaxRetries,
				"error", err,
			)

			if attempt < config.MaxRetries {
				delay := calculateBackoff(config.RetryDelay, attempt)
				logger.Info("retrying database connection",
					"delay", delay.String(),
					"next_attempt", attempt+1,
				)
				time.Sleep(delay)
			}
			continue
		}

		// Test connection with ping
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = pool.Ping(pingCtx)
		pingCancel()

		if err != nil {
			lastErr = fmt.Errorf("failed to ping database (attempt %d/%d): %w", attempt, config.MaxRetries, err)
			logger.Warn("failed to ping database",
				"attempt", attempt,
				"max_retries", config.MaxRetries,
				"error", err,
			)

			pool.Close()
			pool = nil

			if attempt < config.MaxRetries {
				delay := calculateBackoff(config.RetryDelay, attempt)
				logger.Info("retrying database connection",
					"delay", delay.String(),
					"next_attempt", attempt+1,
				)
				time.Sleep(delay)
			}
			continue
		}

		// Connection successful
		logger.Info("database connection pool established",
			"attempt", attempt,
			"total_conns", pool.Stat().TotalConns(),
			"idle_conns", pool.Stat().IdleConns(),
		)

		return pool, nil
	}

	// All retries failed
	logger.Error("failed to establish database connection after all retries",
		"max_retries", config.MaxRetries,
		"error", lastErr,
	)

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", config.MaxRetries, lastErr)
}

// calculateBackoff calculates exponential backoff delay
func calculateBackoff(baseDelay time.Duration, attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^(attempt-1)
	multiplier := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(baseDelay) * multiplier)

	// Cap at 30 seconds
	maxDelay := 30 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// PoolStats represents database connection pool statistics
type PoolStats struct {
	AcquireCount            int64         `json:"acquire_count"`
	AcquireDuration         time.Duration `json:"acquire_duration"`
	AcquiredConns           int32         `json:"acquired_conns"`
	CanceledAcquireCount    int64         `json:"canceled_acquire_count"`
	ConstructingConns       int32         `json:"constructing_conns"`
	EmptyAcquireCount       int64         `json:"empty_acquire_count"`
	IdleConns               int32         `json:"idle_conns"`
	MaxConns                int32         `json:"max_conns"`
	TotalConns              int32         `json:"total_conns"`
	NewConnsCount           int64         `json:"new_conns_count"`
	MaxLifetimeDestroyCount int64         `json:"max_lifetime_destroy_count"`
	MaxIdleDestroyCount     int64         `json:"max_idle_destroy_count"`
}

// GetPoolStats retrieves current pool statistics
func GetPoolStats(pool *pgxpool.Pool) *PoolStats {
	if pool == nil {
		return nil
	}

	stat := pool.Stat()
	return &PoolStats{
		AcquireCount:            stat.AcquireCount(),
		AcquireDuration:         stat.AcquireDuration(),
		AcquiredConns:           stat.AcquiredConns(),
		CanceledAcquireCount:    stat.CanceledAcquireCount(),
		ConstructingConns:       stat.ConstructingConns(),
		EmptyAcquireCount:       stat.EmptyAcquireCount(),
		IdleConns:               stat.IdleConns(),
		MaxConns:                stat.MaxConns(),
		TotalConns:              stat.TotalConns(),
		NewConnsCount:           stat.NewConnsCount(),
		MaxLifetimeDestroyCount: stat.MaxLifetimeDestroyCount(),
		MaxIdleDestroyCount:     stat.MaxIdleDestroyCount(),
	}
}

// HealthCheck performs a health check on the database connection pool
func HealthCheck(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	if pool == nil {
		return fmt.Errorf("pool is nil")
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Ping with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := pool.Ping(pingCtx)
	if err != nil {
		logger.Error("database health check failed", "error", err)
		return fmt.Errorf("database health check failed: %w", err)
	}

	// Get pool stats
	stats := GetPoolStats(pool)
	logger.Debug("database health check passed",
		"total_conns", stats.TotalConns,
		"idle_conns", stats.IdleConns,
		"acquired_conns", stats.AcquiredConns,
	)

	return nil
}

// GracefulShutdown gracefully shuts down the database connection pool
func GracefulShutdown(pool *pgxpool.Pool, timeout time.Duration, logger *slog.Logger) error {
	if pool == nil {
		return nil
	}

	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("initiating graceful database shutdown", "timeout", timeout.String())

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Wait for all connections to be released or timeout
	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("database connection pool closed gracefully")
		return nil
	case <-ctx.Done():
		logger.Warn("database shutdown timeout exceeded, forcing close")
		return fmt.Errorf("shutdown timeout exceeded")
	}
}
