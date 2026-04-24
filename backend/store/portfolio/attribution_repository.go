package portfolio

import (
	"context"
	"errors"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) ListSecurityProfilesBySymbols(ctx context.Context, symbols []string) ([]SecurityProfileRecord, error) {
	if len(symbols) == 0 {
		return []SecurityProfileRecord{}, nil
	}
	var records []SecurityProfileRecord
	err := r.db.WithContext(ctx).
		Where("symbol IN ?", symbols).
		Find(&records).Error
	return records, err
}

func (r *Repository) UpsertSecurityProfiles(ctx context.Context, records []SecurityProfileRecord) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "symbol"}},
			DoUpdates: clause.AssignmentColumns([]string{"exchange", "name", "sector_code", "sector_name", "benchmark_code", "source", "updated_at"}),
		}).
		Create(&records).Error
}

func (r *Repository) UpsertPositionDailySnapshots(ctx context.Context, records []PortfolioPositionDailySnapshotRecord) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "snapshot_date"}, {Name: "symbol"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"exchange", "currency_code", "currency_symbol", "name", "shares", "avg_cost_price", "total_cost_amount",
				"close_price", "prev_close_price", "market_value_amount", "unrealized_pnl_amount", "realized_pnl_cum",
				"position_weight_ratio", "sector_code", "sector_name", "benchmark_code", "updated_at",
			}),
		}).
		Create(&records).Error
}

func (r *Repository) DeletePositionDailySnapshotsInRange(ctx context.Context, userID, startDate, endDate string) error {
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if startDate != "" {
		query = query.Where("snapshot_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("snapshot_date <= ?", endDate)
	}
	return query.Delete(&PortfolioPositionDailySnapshotRecord{}).Error
}

func (r *Repository) ListPositionDailySnapshots(ctx context.Context, userID string, scopes []string, startDate, endDate string) ([]PortfolioPositionDailySnapshotRecord, error) {
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if len(scopes) > 0 {
		query = query.Where("(exchange = 'HKEX' AND ? LIKE '%HKEX%') OR (exchange IN ('SSE','SZSE') AND ? LIKE '%ASHARE%')", joinScopes(scopes), joinScopes(scopes))
	}
	if startDate != "" {
		query = query.Where("snapshot_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("snapshot_date <= ?", endDate)
	}
	var records []PortfolioPositionDailySnapshotRecord
	err := query.Order("snapshot_date ASC, symbol ASC").Find(&records).Error
	return records, err
}

func (r *Repository) GetEarliestActiveEventDate(ctx context.Context, userID string) (string, error) {
	var record PortfolioEventRecord
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_voided = ?", userID, false).
		Order("effective_at ASC, created_at ASC").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return record.TradeDate, nil
}

func sortedSecuritySymbols(records []SecurityProfileRecord) []string {
	result := make([]string, 0, len(records))
	for _, item := range records {
		result = append(result, item.Symbol)
	}
	sort.Strings(result)
	return result
}

func joinScopes(scopes []string) string {
	if len(scopes) == 0 {
		return ""
	}
	result := scopes[0]
	for i := 1; i < len(scopes); i++ {
		result += "," + scopes[i]
	}
	return result
}
