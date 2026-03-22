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

func (r *Repository) List(ctx context.Context, userID string) ([]WatchlistItem, error) {
	var records []WatchlistRecord
	if err := r.db.WithContext(ctx).
		Model(&WatchlistRecord{}).
		Where("user_id = ?", userID).
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

func (r *Repository) Create(ctx context.Context, userID, symbol, name, exchange string) (*WatchlistItem, error) {
	now := time.Now().UTC()
	if exchange == "" {
		exchange = "HKEX"
	}
	record := WatchlistRecord{
		UserID:    userID,
		Symbol:    symbol,
		Name:      name,
		Exchange:  exchange,
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

func (r *Repository) Delete(ctx context.Context, userID, symbol string) error {
	result := r.db.WithContext(ctx).Where("user_id = ? AND symbol = ?", userID, symbol).Delete(&WatchlistRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetBySymbol(ctx context.Context, userID, symbol string) (*WatchlistItem, error) {
	var record WatchlistRecord
	if err := r.db.WithContext(ctx).First(&record, "user_id = ? AND symbol = ?", userID, symbol).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	item := toWatchlistItem(record)
	return &item, nil
}

func (r *Repository) GetActive(ctx context.Context, userID string) (*WatchlistItem, error) {
	var record WatchlistRecord
	if err := r.db.WithContext(ctx).Where("user_id = ? AND is_active = ?", userID, true).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	item := toWatchlistItem(record)
	return &item, nil
}

func (r *Repository) SetActiveSymbol(ctx context.Context, userID, symbol string) (*WatchlistItem, error) {
	returnItem := &WatchlistItem{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&WatchlistRecord{}).Where("user_id = ? AND symbol = ?", userID, symbol).Updates(map[string]any{
			"is_active":  true,
			"updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}

		if err := tx.Model(&WatchlistRecord{}).Where("user_id = ? AND symbol <> ?", userID, symbol).Update("is_active", false).Error; err != nil {
			return err
		}

		var updated WatchlistRecord
		if err := tx.First(&updated, "user_id = ? AND symbol = ?", userID, symbol).Error; err != nil {
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
