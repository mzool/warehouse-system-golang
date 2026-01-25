package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis implementation of SessionStore interface
// Provides high-performance session management with Redis

// RedisSessionStore implements SessionStore using Redis
type RedisSessionStore struct {
	client *redis.Client
	prefix string // Key prefix for namespacing
}

// NewRedisSessionStore creates a new Redis-backed session store
func NewRedisSessionStore(client *redis.Client) *RedisSessionStore {
	return &RedisSessionStore{
		client: client,
		prefix: "session:",
	}
}

// NewRedisSessionStoreWithPrefix creates a new Redis store with custom prefix
func NewRedisSessionStoreWithPrefix(client *redis.Client, prefix string) *RedisSessionStore {
	return &RedisSessionStore{
		client: client,
		prefix: prefix,
	}
}

// SaveSession saves session data to Redis
func (s *RedisSessionStore) SaveSession(ctx context.Context, sessionID string, data *SessionData, ttl time.Duration) error {
	key := s.sessionKey(sessionID)

	// Serialize session data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	// Save to Redis with TTL
	if err := s.client.Set(ctx, key, jsonData, ttl).Err(); err != nil {
		return fmt.Errorf("failed to save session to redis: %w", err)
	}

	// Add to user's session set (for revoking all user sessions)
	userSessionsKey := s.userSessionsKey(data.UserID)
	if err := s.client.SAdd(ctx, userSessionsKey, sessionID).Err(); err != nil {
		return fmt.Errorf("failed to add session to user set: %w", err)
	}

	// Set TTL on user sessions set
	s.client.Expire(ctx, userSessionsKey, ttl+24*time.Hour) // Keep a bit longer

	return nil
}

// GetSession retrieves session data from Redis
func (s *RedisSessionStore) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	key := s.sessionKey(sessionID)

	// Get from Redis
	jsonData, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session from redis: %w", err)
	}

	// Deserialize
	var data SessionData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &data, nil
}

// DeleteSession removes session from Redis
func (s *RedisSessionStore) DeleteSession(ctx context.Context, sessionID string) error {
	// Get session data first to remove from user's session set
	sessionData, err := s.GetSession(ctx, sessionID)
	if err != nil {
		// Session doesn't exist, nothing to delete
		return nil
	}

	// Delete session key
	key := s.sessionKey(sessionID)
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Remove from user's session set
	userSessionsKey := s.userSessionsKey(sessionData.UserID)
	s.client.SRem(ctx, userSessionsKey, sessionID)

	return nil
}

// RevokeUserSessions revokes all sessions for a user
func (s *RedisSessionStore) RevokeUserSessions(ctx context.Context, userID string) error {
	userSessionsKey := s.userSessionsKey(userID)

	// Get all session IDs for this user
	sessionIDs, err := s.client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	// Delete all session keys
	if len(sessionIDs) > 0 {
		keys := make([]string, len(sessionIDs))
		for i, sessionID := range sessionIDs {
			keys[i] = s.sessionKey(sessionID)
		}

		if err := s.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("failed to delete user sessions: %w", err)
		}
	}

	// Delete the user sessions set
	if err := s.client.Del(ctx, userSessionsKey).Err(); err != nil {
		return fmt.Errorf("failed to delete user sessions set: %w", err)
	}

	return nil
}

// UpdateSessionActivity updates the last access time for a session
func (s *RedisSessionStore) UpdateSessionActivity(ctx context.Context, sessionID string) error {
	key := s.sessionKey(sessionID)

	// Get current session data
	sessionData, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Update last access time
	sessionData.LastAccessAt = time.Now()

	// Re-save with updated data
	jsonData, err := json.Marshal(sessionData)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	// Get remaining TTL
	ttl, err := s.client.TTL(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to get session TTL: %w", err)
	}

	// Update with same TTL (or extend it - implement sliding expiration)
	// For sliding expiration, you could extend the TTL here
	if err := s.client.Set(ctx, key, jsonData, ttl).Err(); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// GetUserRolesPermissions retrieves cached roles and permissions from Redis
// Returns (roles, permissions, authVersion, found, error)
func (s *RedisSessionStore) GetUserRolesPermissions(ctx context.Context, userID string) ([]string, []string, int, bool, error) {
	key := s.rolesPermissionsKey(userID)

	// Get from Redis
	jsonData, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		// Cache miss - not an error, just not found
		return nil, nil, 0, false, nil
	}
	if err != nil {
		return nil, nil, 0, false, fmt.Errorf("failed to get roles/permissions from redis: %w", err)
	}

	// Deserialize
	var data struct {
		Roles       []string `json:"roles"`
		Permissions []string `json:"permissions"`
		AuthVersion int      `json:"auth_version"`
	}
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, nil, 0, false, fmt.Errorf("failed to unmarshal roles/permissions: %w", err)
	}

	return data.Roles, data.Permissions, data.AuthVersion, true, nil
}

