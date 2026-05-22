package factorlab

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
	weightTolerance = 0.001
	weightPrecision = 100
)

var factorDefinitions = []FactorDefinition{
	{Key: "value", Label: "价值", Format: "score", Description: "PE、PB、PS 排名分加权后的估值风格得分。"},
	{Key: "dividend_yield", Label: "股息率", Format: "score", Description: "股息收益率排名分。"},
	{Key: "growth", Label: "成长", Format: "score", Description: "盈利增长、收入增长与近一年涨幅排名分加权后的成长得分。"},
	{Key: "quality", Label: "质量", Format: "score", Description: "ROE、经营现金流率与资产权益比排名分加权后的质量得分。"},
	{Key: "momentum", Label: "动量", Format: "score", Description: "近一年涨幅与近一月动量排名分加权后的动量得分。"},
	{Key: "size", Label: "规模", Format: "score", Description: "市值排名分，小市值得分更高。"},
	{Key: "low_volatility", Label: "低波动", Format: "score", Description: "近一月波动率与近一年 Beta 排名分加权后的低波动得分。"},
}

var factorKeyToScoreField = map[string]string{
	"value":          "value_score",
	"dividend_yield": "dividend_yield_score",
	"growth":         "growth_score",
	"quality":        "quality_score",
	"momentum":       "momentum_score",
	"size":           "size_score",
	"low_volatility": "low_volatility_score",
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Meta(ctx context.Context) (FactorLabMetaResponse, error) {
	date, err := s.repo.LatestSnapshotDate(ctx)
	if err != nil {
		return FactorLabMetaResponse{}, err
	}
	lastRun, err := s.repo.LastDailyComputeRun(ctx)
	if err != nil {
		return FactorLabMetaResponse{}, err
	}
	if strings.TrimSpace(date) == "" {
		return FactorLabMetaResponse{HasSnapshot: false, Factors: buildFactorDefinitions(nil), Metrics: buildMetricGroups(nil)}, nil
	}
	stats, err := s.repo.SnapshotStats(ctx, date)
	if err != nil {
		return FactorLabMetaResponse{}, err
	}
	coverage, err := s.repo.MetricCoverage(ctx, date)
	if err != nil {
		return FactorLabMetaResponse{}, err
	}
	return FactorLabMetaResponse{
		HasSnapshot:  true,
		SnapshotDate: date,
		Stale:        isSnapshotStale(date),
		Universe: FactorLabUniverseMeta{
			Total:         stats.Total,
			NewStockCount: stats.NewStockCount,
		},
		Coverage: coverage,
		Factors:  buildFactorDefinitions(coverage),
		Metrics:  buildMetricGroups(nil),
		LastRun:  taskRunToMeta(lastRun),
	}, nil
}

func (s *Service) AdminStatus(ctx context.Context, worker WorkerStatus) (FactorPipelineAdminStatus, error) {
	date, err := s.repo.LatestSnapshotDate(ctx)
	if err != nil {
		return FactorPipelineAdminStatus{}, err
	}
	coverage, err := s.Coverage(ctx, date)
	if err != nil {
		return FactorPipelineAdminStatus{}, err
	}
	dbHealth, err := s.repo.DBQuickCheck(ctx)
	if err != nil {
		dbHealth = "failed: " + err.Error()
	}
	return FactorPipelineAdminStatus{Worker: worker, DBHealth: dbHealth, LatestSnapshot: date, Coverage: coverage}, nil
}

func (s *Service) Coverage(ctx context.Context, snapshotDate string) (FactorCoverageResponse, error) {
	date := strings.TrimSpace(snapshotDate)
	if date == "" {
		latest, err := s.repo.LatestSnapshotDate(ctx)
		if err != nil {
			return FactorCoverageResponse{}, err
		}
		date = latest
	}
	if date == "" {
		return FactorCoverageResponse{RawMetrics: map[string]int64{}, Factors: map[string]int64{}}, nil
	}
	stats, err := s.repo.SnapshotStats(ctx, date)
	if err != nil {
		return FactorCoverageResponse{}, err
	}
	factors, err := s.repo.MetricCoverage(ctx, date)
	if err != nil {
		return FactorCoverageResponse{}, err
	}
	raw, err := s.repo.RawMetricCoverage(ctx, date)
	if err != nil {
		return FactorCoverageResponse{}, err
	}
	warnings := buildCoverageWarnings(stats.Total, raw, factors)
	return FactorCoverageResponse{SnapshotDate: date, Universe: stats.Total, RawMetrics: raw, Factors: factors, Warnings: warnings}, nil
}

func buildCoverageWarnings(total int64, raw, factors map[string]int64) []string {
	if total <= 0 {
		return []string{}
	}
	warnings := []string{}
	for _, key := range []string{"dividend_yield", "performance_1y", "operating_cf_margin"} {
		if float64(raw[key])/float64(total) < 0.8 {
			warnings = append(warnings, fmt.Sprintf("%s 覆盖率低于 80%%", key))
		}
	}
	for _, key := range []string{"value_score", "growth_score", "quality_score", "momentum_score", "size_score", "low_volatility_score"} {
		if float64(factors[key])/float64(total) < 0.8 {
			warnings = append(warnings, fmt.Sprintf("%s 覆盖率低于 80%%", key))
		}
	}
	return warnings
}

func (s *Service) Screen(ctx context.Context, req FactorScreenerRequest) (FactorScreenerResponse, error) {
	input, err := s.normalizeScreenerRequest(ctx, req)
	if err != nil {
		return FactorScreenerResponse{}, err
	}
	result, err := s.repo.ScanSnapshots(ctx, input)
	if err != nil {
		return FactorScreenerResponse{}, err
	}
	items := make([]FactorScreenerItem, 0, len(result.Items))
	for _, record := range result.Items {
		items = append(items, scoreToItem(record, input.FactorWeights))
	}
	sortItems(items, input.SortBy, input.SortOrder)
	start := (input.Page - 1) * input.PageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + input.PageSize
	if end > len(items) {
		end = len(items)
	}
	return FactorScreenerResponse{
		SnapshotDate: input.SnapshotDate,
		Total:        int64(len(items)),
		Page:         input.Page,
		PageSize:     input.PageSize,
		Items:        items[start:end],
	}, nil
}

func (s *Service) normalizeScreenerRequest(ctx context.Context, req FactorScreenerRequest) (ScanInput, error) {
	date := strings.TrimSpace(req.SnapshotDate)
	if date == "" {
		latest, err := s.repo.LatestSnapshotDate(ctx)
		if err != nil {
			return ScanInput{}, err
		}
		date = latest
	}
	if date == "" {
		return ScanInput{}, fmt.Errorf("因子快照尚未生成")
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	weights, err := normalizeFactorWeights(req.FactorWeights)
	if err != nil {
		return ScanInput{}, err
	}
	sortBy := strings.TrimSpace(req.SortBy)
	if sortBy == "" {
		sortBy = "composite_score"
	}
	sortOrder := strings.TrimSpace(req.SortOrder)
	if !strings.EqualFold(sortOrder, "asc") && !strings.EqualFold(sortOrder, "desc") {
		sortOrder = "desc"
	}
	return ScanInput{SnapshotDate: date, FactorWeights: weights, SortBy: sortBy, SortOrder: sortOrder, Page: page, PageSize: pageSize}, nil
}

func normalizeFactorWeights(raw map[string]float64) (map[string]float64, error) {
	if len(raw) == 0 {
		return equalFactorWeights(), nil
	}
	weights := make(map[string]float64)
	sum := 0.0
	for key, weight := range raw {
		key = strings.TrimSpace(key)
		if _, ok := factorKeyToScoreField[key]; !ok {
			return nil, fmt.Errorf("不支持的因子: %s", key)
		}
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 || weight > 100 {
			return nil, fmt.Errorf("%s 的权重必须是 0 到 100 之间的非负百分比", key)
		}
		if !hasAtMostTwoDecimals(weight) {
			return nil, fmt.Errorf("%s 的权重最多保留 2 位小数", key)
		}
		if weight == 0 {
			continue
		}
		weights[key] = weight
		sum += weight
	}
	if len(weights) == 0 {
		return nil, fmt.Errorf("请至少选择一个因子并填写权重")
	}
	if math.Abs(sum-100) > weightTolerance {
		return nil, fmt.Errorf("因子权重合计必须等于 100%%")
	}
	return weights, nil
}

func hasAtMostTwoDecimals(weight float64) bool {
	scaled := weight * weightPrecision
	return math.Abs(scaled-math.Round(scaled)) <= weightTolerance
}

func equalFactorWeights() map[string]float64 {
	weights := make(map[string]float64, len(factorDefinitions))
	weight := 100.0 / float64(len(factorDefinitions))
	for _, factor := range factorDefinitions {
		weights[factor.Key] = weight
	}
	return weights
}

func scoreToItem(record FactorScore, weights map[string]float64) FactorScreenerItem {
	item := FactorScreenerItem{
		SnapshotDate:       record.SnapshotDate,
		Code:               record.Code,
		Symbol:             record.Symbol,
		Name:               record.Name,
		Industry:           record.Industry,
		IsNewStock:         record.IsNewStock,
		ClosePrice:         record.ClosePrice,
		ValueScore:         record.ValueScore,
		DividendYieldScore: record.DividendYieldScore,
		GrowthScore:        record.GrowthScore,
		QualityScore:       record.QualityScore,
		MomentumScore:      record.MomentumScore,
		SizeScore:          record.SizeScore,
		LowVolatilityScore: record.LowVolatilityScore,
	}
	item.CompositeScore = compositeScore(item, weights)
	return item
}

func compositeScore(item FactorScreenerItem, weights map[string]float64) *float64 {
	numerator := 0.0
	denominator := 0.0
	for key, weight := range weights {
		score := item.factorScore(key)
		if score == nil {
			continue
		}
		numerator += *score * weight
		denominator += weight
	}
	if denominator == 0 {
		return nil
	}
	value := numerator / denominator
	return &value
}

func (item FactorScreenerItem) factorScore(key string) *float64 {
	switch key {
	case "value", "value_score":
		return item.ValueScore
	case "dividend_yield", "dividend_yield_score":
		return item.DividendYieldScore
	case "growth", "growth_score":
		return item.GrowthScore
	case "quality", "quality_score":
		return item.QualityScore
	case "momentum", "momentum_score":
		return item.MomentumScore
	case "size", "size_score":
		return item.SizeScore
	case "low_volatility", "low_volatility_score":
		return item.LowVolatilityScore
	case "composite_score":
		return item.CompositeScore
	default:
		return nil
	}
}

func sortItems(items []FactorScreenerItem, sortBy, sortOrder string) {
	desc := !strings.EqualFold(sortOrder, "asc")
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if sortBy == "code" {
			return compareString(left.Code, right.Code, desc)
		}
		if sortBy == "name" {
			return compareString(left.Name, right.Name, desc)
		}
		if sortBy == "industry" {
			return compareString(left.Industry, right.Industry, desc)
		}
		if sortBy == "close_price" {
			return compareFloat(&left.ClosePrice, &right.ClosePrice, desc, left.Code, right.Code)
		}
		return compareFloat(left.factorScore(sortBy), right.factorScore(sortBy), desc, left.Code, right.Code)
	})
}

