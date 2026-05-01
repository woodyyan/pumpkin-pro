package analytics

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Insert(ctx context.Context, record PageViewRecord) error {
	return r.db.WithContext(ctx).Create(&record).Error
}

// ── Admin stats queries ──

func (r *Repository) CountPVSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&PageViewRecord{}).Where("created_at >= ?", since).Count(&count).Error
	return count, err
}

func (r *Repository) CountUVSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&PageViewRecord{}).Where("created_at >= ?", since).Distinct("visitor_id").Count(&count).Error
	return count, err
}

func (r *Repository) DailyPV(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Model(&PageViewRecord{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) DailyUV(ctx context.Context, days int) ([]DailyCount, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var results []DailyCount
	err := r.db.WithContext(ctx).Model(&PageViewRecord{}).
		Select("DATE(created_at) as date, COUNT(DISTINCT visitor_id) as count").
		Where("created_at >= ?", since).
		Group("DATE(created_at)").
		Order("date ASC").
		Scan(&results).Error
	return results, err
}

func (r *Repository) TopPages(ctx context.Context, since time.Time, limit int) ([]PageRank, error) {
	if limit <= 0 {
		limit = 10
	}
	var results []PageRank
	err := r.db.WithContext(ctx).Model(&PageViewRecord{}).
		Select("page_path, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("page_path").
		Order("count DESC").
		Limit(limit).
		Scan(&results).Error
	return results, err
}

func (r *Repository) DeviceBreakdown(ctx context.Context, since time.Time) (*DeviceStats, error) {
	type row struct {
		Category string
		Count    int64
	}
	var results []row
	err := r.db.WithContext(ctx).Model(&PageViewRecord{}).
		Select("CASE WHEN screen_width >= 1024 THEN 'desktop' WHEN screen_width >= 768 THEN 'tablet' ELSE 'mobile' END as category, COUNT(*) as count").
		Where("created_at >= ? AND screen_width > 0", since).
		Group("category").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	stats := &DeviceStats{}
	for _, r := range results {
		switch r.Category {
		case "desktop":
			stats.Desktop = r.Count
		case "tablet":
			stats.Tablet = r.Count
		case "mobile":
			stats.Mobile = r.Count
		}
	}
	return stats, nil
}

func (r *Repository) DeleteOlderThan(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&PageViewRecord{}).Error
}

// ── Device Snapshot queries ──

func (r *Repository) InsertDeviceSnapshot(ctx context.Context, record *DeviceSnapshot) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *Repository) GetDeviceAnalytics(ctx context.Context, since time.Time) (*DeviceAnalyticsResult, error) {
	result := &DeviceAnalyticsResult{}

	// Only count real user interactions (page views and auth events).
	// Exclude api_error sources which often come from server-to-server calls,
	// health checks, or scrapers with misleading UA strings like "node-fetch".
	sourceFilter := "source IN ('page_view', 'auth')"

	// Device type breakdown (distinct visitor_id)
	var deviceRows []struct {
		Category string
		Count    int64
	}
	err := r.db.WithContext(ctx).Model(&DeviceSnapshot{}).
		Select("device_type as category, COUNT(DISTINCT visitor_id) as count").
		Where("created_at >= ? AND device_type != ? AND "+sourceFilter, since, "unknown").
		Group("device_type").
		Scan(&deviceRows).Error
	if err != nil {
		return nil, err
	}
	result.DeviceTypes = normalizeCategoryCounts(deviceRows)

	// OS family breakdown (distinct visitor_id)
	var osRows []struct {
		Category string
		Count    int64
	}
	err = r.db.WithContext(ctx).Model(&DeviceSnapshot{}).
		Select("os_family as category, COUNT(DISTINCT visitor_id) as count").
		Where("created_at >= ? AND os_family != ? AND "+sourceFilter, since, "unknown").
		Group("os_family").
		Scan(&osRows).Error
	if err != nil {
		return nil, err
	}
	result.OSFamilies = normalizeCategoryCounts(osRows)

	// Browser family breakdown (distinct visitor_id)
	var browserRows []struct {
		Category string
		Count    int64
	}
	err = r.db.WithContext(ctx).Model(&DeviceSnapshot{}).
		Select("browser_family as category, COUNT(DISTINCT visitor_id) as count").
		Where("created_at >= ? AND browser_family != ? AND "+sourceFilter, since, "unknown").
		Group("browser_family").
		Scan(&browserRows).Error
	if err != nil {
		return nil, err
	}
	result.BrowserFamilies = normalizeCategoryCounts(browserRows)

	return result, nil
}

func (r *Repository) GetTopActiveUsersWithDevices(ctx context.Context, since time.Time, limit int) ([]TopActiveUserDevice, error) {
	if limit <= 0 {
		limit = 20
	}

	// Find top active users by login days in auth_audit_logs, joined with users for email
	type activeUser struct {
		UserID       string
		ActiveDays   int
		LastActiveAt string
		Email        string
	}

	var activeUsers []activeUser
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.user_id, COUNT(DISTINCT DATE(a.created_at)) as active_days, MAX(a.created_at) as last_active_at, u.email
		FROM auth_audit_logs a
		LEFT JOIN users u ON a.user_id = u.id
		WHERE a.action = ? AND a.success = ? AND a.created_at >= ? AND a.user_id != ''
		GROUP BY a.user_id
		ORDER BY active_days DESC, last_active_at DESC
		LIMIT ?
	`, "login", true, since, limit).Scan(&activeUsers).Error
	if err != nil {
		return nil, err
	}

	if len(activeUsers) == 0 {
		return []TopActiveUserDevice{}, nil
	}

	// Get the most recent device snapshot for each user.
	// Only consider page_view and auth sources to avoid api_error noise
	// (server-to-server calls, health checks, scrapers) polluting the result.
	results := make([]TopActiveUserDevice, 0, len(activeUsers))
	for _, u := range activeUsers {
		var snap DeviceSnapshot
		err := r.db.WithContext(ctx).Model(&DeviceSnapshot{}).
			Where("user_id = ? AND created_at >= ? AND source IN ('page_view', 'auth')", u.UserID, since).
			Order("created_at DESC").
			First(&snap).Error

		browser := "unknown"
		os := "unknown"
		if err == nil && snap.BrowserFamily != "" {
			browser = snap.BrowserFamily
			if snap.BrowserVersion != "" {
				browser += " " + snap.BrowserVersion
			}
		}
		if err == nil && snap.OSFamily != "" {
			os = snap.OSFamily
			if snap.OSVersion != "" {
				os += " " + snap.OSVersion
			}
		}

		results = append(results, TopActiveUserDevice{
			UserID:       u.UserID,
			Email:        u.Email,
			ActiveDays:   u.ActiveDays,
			LastActiveAt: u.LastActiveAt,
			Browser:      browser,
			OS:           os,
		})
	}

	return results, nil
}

func normalizeCategoryCounts(rows []struct {
	Category string
	Count    int64
}) []CategoryCount {
	var total int64
	for _, r := range rows {
		total += r.Count
	}

	results := make([]CategoryCount, 0, len(rows))
	for _, r := range rows {
		pct := float64(0)
		if total > 0 {
			pct = float64(r.Count) * 100.0 / float64(total)
		}
		results = append(results, CategoryCount{
			Category:   r.Category,
			Count:      r.Count,
			Percentage: pct,
		})
	}
	return results
}
