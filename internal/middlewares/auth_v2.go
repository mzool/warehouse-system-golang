package middlewares

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Session-Based Authentication Middleware
// Uses secure signed tokens stored in HttpOnly cookies
// Session data cached in Redis for performance

// ============================================================================
// Core Types
// ============================================================================

// contextKey is a custom type for context keys to avoid collisions
type sessionContextKey string

const (
	sessionUserKey sessionContextKey = "session_user"
	sessionIDKey   sessionContextKey = "session_id"
)

// SessionConfig holds configuration for session-based authentication
type SessionConfig struct {
	// SessionStore handles session storage (Redis implementation)
	SessionStore SessionStore

	// RolePermissionProvider fetches roles and permissions from database
	RolePermissionProvider RolePermissionProvider

	// Secret key for signing session tokens (HMAC)
	SecretKey []byte

	// Cookie configuration
	CookieName     string
	CookiePath     string
	CookieDomain   string
	CookieSecure   bool // Set to true in production (HTTPS only)
	CookieHTTPOnly bool
	CookieSameSite http.SameSite

	// Session duration
	SessionDuration time.Duration

	// Cache duration for roles/permissions
	RoleCacheDuration time.Duration

	// Logger for debugging
	Logger *slog.Logger

	// ErrorHandler handles authentication errors
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

	// Skipper defines paths to skip authentication
	SkipPaths []string

	// RequiredRoles for this endpoint (optional)
	RequiredRoles []string

	// RequiredPermissions for this endpoint (optional)
	RequiredPermissions []string

	// PolicyFunc for custom authorization logic (most flexible)
	PolicyFunc func(*UserSession) bool

	// SessionActivityUpdateTimeout for background updates
	SessionActivityUpdateTimeout time.Duration

	// RoleCacheUpdateTimeout for background role cache updates
	RoleCacheUpdateTimeout time.Duration
}

// SessionData represents cached session information in Redis
type SessionData struct {
	UserID       string    `json:"user_id"` // Always string for consistency
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	IsActive     bool      `json:"is_active"`    // Session active/revoked
	AuthVersion  int       `json:"auth_version"` // Increment on role/permission changes
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastAccessAt time.Time `json:"last_access_at"` // For sliding expiration
	IPAddress    string    `json:"ip_address"`     // Security: track IP
	UserAgent    string    `json:"user_agent"`     // Security: track user agent
}

// UserSession represents the complete user session with roles and permissions
type UserSession struct {
	SessionID   string
	UserID      string
	Email       string
	Username    string
	Roles       []string
	Permissions []string
	AuthVersion int // Current auth version from session
	ExpiresAt   time.Time
	MetaData    map[string]interface{}
}

// SessionStore interface for session storage (Redis implementation)
type SessionStore interface {
	// SaveSession saves session data to store
	SaveSession(ctx context.Context, sessionID string, data *SessionData, ttl time.Duration) error

	// GetSession retrieves session data from store
	GetSession(ctx context.Context, sessionID string) (*SessionData, error)

	// DeleteSession removes session from store (logout/revoke)
	DeleteSession(ctx context.Context, sessionID string) error

	// RevokeUserSessions revokes all sessions for a user (e.g., after password change)
	RevokeUserSessions(ctx context.Context, userID string) error

	// UpdateSessionActivity updates last access time (sliding expiration)
	UpdateSessionActivity(ctx context.Context, sessionID string) error

	// GetUserRolesPermissions retrieves cached roles and permissions
	// Returns (roles, permissions, authVersion, found, error)
	GetUserRolesPermissions(ctx context.Context, userID string) ([]string, []string, int, bool, error)

	// CacheUserRolesPermissions caches roles and permissions with auth version
	CacheUserRolesPermissions(ctx context.Context, userID string, roles, permissions []string, authVersion int, ttl time.Duration) error

	// GetUserAuthVersion retrieves user's current auth version from database
	GetUserAuthVersion(ctx context.Context, userID string) (int, error)
}

