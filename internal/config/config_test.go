package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear all env vars
	for _, env := range []string{
		"SERVER_HOST", "SERVER_PORT",
		"DATABASE_URL",
		"API_KEY", "RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW",
	} {
		os.Unsetenv(env)
	}

	// Set minimum required vars
	os.Setenv("DATABASE_URL", "postgres://testuser:testpass@localhost/testdb?sslmode=disable")
	os.Setenv("API_KEY", "test-key")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed with defaults: %v", err)
	}

	if cfg.ServerHost != "127.0.0.1" {
		t.Errorf("Expected ServerHost default '127.0.0.1', got '%s'", cfg.ServerHost)
	}
	if cfg.ServerPort != 8080 {
		t.Errorf("Expected ServerPort default 8080, got %d", cfg.ServerPort)
	}
	if cfg.DBMaxOpenConns != 25 {
		t.Errorf("Expected DBMaxOpenConns default 25, got %d", cfg.DBMaxOpenConns)
	}
	if cfg.RateLimitRequests != 10 {
		t.Errorf("Expected RateLimitRequests default 10, got %d", cfg.RateLimitRequests)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("SERVER_PORT", "9000")
	os.Setenv("DATABASE_URL", "postgres://testuser:testpass@localhost:9000/testdb?sslmode=require")
	os.Setenv("API_KEY", "test-key")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.ServerPort != 9000 {
		t.Errorf("Expected ServerPort 9000, got %d", cfg.ServerPort)
	}
	if cfg.DatabaseURL != "postgres://testuser:testpass@localhost:9000/testdb?sslmode=require" {
		t.Errorf("Expected DatabaseURL 'postgres://testuser:testpass@localhost:9000/testdb?sslmode=require', got '%s'", cfg.DatabaseURL)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("Expected APIKey 'test-key', got '%s'", cfg.APIKey)
	}
}

func TestValidateRequired(t *testing.T) {
	requiredVars := []string{
		"DATABASE_URL", "API_KEY",
	}

	for _, env := range requiredVars {
		os.Unsetenv(env)
	}

	_, err := Load()
	if err == nil {
		t.Error("Expected error for missing required variables, got nil")
	}
}

func TestLegacyCacheSettingsAreIgnoredForCompatibility(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://testuser:testpass@localhost/testdb?sslmode=disable")
	t.Setenv("API_KEY", "test-key")
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("CACHE_SIZE", "0")
	t.Setenv("CACHE_TTL_SECONDS", "-1")
	t.Setenv("CACHE_WORKERS", "0")
	t.Setenv("CACHE_QUEUE_SIZE", "0")
	t.Setenv("CACHE_SHUTDOWN_WAIT", "0")

	_, err := Load()
	if err != nil {
		t.Fatalf("legacy cache settings must not affect Load(): %v", err)
	}
}

func TestSentryDefaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://testuser:testpass@localhost/testdb?sslmode=disable")
	os.Setenv("API_KEY", "test-key")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("API_KEY")
		os.Unsetenv("SENTRY_DSN")
		os.Unsetenv("SENTRY_ENVIRONMENT")
		os.Unsetenv("SENTRY_RELEASE")
		os.Unsetenv("SENTRY_SAMPLE_RATE")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.SentryDSN != "" {
		t.Errorf("Expected SentryDSN default empty, got '%s'", cfg.SentryDSN)
	}
	if cfg.SentryEnvironment != "development" {
		t.Errorf("Expected SentryEnvironment default 'development', got '%s'", cfg.SentryEnvironment)
	}
	if cfg.SentryRelease != "" {
		t.Errorf("Expected SentryRelease default empty, got '%s'", cfg.SentryRelease)
	}
	if cfg.SentrySampleRate != 0.5 {
		t.Errorf("Expected SentrySampleRate default 0.5, got %f", cfg.SentrySampleRate)
	}
}

func TestSentryFromEnv(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://testuser:testpass@localhost/testdb?sslmode=disable")
	os.Setenv("API_KEY", "test-key")
	os.Setenv("SENTRY_DSN", "https://test@sentry.io/123")
	os.Setenv("SENTRY_ENVIRONMENT", "production")
	os.Setenv("SENTRY_RELEASE", "v1.0.0")
	os.Setenv("SENTRY_SAMPLE_RATE", "0.8")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("API_KEY")
		os.Unsetenv("SENTRY_DSN")
		os.Unsetenv("SENTRY_ENVIRONMENT")
		os.Unsetenv("SENTRY_RELEASE")
		os.Unsetenv("SENTRY_SAMPLE_RATE")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.SentryDSN != "https://test@sentry.io/123" {
		t.Errorf("Expected SentryDSN 'https://test@sentry.io/123', got '%s'", cfg.SentryDSN)
	}
	if cfg.SentryEnvironment != "production" {
		t.Errorf("Expected SentryEnvironment 'production', got '%s'", cfg.SentryEnvironment)
	}
	if cfg.SentryRelease != "v1.0.0" {
		t.Errorf("Expected SentryRelease 'v1.0.0', got '%s'", cfg.SentryRelease)
	}
	if cfg.SentrySampleRate != 0.8 {
		t.Errorf("Expected SentrySampleRate 0.8, got %f", cfg.SentrySampleRate)
	}
}

func TestSentryValidation(t *testing.T) {
	tests := []struct {
		name    string
		envVar  string
		value   string
		wantErr bool
	}{
		{"negative sample rate", "SENTRY_SAMPLE_RATE", "-0.1", true},
		{"sample rate > 1", "SENTRY_SAMPLE_RATE", "1.5", true},
		{"valid sample rate", "SENTRY_SAMPLE_RATE", "0.75", false},
		{"zero sample rate", "SENTRY_SAMPLE_RATE", "0", false},
		{"one sample rate", "SENTRY_SAMPLE_RATE", "1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DATABASE_URL", "postgres://testuser:testpass@localhost/testdb?sslmode=disable")
			os.Setenv("API_KEY", "test-key")
			os.Setenv(tt.envVar, tt.value)
			defer func() {
				os.Unsetenv("DATABASE_URL")
				os.Unsetenv("API_KEY")
				os.Unsetenv(tt.envVar)
			}()

			_, err := Load()
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
