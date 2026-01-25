# Migration Guide: JWT Auth → Session Auth v2

## Overview

Migrating from JWT-based to session-based authentication for better security, instant revocation, and simpler implementation.

## Key Differences

| Aspect | Old (JWT) | New (Session) |
|--------|-----------|---------------|
| Token Storage | Client-side | Server-side (Redis) |
| Token Format | Base64 JWT | Signed random token |
| Revocation | Blacklist (complex) | Instant delete |
| Size | ~1KB | 64 bytes |
| Dependencies | `golang-jwt/jwt` | stdlib + bcrypt |
| Validation | Cryptographic | Redis lookup + signature |
| State | Stateless | Stateful |
| Performance | CPU (validation) | Network (Redis) |

## Step-by-Step Migration

### 1. Install Dependencies

```bash
# Remove JWT dependency (if unused elsewhere)
go mod tidy

# Add only if not present
go get golang.org/x/crypto/bcrypt
go get github.com/redis/go-redis/v9
```

### 2. Setup Redis

**Old (JWT):**
```go
// JWT stored in client, no server state needed
```

**New (Session):**
```go
import "github.com/redis/go-redis/v9"

redisClient := redis.NewClient(&redis.Options{
    Addr:     os.Getenv("REDIS_ADDR"),
    Password: os.Getenv("REDIS_PASSWORD"),
    DB:       0,
})

sessionStore := middlewares.NewRedisSessionStore(redisClient)
```

### 3. Replace Auth Configuration

**Old (JWT):**
```go
authConfig := &middlewares.AuthConfig{
    TokenValidator: middlewares.JWTValidator(jwtSecret),
    ErrorHandler:   defaultAuthErrorHandler,
    SkipPaths:      []string{"/api/login", "/api/register"},
}
```

**New (Session):**
```go
sessionConfig := &middlewares.SessionConfig{
    SessionStore:           sessionStore,
    RolePermissionProvider: roleProvider,
    SecretKey:              []byte(os.Getenv("SESSION_SECRET")),
    CookieName:             "session",
    CookieSecure:           true,
    CookieHTTPOnly:         true,
    SessionDuration:        24 * time.Hour,
    RoleCacheDuration:      15 * time.Minute,
    Logger:                 logger,
    SkipPaths:              []string{"/api/login", "/api/register"},
}
```

### 4. Update Login Handler

**Old (JWT):**
```go
func loginHandler(w http.ResponseWriter, r *http.Request) {
    // Verify credentials...
    
    // Create JWT token
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "sub":   userID,
        "email": email,
        "exp":   time.Now().Add(24 * time.Hour).Unix(),
    })
    
    tokenString, _ := token.SignedString(jwtSecret)
    
    // Return in response body
    json.NewEncoder(w).Encode(map[string]string{
        "token": tokenString,
    })
}
```

**New (Session):**
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
        getUserByEmail, // Your DB function
        r,
    )
    if err != nil {
        http.Error(w, "Invalid credentials", http.StatusUnauthorized)
        return
    }
    
    // Set HttpOnly cookie (not in response body!)
    middlewares.SetSessionCookie(w, sessionConfig, sessionToken)
    
    json.NewEncoder(w).Encode(map[string]string{
        "status": "success",
    })
}
```

### 5. Update Client-Side Code

**Old (JWT - Frontend):**
```javascript
// Client stored token in localStorage
localStorage.setItem('token', response.token);

// Client sent token in header
fetch('/api/profile', {
    headers: {
        'Authorization': `Bearer ${localStorage.getItem('token')}`
    }
});
```

**New (Session - Frontend):**
```javascript
// No token management needed!
// Cookie automatically sent by browser

// Login
fetch('/api/login', {
    method: 'POST',
    credentials: 'include', // Important: send cookies
    body: JSON.stringify({email, password})
});