// RolePermissionProvider interface for fetching roles/permissions from database
type RolePermissionProvider interface {
	// GetUserRolesAndPermissions fetches user roles and permissions from database
	// Returns roles, permissions, authVersion
	GetUserRolesAndPermissions(ctx context.Context, userID string) (roles []string, permissions []string, authVersion int, err error)
}

// Credentials represents login credentials
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ============================================================================
// Session Token Generation and Validation
// ============================================================================

// generateSessionToken creates a cryptographically secure random token
func generateSessionToken() (string, error) {
	// Generate 32 bytes (256 bits) of random data
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Encode to base64 URL-safe format
	token := base64.URLEncoding.EncodeToString(b)
	return token, nil
}

// signToken creates HMAC signature for the session token
func signToken(token string, secretKey []byte) string {
	h := hmac.New(sha256.New, secretKey)
	h.Write([]byte(token))
	signature := h.Sum(nil)
	return base64.URLEncoding.EncodeToString(signature)
}

// createSignedSessionToken creates a signed session token
func createSignedSessionToken(secretKey []byte) (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}

	signature := signToken(token, secretKey)
	// Format: token.signature
	signedToken := token + "." + signature
	return signedToken, nil
}

// validateSignedToken validates the signature of a session token
func validateSignedToken(signedToken string, secretKey []byte) (string, error) {
	parts := strings.Split(signedToken, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid token format")
	}

	token := parts[0]
	providedSig := parts[1]

	// Calculate expected signature
	expectedSig := signToken(token, secretKey)

	// Constant-time comparison to prevent timing attacks
	if !hmac.Equal([]byte(providedSig), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid token signature")
	}

	return token, nil
}

// ============================================================================
// Session Management Functions
// ============================================================================

// CreateSession creates a new session for authenticated user
func CreateSession(ctx context.Context, config *SessionConfig, userID, email, username string, authVersion int, r *http.Request) (string, error) {
	if config.SessionStore == nil {
		return "", fmt.Errorf("session store not configured")
	}

	// Generate signed session token
	signedToken, err := createSignedSessionToken(config.SecretKey)
	if err != nil {
		config.Logger.Error("Failed to generate session token", "error", err)
		return "", fmt.Errorf("failed to create session")
	}

	// Create session data
	now := time.Now()
	sessionData := &SessionData{
		UserID:       userID,
		Email:        email,
		Username:     username,
		IsActive:     true,
		AuthVersion:  authVersion,
		CreatedAt:    now,
		ExpiresAt:    now.Add(config.SessionDuration),
		LastAccessAt: now,
		IPAddress:    getClientIP(r),
		UserAgent:    r.UserAgent(),
	}

	// Save to store
	if err := config.SessionStore.SaveSession(ctx, signedToken, sessionData, config.SessionDuration); err != nil {
		config.Logger.Error("Failed to save session", "error", err, "user_id", userID)
		return "", fmt.Errorf("failed to create session")
	}

	config.Logger.Info("Session created", "user_id", userID, "email", email, "ip", sessionData.IPAddress, "auth_version", authVersion)
	return signedToken, nil
}

// RevokeSession revokes a specific session
func RevokeSession(ctx context.Context, config *SessionConfig, sessionToken string) error {
	if config.SessionStore == nil {
		return fmt.Errorf("session store not configured")
	}

	// Validate token signature first
	_, err := validateSignedToken(sessionToken, config.SecretKey)
	if err != nil {
		return fmt.Errorf("invalid session token")
	}

	if err := config.SessionStore.DeleteSession(ctx, sessionToken); err != nil {
		config.Logger.Error("Failed to revoke session", "error", err)
		return fmt.Errorf("failed to revoke session")
	}

	config.Logger.Info("Session revoked", "session_token", maskToken(sessionToken))
	return nil
}

// RevokeUserSessions revokes all sessions for a user
func RevokeUserSessions(ctx context.Context, config *SessionConfig, userID string) error {
	if config.SessionStore == nil {
		return fmt.Errorf("session store not configured")
	}

	if err := config.SessionStore.RevokeUserSessions(ctx, userID); err != nil {
		config.Logger.Error("Failed to revoke user sessions", "error", err, "user_id", userID)
		return fmt.Errorf("failed to revoke sessions")
	}

	config.Logger.Info("All user sessions revoked", "user_id", userID)
	return nil
}

