package middlewares

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig holds configuration for CORS middleware
// Follows v2 design: no global dependencies, explicit configuration
type CORSConfig struct {
	// AllowOrigins defines a list of origins that may access the resource.
	// Supports wildcards: ["*"] or specific origins: ["https://example.com"]
	// Supports wildcard subdomains: ["*.example.com"]
	// Default: ["*"]
	AllowOrigins []string

	// AllowMethods defines methods allowed when accessing the resource.
	// Default: ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"]
	AllowMethods []string

	// AllowHeaders defines request headers that can be used.
	// If empty, echoes back Access-Control-Request-Headers from preflight
	// Common: ["Content-Type", "Authorization", "X-Request-ID"]
	// Default: [] (echo back)
	AllowHeaders []string

	// ExposeHeaders defines response headers clients can access.
	// Common: ["X-Request-ID", "X-Total-Count"]
	// Default: []
	ExposeHeaders []string

	// AllowCredentials indicates if credentials (cookies, auth) are allowed.
	// IMPORTANT: Cannot use with AllowOrigins = ["*"]
	// Default: false
	AllowCredentials bool

	// MaxAge indicates how long (seconds) preflight results can be cached.
	// Recommended: 3600 (1 hour) for production
	// Default: 0 (no cache)
	MaxAge int

	// Logger for structured logging
	// Default: slog.Default()
	Logger *slog.Logger

	// Skipper defines a function to skip middleware for specific requests.
	// Useful for health checks, metrics endpoints, etc.
	// Default: nil (always apply)
	Skipper func(r *http.Request) bool
}

// DefaultCORSConfig returns a secure default CORS configuration
// Suitable for development - adjust for production
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowHeaders:     []string{}, // Echo back requested headers
		ExposeHeaders:    []string{},
		AllowCredentials: false,
		MaxAge:           0,
		Logger:           slog.Default(),
		Skipper:          nil,
	}
}

// CORS returns a Cross-Origin Resource Sharing (CORS) middleware
// Implements RFC 6454 (Origin) and CORS specification
func CORS(config *CORSConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultCORSConfig()
	}

	// Set defaults
	if len(config.AllowOrigins) == 0 {
		config.AllowOrigins = []string{"*"}
	}
	if len(config.AllowMethods) == 0 {
		config.AllowMethods = []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		}
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	// Security check: credentials with wildcard origin is insecure
	if config.AllowCredentials && len(config.AllowOrigins) == 1 && config.AllowOrigins[0] == "*" {
		config.Logger.Warn("CORS: AllowCredentials with wildcard origin (*) is insecure and will not work - specify exact origins")
		config.AllowCredentials = false
	}

	// Pre-compute header values (performance optimization)
	allowMethods := strings.Join(config.AllowMethods, ", ")
	allowHeaders := strings.Join(config.AllowHeaders, ", ")
	exposeHeaders := strings.Join(config.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(config.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")

			// No Origin header = same-origin request, no CORS needed
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if origin is allowed
			allowedOrigin := getAllowedOrigin(origin, config.AllowOrigins)
			if allowedOrigin == "" {
				// Origin not allowed - deny preflight, log warning for actual requests
				if r.Method == http.MethodOptions {
					config.Logger.Debug("CORS preflight denied", "origin", origin, "path", r.URL.Path)
					w.WriteHeader(http.StatusForbidden)
					return
				}
				config.Logger.Debug("CORS request from disallowed origin", "origin", origin, "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}

			// Set allowed origin (specific origin or "*")
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)

			// Vary header is critical for caching security
			// Tells caches that response varies based on Origin header
			w.Header().Add("Vary", "Origin")

			// Set credentials header (only if allowed and not wildcard)
			if config.AllowCredentials && allowedOrigin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight OPTIONS request
			if r.Method == http.MethodOptions {
				requestMethod := r.Header.Get("Access-Control-Request-Method")
				requestHeaders := r.Header.Get("Access-Control-Request-Headers")

				config.Logger.Debug("CORS preflight",
					"origin", origin,
					"method", requestMethod,
					"headers", requestHeaders,
				)

				// Set allowed methods
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)

				// Set allowed headers
				if allowHeaders != "" {
					// Use configured allowed headers
					w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				} else if requestHeaders != "" {
					// Echo back requested headers if none configured
					w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
				}

				// Vary on request method and headers for preflight caching
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")

				// Set preflight cache duration
				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", maxAge)
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Handle actual request - set expose headers
			if exposeHeaders != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getAllowedOrigin checks if origin is allowed and returns the value to set
// Returns:
// - "*" if wildcard is configured
// - specific origin if matched
// - "" if not allowed
func getAllowedOrigin(origin string, allowOrigins []string) string {
	for _, allowed := range allowOrigins {
		if allowed == "*" {
			return "*"
		}
		if allowed == origin {
			return origin
		}
		// Support wildcard subdomains: *.example.com
		if strings.HasPrefix(allowed, "*.") {
			domain := allowed[1:] // Remove "*"
			if strings.HasSuffix(origin, domain) {
				return origin
			}
		}
	}
	return ""
}
