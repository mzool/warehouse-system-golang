# Security Package

Production-ready security features including CSRF protection, input sanitization, RBAC, password hashing, and security utilities.

## Features

- **CSRF Protection**: Token-based CSRF protection with cookie/header validation
- **Input Sanitization**: XSS prevention, HTML sanitization, SQL injection detection
- **RBAC**: Role-based access control with permission inheritance
- **Password Hashing**: Argon2id password hashing with strength validation
- **Security Utilities**: Token generation, secure comparison, validators
- **Structured Logging**: slog integration for security events

## Quick Start

### 1. CSRF Protection

```go
import "your-project/internal/security"

// Create CSRF protection
csrf := security.NewCSRFProtection(security.DefaultCSRFConfig())

// Use as middleware
mux := http.NewServeMux()
handler := csrf.Middleware(mux)

http.ListenAndServe(":8080", handler)
```

**HTML Form with CSRF Token:**
```html
<form method="POST" action="/submit">
    {{ CSRFTokenHTML .csrf_token }}
    <input type="text" name="data">
    <button type="submit">Submit</button>
</form>
```

**JavaScript/AJAX:**
```javascript
// Get token from meta tag or cookie
const token = document.querySelector('meta[name="csrf-token"]').content;

fetch('/api/data', {
    method: 'POST',
    headers: {
        'X-CSRF-Token': token,
        'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
});
```

### 2. Input Sanitization

```go
// Create sanitizer
sanitizer := security.NewSanitizer(security.DefaultSanitizerConfig())

// Sanitize user input
clean := sanitizer.Sanitize(userInput)

// Quick sanitization
cleanHTML := security.SanitizeHTML("<script>alert('xss')</script>Hello")
// Output: &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;Hello

stripHTML := security.StripHTML("<b>Hello</b> <script>alert(1)</script>")
// Output: Hello

filename := security.SanitizeFilename("../../etc/passwd")
// Output: etcpasswd
```

### 3. RBAC

```go
// Create RBAC with default roles
rbac := security.SetupDefaultRBAC()

// Create a subject (user)
user := &security.Subject{
    ID:    "user123",
    Roles: []string{"user"},
}

// Check permission
if rbac.HasPermission(user, "posts:write") {
    // Allow action
}

// Add subject to context
ctx := security.WithSubject(context.Background(), user)

// Retrieve from context
subject := security.GetSubject(ctx)
```

### 4. Password Hashing

```go
// Hash a password
hash, err := security.HashPassword("mySecurePassword123")

// Verify password
valid, err := security.VerifyPassword("mySecurePassword123", hash)

// Check password strength
err := security.CheckPasswordStrength("weak")
// Returns error if password doesn't meet requirements
```

## CSRF Protection

### Configuration

```go
config := &security.CSRFConfig{
    TokenLength:    32,
    TokenLifetime:  24 * time.Hour,
    CookieName:     "csrf_token",
    HeaderName:     "X-CSRF-Token",
    FieldName:      "csrf_token",
    CookiePath:     "/",
    CookieSecure:   true,
    CookieSameSite: http.SameSiteStrictMode,
    SafeMethods:    []string{"GET", "HEAD", "OPTIONS", "TRACE"},
}

csrf := security.NewCSRFProtection(config)
```

### Custom Error Handler

```go
config.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
    log.Printf("CSRF validation failed: %v", err)
    http.Error(w, "Invalid request", http.StatusForbidden)
}
```

### Getting Token in Handler

```go
func handler(w http.ResponseWriter, r *http.Request) {
    token := security.GetCSRFToken(r)
    
    // Render in template
    tmpl.Execute(w, map[string]interface{}{
        "csrf_token": token,
    })
}
```

### Template Helpers

```go
// Hidden form field
token := security.CSRFTokenHTML(csrfToken)
// <input type="hidden" name="csrf_token" value="...">

// Meta tag for JavaScript
meta := security.CSRFTokenMeta(csrfToken)
// <meta name="csrf-token" content="...">
```

## Input Sanitization

### Custom Sanitizer

```go
config := &security.SanitizerConfig{
    AllowHTML:          true,
    AllowedTags:        []string{"b", "i", "u", "p", "br"},
    StripHTML:          false,
    TrimWhitespace:     true,
    RemoveNullBytes:    true,
    RemoveControlChars: true,
}

sanitizer := security.NewSanitizer(config)
result := sanitizer.Sanitize(input)
```

### Validation Functions