// ============================================================================
// Authentication Functions
// ============================================================================

// AuthenticateUser verifies credentials and creates session
func AuthenticateUser(ctx context.Context, config *SessionConfig, email, password string, getUserByEmail func(ctx context.Context, email string) (userID, username, hashedPassword string, authVersion int, err error), r *http.Request) (string, error) {
	// Fetch user from database
	userID, username, hashedPassword, authVersion, err := getUserByEmail(ctx, email)
	if err != nil {
		config.Logger.Warn("Login attempt for non-existent user", "email", email)
		// Don't reveal if user exists or not
		return "", fmt.Errorf("invalid credentials")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)); err != nil {
		config.Logger.Warn("Failed login attempt", "email", email, "ip", getClientIP(r))
		return "", fmt.Errorf("invalid credentials")
	}

	// Create session with auth version
	sessionToken, err := CreateSession(ctx, config, userID, email, username, authVersion, r)
	if err != nil {
		return "", err
	}

	return sessionToken, nil
}

// ============================================================================
// Middleware
// ============================================================================

// DefaultSessionConfig returns default session configuration
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		CookieName:                   "session",
		CookiePath:                   "/",
		CookieSecure:                 true, // Set to true in production
		CookieHTTPOnly:               true,
		CookieSameSite:               http.SameSiteLaxMode,
		SessionDuration:              24 * time.Hour,
		RoleCacheDuration:            15 * time.Minute,
		SessionActivityUpdateTimeout: 2 * time.Second,
		RoleCacheUpdateTimeout:       3 * time.Second,
		Logger:                       slog.Default(),
		ErrorHandler:                 defaultSessionErrorHandler,
		SkipPaths:                    []string{},
		RequiredRoles:                []string{},
		RequiredPermissions:          []string{},
	}
}

// defaultSessionErrorHandler is the default error handler
func defaultSessionErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	// Don't expose internal errors to client
	response := map[string]interface{}{
		"error":   "Unauthorized",
		"message": "Authentication required",
	}

	json.NewEncoder(w).Encode(response)
}

