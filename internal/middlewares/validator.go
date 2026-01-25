package middlewares

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ValidatorConfig holds configuration for validator middleware
type ValidatorConfig struct {
	// Logger for structured logging (optional, uses slog.Default if nil)
	Logger *slog.Logger

	// ErrorHandler handles validation errors
	// Default: returns JSON error response
	ErrorHandler func(w http.ResponseWriter, r *http.Request, errors []ValidationError)

	// Skipper defines a function to skip middleware
	Skipper func(r *http.Request) bool

	// MaxBodySize limits the size of request body for validation
	// Default: 1MB
	MaxBodySize int64
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string      `json:"field"`
	Value   interface{} `json:"value,omitempty"`
	Tag     string      `json:"tag"`
	Message string      `json:"message"`
	Param   string      `json:"param,omitempty"`
}

// ValidationErrors is a slice of ValidationError
type ValidationErrors []ValidationError

// Error implements the error interface
func (ve ValidationErrors) Error() string {
	var messages []string
	for _, err := range ve {
		messages = append(messages, err.Message)
	}
	return strings.Join(messages, "; ")
}

// Validator interface for custom validators
type Validator interface {
	Validate(value interface{}, param string) bool
	Message(field, param string) string
}

// Built-in validators
var validators = map[string]Validator{
	"required": &RequiredValidator{},
	"email":    &EmailValidator{},
	"min":      &MinValidator{},
	"max":      &MaxValidator{},
	"len":      &LenValidator{},
	"regexp":   &RegexpValidator{},
	"oneof":    &OneOfValidator{},
	"numeric":  &NumericValidator{},
	"alpha":    &AlphaValidator{},
	"alphanum": &AlphaNumValidator{},
	"url":      &URLValidator{},
	"uuid":     &UUIDValidator{},
	"date":     &DateValidator{},
	"datetime": &DateTimeValidator{},
	"regex":    &RegexpValidator{},
}

// RequiredValidator validates that a field is not empty
type RequiredValidator struct{}

func (v *RequiredValidator) Validate(value interface{}, param string) bool {
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
			return rv.Len() > 0
		case reflect.Ptr:
			return !rv.IsNil()
		default:
			return !rv.IsZero()
		}
	}
}

func (v *RequiredValidator) Message(field, param string) string {
	return fmt.Sprintf("%s is required", field)
}

// EmailValidator validates email format
type EmailValidator struct{}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func (v *EmailValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}
	return emailRegex.MatchString(str)
}

func (v *EmailValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be a valid email address", field)
}

// MinValidator validates minimum value/length
type MinValidator struct{}

func (v *MinValidator) Validate(value interface{}, param string) bool {
	min, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return float64(len(v)) >= min
	case int, int8, int16, int32, int64:
		val := reflect.ValueOf(v).Int()
		return float64(val) >= min
	case uint, uint8, uint16, uint32, uint64:
		val := reflect.ValueOf(v).Uint()
		return float64(val) >= min
	case float32, float64:
		val := reflect.ValueOf(v).Float()
		return val >= min
	case []interface{}:
		return float64(len(v)) >= min
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return float64(rv.Len()) >= min
		}
	}
	return false
}

func (v *MinValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be at least %s", field, param)
}

// MaxValidator validates maximum value/length
type MaxValidator struct{}

func (v *MaxValidator) Validate(value interface{}, param string) bool {
	max, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return float64(len(v)) <= max
	case int, int8, int16, int32, int64:
		val := reflect.ValueOf(v).Int()
		return float64(val) <= max
	case uint, uint8, uint16, uint32, uint64:
		val := reflect.ValueOf(v).Uint()
		return float64(val) <= max
	case float32, float64:
		val := reflect.ValueOf(v).Float()
		return val <= max
	case []interface{}:
		return float64(len(v)) <= max
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return float64(rv.Len()) <= max
		}
	}
	return false
}

func (v *MaxValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be at most %s", field, param)
}

// LenValidator validates exact length
type LenValidator struct{}

func (v *LenValidator) Validate(value interface{}, param string) bool {
	length, err := strconv.Atoi(param)
	if err != nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return len(v) == length
	case []interface{}:
		return len(v) == length
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return rv.Len() == length
		}
	}
	return false
}

