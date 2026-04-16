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
	DatabaseURL    string
	DBMaxOpenConns int
	DBMaxIdleConns int

	// Security
	APIKey string

	// Rate Limiting
	RateLimitRequests      int
	RateLimitGetMultiplier int
	RateLimitWindow        int
	RateLimitCleanup       int

	// CORS
	CORSAllowedOrigins   string
	CORSAllowedMethods   string
	CORSAllowedHeaders   string
	CORSAllowCredentials bool
	CORSMaxAge           int

	// Logging
	LogLevel string

	// Cache
	CacheEnabled      bool
	CacheSize         int
	CacheTTLSeconds   int
	CacheWorkers      int
	CacheQueueSize    int
	CacheShutdownWait int

	// Sentry
	SentryDSN         string
	SentryEnvironment string
	SentryRelease     string
	SentrySampleRate  float64
}

// Load loads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	cfg := &Config{
		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnvInt("SERVER_PORT", 8080),

		DatabaseURL:    getEnv("DATABASE_URL", ""),
		DBMaxOpenConns: getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: getEnvInt("DB_MAX_IDLE_CONNS", 5),

		APIKey: getEnv("API_KEY", ""),

		RateLimitRequests:      getEnvInt("RATE_LIMIT_REQUESTS", 10),
		RateLimitGetMultiplier: getEnvInt("RATE_LIMIT_GET_MULTIPLIER", 3),
		RateLimitWindow:        getEnvInt("RATE_LIMIT_WINDOW", 60),
		RateLimitCleanup:       getEnvInt("RATE_LIMIT_CLEANUP", 300),

		CORSAllowedOrigins:   getEnv("CORS_ALLOWED_ORIGINS", "*"),
		CORSAllowedMethods:   getEnv("CORS_ALLOWED_METHODS", "GET,POST,OPTIONS"),
		CORSAllowedHeaders:   getEnv("CORS_ALLOWED_HEADERS", "Content-Type,Authorization,X-Request-ID"),
		CORSAllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", false),
		CORSMaxAge:           getEnvInt("CORS_MAX_AGE", 3600),

		LogLevel: getEnv("LOG_LEVEL", "info"),

		CacheEnabled:      getEnvBool("CACHE_ENABLED", true),
		CacheSize:         getEnvInt("CACHE_SIZE", 1000),
		CacheTTLSeconds:   getEnvInt("CACHE_TTL_SECONDS", 300),
		CacheWorkers:      getEnvInt("CACHE_WORKERS", 2),
		CacheQueueSize:    getEnvInt("CACHE_QUEUE_SIZE", 10000),
		CacheShutdownWait: getEnvInt("CACHE_SHUTDOWN_WAIT", 5),

		SentryDSN:         getEnv("SENTRY_DSN", ""),
		SentryEnvironment: getEnv("SENTRY_ENVIRONMENT", "development"),
		SentryRelease:     getEnv("SENTRY_RELEASE", ""),
		SentrySampleRate:  getEnvFloat("SENTRY_SAMPLE_RATE", 0.5),
	}

	// Validate required fields
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("missing required DATABASE_URL")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("missing required API_KEY")
	}
	if cfg.RateLimitGetMultiplier < 1 {
		return nil, fmt.Errorf("RATE_LIMIT_GET_MULTIPLIER must be at least 1")
	}

	// Validate cache configuration
	if cfg.CacheEnabled {
		if cfg.CacheSize < 1 {
			return nil, fmt.Errorf("CACHE_SIZE must be at least 1")
		}
		if cfg.CacheTTLSeconds < 0 {
			return nil, fmt.Errorf("CACHE_TTL_SECONDS cannot be negative")
		}
		if cfg.CacheWorkers < 1 {
			return nil, fmt.Errorf("CACHE_WORKERS must be at least 1")
		}
		if cfg.CacheQueueSize < 1 {
			return nil, fmt.Errorf("CACHE_QUEUE_SIZE must be at least 1")
		}
	}

	// Validate Sentry configuration
	if cfg.SentrySampleRate < 0 || cfg.SentrySampleRate > 1 {
		return nil, fmt.Errorf("SENTRY_SAMPLE_RATE must be between 0.0 and 1.0")
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

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}
