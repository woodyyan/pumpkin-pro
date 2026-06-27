package aireport

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListReports(ctx context.Context) ([]ResearchReportRecord, error) {
	var records []ResearchReportRecord
	err := r.db.WithContext(ctx).
		Order("source_trade_date DESC").
		Order("created_at DESC").
		Find(&records).Error
	return records, err
}

func (r *Repository) GetReport(ctx context.Context, id string) (*ResearchReportRecord, error) {
	var record ResearchReportRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReportNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) CreateReport(ctx context.Context, record ResearchReportRecord) error {
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *Repository) UpdateReport(ctx context.Context, record ResearchReportRecord) error {
	return r.db.WithContext(ctx).Save(&record).Error
}

func (r *Repository) DeleteReport(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&ResearchReportRecord{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrReportNotFound
	}
	return nil
}

func (r *Repository) GetServiceConfig(ctx context.Context) (*ServiceConfigRecord, error) {
	var record ServiceConfigRecord
	if err := r.db.WithContext(ctx).Order("created_at ASC").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReportNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) SaveServiceConfig(ctx context.Context, record ServiceConfigRecord) error {
	return r.db.WithContext(ctx).Save(&record).Error
}
