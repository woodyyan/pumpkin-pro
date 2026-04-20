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

func (r *Repository) InTx(ctx context.Context, fn func(txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewRepository(tx))
	})
}

func (r *Repository) ListByUser(ctx context.Context, userID string) ([]PortfolioRecord, error) {
	var records []PortfolioRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND shares > 0", userID).
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

	updates := map[string]any{
		"shares":            record.Shares,
		"avg_cost_price":    record.AvgCostPrice,
		"total_cost_amount": record.TotalCostAmount,
		"buy_date":          record.BuyDate,
		"note":              record.Note,
		"cost_method":       record.CostMethod,
		"cost_source":       record.CostSource,
		"last_trade_at":     record.LastTradeAt,
		"last_event_id":     record.LastEventID,
		"updated_at":        record.UpdatedAt,
	}

	return r.db.WithContext(ctx).
		Model(&PortfolioRecord{}).
		Where("id = ?", existing.ID).
		Updates(updates).Error
}

func (r *Repository) Delete(ctx context.Context, userID, symbol string) error {
	return r.InTx(ctx, func(txRepo *Repository) error {
		result := txRepo.db.WithContext(ctx).
			Where("user_id = ? AND symbol = ?", userID, symbol).
			Delete(&PortfolioRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return txRepo.db.WithContext(ctx).
			Where("user_id = ? AND symbol = ?", userID, symbol).
			Delete(&PortfolioEventRecord{}).Error
	})
}

func (r *Repository) CreateEvent(ctx context.Context, record *PortfolioEventRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *Repository) HasActiveEventsBySymbol(ctx context.Context, userID, symbol string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&PortfolioEventRecord{}).
		Where("user_id = ? AND symbol = ? AND is_voided = ?", userID, symbol, false).
		Count(&count).Error
	return count > 0, err
}

func (r *Repository) ListEventsBySymbol(ctx context.Context, userID, symbol string, limit int) ([]PortfolioEventRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	var records []PortfolioEventRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ? AND is_voided = ?", userID, symbol, false).
		Order("effective_at DESC, created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

func (r *Repository) GetLatestActiveEventBySymbol(ctx context.Context, userID, symbol string) (*PortfolioEventRecord, error) {
	var record PortfolioEventRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ? AND is_voided = ?", userID, symbol, false).
		Order("effective_at DESC, created_at DESC").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) ListAllActiveEventsAsc(ctx context.Context, userID, symbol string) ([]PortfolioEventRecord, error) {
	var records []PortfolioEventRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND symbol = ? AND is_voided = ?", userID, symbol, false).
		Order("effective_at ASC, created_at ASC").
		Find(&records).Error
	return records, err
}

func (r *Repository) FindEventByID(ctx context.Context, userID, eventID string) (*PortfolioEventRecord, error) {
	var record PortfolioEventRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND id = ?", userID, eventID).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) VoidEvent(ctx context.Context, userID, eventID, voidedByEventID string) error {
	result := r.db.WithContext(ctx).
		Model(&PortfolioEventRecord{}).
		Where("user_id = ? AND id = ? AND is_voided = ?", userID, eventID, false).
		Updates(map[string]any{
			"is_voided":          true,
			"voided_by_event_id": voidedByEventID,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Investment Profile ──

func (r *Repository) GetInvestmentProfile(ctx context.Context, userID string) (*InvestmentProfileRecord, error) {
	var record InvestmentProfileRecord
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *Repository) UpsertInvestmentProfile(ctx context.Context, record *InvestmentProfileRecord) error {
	var existing InvestmentProfileRecord
	err := r.db.WithContext(ctx).Where("user_id = ?", record.UserID).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.WithContext(ctx).Create(record).Error
		}
		return err
	}
	return r.db.WithContext(ctx).
		Model(&InvestmentProfileRecord{}).
		Where("user_id = ?", record.UserID).
		Updates(map[string]any{
			"total_capital":      record.TotalCapital,
			"risk_preference":    record.RiskPreference,
			"investment_goal":    record.InvestmentGoal,
			"investment_horizon": record.InvestmentHorizon,
			"max_drawdown_pct":   record.MaxDrawdownPct,
			"experience_level":   record.ExperienceLevel,
			"note":               record.Note,
			"updated_at":         record.UpdatedAt,
		}).Error
}