// Subsequent requests
fetch('/api/profile', {
    credentials: 'include' // Browser handles cookie
});
```

### 6. Update Middleware Application

**Old (JWT):**
```go
router.Register(&Route{
    Method: "GET",
    Path: "/profile",
    Middlewares: []Middleware{
        middlewares.Auth(authConfig),
    },
    HandlerFunc: profileHandler,
})
```

**New (Session):**
```go
router.Register(&Route{
    Method: "GET",
    Path: "/profile",
    Middlewares: []Middleware{
        middlewares.SessionAuth(sessionConfig),
    },
    HandlerFunc: profileHandler,
})
```

### 7. Update Context Access

**Old (JWT):**
```go
func profileHandler(w http.ResponseWriter, r *http.Request) {
    userAuth, ok := middlewares.GetUserFromContext(r)
    if !ok {
        http.Error(w, "Unauthorized", 401)
        return
    }
    
    userID := userAuth.ID
    email := userAuth.Email
}
```

**New (Session):**
```go
func profileHandler(w http.ResponseWriter, r *http.Request) {
    session := middlewares.MustGetSession(r) // Or GetSessionFromContext
    
    userID := session.UserID
    email := session.Email
    roles := session.Roles
}
```

### 8. Implement Logout

**Old (JWT):**
```go
// Complex: Need to blacklist token until expiry
func logoutHandler(w http.ResponseWriter, r *http.Request) {
    userAuth, _ := middlewares.GetUserFromContext(r)
    
    // Add to blacklist in Redis
    cache.BlacklistToken(r.Context(), userAuth.ID)
    
    json.NewEncoder(w).Encode(map[string]string{
        "status": "logged out",
    })
}
```

**New (Session):**
```go
// Simple: Just delete the session
func logoutHandler(w http.ResponseWriter, r *http.Request) {
    sessionID, _ := middlewares.GetSessionIDFromContext(r)
    
    // Revoke session
    middlewares.RevokeSession(r.Context(), sessionConfig, sessionID)
    
    // Delete cookie
    middlewares.DeleteSessionCookie(w, sessionConfig)
    
    json.NewEncoder(w).Encode(map[string]string{
        "status": "logged out",
    })
}
```

### 9. Implement Role Provider

**New Requirement:**
```go
type DatabaseRoleProvider struct {
    db *pgxpool.Pool
}

func (p *DatabaseRoleProvider) GetUserRolesAndPermissions(ctx context.Context, userID string) ([]string, []string, error) {
    var roles []string
    var permissions []string
    
    // Query roles
    rows, err := p.db.Query(ctx, `
        SELECT r.name 
        FROM user_roles ur 
        JOIN roles r ON ur.role_id = r.id 
        WHERE ur.user_id = $1
    `, userID)
    if err != nil {
        return nil, nil, err
    }
    defer rows.Close()
    
    for rows.Next() {
        var role string
        rows.Scan(&role)
        roles = append(roles, role)
    }
    
    // Query permissions
    rows, err = p.db.Query(ctx, `
        SELECT p.name 
        FROM user_permissions up 
        JOIN permissions p ON up.permission_id = p.id 
        WHERE up.user_id = $1
    `, userID)
    if err != nil {
        return nil, nil, err
    }
    defer rows.Close()
    
    for rows.Next() {
        var perm string
        rows.Scan(&perm)
        permissions = append(permissions, perm)
    }
    
    return roles, permissions, nil
}
```

### 10. Update Role Checks

**Old (JWT):**
```go
userAuth, _ := middlewares.GetUserFromContext(r)
if !userAuth.HasRole("admin") {
    http.Error(w, "Forbidden", 403)
    return
}
```

**New (Session - Same API):**
```go
session := middlewares.MustGetSession(r)
if !session.HasRole("admin") {
    http.Error(w, "Forbidden", 403)
    return
}
```

### 11. Environment Variables

**Old:**
```env
JWT_SECRET=your-jwt-secret-key
```

**New:**
```env
SESSION_SECRET=your-session-secret-at-least-32-bytes-long
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=your-redis-password
```

### 12. Update Registration

**Old (JWT - passwords might not be hashed):**
```go
func registerHandler(w http.ResponseWriter, r *http.Request) {
    // Store password as-is (BAD!)
    db.Exec("INSERT INTO users (email, password) VALUES ($1, $2)", 
        email, password)
}
```

**New (Session - proper password hashing):**
```go
func registerHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email    string `json:"email"`
        Password string `json:"password"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    // Hash password
    hashedPassword, err := middlewares.HashPassword(req.Password)
    if err != nil {
        http.Error(w, "Registration failed", 500)
        return
    }
    
    // Store hashed password
    db.Exec("INSERT INTO users (email, password) VALUES ($1, $2)",
        req.Email, hashedPassword)
    
    w.WriteHeader(http.StatusCreated)
}
```

## Database Schema Changes

### Add User ID as String

**If using integer IDs:**
```sql
-- Option 1: Use CAST in queries
SELECT CAST(id AS TEXT) as id, email, username, password 
FROM users WHERE email = $1;

-- Option 2: Add computed column (PostgreSQL)
ALTER TABLE users ADD COLUMN id_str TEXT GENERATED ALWAYS AS (id::text) STORED;

-- Option 3: Handle in code
userID := middlewares.UserIDToString(userIDFromDB)
```

### Sessions Table (Optional - Redis is primary)

```sql
-- Optional: Backup session data in database
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    ip_address TEXT,
    user_agent TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
```

## Testing Updates

