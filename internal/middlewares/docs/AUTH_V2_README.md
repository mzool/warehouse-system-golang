# Session-Based Authentication Middleware v2

Secure, performant, production-ready session-based authentication using Redis, signed tokens, and role-based access control.

## Features

✅ **Session-Based Authentication** - No JWT, pure session tokens  
✅ **Cryptographically Secure** - crypto/rand + HMAC signing  
✅ **Redis-Backed** - High-performance caching  
✅ **HttpOnly Secure Cookies** - XSS protection  
✅ **Role & Permission-Based Access** - Flexible authorization  
✅ **Session Revocation** - Instant logout/ban capability  
✅ **Dynamic Role Caching** - Minimal DB queries  
✅ **Sliding Expiration** - Auto-extend active sessions  
✅ **Concurrent Session Limits** - Prevent account sharing  
✅ **IP & User Agent Tracking** - Security monitoring  
✅ **No Internal Error Exposure** - Security best practice  
✅ **Minimal Dependencies** - Only stdlib + bcrypt + redis

## Architecture

```
┌─────────────┐
│   Client    │
│  (Browser)  │
└──────┬──────┘
       │ 1. POST /login (email, password)
       ▼
┌─────────────────────────────────────────┐
│           HTTP Handler                  │
│  ┌─────────────────────────────────┐   │
│  │ 1. Verify credentials (bcrypt)  │   │
│  │ 2. Generate secure token        │   │
│  │ 3. Sign token (HMAC-SHA256)     │   │
│  │ 4. Store session in Redis       │   │
│  │ 5. Set HttpOnly cookie          │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
       │
       │ 2. Subsequent requests with session cookie
       ▼
┌─────────────────────────────────────────┐
│     Session Auth Middleware             │
│  ┌─────────────────────────────────┐   │
│  │ 1. Extract cookie               │   │
│  │ 2. Validate signature           │   │
│  │ 3. Check Redis cache            │   │
│  │ 4. Verify not revoked           │   │
│  │ 5. Check expiration             │   │
│  │ 6. Fetch roles/permissions      │   │
│  │ 7. Validate authorization       │   │
│  │ 8. Add to context               │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
       │
       ▼
┌─────────────┐
│   Redis     │
│  session:data:abc123...def │
│  session:user:123:sessions │
│  session:user:123:roles_perms │
└─────────────┘
```

## Quick Start

### 1. Setup Dependencies

```bash
go get golang.org/x/crypto/bcrypt
go get github.com/redis/go-redis/v9
```

### 2. Initialize Redis

```go
import "github.com/redis/go-redis/v9"

redisClient := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "", // no password
    DB:       0,
})
```

### 3. Create Session Store

```go
sessionStore := middlewares.NewRedisSessionStore(redisClient)
```

### 4. Implement Role Provider

```go
type MyRoleProvider struct {
    db *pgxpool.Pool
}

func (p *MyRoleProvider) GetUserRolesAndPermissions(ctx context.Context, userID string) ([]string, []string, error) {
    // Query your database
    rows, err := p.db.Query(ctx, `
        SELECT role_name FROM user_roles WHERE user_id = $1
    `, userID)
    // ... scan roles ...
    
    return roles, permissions, nil
}
```

### 5. Configure Session Auth

```go
import "log/slog"

sessionConfig := &middlewares.SessionConfig{
    SessionStore:           sessionStore,
    RolePermissionProvider: &MyRoleProvider{db: dbPool},
    SecretKey:              []byte("your-32-byte-secret-key-here!!!"),
    CookieName:             "session",
    CookieSecure:           true, // HTTPS only in production
    CookieHTTPOnly:         true,
    CookieSameSite:         http.SameSiteLaxMode,
    SessionDuration:        24 * time.Hour,
    RoleCacheDuration:      15 * time.Minute,
    Logger:                 slog.Default(),
}
```

### 6. Create Login Handler

```go
func loginHandler(w http.ResponseWriter, r *http.Request) {
    var creds struct {
        Email    string `json:"email"`
        Password string `json:"password"`
    }
    json.NewDecoder(r.Body).Decode(&creds)

    // Authenticate and create session
    sessionToken, err := middlewares.AuthenticateUser(
        r.Context(),
        sessionConfig,
        creds.Email,
        creds.Password,
        func(ctx context.Context, email string) (userID, username, hashedPassword string, err error) {
            // Your DB query
            row := db.QueryRow(ctx, "SELECT id, username, password FROM users WHERE email = $1", email)
            err = row.Scan(&userID, &username, &hashedPassword)
            return
        },
        r,
    )

    if err != nil {
        http.Error(w, "Invalid credentials", http.StatusUnauthorized)
        return
    }

    // Set session cookie
    middlewares.SetSessionCookie(w, sessionConfig, sessionToken)

    json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
```

