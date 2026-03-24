package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port             string
	QuantServiceURL  string
	DB               DBConfig
	StrategySeedPath string
	Auth             AuthConfig
	AdminSeed        AdminSeedConfig
	AI               AIConfig
}

type AIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type AdminSeedConfig struct {
	Email    string
	Password string
}

type DBConfig struct {
	Type     string
	Path     string
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

type AuthConfig struct {
	JWTSecret             string
	AccessTokenTTLMinutes int
	RefreshTokenTTLHours  int
}

func Load() Config {
	return Config{
		Port:             getEnv("PORT", "8080"),
		QuantServiceURL:  trimTrailingSlash(getEnv("QUANT_SERVICE_URL", "http://localhost:8000")),
		StrategySeedPath: getEnv("STRATEGY_SEED_PATH", "seed/strategies.json"),
		AdminSeed: AdminSeedConfig{
			Email:    getEnv("ADMIN_SEED_EMAIL", ""),
			Password: getEnv("ADMIN_SEED_PASSWORD", ""),
		},
		DB: DBConfig{
			Type:     strings.ToLower(getEnv("DB_TYPE", "sqlite")),
			Path:     getEnv("DB_PATH", "data/pumpkin.db"),
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Name:     getEnv("DB_NAME", "pumpkin_pro"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Auth: AuthConfig{
			JWTSecret:             getEnv("AUTH_JWT_SECRET", "dev-only-change-me"),
			AccessTokenTTLMinutes: getEnvAsInt("AUTH_ACCESS_TOKEN_TTL_MINUTES", 120),
			RefreshTokenTTLHours:  getEnvAsInt("AUTH_REFRESH_TOKEN_TTL_HOURS", 168),
		},
		AI: AIConfig{
			APIKey:  getEnv("AI_API_KEY", ""),
			BaseURL: trimTrailingSlash(getEnv("AI_BASE_URL", "https://api.openai.com/v1")),
			Model:   getEnv("AI_MODEL", "gpt-4o-mini"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvAsInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func trimTrailingSlash(raw string) string {
	return strings.TrimRight(raw, "/")
}