// SessionAuth returns a session-based authentication middleware
func SessionAuth(config *SessionConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultSessionConfig()
	}

	// Set defaults
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = defaultSessionErrorHandler
	}
	if config.CookieName == "" {
		config.CookieName = "session"
	}
	if config.SessionDuration == 0 {
		config.SessionDuration = 24 * time.Hour
	}
	if config.RoleCacheDuration == 0 {
		config.RoleCacheDuration = 15 * time.Minute
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Skip authentication for specific paths
			for _, skipPath := range config.SkipPaths {
				if r.URL.Path == skipPath || strings.HasPrefix(r.URL.Path, skipPath) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Extract session token from cookie
			cookie, err := r.Cookie(config.CookieName)
			if err != nil {
				config.Logger.Debug("No session cookie found", "path", r.URL.Path)
				config.ErrorHandler(w, r, fmt.Errorf("authentication required"))
				return
			}

			sessionToken := cookie.Value
			if sessionToken == "" {
				config.Logger.Debug("Empty session token", "path", r.URL.Path)
				config.ErrorHandler(w, r, fmt.Errorf("authentication required"))
				return
			}

			// Validate token signature
			_, err = validateSignedToken(sessionToken, config.SecretKey)
			if err != nil {
				config.Logger.Warn("Invalid session signature", "error", err, "path", r.URL.Path)
				config.ErrorHandler(w, r, fmt.Errorf("invalid session"))
				return
			}

			// Get session data from store
			sessionData, err := config.SessionStore.GetSession(ctx, sessionToken)
			if err != nil {
				config.Logger.Debug("Session not found in store", "error", err)
				config.ErrorHandler(w, r, fmt.Errorf("session expired or invalid"))
				return
			}

			// Verify session is active
			if !sessionData.IsActive {
				config.Logger.Info("Revoked session attempted", "user_id", sessionData.UserID)
				config.ErrorHandler(w, r, fmt.Errorf("session has been revoked"))
				return
			}

			// Check expiration
			if time.Now().After(sessionData.ExpiresAt) {
				config.Logger.Info("Expired session attempted", "user_id", sessionData.UserID)
				config.ErrorHandler(w, r, fmt.Errorf("session has expired"))
				return
			}

			// Security: Verify IP address hasn't changed (optional, can be disabled for mobile users)
			// Uncomment if you want strict IP checking
			// currentIP := getClientIP(r)
			// if sessionData.IPAddress != currentIP {
			// 	config.Logger.Warn("Session IP mismatch", "user_id", sessionData.UserID, "original", sessionData.IPAddress, "current", currentIP)
			// 	config.ErrorHandler(w, r, fmt.Errorf("session security violation"))
			// 	return
			// }

			// Update last access time (sliding expiration) - with timeout control
			go func() {
				timeout := config.SessionActivityUpdateTimeout
				if timeout == 0 {
					timeout = 2 * time.Second
				}
				updateCtx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				if err := config.SessionStore.UpdateSessionActivity(updateCtx, sessionToken); err != nil {
					config.Logger.Debug("Failed to update session activity", "error", err)
				}
			}()

			// Get or fetch roles and permissions
			roles, permissions, currentAuthVersion, err := getRolesAndPermissions(ctx, config, sessionData.UserID)
			if err != nil {
				config.Logger.Error("Failed to get user roles/permissions", "error", err, "user_id", sessionData.UserID)
				config.ErrorHandler(w, r, fmt.Errorf("authorization failed"))
				return
			}

			// Check auth version mismatch (role/permission change requires re-login)
			if currentAuthVersion > 0 && sessionData.AuthVersion > 0 && currentAuthVersion != sessionData.AuthVersion {
				config.Logger.Warn("Auth version mismatch - permissions changed",
					"user_id", sessionData.UserID,
					"session_version", sessionData.AuthVersion,
					"current_version", currentAuthVersion,
				)
				config.ErrorHandler(w, r, fmt.Errorf("permissions updated, please login again"))
				return
			}

			// Check required roles
			if len(config.RequiredRoles) > 0 {
				if !hasAnyRole(roles, config.RequiredRoles) {
					config.Logger.Warn("Insufficient role permissions",
						"user_id", sessionData.UserID,
						"user_roles", roles,
						"required_roles", config.RequiredRoles,
					)
					config.ErrorHandler(w, r, fmt.Errorf("insufficient permissions"))
					return
				}
			}

			// Check required permissions
			if len(config.RequiredPermissions) > 0 {
				if !hasAnyPermission(permissions, config.RequiredPermissions) {
					config.Logger.Warn("Insufficient permissions",
						"user_id", sessionData.UserID,
						"user_permissions", permissions,
						"required_permissions", config.RequiredPermissions,
					)
					config.ErrorHandler(w, r, fmt.Errorf("insufficient permissions"))
					return
				}
			}

			// Check custom policy function (most flexible)
			if config.PolicyFunc != nil {
				// Create temporary session for policy check
				tempSession := &UserSession{
					UserID:      sessionData.UserID,
					Email:       sessionData.Email,
					Username:    sessionData.Username,
					Roles:       roles,
					Permissions: permissions,
					AuthVersion: currentAuthVersion,
				}
				if !config.PolicyFunc(tempSession) {
					config.Logger.Warn("Policy check failed", "user_id", sessionData.UserID)
					config.ErrorHandler(w, r, fmt.Errorf("access denied"))
					return
				}
			}

			// Create user session object
			userSession := &UserSession{
				SessionID:   sessionToken,
				UserID:      sessionData.UserID,
				Email:       sessionData.Email,
				Username:    sessionData.Username,
				Roles:       roles,
				Permissions: permissions,
				AuthVersion: currentAuthVersion,
				ExpiresAt:   sessionData.ExpiresAt,
				MetaData: map[string]interface{}{
					"created_at":     sessionData.CreatedAt,
					"last_access_at": sessionData.LastAccessAt,
					"ip_address":     sessionData.IPAddress,
					"user_agent":     sessionData.UserAgent,
				},
			}

			// Add to context
			ctx = context.WithValue(ctx, sessionUserKey, userSession)
			ctx = context.WithValue(ctx, sessionIDKey, sessionToken)
			r = r.WithContext(ctx)

			config.Logger.Debug("Session validated",
				"user_id", sessionData.UserID,
				"email", sessionData.Email,
				"roles", roles,
			)

			next.ServeHTTP(w, r)
		})
	}
}

