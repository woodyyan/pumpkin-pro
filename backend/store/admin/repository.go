package admin

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ── Admin CRUD ──

func (r *Repository) GetAdminByEmail(ctx context.Context, email string) (*SuperAdminRecord, error) {
	var record SuperAdminRecord
	if err := r.db.WithContext(ctx).First(&record, "email = ?", email).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdminNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) CreateAdmin(ctx context.Context, record SuperAdminRecord) error {
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *Repository) AdminExists(ctx context.Context) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&SuperAdminRecord{}).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ── Stats queries ──

func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("users").Count(&count).Error
	return count, err
}

func (r *Repository) CountUsersSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("users").Where("created_at >= ?", since).Count(&count).Error
	return count, err
}

func (r *Repository) CountActiveUsers7D(ctx context.Context) (int64, error) {
	since := time.Now().UTC().AddDate(0, 0, -7)
	var count int64
	err := r.db.WithContext(ctx).Table("auth_audit_logs").
		Where("action = ? AND success = ? AND created_at >= ?", "login", true, since).
		Distinct("user_id").Count(&count).Error
	return count, err
}

func (r *Repository) CountActiveSessions(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	var count int64
	err := r.db.WithContext(ctx).Table("user_sessions").
		Where("revoked_at IS NULL AND expires_at > ?", now).Count(&count).Error
	return count, err
}

func (r *Repository) CountStrategies(ctx context.Context) (total, system, userCreated, active int64, err error) {
	err = r.db.WithContext(ctx).Table("strategy_definitions").Count(&total).Error
	if err != nil {
		return
	}
	err = r.db.WithContext(ctx).Table("strategy_definitions").Where("user_id = '' OR user_id IS NULL").Count(&system).Error
	if err != nil {
		return
	}
	userCreated = total - system
	err = r.db.WithContext(ctx).Table("strategy_definitions").Where("status = ?", "active").Count(&active).Error
	return
}

func (r *Repository) CountReferencedStrategies(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("symbol_signal_configs").
		Distinct("strategy_id").Where("strategy_id != ''").Count(&count).Error
	return count, err
}

func (r *Repository) CountWatchlistItems(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("live_watchlist_items").Count(&count).Error
	return count, err
}

func (r *Repository) CountUsersWithWatchlist(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("live_watchlist_items").
		Distinct("user_id").Count(&count).Error
	return count, err
}

func (r *Repository) CountActiveSymbols(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("live_watchlist_items").
		Where("is_active = ?", true).Count(&count).Error
	return count, err
}

func (r *Repository) CountWebhookUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("webhook_endpoints").Count(&count).Error
	return count, err
}

func (r *Repository) CountWebhookEnabled(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("webhook_endpoints").
		Where("is_enabled = ?", true).Count(&count).Error
	return count, err
}

func (r *Repository) CountSignalConfigs(ctx context.Context) (total, enabled int64, err error) {
	err = r.db.WithContext(ctx).Table("symbol_signal_configs").Count(&total).Error
	if err != nil {
		return
	}
	err = r.db.WithContext(ctx).Table("symbol_signal_configs").
		Where("is_enabled = ?", true).Count(&enabled).Error
	return
}

func (r *Repository) CountSignalEvents(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("signal_events").Count(&count).Error
	return count, err
}

func (r *Repository) CountSignalEventsSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("signal_events").
		Where("event_time >= ?", since).Count(&count).Error
	return count, err
}

func (r *Repository) CountDeliveries(ctx context.Context) (total, delivered int64, err error) {
	err = r.db.WithContext(ctx).Table("webhook_deliveries").Count(&total).Error
	if err != nil {
		return
	}
	err = r.db.WithContext(ctx).Table("webhook_deliveries").
		Where("status = ?", "delivered").Count(&delivered).Error
	return
}

func (r *Repository) CountDeliveriesSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("webhook_deliveries").
		Where("created_at >= ?", since).Count(&count).Error
	return count, err
}

