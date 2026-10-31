package config

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/thienel/tlog"
)

// ServerConfig holds server configuration
type ServerConfig struct {
	Port        string
	Env         string
	ServiceName string
	Version     string
}

// DatabaseConfig holds PostgreSQL configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	TimeZone string
}

// JWTConfig holds JWT authentication configuration
type JWTConfig struct {
	Secret              string
	AccessExpiryMinutes int
	RefreshExpiryHours  int
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level         string
	EnableConsole bool
	FilePath      string
	MaxSizeMB     int
	MaxBackups    int
	MaxAgeDays    int
	Compress      bool
}

// CookieConfig holds cookie configuration
type CookieConfig struct {
	Name        string
	RefreshName string
	Domain      string
	Secure      bool
	SameSite    string
	Path        string
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool
	RequestsPerMinute int
}

// Config holds all application configuration
type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	JWT       JWTConfig
	Log       LogConfig
	Cookie    CookieConfig
	RateLimit RateLimitConfig

	RedisURL           string
	KBSURL             string
	CORSAllowedOrigins []string
}

var AppConfig *Config

// Load loads all configuration from environment variables
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		tlog.Warn("No .env file found, using environment variables")
	}

	AppConfig = &Config{
		Server:    loadServerConfig(),
		Database:  loadDatabaseConfig(),
		JWT:       loadJWTConfig(),
		Log:       loadLogConfig(),
		Cookie:    loadCookieConfig(),
		RateLimit: loadRateLimitConfig(),

		RedisURL:           getEnv("REDIS_URL", "redis://localhost:6379"),
		KBSURL:             getEnv("KBS_URL", ""),
		CORSAllowedOrigins: parseCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
	}

	return AppConfig, nil
}

func loadServerConfig() ServerConfig {
	return ServerConfig{
		Port:        getEnv("PORT", "8000"),
		Env:         getEnv("ENV", "development"),
		ServiceName: getEnv("SERVICE_NAME", "ai-fraud-detection"),
		Version:     getEnv("SERVICE_VERSION", "1.0.0"),
	}
}

func loadDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnvInt("DB_PORT", 5432),
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", ""),
		DBName:   getEnv("DB_NAME", "ai_fraud_detection"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
		TimeZone: getEnv("DB_TIMEZONE", "Asia/Ho_Chi_Minh"),
	}
}

func loadJWTConfig() JWTConfig {
	return JWTConfig{
		Secret:              getEnv("JWT_SECRET", "change-this-secret-in-production-min-32-chars"),
		AccessExpiryMinutes: getEnvInt("JWT_ACCESS_EXPIRY_MINUTES", 15),
		RefreshExpiryHours:  getEnvInt("JWT_REFRESH_EXPIRY_HOURS", 12),
	}
}

func loadLogConfig() LogConfig {
	return LogConfig{
		Level:         getEnv("LOG_LEVEL", "info"),
		EnableConsole: getEnvBool("LOG_ENABLE_CONSOLE", true),
		FilePath:      getEnv("LOG_FILE_PATH", "./logs/app.log"),
		MaxSizeMB:     getEnvInt("LOG_MAX_SIZE_MB", 100),
		MaxBackups:    getEnvInt("LOG_MAX_BACKUPS", 30),
		MaxAgeDays:    getEnvInt("LOG_MAX_AGE_DAYS", 90),
		Compress:      getEnvBool("LOG_COMPRESS", true),
	}
}

func loadCookieConfig() CookieConfig {
	return CookieConfig{
		Name:        getEnv("COOKIE_NAME", "app_token"),
		RefreshName: getEnv("COOKIE_REFRESH_NAME", "app_refresh"),
		Domain:      getEnv("COOKIE_DOMAIN", ""),
		Secure:      getEnvBool("COOKIE_SECURE", false),
		SameSite:    getEnv("COOKIE_SAMESITE", "Lax"),
		Path:        getEnv("COOKIE_PATH", "/"),
	}
}

func loadRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:           getEnvBool("RATE_LIMIT_ENABLED", true),
		RequestsPerMinute: getEnvInt("RATE_LIMIT_REQUESTS_PER_MIN", 60),
	}
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func parseCSV(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Helper methods on Config

func (c *Config) IsProduction() bool {
	return c.Server.Env == "production"
}

func (c *Config) IsDevelopment() bool {
	return c.Server.Env == "development"
}

func (c *Config) GetRedisAddr() string {
	parsed, err := url.Parse(c.RedisURL)
	if err != nil || parsed.Host == "" {
		return c.RedisURL
	}
	return parsed.Host
}

func GetConfig() *Config {
	return AppConfig
}
