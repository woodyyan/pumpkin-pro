package fundcache

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const CacheTTL = 2 * time.Hour

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Get 查询缓存，返回 (dataJSON, isHit, error)
func (r *Repository) Get(ctx context.Context, symbol string) (string, bool, error) {
	var row FundamentalsCacheRow
	err := r.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		First(&row).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", false, nil // 缓存未命中，不是错误
		}
		return "", false, err
	}

	if time.Since(row.FetchedAt) > CacheTTL {
		// 过期了，删除旧记录（lazy eviction）
		r.db.WithContext(ctx).Delete(&row)
		return "", false, nil
	}

	return row.DataJSON, true, nil
}

// Upsert 写入或更新缓存（覆盖写入）
func (r *Repository) Upsert(ctx context.Context, symbol string, dataJSON string) error {
	row := FundamentalsCacheRow{
		Symbol:    symbol,
		DataJSON:  dataJSON,
		FetchedAt: time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Save(&row).Error
}

// CleanupExpired 删除所有过期条目（可选的定时清理入口）
func (r *Repository) CleanupExpired(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-CacheTTL)
	return r.db.WithContext(ctx).
		Where("fetched_at < ?", cutoff).
		Delete(&FundamentalsCacheRow{}).Error
}
