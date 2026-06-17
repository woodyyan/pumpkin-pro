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
	normalizedMarket := strings.TrimSpace(strings.ToUpper(market))
	err := r.db.WithContext(ctx).
		Where("market = ?", normalizedMarket).
		Order(clause.Expr{SQL: "CASE WHEN trigger = ? THEN 0 ELSE 1 END", Vars: []any{TriggerDailyAuto}}).
		Order("trade_date desc, updated_at desc").
		First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) SaveGenerateLog(ctx context.Context, record GenerateLogRecord) error {
	record.TradeDate = strings.TrimSpace(record.TradeDate)
	record.Trigger = strings.TrimSpace(record.Trigger)
	record.Status = strings.TrimSpace(record.Status)
	record.SnapshotDate = strings.TrimSpace(record.SnapshotDate)
	record.Model = strings.TrimSpace(record.Model)
	record.UserID = strings.TrimSpace(record.UserID)
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *Repository) CreateGenerateLog(ctx context.Context, record GenerateLogRecord) (*GenerateLogRecord, error) {
	record.TradeDate = strings.TrimSpace(record.TradeDate)
	record.Trigger = strings.TrimSpace(record.Trigger)
	record.Status = strings.TrimSpace(record.Status)
	record.SnapshotDate = strings.TrimSpace(record.SnapshotDate)
	record.Model = strings.TrimSpace(record.Model)
	record.UserID = strings.TrimSpace(record.UserID)
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Repository) SaveGenerateTrace(ctx context.Context, record GenerateTraceRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "generate_log_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"system_prompt", "user_prompt", "assistant_reasoning", "assistant_content", "created_at"}),
	}).Create(&record).Error
}

func (r *Repository) ListGenerateLogs(ctx context.Context, limit int) ([]GenerateLogRecord, error) {
	if limit <= 0 {
		limit = maxGenerateLogs
	}
	var items []GenerateLogRecord
	if err := r.db.WithContext(ctx).
		Order("created_at desc, id desc").
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) GetLatestGenerateLog(ctx context.Context) (*GenerateLogRecord, error) {
	var item GenerateLogRecord
	if err := r.db.WithContext(ctx).
		Order("created_at desc, id desc").
		First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetGenerateTraceByLogID(ctx context.Context, generateLogID uint) (*GenerateTraceRecord, error) {
	if generateLogID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var item GenerateTraceRecord
	if err := r.db.WithContext(ctx).
		Where("generate_log_id = ?", generateLogID).
		First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}