func (v *LenValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be exactly %s characters long", field, param)
}

// RegexpValidator validates against a regular expression
type RegexpValidator struct{}

func (v *RegexpValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}

	regex, err := regexp.Compile(param)
	if err != nil {
		return false
	}

	return regex.MatchString(str)
}

func (v *RegexpValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must match the pattern %s", field, param)
}

// OneOfValidator validates that value is one of the allowed values
type OneOfValidator struct{}

func (v *OneOfValidator) Validate(value interface{}, param string) bool {
	str := fmt.Sprintf("%v", value)
	allowed := strings.Split(param, " ")

	for _, allow := range allowed {
		if str == allow {
			return true
		}
	}
	return false
}

func (v *OneOfValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be one of: %s", field, strings.ReplaceAll(param, " ", ", "))
}

// NumericValidator validates numeric values
type NumericValidator struct{}

var numericRegex = regexp.MustCompile(`^[0-9]+$`)

func (v *NumericValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}
	return numericRegex.MatchString(str)
}

func (v *NumericValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must contain only numbers", field)
}

// AlphaValidator validates alphabetic characters
type AlphaValidator struct{}

var alphaRegex = regexp.MustCompile(`^[a-zA-Z]+$`)

func (v *AlphaValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}
	return alphaRegex.MatchString(str)
}

func (v *AlphaValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must contain only letters", field)
}

// AlphaNumValidator validates alphanumeric characters
type AlphaNumValidator struct{}

var alphaNumRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func (v *AlphaNumValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}
	return alphaNumRegex.MatchString(str)
}

func (v *AlphaNumValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must contain only letters and numbers", field)
}

// URLValidator validates URL format
type URLValidator struct{}

func (v *URLValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}

	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func (v *URLValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be a valid URL", field)
}

// UUIDValidator validates UUID format
type UUIDValidator struct{}

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func (v *UUIDValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}
	return uuidRegex.MatchString(strings.ToLower(str))
}

func (v *UUIDValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be a valid UUID", field)
}

// DateValidator validates date format (YYYY-MM-DD)
type DateValidator struct{}

func (v *DateValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}

	_, err := time.Parse("2006-01-02", str)
	return err == nil
}

func (v *DateValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be a valid date (YYYY-MM-DD)", field)
}

// DateTimeValidator validates datetime format (RFC3339)
type DateTimeValidator struct{}

func (v *DateTimeValidator) Validate(value interface{}, param string) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}

	_, err := time.Parse(time.RFC3339, str)
	return err == nil
}

func (v *DateTimeValidator) Message(field, param string) string {
	return fmt.Sprintf("%s must be a valid datetime (RFC3339)", field)
}

// ValidationRule represents a validation rule
type ValidationRule struct {
	Field string
	Rules string
}

// DefaultValidatorConfig returns a default validator configuration
func DefaultValidatorConfig() *ValidatorConfig {
	return &ValidatorConfig{
		ErrorHandler: defaultErrorHandler,
		Skipper:      nil,
		MaxBodySize:  1024 * 1024, // 1MB
	}
}

// defaultErrorHandler is the default error handler
func defaultErrorHandler(w http.ResponseWriter, r *http.Request, errors []ValidationError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	response := map[string]interface{}{
		"status":  "error",
		"message": "Validation failed",
		"details": "One or more fields failed validation",
		"errors":  errors,
	}

	json.NewEncoder(w).Encode(response)
}