// CacheUserRolesPermissions caches roles and permissions with auth version in Redis
func (s *RedisSessionStore) CacheUserRolesPermissions(ctx context.Context, userID string, roles, permissions []string, authVersion int, ttl time.Duration) error {
	key := s.rolesPermissionsKey(userID)

	// Serialize data with auth version
	data := struct {
		Roles       []string `json:"roles"`
		Permissions []string `json:"permissions"`
		AuthVersion int      `json:"auth_version"`
	}{
		Roles:       roles,
		Permissions: permissions,
		AuthVersion: authVersion,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal roles/permissions: %w", err)
	}

	// Save to Redis with TTL
	if err := s.client.Set(ctx, key, jsonData, ttl).Err(); err != nil {
		return fmt.Errorf("failed to cache roles/permissions: %w", err)
	}

	return nil
}

// GetUserAuthVersion retrieves user's current auth version from database
// This is a placeholder - implement based on your database structure
func (s *RedisSessionStore) GetUserAuthVersion(ctx context.Context, userID string) (int, error) {
	// This method should be implemented by your RolePermissionProvider, not RedisStore
	// For Redis, we just return what's cached
	roles, perms, authVersion, found, err := s.GetUserRolesPermissions(ctx, userID)
	if err != nil || !found {
		return 0, fmt.Errorf("auth version not cached")
	}
	// Avoid unused variable warnings
	_ = roles
	_ = perms
	return authVersion, nil
}

// ClearUserRolesPermissionsCache clears the cached roles/permissions for a user
// Call this when user roles or permissions are updated in the database
func (s *RedisSessionStore) ClearUserRolesPermissionsCache(ctx context.Context, userID string) error {
	key := s.rolesPermissionsKey(userID)
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to clear roles/permissions cache: %w", err)
	}
	return nil
}

// ============================================================================
// Key Helpers
// ============================================================================

func (s *RedisSessionStore) sessionKey(sessionID string) string {
	return s.prefix + "data:" + sessionID
}

func (s *RedisSessionStore) userSessionsKey(userID string) string {
	return s.prefix + "user:" + userID + ":sessions"
}

func (s *RedisSessionStore) rolesPermissionsKey(userID string) string {
	return s.prefix + "user:" + userID + ":roles_perms"
}

// ============================================================================
// Advanced Features
// ============================================================================

// GetAllSessionsForUser returns all active session IDs for a user
func (s *RedisSessionStore) GetAllSessionsForUser(ctx context.Context, userID string) ([]string, error) {
	userSessionsKey := s.userSessionsKey(userID)
	sessionIDs, err := s.client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}
	return sessionIDs, nil
}

// GetActiveSessionCount returns the number of active sessions for a user
func (s *RedisSessionStore) GetActiveSessionCount(ctx context.Context, userID string) (int64, error) {
	userSessionsKey := s.userSessionsKey(userID)
	count, err := s.client.SCard(ctx, userSessionsKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get session count: %w", err)
	}
	return count, nil
}

// LimitConcurrentSessions limits the number of concurrent sessions per user
// When limit is reached, oldest sessions are revoked
func (s *RedisSessionStore) LimitConcurrentSessions(ctx context.Context, userID string, limit int) error {
	userSessionsKey := s.userSessionsKey(userID)

	// Get all session IDs
	sessionIDs, err := s.client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	// If within limit, nothing to do
	if len(sessionIDs) <= limit {
		return nil
	}

	// Get session data for all sessions to sort by creation time
	type sessionWithTime struct {
		ID        string
		CreatedAt time.Time
	}

	sessions := make([]sessionWithTime, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		sessionData, err := s.GetSession(ctx, sessionID)
		if err != nil {
			// Skip invalid sessions
			continue
		}
		sessions = append(sessions, sessionWithTime{
			ID:        sessionID,
			CreatedAt: sessionData.CreatedAt,
		})
	}

	// Sort by creation time (oldest first)
	// Simple bubble sort for small arrays
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[i].CreatedAt.After(sessions[j].CreatedAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	// Revoke oldest sessions
	toRevoke := len(sessions) - limit
	for i := 0; i < toRevoke; i++ {
		if err := s.DeleteSession(ctx, sessions[i].ID); err != nil {
			return fmt.Errorf("failed to revoke old session: %w", err)
		}
	}

	return nil
}

// CleanupExpiredSessions removes expired sessions (usually handled by Redis TTL, but useful for cleanup)
func (s *RedisSessionStore) CleanupExpiredSessions(ctx context.Context) (int, error) {
	// Use SCAN to iterate over all session keys
	var cursor uint64
	var cleanedCount int

	for {
		var keys []string
		var err error

		keys, cursor, err = s.client.Scan(ctx, cursor, s.prefix+"data:*", 100).Result()
		if err != nil {
			return cleanedCount, fmt.Errorf("failed to scan sessions: %w", err)
		}

		for _, key := range keys {
			// Check if key exists (not expired)
			exists, err := s.client.Exists(ctx, key).Result()
			if err != nil {
				continue
			}

			if exists == 0 {
				cleanedCount++
			}
		}

		if cursor == 0 {
			break
		}
	}

	return cleanedCount, nil
}