// getRolesAndPermissions fetches roles/permissions with caching and auth version
func getRolesAndPermissions(ctx context.Context, config *SessionConfig, userID string) ([]string, []string, int, error) {
	// Try cache first
	if config.SessionStore != nil {
		roles, permissions, authVersion, found, err := config.SessionStore.GetUserRolesPermissions(ctx, userID)
		if err == nil && found {
			config.Logger.Debug("Roles/permissions retrieved from cache",
				"user_id", userID,
				"roles_count", len(roles),
				"perms_count", len(permissions),
				"auth_version", authVersion,
			)
			return roles, permissions, authVersion, nil
		}
	}

	// Cache miss - fetch from database
	if config.RolePermissionProvider == nil {
		return []string{}, []string{}, 0, nil // No provider = no roles/perms
	}

	roles, permissions, authVersion, err := config.RolePermissionProvider.GetUserRolesAndPermissions(ctx, userID)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to fetch roles/permissions: %w", err)
	}

	// Cache the result with timeout control
	if config.SessionStore != nil {
		go func() {
			timeout := config.RoleCacheUpdateTimeout
			if timeout == 0 {
				timeout = 3 * time.Second
			}
			cacheCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if err := config.SessionStore.CacheUserRolesPermissions(cacheCtx, userID, roles, permissions, authVersion, config.RoleCacheDuration); err != nil {
				config.Logger.Debug("Failed to cache roles/permissions", "error", err, "user_id", userID)
			}
		}()
	}

	config.Logger.Debug("Roles/permissions fetched from database",
		"user_id", userID,
		"roles_count", len(roles),
		"perms_count", len(permissions),
		"auth_version", authVersion,
	)
	return roles, permissions, authVersion, nil
}

// ============================================================================
// Context Helpers
// ============================================================================

// GetSessionFromContext retrieves user session from request context
func GetSessionFromContext(r *http.Request) (*UserSession, bool) {
	session, ok := r.Context().Value(sessionUserKey).(*UserSession)
	return session, ok
}

// GetSessionIDFromContext retrieves session ID from request context
func GetSessionIDFromContext(r *http.Request) (string, bool) {
	sessionID, ok := r.Context().Value(sessionIDKey).(string)
	return sessionID, ok
}

// MustGetSession retrieves session or panics (use in handlers after auth middleware)
func MustGetSession(r *http.Request) *UserSession {
	session, ok := GetSessionFromContext(r)
	if !ok {
		panic("session not found in context - did you forget auth middleware?")
	}
	return session
}

// ============================================================================
// Helper Functions
// ============================================================================