func compareString(left, right string, desc bool) bool {
	if left == right {
		return false
	}
	if desc {
		return left > right
	}
	return left < right
}

func compareFloat(left, right *float64, desc bool, leftCode, rightCode string) bool {
	if left == nil && right == nil {
		return leftCode < rightCode
	}
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	if *left == *right {
		return leftCode < rightCode
	}
	if desc {
		return *left > *right
	}
	return *left < *right
}

func taskRunToMeta(run *FactorTaskRun) FactorTaskRunMeta {
	if run == nil {
		return FactorTaskRunMeta{}
	}
	return FactorTaskRunMeta{
		ID:           run.ID,
		Status:       run.Status,
		SnapshotDate: run.SnapshotDate,
		StartedAt:    &run.StartedAt,
		FinishedAt:   run.FinishedAt,
		ErrorMessage: run.ErrorMessage,
	}
}

func isSnapshotStale(snapshotDate string) bool {
	parsed, err := time.Parse("2006-01-02", snapshotDate)
	if err != nil {
		return false
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().In(loc)
	return now.Sub(parsed.In(loc)) > 96*time.Hour
}

func buildFactorDefinitions(coverage map[string]int64) []FactorDefinition {
	defaultWeight := 100.0 / float64(len(factorDefinitions))
	out := make([]FactorDefinition, 0, len(factorDefinitions))
	for _, factor := range factorDefinitions {
		copy := factor
		copy.DefaultWeight = defaultWeight
		if coverage != nil {
			copy.Coverage = coverage[factorKeyToScoreField[factor.Key]]
		}
		out = append(out, copy)
	}
	return out
}

func buildMetricGroups(coverage map[string]int64) []FactorMetricGroup {
	return []FactorMetricGroup{}
}
