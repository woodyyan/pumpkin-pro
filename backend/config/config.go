package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                     string
	AppPublicBaseURL         string
	QuantServiceURL          string
	BackendCallbackURL       string
	DB                       DBConfig
	StrategySeedPath         string
	Auth                     AuthConfig
	Mail                     MailConfig
	PasswordReset            PasswordResetConfig
	AdminSeed                AdminSeedConfig
	AI                       AIConfig
	Stripe                   StripeConfig
	Backup                   BackupConfig
	Quadrant                 QuadrantConfig
	FactorLab                FactorLabConfig
	FactorIndex              FactorIndexConfig
	PortfolioSnapshot        PortfolioSnapshotConfig
	RankingPortfolioRealtime RankingPortfolioRealtimeConfig
}

type PasswordResetConfig struct {
	TTLMinutes             int
	RateLimitPerIP         int
	RateLimitPerEmail      int
	RateLimitWindowMinutes int
	EmailCooldownSeconds   int
}

type MailConfig struct {
	Provider             string
	FromEmail            string
	FromName             string
	TencentSecretID      string
	TencentSecretKey     string
	TencentToken         string
	TencentRegion        string
	TencentEndpoint      string
	TencentAPIVersion    string
	TencentAPIAction     string
	TencentLanguage      string
	TencentTemplateID    int
	MockLogBodies        bool
	MockFailDelivery     bool
	MockCaptureRecipient string
}

type QuadrantConfig struct {
	DailyComputeEnabled bool
	ComputeHour         int
	ComputeMinute       int
	RunOnNonTradingDay  bool
}

type FactorLabConfig struct {
	DailyComputeEnabled  bool
	ComputeHour          int
	ComputeMinute        int
	DailyModes           []string
	PythonBin            string
	Phase0ScriptPath     string
	Phase1ScriptPath     string
	Phase2ScriptPath     string
	IndustriesSource     string
	DailyBarsSource      string
	FinancialsSource     string
	DividendsSource      string
	ProgressInterval     int
	ItemProgressInterval int
	TimeoutMinutes       int
	StepTimeoutMinutes   int
}

type FactorIndexConfig struct {
	DailyComputeEnabled bool
	RebalanceHour       int
	RebalanceMinute     int
	DailyHour           int
	DailyMinute         int
	TimeoutMinutes      int
}

type PortfolioSnapshotConfig struct {
	DailyComputeEnabled bool
	AShareHour          int
	AShareMinute        int
	HKHour              int
	HKMinute            int
	TimeoutMinutes      int
}