```go
// Email validation
if security.IsValidEmail("user@example.com") {
    // Valid email
}

// Username validation
if security.IsValidUsername("john_doe") {
    // Valid username (alphanumeric, underscore, hyphen, 3-32 chars)
}

// URL validation
if security.IsValidURL("https://example.com") {
    // Valid URL
}

// SQL injection detection
if security.ContainsSQLInjection(input) {
    // Potential SQL injection attempt
}

// XSS detection
if security.ContainsXSS(input) {
    // Potential XSS attempt
}
```

### Specialized Sanitizers

```go
// Sanitize filename
clean := security.SanitizeFilename("../../etc/passwd")

// Sanitize URL
clean := security.SanitizeURL("javascript:alert(1)")

// Sanitize SQL (basic - use parameterized queries!)
clean := security.SanitizeSQL("'; DROP TABLE users; --")

// Escape HTML
clean := security.SanitizeHTML("<script>alert('xss')</script>")

// Strip HTML completely
clean := security.StripHTML("<b>Bold</b> text")
```

## RBAC

### Define Custom Roles

```go
rbac := security.NewRBAC()

// Add roles
rbac.AddRole(&security.Role{
    Name:        "editor",
    Description: "Content editor",
    Permissions: []string{},
    Parent:      "user", // Inherits from user role
})

// Add permissions
rbac.AddPermission(&security.Permission{
    Name:        "articles:publish",
    Description: "Publish articles",
    Resource:    "articles",
    Action:      "publish",
})

// Grant permission to role
rbac.GrantPermission("editor", "articles:publish")
```

### Check Permissions

```go
subject := &security.Subject{
    ID:    "user123",
    Roles: []string{"editor"},
}

// Check specific permission
if rbac.HasPermission(subject, "articles:publish") {
    // User can publish articles
}

// Check role
if rbac.HasRole(subject, "editor") {
    // User is an editor
}

// Get all permissions
perms := rbac.GetSubjectPermissions(subject)
```

### Default RBAC Setup

```go
rbac := security.SetupDefaultRBAC()

// Roles: admin, user, moderator, guest
// Permissions:
//   - users: read, write, delete
//   - posts: read, write, delete
//   - comments: read, write, delete
//   - admin: access

// Admin: All permissions
// Moderator: Inherits user + delete posts/comments
// User: Read/write posts and comments
// Guest: Read only
```

### Context Integration

```go
// Store subject in context
ctx := security.WithSubject(r.Context(), &security.Subject{
    ID:    userID,
    Roles: []string{"user"},
})

// Later, retrieve from context
subject := security.GetSubject(ctx)
if rbac.HasPermission(subject, "posts:delete") {
    // Allow deletion
}
```

## Password Hashing

### Hash Configuration

```go
hasher := security.NewPasswordHasher()

// Custom configuration
hasher.memory = 128 * 1024  // 128 MB
hasher.iterations = 4
hasher.parallelism = 4
hasher.saltLength = 16
hasher.keyLength = 32

hash, err := hasher.Hash("password123")
```

### Password Verification

```go
hasher := security.NewPasswordHasher()

// Hash password
hash, err := hasher.Hash("myPassword123")
if err != nil {
    log.Fatal(err)
}

// Verify password
valid, err := hasher.Verify("myPassword123", hash)
if err != nil {
    log.Fatal(err)
}

if valid {
    // Password matches
} else {
    // Invalid password
}
```

### Password Strength Validation

```go
strength := &security.PasswordStrength{
    MinLength:      12,
    RequireUpper:   true,
    RequireLower:   true,
    RequireNumber:  true,
    RequireSpecial: true,
}

if err := strength.Check(password); err != nil {
    // Password doesn't meet requirements
    fmt.Println(err)
}
```

### User Registration Example

```go
func registerUser(username, password string) error {
    // Check password strength
    if err := security.CheckPasswordStrength(password); err != nil {
        return fmt.Errorf("weak password: %w", err)
    }
    
    // Hash password
    hash, err := security.HashPassword(password)
    if err != nil {
        return err
    }
    
    // Store user with hashed password
    return db.CreateUser(username, hash)
}
```

### User Login Example

```go
func loginUser(username, password string) (bool, error) {
    // Get user from database
    user, err := db.GetUserByUsername(username)
    if err != nil {
        return false, err
    }
    
    // Verify password
    valid, err := security.VerifyPassword(password, user.PasswordHash)
    if err != nil {
        return false, err
    }
    
    return valid, nil
}
```

## Security Utilities

### Token Generation

```go
// Generate random token
token, err := security.GenerateToken(32) // 32 bytes
// Returns base64-encoded string

// Generate secret key
secret, err := security.GenerateSecret(32)
// Returns raw bytes
```

### Secure Comparison