### 7. Protect Routes

```go
// Apply to specific handler
protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    session := middlewares.MustGetSession(r)
    
    json.NewEncoder(w).Encode(map[string]interface{}{
        "user_id": session.UserID,
        "email":   session.Email,
        "roles":   session.Roles,
    })
})

http.Handle("/api/profile", middlewares.SessionAuth(sessionConfig)(protectedHandler))
```

## Usage Examples

### Basic Authentication

```go
// Public endpoint (no auth)
http.HandleFunc("/api/public", publicHandler)

// Protected endpoint (requires login)
http.Handle("/api/profile", 
    middlewares.SessionAuth(sessionConfig)(profileHandler))
```

### Role-Based Access

```go
// Admin only
adminConfig := *sessionConfig
adminConfig.RequiredRoles = []string{"admin"}

http.Handle("/api/admin/users", 
    middlewares.SessionAuth(&adminConfig)(adminHandler))

// Or use convenience function
http.Handle("/api/admin/dashboard",
    middlewares.RequireRole(sessionConfig, "admin")(dashboardHandler))
```

### Permission-Based Access

```go
// Requires specific permission
http.Handle("/api/users/{id}",
    middlewares.RequirePermission(sessionConfig, "users:delete")(deleteHandler))
```

### Multiple Requirements

```go
// Requires BOTH role AND permission
config := *sessionConfig
config.RequiredRoles = []string{"admin", "manager"}
config.RequiredPermissions = []string{"sensitive:access"}

http.Handle("/api/sensitive",
    middlewares.SessionAuth(&config)(sensitiveHandler))
```

### Logout

```go
func logoutHandler(w http.ResponseWriter, r *http.Request) {
    sessionID, _ := middlewares.GetSessionIDFromContext(r)
    
    // Revoke session
    middlewares.RevokeSession(r.Context(), sessionConfig, sessionID)
    
    // Delete cookie
    middlewares.DeleteSessionCookie(w, sessionConfig)
    
    json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}
```

### Revoke All User Sessions

```go
// When user changes password
func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
    session := middlewares.MustGetSession(r)
    
    // Update password in DB...
    
    // Force re-login on all devices
    middlewares.RevokeUserSessions(r.Context(), sessionConfig, session.UserID)
    
    w.Write([]byte("Password changed. Please login again."))
}
```

### Skip Authentication

```go
sessionConfig.SkipPaths = []string{
    "/api/health",
    "/api/public",
    "/api/docs",
}

// Apply globally
http.ListenAndServe(":8080", 
    middlewares.SessionAuth(sessionConfig)(mux))
```

## Security Features

### 1. Secure Token Generation

```go
// Uses crypto/rand for 256-bit random tokens
token := generateSessionToken() // 32 bytes of randomness
```

### 2. HMAC Signing

```go
// Tokens are signed to prevent tampering
// Format: token.signature
signature := HMAC-SHA256(token, secretKey)
signedToken := token + "." + signature
```

### 3. HttpOnly Cookies

```go
// Cookies not accessible via JavaScript (XSS protection)
cookie := &http.Cookie{
    HttpOnly: true,
    Secure:   true, // HTTPS only
    SameSite: http.SameSiteLaxMode, // CSRF protection
}
```

### 4. Session Validation

- Signature verification (prevents tampering)
- Expiration check
- Revocation status
- Optional IP tracking
- User agent tracking

### 5. Password Security

```go
// Bcrypt with cost 12
hashedPassword, _ := middlewares.HashPassword("password123")
err := middlewares.VerifyPassword(hashedPassword, "password123")
```

## Performance Optimizations

### 1. Role/Permission Caching

```go
// Roles cached in Redis for 15 minutes (configurable)
// Reduces database queries dramatically
sessionConfig.RoleCacheDuration = 15 * time.Minute
```

### 2. Non-Blocking Updates

```go
// Session activity updates happen in background
go func() {
    sessionStore.UpdateSessionActivity(ctx, sessionID)
}()
```

### 3. Minimal Session Data

```go
// Only essential data stored in Redis
type SessionData struct {
    UserID    string    // Minimal identifier
    Email     string
    Username  string
    IsActive  bool
    ExpiresAt time.Time
    // ... minimal metadata
}
```

### 4. Efficient Redis Keys

