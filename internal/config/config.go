package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration
type Config struct {
	// Server
	ServerHost string
	ServerPort int

	// Database
	DBHost         string
	DBPort         int
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	DBMaxOpenConns int
	DBMaxIdleConns int

	// Security
	APIKey string

	// Rate Limiting
	RateLimitRequests int
	RateLimitWindow   int
	RateLimitCleanup  int

	// CORS
	CORSAllowedOrigins   string
	CORSAllowedMethods   string
	CORSAllowedHeaders   string
	CORSAllowCredentials bool
	CORSMaxAge           int

	// Logging
	LogLevel string
}

// Load loads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	cfg := &Config{
		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnvInt("SERVER_PORT", 8080),

		DBHost:         getEnv("DB_HOST", ""),
		DBPort:         getEnvInt("DB_PORT", 5432),
		DBUser:         getEnv("DB_USER", ""),
		DBPassword:     getEnv("DB_PASSWORD", ""),
		DBName:         getEnv("DB_NAME", ""),
		DBSSLMode:      getEnv("DB_SSL_MODE", "disable"),
		DBMaxOpenConns: getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: getEnvInt("DB_MAX_IDLE_CONNS", 5),

		APIKey: getEnv("API_KEY", ""),

		RateLimitRequests: getEnvInt("RATE_LIMIT_REQUESTS", 10),
		RateLimitWindow:   getEnvInt("RATE_LIMIT_WINDOW", 60),
		RateLimitCleanup:  getEnvInt("RATE_LIMIT_CLEANUP", 300),

		CORSAllowedOrigins:   getEnv("CORS_ALLOWED_ORIGINS", "*"),
		CORSAllowedMethods:   getEnv("CORS_ALLOWED_METHODS", "GET,POST,OPTIONS"),
		CORSAllowedHeaders:   getEnv("CORS_ALLOWED_HEADERS", "Content-Type,Authorization,X-Request-ID"),
		CORSAllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", false),
		CORSMaxAge:           getEnvInt("CORS_MAX_AGE", 3600),

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	// Validate required fields
	if cfg.DBHost == "" || cfg.DBUser == "" || cfg.DBPassword == "" || cfg.DBName == "" {
		return nil, fmt.Errorf("missing required database configuration")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("missing required API_KEY")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
