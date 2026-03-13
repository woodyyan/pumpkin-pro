package live

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context) ([]WatchlistItem, error) {
	var records []WatchlistRecord
	if err := r.db.WithContext(ctx).
		Model(&WatchlistRecord{}).
		Order("is_active DESC").
		Order("updated_at DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]WatchlistItem, 0, len(records))
	for _, record := range records {
		items = append(items, toWatchlistItem(record))
	}
	return items, nil
}

func (r *Repository) Create(ctx context.Context, symbol, name string) (*WatchlistItem, error) {
	now := time.Now().UTC()
	record := WatchlistRecord{
		Symbol:    symbol,
		Name:      name,
		Exchange:  "HKEX",
		IsActive:  false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || isUniqueError(err.Error()) {
			return nil, ErrConflict
		}
		return nil, err
	}
	item := toWatchlistItem(record)
	return &item, nil
}

func (r *Repository) Delete(ctx context.Context, symbol string) error {
	result := r.db.WithContext(ctx).Where("symbol = ?", symbol).Delete(&WatchlistRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetBySymbol(ctx context.Context, symbol string) (*WatchlistItem, error) {
	var record WatchlistRecord
	if err := r.db.WithContext(ctx).First(&record, "symbol = ?", symbol).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	item := toWatchlistItem(record)
	return &item, nil
}

func (r *Repository) GetActive(ctx context.Context) (*WatchlistItem, error) {
	var record WatchlistRecord
	if err := r.db.WithContext(ctx).Where("is_active = ?", true).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	item := toWatchlistItem(record)
	return &item, nil
}

func (r *Repository) SetActiveSymbol(ctx context.Context, symbol string) (*WatchlistItem, error) {
	returnItem := &WatchlistItem{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&WatchlistRecord{}).Where("symbol = ?", symbol).Updates(map[string]any{
			"is_active":  true,
			"updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}

		if err := tx.Model(&WatchlistRecord{}).Where("symbol <> ?", symbol).Update("is_active", false).Error; err != nil {
			return err
		}

		var updated WatchlistRecord
		if err := tx.First(&updated, "symbol = ?", symbol).Error; err != nil {
			return err
		}
		item := toWatchlistItem(updated)
		*returnItem = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return returnItem, nil
}

func toWatchlistItem(record WatchlistRecord) WatchlistItem {
	return WatchlistItem{
		Symbol:    record.Symbol,
		Name:      record.Name,
		Exchange:  record.Exchange,
		IsActive:  record.IsActive,
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func isUniqueError(errMsg string) bool {
	text := strings.ToLower(strings.TrimSpace(errMsg))
	return strings.Contains(text, "unique") || strings.Contains(text, "duplicate")
}
