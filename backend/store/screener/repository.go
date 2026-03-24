package screener

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CountByUser returns the number of watchlists owned by a user.
func (r *Repository) CountByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&WatchlistRecord{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}

// List returns all watchlists for a user, with stock counts.
func (r *Repository) List(ctx context.Context, userID string) ([]Watchlist, error) {
	var records []WatchlistRecord
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]Watchlist, 0, len(records))
	for _, rec := range records {
		var count int64
		r.db.WithContext(ctx).
			Model(&WatchlistStockRecord{}).
			Where("watchlist_id = ?", rec.ID).
			Count(&count)
		items = append(items, rec.toListItem(int(count)))
	}
	return items, nil
}

// Create inserts a new watchlist and its stocks in a transaction.
func (r *Repository) Create(ctx context.Context, wl WatchlistRecord, stocks []WatchlistStockRecord) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&wl).Error; err != nil {
			if isUniqueErr(err) {
				return ErrConflict
			}
			return err
		}
		if len(stocks) > 0 {
			if err := tx.Create(&stocks).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetByID returns a watchlist and its stocks, scoped to user.
func (r *Repository) GetByID(ctx context.Context, userID, id string) (*WatchlistRecord, []WatchlistStockRecord, error) {
	var wl WatchlistRecord
	if err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&wl).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	var stocks []WatchlistStockRecord
	if err := r.db.WithContext(ctx).
		Where("watchlist_id = ?", id).
		Order("created_at ASC").
		Find(&stocks).Error; err != nil {
		return nil, nil, err
	}

	return &wl, stocks, nil
}

// Delete removes a watchlist and its stocks.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete stocks first (foreign-key-like cleanup)
		if err := tx.Where("watchlist_id = ?", id).Delete(&WatchlistStockRecord{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&WatchlistRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func isUniqueErr(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique") || strings.Contains(text, "duplicate")
}
