package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port               string
	QuantServiceURL    string
	BackendCallbackURL string
	DB                 DBConfig
	StrategySeedPath   string
	Auth               AuthConfig
	AdminSeed          AdminSeedConfig
	AI                 AIConfig
	Backup             BackupConfig
	FactorLab          FactorLabConfig
}

type FactorLabConfig struct {
	DailyComputeEnabled bool
	ComputeHour         int
	ComputeMinute       int
	PythonBin           string
	Phase1ScriptPath    string
	ProgressInterval    int
	TimeoutMinutes      int
}

// BackupConfig holds database backup settings.
type BackupConfig struct {
	DBPath          string // path to pumpkin.db for hot backup
	BackupDir       string // local backup output directory
	CacheADir       string // directory with quadrant_cache.db
	CacheHKDir      string // directory with quadrant_cache_hk.db
	RetentionDays   int    // local retention days (default 7)
	CooldownMinutes int    // minimum minutes between backups (default 120)
	COSBucket       string // Tencent Cloud COS bucket name
	COSRegion       string // COS region, e.g. ap-guangzhou
	COSPrefix       string // COS object key prefix
	COSSecretID     string // CAM SecretId (from env)
	COSSecretKey    string // CAM SecretKey (from env)
}

type AIConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	CipherKey string
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
		Port:               getEnv("PORT", "8080"),
		QuantServiceURL:    trimTrailingSlash(getEnv("QUANT_SERVICE_URL", "http://localhost:8000")),
		BackendCallbackURL: trimTrailingSlash(getEnv("BACKEND_CALLBACK_URL", fmt.Sprintf("http://localhost:%s", getEnv("PORT", "8080")))),
		StrategySeedPath:   getEnv("STRATEGY_SEED_PATH", "seed/strategies.json"),
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
			AccessTokenTTLMinutes: getEnvAsInt("AUTH_ACCESS_TOKEN_TTL_MINUTES", 1440),
			RefreshTokenTTLHours:  getEnvAsInt("AUTH_REFRESH_TOKEN_TTL_HOURS", 168),
		},
		AI: AIConfig{
			APIKey:    getEnv("AI_API_KEY", ""),
			BaseURL:   trimTrailingSlash(getEnv("AI_BASE_URL", "https://api.openai.com/v1")),
			Model:     getEnv("AI_MODEL", "gpt-4o-mini"),
			CipherKey: getEnv("AI_CONFIG_CIPHER_KEY", ""),
		},
		Backup: BackupConfig{
			DBPath:          getEnv("DB_PATH", "data/pumpkin.db"),
			BackupDir:       getEnv("BACKUP_DIR", "data/backups"),
			CacheADir:       getEnv("CACHE_A_DIR", "data/quant"),
			CacheHKDir:      getEnv("CACHE_HK_DIR", "data/quant"),
			RetentionDays:   getEnvAsInt("BACKUP_RETENTION_DAYS", 7),
			CooldownMinutes: getEnvAsInt("BACKUP_COOLDOWN_MINUTES", 120),
			COSBucket:       getEnv("COS_BUCKET", ""),
			COSRegion:       getEnv("COS_REGION", ""),
			COSPrefix:       getEnv("COS_BACKUP_PREFIX", "pumpkin-pro-backups/"),
			COSSecretID:     getEnv("COS_SECRET_ID", ""),
			COSSecretKey:    getEnv("COS_SECRET_KEY", ""),
		},
		FactorLab: FactorLabConfig{
			DailyComputeEnabled: getEnvAsBool("FACTOR_LAB_DAILY_COMPUTE_ENABLED", true),
			ComputeHour:         getEnvAsInt("FACTOR_LAB_COMPUTE_HOUR", 20),
			ComputeMinute:       getEnvAsInt("FACTOR_LAB_COMPUTE_MINUTE", 30),
			PythonBin:           getEnv("FACTOR_LAB_PYTHON_BIN", "python3"),
			Phase1ScriptPath:    getEnv("FACTOR_LAB_PHASE1_SCRIPT", "quant/scripts/compute_factor_lab_phase1.py"),
			ProgressInterval:    getEnvAsInt("FACTOR_LAB_PROGRESS_INTERVAL", 500),
			TimeoutMinutes:      getEnvAsInt("FACTOR_LAB_TIMEOUT_MINUTES", 60),
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

func getEnvAsBool(key string, defaultValue bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return defaultValue
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func trimTrailingSlash(raw string) string {
	return strings.TrimRight(raw, "/")
}
