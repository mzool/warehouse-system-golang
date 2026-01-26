package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config represents the complete application configuration
// This replaces the global AppSettings with a proper struct
type Config struct {
	App        AppConfig
	Database   DatabaseConfig
	Server     ServerConfig
	TLS        TLSConfig
	Auth       AuthConfig
	CORS       CORSConfig
	Pagination PaginationConfig
	Email      EmailConfig
	Rendering  RenderingConfig
	OpenAI     OpenAIConfig
	Redis      RedisConfig
}

// AppConfig holds application-level settings
type AppConfig struct {
	Version     string
	Environment string // development, staging, production
	BasePath    string
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port     string
	Protocol string // http or https
	Domain   string
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	URL               string
	MaxConns          int32
	MinConns          int32
	HealthCheckPeriod time.Duration
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	ConnectTimeout    time.Duration
	MaxRetries        int
	RetryDelay        time.Duration
}

// TLSConfig holds TLS/HTTPS certificate settings
type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	JWTSecret           string
	DefaultUserPassword string
	SessionSecret       string
}

// CORSConfig holds CORS middleware settings
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// PaginationConfig holds pagination settings
type PaginationConfig struct {
	DefaultPageSize int
	MaxPageSize     int
	DefaultPage     int
	AllowUnlimited  bool
}

// EmailConfig holds email/SMTP settings
type EmailConfig struct {
	SMTPHost              string
	SMTPPort              int
	SMTPUsername          string
	SMTPPassword          string
	TechnicalSupportEmail string
}

// RenderingConfig holds template and static file settings
type RenderingConfig struct {
	TemplatesDir string
	StaticDir    string
}

// OpenAIConfig holds OpenAI API settings
type OpenAIConfig struct {
	APIKey string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// LoadConfig loads configuration from environment variables
// Returns Config struct and error instead of mutating global state
func LoadConfig(logger *slog.Logger) (*Config, error) {
	// Load .env file (ignore error if it doesn't exist)
	godotenv.Load()

	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("loading application configuration")

	config := &Config{}

	// Load each section
	if err := loadAppConfig(&config.App, logger); err != nil {
		return nil, fmt.Errorf("failed to load app config: %w", err)
	}

	if err := loadServerConfig(&config.Server, logger); err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}

	if err := loadDatabaseConfig(&config.Database, logger); err != nil {
		return nil, fmt.Errorf("failed to load database config: %w", err)
	}

	loadTLSConfig(&config.TLS, logger)

	if err := loadAuthConfig(&config.Auth, logger); err != nil {
		return nil, fmt.Errorf("failed to load auth config: %w", err)
	}

	loadCORSConfig(&config.CORS, logger)
	loadPaginationConfig(&config.Pagination, logger)
	loadEmailConfig(&config.Email, logger)
	loadRenderingConfig(&config.Rendering, logger)
	loadOpenAIConfig(&config.OpenAI, logger)
	loadRedisConfig(&config.Redis, logger)
	logger.Info("configuration loaded successfully",
		"environment", config.App.Environment,
		"version", config.App.Version,
		"port", config.Server.Port,
	)

	return config, nil
}

func loadAppConfig(cfg *AppConfig, logger *slog.Logger) error {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "1.0.0"
		logger.Warn("VERSION not set, using default", "default", version)
	}
	cfg.Version = version

	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
		logger.Warn("ENV not set, using default", "default", env)
	}
	cfg.Environment = env

	basePath := os.Getenv("BASE_PATH")
	if basePath == "" {
		basePath = "/"
	}
	cfg.BasePath = basePath

	return nil
}

