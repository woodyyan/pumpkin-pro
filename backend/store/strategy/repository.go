package strategy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&StrategyRecord{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) List(ctx context.Context, activeOnly bool) ([]*Strategy, error) {
	query := r.db.WithContext(ctx).Model(&StrategyRecord{})
	if activeOnly {
		query = query.Where("status = ?", "active")
	}

	var records []StrategyRecord
	if err := query.Order("updated_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]*Strategy, 0, len(records))
	for _, record := range records {
		item, err := record.toStrategy()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*Strategy, error) {
	var record StrategyRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return record.toStrategy()
}

func (r *Repository) GetByName(ctx context.Context, name string) (*Strategy, error) {
	var record StrategyRecord
	if err := r.db.WithContext(ctx).First(&record, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return record.toStrategy()
}

func (r *Repository) Create(ctx context.Context, payload StrategyPayload) (*Strategy, error) {
	now := nowUTC()
	record, err := buildRecord(payload, now, now)
	if err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, translateWriteError(err)
	}
	return record.toStrategy()
}

func (r *Repository) Update(ctx context.Context, strategyID string, payload StrategyPayload) (*Strategy, error) {
	var existing StrategyRecord
	if err := r.db.WithContext(ctx).First(&existing, "id = ?", strategyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	record, err := buildRecord(payload, existing.CreatedAt, nowUTC())
	if err != nil {
		return nil, err
	}
	record.ID = strategyID

	if err := r.db.WithContext(ctx).Model(&StrategyRecord{}).Where("id = ?", strategyID).Updates(record).Error; err != nil {
		return nil, translateWriteError(err)
	}
	return r.GetByID(ctx, strategyID)
}

func translateWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrConflict
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "duplicate") {
		return ErrConflict
	}
	return fmt.Errorf("write strategy failed: %w", err)
}
