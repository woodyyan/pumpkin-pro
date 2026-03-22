package backtest

import (
	"context"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, record *BacktestRunRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *Repository) ListByUser(ctx context.Context, userID string, limit, offset int) ([]BacktestRunRecord, int64, error) {
	var total int64
	r.db.WithContext(ctx).
		Model(&BacktestRunRecord{}).
		Where("user_id = ?", userID).
		Count(&total)

	var records []BacktestRunRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error

	return records, total, err
}

func (r *Repository) GetByID(ctx context.Context, userID, id string) (*BacktestRunRecord, error) {
	var record BacktestRunRecord
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&BacktestRunRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) CountByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&BacktestRunRecord{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}

func (r *Repository) DeleteOldest(ctx context.Context, userID string, keepCount int) error {
	// Find the IDs to keep (most recent N)
	var keepIDs []string
	r.db.WithContext(ctx).
		Model(&BacktestRunRecord{}).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(keepCount).
		Pluck("id", &keepIDs)

	if len(keepIDs) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).
		Where("user_id = ? AND id NOT IN ?", userID, keepIDs).
		Delete(&BacktestRunRecord{}).Error
}
