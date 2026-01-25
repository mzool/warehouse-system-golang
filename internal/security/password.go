package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2 configuration parameters (recommended by OWASP)
const (
	DefaultMemory      = 64 * 1024 // 64 MB
	DefaultIterations  = 3         // Number of iterations
	DefaultParallelism = 2         // Number of threads
	DefaultSaltLength  = 16        // Salt length in bytes
	DefaultKeyLength   = 32        // Hash length in bytes
)

var (
	ErrInvalidHash      = errors.New("invalid hash format")
	ErrWeakPassword     = errors.New("password does not meet strength requirements")
	ErrIncompatibleHash = errors.New("incompatible hash version")
)

// PasswordHasher provides password hashing functionality using Argon2id
type PasswordHasher struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

// NewPasswordHasher creates a new password hasher with default Argon2id settings
func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{
		memory:      DefaultMemory,
		iterations:  DefaultIterations,
		parallelism: DefaultParallelism,
		saltLength:  DefaultSaltLength,
		keyLength:   DefaultKeyLength,
	}
}

// Hash generates an Argon2id hash for the given password
// Returns hash in format: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
func (ph *PasswordHasher) Hash(password string) (string, error) {
	// Generate random salt
	salt := make([]byte, ph.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Generate hash using Argon2id
	hash := argon2.IDKey([]byte(password), salt, ph.iterations, ph.memory, ph.parallelism, ph.keyLength)

	// Encode salt and hash to base64
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// Format: $argon2id$v=19$m=memory,t=iterations,p=parallelism$salt$hash
	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, ph.memory, ph.iterations, ph.parallelism, b64Salt, b64Hash)

	return encodedHash, nil
}

// Verify checks if the password matches the hash
func (ph *PasswordHasher) Verify(password, encodedHash string) (bool, error) {
	// Parse the encoded hash
	params, salt, hash, err := ph.decodeHash(encodedHash)
	if err != nil {
		return false, err
	}

	// Generate hash from password using extracted parameters
	otherHash := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, params.keyLength)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(hash, otherHash) == 1 {
		return true, nil
	}

	return false, nil
}

// hashParams holds the parameters used for hashing
type hashParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

// decodeHash extracts parameters, salt, and hash from encoded string
func (ph *PasswordHasher) decodeHash(encodedHash string) (*hashParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return nil, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid version: %w", err)
	}

	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleHash
	}

	params := &hashParams{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.iterations, &params.parallelism); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid parameters: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid salt: %w", err)
	}
	params.saltLength = uint32(len(salt))

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid hash: %w", err)
	}
	params.keyLength = uint32(len(hash))

	return params, salt, hash, nil
}

// SetParams configures Argon2id parameters
func (ph *PasswordHasher) SetParams(memory, iterations uint32, parallelism uint8) error {
	if memory < 1024 {
		return fmt.Errorf("memory must be at least 1024 KB")
	}
	if iterations < 1 {
		return fmt.Errorf("iterations must be at least 1")
	}
	if parallelism < 1 {
		return fmt.Errorf("parallelism must be at least 1")
	}

	ph.memory = memory
	ph.iterations = iterations
	ph.parallelism = parallelism
	return nil
}

// PasswordStrength checks password strength
type PasswordStrength struct {
	MinLength      int
	RequireUpper   bool
	RequireLower   bool
	RequireNumber  bool
	RequireSpecial bool
}

// DefaultPasswordStrength returns default password strength requirements
func DefaultPasswordStrength() *PasswordStrength {
	return &PasswordStrength{
		MinLength:      8,
		RequireUpper:   true,
		RequireLower:   true,
		RequireNumber:  true,
		RequireSpecial: false,
	}
}

// Check validates a password against strength requirements
func (ps *PasswordStrength) Check(password string) error {
	if len(password) < ps.MinLength {
		return fmt.Errorf("%w: minimum length is %d", ErrWeakPassword, ps.MinLength)
	}

	hasUpper := false
	hasLower := false
	hasNumber := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasNumber = true
		default:
			hasSpecial = true
		}
	}

	if ps.RequireUpper && !hasUpper {
		return fmt.Errorf("%w: must contain uppercase letter", ErrWeakPassword)
	}

	if ps.RequireLower && !hasLower {
		return fmt.Errorf("%w: must contain lowercase letter", ErrWeakPassword)
	}

	if ps.RequireNumber && !hasNumber {
		return fmt.Errorf("%w: must contain number", ErrWeakPassword)
	}

	if ps.RequireSpecial && !hasSpecial {
		return fmt.Errorf("%w: must contain special character", ErrWeakPassword)
	}

	return nil
}

// HashPassword is a convenience function for hashing passwords
func HashPassword(password string) (string, error) {
	hasher := NewPasswordHasher()
	return hasher.Hash(password)
}

// VerifyPassword is a convenience function for verifying passwords
func VerifyPassword(password, hash string) (bool, error) {
	hasher := NewPasswordHasher()
	return hasher.Verify(password, hash)
}

// CheckPasswordStrength is a convenience function for checking password strength
func CheckPasswordStrength(password string) error {
	strength := DefaultPasswordStrength()
	return strength.Check(password)
}
