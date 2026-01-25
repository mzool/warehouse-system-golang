package observability

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsConfig holds configuration for Prometheus metrics middleware
type MetricsConfig struct {
	// Logger for structured logging
	Logger *slog.Logger

	// Namespace for metrics (e.g., "myapp")
	Namespace string

	// Subsystem for metrics (e.g., "http")
	Subsystem string

	// Buckets for response time histogram
	Buckets []float64

	// Skipper defines a function to skip middleware
	Skipper func(r *http.Request) bool

	// SkipPaths defines paths that should not be metered
	SkipPaths []string
}

// Metrics holds Prometheus metric collectors
type Metrics struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestSize     *prometheus.HistogramVec
	responseSize    *prometheus.HistogramVec
	activeRequests  *prometheus.GaugeVec
}

// DefaultMetricsConfig returns a default metrics configuration
func DefaultMetricsConfig(namespace string) *MetricsConfig {
	return &MetricsConfig{
		Logger:    nil,
		Namespace: namespace,
		Subsystem: "http",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		Skipper:   nil,
		SkipPaths: []string{"/metrics", "/health", "/live", "/ready"},
	}
}

// NewMetrics creates and registers Prometheus metrics
func NewMetrics(config *MetricsConfig) *Metrics {
	if config == nil {
		config = DefaultMetricsConfig("app")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("initializing prometheus metrics",
		"namespace", config.Namespace,
		"subsystem", config.Subsystem,
	)

	metrics := &Metrics{
		requestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "request_duration_seconds",
				Help:      "HTTP request latency in seconds",
				Buckets:   config.Buckets,
			},
			[]string{"method", "path", "status"},
		),
		requestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "request_size_bytes",
				Help:      "HTTP request size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
			},
			[]string{"method", "path"},
		),
		responseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
			},
			[]string{"method", "path", "status"},
		),
		activeRequests: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "requests_active",
				Help:      "Number of active HTTP requests",
			},
			[]string{"method", "path"},
		),
	}

	return metrics
}

// Middleware returns a Prometheus metrics middleware
func (m *Metrics) Middleware(config *MetricsConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultMetricsConfig("app")
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("metrics middleware initialized")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip metrics for specific paths
			for _, path := range config.SkipPaths {
				if r.URL.Path == path {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Normalize path for metrics (you might want to use router's matched path)
			path := r.URL.Path
			method := r.Method

			// Track active requests
			m.activeRequests.WithLabelValues(method, path).Inc()
			defer m.activeRequests.WithLabelValues(method, path).Dec()

			// Track request size
			if r.ContentLength > 0 {
				m.requestSize.WithLabelValues(method, path).Observe(float64(r.ContentLength))
			}

			// Wrap response writer to capture status code and size
			start := time.Now()
			rw := &metricsResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Serve request
			next.ServeHTTP(rw, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(rw.statusCode)

			m.requestsTotal.WithLabelValues(method, path, status).Inc()
			m.requestDuration.WithLabelValues(method, path, status).Observe(duration)
			m.responseSize.WithLabelValues(method, path, status).Observe(float64(rw.bytesWritten))

			logger.Debug("request metrics recorded",
				"method", method,
				"path", path,
				"status", status,
				"duration", duration,
			)
		})
	}
}

// metricsResponseWriter wraps http.ResponseWriter to capture status code and bytes written
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *metricsResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// MetricsHandler returns a Prometheus metrics HTTP handler
// Endpoint: GET /metrics
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// CustomMetrics allows you to define custom Prometheus metrics
type CustomMetrics struct {
	registry *prometheus.Registry
	logger   *slog.Logger
}

// NewCustomMetrics creates a new custom metrics registry
func NewCustomMetrics(logger *slog.Logger) *CustomMetrics {
	if logger == nil {
		logger = slog.Default()
	}

	return &CustomMetrics{
		registry: prometheus.NewRegistry(),
		logger:   logger,
	}
}

// RegisterCounter registers a new counter metric
func (cm *CustomMetrics) RegisterCounter(name, help string, labels []string) *prometheus.CounterVec {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: help,
		},
		labels,
	)
	cm.registry.MustRegister(counter)
	cm.logger.Debug("registered counter metric", "name", name)
	return counter
}

