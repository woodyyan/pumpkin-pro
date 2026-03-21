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
