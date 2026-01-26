package router

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ContextKey type for context values to avoid collisions
type ContextKey string

// Context keys for request enrichment
const (
	ContextKeyDomain    ContextKey = "router.domain"
	ContextKeyTenantID  ContextKey = "router.tenant_id"
	ContextKeyVersion   ContextKey = "router.version"
	ContextKeyBasePath  ContextKey = "router.base_path"
	ContextKeyRouteInfo ContextKey = "router.route_info"
	ContextKeyRequestID ContextKey = "router.request_id"
	ContextKeyClientIP  ContextKey = "router.client_ip"
)

// Default security limits
const (
	DefaultMaxRequestBodySize = 10 << 20 // 10 MB
	DefaultMaxHeaderBytes     = 1 << 20  // 1 MB
	DefaultReadHeaderTimeout  = 10 * time.Second

	// Metrics limits to prevent memory leaks
	MaxMetricsDurationSamples = 1000  // Max duration samples per endpoint
	MaxMetricsUniqueEndpoints = 10000 // Max unique endpoint keys

	// Request ID settings
	RequestIDLength = 16 // 16 bytes = 32 hex chars
)

// RouterType defines the type of router for logical separation
type RouterType string

const (
	RouterTypeAPI  RouterType = "api"  // API endpoints (JSON responses)
	RouterTypePage RouterType = "page" // Page endpoints (HTML responses)
	RouterTypeAny  RouterType = "any"  // Mixed (default)
)

// Router defines the HTTP router interface
type Router interface {
	Register(route *Route)                // register a new route
	RegisterGroup(group *RouteGroup)      // register a group of routes with shared prefix and middlewares
	RegisterDomain(domain *DomainConfig)  // register a domain with its own routes
	Start() error                         // start the HTTP server
	Shutdown(timeout time.Duration) error // graceful shutdown
}

// RouterImpl implements the Router interface
type RouterImpl struct {
	config            *RouterConfig
	mux               *http.ServeMux
	server            *http.Server
	redirectServer    *http.Server // HTTP->HTTPS redirect server
	logger            *slog.Logger
	templates         *template.Template
	routes            []*Route
	compiledRoutes    map[string]*CompiledRoute // For conflict detection: "METHOD /path" -> route
	registeredOPTIONS map[string]bool           // Track registered OPTIONS paths
	globalMiddlewares []MiddlewaresType
	domains           map[string]*DomainHandler // Domain-based routing
	defaultDomain     string                    // Default domain for unmatched hosts
	routesMu          sync.RWMutex              // Mutex for thread-safe route registration
	tlsConfig         *tls.Config               // SNI-aware TLS configuration
	activeRequests    atomic.Int64              // Track in-flight requests for graceful shutdown
	isShuttingDown    atomic.Bool               // Flag to reject new requests during shutdown
	healthChecks      []HealthCheck             // Registered health checks
	meteringHook      MeteringHook              // Optional usage metering callback
	metrics           *routerMetrics            // Basic metrics collectors
}

// CompiledRoute represents a fully resolved route with all metadata
// This abstraction allows better introspection, conflict detection, and debugging
type CompiledRoute struct {
	Method       string
	Path         string
	FullPattern  string // "METHOD /full/path"
	Handler      http.Handler
	OriginalPath string // Path before base/version prefixing
	Domain       string // Empty for default routes
	Category     string
	Middlewares  []MiddlewaresType
	Input        *RouteInput
	Response     any
	RouterType   RouterType
	RegisteredAt time.Time
}

