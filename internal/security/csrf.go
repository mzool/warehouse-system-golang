package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"warehouse_system/internal/cache"
)

var (
	ErrNoSession = errors.New("no valid session found")
)

// CSRFProtection provides CSRF token generation and validation
// Production-ready with cache backend support (Redis/Memory) and rate limiting
type CSRFProtection struct {
	config *CSRFConfig
	cache  cache.Cache
	logger *slog.Logger
}

// CSRFConfig holds CSRF protection configuration
type CSRFConfig struct {
	// Cache backend for token storage (Redis recommended for production)
	// If nil, falls back to memory cache (not recommended for multi-server deployments)
	Cache cache.Cache

	// Token length in bytes (default: 32)
	TokenLength int

	// Token lifetime (default: 24 hours)
	TokenLifetime time.Duration

	// Cookie name for CSRF token
	CookieName string

	// Header name for CSRF token
	HeaderName string

	// Form field name for CSRF token
	FieldName string

	// Cookie path
	CookiePath string

	// Cookie domain
	CookieDomain string

	// Secure cookie (HTTPS only)
	CookieSecure bool

	// SameSite cookie attribute (Lax recommended for better UX, Strict for max security)
	CookieSameSite http.SameSite

	// Logger for structured logging
	Logger *slog.Logger

	// Skip CSRF check for these methods
	SafeMethods []string

	// Custom error handler
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

	// Rate limiting configuration
	RateLimit *CSRFRateLimitConfig

	// Key prefix for cache storage
	KeyPrefix string

	// Session cookie name (for linking tokens to sessions)
	SessionCookieName string

	// Require valid session (reject requests without session)
	RequireSession bool

	// Skipper defines a function to skip CSRF protection for specific requests
	// Useful for webhooks, API endpoints with Bearer tokens, etc.
	// Return true to skip CSRF validation for the request
	Skipper func(r *http.Request) bool
}

// CSRFRateLimitConfig holds rate limiting configuration for CSRF operations
type CSRFRateLimitConfig struct {
	// Enable rate limiting
	Enabled bool

	// Maximum token generation attempts per key per window
	MaxAttempts int

	// Time window for rate limiting
	Window time.Duration

	// Key generator for rate limiting (default: IP-based)
	KeyGenerator func(r *http.Request) string
}

// DefaultCSRFConfig returns a production-ready CSRF configuration
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		Cache:             nil, // Must be provided by application
		TokenLength:       32,
		TokenLifetime:     24 * time.Hour,
		CookieName:        "csrf_token",
		HeaderName:        "X-CSRF-Token",
		FieldName:         "csrf_token",
		CookiePath:        "/",
		CookieDomain:      "",
		CookieSecure:      true,
		CookieSameSite:    http.SameSiteLaxMode, // Lax for better UX
		Logger:            nil,
		SafeMethods:       []string{"GET", "HEAD", "OPTIONS", "TRACE"},
		ErrorHandler:      nil,
		KeyPrefix:         "csrf:",
		SessionCookieName: "session_id",
		RequireSession:    true,
		RateLimit: &CSRFRateLimitConfig{
			Enabled:     true,
			MaxAttempts: 100, // 100 token generations per IP per window
			Window:      time.Minute,
			KeyGenerator: func(r *http.Request) string {
				return getRealIP(r)
			},
		},
	}
}

