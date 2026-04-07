package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear all env vars
	for _, env := range []string{
		"SERVER_HOST", "SERVER_PORT",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"API_KEY", "RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW",
	} {
		os.Unsetenv(env)
	}

	// Set minimum required vars
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_NAME", "testdb")
	os.Setenv("API_KEY", "test-key")
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
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
	os.Setenv("DB_NAME", "testdb")
	os.Setenv("API_KEY", "test-key")
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("API_KEY")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.ServerPort != 9000 {
		t.Errorf("Expected ServerPort 9000, got %d", cfg.ServerPort)
	}
	if cfg.DBName != "testdb" {
		t.Errorf("Expected DBName 'testdb', got '%s'", cfg.DBName)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("Expected APIKey 'test-key', got '%s'", cfg.APIKey)
	}
}

func TestValidateRequired(t *testing.T) {
	requiredVars := []string{
		"DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "API_KEY",
	}

	for _, env := range requiredVars {
		os.Unsetenv(env)
	}

	_, err := Load()
	if err == nil {
		t.Error("Expected error for missing required variables, got nil")
	}
}