// HasRole checks if user has a specific role
func (u *UserSession) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission checks if user has a specific permission
func (u *UserSession) HasPermission(permission string) bool {
	for _, p := range u.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// HasAnyRole checks if user has any of the specified roles
func (u *UserSession) HasAnyRole(roles []string) bool {
	return hasAnyRole(u.Roles, roles)
}

// HasAllRoles checks if user has all of the specified roles
func (u *UserSession) HasAllRoles(roles []string) bool {
	for _, role := range roles {
		if !u.HasRole(role) {
			return false
		}
	}
	return true
}

// hasAnyRole checks if user has any of the specified roles (internal helper)
func hasAnyRole(userRoles, requiredRoles []string) bool {
	for _, required := range requiredRoles {
		for _, userRole := range userRoles {
			if userRole == required {
				return true
			}
		}
	}
	return false
}

// hasAnyPermission checks if user has any of the specified permissions (internal helper)
func hasAnyPermission(userPerms, requiredPerms []string) bool {
	for _, required := range requiredPerms {
		for _, userPerm := range userPerms {
			if userPerm == required {
				return true
			}
		}
	}
	return false
}

// maskToken masks a token for logging (security)
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// ============================================================================
// Cookie Helpers
// ============================================================================

// SetSessionCookie sets the session cookie
func SetSessionCookie(w http.ResponseWriter, config *SessionConfig, sessionToken string) {
	cookie := &http.Cookie{
		Name:     config.CookieName,
		Value:    sessionToken,
		Path:     config.CookiePath,
		Domain:   config.CookieDomain,
		MaxAge:   int(config.SessionDuration.Seconds()),
		Secure:   config.CookieSecure,
		HttpOnly: config.CookieHTTPOnly,
		SameSite: config.CookieSameSite,
	}
	http.SetCookie(w, cookie)
}

// DeleteSessionCookie deletes the session cookie (logout)
func DeleteSessionCookie(w http.ResponseWriter, config *SessionConfig) {
	cookie := &http.Cookie{
		Name:     config.CookieName,
		Value:    "",
		Path:     config.CookiePath,
		Domain:   config.CookieDomain,
		MaxAge:   -1, // Delete immediately
		Secure:   config.CookieSecure,
		HttpOnly: config.CookieHTTPOnly,
		SameSite: config.CookieSameSite,
	}
	http.SetCookie(w, cookie)
}

// ============================================================================
// Convenience Middleware Builders
// ============================================================================

// cloneConfig creates a copy of SessionConfig to prevent data races
func cloneConfig(c *SessionConfig) *SessionConfig {
	newConfig := *c

	// Deep copy slices to prevent mutations
	if len(c.SkipPaths) > 0 {
		newConfig.SkipPaths = make([]string, len(c.SkipPaths))
		copy(newConfig.SkipPaths, c.SkipPaths)
	}
	if len(c.RequiredRoles) > 0 {
		newConfig.RequiredRoles = make([]string, len(c.RequiredRoles))
		copy(newConfig.RequiredRoles, c.RequiredRoles)
	}
	if len(c.RequiredPermissions) > 0 {
		newConfig.RequiredPermissions = make([]string, len(c.RequiredPermissions))
		copy(newConfig.RequiredPermissions, c.RequiredPermissions)
	}

	return &newConfig
}

// RequireRole creates a session auth middleware that requires specific roles (race-safe)
func RequireRole(config *SessionConfig, roles ...string) func(next http.Handler) http.Handler {
	c := cloneConfig(config)
	c.RequiredRoles = roles
	return SessionAuth(c)
}

// RequirePermission creates a session auth middleware that requires specific permissions (race-safe)
func RequirePermission(config *SessionConfig, permissions ...string) func(next http.Handler) http.Handler {
	c := cloneConfig(config)
	c.RequiredPermissions = permissions
	return SessionAuth(c)
}

// RequireRoleAndPermission creates middleware requiring both roles and permissions (race-safe)
func RequireRoleAndPermission(config *SessionConfig, roles, permissions []string) func(next http.Handler) http.Handler {
	c := cloneConfig(config)
	c.RequiredRoles = roles
	c.RequiredPermissions = permissions
	return SessionAuth(c)
}

// RequirePolicy creates middleware with custom policy-based authorization (most flexible)
func RequirePolicy(config *SessionConfig, policyFunc func(*UserSession) bool) func(next http.Handler) http.Handler {
	c := cloneConfig(config)
	c.PolicyFunc = policyFunc
	return SessionAuth(c)
}

// ============================================================================
// Password Hashing Helpers
// ============================================================================

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	// Use cost 12 for good security/performance balance
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword verifies a password against a hash
func VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// ============================================================================
// User ID Conversion Helpers
// ============================================================================

// UserIDToString converts various user ID types to string
func UserIDToString(id interface{}) string {
	switch v := id.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		return fmt.Sprintf("%v", id)
	}
}