func loadServerConfig(cfg *ServerConfig, logger *slog.Logger) error {
	port := os.Getenv("PORT")
	if port == "" {
		return fmt.Errorf("PORT environment variable is required")
	}
	cfg.Port = port

	protocol := os.Getenv("PROTOCOL")
	if protocol == "" {
		protocol = "http"
		logger.Warn("PROTOCOL not set, using default", "default", protocol)
	}
	cfg.Protocol = protocol

	domain := os.Getenv("DOMAIN")
	if domain == "" {
		domain = "localhost"
		logger.Warn("DOMAIN not set, using default", "default", domain)
	}
	cfg.Domain = domain

	return nil
}

func loadDatabaseConfig(cfg *DatabaseConfig, logger *slog.Logger) error {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return fmt.Errorf("DB_URL environment variable is required")
	}
	cfg.URL = dbURL

	// Pool settings with defaults
	cfg.MaxConns = getEnvAsInt32("DB_MAX_CONNS", 10)
	cfg.MinConns = getEnvAsInt32("DB_MIN_CONNS", 2)

	// Duration settings
	healthCheckSec := getEnvAsInt32("DB_HEALTH_CHECK_PERIOD_SECONDS", 60)
	cfg.HealthCheckPeriod = time.Duration(healthCheckSec) * time.Second

	maxLifetimeMin := getEnvAsInt32("DB_MAX_CONN_LIFETIME_MINUTES", 0)
	cfg.MaxConnLifetime = time.Duration(maxLifetimeMin) * time.Minute

	maxIdleMin := getEnvAsInt32("DB_MAX_CONN_IDLE_TIME_MINUTES", 0)
	cfg.MaxConnIdleTime = time.Duration(maxIdleMin) * time.Minute

	// Connection settings
	cfg.ConnectTimeout = 10 * time.Second
	cfg.MaxRetries = 3
	cfg.RetryDelay = 1 * time.Second

	logger.Debug("database config loaded",
		"max_conns", cfg.MaxConns,
		"min_conns", cfg.MinConns,
	)

	return nil
}

func loadTLSConfig(cfg *TLSConfig, logger *slog.Logger) {
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")

	cfg.CertFile = certFile
	cfg.KeyFile = keyFile
	cfg.Enabled = certFile != "" && keyFile != ""

	if cfg.Enabled {
		logger.Info("TLS enabled", "cert_file", certFile, "key_file", keyFile)
	}
}

func loadAuthConfig(cfg *AuthConfig, logger *slog.Logger) error {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return fmt.Errorf("JWT_SECRET environment variable is required")
	}
	cfg.JWTSecret = jwtSecret

	defaultPassword := os.Getenv("DEFAULT_NEW_USER_PASSWORD")
	if defaultPassword == "" {
		logger.Warn("DEFAULT_NEW_USER_PASSWORD not set")
	}
	cfg.DefaultUserPassword = defaultPassword

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		return fmt.Errorf("SESSION_SECRET environment variable is required")
	}
	cfg.SessionSecret = sessionSecret

	return nil
}

func loadCORSConfig(cfg *CORSConfig, logger *slog.Logger) {
	// Allowed Origins
	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		cfg.AllowedOrigins = splitAndTrim(origins, ",")
	} else {
		cfg.AllowedOrigins = []string{"*"}
		logger.Warn("CORS_ALLOWED_ORIGINS not set, allowing all origins (not recommended for production)")
	}

	// Allowed Methods
	if methods := os.Getenv("CORS_ALLOWED_METHODS"); methods != "" {
		cfg.AllowedMethods = splitAndTrim(methods, ",")
	} else {
		cfg.AllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	}

	// Allowed Headers
	if headers := os.Getenv("CORS_ALLOWED_HEADERS"); headers != "" {
		cfg.AllowedHeaders = splitAndTrim(headers, ",")
	} else {
		cfg.AllowedHeaders = []string{"Content-Type", "Authorization", "X-Requested-With"}
	}

	// Exposed Headers
	if exposed := os.Getenv("CORS_EXPOSE_HEADERS"); exposed != "" {
		cfg.ExposedHeaders = splitAndTrim(exposed, ",")
	} else {
		cfg.ExposedHeaders = []string{}
	}

	// Credentials
	cfg.AllowCredentials = getEnvAsBool("CORS_ALLOW_CREDENTIALS", false)

	// Max Age
	cfg.MaxAge = getEnvAsInt("CORS_MAX_AGE", 3600)

	logger.Debug("CORS config loaded", "origins_count", len(cfg.AllowedOrigins))
}

