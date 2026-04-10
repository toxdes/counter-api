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

	if cfg.ServerHost != "0.0.0.0" {
		t.Errorf("Expected ServerHost default '0.0.0.0', got '%s'", cfg.ServerHost)
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
