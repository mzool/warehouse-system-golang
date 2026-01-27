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
	"warehouse_system/internal/cache"

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
	// Cache handles all caching (sessions, roles, permissions)
	Cache cache.Cache

	// RolePermissionProvider fetches roles and permissions from database
	RolePermissionProvider RolePermissionProvider

	// Secret key for signing session tokens (HMAC)
	SecretKey []byte

	// Cache key patterns
	SessionKeyPrefix     string // Default: "session:"
	RoleKeyPrefix        string // Default: "user:roles:"
	PermissionKeyPrefix  string // Default: "user:perms:"
	AuthVersionKeyPrefix string // Default: "user:authver:"

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

// ============================================================================
// Cache Helper Functions
// ============================================================================

// saveSession saves session data to cache
func saveSession(ctx context.Context, c cache.Cache, keyPrefix, sessionID string, data *SessionData, ttl time.Duration) error {
	key := keyPrefix + sessionID
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}
	return c.Set(ctx, key, jsonData, ttl)
}

// getSession retrieves session data from cache
func getSession(ctx context.Context, c cache.Cache, keyPrefix, sessionID string) (*SessionData, error) {
	key := keyPrefix + sessionID
	data, err := c.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}
	return &session, nil
}

// deleteSession removes session from cache
func deleteSession(ctx context.Context, c cache.Cache, keyPrefix, sessionID string) error {
	key := keyPrefix + sessionID
	return c.Delete(ctx, key)
}

// revokeUserSessions revokes all sessions for a user
func revokeUserSessions(ctx context.Context, c cache.Cache, keyPrefix, userID string) error {
	// Find all sessions for this user
	pattern := keyPrefix + "*"
	keys, err := c.Keys(ctx, pattern)
	if err != nil {
		return err
	}

	// Filter sessions belonging to this user
	var userKeys []string
	for _, key := range keys {
		data, err := c.Get(ctx, key)
		if err != nil {
			continue
		}
		var session SessionData
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}
		if session.UserID == userID {
			userKeys = append(userKeys, key)
		}
	}

	if len(userKeys) > 0 {
		return c.DeleteMulti(ctx, userKeys)
	}
	return nil
}

// updateSessionActivity updates last access time
func updateSessionActivity(ctx context.Context, c cache.Cache, keyPrefix, sessionID string) error {
	session, err := getSession(ctx, c, keyPrefix, sessionID)
	if err != nil {
		return err
	}
	session.LastAccessAt = time.Now()
	return saveSession(ctx, c, keyPrefix, sessionID, session, 0) // 0 = keep existing TTL
}

// getUserRolesPermissions retrieves cached roles and permissions
func getUserRolesPermissions(ctx context.Context, c cache.Cache, rolePrefix, permPrefix, authVerPrefix, userID string) ([]string, []string, int, bool, error) {
	roleKey := rolePrefix + userID
	permKey := permPrefix + userID
	authVerKey := authVerPrefix + userID

	// Get all data
	results, err := c.GetMulti(ctx, []string{roleKey, permKey, authVerKey})
	if err != nil {
		return nil, nil, 0, false, err
	}

	// Check if all keys exist
	roleData, roleExists := results[roleKey]
	permData, permExists := results[permKey]
	authVerData, authVerExists := results[authVerKey]

	if !roleExists || !permExists {
		return nil, nil, 0, false, nil
	}

	var roles, permissions []string
	if err := json.Unmarshal(roleData, &roles); err != nil {
		return nil, nil, 0, false, err
	}
	if err := json.Unmarshal(permData, &permissions); err != nil {
		return nil, nil, 0, false, err
	}

	var authVersion int
	if authVerExists {
		if err := json.Unmarshal(authVerData, &authVersion); err != nil {
			authVersion = 0
		}
	}

	return roles, permissions, authVersion, true, nil
}

