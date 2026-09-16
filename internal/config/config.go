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
	DatabaseURL                string
	DBMaxOpenConns             int
	DBMaxIdleConns             int
	DBMaxIdleTime              int
	DBTimeoutMS                int
	DBStatementTimeoutMS       int
	DBLockTimeoutMS            int
	DBIdleTransactionTimeoutMS int

	// Request and server protection
	RequestReadTimeoutSeconds     int
	RequestMutationTimeoutSeconds int
	ServerReadTimeoutSeconds      int
	ServerWriteTimeoutSeconds     int
	ServerIdleTimeoutSeconds      int
	ServerConcurrency             int
	ServerMaxConnsPerIP           int
	MaxRequestBodyBytes           int

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

		DatabaseURL:                getEnv("DATABASE_URL", ""),
		DBMaxOpenConns:             getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:             getEnvInt("DB_MAX_IDLE_CONNS", 5),
		DBMaxIdleTime:              getEnvInt("DB_MAX_IDLE_TIME_SECONDS", 300),
		DBTimeoutMS:                getEnvInt("DB_TIMEOUT_MS", 2000),
		DBStatementTimeoutMS:       getEnvInt("DB_STATEMENT_TIMEOUT_MS", 2000),
		DBLockTimeoutMS:            getEnvInt("DB_LOCK_TIMEOUT_MS", 500),
		DBIdleTransactionTimeoutMS: getEnvInt("DB_IDLE_TRANSACTION_TIMEOUT_MS", 10000),

		RequestReadTimeoutSeconds:     getEnvInt("REQUEST_READ_TIMEOUT_SECONDS", 5),
		RequestMutationTimeoutSeconds: getEnvInt("REQUEST_MUTATION_TIMEOUT_SECONDS", 10),
		ServerReadTimeoutSeconds:      getEnvInt("SERVER_READ_TIMEOUT_SECONDS", 10),
		ServerWriteTimeoutSeconds:     getEnvInt("SERVER_WRITE_TIMEOUT_SECONDS", 10),
		ServerIdleTimeoutSeconds:      getEnvInt("SERVER_IDLE_TIMEOUT_SECONDS", 30),
		ServerConcurrency:             getEnvInt("SERVER_CONCURRENCY", 128),
		ServerMaxConnsPerIP:           getEnvInt("SERVER_MAX_CONNS_PER_IP", 100),
		MaxRequestBodyBytes:           getEnvInt("MAX_REQUEST_BODY_BYTES", 64*1024),

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
	if cfg.DBMaxOpenConns < 1 {
		return nil, fmt.Errorf("DB_MAX_OPEN_CONNS must be at least 1")
	}
	if cfg.DBMaxIdleConns < 0 || cfg.DBMaxIdleConns > cfg.DBMaxOpenConns {
		return nil, fmt.Errorf("DB_MAX_IDLE_CONNS must be between 0 and DB_MAX_OPEN_CONNS")
	}
	if cfg.DBMaxIdleTime < 1 {
		return nil, fmt.Errorf("DB_MAX_IDLE_TIME_SECONDS must be at least 1")
	}
	if cfg.DBTimeoutMS < 1 || cfg.DBStatementTimeoutMS < 1 || cfg.DBLockTimeoutMS < 1 || cfg.DBIdleTransactionTimeoutMS < 1 {
		return nil, fmt.Errorf("database timeout settings must be positive")
	}
	if cfg.RequestReadTimeoutSeconds < 1 || cfg.RequestMutationTimeoutSeconds < 1 {
		return nil, fmt.Errorf("request timeout settings must be positive")
	}
	if cfg.ServerReadTimeoutSeconds < 1 || cfg.ServerWriteTimeoutSeconds < 1 || cfg.ServerIdleTimeoutSeconds < 1 {
		return nil, fmt.Errorf("server timeout settings must be positive")
	}
	if cfg.ServerConcurrency < 1 || cfg.ServerMaxConnsPerIP < 1 {
		return nil, fmt.Errorf("server concurrency settings must be positive")
	}
	if cfg.MaxRequestBodyBytes < 1 {
		return nil, fmt.Errorf("MAX_REQUEST_BODY_BYTES must be positive")
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
