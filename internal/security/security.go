package security

import (
"crypto/rand"
"crypto/subtle"
"encoding/base64"
"errors"
"fmt"
"io"
)

// Common security errors
var (
ErrInvalidToken      = errors.New("invalid security token")
ErrTokenExpired      = errors.New("security token expired")
ErrInsufficientPerms = errors.New("insufficient permissions")
ErrAccessDenied      = errors.New("access denied")
)

// GenerateToken generates a cryptographically secure random token
func GenerateToken(length int) (string, error) {
bytes := make([]byte, length)
if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
return "", fmt.Errorf("failed to generate token: %w", err)
}
return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateSecret generates a cryptographically secure random secret
func GenerateSecret(length int) ([]byte, error) {
secret := make([]byte, length)
if _, err := io.ReadFull(rand.Reader, secret); err != nil {
return nil, fmt.Errorf("failed to generate secret: %w", err)
}
return secret, nil
}

// SecureCompare performs a constant-time comparison of two strings
func SecureCompare(a, b string) bool {
return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// SecureCompareBytes performs a constant-time comparison of two byte slices
func SecureCompareBytes(a, b []byte) bool {
return subtle.ConstantTimeCompare(a, b) == 1
}