```go
// Constant-time string comparison (prevents timing attacks)
if security.SecureCompare(userToken, expectedToken) {
    // Tokens match
}

// Constant-time byte comparison
if security.SecureCompareBytes(hash1, hash2) {
    // Hashes match
}
```

## Integration Examples

### Complete Authentication Flow

```go
func setupAuth() http.Handler {
    // Setup components
    csrf := security.NewCSRFProtection(nil)
    rbac := security.SetupDefaultRBAC()
    sanitizer := security.NewSanitizer(nil)
    
    mux := http.NewServeMux()
    
    // Registration
    mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            return
        }
        
        // Sanitize inputs
        username := sanitizer.Sanitize(r.FormValue("username"))
        password := r.FormValue("password")
        
        // Validate
        if !security.IsValidUsername(username) {
            http.Error(w, "Invalid username", http.StatusBadRequest)
            return
        }
        
        // Check password strength
        if err := security.CheckPasswordStrength(password); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        
        // Hash password
        hash, err := security.HashPassword(password)
        if err != nil {
            http.Error(w, "Registration failed", http.StatusInternalServerError)
            return
        }
        
        // Create user in database
        // db.CreateUser(username, hash)
        
        w.Write([]byte("Registration successful"))
    })
    
    // Login
    mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            return
        }
        
        username := sanitizer.Sanitize(r.FormValue("username"))
        password := r.FormValue("password")
        
        // Get user
        // user := db.GetUser(username)
        
        // Verify password
        // valid, _ := security.VerifyPassword(password, user.PasswordHash)
        
        // Create session and set subject in context
        subject := &security.Subject{
            ID:    username,
            Roles: []string{"user"},
        }
        
        ctx := security.WithSubject(r.Context(), subject)
        r = r.WithContext(ctx)
        
        w.Write([]byte("Login successful"))
    })
    
    // Protected endpoint
    mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
        subject := security.GetSubject(r.Context())
        if subject == nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        
        if !rbac.HasPermission(subject, "admin:access") {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        
        w.Write([]byte("Admin panel"))
    })
    
    // Apply CSRF protection
    return csrf.Middleware(mux)
}
```

### API Input Validation

```go
func handleAPIRequest(w http.ResponseWriter, r *http.Request) {
    sanitizer := security.NewSanitizer(nil)
    
    // Parse JSON
    var data map[string]string
    json.NewDecoder(r.Body).Decode(&data)
    
    // Sanitize all inputs
    for key, value := range data {
        // Check for injection attempts
        if security.ContainsSQLInjection(value) {
            http.Error(w, "Invalid input", http.StatusBadRequest)
            return
        }
        
        if security.ContainsXSS(value) {
            http.Error(w, "Invalid input", http.StatusBadRequest)
            return
        }
        
        // Sanitize
        data[key] = sanitizer.Sanitize(value)
    }
    
    // Process sanitized data
}
```

## Best Practices

### 1. Always Use CSRF Protection

```go
csrf := security.NewCSRFProtection(nil)
handler := csrf.Middleware(yourHandler)
```

### 2. Sanitize All User Input

```go
sanitizer := security.NewSanitizer(nil)
clean := sanitizer.Sanitize(userInput)
```

### 3. Use Parameterized Queries

```go
// NEVER concatenate SQL
// BAD: db.Query("SELECT * FROM users WHERE id = " + userID)

// GOOD: Use parameterized queries
db.Query("SELECT * FROM users WHERE id = $1", userID)
```

### 4. Hash Passwords with Argon2id

```go
hash, _ := security.HashPassword(password)
// Store hash, never plain password
```

### 5. Implement RBAC

```go
rbac := security.SetupDefaultRBAC()
if !rbac.HasPermission(subject, "resource:action") {
    return security.ErrAccessDenied
}
```

### 6. Validate Input Types

```go
if !security.IsValidEmail(email) {
    return errors.New("invalid email")
}
```

### 7. Use Secure Comparison

```go
// For tokens, hashes, secrets
if security.SecureCompare(token1, token2) {
    // Safe from timing attacks
}
```

## Security Checklist

- [ ] CSRF protection on all state-changing endpoints
- [ ] Input sanitization on all user inputs
- [ ] Password hashing with Argon2id
- [ ] Password strength requirements
- [ ] RBAC for access control
- [ ] SQL injection prevention (parameterized queries)
- [ ] XSS prevention (sanitize output)
- [ ] Secure token generation
- [ ] Constant-time comparisons for secrets
- [ ] HTTPS only in production
- [ ] Secure cookie flags
- [ ] Rate limiting
- [ ] Audit logging

## License

Part of the goengine framework.