// cacheUserRolesPermissions caches roles and permissions
func cacheUserRolesPermissions(ctx context.Context, c cache.Cache, rolePrefix, permPrefix, authVerPrefix, userID string, roles, permissions []string, authVersion int, ttl time.Duration) error {
	roleKey := rolePrefix + userID
	permKey := permPrefix + userID
	authVerKey := authVerPrefix + userID

	roleData, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	permData, err := json.Marshal(permissions)
	if err != nil {
		return err
	}
	authVerData, err := json.Marshal(authVersion)
	if err != nil {
		return err
	}

	items := map[string][]byte{
		roleKey:    roleData,
		permKey:    permData,
		authVerKey: authVerData,
	}

	return c.SetMulti(ctx, items, ttl)
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

// SimpleSessionCreator is a simplified config for creating sessions in handlers
type SimpleSessionCreator struct {
	Cache            cache.Cache
	SecretKey        []byte
	SessionKeyPrefix string        // Default: "session:"
	SessionDuration  time.Duration // Default: 24h
	Logger           *slog.Logger
}

// NewSession creates a new session with simplified config (for use in login handlers)
func NewSession(ctx context.Context, creator *SimpleSessionCreator, userID, email, username string, authVersion int, r *http.Request) (string, error) {
	if creator.Cache == nil {
		return "", fmt.Errorf("cache not configured")
	}

	// Set defaults
	if creator.SessionKeyPrefix == "" {
		creator.SessionKeyPrefix = "session:"
	}
	if creator.SessionDuration == 0 {
		creator.SessionDuration = 24 * time.Hour
	}
	if creator.Logger == nil {
		creator.Logger = slog.Default()
	}

	// Generate signed session token
	signedToken, err := createSignedSessionToken(creator.SecretKey)
	if err != nil {
		creator.Logger.Error("Failed to generate session token", "error", err)
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
		ExpiresAt:    now.Add(creator.SessionDuration),
		LastAccessAt: now,
		IPAddress:    getClientIP(r),
		UserAgent:    r.UserAgent(),
	}

	// Save to cache
	if err := saveSession(ctx, creator.Cache, creator.SessionKeyPrefix, signedToken, sessionData, creator.SessionDuration); err != nil {
		creator.Logger.Error("Failed to save session", "error", err, "user_id", userID)
		return "", fmt.Errorf("failed to create session")
	}

	creator.Logger.Info("Session created", "user_id", userID, "email", email, "ip", sessionData.IPAddress, "auth_version", authVersion)
	return signedToken, nil
}

// CreateSession creates a new session for authenticated user (legacy, kept for backward compatibility)
func CreateSession(ctx context.Context, config *SessionConfig, userID, email, username string, authVersion int, r *http.Request) (string, error) {
	if config.Cache == nil {
		return "", fmt.Errorf("cache not configured")
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

	// Save to cache
	if err := saveSession(ctx, config.Cache, config.SessionKeyPrefix, signedToken, sessionData, config.SessionDuration); err != nil {
		config.Logger.Error("Failed to save session", "error", err, "user_id", userID)
		return "", fmt.Errorf("failed to create session")
	}

	config.Logger.Info("Session created", "user_id", userID, "email", email, "ip", sessionData.IPAddress, "auth_version", authVersion)
	return signedToken, nil
}

// RevokeSession revokes a specific session
func RevokeSession(ctx context.Context, config *SessionConfig, sessionToken string) error {
	if config.Cache == nil {
		return fmt.Errorf("cache not configured")
	}

	// Validate token signature first
	_, err := validateSignedToken(sessionToken, config.SecretKey)
	if err != nil {
		return fmt.Errorf("invalid session token")
	}

	if err := deleteSession(ctx, config.Cache, config.SessionKeyPrefix, sessionToken); err != nil {
		config.Logger.Error("Failed to revoke session", "error", err)
		return fmt.Errorf("failed to revoke session")
	}

	config.Logger.Info("Session revoked", "session_token", maskToken(sessionToken))
	return nil
}

// RevokeUserSessions revokes all sessions for a user
func RevokeUserSessions(ctx context.Context, config *SessionConfig, userID string) error {
	if config.Cache == nil {
		return fmt.Errorf("cache not configured")
	}

	if err := revokeUserSessions(ctx, config.Cache, config.SessionKeyPrefix, userID); err != nil {
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
		SessionKeyPrefix:             "session:",
		RoleKeyPrefix:                "user:roles:",
		PermissionKeyPrefix:          "user:perms:",
		AuthVersionKeyPrefix:         "user:authver:",
		CookieName:                   "session",
		CookiePath:                   "/",
		CookieSecure:                 true,
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
// It intelligently handles both API and UI routes
func defaultSessionErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	// Check if this is an API request or UI request
	if isAPIRequest(r) {
		// API request - return JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)

		response := map[string]interface{}{
			"error":   "Unauthorized",
			"message": "Authentication required",
		}
		json.NewEncoder(w).Encode(response)
	} else {
		// UI request - redirect to login
		http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
	}
}

// isAPIRequest determines if a request is for an API endpoint or a UI page
func isAPIRequest(r *http.Request) bool {
	// Check 1: Path-based detection
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/") {
		return true
	}

	// Check 2: Accept header prefers JSON
	accept := r.Header.Get("Accept")
	if accept != "" {
		// If explicitly accepts JSON and doesn't accept HTML, it's an API request
		acceptsJSON := strings.Contains(accept, "application/json")
		acceptsHTML := strings.Contains(accept, "text/html")

		if acceptsJSON && !acceptsHTML {
			return true
		}
	}

	// Check 3: Content-Type is JSON (for POST/PUT/PATCH requests)
	if r.Method != "GET" && r.Method != "HEAD" {
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") {
			return true
		}
	}

	// Check 4: X-Requested-With header (AJAX requests)
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		return true
	}

	// Default: treat as UI request
	return false
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
	if config.SessionKeyPrefix == "" {
		config.SessionKeyPrefix = "session:"
	}
	if config.RoleKeyPrefix == "" {
		config.RoleKeyPrefix = "user:roles:"
	}
	if config.PermissionKeyPrefix == "" {
		config.PermissionKeyPrefix = "user:perms:"
	}
	if config.AuthVersionKeyPrefix == "" {
		config.AuthVersionKeyPrefix = "user:authver:"
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

			// Get session data from cache
			sessionData, err := getSession(ctx, config.Cache, config.SessionKeyPrefix, sessionToken)
			if err != nil {
				config.Logger.Debug("Session not found in cache", "error", err)
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

			// User-Agent validation with risk scoring
			currentUserAgent := r.UserAgent()
			currentIP := getClientIP(r)

			// Calculate risk score based on UA and IP changes
			riskLevel := assessSessionRisk(sessionData, currentUserAgent, currentIP, config.Logger)

			// Handle based on risk level
			switch riskLevel {
			case RiskHigh:
				// High risk: Revoke session immediately
				config.Logger.Warn("High-risk session detected - revoking session",
					"user_id", sessionData.UserID,
					"reason", "major_ua_and_ip_change",
					"original_ua", normalizeUserAgent(sessionData.UserAgent),
					"current_ua", normalizeUserAgent(currentUserAgent),
					"original_ip", sessionData.IPAddress,
					"current_ip", currentIP,
				)

				if err := deleteSession(ctx, config.Cache, config.SessionKeyPrefix, sessionToken); err != nil {
					config.Logger.Error("Failed to delete high-risk session", "error", err, "user_id", sessionData.UserID)
				}

				config.ErrorHandler(w, r, fmt.Errorf("session security violation - please login again"))
				return

			case RiskMedium:
				// Medium risk: Log warning but allow (soft validation)
				config.Logger.Warn("Medium-risk session activity detected",
					"user_id", sessionData.UserID,
					"reason", "ua_or_ip_change",
					"original_ua", normalizeUserAgent(sessionData.UserAgent),
					"current_ua", normalizeUserAgent(currentUserAgent),
					"original_ip", sessionData.IPAddress,
					"current_ip", currentIP,
				)
				// Continue - allow access but logged for monitoring

			case RiskLow:
				// Low risk: Minor UA differences (version updates, etc.) - just log debug
				if sessionData.UserAgent != currentUserAgent {
					config.Logger.Debug("Minor User-Agent change detected",
						"user_id", sessionData.UserID,
						"original_ua", sessionData.UserAgent,
						"current_ua", currentUserAgent,
					)
				}
			}

			// Update last access time (sliding expiration) - with timeout control
			go func() {
				timeout := config.SessionActivityUpdateTimeout
				if timeout == 0 {
					timeout = 2 * time.Second
				}
				updateCtx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				if err := updateSessionActivity(updateCtx, config.Cache, config.SessionKeyPrefix, sessionToken); err != nil {
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
	if config.Cache != nil {
		roles, permissions, authVersion, found, err := getUserRolesPermissions(ctx, config.Cache, config.RoleKeyPrefix, config.PermissionKeyPrefix, config.AuthVersionKeyPrefix, userID)
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
	if config.Cache != nil {
		go func() {
			timeout := config.RoleCacheUpdateTimeout
			if timeout == 0 {
				timeout = 3 * time.Second
			}
			cacheCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if err := cacheUserRolesPermissions(cacheCtx, config.Cache, config.RoleKeyPrefix, config.PermissionKeyPrefix, config.AuthVersionKeyPrefix, userID, roles, permissions, authVersion, config.RoleCacheDuration); err != nil {
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

// ============================================================================
// Specialized Error Handlers
// ============================================================================

// NewAPIErrorHandler creates an error handler that always returns JSON (for API routes)
func NewAPIErrorHandler() func(w http.ResponseWriter, r *http.Request, err error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)

		response := map[string]interface{}{
			"error":   "Unauthorized",
			"message": "Authentication required",
		}
		json.NewEncoder(w).Encode(response)
	}
}

// NewUIErrorHandler creates an error handler that redirects to login (for UI routes)
func NewUIErrorHandler(loginPath string) func(w http.ResponseWriter, r *http.Request, err error) {
	if loginPath == "" {
		loginPath = "/login"
	}
	return func(w http.ResponseWriter, r *http.Request, err error) {
		// Redirect to login with the original URL as redirect parameter
		redirectURL := loginPath + "?redirect=" + r.URL.Path
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
	}
}

// NewSmartErrorHandler creates an error handler that detects API vs UI requests
// This is the default behavior
func NewSmartErrorHandler(loginPath string) func(w http.ResponseWriter, r *http.Request, err error) {
	if loginPath == "" {
		loginPath = "/login"
	}
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if isAPIRequest(r) {
			// API request - return JSON
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)

			response := map[string]interface{}{
				"error":   "Unauthorized",
				"message": "Authentication required",
			}
			json.NewEncoder(w).Encode(response)
		} else {
			// UI request - redirect to login
			redirectURL := loginPath + "?redirect=" + r.URL.Path
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		}
	}
}

// RiskLevel represents the risk level of a session
type RiskLevel int

const (
	RiskLow    RiskLevel = 0 // Same browser, OS, minor version changes
	RiskMedium RiskLevel = 1 // UA changed but same device type, or IP changed
	RiskHigh   RiskLevel = 2 // Major UA change (browser switch) + IP country change
)

// UserAgentInfo contains normalized UA information
type UserAgentInfo struct {
	BrowserFamily string // Chrome, Firefox, Safari, Edge, etc.
	BrowserMajor  string // Major version only (e.g., "120")
	OSFamily      string // Windows, macOS, Linux, Android, iOS
	DeviceType    string // desktop, mobile, tablet
}

// normalizeUserAgent extracts normalized UA information for comparison
func normalizeUserAgent(ua string) UserAgentInfo {
	ua = strings.ToLower(ua)
	info := UserAgentInfo{
		BrowserFamily: "unknown",
		BrowserMajor:  "0",
		OSFamily:      "unknown",
		DeviceType:    "unknown",
	}

	// Detect Browser Family
	switch {
	case strings.Contains(ua, "edg/") || strings.Contains(ua, "edge/"):
		info.BrowserFamily = "edge"
		info.BrowserMajor = extractMajorVersion(ua, "edg/", "edge/")
	case strings.Contains(ua, "chrome/") && !strings.Contains(ua, "edg"):
		info.BrowserFamily = "chrome"
		info.BrowserMajor = extractMajorVersion(ua, "chrome/")
	case strings.Contains(ua, "firefox/"):
		info.BrowserFamily = "firefox"
		info.BrowserMajor = extractMajorVersion(ua, "firefox/")
	case strings.Contains(ua, "safari/") && !strings.Contains(ua, "chrome"):
		info.BrowserFamily = "safari"
		info.BrowserMajor = extractMajorVersion(ua, "version/")
	case strings.Contains(ua, "opera/") || strings.Contains(ua, "opr/"):
		info.BrowserFamily = "opera"
		info.BrowserMajor = extractMajorVersion(ua, "opera/", "opr/")
	}

	// Detect OS Family
	switch {
	case strings.Contains(ua, "windows nt"):
		info.OSFamily = "windows"
	case strings.Contains(ua, "mac os x") || strings.Contains(ua, "macos"):
		info.OSFamily = "macos"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		info.OSFamily = "ios"
	case strings.Contains(ua, "android"):
		info.OSFamily = "android"
	case strings.Contains(ua, "linux"):
		info.OSFamily = "linux"
	}

	// Detect Device Type
	switch {
	case strings.Contains(ua, "mobile"):
		info.DeviceType = "mobile"
	case strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad"):
		info.DeviceType = "tablet"
	default:
		info.DeviceType = "desktop"
	}

	return info
}

// extractMajorVersion extracts major version from UA string
func extractMajorVersion(ua string, markers ...string) string {
	for _, marker := range markers {
		if idx := strings.Index(ua, marker); idx != -1 {
			versionStart := idx + len(marker)
			versionStr := ua[versionStart:]

			// Find the end of version (first non-digit char after digits)
			var major strings.Builder
			for _, ch := range versionStr {
				if ch >= '0' && ch <= '9' {
					major.WriteRune(ch)
				} else if major.Len() > 0 {
					break
				}
			}

			if major.Len() > 0 {
				return major.String()
			}
		}
	}
	return "0"
}

// uaSignificantlyChanged checks if User-Agent changed significantly
func uaSignificantlyChanged(original, current UserAgentInfo) bool {
	// Same browser family and OS = not significant
	if original.BrowserFamily == current.BrowserFamily &&
		original.OSFamily == current.OSFamily {
		return false
	}

	// Browser family changed (Chrome → Firefox, etc.)
	if original.BrowserFamily != current.BrowserFamily &&
		original.BrowserFamily != "unknown" &&
		current.BrowserFamily != "unknown" {
		return true
	}

	// OS changed (Windows → Android, etc.)
	if original.OSFamily != current.OSFamily &&
		original.OSFamily != "unknown" &&
		current.OSFamily != "unknown" {
		return true
	}

	// Device type changed (desktop → mobile)
	if original.DeviceType != current.DeviceType {
		return true
	}

	return false
}

// ipCountryChanged checks if IP is from a different country (simplified)
// This is a basic implementation - for production, use a GeoIP library
func ipCountryChanged(originalIP, currentIP string) bool {
	// If IPs are identical, no change
	if originalIP == currentIP {
		return false
	}

	// Basic heuristic: Check if IP prefix (first 2 octets) changed
	// This catches most country-level changes for IPv4
	// To use MaxMind GeoIP2 or similar
	originalPrefix := getIPPrefix(originalIP)
	currentPrefix := getIPPrefix(currentIP)

	// If prefixes are very different, likely different countries
	// This is a simplification - real GeoIP is recommended
	return originalPrefix != currentPrefix
}

// getIPPrefix extracts first two octets for basic geo comparison
func getIPPrefix(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	// For IPv6 or malformed, return full IP
	return ip
}

// assessSessionRisk evaluates session risk based on UA and IP changes
func assessSessionRisk(sessionData *SessionData, currentUA, currentIP string, logger *slog.Logger) RiskLevel {
	originalUAInfo := normalizeUserAgent(sessionData.UserAgent)
	currentUAInfo := normalizeUserAgent(currentUA)

	uaChanged := uaSignificantlyChanged(originalUAInfo, currentUAInfo)
	ipChanged := ipCountryChanged(sessionData.IPAddress, currentIP)

	// Risk Matrix:
	// UA Changed + IP Changed (different country) = HIGH RISK
	// UA Changed OR IP Changed = MEDIUM RISK
	// Minor UA differences (versions, etc.) = LOW RISK

	if uaChanged && ipChanged {
		// Both changed significantly - likely session hijacking
		return RiskHigh
	}

	if uaChanged || ipChanged {
		// One changed - suspicious but could be legitimate
		// (e.g., user upgraded browser, or traveling)
		return RiskMedium
	}

	// No significant changes - safe
	return RiskLow
}
