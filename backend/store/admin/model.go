package admin

import "time"

type SuperAdminRecord struct {
	ID           string    `gorm:"primaryKey;size:36"`
	Email        string    `gorm:"size:128;not null;uniqueIndex"`
	PasswordHash string    `gorm:"size:255;not null"`
	Nickname     string    `gorm:"size:64;not null;default:''"`
	Status       string    `gorm:"size:20;not null;default:'active'"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (SuperAdminRecord) TableName() string {
	return "super_admins"
}

type AdminProfile struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

type AdminLoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AdminLoginResult struct {
	Admin  AdminProfile `json:"admin"`
	Tokens AdminTokens  `json:"tokens"`
}

type AdminTokens struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type AdminAccessClaims struct {
	AdminID   string `json:"aid"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// ── Stats response types ──

type StatsResult struct {
	Users       UserStats       `json:"users"`
	Strategies  StrategyStats   `json:"strategies"`
	Live        LiveStats       `json:"live"`
	Signals     SignalStats     `json:"signals"`
	Audit       AuditStats      `json:"audit"`
	Features    FeatureStats    `json:"features"`
	Trends      TrendStats      `json:"trends"`
	Retention   RetentionStats  `json:"retention"`
	Traffic     TrafficStats    `json:"traffic"`
	AI          AIStats         `json:"ai"`
	GeneratedAt string          `json:"generated_at"`
}

type UserStats struct {
	Total          int64 `json:"total"`
	Today          int64 `json:"today"`
	Last7D         int64 `json:"last_7d"`
	Last30D        int64 `json:"last_30d"`
	Active7D       int64 `json:"active_7d"`
	ActiveSessions int64 `json:"active_sessions"`
}

type StrategyStats struct {
	Total       int64 `json:"total"`
	System      int64 `json:"system"`
	UserCreated int64 `json:"user_created"`
	Active      int64 `json:"active"`
	Referenced  int64 `json:"referenced"`
}

type LiveStats struct {
	WatchlistItems     int64 `json:"watchlist_items"`
	UsersWithWatchlist int64 `json:"users_with_watchlist"`
	ActiveSymbols      int64 `json:"active_symbols"`
}

type SignalStats struct {
	WebhookUsers        int64   `json:"webhook_users"`
	WebhookEnabledRate  float64 `json:"webhook_enabled_rate"`
	SignalConfigs       int64   `json:"signal_configs"`
	SignalConfigsEnabled int64  `json:"signal_configs_enabled"`
	TotalEvents         int64   `json:"total_events"`
	TodayEvents         int64   `json:"today_events"`
	DeliverySuccessRate float64 `json:"delivery_success_rate"`
	TodayDeliveries     int64   `json:"today_deliveries"`
}

type AuditStats struct {
	TodayLogins        int64 `json:"today_logins"`
	TodayRegistrations int64 `json:"today_registrations"`
	FailedLogins7D     int64 `json:"failed_logins_7d"`
}

type FeatureStats struct {
	BacktestTotal    int64 `json:"backtest_total"`
	BacktestToday    int64 `json:"backtest_today"`
	BacktestUsers    int64 `json:"backtest_users"`
	PortfolioRecords int64 `json:"portfolio_records"`
	PortfolioUsers   int64 `json:"portfolio_users"`
	ScreenerLists    int64 `json:"screener_lists"`
	ScreenerUsers    int64 `json:"screener_users"`
}

type TrendStats struct {
	DailyRegistrations []DailyCount `json:"daily_registrations"`
	DailyActiveUsers   []DailyCount `json:"daily_active_users"`
	DailySignalEvents  []DailyCount `json:"daily_signal_events"`
}

type RetentionStats struct {
	Day7Rate  float64 `json:"day_7_rate"`
	Day30Rate float64 `json:"day_30_rate"`
}

type TrafficStats struct {
	UTMSources []SourceCount `json:"utm_sources"`
	Referrers  []SourceCount `json:"referrers"`
}

// ── AI 调用统计 ──

type AIStats struct {
	TotalCalls    int64          `json:"total_calls"`
	TodayCalls    int64          `json:"today_calls"`
	Last7DCalls   int64          `json:"last_7d_calls"`
	SuccessRate   float64        `json:"success_rate"`
	AvgResponseMS float64        `json:"avg_response_ms"`
	UniqueUsers   int64          `json:"unique_users"`
	ByFeature     []FeatureCount `json:"by_feature"`
	DailyTrend    []DailyCount   `json:"daily_trend"`
	TopUsers      []TopAIUser    `json:"top_users"`
}

type FeatureCount struct {
	FeatureKey  string `json:"feature_key"`
	FeatureName string `json:"feature_name"`
	Count       int64  `json:"count"`
}

type TopAIUser struct {
	UserID       string `json:"user_id"`
	CallCount    int64  `json:"call_count"`
	LastCalledAt string `json:"last_called_at"`
}

// ── User Funnel Stats ──

// FunnelStep represents one layer of the user conversion funnel.
type FunnelStep struct {
	Label      string `json:"label"`               // e.g. "注册", "登录"
	CountAll   int64  `json:"count_all"`           // total (all time)
	CountToday int64  `json:"count_today"`         // today
	Count7D    int64  `json:"count_7d"`            // last 7 days
	Count30D   int64  `json:"count_30d"`           // last 30 days
}

// FunnelStats is the full funnel response.
type FunnelStats struct {
	Steps       []FunnelStep `json:"steps"`                 // ordered 7 steps
	GeneratedAt string       `json:"generated_at"`
}
