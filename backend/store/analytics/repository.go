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
