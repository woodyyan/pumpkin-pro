package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear relevant env vars to ensure defaults
	for _, key := range []string{
		"PORT", "QUANT_SERVICE_URL", "BACKEND_CALLBACK_URL",
		"STRATEGY_SEED_PATH",
		"DB_TYPE", "DB_PATH", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
		"ADMIN_SEED_EMAIL", "ADMIN_SEED_PASSWORD",
		"AUTH_JWT_SECRET", "AUTH_ACCESS_TOKEN_TTL_MINUTES", "AUTH_REFRESH_TOKEN_TTL_HOURS",
		"AI_API_KEY", "AI_BASE_URL", "AI_MODEL", "AI_CONFIG_CIPHER_KEY",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	// Port
	if cfg.Port != "8080" {
		t.Errorf("expected default Port 8080, got %s", cfg.Port)
	}

	// QuantServiceURL should have trailing slash trimmed
	if cfg.QuantServiceURL != "http://localhost:8000" {
		t.Errorf("expected QuantServiceURL http://localhost:8000, got %s", cfg.QuantServiceURL)
	}

	if cfg.BackendCallbackURL != "http://localhost:8080" {
		t.Errorf("expected BackendCallbackURL http://localhost:8080, got %s", cfg.BackendCallbackURL)
	}

	// DB defaults
	if cfg.DB.Type != "sqlite" {
		t.Errorf("expected DB.Type sqlite, got %s", cfg.DB.Type)
	}
	if cfg.DB.Path != "data/pumpkin.db" {
		t.Errorf("expected DB.Path data/pumpkin.db, got %s", cfg.DB.Path)
	}
	if cfg.DB.Host != "localhost" {
		t.Errorf("expected DB.Host localhost, got %s", cfg.DB.Host)
	}
	if cfg.DB.Port != 5432 {
		t.Errorf("expected DB.Port 5432, got %d", cfg.DB.Port)
	}
	if cfg.DB.User != "postgres" {
		t.Errorf("expected DB.User postgres, got %s", cfg.DB.User)
	}
	if cfg.DB.Name != "pumpkin_pro" {
		t.Errorf("expected DB.Name pumpkin_pro, got %s", cfg.DB.Name)
	}
	if cfg.DB.SSLMode != "disable" {
		t.Errorf("expected DB.SSLMode disable, got %s", cfg.DB.SSLMode)
	}

	// Auth defaults
	if cfg.Auth.JWTSecret != "dev-only-change-me" {
		t.Errorf("expected Auth.JWTSecret dev-only-change-me, got %s", cfg.Auth.JWTSecret)
	}
	if cfg.Auth.AccessTokenTTLMinutes != 1440 {
		t.Errorf("expected Auth.AccessTokenTTLMinutes 1440, got %d", cfg.Auth.AccessTokenTTLMinutes)
	}
	if cfg.Auth.RefreshTokenTTLHours != 168 {
		t.Errorf("expected Auth.RefreshTokenTTLHours 168, got %d", cfg.Auth.RefreshTokenTTLHours)
	}

	// AI defaults
	if cfg.AI.Model != "gpt-4o-mini" {
		t.Errorf("expected AI.Model gpt-4o-mini, got %s", cfg.AI.Model)
	}
	if cfg.AI.CipherKey != "" {
		t.Errorf("expected empty AI.CipherKey by default, got %q", cfg.AI.CipherKey)
	}
}

func TestEnvOverride(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		validate func(Config) bool
	}{
		{"PORT", "9090", func(c Config) bool { return c.Port == "9090" }},
		{"QUANT_SERVICE_URL", "http://quant:9000/", func(c Config) bool { return c.QuantServiceURL == "http://quant:9000" }},
		{"DB_TYPE", "postgres", func(c Config) bool { return c.DB.Type == "postgres" }},
		{"DB_PORT", "5433", func(c Config) bool { return c.DB.Port == 5433 }},
		{"AUTH_JWT_SECRET", "my-super-secret", func(c Config) bool { return c.Auth.JWTSecret == "my-super-secret" }},
		{"AI_MODEL", "gpt-4o", func(c Config) bool { return c.AI.Model == "gpt-4o" }},
		{"AI_CONFIG_CIPHER_KEY", "12345678901234567890123456789012", func(c Config) bool { return c.AI.CipherKey == "12345678901234567890123456789012" }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			os.Setenv(tt.key, tt.value)
			defer os.Unsetenv(tt.key)
			cfg := Load()
			if !tt.validate(cfg) {
				t.Errorf("env var %s=%s not applied correctly", tt.key, tt.value)
			}
		})
	}
}

func TestTrimTrailingSlash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://localhost:8000/", "http://localhost:8000"},
		{"http://localhost:8000//", "http://localhost:8000"},
		{"http://localhost:8000", "http://localhost:8000"},
		{"/api/callback/", "/api/callback"},
		{"", ""},
		{"/", ""},
		{"no-slash", "no-slash"},
	}

	for _, tt := range tests {
		result := trimTrailingSlash(tt.input)
		if result != tt.expected {
			t.Errorf("trimTrailingSlash(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetEnvAsInt(t *testing.T) {
	// Valid integer
	os.Setenv("TEST_INT_VAL", "42")
	defer os.Unsetenv("TEST_INT_VAL")

	result := getEnvAsInt("TEST_INT_VAL", 10)
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}

	// Fallback to default when empty
	result = getEnvAsInt("NONEXISTENT_INT", 99)
	if result != 99 {
		t.Errorf("expected fallback 99, got %d", result)
	}

	// Invalid value falls back to default
	os.Setenv("BAD_INT", "abc")
	defer os.Unsetenv("BAD_INT")
	result = getEnvAsInt("BAD_INT", 7)
	if result != 7 {
		t.Errorf("expected fallback 7 for invalid int, got %d", result)
	}
}