**Old (JWT):**
```go
func TestAuthMiddleware(t *testing.T) {
    // Create JWT token
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    tokenString, _ := token.SignedString(jwtSecret)
    
    // Add to header
    req.Header.Set("Authorization", "Bearer " + tokenString)
}
```

**New (Session):**
```go
func TestAuthMiddleware(t *testing.T) {
    // Create session
    sessionToken, _ := middlewares.CreateSession(
        ctx, config, "123", "test@example.com", "testuser", req,
    )
    
    // Add as cookie
    req.AddCookie(&http.Cookie{
        Name:  "session",
        Value: sessionToken,
    })
}
```

## Deployment Considerations

### Redis Setup

```bash
# Production Redis with persistence
docker run -d \
  --name redis \
  -p 6379:6379 \
  -v redis-data:/data \
  redis:latest \
  redis-server --appendonly yes
```

### Environment Configuration

```yaml
# production.env
SESSION_SECRET=generate-with-openssl-rand-base64-32
REDIS_ADDR=redis:6379
REDIS_PASSWORD=strong-password
COOKIE_SECURE=true
SESSION_DURATION=24h
```

### Monitoring

```go
// Track session metrics
metrics.SessionsCreated.Inc()
metrics.SessionsActive.Set(float64(activeCount))
metrics.SessionsRevoked.Inc()
```

## Backward Compatibility (Transition Period)

Support both JWT and Session during migration:

```go
// Combined middleware
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Try session auth first
        if cookie, err := r.Cookie("session"); err == nil {
            // Validate session
            if session, err := validateSession(cookie.Value); err == nil {
                // Use session
                ctx := context.WithValue(r.Context(), sessionUserKey, session)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }
        }
        
        // Fall back to JWT
        if auth := r.Header.Get("Authorization"); auth != "" {
            // Validate JWT
            if userAuth, err := validateJWT(auth); err == nil {
                // Use JWT (convert to session format)
                ctx := context.WithValue(r.Context(), sessionUserKey, userAuth)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }
        }
        
        // Neither worked
        http.Error(w, "Unauthorized", 401)
    })
}
```

## Rollback Plan

If issues occur:

1. **Keep old JWT code** in separate file
2. **Feature flag** to switch between auth methods
3. **Gradual rollout** per endpoint
4. **Monitor** session Redis performance
5. **Quick revert** via feature flag

```go
if config.UseSessionAuth {
    return middlewares.SessionAuth(sessionConfig)
} else {
    return middlewares.Auth(jwtConfig)
}
```

## Benefits After Migration

✅ **Instant Revocation** - No blacklist complexity  
✅ **Better Security** - HttpOnly cookies, signed tokens  
✅ **Simpler Code** - No JWT library, no token refresh logic  
✅ **Better Control** - Limit sessions, track activity  
✅ **Faster Auth** - Redis lookup vs cryptographic validation  
✅ **Smaller Tokens** - 64 bytes vs 1KB  
✅ **Better Monitoring** - Track active sessions easily  
✅ **Role Caching** - Reduce DB load dramatically  

## Common Issues & Solutions

### Issue: "Session not found"

**Cause:** Redis connection lost or session expired  
**Solution:** Check Redis health, adjust `SessionDuration`

### Issue: Cookies not sent in CORS requests

**Cause:** Missing `credentials: 'include'` in fetch  
**Solution:** Update frontend:
```javascript
fetch(url, {
    credentials: 'include',
    // ...
})
```

### Issue: High Redis memory usage

**Cause:** Too many sessions or long duration  
**Solution:** 
- Reduce `SessionDuration`
- Implement `LimitConcurrentSessions`
- Redis eviction policy: `maxmemory-policy allkeys-lru`

### Issue: Roles not updating after DB change

**Cause:** Cached in Redis  
**Solution:** 
```go
sessionStore.ClearUserRolesPermissionsCache(ctx, userID)
```

## Checklist

Migration complete when:

- [ ] Redis configured and tested
- [ ] Session store implemented
- [ ] Role provider implemented  
- [ ] Login handler updated
- [ ] Logout handler implemented
- [ ] All middleware updated
- [ ] Frontend updated (credentials: include)
- [ ] Context access updated
- [ ] Tests updated
- [ ] Environment variables set
- [ ] Password hashing added
- [ ] Monitoring added
- [ ] Documentation updated
- [ ] Team trained on new system
- [ ] Rollback plan ready

## Next Steps

1. Test in development environment
2. Load test with expected traffic
3. Deploy to staging
4. Monitor Redis performance
5. Gradual production rollout
6. Remove old JWT code after 30 days

## Support

Questions? Check:
- [AUTH_V2_README.md](./AUTH_V2_README.md) - Complete documentation
- [auth_examples_test.go](./auth_examples_test.go) - Usage examples
- [session_redis.go](./session_redis.go) - Redis implementation