// RouteInput holds all possible input types for a route (used for documentation)
type RouteInput struct {
	QueryParameters map[string]string `json:"query_parameters,omitempty"`
	Body            map[string]string `json:"body,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	PathParameters  map[string]string `json:"path_parameters,omitempty"`
	FormData        map[string]string `json:"form_data,omitempty"`
	RequiredAuth    bool              `json:"required_auth"`
}

// MiddlewaresType defines the middleware function signature
type MiddlewaresType func(http.Handler) http.Handler

// RouteGroup represents a group of routes with shared configuration
type RouteGroup struct {
	Prefix      string
	Middlewares []MiddlewaresType
	Routes      []*Route
	Category    string
}

// Route represents a single HTTP route
type Route struct {
	Category       string
	Middlewares    []MiddlewaresType
	Path           string
	HandlerFunc    http.HandlerFunc
	Response       any // Generic response type for documentation
	Input          *RouteInput
	TemplateName   string
	Template       *template.Template
	Method         string
	RouterType     RouterType // API, Page, or Any
	TenantID       string     // Optional: for tenant-specific routes
	SkipBasePath   bool       // Skip base path prefix (e.g., for page routes like /homepage)
	SkipVersioning bool       // Skip version prefix (e.g., for static pages)
	RawPath        bool       // Use path exactly as provided (no modifications)
}

// RouterConfig holds all configuration for the router
type RouterConfig struct {
	// Core settings
	Version  string
	BasePath string
	Port     string
	Domain   string
	Protocol string
	Mode     string // "dev" or "prod"

	// TLS configuration
	TLS *TLSConfig

	// Static file serving
	Static *StaticConfig

	// Template configuration
	Templates *TemplateConfig

	// CORS configuration
	CORS *CORSConfig

	// Server timeouts
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ReadHeaderTimeout time.Duration // Prevents Slowloris attacks

	// Security settings (router-level only)
	Security *SecurityConfig

	// Metrics configuration
	Metrics *MetricsConfig

	// Health check configuration
	Health *HealthConfig
}

// SecurityConfig holds router-level security configuration
// NOTE: For rate limiting, recovery, timeout, validation - use middlewares package
type SecurityConfig struct {
	// Request limits (applied at server level)
	MaxRequestBodySize int64 // Max request body size in bytes (default: 10MB)
	MaxHeaderBytes     int   // Max header size in bytes (default: 1MB)

	// Trusted proxies for X-Forwarded-For header parsing
	TrustedProxies []string     // CIDR ranges or IPs (e.g., "10.0.0.0/8", "192.168.1.1")
	trustedNets    []*net.IPNet // Parsed trusted networks (internal)

	// Request ID (context enrichment - always recommended)
	EnableRequestID bool   // Enable request ID generation/propagation (default: true)
	RequestIDHeader string // Header name for request ID (default: "X-Request-ID")
}

// MetricsConfig holds metrics endpoint configuration
type MetricsConfig struct {
	Enabled     bool   // Enable metrics endpoint
	Path        string // Metrics endpoint path (default: /metrics)
	Namespace   string // Prometheus namespace (default: "router")
	Subsystem   string // Prometheus subsystem (default: "http")
	IncludePath bool   // Include path label (can cause high cardinality)
}

// HealthConfig holds health check endpoint configuration
type HealthConfig struct {
	Enabled       bool   // Enable health endpoints
	LivenessPath  string // Path for liveness probe (default: /health/live)
	ReadinessPath string // Path for readiness probe (default: /health/ready)
}

// HealthCheck represents a health check function
type HealthCheck struct {
	Name    string
	Check   func(ctx context.Context) error
	Timeout time.Duration
}

// HealthStatus represents the health check response
type HealthStatus struct {
	Status    string                 `json:"status"` // "healthy", "unhealthy", "degraded"
	Checks    map[string]CheckResult `json:"checks,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// CheckResult represents a single health check result
type CheckResult struct {
	Status   string        `json:"status"`
	Duration time.Duration `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
}

// MeteringHook is called for each request for usage tracking
type MeteringHook func(ctx context.Context, info *RequestMetrics)

// RequestMetrics contains metrics for a single request
type RequestMetrics struct {
	TenantID     string
	Domain       string
	Method       string
	Path         string
	StatusCode   int
	Duration     time.Duration
	RequestSize  int64
	ResponseSize int64
	Timestamp    time.Time
}

// TLSConfig holds TLS/SSL certificate configuration
type TLSConfig struct {
	Enabled bool
	// Static certificate (legacy, single domain)
	CertFile string
	KeyFile  string
	// SNI-based certificates for multi-domain support
	DomainCerts map[string]*DomainCertificate // domain -> certificate
	// ACME/Let's Encrypt configuration
	ACME *ACMEConfig
}

// DomainCertificate holds certificate information for a specific domain
type DomainCertificate struct {
	Domain   string // Domain or wildcard pattern (e.g., "*.example.com")
	CertFile string
	KeyFile  string
	cert     *tls.Certificate // Loaded certificate (internal)
}

// ACMEConfig holds configuration for automatic certificate management
type ACMEConfig struct {
	Enabled       bool
	Email         string   // Contact email for Let's Encrypt
	CacheDir      string   // Directory to cache certificates
	Domains       []string // Domains to auto-provision
	Staging       bool     // Use staging server for testing
	AcceptTOS     bool     // Accept Terms of Service
	RenewBefore   time.Duration
	HTTPChallenge bool // Enable HTTP-01 challenge
	TLSChallenge  bool // Enable TLS-ALPN-01 challenge
}

// StaticConfig holds static file serving configuration
type StaticConfig struct {
	Enabled   bool
	Dir       string
	URLPrefix string
}

// TemplateConfig holds template rendering configuration
type TemplateConfig struct {
	Enabled bool
	Dir     string
	FuncMap template.FuncMap
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	MaxAge           int
	AllowCredentials bool
}

// DomainConfig holds configuration for a specific domain
type DomainConfig struct {
	Domain      string             // Domain name (e.g., "api.example.com", "*.example.com" for wildcard)
	IsDefault   bool               // Set as default domain for unmatched hosts
	BasePath    string             // Override base path for this domain
	Version     string             // Override API version for this domain
	Middlewares []MiddlewaresType  // Domain-specific middlewares
	Routes      []*Route           // Routes for this domain
	Groups      []*RouteGroup      // Route groups for this domain
	RedirectTo  string             // Redirect all requests to another domain (optional)
	TenantID    string             // Tenant identifier for multi-tenant apps
	RouterType  RouterType         // API, Page, or Any
	Certificate *DomainCertificate // Optional: domain-specific TLS certificate
}

// DomainHandler holds the mux and config for a domain
type DomainHandler struct {
	config            *DomainConfig
	mux               *http.ServeMux
	registeredOPTIONS map[string]bool
	routes            []*Route
	compiledRoutes    map[string]*CompiledRoute
	mu                sync.RWMutex
}

// RouteConflictError represents a route registration conflict
type RouteConflictError struct {
	NewRoute      string
	ExistingRoute string
	Domain        string
	Message       string
}

func (e *RouteConflictError) Error() string {
	if e.Domain != "" {
		return fmt.Sprintf("route conflict on domain %s: %s conflicts with existing route %s - %s",
			e.Domain, e.NewRoute, e.ExistingRoute, e.Message)
	}
	return fmt.Sprintf("route conflict: %s conflicts with existing route %s - %s",
		e.NewRoute, e.ExistingRoute, e.Message)
}

// NewRouter creates a new Router instance with the given configuration
func NewRouter(config *RouterConfig, logger *slog.Logger, globalMiddlewares ...MiddlewaresType) *RouterImpl {
	if config == nil {
		config = DefaultRouterConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Apply security defaults
	config = applySecurityDefaults(config)

	// Parse trusted proxies
	if config.Security != nil && len(config.Security.TrustedProxies) > 0 {
		config.Security.trustedNets = parseTrustedProxies(config.Security.TrustedProxies, logger)
	}

	r := &RouterImpl{
		config:            config,
		mux:               http.NewServeMux(),
		logger:            logger,
		routes:            []*Route{},
		compiledRoutes:    make(map[string]*CompiledRoute),
		registeredOPTIONS: make(map[string]bool),
		globalMiddlewares: []MiddlewaresType{},
		domains:           make(map[string]*DomainHandler),
		defaultDomain:     "",
		healthChecks:      []HealthCheck{},
	}

	// Initialize metrics if configured
	if config.Metrics != nil && config.Metrics.Enabled {
		r.metrics = newRouterMetrics()
		logger.Info("Metrics collection enabled")
	}

	// Add router-specific built-in middlewares (minimal - use middlewares package for full features)
	// 1. Request ID (context enrichment)
	if config.Security != nil && config.Security.EnableRequestID {
		r.globalMiddlewares = append(r.globalMiddlewares, r.requestIDMiddleware())
	}

	// 2. Body size limit (server protection)
	if config.Security != nil && config.Security.MaxRequestBodySize > 0 {
		r.globalMiddlewares = append(r.globalMiddlewares, r.bodySizeLimitMiddleware())
	}

	// 3. Shutdown-aware (graceful shutdown)
	r.globalMiddlewares = append(r.globalMiddlewares, r.shutdownAwareMiddleware())

	// 4. Metrics collection
	if config.Metrics != nil && config.Metrics.Enabled {
		r.globalMiddlewares = append(r.globalMiddlewares, r.metricsMiddleware())
	}

	// Add user-provided middlewares (recovery, rate limit, timeout, etc. from middlewares package)
	r.globalMiddlewares = append(r.globalMiddlewares, globalMiddlewares...)

	// Initialize SNI-based TLS if configured
	if config.TLS != nil && config.TLS.Enabled {
		if err := r.initTLS(); err != nil {
			logger.Error("Failed to initialize TLS", "error", err)
		}
	}

	// Parse templates if enabled
	if config.Templates != nil && config.Templates.Enabled {
		if err := r.parseTemplates(); err != nil {
			logger.Error("Failed to parse templates", "error", err)
		}
	}

	// Setup static file serving if enabled
	if config.Static != nil && config.Static.Enabled {
		r.setupStaticFiles()
	}

	// Register health endpoints if enabled
	if config.Health != nil && config.Health.Enabled {
		r.registerHealthEndpoints()
	}

	// Register metrics endpoint if enabled
	if config.Metrics != nil && config.Metrics.Enabled {
		r.registerMetricsEndpoint()
	}

	// Register documentation routes only in dev mode
	if config.Mode == "dev" {
		r.registerDocumentation()
	}

	return r
}

// applySecurityDefaults ensures security config has sensible defaults
func applySecurityDefaults(config *RouterConfig) *RouterConfig {
	// Initialize security config if nil
	if config.Security == nil {
		config.Security = &SecurityConfig{}
	}

	// Apply defaults
	if config.Security.MaxRequestBodySize == 0 {
		config.Security.MaxRequestBodySize = DefaultMaxRequestBodySize
	}
	if config.Security.MaxHeaderBytes == 0 {
		config.Security.MaxHeaderBytes = DefaultMaxHeaderBytes
	}
	if config.Security.RequestIDHeader == "" {
		config.Security.RequestIDHeader = "X-Request-ID"
	}

	// Enable request ID by default in prod mode
	if config.Mode == "prod" && !config.Security.EnableRequestID {
		config.Security.EnableRequestID = true
	}

	// Apply timeout defaults
	if config.ReadHeaderTimeout == 0 {
		config.ReadHeaderTimeout = DefaultReadHeaderTimeout
	}

	// Initialize health config with defaults if enabled but paths not set
	if config.Health != nil && config.Health.Enabled {
		if config.Health.LivenessPath == "" {
			config.Health.LivenessPath = "/health/live"
		}
		if config.Health.ReadinessPath == "" {
			config.Health.ReadinessPath = "/health/ready"
		}
	}

	return config
}

// parseTrustedProxies parses CIDR ranges and IPs into net.IPNet
func parseTrustedProxies(proxies []string, logger *slog.Logger) []*net.IPNet {
	var nets []*net.IPNet
	for _, proxy := range proxies {
		// Try parsing as CIDR
		if strings.Contains(proxy, "/") {
			_, ipNet, err := net.ParseCIDR(proxy)
			if err != nil {
				logger.Warn("Invalid trusted proxy CIDR", "proxy", proxy, "error", err)
				continue
			}
			nets = append(nets, ipNet)
		} else {
			// Parse as single IP
			ip := net.ParseIP(proxy)
			if ip == nil {
				logger.Warn("Invalid trusted proxy IP", "proxy", proxy)
				continue
			}
			// Convert to /32 or /128 CIDR
			var mask net.IPMask
			if ip.To4() != nil {
				mask = net.CIDRMask(32, 32)
			} else {
				mask = net.CIDRMask(128, 128)
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: mask})
		}
	}
	return nets
}

// DefaultRouterConfig returns a router configuration with sensible defaults
func DefaultRouterConfig() *RouterConfig {
	return &RouterConfig{
		Version:           "v1",
		BasePath:          "/api",
		Port:              "8080",
		Domain:            "localhost",
		Protocol:          "http",
		Mode:              "dev",
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		Security: &SecurityConfig{
			MaxRequestBodySize: DefaultMaxRequestBodySize,
			MaxHeaderBytes:     DefaultMaxHeaderBytes,
			EnableRequestID:    true,
			RequestIDHeader:    "X-Request-ID",
		},
		Health: &HealthConfig{
			Enabled:       true,
			LivenessPath:  "/health/live",
			ReadinessPath: "/health/ready",
		},
	}
}

// setupStaticFiles configures static file serving
func (r *RouterImpl) setupStaticFiles() {
	staticDir := r.config.Static.Dir
	if staticDir == "" {
		return
	}

	// Check if directory exists
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		r.logger.Warn("Static directory does not exist", "dir", staticDir)
		return
	}

	urlPrefix := r.config.Static.URLPrefix
	if urlPrefix == "" {
		urlPrefix = "/" + strings.TrimPrefix(staticDir, "/") + "/"
	}

	// Ensure the prefix ends with a trailing slash for proper pattern matching
	if !strings.HasSuffix(urlPrefix, "/") {
		urlPrefix += "/"
	}

	fileServer := http.FileServer(http.Dir(staticDir))
	stripPrefix := http.StripPrefix(strings.TrimSuffix(urlPrefix, "/"), fileServer)

	// Register with wildcard pattern to handle all static file requests
	// Use the {path...} pattern to match any path under the prefix
	pattern := "GET " + urlPrefix + "{path...}"
	r.mux.Handle(pattern, stripPrefix)

	r.logger.Info("Static files enabled", "dir", staticDir, "prefix", urlPrefix)
}

// GetTemplate returns a template by name
func (r *RouterImpl) GetTemplate(name string) *template.Template {
	if r.templates == nil {
		return nil
	}
	return r.templates.Lookup(name)
}

func (r *RouterImpl) newRoute(route *Route) *Route {
	// assign template if exists
	if route.TemplateName != "" && r.templates != nil {
		tmpl := r.templates.Lookup(route.TemplateName)
		if tmpl != nil {
			route.Template = tmpl
		} else {
			r.logger.Warn("Template not found", "name", route.TemplateName)
		}
	}

	// uppercase method
	route.Method = strings.ToUpper(route.Method)
	return route
}

// sanitizePath cleans and validates a path to prevent traversal attacks
// Returns sanitized path without leading slash
func sanitizePath(p string) string {
	// Clean the path to resolve .., ., and multiple slashes
	cleaned := path.Clean(p)

	// Remove leading slash for consistent handling
	cleaned = strings.TrimPrefix(cleaned, "/")

	// Remove trailing slash
	cleaned = strings.TrimSuffix(cleaned, "/")

	// Prevent empty path
	if cleaned == "" || cleaned == "." {
		return ""
	}

	// Block path traversal attempts that survived path.Clean
	if strings.Contains(cleaned, "..") {
		return ""
	}

	return cleaned
}

// PreparePath makes sure the path is correctly formatted with security sanitization
// Supports multiple path modes:
// - RawPath: Use path exactly as provided (for special cases)
// - SkipBasePath: Skip base path prefix (for page routes like /homepage)
// - SkipVersioning: Skip version prefix (for static pages)
// - TemplateName set: Automatically skip both base path and version (page rendering)
func (r *RouterImpl) preparePath(route *Route) string {
	method := strings.ToUpper(route.Method)

	// Handle root path
	if route.Path == "/" {
		return method + " /"
	}

	// Sanitize the path to prevent traversal attacks
	p := sanitizePath(route.Path)
	if p == "" {
		r.logger.Warn("Path sanitization resulted in empty path, using root",
			"original", route.Path)
		return method + " /"
	}

	// RawPath mode: use path exactly as provided (after sanitization)
	if route.RawPath {
		return method + " /" + p
	}

	// Page routes (templates) or explicit skip flags: no base path or version
	// This handles cases like:
	//   - /homepage (not /api/v1/homepage)
	//   - /dashboard
	//   - /login
	isPageRoute := route.TemplateName != "" ||
		route.RouterType == RouterTypePage ||
		(route.SkipBasePath && route.SkipVersioning)

	if isPageRoute {
		return method + " /" + p
	}

	// Build API path with base path and/or version
	var pathBuilder strings.Builder
	pathBuilder.WriteString(method)
	pathBuilder.WriteString(" /")

	// Add base path unless skipped
	if !route.SkipBasePath && r.config.BasePath != "" {
		basePath := sanitizePath(r.config.BasePath)
		if basePath != "" {
			pathBuilder.WriteString(basePath)
		}
	}

	// Add version unless skipped
	if !route.SkipVersioning && r.config.Version != "" {
		if pathBuilder.Len() > len(method)+2 { // Has content after "METHOD /"
			pathBuilder.WriteString("/")
		}
		pathBuilder.WriteString(r.config.Version)
	}

	// Add the route path
	if pathBuilder.Len() > len(method)+2 {
		pathBuilder.WriteString("/")
	}
	pathBuilder.WriteString(p)

	return pathBuilder.String()
}

// Register registers a single route with conflict detection and context enrichment
func (r *RouterImpl) Register(route *Route) {
	if r.mux == nil {
		r.logger.Error("Router mux is nil, cannot register route")
		return
	}

	route = r.newRoute(route)
	// prepare final path
	finalPath := r.preparePath(route)

	// Check for route conflicts
	r.routesMu.Lock()
	if err := r.checkRouteConflict(finalPath, "", route); err != nil {
		r.routesMu.Unlock()
		if r.config.Mode == "dev" {
			r.logger.Error("Route conflict detected", "error", err)
			panic(err) // Panic in dev mode to catch issues early
		}
		r.logger.Warn("Route conflict detected, overwriting", "error", err)
	}

	// Create compiled route for introspection
	compiled := &CompiledRoute{
		Method:       route.Method,
		Path:         strings.TrimPrefix(finalPath, route.Method+" "),
		FullPattern:  finalPath,
		OriginalPath: route.Path,
		Domain:       "",
		Category:     route.Category,
		Middlewares:  route.Middlewares,
		Input:        route.Input,
		Response:     route.Response,
		RouterType:   route.RouterType,
		RegisteredAt: time.Now(),
	}
	r.compiledRoutes[finalPath] = compiled
	r.routesMu.Unlock()

	// Wrap template rendering in handler if template exists
	if route.Template != nil {
		originalHandler := route.HandlerFunc
		route.HandlerFunc = func(w http.ResponseWriter, req *http.Request) {
			// Set Content-Type to prevent MIME sniffing attacks
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data := getInputData(route.Input, req)
			err := route.Template.Execute(w, data)
			if err != nil {
				r.logger.Error("Template render error", "template", route.TemplateName, "error", err)
				http.Error(w, "Template render error", http.StatusInternalServerError)
				return
			}
			if originalHandler != nil {
				originalHandler(w, req)
			}
		}
	}

	// Wrap handler with context enrichment
	enrichedHandler := r.wrapWithContextEnrichment(route.HandlerFunc, "", r.config.BasePath, r.config.Version, route)

	// Apply global middlewares first, then route-specific
	allMiddlewares := append(r.globalMiddlewares, route.Middlewares...)
	handler := r.chainMiddlewares(enrichedHandler, allMiddlewares)
	compiled.Handler = handler
	r.mux.Handle(finalPath, handler)

	// store route for documentation
	r.routes = append(r.routes, route)

	// Automatically register OPTIONS handler for CORS preflight
	if route.Method != "OPTIONS" {
		optionsPath := strings.Replace(finalPath, route.Method+" ", "OPTIONS ", 1)
		if !r.registeredOPTIONS[optionsPath] {
			// OPTIONS handler with same middlewares for CORS
			optionsHandler := r.chainMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
			}), allMiddlewares)
			r.mux.Handle(optionsPath, optionsHandler)
			r.registeredOPTIONS[optionsPath] = true
		}
	}

	r.logger.Debug("Route registered", "method", route.Method, "path", finalPath, "type", route.RouterType)
}

// RegisterGroup registers a group of routes with shared configuration
func (r *RouterImpl) RegisterGroup(group *RouteGroup) {
	if group == nil {
		return
	}

	for _, route := range group.Routes {
		// Use group category if route doesn't have one
		if route.Category == "" && group.Category != "" {
			route.Category = group.Category
		}

		// Prepend group prefix to route path
		if group.Prefix != "" {
			prefix := strings.TrimSuffix(group.Prefix, "/")
			route.Path = prefix + "/" + strings.TrimPrefix(route.Path, "/")
		}

		// Prepend group middlewares to route middlewares
		if len(group.Middlewares) > 0 {
			route.Middlewares = append(group.Middlewares, route.Middlewares...)
		}

		r.Register(route)
	}

	r.logger.Info("Route group registered", "prefix", group.Prefix, "routes", len(group.Routes))
}

// chainMiddlewares chains middlewares for a handler (bottom-up)
func (r *RouterImpl) chainMiddlewares(handler http.Handler, middlewares []MiddlewaresType) http.Handler {
	// Apply middlewares in reverse order so the first middleware wraps everything
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// =============================================================================
// Response Writer Wrapper for Metering
// =============================================================================

// responseWriterWrapper wraps http.ResponseWriter to capture status code and response size
// Implements http.Flusher, http.Hijacker, and http.Pusher for full compatibility
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

// responseWriterPool reduces allocations for high-throughput scenarios
var responseWriterPool = sync.Pool{
	New: func() any {
		return &responseWriterWrapper{}
	},
}

// acquireResponseWriter gets a response writer from the pool
func acquireResponseWriter(w http.ResponseWriter) *responseWriterWrapper {
	rw := responseWriterPool.Get().(*responseWriterWrapper)
	rw.ResponseWriter = w
	rw.statusCode = http.StatusOK
	rw.bytesWritten = 0
	rw.wroteHeader = false
	return rw
}

// releaseResponseWriter returns a response writer to the pool
func releaseResponseWriter(rw *responseWriterWrapper) {
	rw.ResponseWriter = nil
	responseWriterPool.Put(rw)
}

func newResponseWriterWrapper(w http.ResponseWriter) *responseWriterWrapper {
	return acquireResponseWriter(w)
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Unwrap supports http.ResponseController and middleware that need the underlying writer
func (rw *responseWriterWrapper) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Flush implements http.Flusher for streaming responses (SSE, chunked transfer)
func (rw *responseWriterWrapper) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket upgrades and protocol switching
func (rw *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported by underlying ResponseWriter")
}

// Push implements http.Pusher for HTTP/2 server push
func (rw *responseWriterWrapper) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// =============================================================================
// Built-in Security Middlewares
// =============================================================================

// recoveryMiddleware returns a middleware that recovers from panics
func (r *RouterImpl) recoveryMiddleware() MiddlewaresType {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Get stack trace
					stack := debug.Stack()

					// Log the panic with request context
					requestID := GetRequestID(req.Context())
					r.logger.Error("Panic recovered",
						"error", err,
						"request_id", requestID,
						"method", req.Method,
						"path", req.URL.Path,
						"stack", string(stack),
					)

					// Return 500 error
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)

					if r.config.Mode == "dev" {
						// Include error details in dev mode
						json.NewEncoder(w).Encode(map[string]any{
							"error":      "Internal Server Error",
							"message":    fmt.Sprintf("%v", err),
							"request_id": requestID,
						})
					} else {
						// Minimal info in prod
						json.NewEncoder(w).Encode(map[string]any{
							"error":      "Internal Server Error",
							"request_id": requestID,
						})
					}
				}
			}()
			next.ServeHTTP(w, req)
		})
	}
}

// requestIDMiddleware returns a middleware that generates/propagates request IDs
func (r *RouterImpl) requestIDMiddleware() MiddlewaresType {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Get or generate request ID
			requestID := req.Header.Get(r.config.Security.RequestIDHeader)
			if requestID == "" {
				requestID = generateRequestID()
			}

			// Add to response header
			w.Header().Set(r.config.Security.RequestIDHeader, requestID)

			// Add to context
			ctx := context.WithValue(req.Context(), ContextKeyRequestID, requestID)

			// Extract and add client IP
			clientIP := r.extractClientIP(req)
			ctx = context.WithValue(ctx, ContextKeyClientIP, clientIP)

			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}
}

// bodySizeLimitMiddleware returns a middleware that limits request body size
func (r *RouterImpl) bodySizeLimitMiddleware() MiddlewaresType {
	maxSize := r.config.Security.MaxRequestBodySize
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Limit the request body size
			req.Body = http.MaxBytesReader(w, req.Body, maxSize)
			next.ServeHTTP(w, req)
		})
	}
}

// shutdownAwareMiddleware rejects requests during shutdown
func (r *RouterImpl) shutdownAwareMiddleware() MiddlewaresType {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Check if shutting down
			if r.isShuttingDown.Load() {
				w.Header().Set("Connection", "close")
				w.Header().Set("Retry-After", "30")
				http.Error(w, "Service Unavailable - Shutting Down", http.StatusServiceUnavailable)
				return
			}

			// Track active request
			r.activeRequests.Add(1)
			defer r.activeRequests.Add(-1)

			next.ServeHTTP(w, req)
		})
	}
}

// extractClientIP extracts the real client IP, respecting trusted proxies
func (r *RouterImpl) extractClientIP(req *http.Request) string {
	// Get the remote address
	remoteIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		remoteIP = req.RemoteAddr
	}

	// Check if we should trust proxy headers
	if r.config.Security != nil && len(r.config.Security.trustedNets) > 0 {
		ip := net.ParseIP(remoteIP)
		if ip == nil {
			return remoteIP
		}

		// Check if remote IP is in trusted proxies
		isTrusted := false
		for _, trustedNet := range r.config.Security.trustedNets {
			if trustedNet.Contains(ip) {
				isTrusted = true
				break
			}
		}

		if isTrusted {
			// Check X-Forwarded-For header (first untrusted IP)
			if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
				ips := strings.Split(xff, ",")
				for i := len(ips) - 1; i >= 0; i-- {
					ip := strings.TrimSpace(ips[i])
					parsedIP := net.ParseIP(ip)
					if parsedIP == nil {
						continue
					}

					// Find the first non-trusted IP (rightmost)
					trusted := false
					for _, trustedNet := range r.config.Security.trustedNets {
						if trustedNet.Contains(parsedIP) {
							trusted = true
							break
						}
					}
					if !trusted {
						return ip
					}
				}
			}

			// Check X-Real-IP header
			if xRealIP := req.Header.Get("X-Real-IP"); xRealIP != "" {
				return strings.TrimSpace(xRealIP)
			}
		}
	}

	return remoteIP
}

// generateRequestID generates a cryptographically secure unique request ID
// Format: 32 hex characters (16 bytes of entropy)
func generateRequestID() string {
	b := make([]byte, RequestIDLength)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails (should never happen)
		// This provides less entropy but ensures the system continues working
		return fmt.Sprintf("%016x%016x", time.Now().UnixNano(), time.Now().UnixNano()^0xDEADBEEF)
	}
	return hex.EncodeToString(b)
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := GetFromContext[string](ctx, ContextKeyRequestID)
	return requestID
}

// GetClientIP retrieves the client IP from context
func GetClientIP(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	clientIP, _ := GetFromContext[string](ctx, ContextKeyClientIP)
	return clientIP
}

// =============================================================================
// Health Check Endpoints
// =============================================================================

// RegisterHealthCheck adds a health check to the router
func (r *RouterImpl) RegisterHealthCheck(check HealthCheck) {
	if check.Timeout == 0 {
		check.Timeout = 5 * time.Second
	}
	r.healthChecks = append(r.healthChecks, check)
	r.logger.Info("Health check registered", "name", check.Name)
}

// registerHealthEndpoints registers the health check endpoints
func (r *RouterImpl) registerHealthEndpoints() {
	if r.config.Health == nil || !r.config.Health.Enabled {
		return
	}

	// Liveness probe - just checks if the server is running
	r.mux.HandleFunc("GET "+r.config.Health.LivenessPath, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// If shutting down, report unhealthy
		if r.isShuttingDown.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(HealthStatus{
				Status:    "unhealthy",
				Timestamp: time.Now(),
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(HealthStatus{
			Status:    "healthy",
			Timestamp: time.Now(),
		})
	})

	// Readiness probe - checks all registered health checks
	r.mux.HandleFunc("GET "+r.config.Health.ReadinessPath, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		status := HealthStatus{
			Status:    "healthy",
			Checks:    make(map[string]CheckResult),
			Timestamp: time.Now(),
		}

		// If shutting down, report not ready
		if r.isShuttingDown.Load() {
			status.Status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(status)
			return
		}

		// Run all health checks
		allHealthy := true
		for _, check := range r.healthChecks {
			ctx, cancel := context.WithTimeout(req.Context(), check.Timeout)
			start := time.Now()
			err := check.Check(ctx)
			duration := time.Since(start)
			cancel()

			result := CheckResult{
				Status:   "healthy",
				Duration: duration / time.Millisecond,
			}

			if err != nil {
				result.Status = "unhealthy"
				result.Error = err.Error()
				allHealthy = false
			}

			status.Checks[check.Name] = result
		}

		if !allHealthy {
			status.Status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(status)
	})

	r.logger.Info("Health endpoints registered",
		"liveness", r.config.Health.LivenessPath,
		"readiness", r.config.Health.ReadinessPath,
	)
}

// SetMeteringHook sets the usage metering callback
func (r *RouterImpl) SetMeteringHook(hook MeteringHook) {
	r.meteringHook = hook
	r.logger.Info("Metering hook registered")
}

// =============================================================================
// Basic Metrics (for /metrics endpoint)
// =============================================================================

// routerMetrics holds basic metrics collectors with bounded storage to prevent memory leaks
type routerMetrics struct {
	requestsTotal    map[string]int64       // method:status -> count (bounded by HTTP methods * status codes)
	requestDuration  map[string]*ringBuffer // method:path -> durations (bounded ring buffer)
	requestsInFlight int64
	mu               sync.RWMutex
	endpointCount    int // Track unique endpoints to prevent unbounded growth
}

// ringBuffer is a fixed-size circular buffer for duration samples
type ringBuffer struct {
	data  []float64
	pos   int
	full  bool
	count int64 // Total count for this endpoint
}

// newRingBuffer creates a new ring buffer with fixed size
func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{
		data: make([]float64, size),
	}
}

// add adds a value to the ring buffer
func (rb *ringBuffer) add(value float64) {
	rb.data[rb.pos] = value
	rb.pos = (rb.pos + 1) % len(rb.data)
	if rb.pos == 0 {
		rb.full = true
	}
	rb.count++
}

// values returns all values in the buffer
func (rb *ringBuffer) values() []float64 {
	if rb.full {
		return rb.data
	}
	return rb.data[:rb.pos]
}

// newRouterMetrics creates a new metrics instance
func newRouterMetrics() *routerMetrics {
	return &routerMetrics{
		requestsTotal:   make(map[string]int64),
		requestDuration: make(map[string]*ringBuffer),
	}
}

// record records request metrics with bounded storage
func (m *routerMetrics) record(method, path string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Increment request counter (bounded by method * status combinations ~50 max)
	key := fmt.Sprintf("%s:%d", method, status)
	m.requestsTotal[key]++

	// Record duration in ring buffer (prevents unbounded memory growth)
	durationKey := fmt.Sprintf("%s:%s", method, path)

	// Check if we've hit the endpoint limit (prevent high-cardinality attack)
	if _, exists := m.requestDuration[durationKey]; !exists {
		if m.endpointCount >= MaxMetricsUniqueEndpoints {
			// Use aggregated bucket for overflow
			durationKey = fmt.Sprintf("%s:__overflow__", method)
		}
		if _, exists := m.requestDuration[durationKey]; !exists {
			m.requestDuration[durationKey] = newRingBuffer(MaxMetricsDurationSamples)
			m.endpointCount++
		}
	}

	m.requestDuration[durationKey].add(duration.Seconds())
}

// metricsMiddleware returns a middleware that records request metrics
// Uses sync.Pool for response wrapper to reduce GC pressure under high load
func (r *RouterImpl) metricsMiddleware() MiddlewaresType {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if r.metrics == nil {
				next.ServeHTTP(w, req)
				return
			}

			start := time.Now()
			atomic.AddInt64(&r.metrics.requestsInFlight, 1)
			defer atomic.AddInt64(&r.metrics.requestsInFlight, -1)

			// Acquire pooled response writer to capture status/bytes
			wrapped := acquireResponseWriter(w)

			// Serve the request
			next.ServeHTTP(wrapped, req)

			// Capture values BEFORE releasing to pool (prevents race condition)
			statusCode := wrapped.statusCode
			duration := time.Since(start)

			// Release back to pool immediately after capturing
			releaseResponseWriter(wrapped)

			// Record metrics with captured values (bounded cardinality)
			metricsPath := req.URL.Path
			if r.config.Metrics != nil && !r.config.Metrics.IncludePath {
				metricsPath = "aggregated"
			}
			r.metrics.record(req.Method, metricsPath, statusCode, duration)
		})
	}
}

// registerMetricsEndpoint registers the /metrics endpoint
func (r *RouterImpl) registerMetricsEndpoint() {
	if r.config.Metrics == nil || !r.config.Metrics.Enabled {
		return
	}

	path := r.config.Metrics.Path
	if path == "" {
		path = "/metrics"
	}

	r.mux.HandleFunc("GET "+path, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		namespace := r.config.Metrics.Namespace
		if namespace == "" {
			namespace = "router"
		}
		subsystem := r.config.Metrics.Subsystem
		if subsystem == "" {
			subsystem = "http"
		}

		r.metrics.mu.RLock()
		defer r.metrics.mu.RUnlock()

		// Output Prometheus-compatible metrics
		var sb strings.Builder

		// Requests total by method and status
		sb.WriteString(fmt.Sprintf("# HELP %s_%s_requests_total Total number of HTTP requests\n", namespace, subsystem))
		sb.WriteString(fmt.Sprintf("# TYPE %s_%s_requests_total counter\n", namespace, subsystem))
		for key, count := range r.metrics.requestsTotal {
			parts := strings.Split(key, ":")
			if len(parts) == 2 {
				sb.WriteString(fmt.Sprintf("%s_%s_requests_total{method=\"%s\",status=\"%s\"} %d\n",
					namespace, subsystem, parts[0], parts[1], count))
			}
		}

		// Requests in flight
		sb.WriteString(fmt.Sprintf("\n# HELP %s_%s_requests_in_flight Current number of HTTP requests being processed\n", namespace, subsystem))
		sb.WriteString(fmt.Sprintf("# TYPE %s_%s_requests_in_flight gauge\n", namespace, subsystem))
		sb.WriteString(fmt.Sprintf("%s_%s_requests_in_flight %d\n",
			namespace, subsystem, atomic.LoadInt64(&r.metrics.requestsInFlight)))

		// Active requests (from router)
		sb.WriteString(fmt.Sprintf("\n# HELP %s_%s_active_requests Current active requests tracked by router\n", namespace, subsystem))
		sb.WriteString(fmt.Sprintf("# TYPE %s_%s_active_requests gauge\n", namespace, subsystem))
		sb.WriteString(fmt.Sprintf("%s_%s_active_requests %d\n",
			namespace, subsystem, r.activeRequests.Load()))

		io.WriteString(w, sb.String())
	})

	r.logger.Info("Metrics endpoint registered", "path", path)
}

// checkRouteConflict detects duplicate route registrations
func (r *RouterImpl) checkRouteConflict(pattern string, domain string, route *Route) error {
	var routeMap map[string]*CompiledRoute
	if domain == "" {
		routeMap = r.compiledRoutes
	} else {
		if dh, ok := r.domains[domain]; ok {
			routeMap = dh.compiledRoutes
		} else {
			return nil // Domain doesn't exist yet, no conflict
		}
	}

	if existing, exists := routeMap[pattern]; exists {
		return &RouteConflictError{
			NewRoute:      pattern,
			ExistingRoute: existing.FullPattern,
			Domain:        domain,
			Message:       fmt.Sprintf("registered at %s", existing.RegisteredAt.Format(time.RFC3339)),
		}
	}
	return nil
}

// wrapWithContextEnrichment adds domain/tenant/route context to requests and handles metering
func (r *RouterImpl) wrapWithContextEnrichment(handler http.HandlerFunc, domain, basePath, version string, route *Route) http.HandlerFunc {
	// Safety check for nil handler
	if handler == nil {
		r.logger.Error("nil handler passed to wrapWithContextEnrichment",
			"method", route.Method,
			"path", route.Path,
			"category", route.Category)
		return func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "Internal Server Error: Handler not configured", http.StatusInternalServerError)
		}
	}

	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		start := time.Now()

		// Add domain information
		if domain != "" {
			ctx = context.WithValue(ctx, ContextKeyDomain, domain)
		}

		// Add tenant ID if available
		if route.TenantID != "" {
			ctx = context.WithValue(ctx, ContextKeyTenantID, route.TenantID)
		}

		// Add version and base path
		ctx = context.WithValue(ctx, ContextKeyVersion, version)
		ctx = context.WithValue(ctx, ContextKeyBasePath, basePath)

		// Add route info for debugging/logging
		routeInfo := map[string]string{
			"method":   route.Method,
			"path":     route.Path,
			"category": route.Category,
		}
		ctx = context.WithValue(ctx, ContextKeyRouteInfo, routeInfo)

		// Wrap response writer for metering if hook is set
		if r.meteringHook != nil {
			// Acquire pooled response writer
			wrappedWriter := acquireResponseWriter(w)

			// Serve the request
			handler(wrappedWriter, req.WithContext(ctx))

			// CRITICAL: Capture values BEFORE releasing to pool
			// This prevents race conditions where another goroutine reuses the wrapper
			statusCode := wrappedWriter.statusCode
			responseSizeBytes := wrappedWriter.bytesWritten
			requestSizeBytes := req.ContentLength
			duration := time.Since(start)

			// Release back to pool immediately
			releaseResponseWriter(wrappedWriter)

			// Create detached context for async metering
			// context.WithoutCancel preserves values but won't be cancelled when request ends
			// This ensures metering hooks can complete DB writes, HTTP calls, etc.
			meteringCtx := context.WithoutCancel(ctx)

			// Call metering hook asynchronously with captured values
			// This doesn't block the response and uses stack-captured values (safe)
			go func() {
				metrics := &RequestMetrics{
					TenantID:     route.TenantID,
					Domain:       domain,
					Method:       route.Method,
					Path:         route.Path,
					StatusCode:   statusCode,
					Duration:     duration,
					RequestSize:  requestSizeBytes,
					ResponseSize: responseSizeBytes,
					Timestamp:    start,
				}
				r.meteringHook(meteringCtx, metrics)
			}()
			return
		}

		// Call handler with enriched context (no metering)
		handler(w, req.WithContext(ctx))
	}
}

// initTLS initializes SNI-aware TLS configuration for multi-domain support
func (r *RouterImpl) initTLS() error {
	if r.config.TLS == nil || !r.config.TLS.Enabled {
		return nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	// Load domain-specific certificates for SNI
	if len(r.config.TLS.DomainCerts) > 0 {
		certs := make(map[string]*tls.Certificate)

		for domain, domainCert := range r.config.TLS.DomainCerts {
			cert, err := tls.LoadX509KeyPair(domainCert.CertFile, domainCert.KeyFile)
			if err != nil {
				r.logger.Error("Failed to load certificate for domain",
					"domain", domain,
					"error", err,
				)
				continue
			}
			certs[domain] = &cert
			domainCert.cert = &cert
			r.logger.Info("Loaded TLS certificate for domain", "domain", domain)
		}

		// SNI callback for domain-based certificate selection
		tlsConfig.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			serverName := strings.ToLower(hello.ServerName)

			// Exact match
			if cert, ok := certs[serverName]; ok {
				return cert, nil
			}

			// Wildcard match
			parts := strings.Split(serverName, ".")
			if len(parts) >= 2 {
				for i := 0; i < len(parts)-1; i++ {
					wildcard := "*." + strings.Join(parts[i+1:], ".")
					if cert, ok := certs[wildcard]; ok {
						return cert, nil
					}
				}
			}

			// Fallback to default certificate
			if r.config.TLS.CertFile != "" && r.config.TLS.KeyFile != "" {
				cert, err := tls.LoadX509KeyPair(r.config.TLS.CertFile, r.config.TLS.KeyFile)
				if err != nil {
					return nil, err
				}
				return &cert, nil
			}

			return nil, fmt.Errorf("no certificate found for domain: %s", serverName)
		}
	} else if r.config.TLS.CertFile != "" && r.config.TLS.KeyFile != "" {
		// Single certificate mode (legacy)
		cert, err := tls.LoadX509KeyPair(r.config.TLS.CertFile, r.config.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	r.tlsConfig = tlsConfig
	r.logger.Info("TLS initialized", "domains", len(r.config.TLS.DomainCerts))
	return nil
}

// GetFromContext retrieves a value from the request context
func GetFromContext[T any](ctx context.Context, key ContextKey) (T, bool) {
	val := ctx.Value(key)
	if val == nil {
		var zero T
		return zero, false
	}
	typed, ok := val.(T)
	return typed, ok
}

// GetDomain retrieves the domain from request context
func GetDomain(ctx context.Context) string {
	domain, _ := GetFromContext[string](ctx, ContextKeyDomain)
	return domain
}

// GetTenantID retrieves the tenant ID from request context
func GetTenantID(ctx context.Context) string {
	tenantID, _ := GetFromContext[string](ctx, ContextKeyTenantID)
	return tenantID
}

// GetRouteInfo retrieves route metadata from request context
func GetRouteInfo(ctx context.Context) map[string]string {
	info, _ := GetFromContext[map[string]string](ctx, ContextKeyRouteInfo)
	return info
}

// RegisterDomain registers a domain with its own routes and configuration
func (r *RouterImpl) RegisterDomain(domainConfig *DomainConfig) {
	if domainConfig == nil || domainConfig.Domain == "" {
		r.logger.Error("Domain configuration is nil or domain name is empty")
		return
	}

	// Create domain handler
	domainHandler := &DomainHandler{
		config:            domainConfig,
		mux:               http.NewServeMux(),
		registeredOPTIONS: make(map[string]bool),
		routes:            []*Route{},
		compiledRoutes:    make(map[string]*CompiledRoute),
	}

	// Set as default domain if specified
	if domainConfig.IsDefault {
		r.defaultDomain = domainConfig.Domain
		r.logger.Info("Default domain set", "domain", domainConfig.Domain)
	}

	// Load domain certificate if provided
	if domainConfig.Certificate != nil {
		if r.config.TLS == nil {
			r.config.TLS = &TLSConfig{Enabled: true, DomainCerts: make(map[string]*DomainCertificate)}
		}
		if r.config.TLS.DomainCerts == nil {
			r.config.TLS.DomainCerts = make(map[string]*DomainCertificate)
		}
		r.config.TLS.DomainCerts[domainConfig.Domain] = domainConfig.Certificate
		r.logger.Info("Domain certificate registered", "domain", domainConfig.Domain)
	}

	// Register individual routes for this domain
	for _, route := range domainConfig.Routes {
		r.registerRouteForDomain(domainHandler, route, domainConfig)
	}

	// Register route groups for this domain
	for _, group := range domainConfig.Groups {
		for _, route := range group.Routes {
			// Apply group settings
			if route.Category == "" && group.Category != "" {
				route.Category = group.Category
			}
			if group.Prefix != "" {
				prefix := strings.TrimSuffix(group.Prefix, "/")
				route.Path = prefix + "/" + strings.TrimPrefix(route.Path, "/")
			}
			if len(group.Middlewares) > 0 {
				route.Middlewares = append(group.Middlewares, route.Middlewares...)
			}
			r.registerRouteForDomain(domainHandler, route, domainConfig)
		}
	}

	// Store the domain handler
	r.routesMu.Lock()
	r.domains[domainConfig.Domain] = domainHandler
	r.routesMu.Unlock()

	r.logger.Info("Domain registered",
		"domain", domainConfig.Domain,
		"routes", len(domainHandler.routes),
		"isDefault", domainConfig.IsDefault,
		"tenantID", domainConfig.TenantID,
	)
}

// registerRouteForDomain registers a single route for a specific domain
func (r *RouterImpl) registerRouteForDomain(dh *DomainHandler, route *Route, domainConfig *DomainConfig) {
	route = r.newRoute(route)

	// Inherit tenant ID from domain config if not set on route
	if route.TenantID == "" && domainConfig.TenantID != "" {
		route.TenantID = domainConfig.TenantID
	}

	// Build path with domain-specific config
	basePath := r.config.BasePath
	version := r.config.Version
	if domainConfig.BasePath != "" {
		basePath = domainConfig.BasePath
	}
	if domainConfig.Version != "" {
		version = domainConfig.Version
	}

	finalPath := r.preparePathWithConfig(route, basePath, version)

	// Check for route conflicts within this domain (hold lock for entire operation to prevent race)
	dh.mu.Lock()
	defer dh.mu.Unlock()

	if existing, exists := dh.compiledRoutes[finalPath]; exists {
		err := &RouteConflictError{
			NewRoute:      finalPath,
			ExistingRoute: existing.FullPattern,
			Domain:        domainConfig.Domain,
			Message:       fmt.Sprintf("registered at %s", existing.RegisteredAt.Format(time.RFC3339)),
		}
		if r.config.Mode == "dev" {
			r.logger.Error("Route conflict detected", "error", err)
			panic(err)
		}
		r.logger.Warn("Route conflict detected, overwriting", "error", err)
		// Continue to overwrite the route in prod mode
	}

	// Create compiled route
	compiled := &CompiledRoute{
		Method:       route.Method,
		Path:         strings.TrimPrefix(finalPath, route.Method+" "),
		FullPattern:  finalPath,
		OriginalPath: route.Path,
		Domain:       domainConfig.Domain,
		Category:     route.Category,
		Middlewares:  route.Middlewares,
		Input:        route.Input,
		Response:     route.Response,
		RouterType:   route.RouterType,
		RegisteredAt: time.Now(),
	}
	dh.compiledRoutes[finalPath] = compiled

	// Wrap template rendering if needed
	if route.Template != nil {
		originalHandler := route.HandlerFunc
		route.HandlerFunc = func(w http.ResponseWriter, req *http.Request) {
			// Set Content-Type to prevent MIME sniffing attacks
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data := getInputData(route.Input, req)
			err := route.Template.Execute(w, data)
			if err != nil {
				r.logger.Error("Template render error", "template", route.TemplateName, "error", err)
				http.Error(w, "Template render error", http.StatusInternalServerError)
				return
			}
			if originalHandler != nil {
				originalHandler(w, req)
			}
		}
	}

	// Wrap with context enrichment
	enrichedHandler := r.wrapWithContextEnrichment(route.HandlerFunc, domainConfig.Domain, basePath, version, route)

	// Chain middlewares: global -> domain -> route
	allMiddlewares := append(r.globalMiddlewares, domainConfig.Middlewares...)
	allMiddlewares = append(allMiddlewares, route.Middlewares...)
	handler := r.chainMiddlewares(enrichedHandler, allMiddlewares)
	compiled.Handler = handler

	dh.mux.Handle(finalPath, handler)
	dh.routes = append(dh.routes, route)

	// Register OPTIONS handler for CORS
	if route.Method != "OPTIONS" {
		optionsPath := strings.Replace(finalPath, route.Method+" ", "OPTIONS ", 1)
		if !dh.registeredOPTIONS[optionsPath] {
			optionsHandler := r.chainMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
			}), allMiddlewares)
			dh.mux.Handle(optionsPath, optionsHandler)
			dh.registeredOPTIONS[optionsPath] = true
		}
	}

	r.logger.Debug("Route registered for domain",
		"domain", domainConfig.Domain,
		"method", route.Method,
		"path", finalPath,
		"tenantID", route.TenantID,
	)
}

// preparePathWithConfig builds the route path with custom base path and version
// Respects route flags: RawPath, SkipBasePath, SkipVersioning, and TemplateName
func (r *RouterImpl) preparePathWithConfig(route *Route, basePath, version string) string {
	method := strings.ToUpper(route.Method)

	// Handle root path
	if route.Path == "/" {
		return method + " /"
	}

	// Sanitize the path
	p := sanitizePath(route.Path)
	if p == "" {
		return method + " /"
	}

	// RawPath mode: use path exactly as provided
	if route.RawPath {
		return method + " /" + p
	}

	// Page routes: no base path or version prefix
	isPageRoute := route.TemplateName != "" ||
		route.RouterType == RouterTypePage ||
		(route.SkipBasePath && route.SkipVersioning)

	if isPageRoute {
		return method + " /" + p
	}

	// Build path with optional base path and version
	var pathBuilder strings.Builder
	pathBuilder.WriteString(method)
	pathBuilder.WriteString(" /")

	// Add base path unless skipped
	if !route.SkipBasePath && basePath != "" {
		cleanBase := sanitizePath(basePath)
		if cleanBase != "" {
			pathBuilder.WriteString(cleanBase)
		}
	}

	// Add version unless skipped
	if !route.SkipVersioning && version != "" {
		if pathBuilder.Len() > len(method)+2 {
			pathBuilder.WriteString("/")
		}
		pathBuilder.WriteString(version)
	}

	// Add the route path
	if pathBuilder.Len() > len(method)+2 {
		pathBuilder.WriteString("/")
	}
	pathBuilder.WriteString(p)

	return pathBuilder.String()
}

// matchDomain finds the appropriate domain handler for a host.
// Uses specificity-based matching: exact > longest wildcard > shorter wildcard > default
func (r *RouterImpl) matchDomain(host string) *DomainHandler {
	// Remove port from host safely (supports IPv6 like "[::1]:8080")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)

	r.routesMu.RLock()
	defer r.routesMu.RUnlock()

	// Exact match (highest priority)
	if dh, ok := r.domains[host]; ok {
		return dh
	}

	// Collect all matching wildcards with their specificity scores
	type wildcardMatch struct {
		domain      string
		handler     *DomainHandler
		specificity int // Higher = more specific (more parts in the domain)
	}

	var matches []wildcardMatch
	parts := strings.Split(host, ".")

	if len(parts) >= 2 {
		// Try progressively more general wildcards, but collect all matches
		for i := 0; i < len(parts)-1; i++ {
			wildcard := "*." + strings.Join(parts[i+1:], ".")
			if dh, ok := r.domains[wildcard]; ok {
				// Specificity = number of parts in the wildcard pattern
				// *.sub.example.com (3 parts) > *.example.com (2 parts)
				specificity := len(parts) - i
				matches = append(matches, wildcardMatch{
					domain:      wildcard,
					handler:     dh,
					specificity: specificity,
				})
			}
		}
	}

	// Sort by specificity (highest first) and return the most specific match
	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].specificity > matches[j].specificity
		})
		return matches[0].handler
	}

	// Fallback to default domain
	if r.defaultDomain != "" {
		if dh, ok := r.domains[r.defaultDomain]; ok {
			return dh
		}
	}

	return nil
}

// domainRouter returns a handler that routes based on the Host header
func (r *RouterImpl) domainRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Check for domain-specific handling
		if len(r.domains) > 0 {
			dh := r.matchDomain(req.Host)
			if dh != nil {
				// Handle domain redirect if configured
				if dh.config.RedirectTo != "" {
					redirectURL := req.URL
					redirectURL.Host = dh.config.RedirectTo
					if redirectURL.Scheme == "" {
						redirectURL.Scheme = r.config.Protocol
					}
					http.Redirect(w, req, redirectURL.String(), http.StatusMovedPermanently)
					return
				}
				dh.mux.ServeHTTP(w, req)
				return
			}
		}

		// Fallback to default mux
		r.mux.ServeHTTP(w, req)
	})
}

// GetDomains returns all registered domains
func (r *RouterImpl) GetDomains() []string {
	domains := make([]string, 0, len(r.domains))
	for domain := range r.domains {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

// GetDomainRoutes returns all routes for a specific domain
func (r *RouterImpl) GetDomainRoutes(domain string) []*Route {
	if dh, ok := r.domains[domain]; ok {
		return dh.routes
	}
	return nil
}

// Start starts the HTTP server
func (r *RouterImpl) Start() error {
	if r.mux == nil {
		r.mux = http.NewServeMux()
	}

	// Log server configuration
	r.logger.Info("Starting server",
		"mode", r.config.Mode,
		"basePath", r.config.BasePath,
		"version", r.config.Version,
		"port", r.config.Port,
		"health_checks", len(r.healthChecks),
	)

	if r.config.Mode == "dev" {
		r.logger.Info("Documentation available",
			"json", "/documentation/json",
			"html", "/documentation/html",
		)
	}

	if r.config.Health != nil && r.config.Health.Enabled {
		r.logger.Info("Health endpoints available",
			"liveness", r.config.Health.LivenessPath,
			"readiness", r.config.Health.ReadinessPath,
		)
	}

	// Use domain router if domains are registered, otherwise use default mux
	var handler http.Handler
	if len(r.domains) > 0 {
		handler = r.domainRouter()
		r.logger.Info("Domain-based routing enabled", "domains", len(r.domains))
	} else {
		handler = r.mux
	}

	// Determine max header bytes
	maxHeaderBytes := DefaultMaxHeaderBytes
	if r.config.Security != nil && r.config.Security.MaxHeaderBytes > 0 {
		maxHeaderBytes = r.config.Security.MaxHeaderBytes
	}

	// Create HTTP server with configured timeouts and security settings
	r.server = &http.Server{
		Addr:              ":" + r.config.Port,
		Handler:           handler,
		ReadTimeout:       r.config.ReadTimeout,
		WriteTimeout:      r.config.WriteTimeout,
		IdleTimeout:       r.config.IdleTimeout,
		ReadHeaderTimeout: r.config.ReadHeaderTimeout, // Prevents Slowloris attacks
		MaxHeaderBytes:    maxHeaderBytes,             // Prevents header-based DoS
		ErrorLog:          slog.NewLogLogger(r.logger.Handler(), slog.LevelError),
	}

	// Check if TLS is enabled
	if r.config.TLS != nil && r.config.TLS.Enabled {
		return r.startTLS()
	}

	// HTTP mode
	url := r.config.Protocol + "://" + r.config.Domain + ":" + r.config.Port
	r.logger.Info("Server started",
		"url", url,
		"protocol", "HTTP",
		"read_header_timeout", r.config.ReadHeaderTimeout,
		"max_header_bytes", maxHeaderBytes,
	)
	return r.server.ListenAndServe()
}

// startTLS starts the server with TLS/HTTPS, supporting SNI for multi-domain certificates
func (r *RouterImpl) startTLS() error {
	// Re-initialize TLS to pick up any domain certificates added after NewRouter
	if err := r.initTLS(); err != nil {
		return fmt.Errorf("failed to initialize TLS: %w", err)
	}

	// Use SNI-aware TLS config if available
	if r.tlsConfig != nil {
		r.server.TLSConfig = r.tlsConfig
	}

	// Validate at least one certificate source is available
	hasCerts := (r.config.TLS.CertFile != "" && r.config.TLS.KeyFile != "") ||
		len(r.config.TLS.DomainCerts) > 0 ||
		(r.config.TLS.ACME != nil && r.config.TLS.ACME.Enabled)

	if !hasCerts {
		return fmt.Errorf("TLS enabled but no certificates configured")
	}

	// Validate static certificate files exist if specified
	if r.config.TLS.CertFile != "" {
		if _, err := os.Stat(r.config.TLS.CertFile); err != nil {
			r.logger.Error("TLS certificate not found", "file", r.config.TLS.CertFile, "error", err)
			return err
		}
		if _, err := os.Stat(r.config.TLS.KeyFile); err != nil {
			r.logger.Error("TLS key not found", "file", r.config.TLS.KeyFile, "error", err)
			return err
		}
	}

	// Start HTTP to HTTPS redirect server in production
	if r.config.Mode == "prod" {
		go r.startHTTPRedirect()
	}

	// Start HTTPS server
	r.server.Addr = ":443"
	url := "https://" + r.config.Domain + ":443"
	r.logger.Info("Server started",
		"url", url,
		"protocol", "HTTPS",
		"sni_domains", len(r.config.TLS.DomainCerts),
	)

	// Use TLS config if SNI is configured, otherwise fall back to static cert
	if r.tlsConfig != nil && r.tlsConfig.GetCertificate != nil {
		// SNI mode - certificates are loaded dynamically
		return r.server.ListenAndServeTLS("", "")
	}

	// Static certificate mode
	return r.server.ListenAndServeTLS(r.config.TLS.CertFile, r.config.TLS.KeyFile)
}

// startHTTPRedirect starts HTTP redirect server (port 80 -> 443)
func (r *RouterImpl) startHTTPRedirect() {
	redirectHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		httpsURL := "https://" + req.Host + req.RequestURI
		http.Redirect(w, req, httpsURL, http.StatusMovedPermanently)
	})

	r.redirectServer = &http.Server{
		Addr:              ":80",
		Handler:           redirectHandler,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, // Prevent Slowloris on redirect server too
	}

	r.logger.Info("HTTP redirect server started", "port", "80")
	if err := r.redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		r.logger.Warn("HTTP redirect server failed", "error", err)
	}
}

// Shutdown gracefully shuts down the server
func (r *RouterImpl) Shutdown(timeout time.Duration) error {
	if r.server == nil {
		return nil
	}

	// Signal that we're shutting down
	r.isShuttingDown.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	r.logger.Info("Shutting down server",
		"timeout", timeout,
		"active_requests", r.activeRequests.Load(),
	)

	// Wait for active requests to complete (with timeout)
	waitStart := time.Now()
	for r.activeRequests.Load() > 0 {
		if time.Since(waitStart) > timeout/2 {
			r.logger.Warn("Timeout waiting for active requests",
				"remaining", r.activeRequests.Load(),
			)
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Shutdown the HTTP server
	if err := r.server.Shutdown(ctx); err != nil {
		r.logger.Error("Server shutdown failed", "error", err)
		return err
	}

	// Shutdown the HTTP redirect server if running
	if r.redirectServer != nil {
		if err := r.redirectServer.Shutdown(ctx); err != nil {
			r.logger.Warn("HTTP redirect server shutdown failed", "error", err)
		}
	}

	r.logger.Info("Server shutdown complete",
		"duration", time.Since(waitStart).String(),
	)
	return nil
}

// parseTemplates loads and parses HTML templates
func (r *RouterImpl) parseTemplates() error {
	if r.config.Templates == nil || !r.config.Templates.Enabled {
		return nil
	}

	templatesDir := r.config.Templates.Dir
	if templatesDir == "" {
		return nil
	}

	// Check if template directory exists
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		r.logger.Warn("Template directory does not exist", "dir", templatesDir)
		return err
	}

	// Use custom funcMap if provided, otherwise use defaults
	funcMap := r.config.Templates.FuncMap
	if funcMap == nil {
		funcMap = template.FuncMap{
			"add": func(a, b int) int {
				return a + b
			},
			"substr": func(s string, start, length int) string {
				if start < 0 || start >= len(s) {
					return ""
				}
				end := start + length
				if end > len(s) {
					end = len(s)
				}
				return s[start:end]
			},
			"upper": strings.ToUpper,
			"lower": strings.ToLower,
			"title": strings.ToUpper,
		}
	}

	// Create new template with funcMap
	tmpl := template.New("").Funcs(funcMap)

	// Collect all .html files recursively
	var templateFiles []string
	err := filepath.Walk(templatesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".html") {
			templateFiles = append(templateFiles, path)
		}
		return nil
	})

	if err != nil {
		r.logger.Error("Failed to walk template directory", "error", err)
		return err
	}

	if len(templateFiles) == 0 {
		r.logger.Warn("No templates found", "dir", templatesDir)
		return nil
	}

	// Parse all template files
	tmpl, err = tmpl.ParseFiles(templateFiles...)
	if err != nil {
		r.logger.Error("Failed to parse templates", "error", err)
		return err
	}

	r.templates = tmpl
	r.logger.Info("Templates loaded", "dir", templatesDir, "count", len(templateFiles))
	return nil
}

// registerDocumentation registers documentation routes (dev mode only)
func (r *RouterImpl) registerDocumentation() {
	r.logger.Info("Registering documentation routes")

	// JSON documentation route - includes both default and domain routes
	r.mux.HandleFunc("GET /documentation/json", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		docs := r.generateDocumentation()

		if err := json.NewEncoder(w).Encode(docs); err != nil {
			r.logger.Error("Failed to encode documentation", "error", err)
			http.Error(w, "Failed to generate documentation", http.StatusInternalServerError)
		}
	})

	// HTML documentation route
	r.mux.HandleFunc("GET /documentation/html", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		docs := r.generateDocumentation()

		// Group routes by category
		categoryMap := make(map[string][]map[string]any)
		for _, routeInfo := range docs.Routes {
			category := routeInfo["category"].(string)
			categoryMap[category] = append(categoryMap[category], routeInfo)
		}

		// Sort categories alphabetically
		var categories []string
		for category := range categoryMap {
			categories = append(categories, category)
		}
		sort.Strings(categories)

		data := map[string]any{
			"Categories":    categories,
			"Routes":        categoryMap,
			"Title":         "API Documentation",
			"Domains":       docs.Domains,
			"DefaultDomain": r.defaultDomain,
			"TotalRoutes":   docs.TotalRoutes,
		}

		if r.templates == nil {
			// Fallback to basic HTML if templates not enabled
			r.renderBasicDocsHTML(w, data)
			return
		}

		if err := r.templates.ExecuteTemplate(w, "docs", data); err != nil {
			r.logger.Error("Documentation template error", "error", err)
			r.renderBasicDocsHTML(w, data)
		}
	})
}

// DocumentationOutput represents the full API documentation
type DocumentationOutput struct {
	Routes      []map[string]any            `json:"routes"`
	Domains     map[string][]map[string]any `json:"domains,omitempty"`
	TotalRoutes int                         `json:"total_routes"`
	Version     string                      `json:"version"`
	BasePath    string                      `json:"base_path"`
}

// generateDocumentation creates comprehensive API documentation including domain routes
func (r *RouterImpl) generateDocumentation() *DocumentationOutput {
	docs := &DocumentationOutput{
		Routes:   make([]map[string]any, 0),
		Domains:  make(map[string][]map[string]any),
		Version:  r.config.Version,
		BasePath: r.config.BasePath,
	}

	// Add default routes
	for _, route := range r.routes {
		routeInfo := map[string]any{
			"domain":   "default",
			"category": route.Category,
			"method":   route.Method,
			"path":     r.preparePath(route),
			"type":     string(route.RouterType),
		}

		if route.Input != nil {
			routeInfo["input"] = *route.Input
			routeInfo["requires_authentication"] = route.Input.RequiredAuth
		}

		if route.Response != nil {
			routeInfo["response"] = route.Response
		}

		docs.Routes = append(docs.Routes, routeInfo)
	}

	// Add domain-specific routes
	r.routesMu.RLock()
	for domain, dh := range r.domains {
		domainRoutes := make([]map[string]any, 0)
		for _, route := range dh.routes {
			basePath := r.config.BasePath
			version := r.config.Version
			if dh.config.BasePath != "" {
				basePath = dh.config.BasePath
			}
			if dh.config.Version != "" {
				version = dh.config.Version
			}

			routeInfo := map[string]any{
				"domain":    domain,
				"category":  route.Category,
				"method":    route.Method,
				"path":      r.preparePathWithConfig(route, basePath, version),
				"type":      string(route.RouterType),
				"tenant_id": route.TenantID,
			}

			if route.Input != nil {
				routeInfo["input"] = *route.Input
				routeInfo["requires_authentication"] = route.Input.RequiredAuth
			}

			if route.Response != nil {
				routeInfo["response"] = route.Response
			}

			domainRoutes = append(domainRoutes, routeInfo)
			docs.Routes = append(docs.Routes, routeInfo)
		}
		docs.Domains[domain] = domainRoutes
	}
	r.routesMu.RUnlock()

	docs.TotalRoutes = len(docs.Routes)
	return docs
}

// renderBasicDocsHTML renders a basic HTML documentation page without templates
func (r *RouterImpl) renderBasicDocsHTML(w http.ResponseWriter, data map[string]any) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <style>
        body { font-family: system-ui, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { color: #333; border-bottom: 2px solid #007bff; padding-bottom: 10px; }
        h2 { color: #555; margin-top: 30px; }
        .route { background: white; padding: 15px; margin: 10px 0; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .method { display: inline-block; padding: 4px 8px; border-radius: 4px; font-weight: bold; margin-right: 10px; }
        .GET { background: #28a745; color: white; }
        .POST { background: #007bff; color: white; }
        .PUT { background: #ffc107; color: black; }
        .DELETE { background: #dc3545; color: white; }
        .PATCH { background: #17a2b8; color: white; }
        .path { font-family: monospace; font-size: 14px; }
        .domain-badge { background: #6c757d; color: white; padding: 2px 6px; border-radius: 3px; font-size: 12px; margin-left: 10px; }
        .auth-required { color: #dc3545; font-size: 12px; margin-left: 10px; }
        .summary { background: #e9ecef; padding: 15px; border-radius: 8px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>API Documentation</h1>
        <div class="summary">
            <strong>Total Routes:</strong> %d |
            <strong>Domains:</strong> %d |
            <strong>Version:</strong> %s
        </div>
`
	categories := data["Categories"].([]string)
	routes := data["Routes"].(map[string][]map[string]any)
	totalRoutes := data["TotalRoutes"].(int)
	domains := data["Domains"].(map[string][]map[string]any)

	fmt.Fprintf(w, html, totalRoutes, len(domains), r.config.Version)

	for _, category := range categories {
		if category == "" {
			category = "Uncategorized"
		}
		fmt.Fprintf(w, "<h2>%s</h2>\n", category)
		for _, route := range routes[category] {
			method := route["method"].(string)
			path := route["path"].(string)
			domain := ""
			if d, ok := route["domain"].(string); ok && d != "default" {
				domain = d
			}
			authRequired := false
			if auth, ok := route["requires_authentication"].(bool); ok {
				authRequired = auth
			}

			fmt.Fprintf(w, `<div class="route">
                <span class="method %s">%s</span>
                <span class="path">%s</span>`, method, method, path)
			if domain != "" {
				fmt.Fprintf(w, `<span class="domain-badge">%s</span>`, domain)
			}
			if authRequired {
				fmt.Fprintf(w, `<span class="auth-required">🔒 Auth Required</span>`)
			}
			fmt.Fprintf(w, "</div>\n")
		}
	}

	fmt.Fprintf(w, "</div></body></html>")
}

// getInputData extracts input data from the request safely.
// SECURITY: This function does NOT perform any external I/O (no file reads, no HTTP fetches).
// External data fetching should be done explicitly in application handlers, not in the router.
func getInputData(src *RouteInput, req *http.Request) map[string]string {
	if src == nil {
		return map[string]string{}
	}

	data := map[string]string{}

	// Extract query parameters (safe: reads from request URL only)
	for key, source := range src.QueryParameters {
		if strings.HasPrefix(source, "_") {
			// Static value (prefixed with underscore)
			data[key] = strings.TrimPrefix(source, "_")
		} else {
			// Dynamic value from query string
			data[key] = req.URL.Query().Get(source)
		}
	}

	// Extract body/form parameters (safe: reads from request body only)
	for key, source := range src.Body {
		if strings.HasPrefix(source, "_") {
			// Static value
			data[key] = strings.TrimPrefix(source, "_")
		} else {
			// Form value from request body
			v := req.FormValue(source)
			data[key] = v
		}
	}

	// Extract headers (safe: reads from request headers only)
	for key, source := range src.Headers {
		data[key] = req.Header.Get(source)
	}

	// Extract path parameters using Go 1.22+ PathValue
	for key, source := range src.PathParameters {
		data[key] = req.PathValue(source)
	}

	// Extract form data (safe: reads from request only)
	for key, source := range src.FormData {
		data[key] = req.FormValue(source)
	}

	return data
}
