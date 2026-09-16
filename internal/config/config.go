package config

import (
	"fmt"
	"log"
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
	APIKey              string
	LegacyAPIKeyEnabled bool

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

	// Sentry
	SentryDSN         string
	SentryEnvironment string
	SentryRelease     string
	SentrySampleRate  float64
}

// Load loads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	warnLegacyCacheSettings()

	cfg := &Config{
		// Bind locally by default. Deployments that intentionally expose the
		// process directly (for example, a local development container) can
		// explicitly set SERVER_HOST=0.0.0.0.
		ServerHost: getEnv("SERVER_HOST", "127.0.0.1"),
		ServerPort: getEnvInt("SERVER_PORT", 8080),

		DatabaseURL:    getEnv("DATABASE_URL", ""),
		DBMaxOpenConns: getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: getEnvInt("DB_MAX_IDLE_CONNS", 5),

		APIKey: getEnv("API_KEY", ""),
		// Keep the environment key enabled by default for V1 compatibility.
		// Operators can disable it after migrating to managed credentials.
		LegacyAPIKeyEnabled: getEnvBool("LEGACY_API_KEY_ENABLED", true),

		RateLimitRequests:      getEnvInt("RATE_LIMIT_REQUESTS", 10),
		RateLimitGetMultiplier: getEnvInt("RATE_LIMIT_GET_MULTIPLIER", 3),
		RateLimitWindow:        getEnvInt("RATE_LIMIT_WINDOW", 60),
		RateLimitCleanup:       getEnvInt("RATE_LIMIT_CLEANUP", 300),

		CORSAllowedOrigins:   getEnv("CORS_ALLOWED_ORIGINS", "*"),
		CORSAllowedMethods:   getEnv("CORS_ALLOWED_METHODS", "GET,POST,OPTIONS"),
		CORSAllowedHeaders:   getEnv("CORS_ALLOWED_HEADERS", "Content-Type,Authorization,X-Request-ID,X-API-Key,Idempotency-Key"),
		CORSAllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", false),
		CORSMaxAge:           getEnvInt("CORS_MAX_AGE", 3600),

		LogLevel: getEnv("LOG_LEVEL", "info"),

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
	if cfg.RateLimitRequests < 1 {
		return nil, fmt.Errorf("RATE_LIMIT_REQUESTS must be at least 1")
	}
	if cfg.RateLimitWindow < 1 {
		return nil, fmt.Errorf("RATE_LIMIT_WINDOW must be at least 1 second")
	}
	if cfg.RateLimitCleanup < 1 {
		return nil, fmt.Errorf("RATE_LIMIT_CLEANUP must be at least 1 second")
	}
	if cfg.ServerPort < 1 || cfg.ServerPort > 65535 {
		return nil, fmt.Errorf("SERVER_PORT must be between 1 and 65535")
	}

	// Validate Sentry configuration
	if cfg.SentrySampleRate < 0 || cfg.SentrySampleRate > 1 {
		return nil, fmt.Errorf("SENTRY_SAMPLE_RATE must be between 0.0 and 1.0")
	}

	return cfg, nil
}

func warnLegacyCacheSettings() {
	for _, key := range []string{
		"CACHE_ENABLED",
		"CACHE_SIZE",
		"CACHE_TTL_SECONDS",
		"CACHE_WORKERS",
		"CACHE_QUEUE_SIZE",
		"CACHE_SHUTDOWN_WAIT",
	} {
		if _, ok := os.LookupEnv(key); ok {
			log.Printf("WARNING: %s is deprecated and ignored; PostgreSQL is the sole counter data path", key)
			return
		}
	}
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
