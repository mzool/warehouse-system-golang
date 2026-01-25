package security

import (
"html"
"regexp"
"strings"
)

// Sanitizer provides input sanitization functionality
type Sanitizer struct {
config *SanitizerConfig
}

// SanitizerConfig holds sanitizer configuration
type SanitizerConfig struct {
// Allow HTML tags
AllowHTML bool

// Allowed HTML tags (only used if AllowHTML is true)
AllowedTags []string

// Strip all HTML tags
StripHTML bool

// Trim whitespace
TrimWhitespace bool

// Remove null bytes
RemoveNullBytes bool

// Remove control characters
RemoveControlChars bool
}

// DefaultSanitizerConfig returns a default sanitizer configuration
func DefaultSanitizerConfig() *SanitizerConfig {
return &SanitizerConfig{
AllowHTML:          false,
AllowedTags:        []string{},
StripHTML:          true,
TrimWhitespace:     true,
RemoveNullBytes:    true,
RemoveControlChars: true,
}
}

// NewSanitizer creates a new sanitizer instance
func NewSanitizer(config *SanitizerConfig) *Sanitizer {
if config == nil {
config = DefaultSanitizerConfig()
}

return &Sanitizer{
config: config,
}
}

// Sanitize sanitizes a string according to the configuration
func (s *Sanitizer) Sanitize(input string) string {
result := input

// Remove null bytes
if s.config.RemoveNullBytes {
result = strings.ReplaceAll(result, "\x00", "")
}

// Remove control characters
if s.config.RemoveControlChars {
result = s.removeControlChars(result)
}

// Handle HTML
if s.config.StripHTML {
result = s.stripHTML(result)
} else if !s.config.AllowHTML {
result = html.EscapeString(result)
} else if len(s.config.AllowedTags) > 0 {
result = s.stripDisallowedTags(result)
}

// Trim whitespace
if s.config.TrimWhitespace {
result = strings.TrimSpace(result)
}

return result
}

// SanitizeHTML escapes HTML special characters
func SanitizeHTML(input string) string {
return html.EscapeString(input)
}

// StripHTML removes all HTML tags from a string
func StripHTML(input string) string {
// Remove HTML tags
re := regexp.MustCompile(`<[^>]*>`)
result := re.ReplaceAllString(input, "")

// Decode HTML entities
result = html.UnescapeString(result)

return result
}

// SanitizeSQL escapes SQL special characters (basic protection)
func SanitizeSQL(input string) string {
replacer := strings.NewReplacer(
"'", "''",
"\\", "\\\\",
"\x00", "",
"\n", "\\n",
"\r", "\\r",
"\x1a", "\\Z",
)
return replacer.Replace(input)
}

// SanitizeFilename removes dangerous characters from filenames
func SanitizeFilename(filename string) string {
// Remove path separators
filename = strings.ReplaceAll(filename, "/", "")
filename = strings.ReplaceAll(filename, "\\", "")
filename = strings.ReplaceAll(filename, "..", "")

// Remove null bytes
filename = strings.ReplaceAll(filename, "\x00", "")

// Remove control characters
re := regexp.MustCompile(`[\x00-\x1f\x7f]`)
filename = re.ReplaceAllString(filename, "")

// Limit length
if len(filename) > 255 {
filename = filename[:255]
}

return filename
}

// SanitizeURL validates and sanitizes a URL
func SanitizeURL(input string) string {
// Remove null bytes and control characters
re := regexp.MustCompile(`[\x00-\x1f\x7f]`)
result := re.ReplaceAllString(input, "")

// Trim whitespace
result = strings.TrimSpace(result)

// Ensure it starts with a safe protocol
lower := strings.ToLower(result)
if !strings.HasPrefix(lower, "http://") &&
!strings.HasPrefix(lower, "https://") &&
!strings.HasPrefix(lower, "/") {
return ""
}

return result
}

// Internal methods

func (s *Sanitizer) stripHTML(input string) string {
return StripHTML(input)
}

func (s *Sanitizer) stripDisallowedTags(input string) string {
// Build allowed tags map
allowed := make(map[string]bool)
for _, tag := range s.config.AllowedTags {
allowed[strings.ToLower(tag)] = true
}

// Remove disallowed tags
re := regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9]*)[^>]*>`)
result := re.ReplaceAllStringFunc(input, func(match string) string {
// Extract tag name
tagRe := regexp.MustCompile(`</?([a-zA-Z][a-zA-Z0-9]*)`)
matches := tagRe.FindStringSubmatch(match)
if len(matches) < 2 {
return ""
}

tagName := strings.ToLower(matches[1])
if allowed[tagName] {
return match
}

return ""
})

return result
}

func (s *Sanitizer) removeControlChars(input string) string {
// Remove control characters except newlines and tabs
re := regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
return re.ReplaceAllString(input, "")
}

// Validator functions for common patterns

// IsValidEmail checks if a string is a valid email address
func IsValidEmail(email string) bool {
re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
return re.MatchString(email)
}

// IsValidUsername checks if a string is a valid username
func IsValidUsername(username string) bool {
// Allow alphanumeric, underscore, hyphen (3-32 characters)
re := regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)
return re.MatchString(username)
}

// IsValidURL checks if a string is a valid URL
func IsValidURL(url string) bool {
re := regexp.MustCompile(`^https?://[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(/.*)?$`)
return re.MatchString(url)
}

// ContainsSQLInjection checks if a string contains SQL injection patterns
func ContainsSQLInjection(input string) bool {
patterns := []string{
`(?i)(union|select|insert|update|delete|drop|create|alter|exec|execute)`,
`(?i)(--|;|/\*|\*/|xp_|sp_)`,
`(?i)(or\s+\d+=\d+|and\s+\d+=\d+)`,
`(?i)(or\s+'[^']*'\s*=\s*'[^']*'|and\s+'[^']*'\s*=\s*'[^']*')`,
}

for _, pattern := range patterns {
re := regexp.MustCompile(pattern)
if re.MatchString(input) {
return true
}
}

return false
}

// ContainsXSS checks if a string contains XSS patterns
func ContainsXSS(input string) bool {
patterns := []string{
`(?i)<script[^>]*>`,
`(?i)javascript:`,
`(?i)on\w+\s*=`,
`(?i)<iframe[^>]*>`,
`(?i)<object[^>]*>`,
`(?i)<embed[^>]*>`,
}

for _, pattern := range patterns {
re := regexp.MustCompile(pattern)
if re.MatchString(input) {
return true
}
}

return false
}
