package factorindex

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

func (r *Repository) DeleteDailyRows(ctx context.Context, indexIDs []string, fromDate, toDate string) error {
	if len(indexIDs) == 0 {
		return nil
	}
	query := r.db.WithContext(ctx).Where("index_id IN ?", indexIDs)
	query = applyOptionalDateRange(query, "trade_date", fromDate, toDate)
	return query.Delete(&Daily{}).Error
}

func (r *Repository) DeleteRebalances(ctx context.Context, indexIDs []string, fromDate, toDate string) error {
	if len(indexIDs) == 0 {
		return nil
	}
	baseQuery := r.db.WithContext(ctx).Model(&Rebalance{}).Where("index_id IN ?", indexIDs)
	baseQuery = applyOptionalDateRange(baseQuery, "signal_date", fromDate, toDate)

	var rebalanceIDs []string
	if err := baseQuery.Pluck("id", &rebalanceIDs).Error; err != nil {
		return err
	}
	if len(rebalanceIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("rebalance_id IN ?", rebalanceIDs).Delete(&Constituent{}).Error; err != nil {
			return err
		}
		deleteQuery := tx.Where("index_id IN ?", indexIDs)
		deleteQuery = applyOptionalDateRange(deleteQuery, "signal_date", fromDate, toDate)
		return deleteQuery.Delete(&Rebalance{}).Error
	})
}

func applyOptionalDateRange(query *gorm.DB, column, fromDate, toDate string) *gorm.DB {
	fromDate = strings.TrimSpace(fromDate)
	toDate = strings.TrimSpace(toDate)
	if fromDate != "" {
		query = query.Where(column+" >= ?", fromDate)
	}
	if toDate != "" {
		query = query.Where(column+" <= ?", toDate)
	}
	return query
}