```
session:data:abc123def456...           # Session data
session:user:123:sessions              # User's session set
session:user:123:roles_perms           # Cached roles/permissions
```

## Advanced Features

### Limit Concurrent Sessions

```go
redisStore := sessionStore.(*middlewares.RedisSessionStore)

// After successful login
err := redisStore.LimitConcurrentSessions(ctx, userID, 3)
// Keeps only 3 most recent sessions, revokes older ones
```

### Get Active Sessions

```go
sessionIDs, err := redisStore.GetAllSessionsForUser(ctx, userID)
count, err := redisStore.GetActiveSessionCount(ctx, userID)
```

### Clear Role Cache

```go
// After updating user roles in database
err := redisStore.ClearUserRolesPermissionsCache(ctx, userID)
// Forces fresh fetch on next request
```

### Custom Error Handling

```go
sessionConfig.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
    // Custom error response
    w.WriteHeader(http.StatusUnauthorized)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "error": "UNAUTHORIZED",
        "message": "Please login",
        // Don't expose internal errors
    })
}
```

## Session Lifecycle

```
1. Login
   ├─ Verify credentials (bcrypt)
   ├─ Generate secure token (crypto/rand)
   ├─ Sign token (HMAC-SHA256)
   ├─ Store in Redis (TTL: 24h)
   └─ Set HttpOnly cookie

2. Request
   ├─ Extract cookie
   ├─ Validate signature
   ├─ Check Redis
   ├─ Verify not revoked
   ├─ Check expiration
   ├─ Get/cache roles
   ├─ Validate authorization
   └─ Add to context

3. Logout
   ├─ Delete from Redis
   ├─ Remove from user's session set
   └─ Delete cookie

4. Session Expiry
   └─ Automatic Redis TTL cleanup
```

## Testing

```go
func TestSessionAuth(t *testing.T) {
    // Setup test config
    config := middlewares.DefaultSessionConfig()
    config.SecretKey = []byte("test-secret-key-32-bytes-long!")
    
    // Create test session
    sessionToken, _ := middlewares.CreateSession(
        context.Background(),
        config,
        "123",
        "test@example.com",
        "testuser",
        req,
    )
    
    // Test request with session
    req := httptest.NewRequest("GET", "/api/profile", nil)
    req.AddCookie(&http.Cookie{
        Name:  "session",
        Value: sessionToken,
    })
    
    // Execute
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)
    
    // Assert
    assert.Equal(t, http.StatusOK, w.Code)
}
```

## Migration from JWT

See comparison:

| Feature | JWT | Session (v2) |
|---------|-----|--------------|
| Storage | Client-side | Server-side (Redis) |
| Revocation | Difficult | Instant |
| Size | Large (~1KB) | Small (64 bytes) |
| Security | Exposed if stolen | Revocable |
| Stateless | Yes | No (by design) |
| Performance | Token validation | Redis lookup |

## Production Checklist

- [ ] Set `CookieSecure: true` (HTTPS only)
- [ ] Use strong secret key (min 32 bytes)
- [ ] Configure Redis persistence
- [ ] Set up Redis replication/clustering
- [ ] Monitor session count per user
- [ ] Log suspicious activity (IP changes)
- [ ] Implement rate limiting on login
- [ ] Set appropriate session duration
- [ ] Configure role cache TTL
- [ ] Set up alerts for Redis failures
- [ ] Test session revocation flow
- [ ] Implement "remember me" option
- [ ] Add CAPTCHA for failed logins

## Troubleshooting

### Sessions not persisting

- Check Redis connection
- Verify cookie settings (`Secure`, `SameSite`)
- Check `SessionDuration` TTL

### Roles not updating

- Clear role cache: `ClearUserRolesPermissionsCache()`
- Check `RoleCacheDuration` setting

### "Session expired" immediately

- Check system time synchronization
- Verify `SessionDuration` setting
- Check Redis TTL settings

### High memory usage

- Reduce `SessionDuration`
- Implement `LimitConcurrentSessions`
- Clean up expired sessions regularly

## Best Practices

1. **Secret Key**: Use at least 32 random bytes, store in env variable
2. **HTTPS Only**: Always use `CookieSecure: true` in production
3. **Role Caching**: Balance between DB load and data freshness
4. **Session Limits**: Prevent account sharing with concurrent limits
5. **Error Handling**: Never expose internal errors to clients
6. **Monitoring**: Track session creation, revocation, failures
7. **Cleanup**: Let Redis TTL handle expiry, but monitor memory
8. **Testing**: Always test session lifecycle in integration tests

## License

MIT License