// NewCSRFProtection creates a new CSRF protection instance
// If cache is not provided, falls back to memory cache (NOT recommended for production)
func NewCSRFProtection(config *CSRFConfig) *CSRFProtection {
	if config == nil {
		config = DefaultCSRFConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Use provided cache or fallback to memory cache with warning
	cacheBackend := config.Cache
	if cacheBackend == nil {
		logger.Warn("CSRF using memory cache - not suitable for multi-server deployments",
			"recommendation", "use Redis cache for production")

		memCache := cache.NewMemoryCache(&cache.Config{
			DefaultTTL: config.TokenLifetime,
			Prefix:     config.KeyPrefix,
			Enabled:    true,
		})
		cacheBackend = memCache
	}

	return &CSRFProtection{
		config: config,
		cache:  cacheBackend,
		logger: logger,
	}
}

// Middleware returns a middleware that protects against CSRF attacks
func (c *CSRFProtection) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request should skip CSRF protection
		if c.config.Skipper != nil && c.config.Skipper(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Check if method is safe
		if c.isSafeMethod(r.Method) {
			// Check rate limiting for token generation
			if c.config.RateLimit != nil && c.config.RateLimit.Enabled {
				if !c.checkRateLimit(r) {
					c.logger.Warn("CSRF rate limit exceeded",
						"ip", getRealIP(r),
						"path", r.URL.Path,
					)
					http.Error(w, "Too many requests", http.StatusTooManyRequests)
					return
				}
			}

			// Generate and set token for safe methods
			token, err := c.generateToken(r)
			if err != nil {
				c.logger.Error("failed to generate CSRF token", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			c.setTokenCookie(w, token)

			// Store token in context
			ctx := context.WithValue(r.Context(), csrfTokenKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Validate token for unsafe methods
		if err := c.validateToken(r); err != nil {
			c.logger.Warn("CSRF validation failed",
				"error", err,
				"method", r.Method,
				"path", r.URL.Path,
				"ip", getRealIP(r),
				"user_agent", r.UserAgent(),
			)

			if c.config.ErrorHandler != nil {
				c.config.ErrorHandler(w, r, err)
			} else {
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
			}
			return
		}

		// Validation passed
		next.ServeHTTP(w, r)
	})
}

// GenerateToken generates a new CSRF token for a request
func (c *CSRFProtection) GenerateToken(r *http.Request) (string, error) {
	return c.generateToken(r)
}

// GetToken retrieves the CSRF token from the request context
func GetCSRFToken(r *http.Request) string {
	token, ok := r.Context().Value(csrfTokenKey).(string)
	if !ok {
		return ""
	}
	return token
}

// Internal methods

func (c *CSRFProtection) generateToken(r *http.Request) (string, error) {
	// Get session ID - required if RequireSession is true
	sessionID := c.getSessionID(r)
	if c.config.RequireSession && sessionID == "" {
		return "", ErrNoSession
	}

	// Generate cryptographically secure token
	token, err := GenerateToken(c.config.TokenLength)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	// Store token in cache with session ID as key
	cacheKey := c.tokenCacheKey(sessionID)
	tokenData := &csrfTokenData{
		Token:     token,
		CreatedAt: time.Now(),
		UserAgent: r.UserAgent(),
		IP:        getRealIP(r),
	}

	jsonData, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token data: %w", err)
	}

	ctx := r.Context()
	if err := c.cache.Set(ctx, cacheKey, jsonData, c.config.TokenLifetime); err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	return token, nil
}

func (c *CSRFProtection) validateToken(r *http.Request) error {
	// Get session ID
	sessionID := c.getSessionID(r)
	if sessionID == "" {
		return ErrNoSession
	}

	// Get token from request
	requestToken := c.getRequestToken(r)
	if requestToken == "" {
		return ErrInvalidToken
	}

	// Get stored token from cache
	cacheKey := c.tokenCacheKey(sessionID)
	ctx := r.Context()

	jsonData, err := c.cache.Get(ctx, cacheKey)
	if err != nil {
		return ErrInvalidToken
	}

	var tokenData csrfTokenData
	if err := json.Unmarshal(jsonData, &tokenData); err != nil {
		return ErrInvalidToken
	}

	// Constant-time comparison to prevent timing attacks
	if !SecureCompare(requestToken, tokenData.Token) {
		return ErrInvalidToken
	}

	// Optional: Check if IP or User-Agent changed (security measure)
	// Disabled by default as it can cause issues with legitimate use cases
	// if tokenData.IP != getRealIP(r) {
	//     return ErrTokenMismatch
	// }

	return nil
}

func (c *CSRFProtection) getRequestToken(r *http.Request) string {
	// Try header first (for AJAX/API requests)
	if token := r.Header.Get(c.config.HeaderName); token != "" {
		return token
	}

	// Try form field (for traditional form submissions)
	if err := r.ParseForm(); err == nil {
		if token := r.FormValue(c.config.FieldName); token != "" {
			return token
		}
	}

	return ""
}

func (c *CSRFProtection) setTokenCookie(w http.ResponseWriter, token string) {
	cookie := &http.Cookie{
		Name:     c.config.CookieName,
		Value:    token,
		Path:     c.config.CookiePath,
		Domain:   c.config.CookieDomain,
		MaxAge:   int(c.config.TokenLifetime.Seconds()),
		Secure:   c.config.CookieSecure,
		HttpOnly: false, // JavaScript needs to read this for AJAX requests
		SameSite: c.config.CookieSameSite,
	}

	http.SetCookie(w, cookie)
}

func (c *CSRFProtection) getSessionID(r *http.Request) string {
	// Get session ID from cookie
	cookie, err := r.Cookie(c.config.SessionCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// No fallback to IP - require proper session
	return ""
}

func (c *CSRFProtection) isSafeMethod(method string) bool {
	for _, safe := range c.config.SafeMethods {
		if method == safe {
			return true
		}
	}
	return false
}

func (c *CSRFProtection) tokenCacheKey(sessionID string) string {
	return c.config.KeyPrefix + "token:" + sessionID
}

func (c *CSRFProtection) rateLimitKey(r *http.Request) string {
	keyGen := c.config.RateLimit.KeyGenerator
	if keyGen == nil {
		keyGen = func(r *http.Request) string { return getRealIP(r) }
	}
	return c.config.KeyPrefix + "ratelimit:" + keyGen(r)
}

func (c *CSRFProtection) checkRateLimit(r *http.Request) bool {
	if c.config.RateLimit == nil || !c.config.RateLimit.Enabled {
		return true
	}

	ctx := r.Context()
	key := c.rateLimitKey(r)

	// Get current count
	count, err := c.cache.Increment(ctx, key, 1)
	if err != nil {
		c.logger.Error("rate limit check failed", "error", err)
		return true // Fail open
	}

	// Set expiration on first increment
	if count == 1 {
		c.cache.Expire(ctx, key, c.config.RateLimit.Window)
	}

	return count <= int64(c.config.RateLimit.MaxAttempts)
}

// getRealIP extracts the real IP address from request
// Handles X-Forwarded-For and X-Real-IP headers
func getRealIP(r *http.Request) string {
	// Check X-Forwarded-For header (from proxy/load balancer)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		for idx := 0; idx < len(xff); idx++ {
			if xff[idx] == ',' {
				return xff[:idx]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fallback to RemoteAddr
	return r.RemoteAddr
}

// csrfTokenData holds token metadata for security validation
type csrfTokenData struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
}

// Context key for CSRF token
type contextKey string

const csrfTokenKey contextKey = "csrf_token"

// CSRFTokenHTML returns HTML for a hidden CSRF token field
func CSRFTokenHTML(token string) string {
	return `<input type="hidden" name="csrf_token" value="` + token + `">`
}

// CSRFTokenMeta returns HTML meta tag for CSRF token
func CSRFTokenMeta(token string) string {
	return `<meta name="csrf-token" content="` + token + `">`
}

// CSRFTokenJSON returns JSON representation of CSRF token
func CSRFTokenJSON(token string) string {
	return fmt.Sprintf(`{"csrf_token":"%s"}`, token)
}