func (r *Repository) CountAuditLogins(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("auth_audit_logs").
		Where("action = ? AND success = ? AND created_at >= ?", "login", true, since).Count(&count).Error
	return count, err
}

func (r *Repository) CountAuditRegistrations(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("auth_audit_logs").
		Where("action = ? AND success = ? AND created_at >= ?", "register", true, since).Count(&count).Error
	return count, err
}

func (r *Repository) CountFailedLogins(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("auth_audit_logs").
		Where("action = ? AND success = ? AND created_at >= ?", "login", false, since).Count(&count).Error
	return count, err
}

// ── Trend queries (daily series for charts) ──

type DailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

func (r *Repository) DailyRegistrations(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Table("users").
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) DailyActiveUsers(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Table("auth_audit_logs").
		Select("DATE(created_at) as date, COUNT(DISTINCT user_id) as count").
		Where("action = ? AND success = ? AND created_at >= ?", "login", true, since).
		Group("DATE(created_at)").Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) DailySignalEvents(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Table("signal_events").
		Select("DATE(event_time) as date, COUNT(*) as count").
		Where("event_time >= ?", since).
		Group("DATE(event_time)").Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) DailyDeliveryRate(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	// Returns delivery success rate * 100 (as integer percentage) per day
	var results []DailyCount
	err := r.db.WithContext(ctx).Table("webhook_deliveries").
		Select("DATE(created_at) as date, CAST(SUM(CASE WHEN status='delivered' THEN 1 ELSE 0 END)*100.0/MAX(COUNT(*),1) AS INTEGER) as count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").Order("date ASC").
		Scan(&results).Error
	// Fallback: if the complex query fails, return empty
	if err != nil {
		return []DailyCount{}, nil
	}
	return results, nil
}

// ── Retention (simplified) ──

func (r *Repository) RetentionRate(ctx context.Context, registeredBefore time.Time, loginWithinDays int) (registered int64, retained int64, err error) {
	// Count users who registered before the given date
	err = r.db.WithContext(ctx).Table("users").Where("created_at < ?", registeredBefore).Count(&registered).Error
	if err != nil || registered == 0 {
		return
	}
	// Count how many of them logged in within the last N days
	since := time.Now().UTC().AddDate(0, 0, -loginWithinDays)
	err = r.db.WithContext(ctx).Table("auth_audit_logs").
		Where("action = ? AND success = ? AND created_at >= ?", "login", true, since).
		Where("user_id IN (SELECT id FROM users WHERE created_at < ?)", registeredBefore).
		Distinct("user_id").Count(&retained).Error
	return
}

// ── Additional module counts ──

func (r *Repository) CountBacktestRuns(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("backtest_runs").Count(&count).Error
	return count, err
}

func (r *Repository) CountBacktestRunsSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("backtest_runs").Where("created_at >= ?", since).Count(&count).Error
	return count, err
}

func (r *Repository) CountBacktestUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("backtest_runs").Distinct("user_id").Count(&count).Error
	return count, err
}

func (r *Repository) CountPortfolioRecords(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("user_portfolios").Count(&count).Error
	return count, err
}

func (r *Repository) CountPortfolioUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("user_portfolios").Distinct("user_id").Count(&count).Error
	return count, err
}

func (r *Repository) CountScreenerWatchlists(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("screener_watchlists").Count(&count).Error
	return count, err
}

func (r *Repository) CountScreenerUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("screener_watchlists").Distinct("user_id").Count(&count).Error
	return count, err
}

// ── Traffic source queries ──

type SourceCount struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

