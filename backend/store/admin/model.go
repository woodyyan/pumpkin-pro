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
	Users       UserStats      `json:"users"`
	Strategies  StrategyStats  `json:"strategies"`
	Live        LiveStats      `json:"live"`
	Signals     SignalStats    `json:"signals"`
	Audit       AuditStats     `json:"audit"`
	Features    FeatureStats   `json:"features"`
	Trends      TrendStats     `json:"trends"`
	Retention   RetentionStats `json:"retention"`
	Traffic     TrafficStats   `json:"traffic"`
	AI          AIStats        `json:"ai"`
	GeneratedAt string         `json:"generated_at"`
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
}

type SignalStats struct {
	WebhookUsers         int64   `json:"webhook_users"`
	WebhookEnabledRate   float64 `json:"webhook_enabled_rate"`
	SignalConfigs        int64   `json:"signal_configs"`
	SignalConfigsEnabled int64   `json:"signal_configs_enabled"`
	TotalEvents          int64   `json:"total_events"`
	TodayEvents          int64   `json:"today_events"`
	DeliverySuccessRate  float64 `json:"delivery_success_rate"`
	TodayDeliveries      int64   `json:"today_deliveries"`
}

type AuditStats struct {
	TodayLogins        int64 `json:"today_logins"`
	TodayRegistrations int64 `json:"today_registrations"`
	FailedLogins7D     int64 `json:"failed_logins_7d"`
}

type FeatureStats struct {
	BacktestTotal            int64 `json:"backtest_total"`
	BacktestToday            int64 `json:"backtest_today"`
	BacktestUsers            int64 `json:"backtest_users"`
	PortfolioRecords         int64 `json:"portfolio_records"`
	PortfolioUsers           int64 `json:"portfolio_users"`
	PortfolioActivePositions int64 `json:"portfolio_active_positions"`
	PortfolioActiveUsers     int64 `json:"portfolio_active_users"`
	PortfolioEventTotal      int64 `json:"portfolio_event_total"`
	PortfolioEventToday      int64 `json:"portfolio_event_today"`
	PortfolioEventUsers7D    int64 `json:"portfolio_event_users_7d"`
	PortfolioProfileUsers    int64 `json:"portfolio_profile_users"`
	ScreenerLists            int64 `json:"screener_lists"`
	ScreenerUsers            int64 `json:"screener_users"`
}

type TrendStats struct {
	DailyRegistrations []DailyCount `json:"daily_registrations"`
	DailyActiveUsers   []DailyCount `json:"daily_active_users"`
	DailySignalEvents  []DailyCount `json:"daily_signal_events"`
	DailyPortfolioOps  []DailyCount `json:"daily_portfolio_ops"`
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
	TotalCalls       int64               `json:"total_calls"`
	TodayCalls       int64               `json:"today_calls"`
	Last7DCalls      int64               `json:"last_7d_calls"`
	SuccessRate      float64             `json:"success_rate"`
	AvgResponseMS    float64             `json:"avg_response_ms"`
	UniqueUsers      int64               `json:"unique_users"`
	TotalTokens      int64               `json:"total_tokens"`
	TodayTokens      int64               `json:"today_tokens"`
	Last7DTokens     int64               `json:"last_7d_tokens"`
	AvgTokensPerCall float64             `json:"avg_tokens_per_call"`
	ByFeature        []FeatureCount      `json:"by_feature"`
	ByFeatureTokens  []FeatureTokenCount `json:"by_feature_tokens"`
	DailyTrend       []DailyCount        `json:"daily_trend"`
	DailyTokenTrend  []DailyCount        `json:"daily_token_trend"`
	TopUsers         []TopAIUser         `json:"top_users"`
	TopTokenUsers    []TopAIUser         `json:"top_token_users"`
}

type FeatureCount struct {
	FeatureKey  string `json:"feature_key"`
	FeatureName string `json:"feature_name"`
	Count       int64  `json:"count"`
}

type FeatureTokenCount struct {
	FeatureKey       string `json:"feature_key"`
	FeatureName      string `json:"feature_name"`
	CallCount        int64  `json:"call_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

type TopAIUser struct {
	UserID           string `json:"user_id"`
	Email            string `json:"email,omitempty"`
	CallCount        int64  `json:"call_count"`
	PromptTokens     int64  `json:"prompt_tokens,omitempty"`
	CompletionTokens int64  `json:"completion_tokens,omitempty"`
	TotalTokens      int64  `json:"total_tokens,omitempty"`
	LastCalledAt     string `json:"last_called_at"`
}

type AITokenUsageSummary struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CallCount        int64   `json:"call_count"`
	AvgTokensPerCall float64 `json:"avg_tokens_per_call"`
}

type DailyUserTokenUsage struct {
	Date             string `json:"date"`
	UserID           string `json:"user_id"`
	Email            string `json:"email,omitempty"`
	CallCount        int64  `json:"call_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	LastCalledAt     string `json:"last_called_at"`
}

type AITokenUsageResult struct {
	Days        int                   `json:"days"`
	Summary     AITokenUsageSummary   `json:"summary"`
	DailyUsers  []DailyUserTokenUsage `json:"daily_users"`
	GeneratedAt string                `json:"generated_at"`
}

// ── User Funnel Stats ──

// FunnelStep represents one layer of the user conversion funnel.
type FunnelStep struct {
	Label      string `json:"label"`       // e.g. "注册", "登录"
	CountAll   int64  `json:"count_all"`   // total (all time)
	CountToday int64  `json:"count_today"` // today
	Count7D    int64  `json:"count_7d"`    // last 7 days
	Count30D   int64  `json:"count_30d"`   // last 30 days
}

// FunnelStats is the full funnel response.
type FunnelStats struct {
	Steps       []FunnelStep `json:"steps"` // ordered funnel steps
	GeneratedAt string       `json:"generated_at"`
}