// RegisterGauge registers a new gauge metric
func (cm *CustomMetrics) RegisterGauge(name, help string, labels []string) *prometheus.GaugeVec {
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		labels,
	)
	cm.registry.MustRegister(gauge)
	cm.logger.Debug("registered gauge metric", "name", name)
	return gauge
}

// RegisterHistogram registers a new histogram metric
func (cm *CustomMetrics) RegisterHistogram(name, help string, buckets []float64, labels []string) *prometheus.HistogramVec {
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    name,
			Help:    help,
			Buckets: buckets,
		},
		labels,
	)
	cm.registry.MustRegister(histogram)
	cm.logger.Debug("registered histogram metric", "name", name)
	return histogram
}

// RegisterSummary registers a new summary metric
func (cm *CustomMetrics) RegisterSummary(name, help string, objectives map[float64]float64, labels []string) *prometheus.SummaryVec {
	summary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       name,
			Help:       help,
			Objectives: objectives,
		},
		labels,
	)
	cm.registry.MustRegister(summary)
	cm.logger.Debug("registered summary metric", "name", name)
	return summary
}

// Handler returns the custom metrics HTTP handler
func (cm *CustomMetrics) Handler() http.Handler {
	return promhttp.HandlerFor(cm.registry, promhttp.HandlerOpts{})
}

// Example business metrics helpers

// DatabaseMetrics holds database-specific metrics
type DatabaseMetrics struct {
	QueryDuration *prometheus.HistogramVec
	QueryErrors   *prometheus.CounterVec
	PoolSize      *prometheus.GaugeVec
}

// NewDatabaseMetrics creates database metrics
func NewDatabaseMetrics(namespace string) *DatabaseMetrics {
	return &DatabaseMetrics{
		QueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "query_duration_seconds",
				Help:      "Database query duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"query", "status"},
		),
		QueryErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "query_errors_total",
				Help:      "Total number of database query errors",
			},
			[]string{"query", "error_type"},
		),
		PoolSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "pool_connections",
				Help:      "Number of database connections",
			},
			[]string{"state"}, // idle, active, total
		),
	}
}

// CacheMetrics holds cache-specific metrics
type CacheMetrics struct {
	Hits        *prometheus.CounterVec
	Misses      *prometheus.CounterVec
	Evictions   *prometheus.CounterVec
	SetDuration *prometheus.HistogramVec
	GetDuration *prometheus.HistogramVec
}

// NewCacheMetrics creates cache metrics
func NewCacheMetrics(namespace string) *CacheMetrics {
	return &CacheMetrics{
		Hits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "cache",
				Name:      "hits_total",
				Help:      "Total number of cache hits",
			},
			[]string{"cache"},
		),
		Misses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "cache",
				Name:      "misses_total",
				Help:      "Total number of cache misses",
			},
			[]string{"cache"},
		),
		Evictions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "cache",
				Name:      "evictions_total",
				Help:      "Total number of cache evictions",
			},
			[]string{"cache", "reason"},
		),
		SetDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "cache",
				Name:      "set_duration_seconds",
				Help:      "Cache set operation duration in seconds",
				Buckets:   []float64{.0001, .0005, .001, .0025, .005, .01, .025, .05, .1},
			},
			[]string{"cache"},
		),
		GetDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "cache",
				Name:      "get_duration_seconds",
				Help:      "Cache get operation duration in seconds",
				Buckets:   []float64{.0001, .0005, .001, .0025, .005, .01, .025, .05, .1},
			},
			[]string{"cache"},
		),
	}
}