func (r *Repository) UTMSourceBreakdown(ctx context.Context) ([]SourceCount, error) {
	var results []SourceCount
	err := r.db.WithContext(ctx).Table("users").
		Select("CASE WHEN utm_source = '' THEN '直接访问' ELSE utm_source END as source, COUNT(*) as count").
		Group("utm_source").Order("count DESC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) ReferrerBreakdown(ctx context.Context, since time.Time) ([]SourceCount, error) {
	var results []SourceCount
	// Extract domain from referrer for grouping
	err := r.db.WithContext(ctx).Table("page_views").
		Select("CASE WHEN referrer = '' THEN '直接访问' ELSE referrer END as source, COUNT(*) as count").
		Where("created_at >= ?", since).
		Where("referrer != '' OR referrer = ''").
		Group("source").Order("count DESC").Limit(20).
		Scan(&results).Error
	return results, err
}

// ── AI call log stats ──

func (r *Repository) AITotalCalls(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("ai_call_logs").Count(&count).Error
	return count, err
}

func (r *Repository) AICallsSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("ai_call_logs").Where("created_at >= ?", since).Count(&count).Error
	return count, err
}

func (r *Repository) AISuccessRate(ctx context.Context) (float64, error) {
	var total, success int64
	r.db.WithContext(ctx).Table("ai_call_logs").Count(&total)
	if total == 0 {
		return 0, nil
	}
	r.db.WithContext(ctx).Table("ai_call_logs").Where("status = ?", "success").Count(&success)
	return float64(success) / float64(total), nil
}

func (r *Repository) AIAvgResponseMS(ctx context.Context) (float64, error) {
	type msResult struct{ AvgMs float64 }
	var result msResult
	err := r.db.WithContext(ctx).Table("ai_call_logs").Select("AVG(response_ms) as avg_ms").Scan(&result).Error
	return result.AvgMs, err
}

func (r *Repository) AIUniqueUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("ai_call_logs").Distinct("user_id").Where("user_id != ''").Count(&count).Error
	return count, err
}

