package factorlab

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

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
		return FactorLabMetaResponse{HasSnapshot: false, Metrics: buildMetricGroups(nil)}, nil
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
		Metrics:  buildMetricGroups(coverage),
		LastRun:  taskRunToMeta(lastRun),
	}, nil
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
		items = append(items, snapshotToItem(record))
	}
	return FactorScreenerResponse{
		SnapshotDate: input.SnapshotDate,
		Total:        result.Total,
		Page:         input.Page,
		PageSize:     input.PageSize,
		Items:        items,
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
	filters := make(map[string]FactorFilterRange)
	for key, bounds := range req.Filters {
		key = strings.TrimSpace(key)
		if _, ok := metricColumns[key]; !ok {
			return ScanInput{}, fmt.Errorf("不支持的因子指标: %s", key)
		}
		if bounds.Min == nil && bounds.Max == nil {
			continue
		}
		normalized := normalizeFilterRange(key, bounds)
		if normalized.Min != nil && normalized.Max != nil && *normalized.Min > *normalized.Max {
			return ScanInput{}, fmt.Errorf("%s 的最小值不能大于最大值", key)
		}
		filters[key] = normalized
	}
	return ScanInput{
		SnapshotDate: date,
		Filters:      filters,
		SortBy:       strings.TrimSpace(req.SortBy),
		SortOrder:    strings.TrimSpace(req.SortOrder),
		Page:         page,
		PageSize:     pageSize,
	}, nil
}

func normalizeFilterRange(key string, bounds FactorFilterRange) FactorFilterRange {
	if key != "dividend_yield" {
		return bounds
	}
	var out FactorFilterRange
	if bounds.Min != nil {
		v := *bounds.Min / 100
		out.Min = &v
	}
	if bounds.Max != nil {
		v := *bounds.Max / 100
		out.Max = &v
	}
	return out
}

func snapshotToItem(record FactorSnapshot) FactorScreenerItem {
	return FactorScreenerItem{
		SnapshotDate:            record.SnapshotDate,
		Code:                    record.Code,
		Symbol:                  record.Symbol,
		Name:                    record.Name,
		Board:                   record.Board,
		ListingAgeDays:          record.ListingAgeDays,
		IsNewStock:              record.IsNewStock,
		AvailableTradingDays:    record.AvailableTradingDays,
		ClosePrice:              record.ClosePrice,
		MarketCap:               record.MarketCap,
		PE:                      record.PE,
		PB:                      record.PB,
		PS:                      record.PS,
		DividendYield:           record.DividendYield,
		EarningGrowth:           record.EarningGrowth,
		RevenueGrowth:           record.RevenueGrowth,
		Performance1Y:           record.Performance1Y,
		PerformanceSinceListing: record.PerformanceSinceListing,
		Momentum1M:              record.Momentum1M,
		ROE:                     record.ROE,
		OperatingCFMargin:       record.OperatingCFMargin,
		AssetToEquity:           record.AssetToEquity,
		Volatility1M:            record.Volatility1M,
		Beta1Y:                  record.Beta1Y,
		DataQualityFlags:        parseFlags(record.DataQualityFlags),
	}
}

func parseFlags(raw string) []string {
	var flags []string
	if err := json.Unmarshal([]byte(raw), &flags); err != nil {
		return []string{}
	}
	return flags
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

func buildMetricGroups(coverage map[string]int64) []FactorMetricGroup {
	cover := func(key string) int64 {
		if coverage == nil {
			return 0
		}
		return coverage[key]
	}
	return []FactorMetricGroup{
		{Key: "value", Label: "价值", Items: []FactorMetricDefinition{
			{Key: "pe", Label: "PE", Unit: "倍", Format: "number", Direction: DirectionLowerBetter, Description: "市盈率，越低通常代表估值越便宜；亏损股 PE 可能不可用。", Coverage: cover("pe")},
			{Key: "pb", Label: "PB", Unit: "倍", Format: "number", Direction: DirectionLowerBetter, Description: "市净率，衡量股价相对净资产的估值水平。", Coverage: cover("pb")},
			{Key: "ps", Label: "PS", Unit: "倍", Format: "number", Direction: DirectionLowerBetter, Description: "市销率，使用市值除以营业收入。", Coverage: cover("ps")},
		}},
		{Key: "dividend", Label: "股息率", Items: []FactorMetricDefinition{
			{Key: "dividend_yield", Label: "股息率", Unit: "%", Format: "percentFromRatio", Direction: DirectionHigherBetter, Description: "现金分红相对市值或股价的收益率；输入 3 表示 3%。", Coverage: cover("dividend_yield")},
		}},
		{Key: "growth", Label: "成长", Items: []FactorMetricDefinition{
			{Key: "earning_growth", Label: "盈利增长", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "最新报告期净利润同比增长。", Coverage: cover("earning_growth")},
			{Key: "revenue_growth", Label: "收入增长", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "最新报告期营业收入同比增长。", Coverage: cover("revenue_growth")},
			{Key: "performance_1y", Label: "近一年涨幅", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "最近约 250 个交易日涨跌幅；上市不足一年可能缺失。", Coverage: cover("performance_1y")},
			{Key: "performance_since_listing", Label: "上市以来涨幅", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "基于本地可用日线的上市以来涨跌幅。", Coverage: cover("performance_since_listing")},
		}},
		{Key: "quality", Label: "质量", Items: []FactorMetricDefinition{
			{Key: "roe", Label: "ROE", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "净利润 / 股东权益。", Coverage: cover("roe")},
			{Key: "operating_cf_margin", Label: "经营现金流率", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "经营活动现金流净额 / 营业收入。", Coverage: cover("operating_cf_margin")},
			{Key: "asset_to_equity", Label: "资产权益比", Unit: "倍", Format: "number", Direction: DirectionLowerBetter, Description: "总资产 / 股东权益，用于观察杠杆水平。", Coverage: cover("asset_to_equity")},
		}},
		{Key: "momentum", Label: "动量", Items: []FactorMetricDefinition{
			{Key: "momentum_1m", Label: "近一月动量", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "最近约 20 个交易日涨跌幅。", Coverage: cover("momentum_1m")},
			{Key: "performance_1y", Label: "近一年涨幅", Unit: "%", Format: "percent", Direction: DirectionHigherBetter, Description: "最近约 250 个交易日涨跌幅。", Coverage: cover("performance_1y")},
		}},
		{Key: "size", Label: "规模", Items: []FactorMetricDefinition{
			{Key: "market_cap", Label: "总市值", Unit: "元", Format: "bigNumber", Direction: DirectionNeutral, Description: "公司总市值，筛选时可用于大盘/小盘风格。", Coverage: cover("market_cap")},
		}},
		{Key: "low_volatility", Label: "低波动", Items: []FactorMetricDefinition{
			{Key: "volatility_1m", Label: "近一月波动率", Unit: "%", Format: "percent", Direction: DirectionLowerBetter, Description: "最近约 20 个交易日日收益标准差年化。", Coverage: cover("volatility_1m")},
			{Key: "beta_1y", Label: "近一年 Beta", Unit: "倍", Format: "number", Direction: DirectionLowerBetter, Description: "近一年相对中证全指的市场敏感度。", Coverage: cover("beta_1y")},
		}},
	}
}