func loadPaginationConfig(cfg *PaginationConfig, logger *slog.Logger) {
	cfg.DefaultPageSize = getEnvAsInt("PAGINATION_DEFAULT_SIZE", 20)
	cfg.MaxPageSize = getEnvAsInt("PAGINATION_MAX_SIZE", 100)
	cfg.DefaultPage = getEnvAsInt("PAGINATION_DEFAULT_PAGE", 1)
	cfg.AllowUnlimited = getEnvAsBool("PAGINATION_ALLOW_UNLIMITED", false)

	if cfg.AllowUnlimited {
		logger.Warn("pagination allows unlimited queries - this can be dangerous")
	}

	logger.Debug("pagination config loaded",
		"default_size", cfg.DefaultPageSize,
		"max_size", cfg.MaxPageSize,
	)
}

func loadEmailConfig(cfg *EmailConfig, logger *slog.Logger) {
	cfg.SMTPHost = os.Getenv("SMTP_HOST")
	cfg.SMTPPort = getEnvAsInt("SMTP_PORT", 587)
	cfg.SMTPUsername = os.Getenv("SMTP_USERNAME")
	cfg.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	cfg.TechnicalSupportEmail = os.Getenv("TechnicalSupportEmail")

	if cfg.SMTPHost != "" {
		logger.Debug("email config loaded", "smtp_host", cfg.SMTPHost, "smtp_port", cfg.SMTPPort)
	}
}

func loadRenderingConfig(cfg *RenderingConfig, logger *slog.Logger) {
	cfg.TemplatesDir = os.Getenv("TEMPLATES_DIR")
	if cfg.TemplatesDir == "" {
		cfg.TemplatesDir = "ui/templates"
		logger.Warn("TEMPLATES_DIR not set, using default", "default", cfg.TemplatesDir)
	}

	cfg.StaticDir = os.Getenv("STATIC_DIR")
	if cfg.StaticDir == "" {
		cfg.StaticDir = "ui/static"
		logger.Warn("STATIC_DIR not set, using default", "default", cfg.StaticDir)
	}
}

func loadOpenAIConfig(cfg *OpenAIConfig, logger *slog.Logger) {
	cfg.APIKey = os.Getenv("OPENAI_API_KEY")
	if cfg.APIKey != "" {
		logger.Debug("OpenAI API key loaded")
	}
}

func loadRedisConfig(cfg *RedisConfig, logger *slog.Logger) {
	cfg.Addr = os.Getenv("REDIS_ADDR")
	cfg.Password = os.Getenv("REDIS_PASSWORD")
	cfg.DB = getEnvAsInt("REDIS_DB", 0)

	if cfg.Addr != "" {
		logger.Debug("Redis config loaded", "addr", cfg.Addr, "db", cfg.DB)
	}
}

// Helper functions

func getEnvAsInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

func getEnvAsInt32(key string, defaultVal int32) int32 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return int32(parsed)
		}
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || val == "yes"
	}
	return defaultVal
}

