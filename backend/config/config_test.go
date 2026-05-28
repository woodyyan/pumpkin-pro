package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"PORT", "APP_PUBLIC_BASE_URL", "QUANT_SERVICE_URL", "BACKEND_CALLBACK_URL",
		"STRATEGY_SEED_PATH",
		"DB_TYPE", "DB_PATH", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
		"ADMIN_SEED_EMAIL", "ADMIN_SEED_PASSWORD",
		"AUTH_JWT_SECRET", "AUTH_ACCESS_TOKEN_TTL_MINUTES", "AUTH_REFRESH_TOKEN_TTL_HOURS",
		"MAIL_PROVIDER", "MAIL_FROM_EMAIL", "MAIL_FROM_NAME", "MAIL_TENCENT_SECRET_ID", "MAIL_TENCENT_SECRET_KEY",
		"MAIL_TENCENT_TOKEN", "MAIL_TENCENT_REGION", "MAIL_TENCENT_ENDPOINT", "MAIL_TENCENT_API_VERSION", "MAIL_TENCENT_API_ACTION",
		"MAIL_TENCENT_LANGUAGE", "MAIL_TENCENT_TEMPLATE_ID",
		"MAIL_MOCK_LOG_BODIES", "MAIL_MOCK_FAIL_DELIVERY", "MAIL_MOCK_CAPTURE_RECIPIENT",
		"PASSWORD_RESET_TTL_MINUTES", "PASSWORD_RESET_RATE_LIMIT_PER_IP", "PASSWORD_RESET_RATE_LIMIT_PER_EMAIL",
		"PASSWORD_RESET_RATE_LIMIT_WINDOW_MINUTES", "PASSWORD_RESET_EMAIL_COOLDOWN_SECONDS",
		"AI_API_KEY", "AI_BASE_URL", "AI_MODEL", "AI_CONFIG_CIPHER_KEY",
		"FACTOR_LAB_DAILY_COMPUTE_ENABLED", "FACTOR_LAB_COMPUTE_HOUR", "FACTOR_LAB_COMPUTE_MINUTE",
		"FACTOR_LAB_PYTHON_BIN", "FACTOR_LAB_PHASE0_SCRIPT", "FACTOR_LAB_PHASE1_SCRIPT", "FACTOR_LAB_PHASE2_SCRIPT",
		"FACTOR_LAB_DAILY_BARS_SOURCE", "FACTOR_LAB_FINANCIALS_SOURCE", "FACTOR_LAB_DIVIDENDS_SOURCE",
		"FACTOR_LAB_PROGRESS_INTERVAL", "FACTOR_LAB_ITEM_PROGRESS_INTERVAL", "FACTOR_LAB_TIMEOUT_MINUTES", "FACTOR_LAB_STEP_TIMEOUT_MINUTES",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default Port 8080, got %s", cfg.Port)
	}
	if cfg.AppPublicBaseURL != "https://wolongtrader.top" {
		t.Errorf("expected AppPublicBaseURL https://wolongtrader.top, got %s", cfg.AppPublicBaseURL)
	}
	if cfg.QuantServiceURL != "http://localhost:8000" {
		t.Errorf("expected QuantServiceURL http://localhost:8000, got %s", cfg.QuantServiceURL)
	}
	if cfg.BackendCallbackURL != "http://localhost:8080" {
		t.Errorf("expected BackendCallbackURL http://localhost:8080, got %s", cfg.BackendCallbackURL)
	}
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
	if cfg.Auth.JWTSecret != "dev-only-change-me" {
		t.Errorf("expected Auth.JWTSecret dev-only-change-me, got %s", cfg.Auth.JWTSecret)
	}
	if cfg.Auth.AccessTokenTTLMinutes != 1440 {
		t.Errorf("expected Auth.AccessTokenTTLMinutes 1440, got %d", cfg.Auth.AccessTokenTTLMinutes)
	}
	if cfg.Auth.RefreshTokenTTLHours != 168 {
		t.Errorf("expected Auth.RefreshTokenTTLHours 168, got %d", cfg.Auth.RefreshTokenTTLHours)
	}
	if cfg.Mail.Provider != "mock" {
		t.Errorf("expected Mail.Provider mock, got %s", cfg.Mail.Provider)
	}
	if cfg.Mail.FromEmail != "no-reply@wolongtrader.top" {
		t.Errorf("expected Mail.FromEmail no-reply@wolongtrader.top, got %s", cfg.Mail.FromEmail)
	}
	if cfg.Mail.TencentRegion != "ap-hongkong" {
		t.Errorf("expected Mail.TencentRegion ap-hongkong, got %s", cfg.Mail.TencentRegion)
	}
	if cfg.Mail.TencentLanguage != "zh-CN" {
		t.Errorf("expected Mail.TencentLanguage zh-CN, got %s", cfg.Mail.TencentLanguage)
	}
	if cfg.PasswordReset.TTLMinutes != 30 {
		t.Errorf("expected PasswordReset.TTLMinutes 30, got %d", cfg.PasswordReset.TTLMinutes)
	}
	if cfg.PasswordReset.RateLimitPerIP != 10 {
		t.Errorf("expected PasswordReset.RateLimitPerIP 10, got %d", cfg.PasswordReset.RateLimitPerIP)
	}
	if cfg.PasswordReset.RateLimitPerEmail != 3 {
		t.Errorf("expected PasswordReset.RateLimitPerEmail 3, got %d", cfg.PasswordReset.RateLimitPerEmail)
	}
	if cfg.AI.Model != "gpt-4o-mini" {
		t.Errorf("expected AI.Model gpt-4o-mini, got %s", cfg.AI.Model)
	}
	if cfg.AI.CipherKey != "" {
		t.Errorf("expected empty AI.CipherKey by default, got %q", cfg.AI.CipherKey)
	}
	if !cfg.FactorLab.DailyComputeEnabled {
		t.Errorf("expected FactorLab.DailyComputeEnabled true by default")
	}
	if cfg.FactorLab.ComputeHour != 21 || cfg.FactorLab.ComputeMinute != 0 {
		t.Errorf("expected FactorLab schedule 21:00, got %02d:%02d", cfg.FactorLab.ComputeHour, cfg.FactorLab.ComputeMinute)
	}
	if cfg.FactorLab.Phase0ScriptPath != "quant/scripts/update_factor_lab_phase0_incremental.py" {
		t.Errorf("unexpected FactorLab phase0 script: %s", cfg.FactorLab.Phase0ScriptPath)
	}
	if cfg.FactorLab.ProgressInterval != 500 || cfg.FactorLab.ItemProgressInterval != 1 {
		t.Errorf("unexpected FactorLab progress defaults: %d/%d", cfg.FactorLab.ProgressInterval, cfg.FactorLab.ItemProgressInterval)
	}
}