// RankingPortfolioRealtimeConfig configures the intraday realtime-price refresh
// for the ranking simulated portfolio. All time points are Beijing time.
type RankingPortfolioRealtimeConfig struct {
	Enabled      bool
	ASharePoints []string // "HH:MM" Beijing time
	HKPoints     []string // "HH:MM" Beijing time
	// CutoverDate is the go-live date (Beijing time, "YYYY-MM-DD") of the
	// open-entry pricing change. The admin repair job will never backfill
	// dates strictly before this date. Defaults to "2026-06-10" (first live day).
	CutoverDate string
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

type StripeConfig struct {
	Mode                  string
	SecretKey             string
	WebhookSecret         string
	DefaultCurrency       string
	AllowedPaymentMethods []string
	SuccessPath           string
	CancelPath            string
	CheckoutExpireMinutes int
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
		AppPublicBaseURL:   trimTrailingSlash(getEnv("APP_PUBLIC_BASE_URL", "https://wolongtrader.top")),
		QuantServiceURL:    trimTrailingSlash(getEnv("QUANT_SERVICE_URL", "http://localhost:8000")),
		BackendCallbackURL: trimTrailingSlash(getEnv("BACKEND_CALLBACK_URL", fmt.Sprintf("http://localhost:%s", getEnv("PORT", "8080")))),
		StrategySeedPath:   getEnv("STRATEGY_SEED_PATH", "seed/strategies.json"),
		Mail: MailConfig{
			Provider:             strings.ToLower(getEnv("MAIL_PROVIDER", "mock")),
			FromEmail:            getEnv("MAIL_FROM_EMAIL", "no-reply@wolongtrader.top"),
			FromName:             getEnv("MAIL_FROM_NAME", "卧龙 Trader"),
			TencentSecretID:      getEnv("MAIL_TENCENT_SECRET_ID", ""),
			TencentSecretKey:     getEnv("MAIL_TENCENT_SECRET_KEY", ""),
			TencentToken:         getEnv("MAIL_TENCENT_TOKEN", ""),
			TencentRegion:        getEnv("MAIL_TENCENT_REGION", "ap-hongkong"),
			TencentEndpoint:      trimTrailingSlash(getEnv("MAIL_TENCENT_ENDPOINT", "https://ses.tencentcloudapi.com")),
			TencentAPIVersion:    getEnv("MAIL_TENCENT_API_VERSION", "2020-10-02"),
			TencentAPIAction:     getEnv("MAIL_TENCENT_API_ACTION", "SendEmail"),
			TencentLanguage:      getEnv("MAIL_TENCENT_LANGUAGE", "zh-CN"),
			TencentTemplateID:    getEnvAsInt("MAIL_TENCENT_TEMPLATE_ID", 0),
			MockLogBodies:        getEnvAsBool("MAIL_MOCK_LOG_BODIES", true),
			MockFailDelivery:     getEnvAsBool("MAIL_MOCK_FAIL_DELIVERY", false),
			MockCaptureRecipient: getEnv("MAIL_MOCK_CAPTURE_RECIPIENT", "dev-null@local.invalid"),
		},
		PasswordReset: PasswordResetConfig{
			TTLMinutes:             getEnvAsInt("PASSWORD_RESET_TTL_MINUTES", 30),
			RateLimitPerIP:         getEnvAsInt("PASSWORD_RESET_RATE_LIMIT_PER_IP", 10),
			RateLimitPerEmail:      getEnvAsInt("PASSWORD_RESET_RATE_LIMIT_PER_EMAIL", 3),
			RateLimitWindowMinutes: getEnvAsInt("PASSWORD_RESET_RATE_LIMIT_WINDOW_MINUTES", 60),
			EmailCooldownSeconds:   getEnvAsInt("PASSWORD_RESET_EMAIL_COOLDOWN_SECONDS", 60),
		},
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
		Stripe: StripeConfig{
			Mode:                  strings.ToLower(getEnv("STRIPE_MODE", "test")),
			SecretKey:             getEnv("STRIPE_SECRET_KEY", ""),
			WebhookSecret:         getEnv("STRIPE_WEBHOOK_SECRET", ""),
			DefaultCurrency:       strings.ToLower(getEnv("STRIPE_DEFAULT_CURRENCY", "cny")),
			AllowedPaymentMethods: parseCommaSeparated(getEnv("STRIPE_ALLOWED_PAYMENT_METHODS", "card")),
			SuccessPath:           getEnv("STRIPE_CHECKOUT_SUCCESS_PATH", "/admin?tab=payments&checkout=success"),
			CancelPath:            getEnv("STRIPE_CHECKOUT_CANCEL_PATH", "/admin?tab=payments&checkout=cancel"),
			CheckoutExpireMinutes: getEnvAsInt("STRIPE_CHECKOUT_EXPIRE_MINUTES", 60),
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
		Quadrant: QuadrantConfig{
			DailyComputeEnabled: getEnvAsBool("QUADRANT_DAILY_COMPUTE_ENABLED", true),
			ComputeHour:         getEnvAsInt("QUADRANT_COMPUTE_HOUR", 20),
			ComputeMinute:       getEnvAsInt("QUADRANT_COMPUTE_MINUTE", 0),
			RunOnNonTradingDay:  getEnvAsBool("QUADRANT_RUN_ON_NON_TRADING_DAY", false),
		},
		FactorLab: FactorLabConfig{
			DailyComputeEnabled:  getEnvAsBool("FACTOR_LAB_DAILY_COMPUTE_ENABLED", true),
			ComputeHour:          getEnvAsInt("FACTOR_LAB_COMPUTE_HOUR", 21),
			ComputeMinute:        getEnvAsInt("FACTOR_LAB_COMPUTE_MINUTE", 0),
			DailyModes:           parseCSVEnv(getEnv("FACTOR_LAB_DAILY_MODES", "securities,industries,daily-bars,index-bars,financials")),
			PythonBin:            getEnv("FACTOR_LAB_PYTHON_BIN", "python3"),
			Phase0ScriptPath:     getEnv("FACTOR_LAB_PHASE0_SCRIPT", "quant/scripts/update_factor_lab_phase0_incremental.py"),
			Phase1ScriptPath:     getEnv("FACTOR_LAB_PHASE1_SCRIPT", "quant/scripts/compute_factor_lab_phase1.py"),
			Phase2ScriptPath:     getEnv("FACTOR_LAB_PHASE2_SCRIPT", "quant/scripts/compute_factor_lab_phase2.py"),
			IndustriesSource:     getEnv("FACTOR_LAB_INDUSTRIES_SOURCE", "auto"),
			DailyBarsSource:      getEnv("FACTOR_LAB_DAILY_BARS_SOURCE", "auto"),
			FinancialsSource:     getEnv("FACTOR_LAB_FINANCIALS_SOURCE", "auto"),
			DividendsSource:      getEnv("FACTOR_LAB_DIVIDENDS_SOURCE", "auto"),
			ProgressInterval:     getEnvAsInt("FACTOR_LAB_PROGRESS_INTERVAL", 500),
			ItemProgressInterval: getEnvAsInt("FACTOR_LAB_ITEM_PROGRESS_INTERVAL", 1),
			TimeoutMinutes:       getEnvAsInt("FACTOR_LAB_TIMEOUT_MINUTES", 180),
			StepTimeoutMinutes:   getEnvAsInt("FACTOR_LAB_STEP_TIMEOUT_MINUTES", 30),
		},
		FactorIndex: FactorIndexConfig{
			DailyComputeEnabled: getEnvAsBool("FACTOR_INDEX_DAILY_COMPUTE_ENABLED", true),
			RebalanceHour:       getEnvAsInt("FACTOR_INDEX_REBALANCE_HOUR", 20),
			RebalanceMinute:     getEnvAsInt("FACTOR_INDEX_REBALANCE_MINUTE", 10),
			DailyHour:           getEnvAsInt("FACTOR_INDEX_DAILY_HOUR", 20),
			DailyMinute:         getEnvAsInt("FACTOR_INDEX_DAILY_MINUTE", 30),
			TimeoutMinutes:      getEnvAsInt("FACTOR_INDEX_TIMEOUT_MINUTES", 120),
		},
		PortfolioSnapshot: PortfolioSnapshotConfig{
			DailyComputeEnabled: getEnvAsBool("PORTFOLIO_SNAPSHOT_DAILY_COMPUTE_ENABLED", true),
			AShareHour:          getEnvAsInt("PORTFOLIO_SNAPSHOT_ASHARE_HOUR", 16),
			AShareMinute:        getEnvAsInt("PORTFOLIO_SNAPSHOT_ASHARE_MINUTE", 0),
			HKHour:              getEnvAsInt("PORTFOLIO_SNAPSHOT_HK_HOUR", 17),
			HKMinute:            getEnvAsInt("PORTFOLIO_SNAPSHOT_HK_MINUTE", 0),
			TimeoutMinutes:      getEnvAsInt("PORTFOLIO_SNAPSHOT_TIMEOUT_MINUTES", 120),
		},
		RankingPortfolioRealtime: RankingPortfolioRealtimeConfig{
			Enabled: getEnvAsBool("RANKING_PORTFOLIO_REALTIME_ENABLED", true),
			// Empty point slices fall back to built-in A-share/HK defaults.
			ASharePoints: nil,
			HKPoints:     nil,
			CutoverDate:  getEnv("RANKING_PORTFOLIO_CUTOVER_DATE", "2026-06-10"),
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

func parseCommaSeparated(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func parseCSVEnv(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
