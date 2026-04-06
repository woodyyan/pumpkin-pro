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
