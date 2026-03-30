package portfolio

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("portfolio: not found")

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListByUser(ctx context.Context, userID string) ([]PortfolioRecord, error) {
	var records []PortfolioRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").
		Find(&records).Error
	return records, err
}

func (r *Repository) GetBySymbol(ctx context.Context, userID, symbol string) (*PortfolioRecord, error) {
	var record PortfolioRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ?", userID, symbol).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) Upsert(ctx context.Context, record *PortfolioRecord) error {
	var existing PortfolioRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ?", record.UserID, record.Symbol).
		First(&existing).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.WithContext(ctx).Create(record).Error
		}
		return err
	}

	return r.db.WithContext(ctx).
		Model(&PortfolioRecord{}).
		Where("id = ?", existing.ID).
		Updates(map[string]any{
			"shares":         record.Shares,
			"avg_cost_price": record.AvgCostPrice,
			"buy_date":       record.BuyDate,
			"note":           record.Note,
			"updated_at":     record.UpdatedAt,
		}).Error
}

func (r *Repository) Delete(ctx context.Context, userID, symbol string) error {
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ?", userID, symbol).
		Delete(&PortfolioRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
