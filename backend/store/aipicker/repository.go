package aipicker

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SaveDailyResult(ctx context.Context, record DailyResult) error {
	record.Market = strings.TrimSpace(strings.ToUpper(record.Market))
	record.TradeDate = strings.TrimSpace(record.TradeDate)
	record.Trigger = strings.TrimSpace(record.Trigger)
	record.SelectionBasis = strings.TrimSpace(record.SelectionBasis)
	record.Model = strings.TrimSpace(record.Model)
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	record.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "market"}, {Name: "trade_date"}, {Name: "trigger"}},
		DoUpdates: clause.AssignmentColumns([]string{"snapshot_date", "selection_basis", "model", "payload_json", "updated_at"}),
	}).Create(&record).Error
}

func (r *Repository) GetLatestDailyResult(ctx context.Context, market string) (*DailyResult, error) {
	var item DailyResult
	err := r.db.WithContext(ctx).
		Where("market = ? AND trigger = ?", strings.TrimSpace(strings.ToUpper(market)), TriggerDailyAuto).
		Order("trade_date desc, updated_at desc").
		First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}