func (r *Repository) AIByFeatureBreakdown(ctx context.Context) ([]FeatureCount, error) {
	var results []FeatureCount
	err := r.db.WithContext(ctx).Table("ai_call_logs").
		Select("feature_key, feature_name, COUNT(*) as count").
		Group("feature_key, feature_name").Order("count DESC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) AIDailyTrend(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Table("ai_call_logs").
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) AITopUsers(ctx context.Context, limit int) ([]TopAIUser, error) {
	var results []TopAIUser
	err := r.db.WithContext(ctx).Table("ai_call_logs AS logs").
		Select("logs.user_id as user_id, COALESCE(users.email, '') as email, COUNT(*) as call_count, MAX(logs.created_at) as last_called_at").
		Joins("LEFT JOIN users ON users.id = logs.user_id").
		Where("logs.user_id != '' AND logs.user_id IS NOT NULL").
		Group("logs.user_id, users.email").Order("call_count DESC").Limit(limit).
		Scan(&results).Error
	return results, err
}

func (r *Repository) AISumUsage(ctx context.Context, since time.Time) (AITokenUsageSummary, error) {
	var result AITokenUsageSummary
	query := r.db.WithContext(ctx).Table("ai_call_logs").
		Select("COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(total_tokens), 0) as total_tokens, COUNT(*) as call_count")
	if !since.IsZero() {
		query = query.Where("created_at >= ?", since)
	}
	if err := query.Scan(&result).Error; err != nil {
		return AITokenUsageSummary{}, err
	}
	if result.CallCount > 0 {
		result.AvgTokensPerCall = float64(result.TotalTokens) / float64(result.CallCount)
	}
	return result, nil
}

func (r *Repository) AIByFeatureTokenBreakdown(ctx context.Context) ([]FeatureTokenCount, error) {
	var results []FeatureTokenCount
	err := r.db.WithContext(ctx).Table("ai_call_logs").
		Select("feature_key, feature_name, COUNT(*) as call_count, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(total_tokens), 0) as total_tokens").
		Group("feature_key, feature_name").Order("total_tokens DESC, call_count DESC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) AIDailyTokenTrend(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Table("ai_call_logs").
		Select("DATE(created_at) as date, COALESCE(SUM(total_tokens), 0) as count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) AITopUsersByTokens(ctx context.Context, limit int) ([]TopAIUser, error) {
	var results []TopAIUser
	err := r.db.WithContext(ctx).Table("ai_call_logs AS logs").
		Select("logs.user_id as user_id, COALESCE(users.email, '') as email, COUNT(*) as call_count, COALESCE(SUM(logs.prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(logs.completion_tokens), 0) as completion_tokens, COALESCE(SUM(logs.total_tokens), 0) as total_tokens, MAX(logs.created_at) as last_called_at").
		Joins("LEFT JOIN users ON users.id = logs.user_id").
		Where("logs.user_id != '' AND logs.user_id IS NOT NULL").
		Group("logs.user_id, users.email").Order("total_tokens DESC, call_count DESC").Limit(limit).
		Scan(&results).Error
	return results, err
}

func (r *Repository) AIDailyUserTokenUsage(ctx context.Context, days, limit int) ([]DailyUserTokenUsage, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyUserTokenUsage
	err := r.db.WithContext(ctx).Table("ai_call_logs AS logs").
		Select("DATE(logs.created_at) as date, logs.user_id as user_id, COALESCE(users.email, '') as email, COUNT(*) as call_count, COALESCE(SUM(logs.prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(logs.completion_tokens), 0) as completion_tokens, COALESCE(SUM(logs.total_tokens), 0) as total_tokens, MAX(logs.created_at) as last_called_at").
		Joins("LEFT JOIN users ON users.id = logs.user_id").
		Where("logs.user_id != '' AND logs.user_id IS NOT NULL AND logs.created_at >= ?", since).
		Group("DATE(logs.created_at), logs.user_id, users.email").
		Order("date DESC, total_tokens DESC, call_count DESC").
		Limit(limit).
		Scan(&results).Error
	return results, err
}

// ── User Funnel queries ──

// CountUV counts unique visitors (DISTINCT visitor_id) from page_views.
func (r *Repository) CountUV(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("page_views").
		Distinct("visitor_id").Where("visitor_id != ''").Count(&count).Error
	return count, err
}

// CountUVSince counts UV since given time.
func (r *Repository) CountUVSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("page_views").
		Where("created_at >= ? AND visitor_id != ''", since).
		Distinct("visitor_id").Count(&count).Error
	return count, err
}

// CountUniqueLoginsSince counts distinct users who logged in successfully since the given time.
func (r *Repository) CountUniqueLoginsSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("auth_audit_logs").
		Where("action = ? AND success = ? AND created_at >= ?", "login", true, since).
		Distinct("user_id").Count(&count).Error
	return count, err
}

// CountUsersWithWatchlistSince counts distinct users who have at least one watchlist item since.
func (r *Repository) CountUsersWithWatchlistSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("live_watchlist_items").
		Where("created_at >= ?", since).
		Distinct("user_id").Count(&count).Error
	return count, err
}

// CountUsersWithSignalConfigsSince counts distinct users who have configured at least one signal since.
func (r *Repository) CountUsersWithSignalConfigsSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("symbol_signal_configs").
		Where("created_at >= ?", since).
		Distinct("user_id").Count(&count).Error
	return count, err
}

// CountBacktestUsersSince counts distinct users who have run backtests since.
func (r *Repository) CountBacktestUsersSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("backtest_runs").
		Where("created_at >= ?", since).
		Distinct("user_id").Count(&count).Error
	return count, err
}

// CountAIUniqueUsersSince counts distinct users who have used AI features since.
func (r *Repository) CountAIUniqueUsersSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("ai_call_logs").
		Where("created_at >= ? AND user_id != '' AND user_id IS NOT NULL", since).
		Distinct("user_id").Count(&count).Error
	return count, err
}
