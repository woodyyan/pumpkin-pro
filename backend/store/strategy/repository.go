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

func (r *Repository) CountSystem(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&StrategyRecord{}).Where("user_id = ?", "").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) List(ctx context.Context, userID string, activeOnly bool, includeSystem bool) ([]*Strategy, error) {
	query := r.db.WithContext(ctx).Model(&StrategyRecord{})
	if includeSystem && strings.TrimSpace(userID) != "" {
		query = query.Where("user_id = ? OR user_id = ?", userID, "")
	} else {
		query = query.Where("user_id = ?", strings.TrimSpace(userID))
	}
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

func (r *Repository) GetByID(ctx context.Context, id string, userID string, includeSystem bool) (*Strategy, error) {
	record, err := r.getRecordByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canAccessRecord(record, userID, includeSystem) {
		return nil, ErrNotFound
	}
	return record.toStrategy()
}

func (r *Repository) GetByName(ctx context.Context, name string, userID string, includeSystem bool) (*Strategy, error) {
	query := r.db.WithContext(ctx)
	if includeSystem && strings.TrimSpace(userID) != "" {
		query = query.Where("(user_id = ? OR user_id = ?) AND name = ?", userID, "", name)
	} else {
		query = query.Where("user_id = ? AND name = ?", strings.TrimSpace(userID), name)
	}

	var record StrategyRecord
	if err := query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return record.toStrategy()
}

func (r *Repository) Create(ctx context.Context, userID string, payload StrategyPayload) (*Strategy, error) {
	now := nowUTC()
	record, err := buildRecord(strings.TrimSpace(userID), payload, now, now)
	if err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, translateWriteError(err)
	}
	return record.toStrategy()
}

func (r *Repository) Update(ctx context.Context, strategyID string, userID string, payload StrategyPayload) (*Strategy, error) {
	existing, err := r.getRecordByID(ctx, strategyID)
	if err != nil {
		return nil, err
	}
	ownerID := strings.TrimSpace(existing.UserID)
	currentUserID := strings.TrimSpace(userID)
	if ownerID != "" && ownerID != currentUserID {
		return nil, ErrForbidden
	}

	record, err := buildRecord(existing.UserID, payload, existing.CreatedAt, nowUTC())
	if err != nil {
		return nil, err
	}
	record.ID = strategyID

	if err := r.db.WithContext(ctx).Model(&StrategyRecord{}).Where("id = ?", strategyID).Updates(record).Error; err != nil {
		return nil, translateWriteError(err)
	}
	return r.GetByID(ctx, strategyID, userID, true)
}

func (r *Repository) getRecordByID(ctx context.Context, id string) (*StrategyRecord, error) {
	var record StrategyRecord
	if err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func canAccessRecord(record *StrategyRecord, userID string, includeSystem bool) bool {
	if record == nil {
		return false
	}
	owner := strings.TrimSpace(record.UserID)
	if owner == strings.TrimSpace(userID) {
		return true
	}
	if includeSystem && owner == "" {
		return true
	}
	return false
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