func TestEnvOverride(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		validate func(Config) bool
	}{
		{"PORT", "9090", func(c Config) bool { return c.Port == "9090" }},
		{"APP_PUBLIC_BASE_URL", "https://example.com/", func(c Config) bool { return c.AppPublicBaseURL == "https://example.com" }},
		{"QUANT_SERVICE_URL", "http://quant:9000/", func(c Config) bool { return c.QuantServiceURL == "http://quant:9000" }},
		{"DB_TYPE", "postgres", func(c Config) bool { return c.DB.Type == "postgres" }},
		{"DB_PORT", "5433", func(c Config) bool { return c.DB.Port == 5433 }},
		{"AUTH_JWT_SECRET", "my-super-secret", func(c Config) bool { return c.Auth.JWTSecret == "my-super-secret" }},
		{"AI_MODEL", "gpt-4o", func(c Config) bool { return c.AI.Model == "gpt-4o" }},
		{"MAIL_PROVIDER", "tencent", func(c Config) bool { return c.Mail.Provider == "tencent" }},
		{"MAIL_TENCENT_TEMPLATE_ID", "179710", func(c Config) bool { return c.Mail.TencentTemplateID == 179710 }},
		{"PASSWORD_RESET_RATE_LIMIT_PER_IP", "12", func(c Config) bool { return c.PasswordReset.RateLimitPerIP == 12 }},
		{"AI_CONFIG_CIPHER_KEY", "12345678901234567890123456789012", func(c Config) bool { return c.AI.CipherKey == "12345678901234567890123456789012" }},
		{"FACTOR_LAB_COMPUTE_HOUR", "21", func(c Config) bool { return c.FactorLab.ComputeHour == 21 }},
		{"FACTOR_LAB_COMPUTE_MINUTE", "0", func(c Config) bool { return c.FactorLab.ComputeMinute == 0 }},
		{"FACTOR_LAB_PHASE0_SCRIPT", "/app/quant/scripts/update_factor_lab_phase0_incremental.py", func(c Config) bool { return c.FactorLab.Phase0ScriptPath == "/app/quant/scripts/update_factor_lab_phase0_incremental.py" }},
		{"FACTOR_LAB_ITEM_PROGRESS_INTERVAL", "2", func(c Config) bool { return c.FactorLab.ItemProgressInterval == 2 }},
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
	os.Setenv("TEST_INT_VAL", "42")
	defer os.Unsetenv("TEST_INT_VAL")

	result := getEnvAsInt("TEST_INT_VAL", 10)
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}

	result = getEnvAsInt("NONEXISTENT_INT", 99)
	if result != 99 {
		t.Errorf("expected fallback 99, got %d", result)
	}

	os.Setenv("BAD_INT", "abc")
	defer os.Unsetenv("BAD_INT")
	result = getEnvAsInt("BAD_INT", 7)
	if result != 7 {
		t.Errorf("expected fallback 7 for invalid int, got %d", result)
	}
}