// ValidateJSON validates JSON request body
func ValidateJSON(rules []ValidationRule, config *ValidatorConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultValidatorConfig()
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("json validator middleware initialized",
		"rules_count", len(rules),
		"max_body_size", config.MaxBodySize,
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				logger.Debug("json validation skipped",
					"method", r.Method,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			// Skip validation for OPTIONS preflight requests
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Only validate for methods with body
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			// Read and parse JSON body
			body, err := io.ReadAll(io.LimitReader(r.Body, config.MaxBodySize))
			if err != nil {
				logger.Warn("failed to read request body",
					"method", r.Method,
					"path", r.URL.Path,
					"error", err,
				)
				config.ErrorHandler(w, r, []ValidationError{
					{Field: "body", Message: "Failed to read request body"},
				})
				return
			}

			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				logger.Warn("invalid json format",
					"method", r.Method,
					"path", r.URL.Path,
					"error", err,
				)
				config.ErrorHandler(w, r, []ValidationError{
					{Field: "body", Message: "Invalid JSON format"},
				})
				return
			}

			// Validate fields
			var validationErrors []ValidationError
			for _, rule := range rules {
				errors := validateField(rule.Field, data[rule.Field], rule.Rules)
				validationErrors = append(validationErrors, errors...)
			}

			if len(validationErrors) > 0 {
				logger.Info("json validation failed",
					"method", r.Method,
					"path", r.URL.Path,
					"errors_count", len(validationErrors),
				)
				config.ErrorHandler(w, r, validationErrors)
				return
			}

			logger.Debug("json validation passed",
				"method", r.Method,
				"path", r.URL.Path,
			)

			// Replace the consumed body so the next handler can read it
			r.Body = io.NopCloser(bytes.NewBuffer(body))

			next.ServeHTTP(w, r)
		})
	}
}

// ValidateQuery validates query parameters
func ValidateQuery(rules []ValidationRule, config *ValidatorConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultValidatorConfig()
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("query validator middleware initialized",
		"rules_count", len(rules),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				logger.Debug("query validation skipped",
					"method", r.Method,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			// Skip validation for OPTIONS preflight requests
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			query := r.URL.Query()
			var validationErrors []ValidationError

			for _, rule := range rules {
				value := query.Get(rule.Field)
				errors := validateField(rule.Field, value, rule.Rules)
				validationErrors = append(validationErrors, errors...)
			}

			if len(validationErrors) > 0 {
				logger.Info("query validation failed",
					"method", r.Method,
					"path", r.URL.Path,
					"errors_count", len(validationErrors),
				)
				config.ErrorHandler(w, r, validationErrors)
				return
			}

			logger.Debug("query validation passed",
				"method", r.Method,
				"path", r.URL.Path,
			)

			next.ServeHTTP(w, r)
		})
	}
}

// ValidateHeaders validates request headers
func ValidateHeaders(rules []ValidationRule, config *ValidatorConfig) func(next http.Handler) http.Handler {
	if config == nil {
		config = DefaultValidatorConfig()
	}

	// Use provided logger or default
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Debug("header validator middleware initialized",
		"rules_count", len(rules),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip middleware if skipper function returns true
			if config.Skipper != nil && config.Skipper(r) {
				logger.Debug("header validation skipped",
					"method", r.Method,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			// Skip validation for OPTIONS preflight requests
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			var validationErrors []ValidationError

			for _, rule := range rules {
				value := r.Header.Get(rule.Field)
				errors := validateField(rule.Field, value, rule.Rules)
				validationErrors = append(validationErrors, errors...)
			}

			if len(validationErrors) > 0 {
				logger.Info("header validation failed",
					"method", r.Method,
					"path", r.URL.Path,
					"errors_count", len(validationErrors),
				)
				config.ErrorHandler(w, r, validationErrors)
				return
			}

			logger.Debug("header validation passed",
				"method", r.Method,
				"path", r.URL.Path,
			)

			next.ServeHTTP(w, r)
		})
	}
}

// validateField validates a single field with given rules
func validateField(field string, value interface{}, rulesStr string) []ValidationError {
	var errors []ValidationError

	if rulesStr == "" {
		return errors
	}

	rules := strings.Split(rulesStr, ",")
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		parts := strings.SplitN(rule, "=", 2)

		tag := parts[0]
		param := ""
		if len(parts) > 1 {
			param = parts[1]
		}

		validator, exists := validators[tag]
		if !exists {
			continue
		}

		if !validator.Validate(value, param) {
			errors = append(errors, ValidationError{
				Field:   field,
				Value:   value,
				Tag:     tag,
				Message: validator.Message(field, param),
				Param:   param,
			})
		}
	}

	return errors
}

// RegisterValidator registers a custom validator
func RegisterValidator(tag string, validator Validator) {
	validators[tag] = validator
}