func splitAndTrim(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// IsDevelopment returns true if running in development environment
func (c *Config) IsDevelopment() bool {
	return c.App.Environment == "development"
}

// IsProduction returns true if running in production environment
func (c *Config) IsProduction() bool {
	return c.App.Environment == "production"
}

// IsStaging returns true if running in staging environment
func (c *Config) IsStaging() bool {
	return c.App.Environment == "staging"
}

// GetServerAddress returns the full server address (protocol://domain:port)
func (c *Config) GetServerAddress() string {
	if c.Server.Protocol == "https" && c.Server.Port == "443" {
		return fmt.Sprintf("https://%s", c.Server.Domain)
	}
	if c.Server.Protocol == "http" && c.Server.Port == "80" {
		return fmt.Sprintf("http://%s", c.Server.Domain)
	}
	return fmt.Sprintf("%s://%s:%s", c.Server.Protocol, c.Server.Domain, c.Server.Port)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}
	if c.Database.URL == "" {
		return fmt.Errorf("database URL is required")
	}
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("JWT secret is required")
	}
	if c.IsProduction() && len(c.CORS.AllowedOrigins) == 1 && c.CORS.AllowedOrigins[0] == "*" {
		return fmt.Errorf("CORS wildcard origin (*) is not allowed in production")
	}
	return nil
}

// Deprecated: Global AppSettings is deprecated
// Use LoadConfig() to get a Config instance instead
// This is kept for backward compatibility only
var AppSettings = &Settings{}

// Deprecated: Settings struct is deprecated
// Use Config struct with LoadConfig() instead
type Settings struct {
	Version                     string
	BasePath                    string
	Env                         string
	Port                        string
	Protocol                    string
	Domain                      string
	TLSCertFile                 string
	TLSKeyFile                  string
	DB_URL                      string
	DB_MaxConns                 int32
	DB_MinConns                 int32
	DB_HealthCheckPeriodSeconds int32
	DB_MaxConnLifetimeMinutes   int32
	DB_MaxConnIdleTimeMinutes   int32
	TemplatesDir                string
	StaticDir                   string
	Pagination                  PaginationSettings
	CORS_ALLOWED_ORIGINS        []string
	CORS_ALLOWED_METHODS        []string
	CORS_ALLOWED_HEADERS        []string
	CORS_ALLOW_CREDENTIALS      bool
	CORS_MAX_AGE                int
	CORS_EXPOSE_HEADERS         []string
	JWTSecret                   string
	SMTPHost                    string
	SMTPPort                    int
	SMTPUsername                string
	SMTPPassword                string
	TechnicalSupportEmail       string
	OPENAI_API_KEY              string
	DEFAULT_NEW_USER_PASSWORD   string
}

// Deprecated: PaginationSettings is deprecated
// Use PaginationConfig instead
type PaginationSettings struct {
	DefaultPageSize int
	MaxPageSize     int
	DefaultPage     int
	AllowUnlimited  bool
}

// Deprecated: Load is deprecated
// Use LoadConfig() instead which returns Config and error
func Load() {
	godotenv.Load()

	AppSettings.DB_URL = os.Getenv("DB_URL")

	if val := os.Getenv("DB_MAX_CONNS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			AppSettings.DB_MaxConns = int32(parsed)
		}
	}
	if val := os.Getenv("DB_MIN_CONNS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			AppSettings.DB_MinConns = int32(parsed)
		}
	}
	if val := os.Getenv("DB_HEALTH_CHECK_PERIOD_SECONDS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			AppSettings.DB_HealthCheckPeriodSeconds = int32(parsed)
		}
	}
	if val := os.Getenv("DB_MAX_CONN_LIFETIME_MINUTES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			AppSettings.DB_MaxConnLifetimeMinutes = int32(parsed)
		}
	}
	if val := os.Getenv("DB_MAX_CONN_IDLE_TIME_MINUTES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			AppSettings.DB_MaxConnIdleTimeMinutes = int32(parsed)
		}
	}

	AppSettings.DEFAULT_NEW_USER_PASSWORD = os.Getenv("DEFAULT_NEW_USER_PASSWORD")
	AppSettings.Version = os.Getenv("VERSION")
	AppSettings.BasePath = os.Getenv("BASE_PATH")
	AppSettings.Port = os.Getenv("PORT")
	AppSettings.Env = os.Getenv("ENV")
	AppSettings.Protocol = os.Getenv("PROTOCOL")
	AppSettings.Domain = os.Getenv("DOMAIN")
	AppSettings.TLSCertFile = os.Getenv("TLS_CERT_FILE")
	AppSettings.TLSKeyFile = os.Getenv("TLS_KEY_FILE")
	AppSettings.TemplatesDir = os.Getenv("TEMPLATES_DIR")
	AppSettings.StaticDir = os.Getenv("STATIC_DIR")
	AppSettings.JWTSecret = os.Getenv("JWT_SECRET")

	loadPaginationSettingsOld()
	loadCORSSettingsOld()
	loadEmailSettingsOld()

	AppSettings.OPENAI_API_KEY = os.Getenv("OPENAI_API_KEY")
}

