package factorlab

import (
	"context"
	"fmt"
	"strings"
)

var metricColumns = map[string]string{
	"market_cap":                "market_cap",
	"pe":                        "pe",
	"pb":                        "pb",
	"ps":                        "ps",
	"dividend_yield":            "dividend_yield",
	"earning_growth":            "earning_growth",
	"revenue_growth":            "revenue_growth",
	"performance_1y":            "performance_1y",
	"performance_since_listing": "performance_since_listing",
	"momentum_1m":               "momentum_1m",
	"roe":                       "roe",
	"operating_cf_margin":       "operating_cf_margin",
	"asset_to_equity":           "asset_to_equity",
	"volatility_1m":             "volatility_1m",
	"beta_1y":                   "beta_1y",
}

var sortableColumns = map[string]string{
	"code":                      "code",
	"name":                      "name",
	"board":                     "board",
	"close_price":               "close_price",
	"available_trading_days":    "available_trading_days",
	"market_cap":                "market_cap",
	"pe":                        "pe",
	"pb":                        "pb",
	"ps":                        "ps",
	"dividend_yield":            "dividend_yield",
	"earning_growth":            "earning_growth",
	"revenue_growth":            "revenue_growth",
	"performance_1y":            "performance_1y",
	"performance_since_listing": "performance_since_listing",
	"momentum_1m":               "momentum_1m",
	"roe":                       "roe",
	"operating_cf_margin":       "operating_cf_margin",
	"asset_to_equity":           "asset_to_equity",
	"volatility_1m":             "volatility_1m",
	"beta_1y":                   "beta_1y",
}

func (r *Repository) LatestSnapshotDate(ctx context.Context) (string, error) {
	var date string
	err := r.db.WithContext(ctx).
		Model(&FactorSnapshot{}).
		Select("COALESCE(MAX(snapshot_date), '')").
		Scan(&date).Error
	return date, err
}

func (r *Repository) LastDailyComputeRun(ctx context.Context) (*FactorTaskRun, error) {
	var run FactorTaskRun
	err := r.db.WithContext(ctx).
		Where("task_type = ?", TaskTypeDailyCompute).
		Order("started_at DESC").
		First(&run).Error
	if err != nil {
		if isRecordNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (r *Repository) SnapshotStats(ctx context.Context, snapshotDate string) (SnapshotStats, error) {
	var stats SnapshotStats
	query := r.db.WithContext(ctx).Model(&FactorSnapshot{}).Where("snapshot_date = ?", snapshotDate)
	if err := query.Count(&stats.Total).Error; err != nil {
		return stats, err
	}
	if err := query.Where("is_new_stock = ?", true).Count(&stats.NewStockCount).Error; err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *Repository) MetricCoverage(ctx context.Context, snapshotDate string) (map[string]int64, error) {
	coverage := make(map[string]int64, len(metricColumns))
	for key, column := range metricColumns {
		var count int64
		if err := r.db.WithContext(ctx).
			Model(&FactorSnapshot{}).
			Where("snapshot_date = ? AND "+column+" IS NOT NULL", snapshotDate).
			Count(&count).Error; err != nil {
			return nil, err
		}
		coverage[key] = count
	}
	return coverage, nil
}

func (r *Repository) ScanSnapshots(ctx context.Context, input ScanInput) (ScanResult, error) {
	query := r.db.WithContext(ctx).
		Model(&FactorSnapshot{}).
		Where("snapshot_date = ?", input.SnapshotDate)

	for key, bounds := range input.Filters {
		column, ok := metricColumns[key]
		if !ok {
			return ScanResult{}, fmt.Errorf("unsupported filter: %s", key)
		}
		query = query.Where(column + " IS NOT NULL")
		if bounds.Min != nil {
			query = query.Where(column+" >= ?", *bounds.Min)
		}
		if bounds.Max != nil {
			query = query.Where(column+" <= ?", *bounds.Max)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ScanResult{}, err
	}

	sortColumn := sortableColumns[input.SortBy]
	if sortColumn == "" {
		sortColumn = "code"
	}
	sortOrder := "ASC"
	if strings.EqualFold(input.SortOrder, "desc") {
		sortOrder = "DESC"
	}
	offset := (input.Page - 1) * input.PageSize
	var records []FactorSnapshot
	if err := query.
		Order(sortColumn + " IS NULL ASC").
		Order(sortColumn + " " + sortOrder).
		Order("code ASC").
		Offset(offset).
		Limit(input.PageSize).
		Find(&records).Error; err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Total: total, Items: records}, nil
}

func isRecordNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "record not found")
}
