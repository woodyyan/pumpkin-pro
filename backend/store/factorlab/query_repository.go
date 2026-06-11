package factorlab

import (
	"context"
	"encoding/json"
	"strings"
)

var factorScoreColumns = map[string]string{
	"value_score":          "value_score",
	"dividend_yield_score": "dividend_yield_score",
	"growth_score":         "growth_score",
	"quality_score":        "quality_score",
	"momentum_score":       "momentum_score",
	"size_score":           "size_score",
	"low_volatility_score": "low_volatility_score",
}

var rawMetricColumns = map[string]string{
	"pe":              "pe",
	"pb":              "pb",
	"ps":              "ps",
	"dividend_yield":  "dividend_yield",
	"earning_growth":  "earning_growth",
	"revenue_growth":  "revenue_growth",
	"performance_1y":  "performance_1y",
	"roe":             "roe",
	"fcf_margin":      "fcf_margin",
	"asset_to_equity": "asset_to_equity",
	"momentum_1m":     "momentum_1m",
	"market_cap":      "market_cap",
	"volatility_1m":   "volatility_1m",
	"beta_1y":         "beta_1y",
}

func (r *Repository) LatestSnapshotDate(ctx context.Context) (string, error) {
	var date string
	err := r.db.WithContext(ctx).
		Model(&FactorScore{}).
		Select("COALESCE(MAX(snapshot_date), '')").
		Scan(&date).Error
	return date, err
}

func (r *Repository) LastDailyComputeRun(ctx context.Context) (*FactorTaskRun, error) {
	var run FactorTaskRun
	err := r.db.WithContext(ctx).
		Where("task_type IN ?", []string{TaskTypeDailyCompute, "factor_score_compute"}).
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
	query := r.db.WithContext(ctx).Model(&FactorScore{}).Where("snapshot_date = ?", snapshotDate)
	if err := query.Count(&stats.Total).Error; err != nil {
		return stats, err
	}
	if err := query.Where("is_new_stock = ?", true).Count(&stats.NewStockCount).Error; err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *Repository) MetricCoverage(ctx context.Context, snapshotDate string) (map[string]int64, error) {
	return r.coverageForColumns(ctx, &FactorScore{}, snapshotDate, factorScoreColumns)
}

func (r *Repository) RawMetricCoverage(ctx context.Context, snapshotDate string) (map[string]int64, error) {
	return r.coverageForColumns(ctx, &FactorSnapshot{}, snapshotDate, rawMetricColumns)
}

func (r *Repository) coverageForColumns(ctx context.Context, model any, snapshotDate string, columns map[string]string) (map[string]int64, error) {
	coverage := make(map[string]int64, len(columns))
	for key, column := range columns {
		var count int64
		if err := r.db.WithContext(ctx).
			Model(model).
			Where("snapshot_date = ? AND "+column+" IS NOT NULL", snapshotDate).
			Count(&count).Error; err != nil {
			return nil, err
		}
		coverage[key] = count
	}
	return coverage, nil
}

func (r *Repository) ListTaskRuns(ctx context.Context, limit int) ([]FactorTaskRun, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var runs []FactorTaskRun
	err := r.db.WithContext(ctx).
		Where("task_type IN ?", []string{"backfill", TaskTypeDailyCompute, "factor_score_compute"}).
		Order("started_at DESC").
		Limit(limit).
		Find(&runs).Error
	return runs, err
}

func (r *Repository) DBQuickCheck(ctx context.Context) (string, error) {
	var result string
	err := r.db.WithContext(ctx).Raw("PRAGMA quick_check").Row().Scan(&result)
	return result, err
}

func (r *Repository) LatestBackfillRunByMode(ctx context.Context, mode string) (*FactorTaskRun, error) {
	var runs []FactorTaskRun
	err := r.db.WithContext(ctx).
		Where("task_type = ?", TaskTypeBackfill).
		Order("started_at DESC").
		Limit(20).
		Find(&runs).Error
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		var payload map[string]any
		if json.Unmarshal([]byte(run.ParamsJSON), &payload) != nil {
			continue
		}
		argsPayload, _ := payload["args"].(map[string]any)
		modeValue, _ := payload["mode"].(string)
		if strings.TrimSpace(modeValue) == strings.TrimSpace(mode) {
			copyRun := run
			return &copyRun, nil
		}
		if argsMode, _ := argsPayload["mode"].(string); strings.TrimSpace(argsMode) == strings.TrimSpace(mode) {
			copyRun := run
			return &copyRun, nil
		}
	}
	return nil, nil
}

func (r *Repository) ActiveAShareUniverseCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*)
		FROM factor_securities
		WHERE exchange IN ('SSE', 'SZSE')
		  AND board IN ('MAIN', 'CHINEXT')
		  AND is_active = 1
	`).Row().Scan(&count)
	return count, err
}

func (r *Repository) CoveredAShareIndustryProfileCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*)
		FROM company_profiles
		WHERE exchange IN ('SSE', 'SZSE')
		  AND code <> ''
		  AND industry_name <> ''
	`).Row().Scan(&count)
	return count, err
}

func (r *Repository) LatestIndustryRefreshAt(ctx context.Context) (string, error) {
	var value string
	err := r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(MAX(updated_at), '')
		FROM factor_security_industries
		WHERE industry_name <> ''
	`).Row().Scan(&value)
	return strings.TrimSpace(value), err
}

func (r *Repository) ScanSnapshots(ctx context.Context, input ScanInput) (ScanResult, error) {
	var records []FactorScore
	if err := r.db.WithContext(ctx).
		Model(&FactorScore{}).
		Where("snapshot_date = ?", input.SnapshotDate).
		Find(&records).Error; err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Total: int64(len(records)), Items: records}, nil
}

func isRecordNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "record not found")
}