func loadPaginationSettingsOld() {
	defaultPageSize := 20
	if val := os.Getenv("PAGINATION_DEFAULT_SIZE"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			defaultPageSize = parsed
		}
	}

	maxPageSize := 100
	if val := os.Getenv("PAGINATION_MAX_SIZE"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			maxPageSize = parsed
		}
	}

	defaultPage := 1
	if val := os.Getenv("PAGINATION_DEFAULT_PAGE"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			defaultPage = parsed
		}
	}

	allowUnlimited := false
	if val := os.Getenv("PAGINATION_ALLOW_UNLIMITED"); val == "true" || val == "1" {
		allowUnlimited = true
	}

	AppSettings.Pagination = PaginationSettings{
		DefaultPageSize: defaultPageSize,
		MaxPageSize:     maxPageSize,
		DefaultPage:     defaultPage,
		AllowUnlimited:  allowUnlimited,
	}
}

func loadCORSSettingsOld() {
	if val := os.Getenv("CORS_EXPOSE_HEADERS"); val != "" {
		AppSettings.CORS_EXPOSE_HEADERS = splitAndTrim(val, ",")
	} else {
		AppSettings.CORS_EXPOSE_HEADERS = []string{}
	}

	if val := os.Getenv("CORS_ALLOWED_ORIGINS"); val != "" {
		AppSettings.CORS_ALLOWED_ORIGINS = splitAndTrim(val, ",")
	} else {
		AppSettings.CORS_ALLOWED_ORIGINS = []string{"*"}
	}

	if val := os.Getenv("CORS_ALLOWED_HEADERS"); val != "" {
		AppSettings.CORS_ALLOWED_HEADERS = splitAndTrim(val, ",")
	}

	if val := os.Getenv("CORS_MAX_AGE"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			AppSettings.CORS_MAX_AGE = parsed
		}
	}

	if val := os.Getenv("CORS_ALLOW_CREDENTIALS"); val != "" {
		if val == "true" || val == "1" {
			AppSettings.CORS_ALLOW_CREDENTIALS = true
		} else {
			AppSettings.CORS_ALLOW_CREDENTIALS = false
		}
	}

	if val := os.Getenv("CORS_ALLOWED_METHODS"); val != "" {
		AppSettings.CORS_ALLOWED_METHODS = splitAndTrim(val, ",")
	} else {
		AppSettings.CORS_ALLOWED_METHODS = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	}
}

func loadEmailSettingsOld() {
	AppSettings.SMTPHost = os.Getenv("SMTP_HOST")
	if val := os.Getenv("SMTP_PORT"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			AppSettings.SMTPPort = parsed
		}
	}
	AppSettings.SMTPUsername = os.Getenv("SMTP_USERNAME")
	AppSettings.SMTPPassword = os.Getenv("SMTP_PASSWORD")
	AppSettings.TechnicalSupportEmail = os.Getenv("TechnicalSupportEmail")
}

// Deprecated helper functions kept for compatibility
func GetAndCheckEnv(name string) string {
	return os.Getenv(name)
}

func SplitAndTrim(s, sep string) []string {
	return splitAndTrim(s, sep)
}
