package middlewares

import (
	"log/slog"
	"net/http"
	"strconv"
)

// SecurityConfig holds configuration for security headers middleware
type SecurityConfig struct {
	// Logger for structured logging (optional, uses slog.Default if nil)
	Logger *slog.Logger

	// XSSProtection provides protection against XSS attacks
	// Default: "1; mode=block"
	XSSProtection string

	// ContentTypeNosniff prevents browsers from MIME-sniffing
	// Default: "nosniff"
	ContentTypeNosniff string

	// XFrameOptions prevents clickjacking attacks
	// Values: "DENY", "SAMEORIGIN", "ALLOW-FROM uri"
	// Default: "DENY"
	XFrameOptions string

	// HSTSMaxAge sets HTTP Strict Transport Security max age
	// Default: 31536000 (1 year)
	HSTSMaxAge int

	// HSTSIncludeSubdomains includes subdomains in HSTS policy
	// Default: false
	HSTSIncludeSubdomains bool

	// HSTSPreload enables HSTS preload list inclusion
	// Default: false
	HSTSPreload bool

	// ContentSecurityPolicy sets CSP header
	// Default: "default-src 'self'"
	ContentSecurityPolicy string

	// ReferrerPolicy controls referrer information
	// Default: "strict-origin-when-cross-origin"
	ReferrerPolicy string

	// PermissionsPolicy controls browser features
	// Default: "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()"
	PermissionsPolicy string

	// CrossOriginEmbedderPolicy controls cross-origin embedding
	// Default: "require-corp"
	CrossOriginEmbedderPolicy string

	// CrossOriginOpenerPolicy controls cross-origin windows
	// Default: "same-origin"
	CrossOriginOpenerPolicy string

	// CrossOriginResourcePolicy controls cross-origin resource sharing
	// Default: "same-origin"
	CrossOriginResourcePolicy string

	// Skipper defines a function to skip middleware
	Skipper func(r *http.Request) bool
}

// DefaultSecurityConfig returns a default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		XSSProtection:             "1; mode=block",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "DENY",
		HSTSMaxAge:                31536000, // 1 year
		HSTSIncludeSubdomains:     false,
		HSTSPreload:               false,
		ContentSecurityPolicy:     "default-src 'self'",
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		PermissionsPolicy:         "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()",
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		Skipper:                   nil,
	}
}

// Security returns a middleware that sets security headers
func Security(config *SecurityConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Log middleware initialization
	logger.Debug("security headers middleware initialized",
		"hsts_max_age", config.HSTSMaxAge,
		"hsts_include_subdomains", config.HSTSIncludeSubdomains,
		"x_frame_options", config.XFrameOptions,
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				logger.Debug("security headers skipped",
					"method", r.Method,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			// X-XSS-Protection
			if config.XSSProtection != "" {
				w.Header().Set("X-XSS-Protection", config.XSSProtection)
			}

			// X-Content-Type-Options
			if config.ContentTypeNosniff != "" {
				w.Header().Set("X-Content-Type-Options", config.ContentTypeNosniff)
			}

			// X-Frame-Options
			if config.XFrameOptions != "" {
				w.Header().Set("X-Frame-Options", config.XFrameOptions)
			}

			// Strict-Transport-Security (only for HTTPS)
			if r.TLS != nil && config.HSTSMaxAge > 0 {
				value := "max-age=" + strconv.Itoa(config.HSTSMaxAge)
				if config.HSTSIncludeSubdomains {
					value += "; includeSubDomains"
				}
				if config.HSTSPreload {
					value += "; preload"
				}
				w.Header().Set("Strict-Transport-Security", value)
			}

			// Content-Security-Policy
			if config.ContentSecurityPolicy != "" {
				w.Header().Set("Content-Security-Policy", config.ContentSecurityPolicy)
			}

			// Referrer-Policy
			if config.ReferrerPolicy != "" {
				w.Header().Set("Referrer-Policy", config.ReferrerPolicy)
			}

			// Permissions-Policy
			if config.PermissionsPolicy != "" {
				w.Header().Set("Permissions-Policy", config.PermissionsPolicy)
				logger.Debug("security headers applied",
					"method", r.Method,
					"path", r.URL.Path,
					"is_tls", r.TLS != nil,
				)

			}

			// Cross-Origin-Embedder-Policy
			if config.CrossOriginEmbedderPolicy != "" {
				w.Header().Set("Cross-Origin-Embedder-Policy", config.CrossOriginEmbedderPolicy)
			}

			// Cross-Origin-Opener-Policy
			if config.CrossOriginOpenerPolicy != "" {
				w.Header().Set("Cross-Origin-Opener-Policy", config.CrossOriginOpenerPolicy)
			}

			// Cross-Origin-Resource-Policy
			if config.CrossOriginResourcePolicy != "" {
				w.Header().Set("Cross-Origin-Resource-Policy", config.CrossOriginResourcePolicy)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ProductionSecurityConfig returns a production-ready security configuration
func ProductionSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		XSSProtection:             "1; mode=block",
		ContentTypeNosniff:        "nosniff",
		XFrameOptions:             "DENY",
		HSTSMaxAge:                63072000, // 2 years
		HSTSIncludeSubdomains:     true,
		HSTSPreload:               true,
		ContentSecurityPolicy:     "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; media-src 'self'; object-src 'none'; child-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'",
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		PermissionsPolicy:         "accelerometer=(), ambient-light-sensor=(), autoplay=(), battery=(), camera=(), cross-origin-isolated=(), display-capture=(), document-domain=(), encrypted-media=(), execution-while-not-rendered=(), execution-while-out-of-viewport=(), fullscreen=(), geolocation=(), gyroscope=(), keyboard-map=(), magnetometer=(), microphone=(), midi=(), navigation-override=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), sync-xhr=(), usb=(), web-share=(), xr-spatial-tracking=()",
		CrossOriginEmbedderPolicy: "require-corp",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		Skipper:                   nil,
	}
}
